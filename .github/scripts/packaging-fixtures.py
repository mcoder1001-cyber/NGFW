#!/usr/bin/env python3
"""Run every offline packaging fixture; CI must not silently accept skips."""
from pathlib import Path
import sys
import unittest


def run_suite(suite, stream=None):
    result = unittest.TextTestRunner(verbosity=2, stream=stream).run(suite)
    print(f'Packaging fixtures: {result.testsRun} run, '
          f'{len(result.failures)} failures, {len(result.errors)} errors, '
          f'{len(result.skipped)} skipped, '
          f'{len(result.expectedFailures)} expected failures, '
          f'{len(result.unexpectedSuccesses)} unexpected successes', flush=True)
    if result.skipped or result.expectedFailures:
        print('Packaging fixture gate failed: every discovered test must execute and pass.',
              file=sys.stderr)
    return 0 if (result.testsRun and result.wasSuccessful()
                 and not result.skipped and not result.expectedFailures) else 1


def main():
    root = Path(__file__).resolve().parents[2]
    suite = unittest.defaultTestLoader.discover(
        str(root / 'deploy/debian/vrx/tests'), pattern='test_*.py')
    return run_suite(suite)


if __name__ == '__main__':
    sys.exit(main())
