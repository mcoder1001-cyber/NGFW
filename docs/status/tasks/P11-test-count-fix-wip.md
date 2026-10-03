# P11 current VPP script count — frozen tested fixture correction

Branch `codex/p11-test-count-fix-20261003`; own isolated `NGFW-p11-test-count-fix`; base `4909418bfc0f2a53700833982b16fc175d9892c6`.
Owned ONLY `deploy/strongswan/test_verify_inputs.py`, `docs/status/tasks/P11-test-count-fix-envelope.md`, and this WIP.
Frozen test source `753ca48040a7b6cc2383b733a2376e48bfb769d1`; initial envelope `7f2e941f8422eacaba5c940569896d6208bb4bce`. Final evidence checkpoint SHA/tree are obtained after this commit. Root connector-publication acknowledgement pending; developer does not invent a remote SHA or self-review/merge.

## Actual RED reproduction

Root integration log `/tmp/p11-affinity-composition.log` failed one test in27.536s at `AssertionError: 72 != 66`. This root result is not this worker's test execution.

Own fresh reproduction before correction:

```text
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest test_verify_inputs.IntakeTests.test_real_vpp_tests_and_static_verifier_in_snapshot_environment -v
AssertionError: 72 != 66
Ran 1 test in 28.905s
FAILED (failures=1)
```

Raw `/tmp/p11-test-count-fix-before.log`. The reviewed affinity change adds six genuine current-script cases; the hardcoded historical 66-total assertion is brittle. No product or VPP script failure is implied by that count mismatch.

## Correction

Keep BOTH actual unchanged commands: full current `deploy/vpp/tests/run.sh` and full static `deploy/vpp/verify.sh`, with no no-tests flag or positive static stub. Replace the historical fixed total with exact current-script transcript invariants: nonempty output; one terminal summary with strictly positive passed total and zero failures; EVERY preceding line an ok numbered record; record ordinals exactly 1 through the summary's declared count. Failed/unknown records, duplicate/missing/extra ordinals, duplicate summaries, zero tests and count disagreements refuse. A >=66-only condition is not used. Comment now says full current test script.

No product code, VPP scripts/gates, CPU cases, skips, builder, live VPP, service, original/root integration/P10/main or other-worktree changes. PYTHONDONTWRITEBYTECODE=1 prevents untracked caches. Existing full original intake/stage test counts remain unchanged.

## Actual GREEN verification

```text
tools/ci.sh check --base 4909418bfc0f2a53700833982b16fc175d9892c6
check PASSED (0m09s)
git diff --check
PASS
PYTHONDONTWRITEBYTECODE=1 python3 test_verify_inputs.py
Ran 11 tests in 97.737s
OK (exit0, no skips)
PYTHONDONTWRITEBYTECODE=1 python3 test_prepare_stage.py
Ran 23 tests in 139.118s
OK (exit0, no skips)
```

Both complete fixture suite logs contain the REAL full current script `72 passed, 0 failed`, then the REAL static verifier's `ok tests/run.sh: 72 passed, 0 failed` and `verify.sh: OK`, under `verified_snapshot` clean HOME=/nonexistent with no inherited BASH_ENV. The positive script/static commands are NOT mocked. Existing final VPP package/source provenance boundaries in the tiny synthetic intake/stage fixtures ARE explicitly stubbed; these fixture passes do NOT approve real input/build provenance, installability, plugin linkage, native build, service operation or release/whole P11 acceptance. Real gate refusal of synthetic build remains a passing negative test.

Raw `/tmp/p11-test-count-fix-intake.log`, `/tmp/p11-test-count-fix-stage.log`.

Additional meaningful parser checks compile the ACTUAL inline assertion AST block (not a duplicated implementation) against synthetic output transcripts: consistent complete totals1/66/72/73 PASS;10 refusal cases PASS (empty, zero, failed record, failed summary, count disagreement, duplicate record, missing ordinal, extra record, duplicate summary, unknown record). Raw `/tmp/p11-test-count-fix-transcripts.log`. These are output-validation checks, not mocked VPP gate or artifact provenance evidence.

## Remaining and publication request

Current own code failure: none. All own tests ended; own worktree clean before this documentation-only evidence checkpoint. Root explicitly directed no additional full local quick for this bounded fixture correction; the unchanged complete FINAL hosted gate remains mandatory. Exact-tree publication identity confirmation, fresh independent R1 review and final integration/hosted gates remain pending. No readiness/merge claim before those pass.

Suggested PR title: `fix(test): validate complete current VPP success records without a historical fixed count`.
Suggested body: The reviewed CPU-affinity change adds six real VPP tests, making the P11 intake fixture's old66 assertion fail despite all72 passing. Require exact terminal positive/zero-failure summary and sequential successful-record agreement while executing both full current real commands unchanged. All11 intake and23 stage fixtures pass; both actual RED and GREEN evidence plus synthetic provenance boundaries remain explicit. No product/gate/skip/builder change. Independent review and unchanged final hosted gate required before guarded integration.

Next commands: `git rev-parse HEAD && git rev-parse HEAD^{tree}`; root connector-publishes exact final tree and requests independent R1 plus unchanged final hosted gate. Developer remains available for remote identity confirmation, never self-reviews or merges.
