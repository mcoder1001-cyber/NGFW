# Task P09 — CI gate for a local-only repository   (prepend 00-CONTEXT.md)

## Goal
Git on this host has no remote, no PRs and no CI service. `tools/ci.sh` (already exists, minimal) **is** the CI: the manager runs it in
every worker's worktree before merge and on `main` after merge. Make it complete, fast, and impossible to bypass silently.

## Build exactly this
1. `tools/ci.sh quick` (default, unit-only, < 6 min): `pnpm install --frozen-lockfile --prefer-offline` → `pnpm gen` → **fail if generated
   paths are dirty** (`packages/proto/gen`, `apps/agent/gen`, `packages/schema/dist`, `packages/api-client/src/generated` — not the whole tree)
   → lint → typecheck → unit tests (TS + Go, `VRX_INTEGRATION` unset) → build → `make lint test build` in `apps/agent`.
2. `tools/ci.sh full`: quick + integration: takes `flock -x /run/lock/vrx-lab.lock`, exports the slot from `tools/lab env 12` (the CI slot),
   `tools/lab rig up w12`, runs Go/TS integration suites with `VRX_INTEGRATION=1`, `rig down`, releases the lock. Never restarts VPP.
3. `tools/ci.sh --base <ref>`: contract guard (contract files changed ⇒ a `contract(` commit subject on the branch, else fail) and
   forbidden-pattern grep (shell exec in control plane, Dockerfiles/compose, `pkill`/`killall` in scripts, secrets patterns).
4. `golangci-lint` installed (binary from GitHub releases to `/usr/local/bin`; `make lint` uses it) and `gitleaks` if downloadable;
   both wired into quick. Record versions in `docs/contributing.md`.
5. Pre-merge hook the manager can install: `tools/ci.sh install-hooks` → `.git/hooks/pre-merge-commit` on `/root/ngfw` running `quick`.
6. Speed: pnpm store shared (`/root/.pnpm-store`), Go build cache, turbo cache; quick on an unchanged tree < 2 min.
7. `.github/workflows/ci.yml` that just calls `tools/ci.sh quick` — kept for the day a remote exists; not required to run now.
8. `docs/contributing.md`: the gate, the contract rule, the shared-host rules, how the manager merges.

## Acceptance (paste the evidence)
- [ ] A branch that hand-edits `packages/api-client/src/generated` fails the dirty gate with a clear message
- [ ] A branch touching `packages/proto` without a `contract(` commit fails `--base main`
- [ ] `tools/ci.sh quick` on `main`: `CI GATE PASSED`, wall time reported; `tools/ci.sh full` green with the rig (evidence pasted)

## Out of scope
GitHub Actions runners, release/publish pipelines (P10), performance CI, any change to `/etc/vpp`.
