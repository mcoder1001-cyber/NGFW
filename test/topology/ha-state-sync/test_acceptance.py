import importlib.util
import socket
import json
import os
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest.mock import patch


def load(name):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


acceptance, flow = load('acceptance'), load('flow')
fault = load('kill_vpp_lab')


class Api:
    def __init__(self, response):
        self.response = response
        self.calls = []

    def call(self, method, path, body=None):
        self.calls.append((method, path, body))
        return self.response


class ConcreteEvidence(unittest.TestCase):
    def test_one_real_socket_many_challenges(self):
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen(1)
            server = threading.Thread(target=flow.serve, args=(listener,), daemon=True)
            server.start()
            client = flow.Flow('127.0.0.1', '127.0.0.1', listener.getsockname()[1])
            try:
                replies = [client.exchange() for _ in range(5)]
                self.assertEqual([r['sequence'] for r in replies], [1, 2, 3, 4, 5])
                self.assertTrue(all(r['outside'] == list(client.local) for r in replies))
            finally:
                client.close()
            server.join(2)
            self.assertFalse(server.is_alive())

    def test_replayed_challenge_fails(self):
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen(1)
            def bad_server():
                with listener.accept()[0] as channel:
                    channel.recv(4096)
                    channel.sendall(b'{"sequence":1,"challenge":"replay","peer":["127.0.0.1",1]}\n')
            thread = threading.Thread(target=bad_server, daemon=True)
            thread.start()
            client = flow.Flow('127.0.0.1', '127.0.0.1', listener.getsockname()[1])
            try:
                with self.assertRaisesRegex(RuntimeError, 'challenge'):
                    client.exchange()
            finally:
                client.close()
            thread.join(2)

    def test_one_connection_survives_echo_delayed_beyond_three_seconds(self):
        # Default 1-second VRRP advertisements permit a master-down interval
        # longer than the former 3-second socket timeout in the kill mode.
        with socket.socket() as listener:
            listener.bind(('127.0.0.1', 0))
            listener.listen(1)
            def delayed_server():
                channel, peer = listener.accept()
                with channel, channel.makefile('rb') as stream:
                    request = flow.line(stream)
                    time.sleep(3.2)
                    request['peer'] = list(peer)
                    channel.sendall((json.dumps(request) + '\n').encode())
            thread = threading.Thread(target=delayed_server, daemon=True)
            thread.start()
            client = flow.Flow('127.0.0.1', '127.0.0.1', listener.getsockname()[1])
            try:
                self.assertEqual(client.exchange()['outside'], list(client.local))
            finally:
                client.close()
            thread.join(2)
            self.assertFalse(thread.is_alive())

    def test_exact_tuple_and_truncation(self):
        observation = {'inside': ['10.1.1.2', 40000], 'outside': ['192.0.2.2', 50000]}
        item = dict(insideAddress='10.1.1.2', insidePort=40000, outsideAddress='192.0.2.2',
            outsidePort=50000, protocol='tcp', tableId=0, vrf='default', timedOut=False)
        page = {'total': 1, 'items': [item], 'truncated': False}
        self.assertEqual(acceptance.session(Api(page), observation, 'default'), item)
        for bad in ({**page, 'truncated': True}, {**page, 'total': 2},
                    {**page, 'items': []}, {**page, 'items': [item, item]},
                    {**page, 'items': [{**item, 'outsidePort': 50001}]},
                    {**page, 'items': [{**item, 'timedOut': True}]}):
            with self.assertRaises(acceptance.Refused):
                acceptance.session(Api(bad), observation, 'default')

    def test_foreign_candidate_lease_never_discarded(self):
        api = Api({'ownerKeyId': 'other', 'lockedAt': 'later'})
        transition = acceptance.PriorityTransition(api, 'lan', 90)
        transition.owner = ('ours', 'before')
        with self.assertRaises(acceptance.Refused):
            transition.lock()
        self.assertTrue(all(method == 'GET' for method, _, _ in api.calls))

    def test_foreign_pending_never_reverted(self):
        api = Api({'pending': {'txnId': 'foreign'}})
        transition = acceptance.PriorityTransition(api, 'lan', 90)
        transition.txn = 'ours'
        with self.assertRaises(acceptance.Refused):
            transition.cleanup()
        self.assertEqual(api.calls, [('GET', '/config/commit/pending', None)])

    def test_tls_origins_and_namespace_guard(self):
        for base in ('http://a', 'https://key@a', 'https://a/path', 'https://a?token=secret'):
            with self.assertRaises(acceptance.Refused):
                acceptance.Api(base, 'secret')
        with self.assertRaises(acceptance.Refused):
            acceptance.Worker('host', '10.1.1.2', '192.0.2.2', 18750)
        with self.assertRaises(acceptance.Refused):
            acceptance.NoRedirect().redirect_request(None, None, 302, '', {}, 'https://other')

    def test_kill_never_runs_without_exact_handover(self):
        with patch.object(acceptance.subprocess, 'run') as run:
            for target, handover in [('a', False), ('-oProxyCommand=bad', True), ('a;bad', True)]:
                with self.assertRaises(acceptance.Refused):
                    acceptance.KillTransition(target, 'a' * 36, 99, 'a' * 32, handover)
            run.assert_not_called()

    def test_fault_refuses_shared_host_boot_and_pid_reuse(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            for sub in ('etc/ngfw', 'proc/sys/kernel/random', 'proc/99', 'usr/bin'):
                (root / sub).mkdir(parents=True, exist_ok=True)
            marker = root / 'etc/ngfw/ha-acceptance-lab.json'
            marker.write_text(json.dumps({'bootId': 'boot', 'handoverNonce': 'nonce'}))
            marker.chmod(0o600)
            (root / 'proc/sys/kernel/random/boot_id').write_text('boot\n')
            (root / 'usr/bin/vpp').touch()
            (root / 'proc/99/exe').symlink_to(root / 'usr/bin/vpp')
            (root / 'proc/99/cgroup').write_text('0::/system.slice/vpp.service\n')
            (root / 'proc/99/stat').write_text('99 (vpp) ' + ' '.join(['S'] + ['0'] * 18 + ['123']))
            original_stat = Path.lstat
            def fixture_stat(path, *args, **kwargs):
                values = list(original_stat(path, *args, **kwargs))
                values[4] = 0  # Model root-owned fixture even on an unprivileged CI runner.
                return os.stat_result(values)
            with patch.object(Path, 'lstat', fixture_stat):
                self.assertEqual(fault.verify(root, 'boot', 99, 'nonce', True), '123')
                for boot, nonce, vm in [('other', 'nonce', True), ('boot', 'bad', True), ('boot', 'nonce', False)]:
                    with self.assertRaises(RuntimeError):
                        fault.verify(root, boot, 99, nonce, vm)
                marker.chmod(0o666)
                with self.assertRaises(RuntimeError):
                    fault.verify(root, 'boot', 99, 'nonce', True)

    def test_full_exercise_requires_peer_tuple_roles_and_forwarding(self):
        observation = {'inside': ['10.1.1.2', 40000], 'outside': ['192.0.2.2', 50000], 'sequence': 1}
        item = dict(insideAddress='10.1.1.2', insidePort=40000, outsideAddress='192.0.2.2',
            outsidePort=50000, protocol='tcp', tableId=0, vrf='default', timedOut=False, packets=10)
        state = {'failover': False, 'seq': 0}
        class Node:
            def __init__(self, primary): self.primary = primary
            def call(self, method, path, body=None):
                if path == '/state/ha/vrrp':
                    master = self.primary != state['failover']
                    return {'routers': [{'name': 'lan', 'state': 'master' if master else 'backup'}]}
                if path == '/state/ha/sync':
                    return {'kinds': [{'kind': 'nat44-ei', 'active': True}]}
                if path.startswith('/state/nat/ei/sessions'):
                    return {'total': 1, 'items': [{**item, 'packets': 20 if state['failover'] else 10}]}
                if path == '/actions/ha/sync/resync': return {'summary': 'completed'}
                raise AssertionError(path)
        class Worker:
            def exchange(self):
                state['seq'] += 1
                return {**observation, 'sequence': state['seq']}
        def failover(): state['failover'] = True
        report = acceptance.exercise(Node(True), Node(False), Worker(), 'lan', 'default', failover)
        self.assertEqual(report['two_node_continuity'], 'passed')
        self.assertGreaterEqual(report['same_connection_exchanges'], 3)
        state.update(failover=False, seq=0)
        with patch.object(acceptance.time, 'monotonic', side_effect=[0, 0, 21]):
            with self.assertRaisesRegex(acceptance.Refused, 'did not converge'):
                acceptance.exercise(Node(True), Node(False), Worker(), 'lan', 'default', lambda: None)

    def test_candidate_drift_prevents_cleanup(self):
        class CandidateApi:
            def __init__(self): self.writes = []
            def call(self, method, path, body=None):
                if method != 'GET': self.writes.append(path)
                return {'/config/commit/pending': {'pending': None},
                    '/config/lock': {'ownerKeyId': 'ours', 'lockedAt': 'before'},
                    '/config/diff': {'baseRevision': 1},
                    '/config/candidate': {'unexpected': 'other work'}}[path]
        api = CandidateApi()
        transition = acceptance.PriorityTransition(api, 'lan', 90)
        transition.owner = ('ours', 'before')
        transition.baseline = 1
        transition.expected_candidate = acceptance.digest({'ha': 'ours'})
        with self.assertRaisesRegex(acceptance.Refused, 'candidate changed'):
            transition.cleanup()
        self.assertEqual(api.writes, [])


if __name__ == '__main__':
    unittest.main()
