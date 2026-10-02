"""Host-independent refusal, slot isolation and subprocess lifecycle checks."""
import os
import json
import io
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
from commands import CommandFailed, run_command
from check import accepted
from scenario import Refused, plan, require_implemented, slot_values, validate_environment, validate_lease

HERE = Path(__file__).resolve().parent


class Foundation(unittest.TestCase):
    def test_plan_never_claims_chain_or_stage_pass(self):
        result = plan(14)
        self.assertEqual(result['status'], 'NOTIMPLEMENTED')
        self.assertFalse(result['whole_chain_proven'])
        self.assertEqual(len(result['stages']), 7)
        self.assertTrue(all(stage['status'] == 'NOTIMPLEMENTED' for stage in result['stages']))
        self.assertEqual(result['captures'][0], {'side': 'lan', 'namespace': 'ns-w14-lan', 'device': 'w14l1'})
        with self.assertRaisesRegex(Refused, 'composed executors'):
            require_implemented()

    def test_reserved_invalid_slots_and_cross_slot_environment_refused(self):
        for slot in (0, 12, 13, 33, True, '3'):
            with self.subTest(slot=slot), self.assertRaises(Refused):
                slot_values(slot)
        env = dict(slot_values(14), VRX_INTEGRATION='1', VRX_TRAFFIC_A_HOST='1')
        self.assertEqual(validate_environment(env, 14), slot_values(14))
        for key, value in [('VRX_TEST_PREFIX', 'w15'), ('VRX_HTTP_PORT', '4400'), ('VRX_GLOBALS_OWNER', '1'),
                           ('VRX_VPP_ID_RANGE', 'all'), ('VRX_INTEGRATION', '0'), ('VRX_NAT64_TENANT_VRF_HOST', '1')]:
            with self.subTest(key=key), self.assertRaises(Refused):
                validate_environment(dict(env, **{key: value}), 14)

    def test_manager_lease_identity_expiry_mode_and_symlink_boundaries(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); physical=root/'lease.json'
            # Map the fixed host path to a real temporary file. No /run writes.
            class FixturePath:
                owner=0  # Model manager UID; actual file mode/type/content remain real.
                def __str__(self):return '/run/vrx-test/w3/traffic-a-lease.json'
                def lstat(self):
                    info=list(physical.lstat());info[4]=self.owner
                    return os.stat_result(info)
                def read_text(self):return physical.read_text()
            data=dict(task='TEST-traffic-A',slot=3,prefix='w3',boot_id='fixture-boot',expires_unix=101,lease_id='a'*32)
            physical.write_text(json.dumps(data));physical.chmod(0o600)
            self.assertEqual(validate_lease(FixturePath(),3,'fixture-boot',now=100),data)
            for changes in ({'expires_unix':100},{'expires_unix':7301},{'boot_id':'other-boot'},
                            {'slot':4},{'task':'other-task'},{'lease_id':'malformed'}):
                with self.subTest(changes=changes):
                    physical.write_text(json.dumps(dict(data,**changes)))
                    with self.assertRaises(Refused):validate_lease(FixturePath(),3,'fixture-boot',now=100)
            physical.write_text(json.dumps(data))
            FixturePath.owner=1000
            with self.assertRaises(Refused):validate_lease(FixturePath(),3,'fixture-boot',now=100)
            FixturePath.owner=0
            physical.chmod(0o644)
            with self.assertRaises(Refused):validate_lease(FixturePath(),3,'fixture-boot',now=100)
            foreign=root/'foreign';physical.rename(foreign);physical.symlink_to(foreign)
            with self.assertRaises(Refused):validate_lease(FixturePath(),3,'fixture-boot',now=100)
            with self.assertRaises(Refused):validate_lease(FixturePath(),12,'fixture-boot',now=100)

    def test_live_cli_refuses_before_any_external_command(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); calls = root / 'calls'
            for name in ('go', 'ip', 'tcpdump', 'vppctl', 'ssh', 'flock'):
                stub = root / name
                stub.write_text('#!/bin/sh\nprintf forbidden >> "$CALL_LOG"\nexit 99\n')
                stub.chmod(0o755)
            env = dict(os.environ, **slot_values(3), VRX_INTEGRATION='1', VRX_TRAFFIC_A_HOST='1',
                       PATH=str(root), CALL_LOG=str(calls))
            result = subprocess.run([sys.executable, str(HERE / 'run.py'), 'run', '--slot', '3'],
                                    env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 1)
            self.assertIn('NOTIMPLEMENTED', result.stderr)
            self.assertFalse(calls.exists())

    def test_nonzero_exit_is_not_packet_pass_and_private_log_retained(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'log'
            with self.assertRaisesRegex(CommandFailed, 'exited 7'):
                run_command([sys.executable, '-c', 'print("fixture failure");raise SystemExit(7)'],
                            directory, dict(os.environ), output, 2)
            self.assertIn('fixture failure', output.read_text())
            self.assertEqual(output.stat().st_mode & 0o777, 0o600)
            with self.assertRaises(FileExistsError):
                run_command([sys.executable, '-c', 'pass'], directory, dict(os.environ), output, 2)

    def test_timeout_stops_our_process_and_refuses_success(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'log'
            started = time.monotonic()
            with self.assertRaises(subprocess.TimeoutExpired):
                run_command([sys.executable, '-c', 'import time;print("started",flush=True);time.sleep(20)'],
                            directory, dict(os.environ), output, 0.15)
            self.assertLess(time.monotonic() - started, 4)
            self.assertIn('started', output.read_text())

    def test_timeout_stops_descendant_that_ignores_term(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'log'
            child = 'import signal,time,os,pathlib;signal.signal(signal.SIGTERM,signal.SIG_IGN);birth=pathlib.Path("/proc/self/stat").read_text().rsplit(")",1)[1].split()[19];print("child="+str(os.getpid())+" birth="+birth,flush=True);time.sleep(20)'
            leader = f'import subprocess,sys,time;subprocess.Popen([sys.executable,"-c",{child!r}]);time.sleep(20)'
            pid = birth = None
            def same_child():
                if pid is None:return False
                try:
                    return Path(f'/proc/{pid}/stat').read_text().rsplit(')',1)[1].split()[19] == birth
                except FileNotFoundError:return False
            original_popen = subprocess.Popen
            def ready_process(*arguments, **keywords):
                process = original_popen(*arguments, **keywords)
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline:
                    if output.exists() and 'birth=' in output.read_text():return process
                    time.sleep(0.01)
                # The fixture must prove its child installed SIG_IGN before
                # starting the production wait/timeout. Clean only our group.
                os.killpg(process.pid, 9);process.wait(timeout=2)
                raise AssertionError('fixture descendant never announced readiness')
            try:
                with patch('commands.subprocess.Popen', ready_process):
                    with self.assertRaises(subprocess.TimeoutExpired):
                        run_command([sys.executable, '-c', leader], directory, dict(os.environ), output, 0.15)
                identity = output.read_text().strip().split()
                pid = int(identity[0].split('=')[1]);birth = identity[1].split('=')[1]
                status = Path(f'/proc/{pid}/status')
                deadline = time.monotonic() + 2
                while same_child() and status.exists() and '\nState:\tZ' not in status.read_text() and time.monotonic() < deadline:
                    time.sleep(0.01)
                self.assertTrue(not same_child() or not status.exists() or '\nState:\tZ' in status.read_text(),
                                'own descendant remains executing: '+(status.read_text() if status.exists() else 'gone'))
            finally:
                # Never signal a PID that has been reused by another worker.
                if same_child():
                    try: os.kill(pid, 9)
                    except ProcessLookupError: pass

    def test_strict_source_gate_rejects_every_nonpassing_status(self):
        bodies={'pass':'pass','failure':'self.fail("fixture")','error':'raise RuntimeError("fixture")',
                'skip':'self.skipTest("fixture")','expectedFailure':'self.fail("fixture")','unexpectedSuccess':'pass'}
        for name, body in bodies.items():
            scope={'unittest':unittest}
            decorator='    @unittest.expectedFailure\n' if name in ('expectedFailure','unexpectedSuccess') else ''
            exec('class Fixture(unittest.TestCase):\n'+decorator+'    def test_case(self):\n        '+body+'\n',scope)
            result=unittest.TextTestRunner(stream=io.StringIO()).run(unittest.defaultTestLoader.loadTestsFromTestCase(scope['Fixture']))
            self.assertEqual(accepted(result),name=='pass',name)
        result=unittest.TextTestRunner(stream=io.StringIO()).run(unittest.TestSuite())
        self.assertFalse(accepted(result))

    def test_log_symlink_and_invalid_timeout_refused_without_execution(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory); foreign = root / 'foreign'; foreign.write_text('unchanged')
            link = root / 'log'; link.symlink_to(foreign)
            with self.assertRaises(FileExistsError):
                run_command([sys.executable, '-c', 'pass'], directory, dict(os.environ), link, 2)
            self.assertEqual(foreign.read_text(), 'unchanged')
            with self.assertRaises(CommandFailed):
                run_command([sys.executable, '-c', 'pass'], directory, dict(os.environ), root / 'new', 1201)
            self.assertFalse((root / 'new').exists())


if __name__ == '__main__':
    unittest.main(verbosity=2)
