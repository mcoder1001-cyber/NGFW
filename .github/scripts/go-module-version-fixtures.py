#!/usr/bin/env python3
"""Exact six offline module-version cases; independent of original36 gate."""
import importlib.util
import io
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
NAMES = (
    'test_current_minor_and_explicit_patch_use_original_pin',
    'test_unsupported_versions_refuse_before_any_mutation',
    'test_missing_duplicate_malformed_and_trailing_directives_refuse',
    'test_script_location_ignores_cwd_and_alternate_environment',
    'test_nonregular_symlink_canonical_module_refused',
    'test_module_reader_failure_refuses_before_mutation',
)


def inventory(subject):
    actual = unittest.defaultTestLoader.getTestCaseNames(subject)
    if set(actual) != set(NAMES) or len(actual) != 6:
        raise ValueError(f'exact six ModuleVersion methods required; found {actual}')


def complete(result):
    return (result.testsRun == 6 and result.wasSuccessful() and not result.skipped
            and not result.expectedFailures and not result.unexpectedSuccesses)


def policy():
    checks = 0
    for status, body in [('pass', 'pass'), ('failure', 'self.fail("fixture")'),
                         ('error', 'raise RuntimeError("fixture")'),
                         ('skip', 'self.skipTest("fixture")'),
                         ('expectedFailure', 'self.fail("fixture")'),
                         ('unexpectedSuccess', 'pass')]:
        scope = {'unittest': unittest}
        decorator = '    @unittest.expectedFailure\n' if status in ('expectedFailure', 'unexpectedSuccess') else ''
        exec('class Fixture(unittest.TestCase):\n' + decorator + '    def test_case(self):\n        ' + body + '\n', scope)
        result = unittest.TextTestRunner(stream=io.StringIO()).run(
            unittest.TestSuite(scope['Fixture']('test_case') for _ in range(6)))
        if complete(result) != (status == 'pass'):
            raise RuntimeError(f'incorrect outcome guard: {status}')
        checks += 1
    for count in (0, 5):
        result = unittest.TextTestRunner(stream=io.StringIO()).run(
            unittest.TestSuite(unittest.FunctionTestCase(lambda: None) for _ in range(count)))
        if complete(result):
            raise RuntimeError('zero/reduced success accepted')
        checks += 1
    for names, expected in [(NAMES, True), ((), False), (NAMES[:-1], False),
                            (NAMES + ('test_extra',), False)]:
        subject = type('InventoryFixture', (unittest.TestCase,), {name: lambda self: None for name in names})
        try:
            inventory(subject)
        except ValueError:
            if expected:
                raise
        else:
            if not expected:
                raise RuntimeError('empty/missing/extra inventory accepted')
        checks += 1
    print(f'Go module gate policy: {checks} PASS (not source/lab acceptance)')
    return 0


def main():
    if sys.argv[1:] == ['--check-policy']:
        return policy()
    if sys.argv[1:]:
        print('usage: go-module-version-fixtures.py [--check-policy]', file=sys.stderr)
        return 1
    path = ROOT / 'docs/status/tasks/TD-19-test-go-module-version.py'
    specification = importlib.util.spec_from_file_location('module_version_fixtures', path)
    if specification is None or specification.loader is None:
        raise RuntimeError('ModuleVersion source fixture unavailable')
    module = importlib.util.module_from_spec(specification)
    specification.loader.exec_module(module)
    inventory(module.ModuleVersion)
    suite = unittest.TestSuite(module.ModuleVersion(name) for name in NAMES)
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'Go module source fixtures: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if complete(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
