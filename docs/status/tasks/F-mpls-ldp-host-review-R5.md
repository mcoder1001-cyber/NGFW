# R5 final performance and scale review

Reviewed product SHA: `339f400c1a137dcc0e988223d368b4ba97704420` (includes bounded-source checkpoint 99218b6d).

No remaining unaccepted BLOCKER or MAJOR findings. The binding/adjacency join now hashes neighbor links, iterates active prefix hops and memoizes prefix/neighbor matches. Explicit join-work limit 100000 prevents adversarial neighbor/prefix combinations from becoming unbounded quadratic work; context checks run inside the join. Reads cap each command at 4MiB and cap rows, RIB hops, adjacency rows and translated paths at 10000. Discovery interface count is bounded separately.

Dynamic reconciliation supports 256 distinct label routes, rejects oversized observations, and retains the last good state during the documented 60-second failure hold-down. Per-route paths are deduplicated and reject more than 255 distinct paths. Cache holds only one observation/route snapshot; dirty apply failures retry unchanged state; prospective LCP and label-range filtering suppress stale dependencies. No per-binding goroutines are introduced.

Accepted MAJOR debt: named LDP Create retains the inherited per-label authoritative table dump for ownership/conflict safety. The 256-label cap bounds feature-induced churn but does not bound unrelated shared-table rows. Manager accepted safe per-transaction bulk conflict retrieval follow-up, owner Codex manager, due 2026-10-11, in F-mpls-ldp-host-scale-debt.md, with root consolidation into docs/tech-debt.md. No high-scale or throughput support claimed.

Reviewer verification in this worktree:

```text
cd apps/agent && /workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/frrsync/ldp
ok ngfw/agent/internal/frrsync/ldp 0.038s
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/subsystems -run TestLdp
ok ngfw/agent/internal/subsystems 0.028s
```

Verdict: **APPROVE** with manager-accepted scale debt.
