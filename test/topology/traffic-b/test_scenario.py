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
