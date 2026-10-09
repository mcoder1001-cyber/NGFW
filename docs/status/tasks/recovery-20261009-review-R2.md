# Recovery closure independent review (R2)

Reviewer branch: `codex/review-recovery-20261009`. Reviewed source: `42b0c993793a497a1597fb36f08729f52a51b2ec` on `codex/pppoe-kernel-carrier-20261008`. Owned file: this report only; no product changes.

Scope: the new scheduler pending-CREATE prerequisite closure, missing-TAP recovery marker/graph, durable namespace ownership declarations and carrier descriptor registration. This is a focused independent security/correctness review, not approval of unrelated carrier changes or native acceptance.

## Findings

No blocker or major finding in this delta from source inspection.

- Prerequisite expansion admits only existing `OpCreate` entries from the validated plan; it does not fabricate an absent optional dependency. It follows provided aliases and observed intermediate dependencies, topologically orders the combined restoration set, and rejects a cycle.
- Early creates use the normal executor/journal path. The done/live check prevents a duplicate later creation, and reverse rollback removes early prerequisites before restoring the previous dependency tree. Existing live prerequisites are traversed without being recreated merely because they are dependencies.
- The repair marker is retrieved-only. Namespace deletion retains the exact observed owner/specification, boot, inode and generation receipt. A replacement namespace gets a new generation before its consumers are restored; foreign or incomplete receipts are rejected.
- Namespace ownership declares the helper's boot-scoped disk ledger as persistent across agent restarts. A volatile provider fails the ownership check. TAP ownership remains the VPP owner tag plus independently verified namespace admission, with a separate descriptor identity from remote-access TAPs.
- This source delta adds no shell command execution, credential output, API endpoint, dependency or broader privilege allowance. Reviewed new tests use synthetic metadata and the sanctioned test-secret literal.

## Verification

Worktree: `/workspace/scratch/9baf7442ffbf/ngfw-review-recovery`, Go 1.26.0, race detector, `GOMAXPROCS=2`, `GOFLAGS=-p=2 -mod=readonly`. Focused commands only; user-requested aggregate CI deferral preserved.

Commands executed from `apps/agent`:

```text
go test -race -count=1 ./internal/scheduler ./internal/descriptors/pppoe
ok  ngfw/agent/internal/scheduler          2.777s
ok  ngfw/agent/internal/descriptors/pppoe  1.103s

go test -race -count=1 ./internal/subsystems -run 'TestCarrier|TestRequirePersistent|TestEveryInterfaceCreatorNamesItsAlias'
ok  ngfw/agent/internal/subsystems        5.510s
```

`git diff HEAD^ HEAD --check` passed. New recovery/ownership and selected carrier controls have no skip path. Existing native opt-in PPP tests in the descriptor package were not enabled and do not count as acceptance. The scheduler suite ran without `-short`.


The carrier recovery graph exercises the actual NamespaceDescriptor and scheduler; TAP transport and daemon operations are recording stand-ins. The subsystem controls exercise actual carrier runtime/registration with fake transport/runner. These are unit-level proofs, not native VPP/kernel/systemd or packet acceptance. No live service or laboratory mutation was performed.

Verdict: **APPROVE** for the specified recovery/ownership delta at `42b0c993`; zero blocker, major or minor findings. Final combined-source gate and native acceptance remain separate requirements.


## Combined integration preservation review

Reviewed final candidate `33db7c632c38e72239e8861bcabf23ac0c45a6aa`, tree `bf2e858290a8789321ba92e30cd944fec23ecd0b`, in independent detached worktree `/workspace/scratch/9baf7442ffbf/ngfw-review-integration`. Source is frozen; this supplement changes only the reviewer report.

The resolved `agent/projection.go` and `desired/interfaces.go` retain both owner-bound `HostServiceSecretOptions(owner)` consumers, unnumbered projection/readback, supervised PPP interface treatment, carrier projection and WAN membership context with the carrier owner argument. No credential/unnumbered hook was lost when integrating PPP.

Reviewed `multiwan.RouteDescriptor` and its product registration: an optional PPP client configuration dependency orders WAN route creation after standalone PPP default/readiness withdrawal. The inverse order removes WAN paths before restoring standalone policy; a system without PPP remains valid. The actual scheduler/client/runtime/WAN descriptor test covers injected route failure and rollback, subsequent join, all-down health without fallback bypass, and leave. Native topology and transport remain stand-ins.

The NCP epoch fence now reads the generation before kernel/VPP observations and compares it again before returning verified forwarding. Product forwarding controls reject drift, in-flight process replacement and same-process NCP replacement. The separate observation-failure source review belongs to the carrier security reviewer; it was not silently folded into this review's scope.

Exact frozen-candidate commands, Go 1.26.0, race, count=1:

```text
go test -race -count=1 ./internal/agent -run 'TestHostCredentialProjectionSelectsOwnerAndRotation|TestUnnumberedDomainApplyRetrieveRevoke|TestWANRoutingOnlyJoinLeaveReprojectsPPPDefaultOwnership'
ok  ngfw/agent/internal/agent       1.234s

go test -race -count=1 ./internal/subsystems -run 'TestCarrierWANTransactionOrdersRoutesAndRollsBackFailure|TestCarrier.*Forward|TestCarrier.*Epoch'
ok  ngfw/agent/internal/subsystems  1.282s
```

Selected controls have no skips. Aggregate/hosted CI remains deferred to the manager's final combined campaign. No native or laboratory acceptance is claimed.

Combined narrow integration verdict: **APPROVE** at `33db7c63`; zero blocker, major or minor findings for the conflict preservation, WAN ordering and NCP fence scope above.
