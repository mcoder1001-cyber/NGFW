#!/usr/bin/env python3
"""Strict fixed selector inventory; real certificate generation must complete."""
from collections import Counter
import importlib.util
from pathlib import Path
import sys
import unittest

EXPECTED = (
    'td19_frr_selection.ControlledSelection.test_actual_gpg_malformed_raw_refuses_without_key_generation',
    'td19_frr_selection.ControlledSelection.test_partial_import_nonzero_never_exports_or_publishes',
    'td19_frr_selection.ControlledSelection.test_secret_or_parser_failure_refuses_before_import',
    'td19_frr_selection.ControlledSelection.test_export_failure_and_existing_output_preserve_private_invariants',
    'td19_frr_selection.ControlledSelection.test_nonregular_size_and_nonfull40_pins_refuse_before_gpg',
    'td19_frr_selection.ControlledSelection.test_repository_entry_failure_at_either_gate_never_mutates_host',
    'td19_frr_selection.RealCertificates.test_real_duplicate_extra_selection_preserves_subkeys_and_exact_installed_set',
    'td19_frr_selection.RealCertificates.test_unselected_signer_refused_until_fixture_owner_explicitly_authorizes_it',
    'td19_frr_selection.RealCertificates.test_missing_authorized_primary_is_not_silently_omitted',
    'td19_frr_selection.RealCertificates.test_conflicting_duplicate_revocation_is_preserved_and_refused',
    'td19_frr_selection.RealCertificates.test_secret_and_malformed_raw_material_are_rejected',
    'td19_frr_selection.RealCertificates.test_actual_non_signing_authorized_primary_is_refused',
    'td19_frr_selection.RealCertificates.test_actual_expired_primary_is_refused',
)


def flatten(suite):
    for item in suite:
        if isinstance(item, unittest.TestSuite):
            yield from flatten(item)
        else:
            yield item


class CompletedResult(unittest.TextTestResult):
    def __init__(self, *args):
        super().__init__(*args)
        self.completed = []

    def stopTest(self, test):
        self.completed.append(test.id())
        super().stopTest(test)


def run_suite(suite, stream=sys.stderr):
    inventory = [test.id() for test in flatten(suite)]
    if Counter(inventory) != Counter(EXPECTED):
        print('REFUSED selector inventory: expected exactly13 named fixtures; '
              f'observed{len(inventory)} missing={list((Counter(EXPECTED)-Counter(inventory)).elements())} '
              f'extra={list((Counter(inventory)-Counter(EXPECTED)).elements())}', file=stream)
        return 1
    result = unittest.TextTestRunner(stream=stream, verbosity=2,
                                     resultclass=CompletedResult).run(suite)
    accepted = (result.testsRun == len(EXPECTED)
                and Counter(result.completed) == Counter(EXPECTED)
                and result.wasSuccessful() and not result.skipped
                and not result.expectedFailures and not result.unexpectedSuccesses)
    print(f'selector fixtures: started={result.testsRun} completed={len(result.completed)} '
          f'failures={len(result.failures)} errors={len(result.errors)} '
          f'skipped={len(result.skipped)} expectedFailures={len(result.expectedFailures)} '
          f'unexpectedSuccesses={len(result.unexpectedSuccesses)} accepted={accepted}', file=stream)
    return 0 if accepted else 1


def main():
    spec = importlib.util.spec_from_file_location('td19_frr_selection',
        Path(__file__).with_name('TD-19-test-frr-selection.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return run_suite(unittest.TestLoader().loadTestsFromModule(module))


if __name__ == '__main__':
    raise SystemExit(main())
