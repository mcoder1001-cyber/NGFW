"""Actual temporary subprocess capture fixtures; never execute ip or tcpdump."""
import os
import json
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

from evidence import capture
from producer import capture_argv, produce
from scenario import Refused


class Producer(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.root = Path(self.temp.name); self.root.chmod(0o700)
        self.processes = []
        self.argv_seen = []

    def tearDown(self):
        for process in self.processes:
            if process.poll() is None:
                os.killpg(process.pid, signal.SIGKILL); process.wait(timeout=2)
        self.temp.cleanup()

    def executor(self, source):
        # Test-only executable is selected explicitly, never inferred from argv.
        stub = self.root/'stub.py'; stub.write_text(source)
        def start(argv, *, environment):
            self.argv_seen.append((argv, environment))
            process = subprocess.Popen([sys.executable, '-B', str(stub)],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, stdin=subprocess.DEVNULL,
                start_new_session=True, close_fds=True)
            self.processes.append(process)
            return process
        return start

    def good_source(self, accounting=None):
        # Generate a correctly checksummed packet at capture time in the stub.
        module = str(Path(__file__).parent)
        counters = accounting if accounting is not None else '1 packets captured\n1 packets received by filter\n0 packets dropped by kernel\n'
        return (f'import sys,time\nsys.path.insert(0,{module!r})\n'
                'from test_correlation import HEADER,packet,FLOW\n'
                'sys.stdout.buffer.write(HEADER+packet(FLOW,timestamp=time.time()))\n'
                'sys.stdout.buffer.flush()\n' + f'sys.stderr.write({counters!r})\n')

    def run_fixture(self, source, **kwargs):
        return produce(self.root, 3, 'lan', 'acl', 'a'*32, kwargs.pop('timeout',3),
                       executor=self.executor(source), fixture=True, **kwargs)

    def test_real_stub_packet_and_fixed_pipe_contract(self):
        result = self.run_fixture(self.good_source())
        self.assertEqual(result['status'],'FIXTURE_CAPTURED')
        self.assertFalse(result['whole_chain_proven']); self.assertFalse(result['live_provenance_verified'])
        self.assertEqual(self.argv_seen[0][0],tuple(capture_argv(3,'lan')))
        self.assertEqual(self.argv_seen[0][1],{'PATH':'/usr/sbin:/usr/bin:/sbin:/bin','LC_ALL':'C'})
        path=Path(result['path'])
        self.assertEqual(path.stat().st_mode&0o777,0o600)
        self.assertEqual(len(capture(path,result['metadata'],3,'lan','a'*32,fixture=True,expected_stage='acl')),1)
        self.assertIsNotNone(self.processes[0].poll())

    def test_live_missing_ownership_and_fixture_default_refuse_before_creation(self):
        for kwargs in ({},{'fixture':True}):
            with self.assertRaises(Refused): produce(self.root,3,'lan','acl','a'*32,1,**kwargs)
        self.assertFalse((self.root/'lan.pcap').exists())
        for slot in (12,13,0):
            with self.assertRaises(Refused):capture_argv(slot,'lan')
        with self.assertRaises(Refused):capture_argv(3,'foreign')

    def test_directory_symlink_mode_owner_and_existing_output_refuse(self):
        executor=self.executor('raise AssertionError("must not execute")')
        self.root.chmod(0o755)
        with self.assertRaises(Refused):produce(self.root,3,'lan','acl','a'*32,1,executor=executor,fixture=True)
        self.root.chmod(0o700)
        with patch('producer.os.geteuid',return_value=self.root.stat().st_uid+1):
            with self.assertRaises(Refused):produce(self.root,3,'lan','acl','a'*32,1,executor=executor,fixture=True)
        link=self.root/'link';link.symlink_to(self.root, target_is_directory=True)
        with self.assertRaises(Refused):produce(link,3,'lan','acl','a'*32,1,executor=executor,fixture=True)
        output=self.root/'lan.pcap';output.write_bytes(b'preserve')
        with self.assertRaises(FileExistsError):produce(self.root,3,'lan','acl','a'*32,1,executor=executor,fixture=True)
        self.assertEqual(output.read_bytes(),b'preserve');self.assertEqual(self.processes,[])

    def test_loss_duplicate_unknown_accounting_never_yields_capture(self):
        cases=['1 packets captured\n1 packets received by filter\n1 packets dropped by kernel\n',
               '1 packets captured\n1 packets received by filter\n',
               '1 packets captured\n1 packets received by filter\n0 packets dropped by kernel\n0 packets dropped by kernel\n',
               '2 packets captured\n2 packets received by filter\n0 packets dropped by kernel\n']
        for accounting in cases:
            with self.subTest(accounting=accounting),self.assertRaises(Refused):self.run_fixture(self.good_source(accounting))
            (self.root/'lan.pcap').unlink()

    def test_stdout_and_stderr_byte_bounds_stop_owned_process(self):
        for stream,limit in (('stdout','MAX_PCAP'),('stderr','MAX_ACCOUNTING')):
            source=f'import sys,signal\nsignal.signal(signal.SIGTERM,signal.SIG_IGN)\nwhile True:\n sys.{stream}.buffer.write(b"x"*65536)\n sys.{stream}.buffer.flush()\n'
            with patch('producer.'+limit,1024),self.assertRaisesRegex(Refused,'byte bound'):
                self.run_fixture(source)
            self.assertLessEqual((self.root/'lan.pcap').stat().st_size,1024 if stream=='stdout' else 0)
            self.assertIsNotNone(self.processes[-1].poll())
            (self.root/'lan.pcap').unlink()

    def test_timeout_kills_own_ignored_term_descendant_while_leader_alive(self):
        identity=self.root/'child'
        source=('import subprocess,sys,time\n'
                f'child=subprocess.Popen([sys.executable,"-c", "import signal,time,os;signal.signal(signal.SIGTERM,signal.SIG_IGN);open({str(identity)!r},\\\"w\\\").write(str(os.getpid()));time.sleep(30)"])\n'
                f'while not __import__("os").path.exists({str(identity)!r}):time.sleep(.01)\n'
                'time.sleep(30)\n')
        start=self.executor(source)
        def ready_executor(argv, *, environment):
            process=start(argv,environment=environment)
            deadline=time.monotonic()+5
            while not identity.exists():
                if process.poll() is not None or time.monotonic()>deadline:
                    raise AssertionError('child did not reach SIG_IGN readiness')
                time.sleep(.01)
            return process
        with self.assertRaisesRegex(Refused,'timed out'):
            produce(self.root,3,'lan','acl','a'*32,0.4,executor=ready_executor,fixture=True)
        self.assertTrue(identity.exists())
        pid=int(identity.read_text())
        for _ in range(100):
            status=Path(f'/proc/{pid}/stat')
            if not status.exists() or status.read_text().split()[2]=='Z':break
            time.sleep(.01)
        else:self.fail('owned ignored-TERM descendant survived')
        self.assertIsNotNone(self.processes[-1].poll())

    def test_timeout_cleans_child_after_confirmed_unreaped_leader_exit(self):
        if not hasattr(os, 'waitid') or not hasattr(os, 'WNOWAIT'):
            self.fail('UNSUPPORTED: Linux waitid WNOWAIT required for exact regression')
        identity=self.root/'exited-parent-child'
        child_source=("import signal,time,os,json;signal.signal(signal.SIGTERM,signal.SIG_IGN);"
                      "raw=open('/proc/self/stat').read();birth=raw.rsplit(') ',1)[1].split()[19];"
                      f"open({str(identity)!r},'w').write(json.dumps(dict(pid=os.getpid(),birth=birth)));"
                      "time.sleep(30)")
        source=('import subprocess,sys,time,os\n'
                f'child=subprocess.Popen([sys.executable,"-c",{child_source!r}])\n'
                f'while not os.path.exists({str(identity)!r}):time.sleep(.01)\n'
                'os._exit(0)\n')
        start=self.executor(source);observed=[]
        def exited_executor(argv, *, environment):
            process=start(argv,environment=environment)
            deadline=time.monotonic()+5
            while True:
                event=os.waitid(os.P_PID,process.pid,os.WEXITED|os.WNOWAIT|os.WNOHANG)
                if event is not None:
                    self.assertEqual(event.si_pid,process.pid)
                    self.assertEqual(event.si_code,os.CLD_EXITED)
                    self.assertEqual(event.si_status,0)
                    self.assertTrue(identity.exists())
                    self.assertIsNone(process.returncode) # waitid did not reap.
                    observed.append(event)
                    return process
                if time.monotonic()>deadline:self.fail('parent did not exit before producer starts')
                time.sleep(.01)
        with self.assertRaisesRegex(Refused,'timed out'):
            produce(self.root,3,'lan','acl','a'*32,0.4,executor=exited_executor,fixture=True)
        self.assertEqual(len(observed),1)
        self.assertEqual(self.processes[-1].returncode,0)
        # Verify the actual child's kernel birth identity, never a recycled PID.
        child=json.loads(identity.read_text());path=Path(f"/proc/{child['pid']}/stat")
        for _ in range(100):
            if not path.exists():break
            fields=path.read_text().rsplit(') ',1)[1].split()
            if fields[19]!=child['birth'] or fields[0]=='Z':break
            time.sleep(.01)
        else:self.fail('same-birth ignored-TERM descendant survived parent-exit cleanup')
        with self.assertRaises(ChildProcessError):
            os.waitid(os.P_PID,self.processes[-1].pid,os.WEXITED|os.WNOWAIT|os.WNOHANG)
