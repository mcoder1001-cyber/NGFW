#!/usr/bin/python3
"""Host-independent contract/failure tests; never executes namespace commands."""
import contextlib
import importlib.util
import json
import tempfile
from types import SimpleNamespace
from pathlib import Path
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location('carrier', Path(__file__).resolve().parents[1] / 'pppoe-kernel-carrier.py')
carrier = importlib.util.module_from_spec(spec)
spec.loader.exec_module(carrier)
TOKEN = carrier.token_for('ngfw', 'wan1')
RAW, TRANSIT = carrier.link_names(TOKEN)


class MemoryCarrier(carrier.Carrier):
    def __init__(self):
        super().__init__()
        self.token = carrier.token_for('ngfw', 'wan1')
        self.record = {'owner': 'ngfw', 'logical': 'wan1', 'generation': 'a' * 32,
                       'namespace': [4, 123], 'configured': False,
                       'bound': {RAW: 2, TRANSIT: 3}, 'physical_mac': '02:00:00:00:00:01',
                       'transit': {'local4': '169.254.254.2/30', 'peer4': '169.254.254.1',
                                   'local6': 'fd00:6e67:6677::2/126', 'peer6': 'fd00:6e67:6677::1'}}
        self.record['boot'] = 'fake-boot'
        self.record['spec'] = {'mtu': 1492}
        self.calls = []
        self.saved_failure = False
        self.firewall_changed = False
        self.ppp_mtu = 1492
        self.failure = None
        self.extra = []
        self.change_link = False
        self.link_reads = 0

    def save(self, token, record):
        if self.saved_failure and record.get('configured'):
            raise OSError('injected persistence failure')
        self.record = dict(record)

    def load(self, token, generation=None):
        if token != self.token or generation != self.record['generation']:
            raise ValueError('stale identity')
        return dict(self.record)

    @contextlib.contextmanager
    def pinned(self, token, record):
        yield 42

    def inside(self, fd, argv, data=None, timeout=20):
        self.calls.append((argv, data))
        if self.failure and self.failure in argv:
            raise RuntimeError('injected command failure')
        if argv[-2:] == ['link', 'show']:
            self.link_reads += 1
            links = [{'ifname': 'lo', 'ifindex': 1},
                     {'ifname': 'ppp0', 'ifindex': 4, 'link_type': 'ppp', 'flags': ['UP']}]
            for name, index in [(RAW, 2), (TRANSIT, 3)]:
                links.append({'ifname': name, 'ifindex': index + (10 if self.change_link and self.link_reads > 1 else 0),
                              'address': '02:00:00:00:00:01', 'ifalias': self.token + ':' + self.record['generation'] + ':' + name,
                              'linkinfo': {'info_kind': 'tun', 'info_data': {'type': 'tap'}}})
            return json.dumps(links + self.extra)
        if argv[-3:] == ['-j', 'rule', 'show']:
            added = [cmd for cmd, _ in self.calls if cmd[:4] == [carrier.IP, argv[1], 'rule', 'add']]
            if not added:
                return '[{"priority": 0, "table": "local"}, {"priority": 32766, "table": "main"}]'
            rows = []
            for command in added:
                row = {'priority': int(command[command.index('pref') + 1]), 'table': command[-1]}
                for field in ('iif', 'ipproto', 'sport', 'dport'):
                    if field in command:
                        row[field] = command[command.index(field) + 1]
                if 'to' in command:
                    row['dst'] = command[command.index('to') + 1]
                rows.append(row)
            return json.dumps(rows)
        if argv[-2:] == ['address', 'show']:
            return json.dumps([{'ifname': RAW, 'addr_info': []}, {'ifname': 'ppp0', 'mtu': self.ppp_mtu, 'addr_info': [{'local': '192.0.2.10', 'prefixlen': 32}]},
                               {'ifname': TRANSIT, 'mtu': 1492, 'addr_info': [
                                   {'local': '169.254.254.2', 'prefixlen': 30},
                                   {'local': 'fd00:6e67:6677::2', 'prefixlen': 126}]}])
        if 'route' in argv and 'show' in argv:
            table = argv[-1]
            row = {'dst': 'default', 'dev': TRANSIT if table == '100' else 'ppp0'}
            if table == '100':
                row['gateway'] = self.record['transit']['peer' + argv[1][1:]]
            return json.dumps([row])
        if argv[:2] == [carrier.SYSCTL, '-n']:
            return ('0' if 'rp_filter' in argv[2] or 'accept_ra_defrtr' in argv[2]
                    else '2' if argv[2].endswith('accept_ra') else '1')
        if argv == [carrier.NFT, '-j', 'list', 'table', 'inet', 'ngfw_ppp']:
            objects = [{'table': {'family': 'inet', 'name': 'ngfw_ppp'}}]
            for name in ('input', 'output', 'forward'):
                objects.append({'chain': {'family': 'inet', 'table': 'ngfw_ppp', 'name': name,
                                         'hook': name, 'policy': 'drop', 'type': 'filter', 'prio': 0}})
            for i in range(carrier.nft_policy(True, TOKEN).count('add rule ')):
                objects.append({'rule': {'family': 'inet', 'table': 'ngfw_ppp', 'chain': 'input',
                                        'expr': [{'fixture': i + (100 if self.firewall_changed else 0)}]}})
            return json.dumps({'nftables': objects})
        if argv == [carrier.NFT, '-j', 'list', 'tables']:
            return '{"nftables": [{"table": {"family": "inet", "name": "ngfw_ppp"}}]}'
        return ''


