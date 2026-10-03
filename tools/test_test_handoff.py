"""Test isolated scheduling, stale trees and process-group cleanup; no host lab."""
import fcntl
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

TOOLS = Path(__file__).resolve().parent


class HandoffTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(dir='/root/.cache')
        self.root = Path(self.temp.name)
        self.repo = self.root / 'repo'
        self.repo.mkdir()
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        (self.repo / 'source').write_text('fixed\n')
        subprocess.run(['git', '-C', str(self.repo), 'add', 'source'], check=True)
        subprocess.run(['git', '-C', str(self.repo), '-c', 'user.name=fixture',
                        '-c', 'user.email=fixture@example.test', 'commit', '-qm', 'fixture'], check=True)
        self.head = subprocess.check_output(['git', '-C', str(self.repo), 'rev-parse', 'HEAD'], text=True).strip()
        self.locks = self.root / 'locks'
        self.locks.mkdir()
        (self.locks / 'ngfw-heavy-max').write_text('2\n')
        self.env = os.environ.copy()
        self.env.pop('NGFW_HEAVY_HELD', None)
        self.env.pop('NGFW_INTEGRATION', None)
        self.env.pop('VRX_HEAVY_HELD', None)
        self.env.pop('VRX_INTEGRATION', None)
        self.env['NGFW_TEST_LOCK_DIR'] = str(self.locks)
        self.env['NGFW_FAST_TIMEOUT_SECONDS'] = '30'

    def tearDown(self):
        self.temp.cleanup()

    def worker(self, command, deadline=15, before=None, observe_cleanup=False, force_cleanup_timeout=False):
        path = self.root / 'job.json'
        path.write_text(json.dumps(dict(cwd=str(self.repo), head=self.head, lane='fast',
                                       command=command, deadline_seconds=deadline,
                                       state='submitted', log=str(self.root / 'job.log'))))
        if before:
            before()
        worker_command = [sys.executable, str(TOOLS / 'test-handoff.py'), '_run', str(path)]
        if observe_cleanup or force_cleanup_timeout:
            # Observe the actual live child's state at final persistence, not
            # merely after the worker has exited. Timeout probe fails closed.
            runner = (
                'import importlib.util,json; from pathlib import Path\n'
                f's=importlib.util.spec_from_file_location("worker",{str(TOOLS / "test-handoff.py")!r})\n'
                'm=importlib.util.module_from_spec(s); s.loader.exec_module(m)\n'
                'original=m.save\n'
                'def observed(path,data):\n'
                '    if "finished" in data:\n'
                f'        child=int(Path({str(self.root / "cleanup-child.pid")!r}).read_text())\n'
                '        stat=Path(f"/proc/{child}/stat")\n'
                '        data["child_state_at_final_save"]=stat.read_text().rsplit(")",1)[1].split()[0] if stat.exists() else "absent"\n'
                '    original(path,data)\n'
                'm.save=observed\n'
            )
            if force_cleanup_timeout:
                runner += 'm.wait_group=lambda process, seconds: False\n'
            runner += f'm.run(Path({str(path)!r}))\n'
            worker_command = [sys.executable, '-c', runner]
        subprocess.run(worker_command, env=self.env, check=True, timeout=30)
        return json.loads(path.read_text())

    def test_fast_can_run_while_two_long_slots_locked(self):
        with open(self.locks / 'ngfw-heavy-1.lock', 'w') as one, open(self.locks / 'ngfw-heavy-2.lock', 'w') as two:
            fcntl.flock(one, fcntl.LOCK_EX)
            fcntl.flock(two, fcntl.LOCK_EX)
            result = self.worker([sys.executable, '-c', 'print("done")'])
        self.assertEqual(result['state'], 'passed')
        self.assertIn('slot 3', (self.root / 'job.log').read_text())

    def test_nonzero_is_failed(self):
        result = self.worker([sys.executable, '-c', 'raise SystemExit(7)'])
        self.assertEqual((result['state'], result['exit_code']), ('failed', 7))

    def test_dirty_checkpoint_never_runs(self):
        result = self.worker([sys.executable, '-c', 'raise Exception("must not run")'],
                             before=lambda: (self.repo / 'source').write_text('changed'))
        self.assertEqual(result['state'], 'stale')
        self.assertFalse((self.root / 'job.log').exists())

    def test_untracked_source_never_runs(self):
        result = self.worker([sys.executable, '-c', 'pass'],
                             before=lambda: (self.repo / 'extra.py').write_text('pass'))
        self.assertEqual(result['state'], 'stale')

    def test_mutation_during_check_invalidates_result(self):
        result = self.worker([sys.executable, '-c', 'from pathlib import Path; Path("source").write_text("changed")'])
        self.assertEqual(result['state'], 'stale')

    def test_emergency_cap_and_live_integration_are_refused(self):
        (self.locks / 'ngfw-heavy-max').write_text('1\n')
        result = self.worker([sys.executable, '-c', 'pass'])
        self.assertEqual((result['state'], result['exit_code']), ('failed', 2))
        (self.locks / 'ngfw-heavy-max').write_text('2\n')
        self.env['NGFW_INTEGRATION'] = '1'
        direct = subprocess.run([str(TOOLS / 'test-fast.sh'), 'true'], env=self.env,
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.assertEqual(direct.returncode, 2)

    def test_legacy_inherited_flags_cannot_enable_host_tests_or_bypass_locks(self):
        flags = ('NGFW_INTEGRATION', 'VRX_INTEGRATION',
                 'NGFW_HEAVY_HELD', 'VRX_HEAVY_HELD')
        for flag in flags:
            with self.subTest(flag=flag):
                env = dict(self.env, **{flag: '1'})
                direct = subprocess.run([str(TOOLS / 'test-fast.sh'), 'true'], env=env,
                                        stdout=subprocess.PIPE, stderr=subprocess.PIPE)
                self.assertEqual(direct.returncode, 2)
        self.env.update({flag: '1' for flag in flags})
        blocked = ('NGFW_INTEGRATION', 'VRX_INTEGRATION', 'VRX_HEAVY_HELD')
        command = ('import os; flags=' + repr(blocked) +
                   '; assert not any(flag in os.environ for flag in flags); '
                   'assert os.environ.get("NGFW_HEAVY_HELD") == "3"')
        result = self.worker([sys.executable, '-c', command])
        self.assertEqual(result['state'], 'passed')

    def cleanup_mutator(self, mutation, **worker_options):
        # The child signals readiness before the successful parent exits. It
        # mutates only on cleanup's TERM, after the worker's first clean check.
        ready = self.root / 'cleanup-child.ready'
        pidfile = self.root / 'cleanup-child.pid'
        child = (
            'import signal,time; from pathlib import Path\n'
            'def cleanup(signum, frame):\n'
            '    signal.signal(signal.SIGTERM, signal.SIG_IGN)\n'
            f'    {mutation}\n'
            'signal.signal(signal.SIGTERM, cleanup)\n'
            f'Path({str(ready)!r}).write_text("ready")\n'
            'while True: time.sleep(1)\n'
        )
        parent = (
            'import subprocess,sys,time; from pathlib import Path\n'
            f'p=subprocess.Popen([sys.executable,"-c",{child!r}])\n'
            f'Path({str(pidfile)!r}).write_text(str(p.pid))\n'
            'deadline=time.monotonic()+5\n'
            f'while not Path({str(ready)!r}).exists():\n'
            '    if p.poll() is not None or time.monotonic()>deadline: raise RuntimeError("child not ready")\n'
            '    time.sleep(0.01)\n'
        )
        result = self.worker([sys.executable, '-c', parent], **worker_options)
        self.assertEqual(result['exit_code'], 0)  # main validation succeeded
        pid = int(pidfile.read_text())
        stat = Path(f'/proc/{pid}/stat')
        if stat.exists():
            self.assertEqual(stat.read_text().split()[2], 'Z', 'cleanup descendant must be dead')
        with open(self.locks / 'ngfw-heavy-3.lock', 'w') as slot:
            fcntl.flock(slot, fcntl.LOCK_EX | fcntl.LOCK_NB)
        return result

    def test_cleanup_descendant_mutation_invalidates_success(self):
        result = self.cleanup_mutator('Path("source").write_text("changed during cleanup")')
        self.assertEqual((self.repo / 'source').read_text(), 'changed during cleanup')
        self.assertEqual(result['state'], 'stale')

    def test_cleanup_verification_error_fails_closed(self):
        result = self.cleanup_mutator('Path(".git").rename(".git-after-cleanup")')
        self.assertFalse((self.repo / '.git').exists())
        self.assertEqual(result['state'], 'failed')
        self.assertIn('Final checkpoint verification failed', result['error'])

    def test_success_is_saved_only_after_real_child_is_quiescent(self):
        result = self.cleanup_mutator('pass', observe_cleanup=True)
        self.assertEqual(result['state'], 'passed')
        self.assertIn(result['child_state_at_final_save'], ('Z', 'X', 'absent'))

    def test_cleanup_quiescence_timeout_cannot_seal_success(self):
        result = self.cleanup_mutator('pass', force_cleanup_timeout=True)
        self.assertEqual(result['state'], 'failed')
        self.assertIn('Process cleanup verification failed', result['error'])
        self.assertIn('quiescent', result['error'])

    def test_deadline_cleans_stubborn_descendant_and_releases_lock(self):
        pidfile = self.root / 'child.pid'
        child = 'import signal,time; signal.signal(signal.SIGTERM,signal.SIG_IGN); time.sleep(60)'
        parent = f'import subprocess,sys,time; from pathlib import Path; p=subprocess.Popen([sys.executable,"-c",{child!r}]); Path({str(pidfile)!r}).write_text(str(p.pid)); time.sleep(60)'
        result = self.worker([sys.executable, '-c', parent], deadline=1)
        self.assertEqual(result['state'], 'failed')
        pid = int(pidfile.read_text())
        stat = Path(f'/proc/{pid}/stat')
        if stat.exists():
            self.assertEqual(stat.read_text().split()[2], 'Z', 'descendant must be dead')
        with open(self.locks / 'ngfw-heavy-3.lock', 'w') as slot:
            fcntl.flock(slot, fcntl.LOCK_EX | fcntl.LOCK_NB)


if __name__ == '__main__':
    unittest.main()
