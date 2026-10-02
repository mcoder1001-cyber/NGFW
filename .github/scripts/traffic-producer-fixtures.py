#!/usr/bin/env python3
"""Inactive producer gate: forty source cases, never traffic proof."""
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
DIRECTORY = Path(__file__).resolve().parents[2] / 'test/topology/traffic-a'
sys.path.insert(0, str(DIRECTORY))
from check import accepted

MODULES = ('test_foundation', 'test_evidence', 'test_correlation', 'test_producer')
EXPECTED = 40


def valid_counts(counts):
    return (len(counts) == len(MODULES)
            and all(type(count) is int and count > 0 for count in counts)
            and sum(counts) == EXPECTED)


def complete(result):
    return result.testsRun == EXPECTED and accepted(result)


def policy_checks():
    # Gate-policy checks, separately reported; these are not the source cases.
    checks = 0
    for counts, expected in [((1, 1, 1, 37), True), ((0, 1, 1, 38), False),
                             ((1, 1, 38), False), ((1, 1, 1, 1), False),
                             ((True, 1, 1, 37), False)]:
        if valid_counts(counts) != expected:
            raise RuntimeError(f'incorrect module-count policy: {counts}')
        checks += 1
    for status in ('pass', 'zero', 'reduced', 'failures', 'errors', 'skipped',
                   'expectedFailures', 'unexpectedSuccesses'):
        result = unittest.TestResult()
        result.testsRun = 0 if status == 'zero' else 39 if status == 'reduced' else EXPECTED
        if status in ('failures', 'errors', 'skipped', 'expectedFailures', 'unexpectedSuccesses'):
            getattr(result, status).append(('gate-policy-fixture', 'fixture'))
        if complete(result) != (status == 'pass'):
            raise RuntimeError(f'incorrect outcome policy: {status}')
        checks += 1
    print(f'producer gate-policy checks: {checks} PASS (not source/lab acceptance)')
    return 0


def main():
    if sys.argv[1:] == ['--check-policy']:
        return policy_checks()
    if sys.argv[1:]:
        print('usage: traffic-producer-fixtures.py [--check-policy]', file=sys.stderr)
        return 1
    suites = [unittest.defaultTestLoader.loadTestsFromName(module) for module in MODULES]
    counts = [suite.countTestCases() for suite in suites]
    if not valid_counts(counts):
        print(f'Expected four nonempty modules and {EXPECTED} cases; loaded {counts}', file=sys.stderr)
        return 1
    # Missing/import-error modules create failing loader cases, never silent skips.
    result = unittest.TextTestRunner(verbosity=2).run(unittest.TestSuite(suites))
    print(f'producer source fixtures: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} '
          f'unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if complete(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
