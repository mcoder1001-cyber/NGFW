#!/usr/bin/env python3
"""Bounded gate controls; does not run real selector fixture factory."""
import importlib.util
import io
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('selector_gate', Path(__file__).with_name('TD-19-run-frr-selection.py'))
gate = importlib.util.module_from_spec(spec); spec.loader.exec_module(gate)


def named(name, function):
    class Named(unittest.FunctionTestCase):
        def id(self): return name
    return Named(function)


def suite(names=gate.EXPECTED, first=None):
    return unittest.TestSuite(named(name, first if i == 0 and first else lambda: None)
                              for i, name in enumerate(names))


class Policy(unittest.TestCase):
    def run_gate(self, fixtures, expected):
        output = io.StringIO()
        self.assertEqual(gate.run_suite(fixtures, output), expected, output.getvalue())
        return output.getvalue()

    def test_exact_named_inventory_completes(self):
        self.assertIn('completed=13', self.run_gate(suite(), 0))

    def test_zero_reduced_missing_extra_duplicate_and_replaced_refuse(self):
        for names in ((), gate.EXPECTED[:-1], gate.EXPECTED + ('foreign.test',),
                      gate.EXPECTED[:-1] + (gate.EXPECTED[0],),
                      gate.EXPECTED[:-1] + ('foreign.test',)):
            with self.subTest(names=names):
                self.assertIn('REFUSED', self.run_gate(suite(names), 1))

    def test_failure_error_and_skip_refuse(self):
        def fail(): raise AssertionError('fixture failure')
        def error(): raise RuntimeError('fixture error')
        def skip(): raise unittest.SkipTest('fixture skip')
        for function in (fail, error, skip):
            with self.subTest(function=function): self.run_gate(suite(first=function), 1)

    def test_expected_failure_and_unexpected_success_refuse(self):
        class Outcome(unittest.TestCase):
            @unittest.expectedFailure
            def test_expected(self): self.fail('expected failure')
            @unittest.expectedFailure
            def test_unexpected(self): pass
            def id(self): return gate.EXPECTED[0]
        for name in ('test_expected', 'test_unexpected'):
            fixtures = suite(gate.EXPECTED[1:]); fixtures.addTest(Outcome(name))
            self.run_gate(fixtures, 1)

    def test_class_setup_failure_cannot_claim_completed_inventory(self):
        class FailedSetup(unittest.TestCase):
            @classmethod
            def setUpClass(cls): raise RuntimeError('factory unavailable')
            def test_case(self): pass
            def id(self): return gate.EXPECTED[0]
        fixtures = unittest.TestSuite([FailedSetup('test_case'), *list(suite(gate.EXPECTED[1:]))])
        output = self.run_gate(fixtures, 1)
        self.assertIn('completed=12', output)
        self.assertIn('errors=1', output)


if __name__ == '__main__':
    result = unittest.TextTestRunner(verbosity=2).run(unittest.TestLoader().loadTestsFromTestCase(Policy))
    raise SystemExit(0 if (result.testsRun == 5 and result.wasSuccessful()
                          and not result.skipped and not result.expectedFailures
                          and not result.unexpectedSuccesses) else 1)
