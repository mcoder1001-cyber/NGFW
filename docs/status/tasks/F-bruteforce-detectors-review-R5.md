# R5 performance and scale review

Reviewed product SHA: `5c88cb5f2b09f59a161dcfa6d72cf89b831c207a`.

No BLOCKER or MAJOR findings in the new detector path. Distinct-port windows cap 4096 ports/source, 10000 sources and 100000 aggregate observations independently of configured block capacity. Capacity reclamation scans globally at most once per second; ordinary events scan only their bounded source window. Expiry and clear decrement aggregate accounting. Exhaustion discards evidence without inventing threshold hits. Host journal output is limited to 4MiB and 5000 records; cursor retention is 20000, IKE context 1024, native deduplication 10000 with 24-hour expiry. Polls use fixed intervals, runner timeout and cancellation.

MINOR (inherited): `apps/api/src/features/auto-block/engine.ts`, SlidingWindows.hits has no aggregate source limit and the service prunes with a 24-hour horizon. Churn in SSH/web-login sources can grow memory within that horizon. Follow-up should impose a fixed aggregate source/sample budget and use configured expiry; scan evidence in this branch uses the separate bounded implementation.

Verification run by reviewer in this worktree:

```text
cd apps/api && pnpm exec vitest run src/features/auto-block/port-window.test.ts
Test Files 1 passed (1)
Tests 5 passed (5)
Duration 657ms
```

These tests cover independent source cap, aggregate accounting, refresh, exact expiry, capacity recovery and unsupported thresholds. No packet throughput or hardware acceptance claimed.

Verdict: **APPROVE** (one inherited MINOR follow-up).
