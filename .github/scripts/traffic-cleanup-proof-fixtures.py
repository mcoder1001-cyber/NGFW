#!/usr/bin/env python3
"""Separate cleanup controls/repetitions; never inflate the original47 count."""
import importlib.util
import io
from pathlib import Path
import sys
import unittest
from unittest.mock import patch

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'test/topology/traffic-a'))
from check import accepted

REPEATED_NAMES = (
    'test_foundation.Foundation.test_timeout_stops_descendant_that_ignores_term',
    'test_foundation.Foundation.test_noisy_child_is_stopped_at_exact_private_log_byte_limit',
)


def complete(result, expected):
    return type(result.testsRun) is int and result.testsRun == expected and accepted(result)


def run_suite(suite, expected, label):
    if suite.countTestCases() != expected:
        print(f'{label}: expected {expected} loaded cases, got {suite.countTestCases()}', file=sys.stderr)
        return 1
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'{label}: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if complete(result, expected) else 1


def controls():
    path = ROOT / 'docs/status/tasks/TEST-traffic-A-cleanup-esrch-controls.py'
    specification = importlib.util.spec_from_file_location('cleanup_esrch_controls', path)
    if specification is None or specification.loader is None:
        raise RuntimeError('required cleanup control module unavailable')
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    suite = unittest.defaultTestLoader.loadTestsFromTestCase(module.Controls)
    return run_suite(suite, 2, 'cleanup error controls')


def repetitions():
    suite = unittest.defaultTestLoader.loadTestsFromNames(REPEATED_NAMES * 5)
    # Numeric PID signalling is forbidden even in fixture fallback paths.
    with patch('os.kill', side_effect=AssertionError('numeric PID signalling prohibited')):
        return run_suite(suite, 10, 'cleanup owned-group repetitions')


def policy_checks():
    checks = 0
    for expected in (2, 10):
        for status, body in [('pass', 'pass'), ('failure', 'self.fail("fixture")'),
                             ('error', 'raise RuntimeError("fixture")'),
                             ('skip', 'self.skipTest("fixture")'),
                             ('expectedFailure', 'self.fail("fixture")'),
                             ('unexpectedSuccess', 'pass')]:
            scope = {'unittest': unittest}
            decorator = '    @unittest.expectedFailure\n' if status in ('expectedFailure', 'unexpectedSuccess') else ''
            exec('class Fixture(unittest.TestCase):\n' + decorator + '    def test_case(self):\n        ' + body + '\n', scope)
            result = unittest.TextTestRunner(stream=io.StringIO()).run(
                unittest.TestSuite(scope['Fixture']('test_case') for _ in range(expected)))
            if complete(result, expected) != (status == 'pass'):
                raise RuntimeError(f'bad {expected}-case outcome policy: {status}')
            checks += 1
        for count in (0, expected - 1):
            result = unittest.TextTestRunner(stream=io.StringIO()).run(
                unittest.TestSuite(unittest.FunctionTestCase(lambda: None) for _ in range(count)))
            if complete(result, expected):
                raise RuntimeError('zero/reduced execution accepted')
            checks += 1
    missing = unittest.defaultTestLoader.loadTestsFromName(REPEATED_NAMES[0] + '_missing')
    result = unittest.TextTestRunner(stream=io.StringIO()).run(missing)
    if not result.errors or complete(result, 10):
        raise RuntimeError('missing original method accepted')
    checks += 1
    print(f'cleanup gate-policy checks: {checks} PASS (not source/lab acceptance)')
    return 0


def main():
    commands = {'--controls': controls, '--repetitions': repetitions, '--check-policy': policy_checks}
    if len(sys.argv) != 2 or sys.argv[1] not in commands:
        print('usage: traffic-cleanup-proof-fixtures.py --controls|--repetitions|--check-policy', file=sys.stderr)
        return 1
    return commands[sys.argv[1]]()


if __name__ == '__main__':
    raise SystemExit(main())
