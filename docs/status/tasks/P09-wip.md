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

## In progress / left
- Evidence runs in a throwaway clone (`/tmp/vrx-p09-evidence` on the host): hand-edited generated client fails; proto change
  without `contract(` commit fails; `full` with a stub `tools/lab`; pre-merge-commit hook blocks a red merge.
- `docs/status/tasks/P09.md` with the pasted output; final `tools/ci.sh --base main`; commit.

## Decisions taken so far (to be copied to the LOG by the manager)
- pnpm store: kept root's default store (already shared by all worktrees) instead of moving it to `/root/.pnpm-store`.
- `full` passes with a loud WARNING when `tools/lab` is absent (P04 not merged); `VRX_CI_REQUIRE_INTEGRATION=1` makes it fail.
- turbo lint/typecheck/test/build run as one invocation (gen is `cache:false`, would otherwise run 4×); shared `TURBO_CACHE_DIR`.
- `packages/api-client/src/generated` stays in the contract guard (P01's set; not weakened).
