#!/usr/bin/python3
"""Host-independent contract/failure tests; never executes namespace commands."""
import contextlib
import importlib.util
import json
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('carrier', Path(__file__).resolve().parents[1] / 'pppoe-kernel-carrier.py')
carrier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(carrier)


class MemoryCarrier(carrier.Carrier):
    def __init__(self):
        super().__init__()
        self.token = carrier.token_for('ngfw', 'wan1')
        self.record = {'owner': 'ngfw', 'logical': 'wan1', 'generation': 'a' * 32,
                       'namespace': [4, 123], 'configured': False,
                       'bound': {'pppwan': 2, 'ppptransit': 3}, 'physical_mac': '02:00:00:00:00:01',
                       'transit': {'local4': '169.254.254.2/30', 'peer4': '169.254.254.1',
                                   'local6': 'fd00:6e67:6677::2/126', 'peer6': 'fd00:6e67:6677::1'}}
        self.calls = []
        self.failure = None
        self.extra = []
        self.change_link = False
        self.link_reads = 0

    def save(self, token, record):
        self.record = dict(record)

    def load(self, token, generation=None):
        if token != self.token or generation != self.record['generation']:
            raise ValueError('stale identity')
        return dict(self.record)

    @contextlib.contextmanager
    def pinned(self, token, record):
        yield 42

    def inside(self, fd, argv, data=None):
        self.calls.append((argv, data))
        if self.failure and self.failure in argv:
            raise RuntimeError('injected command failure')
        if argv[-2:] == ['link', 'show']:
            self.link_reads += 1
            links = [{'ifname': 'lo', 'ifindex': 1},
                     {'ifname': 'ppp0', 'ifindex': 4, 'link_type': 'ppp', 'flags': ['UP']}]
            for name, index in [('pppwan', 2), ('ppptransit', 3)]:
                links.append({'ifname': name, 'ifindex': index + (10 if self.change_link and self.link_reads > 1 else 0),
                              'address': '02:00:00:00:00:01', 'ifalias': self.token + ':' + self.record['generation'] + ':' + name,
                              'linkinfo': {'info_kind': 'tun', 'info_data': {'type': 'tap'}}})
            return json.dumps(links + self.extra)
        if argv[-3:] == ['-j', 'rule', 'show']:
            return '[{"priority": 0, "table": "local"}, {"priority": 32766, "table": "main"}]'
        if argv == [carrier.NFT, '-j', 'list', 'tables']:
            return '{"nftables": [{"table": {"family": "inet", "name": "ngfw_ppp"}}]}'
        return ''


