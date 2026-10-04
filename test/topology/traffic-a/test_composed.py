import unittest
from composed import topology
from scenario import Refused


class ComposedTests(unittest.TestCase):
    def test_chain_linkage(self):
        baseline, chain = topology(14, '02:14:00:00:00:02')
        self.assertEqual(set(baseline['interfaces']), {'host-w14l0', 'host-w14w0'})
        self.assertEqual(chain['interfaces']['loop1410']['vrf'], 'w14-ta')
        self.assertEqual(chain['interfaces']['host-w14l0']['subinterfaces']['100']['l2']['bridgeDomain'], 'w14-bd')
        self.assertEqual(len(chain['routing']['static'][0]['nextHops']), 2)
        self.assertEqual(chain['nat']['inside'], ['loop1410'])
        self.assertEqual(chain['routing']['pbr']['policies']['w14-to202']['paths'][0]['interface'], 'host-w14w0.202')
        self.assertEqual(chain['acl']['lists']['w14-in']['rules'][-1]['action'], 'deny')

    def test_out_of_slot_input_refused(self):
        for slot in (12, 13, 0, 33):
            with self.assertRaises(Refused):
                topology(slot, '02:14:00:00:00:02')
        with self.assertRaises(Refused):
            topology(14, 'untrusted shell text')

class PacketProofTests(unittest.TestCase):
    def fixture(self):
        from test_correlation import HEADER, packet
        from correlation import Flow
        lan, wan = [], []
        probes = []
        for index, port in enumerate((8000, 8000, 8001, 8002)):
            source_port = 40000 + index
            sequence, ip_id = 100 + index, 500 + index
            incoming = Flow('10.14.10.2', '10.14.99.1', 6, source_port, port)
            lan.append(packet(incoming, ip_id=ip_id, sequence=sequence, vlan=(100,), payload=b''))
            probes.append({'source': source_port, 'port': port, 'expected': port != 8002, 'success': port != 8002})
            if port != 8002:
                outgoing = Flow('10.14.21.100', incoming.destination, 6, 50000 + index, port)
                wan.append(packet(outgoing, ip_id=ip_id, sequence=sequence, ttl=63, vlan=(201 if index == 0 else 202,), payload=b''))
                reply = Flow(incoming.destination, incoming.source, 6, port, source_port)
                lan.append(packet(reply, ip_id=600 + index, sequence=200 + index, vlan=(100,), flags=18, payload=b''))
        ping = Flow('10.14.10.2', '10.14.99.1', 1, icmp_identifier=99, icmp_type=8)
        spoof = Flow('10.14.9.9', '10.14.99.1', 1, icmp_identifier=100, icmp_type=8)
        reply = Flow('10.14.99.1', '10.14.10.2', 1, icmp_identifier=99, icmp_type=0)
        translated = Flow('10.14.21.100', '10.14.99.1', 1, icmp_identifier=150, icmp_type=8)
        lan += [packet(ping, ip_id=700, sequence=1, vlan=(100,)), packet(spoof, ip_id=701, sequence=2, vlan=(100,)), packet(reply, ip_id=702, sequence=1, vlan=(100,))]
        wan += [packet(translated, ip_id=700, sequence=1, vlan=(201,), ttl=63)]
        return HEADER + b''.join(lan), HEADER + b''.join(wan), probes

    def test_complete_synthetic_expectations(self):
        from execute import packet_proof
        lan, wan, probes = self.fixture()
        self.assertEqual(packet_proof(14, 'ed', lan, wan, probes)['ECMP'], [201, 202])

    def test_missing_wan_refused(self):
        from execute import packet_proof
        from test_correlation import HEADER
        lan, _, probes = self.fixture()
        with self.assertRaises(Refused):
            packet_proof(14, 'ed', lan, HEADER, probes)

    def test_claimed_success_cannot_bypass_capture(self):
        from execute import packet_proof
        lan, wan, probes = self.fixture()
        probes[-1]['success'] = True
        with self.assertRaises(Refused):
            packet_proof(14, 'ed', lan, wan, probes)

    def test_ei_requires_mapping_evidence(self):
        from execute import packet_proof
        lan, wan, probes = self.fixture()
        with self.assertRaises(Refused):
            packet_proof(14, 'ei', lan, wan, probes)

class ReadbackTests(unittest.TestCase):
    def fixture(self):
        snapshots = {'bridge': 'loop1410 host-w14l0.100', 'fib': '10.14.21.2 10.14.22.2',
                     'abf': 'loop1410', 'acl': 'w14-in w14-pbr', 'nat': '10.14.10.2 10.14.21.100'}
        names = ('host-w14l0.100', 'host-w14w0.201', 'host-w14w0.202')
        counter = lambda name, n: {'name': name, 'vppName': name, 'counters': {'rxPackets': str(n), 'txPackets': '1'}}
        return snapshots, {name: counter(name, 0) for name in names}, {name: counter(name, 2) for name in names}

    def test_readback_counter_link(self):
        from execute import readback_proof
        snapshots, before, after = self.fixture()
        result = readback_proof(14, snapshots, '', '3 ip4-rx-urpf-strict uRPF check failed error', before, after)
        self.assertEqual(result['strict_uRPF_drop_delta'], 3)

    def test_missing_binding_or_counter_refused(self):
        from execute import readback_proof
        snapshots, before, after = self.fixture()
        for broken in (dict(snapshots, fib='10.14.21.2'), snapshots):
            with self.assertRaises(Refused):
                readback_proof(14, broken, '', '', before, after)
        after['host-w14w0.201']['name'] = 'foreign'
        with self.assertRaises(Refused):
            readback_proof(14, snapshots, '', '3 ip4-rx-urpf-strict uRPF check failed error', before, after)

