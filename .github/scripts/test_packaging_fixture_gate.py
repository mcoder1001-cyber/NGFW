"""Execute tiny real unittest suites against the strict fixture gate policy."""
import contextlib
import importlib.util
import io
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('packaging_fixture_gate',
    Path(__file__).with_name('packaging-fixtures.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class GatePolicy(unittest.TestCase):
    def check_status(self, method, expected, summary):
        fixture = type('IsolatedFixture', (unittest.TestCase,), {'runTest': method})
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            status = gate.run_suite(unittest.TestSuite([fixture()]), stream=output)
        self.assertEqual(status, expected, output.getvalue())
        self.assertIn(summary, output.getvalue())

    def test_success_passes(self):
        self.check_status(lambda case: None, 0, '1 run, 0 failures, 0 errors, 0 skipped')

    def test_failure_rejects(self):
        self.check_status(lambda case: case.fail('fixture failure'), 1, '1 failures')

    def test_error_rejects(self):
        def error(case):
            raise RuntimeError('fixture error')
        self.check_status(error, 1, '1 errors')

    def test_skip_rejects(self):
        self.check_status(lambda case: case.skipTest('fixture skip'), 1, '1 skipped')

    def test_expected_failure_rejects(self):
        method = unittest.expectedFailure(lambda case: case.fail('expected fixture failure'))
        self.check_status(method, 1, '1 expected failures')

    def test_unexpected_success_rejects(self):
        self.check_status(unittest.expectedFailure(lambda case: None), 1, '1 unexpected successes')

    def test_zero_tests_rejects(self):
        output = io.StringIO()
        with contextlib.redirect_stdout(output), contextlib.redirect_stderr(output):
            status = gate.run_suite(unittest.TestSuite(), stream=output)
        self.assertEqual(status, 1)
        self.assertIn('0 run', output.getvalue())
