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
        for result in ({'status':'failed'}, {'status':'applied','notApplied':['interfaces']},
                       {'status':'applied','results':[{'reason':'agent.unsupported-field'}]}):
            with self.assertRaises(Refused):check_commit(result)

if __name__=='__main__':unittest.main()
