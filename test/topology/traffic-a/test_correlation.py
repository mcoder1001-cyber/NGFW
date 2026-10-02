"""Explicit offline packet fixtures; no namespace or live tcpdump execution."""
from dataclasses import replace
import hashlib
import ipaddress
from pathlib import Path
import struct
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
from correlation import ExpectedCase, Flow, Probe, correlate
from evidence import read_pcap
from scenario import Refused

MARKER = b'vrx-traffic-a-fixture-nonce'
DIGEST = hashlib.sha256(MARKER).hexdigest()
FLOW = Flow('10.3.1.2', '10.3.2.2', 6, 43001, 8000)
MAC1 = '02:00:00:00:00:02'
MAC2 = '02:00:00:00:00:03'
HEADER = b'\xd4\xc3\xb2\xa1' + struct.pack('<HHIIII', 2, 4, 0, 0, 65535, 1)


def packet(flow, ip_id=101, sequence=901, ttl=64, vlan=(), mac=MAC1, payload=MARKER, timestamp=100.4, flags=2, ack=0):
    if flow.protocol == 6:
        transport = struct.pack('!HHII', flow.source_port, flow.destination_port, sequence, ack) + bytes([0x50, flags]) + b'\x00' * 6 + payload
    else:
        transport = bytes([flow.icmp_type, 0]) + b'\x00\x00' + struct.pack('!HH', flow.icmp_identifier, sequence) + payload
    ip = b'\x45\x00' + struct.pack('!HH', 20 + len(transport), ip_id) + b'\x40\x00' + bytes([ttl, flow.protocol]) + b'\x00\x00'
    ip += ipaddress.IPv4Address(flow.source).packed + ipaddress.IPv4Address(flow.destination).packed
    ethernet = bytes.fromhex(mac.replace(':','')) + b'\x02\x00\x00\x00\x00\x01'
    for tag in vlan: ethernet += b'\x81\x00' + struct.pack('!H',tag)
    frame = ethernet+b'\x08\x00'+ip+transport
    seconds=int(timestamp);fraction=round((timestamp-seconds)*1000000)
    return struct.pack('<IIII',seconds,fraction,len(frame),len(frame))+frame