class CarrierTests(unittest.TestCase):
    def test_names_are_fixed_and_unambiguous(self):
        self.assertNotEqual(carrier.token_for('ab', 'c'), carrier.token_for('a', 'bc'))
        self.assertEqual(len(carrier.token_for('ngfw', 'wan1')), 16)
        for bad in ('../wan', 'wan/x', 'wan\n1', 'wan 1', ';id', ''):
            with self.assertRaises(ValueError):
                carrier.token_for('ngfw', bad)

    def test_stale_generation_never_mutates(self):
        c = MemoryCarrier()
        with self.assertRaises(ValueError):
            c.configure(c.token, 'b' * 32)
        self.assertEqual(c.calls, [])

    def test_ingress_routes_precede_local_in_both_families(self):
        c = MemoryCarrier()
        result = c.configure(c.token, 'a' * 32)
        for family in ('-4', '-6'):
            commands = [cmd for cmd, _ in c.calls if cmd[:2] == [carrier.IP, family]]
            for interface, priority, table in [('ppp0', '10', '100'), ('ppptransit', '20', '101')]:
                self.assertIn([carrier.IP, family, 'rule', 'add', 'pref', priority, 'iif', interface, 'lookup', table], commands)
            self.assertIn([carrier.IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'], commands)
        self.assertTrue(result['configured'])
        self.assertNotIn('ready', result)
        self.assertEqual(c.calls[0][1], carrier.nft_policy(False))
        self.assertEqual(c.calls[-1][1], carrier.nft_policy(True))

    def test_ipv6_control_is_preserved_but_raw_wan_ip_is_not_allowed(self):
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        for destination in ('fe80::/10', 'ff00::/8'):
            self.assertIn(([carrier.IP, '-6', 'rule', 'add', 'pref', '5', 'to', destination, 'lookup', 'local'], None), c.calls)
        policy = carrier.nft_policy(True)
        self.assertIn('udp sport 547 udp dport 546', policy)
        self.assertNotIn('masquerade', policy)
        self.assertNotIn('snat', policy)
        self.assertNotIn('pppwan', policy)
        self.assertEqual(policy.count('policy drop'), 3)

    def test_partial_failure_keeps_forwarding_closed(self):
        c = MemoryCarrier()
        c.failure = '101'
        with self.assertRaises(RuntimeError):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])
        self.assertFalse(any(data == carrier.nft_policy(True) for _, data in c.calls))

    def test_foreign_links_rejected_before_route_mutation(self):
        c = MemoryCarrier()
        c.extra = [{'ifname': 'eth0', 'ifindex': 8}]
        with self.assertRaises(ValueError):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(any('route' in cmd for cmd, _ in c.calls))

    def test_link_replacement_does_not_enable_forwarding(self):
        c = MemoryCarrier()
        c.change_link = True
        with self.assertRaises(ValueError):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])
        self.assertFalse(any(data == carrier.nft_policy(True) for _, data in c.calls))

    def test_delete_refuses_attached_or_live_namespace(self):
        c = MemoryCarrier()
        with self.assertRaises(ValueError):
            c.delete(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])

    def test_spec_rejects_unsafe_and_ambiguous_transit(self):
        spec = {'owner': 'ngfw', 'logical': 'wan1', 'parent': 'wanraw', 'mtu': 1492,
                'host4': '169.254.254.2/30', 'peer4': '169.254.254.1',
                'host6': 'fd00:6e67:6677::2/126', 'peer6': 'fd00:6e67:6677::1'}
        self.assertEqual(carrier.validate_spec('ngfw', 'wan1', spec), spec)
        for key, value in [('parent', 'wan1'), ('mtu', True), ('mtu', 1493),
                           ('host4', '169.254.254.0/30'), ('peer4', '169.254.254.7'),
                           ('peer6', 'fd00:6e67:6677::2')]:
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                carrier.validate_spec('ngfw', 'wan1', {**spec, key: value})
        with self.assertRaises(ValueError):
            carrier.validate_spec('ngfw', 'wan1', {**spec, 'arbitrary_command': 'invalid'})

    def test_transit_nd_and_pmtu_errors_are_not_blackholed(self):
        policy = carrier.nft_policy(True)
        transit_nd = next(line for line in policy.splitlines()
                          if 'input iifname "ppptransit"' in line)
        self.assertNotIn('saddr fe80', transit_nd)
        self.assertIn('hoplimit 255', transit_nd)
        self.assertIn('packet-too-big', policy)
        self.assertIn('ip protocol icmp icmp type { destination-unreachable', policy)

    def test_default_local_rule_is_explicitly_removed(self):
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        for family in ('-4', '-6'):
            removal = ([carrier.IP, family, 'rule', 'delete', 'pref', '0'], None)
            addition = ([carrier.IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'], None)
            self.assertLess(c.calls.index(removal), c.calls.index(addition))

    def test_namespace_exec_pins_fd_without_shell(self):
        calls = []
        c = carrier.Carrier(runner=lambda argv, **kwargs: calls.append((argv, kwargs)))
        c.inside(42, [carrier.IP, '-j', 'link', 'show'])
        argv, kwargs = calls[0]
        self.assertEqual(argv[:3], [carrier.NSENTER, '--net=/proc/self/fd/42', '--'])
        self.assertEqual(kwargs['pass_fds'], (42,))


if __name__ == '__main__':
    unittest.main()
