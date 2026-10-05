import json
import os
from pathlib import Path
import struct
import tempfile
import unittest
from unittest.mock import patch

import execute
from driver import Refused
from peer import parse_ipfix


class ExecutorChecks(unittest.TestCase):
    def test_ipfix_requires_valid_framed_data_set(self):
        packet = struct.pack('!HHIIIHH4s', 10, 24, 0, 0, 14, 256, 8, b'data')
        self.assertEqual(parse_ipfix(packet), [(256, 4)])
        for bad in (packet[:-1], packet[:16] + struct.pack('!HH4s', 100, 8, b'data'), b'fixture'):
            with self.assertRaises(ValueError):
                parse_ipfix(bad)

    def test_failed_packet_stage_rolls_back_and_restores_globals(self):
        runner = execute.Runner(14, Path('/unused'), None)
        runner.current = 1
        runner.rig_revision = 1
        runner.rig_config = {'interfaces': {}}
        runner.api = type('API', (), {'call': lambda *_: runner.rig_config})()
        called = []
        runner.command = lambda *_: 'NRestarts=0\nMainPID=99'
        runner.commit = lambda *_: called.append('commit')
        runner.rollback = lambda revision: called.append(('rollback', revision))
        runner.snapshot = lambda operation: called.append(operation)
        def failed():
            raise Refused('no packet')
        with self.assertRaisesRegex(Refused, 'no packet'):
            runner.stage('mpls', {}, failed)
        self.assertEqual(called, ['commit', ('rollback', 1), 'restore'])

    def test_no_source_check_grants_manager_lease(self):
        runner = execute.Runner(14, Path('/unused'), None)
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / 'lease.json'
            def opening(_path, flags):
                return original_open(path, flags)
            original_open = os.open
            value = {'task': 'TEST-traffic-C', 'slot': 14, 'prefix': 'w14', 'boot_id': runner.boot,
                     'expires_unix': 2000, 'lease_id': 'a' * 32,
                     'daemon_owner': 'keepalived', 'globals_owner': True, 'quiet': True}
            path.write_text(json.dumps(value))
            path.chmod(0o600)
            with patch.object(execute.os, 'open', side_effect=opening), patch.object(execute.time, 'time', return_value=1000):
                runner.lease()
                self.assertEqual(runner.lease_id, 'a' * 32)
                value['quiet'] = False
                path.write_text(json.dumps(value))
                with self.assertRaises(Refused):
                    runner.lease()
            path.chmod(0o644)
            with patch.object(execute.os, 'open', side_effect=opening):
                with self.assertRaises(Refused):
                    runner.lease()

    def test_foreign_candidate_never_discarded_on_recovery(self):
        runner = execute.Runner(14, Path('/unused'), None)
        calls = []
        runner.lease = lambda: None
        runner.current = 2
        def call(method, path):
            calls.append((method, path))
            return {'locked': True, 'ownerKeyId': 'foreign', 'lockedAt': 'now'}
        runner.api = type('API', (), {'call': staticmethod(call)})()
        runner.candidate_owner = {'ownerKeyId': 'ours', 'lockedAt': 'now'}
        with self.assertRaises(Refused):
            runner.rollback(1)
        self.assertEqual(calls, [('GET', '/config/lock')])


if __name__ == '__main__':
    unittest.main()
