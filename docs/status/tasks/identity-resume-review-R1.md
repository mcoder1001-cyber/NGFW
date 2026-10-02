# Identity resume — fresh independent R1 review

Reviewed HEAD `721ecc1dc15a99c8d93bee6a9126c545676fc89f`, tree `1a46cbd6e1dc7b89043a3040c5e8015d73b06848`, against main `53a43ce5b91e71f3fedc282f9c5c54ba22fc9fb8`. Isolated review branch `codex/identity-correctness-review-20261002`, worktree `/workspace/scratch/de92de7d9874/NGFW-identity-r1`. No product edits.

Read F-system-identity task, R1/shared review instructions, current envelope/checkpoints/user guide, prior independent review and its corrections, plus subsequent integration review from the old read-only worktree. Product comparison to current recovered `1532fd59` and `2d9dc186` is empty. Comparing git blob IDs across repositories proves all apps/packages at historical integration-reviewed `30de41fd4f9a7913a2614c46a2ec5557a3bfcadc` equal the reviewed current HEAD. This is a recovered continuation, not a newly implemented renderer lifecycle.

## Findings

No unresolved R1 BLOCKER or MAJOR in this bounded code continuation. Installed hostname/timezone, configured resolver settings and runtime-file observations are independently read from bounded files rather than fabricated from desired-state memory. Unavailable facts are explicit; resolver readability is not mislabeled daemon health. Slot scopes omit global kernel hostname/resolver. Uptime rejects nonfinite/out-of-range/invalid text. Timezone symlink traversal outside zoneinfo is rejected.

Protected system state retains health/revision/pending/sync while adding nullable identity; older/unavailable agent RPC is caught. Public banner reads only committed login text, bounds UTF-16 safely and is rendered literally; failed banner retrieval does not prevent login. UI distinguishes installed facts from candidate/running configuration, maintains editable candidate and pointer errors, handles unavailable identity, and uses persisted Persian digit settings.

No mutation/rollback path was added by this delta. Existing temporary-root golden, validation, no-op restart and lifecycle/drift tests remain present; observational tests touch actual temporary files. Real slot commit/400/restart logs and real-endpoint browser screenshot remain owner-authorized deferred, NOT PASS. DNS service restart remains an explicit existing privileged handoff, not represented as implemented.

## Acceptance mapping

| Continuation requirement | Implementation / evidence |
| --- | --- |
| Installed hostname/timezone/DNS and partial errors | sysident/state.go; real temporary-file state tests |
| Owner and slot isolation | rpc_system_identity.go; TestSystemIdentityStateOwnerAndWiredPaths |
| Existing health compatibility / old RPC | state.controller.ts; system-identity-state.test.ts |
| Committed public login banner / failure / length / literal text | login-banner.controller tests; LoginBanner UI tests; route-guard inventory |
| Candidate editing / pointer errors / localized uptime | SystemIdentityPage.test.tsx |
| Prior renderer validation and idempotent restart | sysident_test.go golden, validator, unchanged-writes-nothing and lifecycle tests |
| Complete quick gate, real appliance/browser/restart | Still required/deferred as stated below; no invented PASS |

## Actual independent checks

Pinned PATH `/workspace/scratch/96b8b6fbc8a7/toolchain/bin`. API/web tests executed this review worktree source with dependencies symlinked to the current identity worker install; workspace package build dependencies therefore resolve there. Not a clean-install/full-build claim.

```text
# apps/api
node node_modules/vitest/vitest.mjs run src/state/login-banner.controller.test.ts src/state/system-identity-state.test.ts src/auth/route-guard.test.ts
Test Files 3 passed (3)
Tests 13 passed (13)
Duration 60.80s

# apps/web
node node_modules/vitest/vitest.mjs run src/domains/system/identity/LoginBanner.test.tsx src/domains/system/identity/SystemIdentityPage.test.tsx
Test Files 2 passed (2)
Tests 6 passed (6)
Duration 60.86s

# root
bash tools/ci.sh check --base main
check PASSED (0m07s)
```

All above exited 0. Check includes contract guard, forbidden patterns, gitleaks (approximately 120.42 KB, no leaks), packet-trace ban and slot scheme. Web run printed the existing HydrateFallback warning. Initial Vitest invocation from repo root failed MODULE_NOT_FOUND; corrected package-directory invocations above passed. `git diff --check main...HEAD` exited 0.

Full `tools/ci.sh --base main` was not duplicated by this reviewer while the developer runs the current complete gate; this is an explicit limitation relative to R1 prompt item 5. Developer-reported TMPDIR licensing replay is not claimed as this reviewer's execution or a complete green gate. Manager must obtain unchanged complete quick PASS on final integration tree before merge. No live VPP/daemon/network delivery/browser acceptance is claimed.

Fresh Go checks, with GOTOOLCHAIN=local, GOMODCACHE=/workspace/scratch/96b8b6fbc8a7/toolchain/cache/go-mod, GOCACHE=/tmp/identity-r1-go-cache, GOMAXPROCS=2, GOFLAGS=-p=2:
```text
# apps/agent
go test -race ./internal/renderers/sysident ./internal/agent -run 'Test(ObservedState|SystemIdentityState)'
ok ngfw/agent/internal/renderers/sysident 1.053s
ok ngfw/agent/internal/agent 1.238s

go test -race ./internal/renderers/sysident
ok ngfw/agent/internal/renderers/sysident 1.156s
```
Both exited 0; full renderer unit race suite covers preserved golden/lifecycle/validation behavior without modifying real host identity.

Verdict: **APPROVE — R1 code correctness/tests only**. Complete merge verification remains NOT PASSED by this report and must be supplied by the mandatory unchanged quick gate.
