"""Synthetic pcap fixtures validate parsers only; never count as traffic proof."""
import copy
import hashlib
import json
import os
from unittest.mock import patch
from pathlib import Path
import struct
import sys
import tempfile
import unittest

sys.dont_write_bytecode = True
from evidence import checksum, Indeterminate, capture, read_pcap, validate_stage_contract, validate_go_events
from scenario import Refused


def pcap(vlan=False):
    tcp = struct.pack('!HH', 43001, 8000) + b'\x00' * 8 + b'\x50\x02' + b'\x00' * 6
    ip = b'\x45\x00' + struct.pack('!H', 20+len(tcp)) + b'\x00\x00\x40\x00\x40\x06\x00\x00' + bytes([10, 3, 1, 2, 10, 3, 2, 2])
    tcp = tcp[:16] + struct.pack('!H', checksum(ip[12:20] + struct.pack('!BBH', 0, 6, len(tcp)) + tcp)) + tcp[18:]
    ip = ip[:10] + struct.pack('!H', checksum(ip)) + ip[12:]
    ethernet = b'\x02\x00\x00\x00\x00\x02' + b'\x02\x00\x00\x00\x00\x01'
    ethernet += b'\x81\x00\x00\x64\x08\x00' if vlan else b'\x08\x00'
    frame = ethernet+ip+tcp
    return b'\xd4\xc3\xb2\xa1' + struct.pack('<HHIIII', 2, 4, 0, 0, 65535, 1) + struct.pack('<IIII', 100, 500000, len(frame), len(frame)) + frame