class CandidateAuthorityTests(unittest.TestCase):
    class Api:
        def __init__(self, revision=7, locked=False):
            self.revision, self.locked = revision, locked
            self.calls = []

        def call(self, method, path, body=None):
            self.calls.append((method, path))
            if path == '/config/lock':
                return {'locked': self.locked, 'ownerKeyId': 'dedicated-key', 'lockedAt': 'fixture'}
            if method == 'PATCH':
                self.locked = True
                return {}
            return {'baseRevision': self.revision, 'changes': []}

    def test_existing_lock_is_never_broken_or_discarded(self):
        from execute import Runner
        api = self.Api(locked=True)
        runner = Runner.__new__(Runner)
        runner.api = api
        with self.assertRaises(Refused):
            runner.claim(7)
        self.assertEqual(api.calls, [('GET', '/config/lock')])

    def test_revision_changed_after_claim_preserves_candidate(self):
        from execute import Runner
        api = self.Api(revision=8)
        runner = Runner.__new__(Runner)
        runner.api = api
        with self.assertRaises(Refused):
            runner.claim(7)
        self.assertNotIn(('POST', '/config/discard'), api.calls)
        self.assertTrue(api.locked)

    def test_owned_clean_candidate_is_claimed(self):
        from execute import Runner
        runner = Runner.__new__(Runner)
        runner.api = self.Api()
        self.assertEqual(runner.claim(7)['ownerKeyId'], 'dedicated-key')

class CleanupTests(unittest.TestCase):
    def test_fixture_failure_still_stops_children_and_downs_links(self):
        from execute import Runner
        from pathlib import Path
        import io
        from unittest.mock import patch
        events = []
        runner = Runner.__new__(Runner)
        runner.prefix, runner.repo = 'w14', Path('/fixture')
        runner.children = [('server', io.BytesIO()), ('capture', io.BytesIO())]
        runner.stop = lambda process: events.append(('stop', process))
        runner.ns = lambda *args: events.append(('peer-down', args[0]))
        runner.command = lambda args: events.append(('host-down', args))
        runner.global_lock = 1
        runner.fixture_log = io.BytesIO()

        class Fixture:
            stdin, stdout = io.BytesIO(), io.BytesIO()
            returncode = 1
            def wait(self, timeout):
                events.append(('fixture-wait', timeout))

        runner.fixture = Fixture()
        with patch('execute.fcntl.flock'):
            with self.assertRaises(Refused):
                runner.cleanup(True)
        self.assertEqual(events[:2], [('stop', 'capture'), ('stop', 'server')])
        self.assertEqual(sum(event[0] == 'peer-down' for event in events), 2)
        self.assertEqual(sum(event[0] == 'host-down' for event in events), 2)
        self.assertTrue(all(log.closed for _, log in runner.children))

class RigDeletionTests(unittest.TestCase):
    def test_veth_down_before_config_delete_even_without_initial_revision(self):
        from execute import Runner
        for revision in (None, 0, 2):
            events = []
            runner = Runner.__new__(Runner)
            runner.prefix = 'w14'
            runner.peers_down = lambda: events.append('veth-down')
            runner.commit = lambda patch, expected: events.append(('commit', patch, expected)) or 9
            runner.claim = lambda expected: events.append(('claim', expected))

            class Api:
                def call(self, method, path):
                    events.append(('api', path))
                    return {'status': 'applied', 'revision': {'id': 9}}

            runner.api = Api()
            self.assertEqual(runner.restore_original(revision, 8), 9)
            self.assertEqual(events[0], 'veth-down')
            if revision in (None, 0):
                self.assertEqual(events[1][0], 'commit')
                self.assertEqual(events[1][1]['interfaces'], {'host-w14l0': None, 'host-w14w0': None})
            else:
                self.assertEqual(events[1], ('claim', 8))

    def test_peer_failure_still_attempts_host_down_and_other_side(self):
        from execute import Runner
        events = []
        runner = Runner.__new__(Runner)
        runner.prefix = 'w14'
        def fail(*args):
            events.append(('peer', args[0]))
            raise Refused('fixture failure')
        runner.ns = fail
        runner.command = lambda args: events.append(('host', args[-2]))
        with self.assertRaises(Refused):
            runner.peers_down()
        self.assertEqual(len(events), 4)