class ProbeCarrier(MemoryCarrier):
    def __init__(self):
        super().__init__()
        self.record['configured'] = True
        self.temporary_rules = []
        self.probe_fails = False
        self.cleanup_fails = False
        self.verified = 0

    def verify_state(self, fd, token, record):
        self.verified += 1
        if self.cleanup_fails and self.verified > 1:
            raise ValueError('injected cleanup readback failure')
        return {'verified': True}

    def inside(self, fd, argv, data=None, timeout=20):
        if argv[0] == carrier.SETPRIV:
            self.calls.append((argv, data))
            if timeout != 4:
                raise AssertionError('probe lacks bounded supervisor timeout')
            if self.probe_fails:
                raise TimeoutError('injected fixed probe timeout')
            return '{"sent":1,"received":1,"latencyMs":2,"unavailable":false}'
        if argv[:4] == [carrier.IP, '-4', 'rule', 'add']:
            self.temporary_rules.append(int(argv[argv.index('pref') + 1]))
        if argv[:4] == [carrier.IP, '-4', 'rule', 'delete']:
            self.temporary_rules.remove(int(argv[-1]))
        if argv[-3:] == ['-j', 'rule', 'show']:
            self.calls.append((argv, data))
            return json.dumps([{'priority': value} for value in self.temporary_rules])
        return super().inside(fd, argv, data, timeout)


