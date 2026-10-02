#!/usr/bin/env python3
"""Strict source-fixture gate; no skipped/expected failures or zero tests pass."""
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True


def accepted(result):
    return (result.testsRun > 0 and result.wasSuccessful() and not result.skipped
            and not result.expectedFailures and not result.unexpectedSuccesses)


def main():
    suite = unittest.defaultTestLoader.discover(str(Path(__file__).resolve().parent), pattern='test_*.py')
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'traffic-a source fixtures: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if accepted(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
