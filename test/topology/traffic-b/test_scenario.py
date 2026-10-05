import copy
import unittest
from scenario import PHASES, Refused, accept, slot_values
from tunnels import topology, check_commit

class AcceptanceTests(unittest.TestCase):
    def proof(self):
        phase = PHASES[0]
        return dict(phase=phase.name, slot=27, prefix='w27', run_id='current', source_sha='head',
                    status='passed', commit=dict(status='applied',notApplied=[],unsupported=[],revision=4),
                    packets=dict.fromkeys(phase.packets,True), rollback=dict(status='applied',owned_residue=[]),
                    shared_vpp_before=dict(MainPID=12,NRestarts=0), shared_vpp_after=dict(MainPID=12,NRestarts=0),cleanup=True)
    def verify(self, value):
        return accept(PHASES[0],value,slot=27,run_id='current',source_sha='head')
    def test_matching_envelope_only(self):
        self.verify(self.proof())
    def test_missing_packet_or_cleanup_refused(self):
        for mutate in [lambda v:v['packets'].pop('esp-no-plaintext'),lambda v:v.update(cleanup=False),
                       lambda v:v['rollback'].update(owned_residue=['gre27041']),
                       lambda v:v['commit'].update(notApplied=['vpn']),lambda v:v.update(run_id='stale'),
                       lambda v:v['shared_vpp_after'].update(MainPID=13),
                       lambda v:v['packets'].update({'esp-no-plaintext':1})]:
            v=copy.deepcopy(self.proof());mutate(v)
            with self.assertRaises(Refused):self.verify(v)
    def test_slots_and_port_collision_scheme(self):
        for bad in (0,12,13,33,True,'27'):
            with self.assertRaises(Refused):slot_values(bad)
        self.assertEqual(slot_values(27)['NGFW_HTTP_PORT'],'12700')
    def test_gre_and_vxlan_are_actual_kernel_peers(self):
        gre,commands,device,target=topology(27,'gre')
        self.assertEqual(gre['tunnels']['gre']['w27-tb-gre']['instance'],27041)
        self.assertIn('gre',commands[0]);self.assertEqual(target,'10.27.241.1')
        vx,commands,_,_=topology(27,'vxlan')
        self.assertEqual(vx['tunnels']['vxlan']['w27-tb-vxlan']['decap'],'l2')
        self.assertTrue(vx['interfaces']['loop2741']['l2']['bvi'])
    def test_failed_api_and_unsupported_commit(self):
        check_commit({"status":"applied","revision":{"id":1},"notApplied":[]})
        for result in ({'status':'failed'}, {'status':'applied','notApplied':['interfaces']},
                       {'status':'applied','results':[{'reason':'agent.unsupported-field'}]}):
            with self.assertRaises(Refused):check_commit(result)

    def test_baseline_warnings_never_allow_changed_or_new_fields(self):
        warning = {'pointer': '/services/ntp', 'rule': 'agent.unsupported-field', 'message': 'disabled'}
        result = {'status': 'applied', 'revision': {'id': 2}, 'warnings': [warning]}
        baseline = check_commit(result, changed_paths=('/vrfs',))
        check_commit(result, baseline_warnings=baseline, changed_paths=('/tunnels', '/interfaces'))
        for pointer in ('/tunnels/gre/owned', '/interfaces/host-w27w0', '/services/new'):
            changed = copy.deepcopy(result)
            changed['warnings'][0]['pointer'] = pointer
            with self.assertRaises(Refused):
                check_commit(changed, baseline_warnings=baseline, changed_paths=('/tunnels', '/interfaces'))

if __name__=='__main__':unittest.main()

