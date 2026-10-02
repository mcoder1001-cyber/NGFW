#!/usr/bin/env python3
"""Run every offline packaging fixture; CI must not silently accept skips."""
from pathlib import Path
import sys
import unittest


def main():
    root = Path(__file__).resolve().parents[2]
    suite = unittest.defaultTestLoader.discover(
        str(root / 'deploy/debian/vrx/tests'), pattern='test_*.py')
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'Packaging fixtures: {result.testsRun} run, '
          f'{len(result.failures)} failures, {len(result.errors)} errors, '
          f'{len(result.skipped)} skipped', flush=True)
    if result.skipped:
        print('Packaging fixture gate failed: every discovered test must execute.',
              file=sys.stderr)
    return 0 if result.testsRun and result.wasSuccessful() and not result.skipped else 1


if __name__ == '__main__':
    sys.exit(main())
