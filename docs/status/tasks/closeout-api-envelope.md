# API NAT46 negative-validation fixture envelope

- Branch/worktree: `codex/closeout-api-nat46`, `/root/ngfw-wt/codex-closeout-api-nat46`.
- Base/source: `af83737b2`; manager supplied root aggregate API integration failure.
- Owner: management_acceptance child agent; owned file `apps/api/test/e2e/nat46.e2e.test.ts` and `docs/status/tasks/closeout-api*`.
- Scope: correct explicit schema-vs-semantic rejection boundary; retain strict problem/pointer assertions. No product source changes.
- Slot9 PostgreSQL and Valkey only; suite deliberately uses existing in-process fake agent. No real VPP mutation/host service restart.
- Complete aggregate suite/quick gate and independent review owned by manager.
- Publication blocked by manager's observed HTTP403; no remote SHA/success claimed.
