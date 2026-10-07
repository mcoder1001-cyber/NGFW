# Global Blocking missing host-independent controls

Owner assignment 2026-10-07. Branch `codex/resume-global-blocking-tests-20261007`;
worktree `/root/ngfw-wt/resume-global-blocking-tests-20261007`.
Clean base origin/main `3ddb1680e475e94d43e8036cd3776bc60c87208b`.
Read published resume-host-prereqs audit and original F-global-blocking prompt/status.
Reuse integrated implementation. Own ONLY these exact files before editing tests:

- `apps/api/src/features/global-blocking/refresh-retention.test.ts` (new)
- `apps/agent/internal/renderers/nftables/blocking_retention_test.go` (new)
- `docs/status/tasks/resume-global-blocking-tests-20261007-envelope.md`
- `docs/status/tasks/resume-global-blocking-tests-20261007-wip.md`

Fixtures live inside those tests; no extra utility ownership needed.
Add meaningful failed/empty/garbage refresh retention and IPv6/anti-lockout controls
using existing contracts. No production, contract, main, board, other-worktree,
host configuration, packet, browser or performance edits/execution.
If a real bug reproduces, publish negative test and propose exact narrow production
fix for manager authorization; do not implement it under this envelope.
Commit coherent changes/publish every checkpoint, WIP within15minutes. Focused
tests through tools/heavy.sh; complete hosted quick/review/integration manager-owned.
Native enforcement, real scheduled feed and browser acceptance explicitly NOTRUN.
