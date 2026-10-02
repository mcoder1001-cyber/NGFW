#!/usr/bin/env python3
"""Explicit, strict TD-19 fixture runner; hyphenated files evade discovery."""
import importlib.util
from pathlib import Path
import sys
import unittest

# Fixtures are imported from source; do not dirty the repository with bytecode.
sys.dont_write_bytecode = True

SCRIPTS = (
    'TD-19-test-preflight.py',
    'TD-19-test-transfer.py',
    'TD-19-test-provision-order.py',
    'TD-19-test-build-preflight.py',
    'TD-19-test-containerlab.py',
    'TD-19-test-repo-keys.py',
)


def accepted(result):
    """Require actual passing tests, without skipped or expected failures."""
    return (result.testsRun > 0 and result.wasSuccessful()
            and not result.skipped and not result.expectedFailures
            and not result.unexpectedSuccesses)


def load_suite(directory):
    loader = unittest.TestLoader()
    suite = unittest.TestSuite()
    for index, filename in enumerate(SCRIPTS):
        name = f'td19_fixture_{index}'
        specification = importlib.util.spec_from_file_location(name, directory / filename)
        if specification is None or specification.loader is None:
            raise RuntimeError(f'cannot load {filename}')
        module = importlib.util.module_from_spec(specification)
        sys.modules[name] = module
        specification.loader.exec_module(module)
        tests = loader.loadTestsFromModule(module)
        if tests.countTestCases() == 0:
            raise RuntimeError(f'zero tests loaded from {filename}')
        suite.addTests(tests)
    return suite


def main():
    try:
        suite = load_suite(Path(__file__).resolve().parent)
    except Exception as error:
        print(f'fixture loading failed: {error}', file=sys.stderr)
        return 1
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'TD19 fixtures: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} '
          f'unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if accepted(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
