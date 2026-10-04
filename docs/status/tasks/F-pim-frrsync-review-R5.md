# R5 final performance and scale review

Reviewed final product SHA: `859638f03a25e97e6f0ca9c2d2418df42d1d3efe`. Initial review: `1025cae41053c5b207754b0496798fcd1f32b514`.

MAJOR: `apps/agent/internal/descriptors/mfib/mfib.go`, Create/Delete full-table dump per dynamic route, activated by `apps/agent/internal/frrsync/pim/pim.go` MaxRoutes=10000. A 10000-route insertion/withdrawal processes approximately 50 million dynamic route records plus foreign shared-table entries, and interface/table dumps repeat per object. This is quadratic churn and cannot support a 10000-route claim.

Safe scoped resolution proposed to manager: explicitly support at most 256 dynamic routes, reject larger snapshots without replacing cache, document the scale limit and test 257-route rejection plus cache preservation. Preserve ownership/existence safety. Record manager-accepted debt for safe per-family conflict snapshots and bounded shared-table retrieval, owner Codex manager, due 2026-10-11. The cap limits feature-induced churn; it is not a throughput claim and does not bound unrelated shared-table entries. Final capped SHA and accepted debt must be rechecked.

Parser input bounded to below 4MiB; cache stores one validated snapshot; joins use interface hashes; read deadline15s; no per-route goroutines or lock held during sync callback. Translation/sorting is bounded by snapshot byte size.

Reviewer verification in this worktree:

```text
cd apps/agent && /workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/frrsync/pim
ok ngfw/agent/internal/frrsync/pim 0.025s
```

Final verification: MaxDynamicRoutes=256 is enforced before cache replacement; the new test accepts 256, rejects 257 despite successful parser validation, and checks unchanged prior cache keys. Documentation distinguishes parser safety bounds from supported runtime state. The manager explicitly accepted inherited per-object dump debt, owner Codex manager, due 2026-10-11, documented in F-pim-frrsync-debt.md and pending root consolidation into docs/tech-debt.md. Foreign/shared-table size remains a limitation of that accepted debt. Ownership checks were preserved.

Verdict at final reviewed SHA: **APPROVE** with the manager-accepted scale debt; initial MAJOR resolved through the explicit support cap and accepted debt.