class SafetyTests(unittest.TestCase):
    def test_direct_child_refuses_before_mounting(self):
        import os
        from pathlib import Path
        import subprocess
        import sys
        before=os.readlink('/proc/self/ns/mnt')
        result=subprocess.run([sys.executable,str(Path(__file__).parent/'private.py'),'--child','--slot','27','/usr/bin/true'],
                              env=dict(os.environ,NGFW_INTEGRATION='1'),capture_output=True,timeout=10)
        self.assertNotEqual(result.returncode,0)
        self.assertEqual(os.readlink('/proc/self/ns/mnt'),before)
    def test_capture_waits_for_ready_signal_and_protects_text(self):
        import os
        from pathlib import Path
        import stat
        import tempfile
        from unittest.mock import patch
        from probe import Capture
        read,write=os.pipe();os.write(write,b'tcpdump: listening on w27w1\n');os.close(write)
        class Process:
            def __init__(self):self.stderr=os.fdopen(read,'rb');self.done=False
            def poll(self):return 0 if self.done else None
            def terminate(self):self.done=True
            def wait(self,timeout=None):return 0
        with tempfile.TemporaryDirectory() as directory:
            output=Path(directory)/'capture.txt'
            with patch('probe.subprocess.Popen',return_value=Process()):
                capture=Capture('ns-w27-wan','w27w1','icmp',output)
                self.assertEqual(stat.S_IMODE(output.stat().st_mode),0o600)
                capture.close()