class Correlation(unittest.TestCase):
    def case(self, **changes):
        case = ExpectedCase('acl','permit','forward','lan',(Probe(FLOW,FLOW,101,901,DIGEST),),100.2,100.8)
        return replace(case,**changes)

    def run_case(self, case, ingress, egress, changes=None, alias=False):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory); files={side:root/(side+'.pcap') for side in ('lan','wan')}
            files[case.input_side].write_bytes(HEADER+b''.join(ingress));files['wan' if case.input_side=='lan' else 'lan'].write_bytes(HEADER+b''.join(egress))
            for path in files.values():path.chmod(0o600)
            metadata={}
            for side,path in files.items():
                metadata[side]=dict(origin='source_fixture',run_id='a'*32,stage=case.stage,namespace=f'ns-w3-{side}',device=f'w3{"l" if side=="lan" else "w"}1',
                    argv=['ip','netns','exec',f'ns-w3-{side}','tcpdump','-n','-U','-s','65535','-i',f'w3{"l" if side=="lan" else "w"}1','-w',str(path),'icmp','or','tcp'],
                    sha256=hashlib.sha256(path.read_bytes()).hexdigest(),started=100,ended=102,received=len(ingress if side==case.input_side else egress),dropped=0)
            if changes:
                for side,values in changes.items():metadata[side].update(values)
            if alias:files['wan']=files['lan']
            return correlate(case,files,metadata,3,'a'*32,fixture=True)

    def test_exact_forward_vlan_and_bvi_ttl_expectations(self):
        case=self.case(stage='vlan',outcome='tagged-ingress',input_vlans=(100,),output_vlans=(100,))
        result=self.run_case(case,[packet(FLOW,vlan=(100,))],[packet(FLOW,vlan=(100,),ttl=63,timestamp=100.5)])
        self.assertEqual(result['status'],'FIXTURE_CORRELATED')
        self.assertFalse(result['whole_chain_proven']);self.assertFalse(result['live_provenance_verified'])
        case=self.case(stage='bridge-bvi',outcome='bvi-route',input_vlans=(100,),output_vlans=())
        self.run_case(case,[packet(FLOW,vlan=(100,))],[packet(FLOW,ttl=63,timestamp=100.5)])
        case=self.case(stage='bridge-bvi',outcome='bridge-forward',ttl_decrement=0)
        self.run_case(case,[packet(FLOW)],[packet(FLOW,timestamp=100.5)])

    def test_wrong_vlan_ttl_source_port_payload_or_duplicate_output_refused(self):
        case=self.case();ingress=[packet(FLOW)]
        wrong_flow=replace(FLOW,source_port=43002)
        for output in ([packet(FLOW,ttl=64)],[packet(FLOW,ttl=63,vlan=(200,))],
                       [packet(wrong_flow,ttl=63)],[packet(FLOW,ttl=63,payload=b'foreign')],
                       [packet(FLOW,ttl=63),packet(FLOW,ttl=63)],[]):
            with self.subTest(output_count=len(output)),self.assertRaises(Refused):self.run_case(case,ingress,output)

    def test_drop_requires_actual_input_and_rejects_any_correlated_output(self):
        case=self.case(outcome='deny',mode='drop',probes=(Probe(FLOW,None,101,901,DIGEST),))
        self.run_case(case,[packet(FLOW)],[])
        with self.assertRaises(Refused):self.run_case(case,[],[])
        for output in ([packet(FLOW,ttl=63)],[packet(replace(FLOW,source='10.3.2.111'),ttl=63)],
                       [packet(FLOW,ttl=63,payload=b'changed',flags=16)]):
            with self.subTest(output=output),self.assertRaises(Refused):self.run_case(case,[packet(FLOW)],output)
        with self.assertRaises(Refused):self.run_case(replace(case,mode='forward'),[packet(FLOW)],[packet(FLOW,ttl=63)])

    def test_nat_translation_matches_explicit_ip_and_port_not_any_output(self):
        translated=replace(FLOW,source='10.3.2.111',source_port=45001)
        case=self.case(stage='nat44-ed',outcome='translated-source',mode='translate',probes=(Probe(FLOW,translated,101,901,DIGEST),))
        self.run_case(case,[packet(FLOW)],[packet(translated,ttl=63,timestamp=100.5)])
        for wrong in (FLOW,replace(translated,source_port=45002),replace(translated,source='10.3.2.112')):
            with self.subTest(wrong=wrong),self.assertRaises(Refused):self.run_case(case,[packet(FLOW)],[packet(wrong,ttl=63)])
        outside=replace(FLOW,source='172.30.126.195')
        with self.assertRaises(Refused):self.run_case(replace(case,probes=(Probe(outside,translated,101,901,DIGEST),)),[packet(FLOW)],[packet(translated,ttl=63)])

    def test_icmp_identifiers_sequences_and_echo_payload_correlate(self):
        incoming=Flow('10.3.1.2','10.3.2.2',1,icmp_identifier=300,icmp_type=8)
        translated=replace(incoming,source='10.3.2.111',icmp_identifier=400)
        case=self.case(stage='nat44-ei',outcome='translated-source',mode='translate',probes=(Probe(incoming,translated,101,9,DIGEST),))
        self.run_case(case,[packet(incoming,sequence=9)],[packet(translated,sequence=9,ttl=63,timestamp=100.5)])
        with self.assertRaises(Refused):self.run_case(case,[packet(incoming,sequence=9)],[packet(translated,sequence=10,ttl=63)])

    def test_ecmp_requires_both_specific_next_hop_identities(self):
        probes=(Probe(FLOW,FLOW,101,901,DIGEST),Probe(replace(FLOW,source_port=43002),replace(FLOW,source_port=43002),102,902,DIGEST))
        case=self.case(stage='vrf-ecmp',outcome='both-ecmp-paths',mode='ecmp',probes=probes,egress_macs=(MAC1,MAC2))
        ingress=[packet(FLOW),packet(probes[1].input_flow,ip_id=102,sequence=902)]
        egress=[packet(FLOW,ttl=63,timestamp=100.5),packet(probes[1].output_flow,ip_id=102,sequence=902,ttl=63,mac=MAC2,timestamp=100.5)]
        self.run_case(case,ingress,egress)
        for bad in ([egress[0],packet(probes[1].output_flow,ip_id=102,sequence=902,ttl=63,mac=MAC1)],
                    [egress[0],packet(probes[1].output_flow,ip_id=102,sequence=902,ttl=63,mac='02:00:00:00:00:04')]):
            with self.assertRaises(Refused):self.run_case(case,ingress,bad)

    def test_endpoint_independence_across_distinct_destinations(self):
        second=replace(FLOW,destination='10.3.2.3')
        translated=replace(FLOW,source='10.3.2.111',source_port=45001)
        translated2=replace(translated,destination=second.destination)
        probes=(Probe(FLOW,translated,101,901,DIGEST),Probe(second,translated2,102,902,DIGEST))
        case=self.case(stage='nat44-ei',outcome='endpoint-independent-mapping',mode='endpoint-independent',probes=probes)
        ingress=[packet(FLOW),packet(second,ip_id=102,sequence=902)]
        egress=[packet(translated,ttl=63,timestamp=100.5),packet(translated2,ip_id=102,sequence=902,ttl=63,timestamp=100.5)]
        self.run_case(case,ingress,egress)
        changing=replace(translated2,source_port=45002)
        with self.assertRaises(Refused):self.run_case(replace(case,probes=(probes[0],replace(probes[1],output_flow=changing))),ingress,[egress[0],packet(changing,ip_id=102,sequence=902,ttl=63)])

    def test_pbr_selected_path_urpf_drop_and_reverse_nat_response(self):
        case=self.case(stage='urpf-pbr',outcome='pbr-selected-path',egress_macs=(MAC2,))
        self.run_case(case,[packet(FLOW)],[packet(FLOW,ttl=63,mac=MAC2,timestamp=100.5)])
        with self.assertRaises(Refused):self.run_case(case,[packet(FLOW)],[packet(FLOW,ttl=63,mac=MAC1)])
        case=self.case(stage='urpf-pbr',outcome='urpf-spoof-drop',mode='drop',probes=(Probe(FLOW,None,101,901,DIGEST),))
        self.run_case(case,[packet(FLOW)],[])
        incoming=Flow('10.3.2.2','10.3.2.111',6,8000,45001)
        delivered=replace(incoming,destination='10.3.1.2',destination_port=43001)
        case=self.case(stage='nat44-ed',outcome='tcp-response',mode='translate',input_side='wan',
                       probes=(Probe(incoming,delivered,101,901,DIGEST,tcp_ack=902,tcp_flags=24),))
        self.run_case(case,[packet(incoming,flags=24,ack=902)],
                      [packet(delivered,ttl=63,flags=24,ack=902,timestamp=100.5)])

    def test_drop_needs_quiet_window_and_late_output_refused(self):
        drop=self.case(outcome='deny',mode='drop',probes=(Probe(FLOW,None,101,901,DIGEST),))
        with self.assertRaises(Refused):self.run_case(drop,[packet(FLOW)],[],{'wan':{'ended':100.9}})
        with self.assertRaises(Refused):self.run_case(replace(drop,settle_seconds=0),[packet(FLOW)],[])
        with self.assertRaises(Refused):self.run_case(self.case(),[packet(FLOW)],
                    [packet(FLOW,ttl=63,timestamp=101.6)],{'wan':{'ended':102}})

    def test_stage_ttl_semantics_and_natural_nat_remote_identity_preserved(self):
        case=self.case(stage='bridge-bvi',outcome='bridge-forward',ttl_decrement=1)
        with self.assertRaises(Refused):self.run_case(case,[packet(FLOW)],[packet(FLOW,ttl=63)])
        translated=replace(FLOW,source='10.3.2.111',source_port=45001,destination='10.3.2.3')
        case=self.case(stage='nat44-ed',outcome='translated-source',mode='translate',probes=(Probe(FLOW,translated,101,901,DIGEST),))
        with self.assertRaises(Refused):self.run_case(case,[packet(FLOW)],[packet(translated,ttl=63)])
        incoming=Flow('10.3.2.2','10.3.2.111',6,8000,45001)
        wrong=replace(incoming,source='10.3.2.3',destination='10.3.1.2',destination_port=43001)
        case=self.case(stage='nat44-ed',outcome='tcp-response',mode='translate',input_side='wan',probes=(Probe(incoming,wrong,101,901,DIGEST),))
        with self.assertRaises(Refused):self.run_case(case,[packet(incoming)],[packet(wrong,ttl=63)])

    def test_identity_capture_window_loss_alias_stage_and_record_bounds(self):
        case=self.case();ingress=[packet(FLOW)];egress=[packet(FLOW,ttl=63,timestamp=100.5)]
        for changes in ({'wan':{'run_id':'b'*32}},{'wan':{'stage':'vlan'}},{'wan':{'dropped':1}},
                        {'wan':{'started':100.3}},{'wan':{'ended':100.7}},{'wan':{'sha256':'0'*64}}):
            with self.subTest(changes=changes),self.assertRaises(Refused):self.run_case(case,ingress,egress,changes)
        with self.assertRaises(Refused):self.run_case(case,ingress,egress,alias=True)
        with self.assertRaises(Refused):self.run_case(case,ingress*2,egress)
        with self.assertRaises(Refused):read_pcap(HEADER+packet(FLOW)*2049)


if __name__ == '__main__':
    unittest.main(verbosity=2)
