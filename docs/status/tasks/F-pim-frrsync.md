# F-pim-frrsync implementation evidence

Branch: task/F-pim-frrsync-20261004. Initial coherent code checkpoint: 4c006935. Manager verified remote code checkpoint 95013463923f0d68ff2bf73ed5fd6d2fc8e9afed.

Built the existing IPv4 PIM contract through the FRR RP section and interface seam S2; preserved it through FRRDoc projection. The default-VRF FRR JSON reader feeds an exclusive mfib.route.pim descriptor through dynamic seam S1. It validates full observations before replacing the synchronized cache, preserves forwarding on error and withdraws on a successful empty snapshot. Mapping derives from each transaction's configuration, so disabled PIM and removed dependencies withdraw routes. Dynamic and static descriptors use separate persistent ownership records and cannot adopt/overwrite each other. No poller writes VPP directly.

Actual verification (Go 1.26.0):

```text
go test -race -count=1 ./internal/renderers/frr/pim ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/desired
ok ngfw/agent/internal/renderers/frr/pim 1.056s
ok ngfw/agent/internal/frrsync/pim 1.108s
ok ngfw/agent/internal/descriptors/mfib 1.116s
ok ngfw/agent/internal/desired 35.051s

go test -race -count=1 ./internal/subsystems -run TestPim
ok ngfw/agent/internal/subsystems 1.246s

go vet ./internal/renderers/frr/pim ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/subsystems
exit 0 (no output)

tools/ci.sh check --base main
check PASSED (0m04s)
WARN gitleaks not installed — built-in secret grep only
```

A full unit run of internal/subsystems fails TestPBRPolicyNamesFACLList because abf.policy requires missing acl.acl/lan-b. This failure is reported to the manager for independent baseline reproduction; it is not hidden or called green.

Tests cover RP/token rejection, combined interface block, missing/unsafe/duplicate mapping, (S,G)/(*,G), IIF/OIL direction, parse/daemon failure retention, successful withdrawal, removed configuration dependencies, lock-free sync callback, exclusive static/dynamic ownership, FRRDoc deep clone and global vs numbered/no-ID source registration.

The opt-in TestPimdScopedHarness compiles and skips without NGFW_INTEGRATION=1. Live FRR pimd configuration, peer topology, VPP show ip mfib, packet forwarding, and recreation within 30 seconds after simulated VPP loss: NOT RUN (no host VPP/FRR in cloud workspace). The current contract has no PIM VRF field; only an explicitly global-owned product agent syncs table zero. Numbered lab slots cannot modify global table zero. This limitation is intentional fail-closed scope, not claimed host acceptance.

Out of scope: new contracts/API/UI, IPv6/VRF PIM, BIER, host daemon changes, VPP restarts. Local full quick gate attempted: CI GATE FAILED — golangci-lint gitleaks not installed and the download failed. Installer log shows tar could not change ownership to packaged UIDs under container; sudo also failed. No reduced gate substituted or aggregate success claimed. Full hosted quick CI result remains separate from scoped race evidence. See renderer documentation and envelope for reproduction and ownership.

## R5 bounded scale resolution

Runtime snapshots now support at most 256 observed route records. The parser's separate 10,000-record input guard remains intact. A valid 257-record snapshot fails Refresh before cache replacement, preserving every previous route key. This deliberately conservative cap also counts uninstalled observations. MFIB foreign-route ownership and shared-table existence checks are unchanged.

Accepted debt: Codex manager owns safe batched conflict/existence checks and bounded shared-table snapshots, due 2026-10-11; see F-pim-frrsync-debt.md. The cap bounds feature-induced churn, not unrelated table size, and implies no throughput claim.

```text
go test -race -count=1 ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/renderers/frr/pim
ok ngfw/agent/internal/frrsync/pim 1.182s
ok ngfw/agent/internal/descriptors/mfib 1.031s
ok ngfw/agent/internal/renderers/frr/pim 1.045s
go vet ./internal/frrsync/pim
exit 0 (no output)
```

## R8 operator visibility resolution

The source now reports each completed read+sync outcome; its production reporter emits a normal-level structured WARN and existing ERROR event once on a failure transition, with a readable fixed message and safe source/component/status attributes. Repeated failures in the same outage do not spam warnings/events. A successful read and scheduler sync emits recovery and rearms subsequent failure notification. Raw daemon/parser/scheduler payloads are not included. Shutdown cancellation does not trigger an outage.

```text
go test -race -count=1 ./internal/frrsync/pim ./internal/subsystems -run 'Test(Pim|Run|Translate|Failed|Mapping|Supported)'
ok ngfw/agent/internal/frrsync/pim 1.559s
ok ngfw/agent/internal/subsystems 1.461s
go vet ./internal/frrsync/pim ./internal/subsystems
exit 0 (no output)
tools/ci.sh check --base main
check PASSED (0m04s)
```

Tests verify repeated-failure event/log suppression, recovery/new failure visibility, readable messages, no sensitive error-payload disclosure and read/scheduler retry success/failure signals. Baseline aggregate quick CI remains BLOCKED-ENV, distinct from the scoped passing evidence and deferred real lab acceptance.
