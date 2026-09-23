# P09 — WIP status (kept current for a CONTINUE respawn)

branch `task/P09` · worktree `/root/ngfw-wt/P09` · slot 4 · started 2026-09-23 13:13

## Done
- `tools/ci.sh` rewritten: `quick` (default) · `full` · `check` · `gen-check` · `install-tools` · `install-hooks`; `--base <ref>` on any mode.
  Backward compatible (`tools/ci.sh`, `tools/ci.sh --base main`). Per-step logs under `/root/ngfw-wt/logs/ci/`, summary with wall time,
  last line exactly `CI GATE PASSED`.
- golangci-lint 2.13.2 + gitleaks 8.30.1 installed on the host into `/usr/local/bin` via `tools/ci.sh install-tools` (sha256-verified).
- `.github/gitleaks.toml`, `.github/workflows/ci.yml`, root `package.json` `gen:check` → `tools/ci.sh gen-check`.
- `docs/contributing.md` written.
- First real run in the worktree: `tools/ci.sh --base main` → CI GATE PASSED, 1m04s (cold turbo cache).

## Done in the CONTINUE respawn (2026-09-23 ~15:00)
- quick step 8: every Go module under `test/` — `gofmt -l`, `go vet ./...`, `go test -count=1 ./...` in unit mode (P04 review F6).
- `full`: exclusive lock for `rig up`, converted to shared while the suites run (harnesses take their own `flock -s`; exclusive
  would deadlock), exclusive again for `rig down`; exports `VRX_LAB_LOCK_HELD=1 VRX_CI_FULL=1`; `-timeout 20m` on the Go suites.
- `slot_env` fallback = `tools/lab env` arithmetic (D-025: metrics 9100+10N+1; adds VRX_VALKEY_DB).
- docs/contributing.md updated (step 8, `full` lock behaviour, D-031 note, slot 12 values). shellcheck clean.

## Done later in the respawn (~15:20)
- Dirty gate compares working tree vs index (a staged merge inside the hook is not "dirty"); contract guard runs first (git-only).
- Hook: git writes MERGE_HEAD only after pre-merge-commit → the merged ref comes from GIT_REFLOG_ACTION; hook runs
  `quick --base HEAD` with `VRX_CI_HEAD_REF=<ref>` (contract guard + gitleaks on the branch being merged). Verified: contract-less
  proto branch stopped in 1.2 s, contract branch passes in 44 s.
- `full`: P04's fix round made `tools/lab rig up` refuse under an exclusive holder → exclusive barrier, then shared for rig up →
  suites → rig down. Evidence run F (main+P04, slot 12) in progress.

## Closed 2026-09-23 ~15:25 — everything is in docs/status/tasks/P09.md (evidence §1–§10). Nothing left.

## Decisions taken so far (to be copied to the LOG by the manager)
- pnpm store: kept root's default store (already shared by all worktrees) instead of moving it to `/root/.pnpm-store`.
- `full` passes with a loud WARNING when `tools/lab` is absent (P04 not merged); `VRX_CI_REQUIRE_INTEGRATION=1` makes it fail.
- turbo lint/typecheck/test/build run as one invocation (gen is `cache:false`, would otherwise run 4×); shared `TURBO_CACHE_DIR`.
- `packages/api-client/src/generated` stays in the contract guard (P01's set; not weakened).
