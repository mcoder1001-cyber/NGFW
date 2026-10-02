#!/usr/bin/env python3
"""Preserve the original sixteen named source cases alongside expanded gates."""
import io
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
DIRECTORY = Path(__file__).resolve().parents[2] / 'test/topology/traffic-a'
sys.path.insert(0, str(DIRECTORY))
from check import accepted

# Extracted from fda0ddc7 AST: every original Foundation/Evidence test method.
# Expanded module cases are ALL run by mandatory correlation33/producer40 gates.
ORIGINAL_NAMES = (
    'test_foundation.Foundation.test_plan_never_claims_chain_or_stage_pass',
    'test_foundation.Foundation.test_reserved_invalid_slots_and_cross_slot_environment_refused',
    'test_foundation.Foundation.test_manager_lease_identity_expiry_mode_and_symlink_boundaries',
    'test_foundation.Foundation.test_live_cli_refuses_before_any_external_command',
    'test_foundation.Foundation.test_nonzero_exit_is_not_packet_pass_and_private_log_retained',
    'test_foundation.Foundation.test_timeout_stops_our_process_and_refuses_success',
    'test_foundation.Foundation.test_timeout_stops_descendant_that_ignores_term',
    'test_foundation.Foundation.test_strict_source_gate_rejects_every_nonpassing_status',
    'test_foundation.Foundation.test_log_symlink_and_invalid_timeout_refused_without_execution',
    'test_evidence.Evidence.test_ipv4_tcp_vlan_decoded_with_identity',
    'test_evidence.Evidence.test_truncated_bad_framing_and_partial_snapshot_refused',
    'test_evidence.Evidence.test_fixture_capture_cannot_be_promoted_to_live',
    'test_evidence.Evidence.test_foreign_namespace_loss_digest_and_wrong_interval_refused',
    'test_evidence.Evidence.test_structural_contract_is_never_whole_chain_or_packet_pass',
    'test_evidence.Evidence.test_missing_required_outcome_capture_readback_and_foreign_counter_refused',
    'test_evidence.Evidence.test_selected_go_test_cannot_pass_from_skip_exit_or_zero_execution',
)
EXPECTED = 16


def complete(result):
    return result.testsRun == EXPECTED and accepted(result)


def load_suite(names=ORIGINAL_NAMES):
    if len(names) != EXPECTED or len(set(names)) != EXPECTED:
        raise ValueError('original foundation contract must contain sixteen unique names')
    suite = unittest.defaultTestLoader.loadTestsFromNames(names)
    if suite.countTestCases() != EXPECTED:
        raise ValueError('original foundation suite did not load sixteen cases')
    return suite


def policy_checks():
    checks = 0
    bodies = {'pass': 'pass', 'failure': 'self.fail("fixture")',
              'error': 'raise RuntimeError("fixture")', 'skip': 'self.skipTest("fixture")',
              'expectedFailure': 'self.fail("fixture")', 'unexpectedSuccess': 'pass'}
    for status, body in bodies.items():
        scope = {'unittest': unittest}
        decorator = '    @unittest.expectedFailure\n' if status in ('expectedFailure', 'unexpectedSuccess') else ''
        exec('class Fixture(unittest.TestCase):\n' + decorator + '    def test_case(self):\n        ' + body + '\n', scope)
        suite = unittest.TestSuite(scope['Fixture']('test_case') for _ in range(EXPECTED))
        result = unittest.TextTestRunner(stream=io.StringIO()).run(suite)
        if complete(result) != (status == 'pass'):
            raise RuntimeError(f'incorrect original-case outcome policy: {status}')
        checks += 1
    for count in (0, 15):
        result = unittest.TextTestRunner(stream=io.StringIO()).run(
            unittest.TestSuite(unittest.FunctionTestCase(lambda: None) for _ in range(count)))
        if complete(result):
            raise RuntimeError('zero/reduced foundation execution accepted')
        checks += 1
    # Real loader failure for a missing original method cannot silently pass.
    missing = unittest.defaultTestLoader.loadTestsFromName(ORIGINAL_NAMES[0] + '_missing')
    result = unittest.TextTestRunner(stream=io.StringIO()).run(missing)
    if not result.errors or complete(result):
        raise RuntimeError('missing original method did not fail')
    checks += 1
    for names in (ORIGINAL_NAMES[:-1], ORIGINAL_NAMES[:-1] + (ORIGINAL_NAMES[0],)):
        try:
            load_suite(names)
        except ValueError:
            checks += 1
        else:
            raise RuntimeError('missing/duplicate original name accepted')
    print(f'foundation named-contract policy checks: {checks} PASS (not source/lab acceptance)')
    return 0


def main():
    if sys.argv[1:] == ['--check-policy']:
        return policy_checks()
    if sys.argv[1:]:
        print('usage: traffic-foundation-fixtures.py [--check-policy]', file=sys.stderr)
        return 1
    try:
        suite = load_suite()
    except ValueError as error:
        print(error, file=sys.stderr)
        return 1
    result = unittest.TextTestRunner(verbosity=2).run(suite)
    print(f'original foundation source fixtures: tests={result.testsRun} failures={len(result.failures)} '
          f'errors={len(result.errors)} skipped={len(result.skipped)} '
          f'expectedFailures={len(result.expectedFailures)} '
          f'unexpectedSuccesses={len(result.unexpectedSuccesses)}')
    return 0 if complete(result) else 1


if __name__ == '__main__':
    raise SystemExit(main())
