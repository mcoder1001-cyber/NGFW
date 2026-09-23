# Task P09 — CI pipeline   (prepend 00-CONTEXT.md)

## Goal
GitHub Actions (or GitLab CI — match the repo host) that makes CI the arbiter for every PR
produced by every agent.

## Build exactly this
1. `pr.yml` on pull_request: `pnpm install --frozen-lockfile` → `pnpm gen` → **fail if
   `git status --porcelain` is non-empty** → lint → typecheck → unit (TS + Go) → build →
   integration (`tools/lab up single`, wait for VPP health, run Go + TS integration
   suites) → Playwright E2E against the same stack → upload traces/videos on failure.
2. Contract guard: if `packages/schema/**`, `packages/proto/**` or `**/gen/**` changed and the
   PR lacks label `contract`, fail with a clear message.
3. Security: `pnpm audit --audit-level=high`, `govulncheck ./...`, gitleaks secret scan,
   Trivy on the built packages/images; a grep gate that fails on `child_process`, `execSync`,
   `exec.Command` outside an allow-listed file.
4. `nightly.yml`: full topology suite + chaos (`kill vpp`) + 1-hour soak with iperf3 through
   VPP; publish a status badge and `docs/status/nightly-latest.md`.
5. Caching: pnpm store, Go build/module cache, the base qcow2 image (rebuild weekly or when scripts/ change; store as a CI artefact).
6. Concurrency: cancel superseded runs per branch. Target wall time for `pr.yml` < 15 min.
7. `CODEOWNERS`: `packages/schema`, `packages/proto`, `deploy/**` require a human reviewer.
8. Branch protection documented in `docs/contributing.md`: no direct push to `main`,
   required checks, squash merge, Conventional Commit title lint.

## Acceptance
- [ ] A PR that hand-edits `packages/api-client` fails at the gen-dirty gate
- [ ] A PR touching `packages/proto` without the label fails the contract guard
- [ ] Green run recorded on the P01 scaffold; total time reported

## Out of scope
Release/publish pipelines (P10), performance CI (needs hardware).