class BrokerCarrier(MemoryCarrier):
    def __init__(self):
        super().__init__()
        self.documents = {}
        self.clock = 100.0
        self.expire_while_locking = False
        self.operations = []

    def broker_read(self, token, area):
        if (token, area) not in self.documents:
            raise FileNotFoundError()
        return json.loads(json.dumps(self.documents[token, area]))

    def broker_write(self, token, area, document):
        self.documents[token, area] = json.loads(json.dumps(document))

    @contextlib.contextmanager
    def locked(self):
        if self.expire_while_locking:
            self.clock += 11
        yield

    def verify(self, token, generation):
        self.operations.append((token, generation))
        return {'verified': True}


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
            for interface, priority, table in [('ppp0', '10', '100'), (TRANSIT, '20', '101')]:
                self.assertIn([carrier.IP, family, 'rule', 'add', 'pref', priority, 'iif', interface, 'lookup', table], commands)
            self.assertIn([carrier.IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'], commands)
        self.assertTrue(result['configured'])
        self.assertNotIn('ready', result)
        self.assertEqual(c.calls[0][1], carrier.nft_policy(False, TOKEN))
        self.assertIn(carrier.nft_policy(True, TOKEN), [data for _, data in c.calls])

    def test_ipv6_control_is_preserved_but_raw_wan_ip_is_not_allowed(self):
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        for destination in ('fe80::/10', 'ff00::/8'):
            self.assertIn(([carrier.IP, '-6', 'rule', 'add', 'pref', '5', 'to', destination, 'lookup', 'local'], None), c.calls)
        policy = carrier.nft_policy(True, TOKEN)
        self.assertIn('udp sport 547 udp dport 546', policy)
        self.assertNotIn('masquerade', policy)
        self.assertNotIn('snat', policy)
        self.assertNotIn(RAW, policy)
        self.assertEqual(policy.count('policy drop'), 3)

    def test_partial_failure_keeps_forwarding_closed(self):
        c = MemoryCarrier()
        c.failure = '101'
        with self.assertRaises(RuntimeError):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])
        self.assertFalse(any(data == carrier.nft_policy(True, TOKEN) for _, data in c.calls))

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
        self.assertFalse(any(data == carrier.nft_policy(True, TOKEN) for _, data in c.calls))

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
        policy = carrier.nft_policy(True, TOKEN)
        transit_nd = next(line for line in policy.splitlines()
                          if 'input iifname "' + TRANSIT + '"' in line)
        self.assertNotIn('saddr fe80', transit_nd)
        self.assertIn('hoplimit 255', transit_nd)
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        self.assertIn(([carrier.IP, '-6', 'rule', 'add', 'pref', '6', 'iif', TRANSIT,
                        'to', 'fd00:6e67:6677::2/128', 'lookup', 'local'], None), c.calls)
        self.assertIn('packet-too-big', policy)
        self.assertIn('ip protocol icmp icmp type { destination-unreachable', policy)

    def test_default_local_rule_is_explicitly_removed(self):
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        for family in ('-4', '-6'):
            removal = ([carrier.IP, family, 'rule', 'delete', 'pref', '0'], None)
            addition = ([carrier.IP, family, 'rule', 'add', 'pref', '100', 'lookup', 'local'], None)
            self.assertLess(c.calls.index(removal), c.calls.index(addition))

    def test_verify_rejects_firewall_drift_despite_configured_flag(self):
        c = MemoryCarrier()
        c.configure(c.token, 'a' * 32)
        self.assertTrue(c.verify(c.token, 'a' * 32)['verified'])
        c.firewall_changed = True
        self.assertTrue(c.record['configured'])
        with self.assertRaisesRegex(ValueError, 'firewall changed'):
            c.verify(c.token, 'a' * 32)

    def test_final_persistence_failure_recloses_forwarding(self):
        c = MemoryCarrier()
        c.saved_failure = True
        with self.assertRaises(OSError):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])
        self.assertEqual(c.calls[-1][1], carrier.nft_policy(False, TOKEN))

    def test_pinned_namespace_rejects_path_replacement_and_non_namespace(self):
        with tempfile.TemporaryDirectory() as directory:
            namespace = Path(directory) / TOKEN
            namespace.write_text('not a namespace')
            info = namespace.stat()
            record = {'namespace': [info.st_dev, info.st_ino]}
            c = carrier.Carrier(netns=directory)
            with mock.patch.object(carrier.fcntl, 'ioctl', return_value=0), self.assertRaises(ValueError):
                with c.pinned(TOKEN, record):
                    self.fail('non-netns accepted')
            moved = namespace.with_suffix('.old')
            namespace.rename(moved)
            namespace.write_text('replacement')
            with self.assertRaisesRegex(ValueError, 'identity changed'):
                with c.pinned(TOKEN, record):
                    self.fail('replaced path accepted')
            namespace.unlink()
            namespace.symlink_to(moved)
            with self.assertRaises(OSError):
                with c.pinned(TOKEN, record):
                    self.fail('symlink accepted')

    def test_prepare_rejects_multicast_mac_before_network_mutation(self):
        c = MemoryCarrier()
        with self.assertRaises(ValueError):
            c.prepare(TOKEN, 'a' * 32, 2, 3, '01:00:00:00:00:01', c.record['transit'])
        self.assertEqual(c.calls, [])

    def test_lower_negotiated_mtu_is_not_silently_ready(self):
        c = MemoryCarrier()
        c.ppp_mtu = 1480
        with self.assertRaisesRegex(ValueError, 'MTU differs'):
            c.configure(c.token, 'a' * 32)
        self.assertFalse(c.record['configured'])
        self.assertFalse(any(data == carrier.nft_policy(True, TOKEN) for _, data in c.calls))

    def test_leaf_capability_and_namespace_checks_fail_closed(self):
        c = MemoryCarrier()
        good = {name: hex(carrier.PPP_CAPS) for name in ('CapEff', 'CapPrm', 'CapBnd')}
        good.update(CapInh='0', CapAmb='0', NoNewPrivs='1')
        with mock.patch.object(carrier.os, 'geteuid', return_value=0):
            self.assertEqual(c.check_leaf(TOKEN, 'a' * 32, good, [4, 123]), c.record)
            for name, value in [('CapEff', hex(carrier.PPP_CAPS | (1 << 21))),
                                ('CapPrm', '0'), ('CapBnd', 'ffffffff'),
                                ('CapInh', '1'), ('CapAmb', '1'), ('NoNewPrivs', '0')]:
                with self.subTest(name=name), self.assertRaises(ValueError):
                    c.check_leaf(TOKEN, 'a' * 32, {**good, name: value}, [4, 123])
            with self.assertRaisesRegex(ValueError, 'namespace'):
                c.check_leaf(TOKEN, 'a' * 32, good, [4, 124])
            with self.assertRaises(ValueError):
                c.check_leaf(TOKEN, 'b' * 32, good, [4, 123])
        argv = c.leaf_argv(TOKEN, 'a' * 32, 'ppp-exec')
        self.assertEqual(argv[0], carrier.SETPRIV)
        self.assertIn('--bounding-set=-all,+net_admin,+net_raw', argv)
        self.assertIn('--inh-caps=-all', argv)
        self.assertIn('--ambient-caps=-all', argv)
        self.assertNotIn('/usr/sbin/pppd', argv)

    def test_probe_policy_has_expiring_exact_targets_without_forward_exemption(self):
        for kind in ('icmp', 'dns', 'http'):
            policy = carrier.probe_policy(TOKEN, kind, '198.51.100.1', ['192.0.2.10'])
            baseline = carrier.nft_policy(True, TOKEN)
            delta = policy[len(baseline):]
            self.assertIn('198.51.100.1 timeout 5s', delta)
            self.assertIn('192.0.2.10 timeout 5s', delta)
            self.assertIn('ct mark 0x4e47 ct state established', delta)
            self.assertIn('ct mark set 0x4e47 accept', delta)
            self.assertNotIn('forward', delta)
        for kind, target in [('exec', '198.51.100.1'), ('http', 'example.com'),
                             ('http', '127.0.0.1'), ('dns', '224.0.0.1'), ('icmp', '2001:db8::1')]:
            with self.assertRaises(ValueError):
                carrier.validate_probe(kind, target)

    def test_probe_cleanup_runs_on_success_and_timeout(self):
        for fails in (False, True):
            c = ProbeCarrier()
            c.probe_fails = fails
            if fails:
                with self.assertRaises(TimeoutError):
                    c.probe(TOKEN, 'a' * 32, 'http', '198.51.100.1')
            else:
                self.assertEqual(c.probe(TOKEN, 'a' * 32, 'http', '198.51.100.1')['received'], 1)
            self.assertEqual(c.temporary_rules, [])
            self.assertEqual(c.verified, 2)
            self.assertTrue(c.record['configured'])
            self.assertIn(carrier.nft_policy(True, TOKEN), [data for _, data in c.calls])

    def test_ambiguous_probe_cleanup_withdraws_carrier(self):
        c = ProbeCarrier()
        c.cleanup_fails = True
        with self.assertRaises(ValueError):
            c.probe(TOKEN, 'a' * 32, 'dns', '198.51.100.1')
        self.assertFalse(c.record['configured'])
        self.assertEqual(c.calls[-1][1], carrier.nft_policy(False, TOKEN))

    def test_broker_rejects_unknown_fields_and_nonce_replay(self):
        request = {'op': 'verify', 'token': TOKEN, 'generation': 'a' * 32}
        self.assertEqual(carrier.request_token(request), TOKEN)
        for bad in ({**request, 'command': '/bin/sh'}, {**request, 'op': 'run'},
                    {**request, 'generation': 'wrong'}):
            with self.assertRaises(ValueError):
                carrier.request_token(bad)
        c = BrokerCarrier()
        with mock.patch.object(carrier.time, 'monotonic', side_effect=lambda: c.clock), mock.patch.object(carrier.os, 'unlink'):
            queued = c.broker_queue(TOKEN, 'b' * 32, request)
            self.assertEqual(set(queued), {'token', 'nonce', 'boot', 'expires', 'request_sha256'})
            with self.assertRaises(ValueError):
                c.broker_queue(TOKEN, 'c' * 32, request)
            c.broker_execute(TOKEN)
            result = c.broker_result(TOKEN, 'b' * 32)
            self.assertTrue(result['ok'])
            self.assertEqual(result['request_sha256'], queued['request_sha256'])
            with self.assertRaises(ValueError):
                c.broker_result(TOKEN, 'c' * 32)
            c.clock += 11
            with self.assertRaises(ValueError):
                c.broker_result(TOKEN, 'b' * 32)

    def test_broker_rechecks_expiry_after_lock_before_mutation(self):
        c = BrokerCarrier()
        c.expire_while_locking = True
        with mock.patch.object(carrier.time, 'monotonic', side_effect=lambda: c.clock), mock.patch.object(carrier.os, 'unlink'):
            c.broker_queue(TOKEN, 'b' * 32, {'op': 'verify', 'token': TOKEN, 'generation': 'a' * 32})
            result = c.broker_execute(TOKEN)
        self.assertFalse(result['ok'])
        self.assertIn('TimeoutError', result['error'])
        self.assertEqual(c.operations, [])

    def test_probe_binary_digest_and_pinned_inode(self):
        with tempfile.TemporaryDirectory() as directory:
            binary = Path(directory) / 'probe'
            binary.write_bytes(b'ELF fixture, never executed')
            binary.chmod(0o755)
            digest = Path(str(binary) + '.sha256')
            digest.write_text(carrier.hashlib.sha256(binary.read_bytes()).hexdigest() + '\n')
            digest.chmod(0o644)
            original_fstat = carrier.os.fstat
            def root_stat(fd):
                info = original_fstat(fd)
                return SimpleNamespace(st_mode=info.st_mode, st_uid=0, st_size=info.st_size)
            with mock.patch.object(carrier, 'WAN_PROBE', str(binary)), mock.patch.object(carrier, 'protected_directory'), mock.patch.object(carrier.os, 'fstat', side_effect=root_stat):
                fd = carrier.open_verified_probe()
                try:
                    binary.rename(binary.with_suffix('.old'))
                    binary.write_bytes(b'replacement')
                    self.assertEqual(carrier.os.read(fd, 100), b'ELF fixture, never executed')
                finally:
                    carrier.os.close(fd)
                binary.chmod(0o755)
                with self.assertRaisesRegex(ValueError, 'digest mismatch'):
                    carrier.open_verified_probe()
                digest.chmod(0o666)
                with self.assertRaisesRegex(ValueError, 'untrusted probe digest'):
                    carrier.open_verified_probe()

    def test_unit_assets_keep_ownership_state_out_of_daemon_writes(self):
        assets = Path(__file__).resolve().parents[1] / 'pppoe-carrier-assets'
        daemon = (assets / 'ngfw-pppoe-carrier@.service').read_text()
        broker = (assets / 'ngfw-pppoe-broker@.service').read_text()
        self.assertIn('BindReadOnlyPaths=/run/ngfw-pppoe-carrier:/run/ngfw-pppoe-carrier', daemon)
        self.assertIn('BindPaths=/run/ngfw/pppoe/%i:/run/ngfw/pppoe/%i', daemon)
        self.assertIn('ReadWritePaths=/run/ngfw/pppoe/%i', daemon)
        self.assertIn('TemporaryFileSystem=/run:rw /etc:ro /var/lib:ro', daemon)
        self.assertNotIn('BindPaths=/run/ngfw-pppoe-carrier', daemon)
        self.assertNotIn('pppoe-broker', daemon)
        directives = [line for line in broker.splitlines() if not line.startswith('#')]
        for prefix in ('PrivateTmp=', 'ProtectSystem=', 'ProtectHome=', 'BindPaths=', 'ReadWritePaths='):
            self.assertFalse(any(line.startswith(prefix) for line in directives))
        self.assertIn('KillMode=control-group', directives)
        self.assertIn('TimeoutStartSec=12', directives)
        self.assertIn('ExecStart=/usr/bin/python3 -I /usr/lib/ngfw/pppoe-carrier.py broker-execute %i', directives)

    def test_daemon_start_requires_prior_broker_withdrawal(self):
        c = MemoryCarrier()
        c.record['configured'] = True
        # This is a read-only launcher refusal: no privileged helper writes.
        c.load = lambda token: dict(c.record)
        with self.assertRaisesRegex(ValueError, 'broker must withdraw'):
            c.launch(TOKEN)
        self.assertEqual(c.calls, [])

    def test_namespace_exec_pins_fd_without_shell(self):
        calls = []
        c = carrier.Carrier(runner=lambda argv, **kwargs: calls.append((argv, kwargs)))
        c.inside(42, [carrier.IP, '-j', 'link', 'show'])
        argv, kwargs = calls[0]
        self.assertEqual(argv[:3], [carrier.NSENTER, '--net=/proc/self/fd/42', '--'])
        self.assertEqual(kwargs['pass_fds'], (42,))


if __name__ == '__main__':
    unittest.main()
