# Scheduler recreation prerequisite correction

Branch `codex/pppoe-recovery-graph-20261008`, local base `2b3709fe`; original regression commits `03fe3cf9` and `a26a6004`. Published regression `ceaa909f30226b90b37c38bcd849d04a772d0f93` on public candidate `4ff2c73002ce24b28824950b1e6e5f85805dc7d0`.

Owned source: `internal/scheduler/reconciler.go`, new `recreation_prerequisites_test.go`, and new PPP `carrier_recovery_graph_test.go`. No carrier runtime or shared desired files changed.

Observed failure: helper inventory requests namespace repair while raw VPP TAP is absent; actual namespace descriptor recreation stopped live daemon/transit correctly but immediately recreated daemon before the missing raw TAP's later planned CREATE. Transaction became degraded. Host-only TAP loss retaining both VPP objects passed.

Correction: extend live-dependent recreation with transitive pending planned CREATE prerequisites, following provided aliases and non-mutating intermediate dependencies. Existing topo orders combined restore set and rejects cycles. New prerequisite creation uses normal executor path and journal, marks done, and prevents duplicate later CREATE. Only already-admitted planned creates qualify; absent unconfigured optional dependencies are not synthesized.

Validation actually executed (Go 1.26.0, race, count=1):

- Both actual NamespaceDescriptor/scheduler TAP-loss graph controls PASS, 1.046s. Fresh scheduler reads actual repair marker, deletes daemon then surviving TAPs then exact old generation/inode namespace, creates fresh namespace before TAP consumers, and binds new consumers to new generation.
- Focused generic prerequisite-chain/alias and later-failure rollback test PASS, 1.030s. Asserts create once, optional absence preserved, reverse journal deletes early prerequisites and restores original objects.
- Entire scheduler package race tests PASS, 2.241s; this is a source-package check, not aggregate/hosted CI.
- `git diff --check` PASS.

Remaining: independent scheduler review, parent integration into final carrier candidate and combined unchanged-source verification. Aggregate CI remains held by user request. Native target and laboratory operations were not executed.
