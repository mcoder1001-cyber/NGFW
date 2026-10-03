# P11 parallel build intake work

Branch: codex/p11-resume-parallel-20261003; base 19aa88a5.
Owned: deploy/strongswan/**; deploy/debian/vrx-strongswan/**;
docs/status/tasks/P11-parallel-*.
Checkpoint publication pending; resolve actual local HEAD/remote branch SHA.

Completed code: read-only bounded snapshot and verified VPP staging quartet
metadata/hash gate; source digest supplied explicitly by a separately trusted
caller, tar member checks for pregenerated release files, no extraction/download.
Exact historical source metadata recorded without authentication/security claims.
Meaningful synthetic positive/negative archive tests plus real VPP refusal.
Final focused fixtures: 9 PASS; actual command/output below. No real input build claimed.
Next command: PYTHONDONTWRITEBYTECODE=1 python3 deploy/strongswan/test_verify_inputs.py.
Unchanged check gate PASS final recheck (9s). Local product checkpoint27d3d800; publish via connector.
Whole quick hosted gate/panel deferred to durable review queue when slots free.

P10 handoff: frozen local71cee90b remotee53bc20b PR98. Verifier23 PASS113.268s,
export10 PASS215.337s, installer11 PASS484.596s. Own redundant quick602763 and
observed descendants cancelled for capacity before completion, not PASS.
Artifact attach pending functions cell26 was cancelled before response.

P11 remains incomplete: safe builder/.deb, source security/licensing resolution,
plugin ABI/id-window corrections, full integration, clean target and hardware
acceptance NOT RUN. No security boundary changed or gates weakened.

## Actual focused fixture evidence

```text
test_hash_version_and_metadata_mismatch (__main__.IntakeTests.test_hash_version_and_metadata_mismatch) ... ok
test_missing_duplicate_and_unsafe_staging_entries (__main__.IntakeTests.test_missing_duplicate_and_unsafe_staging_entries) ... ok
test_missing_generated_files_and_nonexecutable_configure (__main__.IntakeTests.test_missing_generated_files_and_nonexecutable_configure) ... ok
test_positive_reports_hashes_without_modifying_inputs (__main__.IntakeTests.test_positive_reports_hashes_without_modifying_inputs) ... ok
test_real_vpp_gate_refuses_synthetic_build (__main__.IntakeTests.test_real_vpp_gate_refuses_synthetic_build) ... ok
test_source_and_expanded_bounds (__main__.IntakeTests.test_source_and_expanded_bounds) ... ok
test_source_symlink_and_replacement_refused (__main__.IntakeTests.test_source_symlink_and_replacement_refused) ... ok
test_tar_paths_duplicates_links_and_special_files_refused (__main__.IntakeTests.test_tar_paths_duplicates_links_and_special_files_refused) ... ok
test_truncated_bzip2_cli_refuses_without_traceback (__main__.IntakeTests.test_truncated_bzip2_cli_refuses_without_traceback) ... ok

----------------------------------------------------------------------
Ran 9 tests in 13.482s

OK
```

## Actual unchanged check evidence

```text
tools/ci.sh check --base origin/main
check PASSED (0m09s)
```

Complete quick/hosted CI and mandatory fresh reviewer panel are NOT RUN for this
P11 slice; keep the PR in the review queue, unmerged.

## Provisional R2 correction

Root identified a public module API bypass: verify(..., digest=None) could use
the metadata-only optional digest helper. Public verify now independently refuses
missing/non-string/invalid digests before any snapshot; direct None/empty/int
and CLI empty-digest negatives are included. No approval inferred.

```text
test_hash_version_and_metadata_mismatch (__main__.IntakeTests.test_hash_version_and_metadata_mismatch) ... ok
test_missing_duplicate_and_unsafe_staging_entries (__main__.IntakeTests.test_missing_duplicate_and_unsafe_staging_entries) ... ok
test_missing_generated_files_and_nonexecutable_configure (__main__.IntakeTests.test_missing_generated_files_and_nonexecutable_configure) ... ok
test_positive_reports_hashes_without_modifying_inputs (__main__.IntakeTests.test_positive_reports_hashes_without_modifying_inputs) ... ok
test_real_vpp_gate_refuses_synthetic_build (__main__.IntakeTests.test_real_vpp_gate_refuses_synthetic_build) ... ok
test_source_and_expanded_bounds (__main__.IntakeTests.test_source_and_expanded_bounds) ... ok
test_source_symlink_and_replacement_refused (__main__.IntakeTests.test_source_symlink_and_replacement_refused) ... ok
test_tar_paths_duplicates_links_and_special_files_refused (__main__.IntakeTests.test_tar_paths_duplicates_links_and_special_files_refused) ... ok
test_truncated_bzip2_cli_refuses_without_traceback (__main__.IntakeTests.test_truncated_bzip2_cli_refuses_without_traceback) ... ok

----------------------------------------------------------------------
Ran 9 tests in 13.373s

OK
```

Corrected hosted complete quick and fresh panel remain required.
