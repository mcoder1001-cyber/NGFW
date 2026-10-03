# OSPF FRR adjacency compatibility correction

Branch `codex/ospf-state-compat-20261003`, new isolated OSPF-state-compat worktree from frozen `ae282009`; initial source and root integration validation trees untouched. Owned only state.ts, state.test.ts and this report. Root owns publication/tests/shared integration.

Addresses independent reviewer concrete FRR10.4 shapes: point-to-point neighbor state `Full/-` (ospf_dump.c:153) and NBMA Attempt row keyed `neighbor` while router ID is unknown (ospf_vty.c:4460). Source references are reviewer-provided; primary upstream links are being supplied by reviewer. State allowlist now accepts dash role suffix. Only the known `neighbor` placeholder key bypasses IPv4 router-ID requirement; its shape and count are validated, row omitted, and generic partial-observation warning set so other valid adjacencies remain available. All-placeholder data returns empty neighbors WITH partial warning. Arbitrary malformed router IDs still fail closed.

Added three realistic projection regressions: valid Full/- retained; NBMA placeholder plus healthy adjacency skips only placeholder and announces partial data; all-placeholder data announces partial observation and 2001 placeholder rows hit existing work bound. No DTO change or raw/provider diagnostics exposed. Existing byte/instance/count limits remain.

Actual checks: Prettier and git diff --check passed. No Vitest/typecheck/full suite or host calls run. Root finite command `pnpm --filter @ngfw/api exec vitest run src/features/ospf/state.test.ts src/features/ospf/ospf.controller.test.ts --maxWorkers=1`, then feature eslint/APItypecheck on integrated tree. Independent re-review required. Registered root checkpoint b9c1e932 receives only this successor diff; its existing CI job is not edited.

## Mapped-error fixture diagnosis and passing follow-up

Root focused run and author reproduced the original mapped-agent-error case failing after explicit catch/type/status/body assertions. Temporary safe step prints showed all assertions completed; product controller did not swallow or wrap the error. Replacing the reused rejected mock fixture with a plain async failing adapter, preserving all public problem assertions, passed the isolated case. No claim about a general Vitest defect is made; this fixture no longer depends on that rejection-mock behavior. Temporary prints were removed.

Added actual HTTP503 regression using the existing ProblemFilter and real AuthGuard in the minimal app, with a plain failing RPC adapter; verifies status, application/problem+json, exact mapped public body and request instance. Product code unchanged.

Actual final author validation: **21/21 tests, 2/2 files passed**,17.43s execution after32s slot3 queue, under existing finite test-fast helper and60s outer deadline. Direct Vitest binary used with compatible root-prepared dependencies in untracked links. Earlier runs were19/20 failed, then isolated repaired case1/1 passed; evidence is preserved honestly. Exact passing command from apps/api:

```sh
timeout 60 ../../tools/test-fast.sh /root/.codex/worktrees/0b16/developers/OSPF-main/apps/api/node_modules/.bin/vitest run src/features/ospf/state.test.ts src/features/ospf/ospf.controller.test.ts --maxWorkers=1
```

Prettier and diff check passed. Root integrated typecheck/lint/generation, independent re-review and hosted complete quick gate still required. Initial pnpm wrapper rejected a symlink task-state directory after a cached dependency preparation step; author switched to direct binary rather than altering its guard.