class StackRecoveryTests(unittest.TestCase):
    def test_partial_database_creation_cleans_reserved_owner(self):
        import os
        from pathlib import Path
        import tempfile
        from unittest.mock import patch
        import subprocess
        import stack
        calls=[]
        def invoke(argv,**kwargs):
            calls.append(argv)
            if 'create' in argv:raise subprocess.CalledProcessError(1,argv)
            return subprocess.CompletedProcess(argv,0)
        with tempfile.TemporaryDirectory() as folder:
            physical=Path(folder)/'api';physical.touch()
            with patch.dict(os.environ,NGFW_DISPOSABLE_VPP='1',NGFW_TRAFFIC_PRIVATE_VPP_PID='1',NGFW_VPP_API_SOCKET=str(physical)), \
                 patch('stack.socket.socket'),patch('stack.private_identity'),patch('stack.os.path.samefile',return_value=True), \
                 patch.object(Path,'is_file',return_value=True),patch.object(Path,'exists',return_value=False), \
                 patch('stack.subprocess.check_output',return_value=''),patch('stack.subprocess.run',side_effect=invoke):
                with self.assertRaises(subprocess.CalledProcessError):
                    with stack.product_stack(27):pass
        self.assertTrue(any('drop' in command and 'w27tb' in command for command in calls))

    def test_foreign_role_refused_without_creation_or_drop(self):
        import os
        from pathlib import Path
        import tempfile
        from unittest.mock import patch
        import stack
        with tempfile.TemporaryDirectory() as folder:
            physical=Path(folder)/'api';physical.touch()
            with patch.dict(os.environ,NGFW_DISPOSABLE_VPP='1',NGFW_TRAFFIC_PRIVATE_VPP_PID='1',NGFW_VPP_API_SOCKET=str(physical)), \
                 patch('stack.socket.socket'),patch('stack.private_identity'),patch('stack.os.path.samefile',return_value=True),patch.object(Path,'is_file',return_value=True), \
                 patch.object(Path,'exists',return_value=False),patch('stack.subprocess.check_output',side_effect=['','ngfw_w27tb']), \
                 patch('stack.subprocess.run') as mutate:
                with self.assertRaises(Refused):
                    with stack.product_stack(27):pass
                mutate.assert_not_called()

    def test_attached_regular_file_and_foreign_peer_refused(self):
        import os
        from pathlib import Path
        import tempfile
        import socket
        import stack
        with tempfile.TemporaryDirectory() as directory:
            file=Path(directory)/'regular';file.touch()
            with self.assertRaises(Refused):stack.attached_identity(file,'w27')
            address=Path(directory)/'agent.sock'
            with socket.socket(socket.AF_UNIX) as server:
                server.bind(str(address));server.listen()
                # This test's PID differs from bridge's required fixture parent.
                with self.assertRaises(Refused):stack.attached_identity(address,'w27')

    def test_dispatcher_sigterm_cleans_owned_command(self):
        import os
        from pathlib import Path
        import signal
        import subprocess
        import sys
        import tempfile
        import time
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);ready=root/'ready';clean=root/'clean';log=root/'log'
            child="import signal,time,pathlib; signal.signal(signal.SIGTERM,lambda s,f:(pathlib.Path("+repr(str(clean))+").write_text('clean'),exit(0))); pathlib.Path("+repr(str(ready))+").touch(); time.sleep(60)"
            script="import run,sys,os; run.fcntl.flock=lambda *a:None; run.campaign=lambda *a,**k:run.execute([sys.executable,'-c',"+repr(child)+"],dict(os.environ),__import__('pathlib').Path("+repr(str(log))+")); sys.argv=['run.py','--slot','27','--phase','wireguard']; run.main()"
            worker=subprocess.Popen([sys.executable,'-c',script],cwd=Path(__file__).parent,env=dict(os.environ,NGFW_INTEGRATION='1'),stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            try:
                deadline=time.monotonic()+5
                while not ready.exists():
                    if worker.poll() is not None or time.monotonic()>deadline:self.fail('owned command did not start')
                    time.sleep(.02)
                worker.send_signal(signal.SIGTERM);worker.wait(timeout=10)
                self.assertEqual(clean.read_text(),'clean')
                self.assertEqual(worker.returncode,143)
            finally:
                if worker.poll() is None:worker.kill();worker.wait()

    def test_wireguard_rig_cleanup_survives_rollback_and_state_failure(self):
        from unittest.mock import Mock
        from wireguard_rest import cleanup
        for rollback in (True,False):
            control=Mock();api=Mock();command=Mock()
            if rollback:control.close.side_effect=Refused('rollback failed')
            else:api.call.side_effect=Refused('state failed')
            with self.assertRaises(Refused):cleanup(control,api,True,command,'w27')
            self.assertEqual(command.call_args[0][0][-3:],['rig','down','w27'])

    def test_owned_fixture_peer_identity_positive_and_namespace_negative(self):
        import os
        from pathlib import Path
        import socket
        import tempfile
        from unittest.mock import patch
        import stack
        with tempfile.TemporaryDirectory() as directory:
            address=Path(directory)/'agent.sock'
            with socket.socket(socket.AF_UNIX) as server:
                server.bind(str(address));server.listen()
                def links(path):
                    return '/private/agent.test' if path.endswith('/exe') else 'private-namespace'
                with patch.dict(os.environ,NGFW_TRAFFIC_EXPECTED_AGENT_PID=str(os.getpid()),NGFW_TRAFFIC_VERIFIED_OWNER='w27'), \
                     patch('stack.os.getppid',return_value=os.getpid()),patch('stack.os.readlink',side_effect=links):
                    self.assertEqual(stack.attached_identity(address,'w27')['pid'],os.getpid())
                    with patch('stack.os.readlink',side_effect=['foreign','private']):
                        with self.assertRaises(Refused):stack.attached_identity(address,'w27')

    def test_owned_descendant_stopped_after_group_leader_exit(self):
        import os
        from pathlib import Path
        import subprocess
        import sys
        import tempfile
        import time
        from owned_process import stop_session
        with tempfile.TemporaryDirectory() as directory:
            pidfile=Path(directory)/'descendant'
            script="import os,time,pathlib; p=os.fork(); pathlib.Path("+repr(str(pidfile))+").write_text(str(p)) if p else time.sleep(60)"
            leader=subprocess.Popen([sys.executable,'-c',script],start_new_session=True)
            leader.wait(timeout=5);pid=int(pidfile.read_text())
            try:
                stop_session(leader,grace=.3)
                deadline=time.monotonic()+2
                while time.monotonic()<deadline:
                    status=Path(f'/proc/{pid}/stat')
                    if not status.exists() or status.read_text().split()[2]=='Z':break
                    time.sleep(.02)
                else:self.fail('owned descendant survived cleanup')
            finally:
                try:os.kill(pid,9)
                except ProcessLookupError:pass
