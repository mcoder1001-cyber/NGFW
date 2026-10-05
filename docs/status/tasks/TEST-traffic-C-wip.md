# TEST-traffic-C recovery

Branch: `codex/test-traffic-c`; worktree: `/tmp/ngfw-traffic-c`.
Owned files: `test/topology/traffic-c/**`, `docs/status/tasks/TEST-traffic-C*`.
Base/local remote ancestry: `d314f0728`; checkpoint SHA available with `git rev-parse HEAD`.
Publication: pending first checkpoint push; never interpret local commit as published.
Completed code/tests and unfinished code: see `TEST-traffic-C.md`.
No live processes/daemon/slot ownership. No shared host modifications.
Current failure: live orchestration is deliberately refused because code is incomplete.
Next command: `sed -n '144,220p' packages/schema/src/domains/routing.ts` to derive the MPLS routed candidate.
