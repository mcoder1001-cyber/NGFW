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

## In progress / left
- Evidence runs in the throwaway clone `/tmp/vrx-p09-evidence` (host): hook-guarded merge of task/P09 into main; warm quick wall time;
  hand-edited generated client fails; proto change without `contract(` fails; red merge blocked by the hook; `full` without tools/lab
  (WARNING) and with P04's tools/lab on slot 12.
- `docs/status/tasks/P09.md` with the pasted output; final `tools/ci.sh --base main`; commit.

## Decisions taken so far (to be copied to the LOG by the manager)
- pnpm store: kept root's default store (already shared by all worktrees) instead of moving it to `/root/.pnpm-store`.
- `full` passes with a loud WARNING when `tools/lab` is absent (P04 not merged); `VRX_CI_REQUIRE_INTEGRATION=1` makes it fail.
- turbo lint/typecheck/test/build run as one invocation (gen is `cache:false`, would otherwise run 4×); shared `TURBO_CACHE_DIR`.
- `packages/api-client/src/generated` stays in the contract guard (P01's set; not weakened).
