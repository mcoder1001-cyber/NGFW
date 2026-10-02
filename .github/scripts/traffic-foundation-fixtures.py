#!/usr/bin/env python3
"""Frozen inactive foundation: explicit sixteen source cases, never lab proof."""
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
DIRECTORY = Path(__file__).resolve().parents[2] / 'test/topology/traffic-a'
sys.path.insert(0, str(DIRECTORY))
from check import accepted


def main():
    # Explicit modules/count prevent an empty or silently reduced suite passing.
    suite = unittest.defaultTestLoader.loadTestsFromNames(('test_foundation', 'test_evidence'))
    if suite.countTestCases() != 16:
        print(f'Expected sixteen foundation cases; loaded {suite.countTestCases()}', file=sys.stderr)
        return 1
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'traffic foundation: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} '
          f'unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if accepted(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