class Evidence(unittest.TestCase):
    def test_ipv4_tcp_vlan_decoded_with_identity(self):
        packets, count = read_pcap(pcap(True))
        self.assertEqual(count, 1)
        packet = packets[0]
        self.assertEqual((packet.source, packet.destination, packet.source_port, packet.destination_port),
                         ('10.3.1.2', '10.3.2.2', 43001, 8000))
        self.assertEqual(packet.vlans, (100,))
        self.assertEqual(packet.destination_mac, '02:00:00:00:00:02')

    def test_truncated_bad_framing_and_partial_snapshot_refused(self):
        original = pcap()
        for changed in (b'', original[:20], original[:-1], original+b'x', b'pcap'+original[4:]):
            with self.subTest(length=len(changed)), self.assertRaises(Refused):
                read_pcap(changed)
        changed = bytearray(original); changed[20:24] = struct.pack('<I', 113)
        with self.assertRaises(Refused): read_pcap(changed)
        changed = bytearray(original); changed[36:40] = struct.pack('<I', 999)
        with self.assertRaises(Refused): read_pcap(changed)

    def metadata(self, path):
        return dict(origin='source_fixture',run_id='a'*32,namespace='ns-w3-lan',device='w3l1',
                    argv=['ip','netns','exec','ns-w3-lan','tcpdump','-n','-U','-s','65535','-i','w3l1','-w',str(path),'icmp','or','tcp'],
                    sha256=hashlib.sha256(path.read_bytes()).hexdigest(),started=100,ended=101,received=1,dropped=0)

    def test_fixture_capture_cannot_be_promoted_to_live(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'capture.pcap';path.write_bytes(pcap());path.chmod(0o600)
            metadata=self.metadata(path)
            self.assertEqual(len(capture(path,metadata,3,'lan','a'*32,fixture=True)),1)
            with self.assertRaisesRegex(Refused,'provenance'):
                capture(path,metadata,3,'lan','a'*32)
            with self.assertRaisesRegex(Refused,'fixed private'):
                capture(path,dict(metadata,origin='tcpdump_live'),3,'lan','a'*32)

    def test_foreign_namespace_loss_digest_and_wrong_interval_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'capture.pcap';path.write_bytes(pcap());path.chmod(0o600)
            metadata=self.metadata(path)
            for change in ({'namespace':'ns-w4-lan'},{'device':'eth0'},{'dropped':1},{'received':0},{'sha256':'0'*64},
                           {'started':101,'ended':102},{'ended':float('nan')},{'run_id':'b'*32}):
                with self.subTest(change=change), self.assertRaises(Refused):
                    capture(path,dict(metadata,**change),3,'lan','a'*32,fixture=True)
            link=path.parent/'link';link.symlink_to(path)
            with self.assertRaises(Refused):capture(link,self.metadata(link),3,'lan','a'*32,fixture=True)

    def test_invalid_checksums_are_indeterminate_not_matched(self):
        # Header corruption and transport checksum corruption have separate causes.
        for offset in (40+14+8, 40+14+20+16):
            data = bytearray(pcap()); data[offset] ^= 1
            with self.subTest(offset=offset), self.assertRaises(Indeterminate) as error:
                read_pcap(data)
            self.assertEqual(error.exception.status, 'INDETERMINATE')
        from test_correlation import packet, HEADER, Flow
        icmp = bytearray(HEADER + packet(Flow('10.3.1.2','10.3.2.2',1,icmp_identifier=3,icmp_type=8), sequence=4))
        icmp[-1] ^= 1
        with self.assertRaisesRegex(Indeterminate, 'ICMP checksum'):
            read_pcap(icmp)

    def test_fragments_short_transport_and_unsupported_records_fail_closed(self):
        for fragment in (0x2000, 1):
            data = bytearray(pcap()); data[60:62] = struct.pack('!H', fragment)
            with self.assertRaisesRegex(Refused, 'fragmented'):
                read_pcap(data)
        data = bytearray(pcap()); data[52:54] = b'\x08\x06'
        with self.assertRaises(Indeterminate): read_pcap(data)
        # Preserve valid IP checksum while declaring an incomplete TCP header.
        data = bytearray(pcap()); data[56:58] = struct.pack('!H', 30)
        data[64:66] = b'\x00\x00'
        data[64:66] = struct.pack('!H', checksum(bytes(data[54:74])))
        with self.assertRaisesRegex(Refused, 'truncated TCP'): read_pcap(data)

    def test_fd_capture_path_swap_never_parses_replacement_bytes(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'capture.pcap'; path.write_bytes(pcap()); path.chmod(0o600)
            metadata=self.metadata(path); before=path.stat(); original_read=os.read; calls=[]
            replacement=path.parent/'replacement'; replacement.write_bytes(b'invalid replacement'); replacement.chmod(0o600)
            def swapped(fd, size):
                if not calls:
                    os.replace(replacement, path)
                calls.append(size)
                return original_read(fd, size)
            with patch('evidence.os.read', side_effect=swapped), patch('evidence.read_pcap', side_effect=AssertionError('changed capture parsed')):
                with self.assertRaisesRegex(Refused, 'changed while acquired'):
                    capture(path,metadata,3,'lan','a'*32,fixture=True,with_identity=True)
            self.assertNotEqual((before.st_dev,before.st_ino),(path.stat().st_dev,path.stat().st_ino))
            with self.assertRaises(Refused): capture(path,metadata,3,'lan','a'*32,fixture=True)

    def test_mutation_during_fd_read_and_offload_zero_checksum_are_refused(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'capture.pcap';path.write_bytes(pcap());path.chmod(0o600)
            metadata=self.metadata(path);original_read=os.read;changed=[]
            def growing(fd,size):
                if not changed:
                    with path.open('ab') as stream:stream.write(b'changed')
                    changed.append(True)
                return original_read(fd,size)
            with patch('evidence.os.read',side_effect=growing),self.assertRaisesRegex(Refused,'changed while'):
                capture(path,metadata,3,'lan','a'*32,fixture=True)
        data=bytearray(pcap());data[90:92]=b'\x00\x00'
        with self.assertRaises(Indeterminate):read_pcap(data)
        data=bytearray(pcap());data[63]=17;data[64:66]=b'\x00\x00'
        data[64:66]=struct.pack('!H',checksum(bytes(data[54:74])))
        with self.assertRaisesRegex(Indeterminate,'unsupported IPv4 transport'):read_pcap(data)

    def test_fd_capture_refuses_fifo_owner_mode_and_size_without_reads(self):
        with tempfile.TemporaryDirectory() as directory:
            path=Path(directory)/'capture.pcap';path.write_bytes(pcap());path.chmod(0o600)
            metadata=self.metadata(path)
            fifo=path.parent/'fifo';os.mkfifo(fifo,0o600)
            fifo_metadata=dict(metadata,argv=[*metadata['argv']]);fifo_metadata['argv'][13]=str(fifo)
            with patch('evidence.os.read',side_effect=AssertionError('invalid capture was read')):
                with self.assertRaises(Refused):capture(fifo,fifo_metadata,3,'lan','a'*32,fixture=True)
                path.chmod(0o644)
                with self.assertRaises(Refused):capture(path,metadata,3,'lan','a'*32,fixture=True)
                path.chmod(0o600)
                with patch('evidence.os.geteuid',return_value=path.stat().st_uid+1):
                    with self.assertRaises(Refused):capture(path,metadata,3,'lan','a'*32,fixture=True)
                with path.open('r+b') as stream:stream.truncate(16*1024*1024+1)
                with self.assertRaises(Refused):capture(path,metadata,3,'lan','a'*32,fixture=True)

    def report(self):
        return dict(slot=3,run_id='a'*32,origin='source_fixture',outcomes=['permit','deny'],capture_sides=['lan','wan'],
                    readback=dict(desired_sha256='b'*64,retrieved_sha256='b'*64,revision=7),
                    counters=[dict(interface='host-w3l0',before=1,after=3),dict(interface='host-w3w0',before=0,after=1)])

    def test_structural_contract_is_never_whole_chain_or_packet_pass(self):
        result=validate_stage_contract('acl',self.report())
        self.assertEqual(result['status'],'CONTRACT_VALIDATED')
        self.assertFalse(result['whole_chain_proven'])
        self.assertFalse(result['packet_outcomes_proven'])

    def test_missing_required_outcome_capture_readback_and_foreign_counter_refused(self):
        for changed in ('outcome','side','readback','counter','reset'):
            report=copy.deepcopy(self.report())
            if changed=='outcome':report['outcomes']=['permit']
            elif changed=='side':report['capture_sides']=['wan']
            elif changed=='readback':report['readback']['retrieved_sha256']='c'*64
            elif changed=='counter':report['counters'][0]['interface']='host-w4l0'
            else:report['counters'][0]['after']=0
            with self.subTest(changed=changed), self.assertRaises(Refused):
                validate_stage_contract('acl',report)

    def test_selected_go_test_cannot_pass_from_skip_exit_or_zero_execution(self):
        events=[dict(Action='run',Package='fixture/module',Test='TestScenario'),
                dict(Action='pass',Package='fixture/module',Test='TestScenario'),
                dict(Action='pass',Package='fixture/module')]
        encode=lambda items:'\n'.join(json.dumps(item) for item in items)
        result=validate_go_events(encode(events),'TestScenario','fixture/module')
        self.assertFalse(result['packet_outcomes_proven'])
        variants=[[],events[1:],events[:-1],events+[dict(Action='fail',Package='fixture/module')],
                  [events[0],dict(Action='skip',Package='fixture/module',Test='TestScenario/packet'),*events[1:]],
                  [events[0],dict(Action='run',Package='fixture/module',Test='TestOther'),*events[1:]],
                  [dict(Action='run',Package='foreign',Test='TestScenario'),*events[1:]]]
        for variant in variants:
            with self.subTest(variant=variant),self.assertRaises(Refused):
                validate_go_events(encode(variant),'TestScenario','fixture/module')
        with self.assertRaises(Refused):validate_go_events('not json','TestScenario','fixture/module')


if __name__ == '__main__':
    unittest.main(verbosity=2)
