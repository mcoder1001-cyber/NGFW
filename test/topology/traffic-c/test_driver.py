import copy
import unittest
from driver import Refused, applied, counter_deltas, mpls_packets, ping_outage, plan, srv6_packets, transaction


def receipt():
    return {'status': 'applied', 'notApplied': [], 'warnings': [], 'results': [], 'revision': {'id': 2}}


class FakeAPI:
    def __init__(self):
        self.calls = []
        self.document = {'routing': {'mpls': {}}}
        self.baseline = copy.deepcopy(self.document)
        self.fail_discard = False
        self.dirty = False

    def request(self, method, path, payload=None):
        self.calls.append((method, path))
        if path == 'config/revisions':
            return {'items': [{'id': 1}]}
        if path in ('config', 'config/candidate'):
            if path.endswith('candidate') and self.dirty:
                return {'different': True}
            return copy.deepcopy(self.document)
        if method == 'PATCH':
            self.document = copy.deepcopy(payload)
        if path == 'config/rollback/1':
            self.document = copy.deepcopy(self.baseline)
        if path == 'config/discard' and self.fail_discard:
            raise Refused('discard failed')
        return receipt()


class DriverChecks(unittest.TestCase):
    def test_reserved_slots_refused(self):
        for slot in (0, 12, 13, 33, True):
            with self.subTest(slot=slot), self.assertRaises(Refused):
                plan(slot)
        self.assertFalse(plan(14)['live_acceptance'])
        self.assertEqual(plan(14)['api_port'], 11400)

    def test_missing_or_partial_receipts_refused(self):
        self.assertEqual(applied(receipt()), 2)
        for field, bad in [('status', 'unchanged'), ('notApplied', ['qos']),
                           ('warnings', [{'code': 'agent.unsupported-field'}]),
                           ('results', [{'code': 'agent.unsupported-field'}]),
                           ('revision', None)]:
            value = receipt()
            value[field] = bad
            with self.subTest(field=field), self.assertRaises(Refused):
                applied(value)
        for field in ('notApplied', 'warnings', 'results'):
            value = receipt()
            del value[field]
            with self.assertRaises(Refused):
                applied(value)

    def test_mpls_tuple_and_label_correlate(self):
        line = 'ethertype MPLS unicast (0x8847), label 140060, 10.14.1.2 > 10.14.98.2: ICMP echo request'
        self.assertEqual(mpls_packets(line, 14, 140060), 1)
        for value in (line.replace('140060', '140061'), line.replace('10.14.1.2', '10.15.1.2'),
                      line.replace(', 10.14', '\n10.14')):
            with self.assertRaises(Refused):
                mpls_packets(value, 14, 140060)

    def test_srv6_header_and_inner_tuple_correlate(self):
        line = 'IP6 fd00:e::1 > fd00:e:ee::1: RT6 (type=4, segleft=0, [0]fd00:e:ee::1) 10.14.1.2 > 10.14.160.2: ICMP echo request'
        self.assertEqual(srv6_packets(line, 14, 'fd00:e:ee::1'), 1)
        for value in (line.replace('type=4', 'type=2'), line.replace('10.14.160.2', '10.15.160.2')):
            with self.assertRaises(Refused):
                srv6_packets(value, 14, 'fd00:e:ee::1')

    def test_outage_includes_start_end_and_duplicates(self):
        text = '[101.000] 64 bytes from 10.14.1.254: icmp_seq=1 ttl=64 time=1 ms\n[102.000] 64 bytes from 10.14.1.254: icmp_seq=2 ttl=64 time=1 ms'
        self.assertEqual(ping_outage(text, 100, 103), 1)
        for start, end in ((97, 103), (100, 106), (float('nan'), 103)):
            with self.assertRaises(Refused):
                ping_outage(text, start, end)
        with self.assertRaises(Refused):
            ping_outage(text + '\n' + text, 100, 103)

    def test_policer_requires_three_increases_and_real_drops(self):
        before = {'conform': 1, 'exceed': 2, 'violate': 3}
        after = {'conform': 2, 'exceed': 4, 'violate': 6}
        self.assertEqual(counter_deltas(before, after, 100, 10)['violate'], 3)
        for sent, received in ((100, 100), (100, 0), (True, 0)):
            with self.assertRaises(Refused):
                counter_deltas(before, after, sent, received)
        after['exceed'] = 2
        with self.assertRaises(Refused):
            counter_deltas(before, after, 100, 10)

    def test_rollback_on_failed_evidence(self):
        api = FakeAPI()
        def evidence(_):
            raise Refused('packet proof missing')
        with self.assertRaisesRegex(Refused, 'packet proof missing'):
            transaction(api, 'routing/mpls', {'tables': {'14060': {}}}, evidence)
        self.assertEqual(api.document, api.baseline)
        self.assertIn(('POST', 'config/rollback/1'), api.calls)

    def test_rollback_still_attempted_after_discard_failure(self):
        api = FakeAPI()
        api.fail_discard = True
        with self.assertRaisesRegex(Refused, 'discard failed'):
            transaction(api, 'routing/mpls', {}, lambda _: True)
        self.assertIn(('POST', 'config/rollback/1'), api.calls)

    def test_dirty_candidate_is_never_mutated(self):
        api = FakeAPI()
        api.dirty = True
        with self.assertRaises(Refused):
            transaction(api, 'routing/mpls', {}, lambda _: True)
        self.assertTrue(all(method == 'GET' for method, _ in api.calls))


if __name__ == '__main__':
    unittest.main()
