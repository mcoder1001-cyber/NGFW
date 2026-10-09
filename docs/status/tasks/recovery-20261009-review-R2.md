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
