import importlib.util
from pathlib import Path
import unittest
from unittest.mock import Mock, patch
import urllib.error
import argparse
import os

spec = importlib.util.spec_from_file_location('capture_live', Path(__file__).with_name('live.py'))
live = importlib.util.module_from_spec(spec)
spec.loader.exec_module(live)


class RunGuardTests(unittest.TestCase):
    def args(self, **overrides):
        values = dict(vpp_socket='/run/ngfw-test/w5/vpp/api.sock', dedicated_vpp=True,
                      vpp_unit='ngfw-test-w5-vpp', interface='host-w5l0', bpf='')
        values.update(overrides)
        return argparse.Namespace(**values)

    def test_shared_socket_is_rejected_before_auth_or_traffic(self):
        with self.assertRaisesRegex(ValueError, 'shared VPP'):
            live.run(self.args(vpp_socket='/run/vpp/api.sock'))

    def test_shared_unit_is_rejected(self):
        with self.assertRaises(ValueError):
            live.run(self.args(vpp_unit='vpp.service'))

    def test_dedicated_acknowledgement_is_required(self):
        with self.assertRaises(ValueError):
            live.run(self.args(dedicated_vpp=False))

    def test_ci_slot_and_foreign_interface_are_rejected(self):
        for prefix, interface in [('w12', 'host-w12l0'), ('w5', 'host-w50l0')]:
            with patch.dict(os.environ, {'NGFW_CAPTURE_TOKEN': 'test-only', 'NGFW_TEST_PREFIX': prefix}, clear=True):
                with self.assertRaises(ValueError):
                    live.run(self.args(interface=interface))

    def test_bpf_without_explicit_opt_in_is_rejected(self):
        with patch.dict(os.environ, {'NGFW_CAPTURE_TOKEN': 'test-only', 'NGFW_TEST_PREFIX': 'w5'}, clear=True):
            with self.assertRaisesRegex(ValueError, 'NGFW_DF8_GLOBALS'):
                live.run(self.args(bpf='icmp'))


class RequestTests(unittest.TestCase):
    def test_unexpected_status_fails(self):
        with self.assertRaisesRegex(RuntimeError, 'expected 202'):
            live.require_status((409, b'busy'), 202)

    def test_binary_is_not_json_decoded(self):
        self.assertEqual(live.require_status((200, b'\xd4\xc3\xb2\xa1'), 200), b'\xd4\xc3\xb2\xa1')

    def test_http_error_returns_status_and_problem(self):
        import io
        error = urllib.error.HTTPError('http://localhost', 409, 'busy', {}, io.BytesIO(b'{"type":"capture-busy"}'))
        with patch.object(live, 'private_opener', return_value=Mock(open=Mock(side_effect=error))):
            self.assertEqual(live.request('http://localhost', 'test-only', 'POST', '/api/v1/actions/capture', {}),
                             (409, b'{"type":"capture-busy"}'))

    def test_timeout_is_not_converted_into_success(self):
        with patch.object(live, 'private_opener', return_value=Mock(open=Mock(side_effect=TimeoutError('offline')))):
            with self.assertRaises(TimeoutError):
                live.request('http://localhost', 'test-only', 'GET', '/api/v1/state/captures')

if __name__ == '__main__':
    unittest.main()
