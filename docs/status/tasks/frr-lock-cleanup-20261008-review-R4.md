# FRR lock cleanup — R4 review

Reviewed source: `a24e9dadc2a37c35191290cb41697f2167fe67ee`.

## Findings

**BLOCKER — cleanup without acquired ownership.** `apps/agent/internal/agent/ospf_topology_integration_test.go:99–113`: `startRootFRR` registers destructive `Stop` cleanup before acquiring the slot lock. When the nonblocking lock fails, `t.Fatalf` invokes that cleanup with no ownership. `Stop` reads the existing holder's pidfiles and can signal its daemons, remove its matching symlink, and remove its base directory. Closing the failed lock descriptor fixes the leak but leaves the contention failure unsafe. Register cleanup only after successful acquisition, and/or require acquired ownership in `Stop` before any destructive action. Add an offline regression using private temporary paths proving a rejected contender leaves the owner's files and symlink intact.

The harness change correctly returns the acquired cleanup handle after failed base removal; its regression uses temporary directories and does not create namespaces or daemons. The new root lock helper closes rejected descriptors, but its regression exercises acquisition only, not startup cleanup.

## Verification

Attempted from `apps/agent`:

```text
go test -race -count=1 ./internal/renderers/frr/frrtest ./internal/agent -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors'
/bin/bash: line 1: go: command not found
```

No test pass is claimed; independent source review found the blocker above. No host integration was attempted.

## Ownership fix recheck

Rechecked local commit `88d97a9ca9914d673499b275f7c19752ffe648fe`, tree `e906397df156dc44da74525b3657cbae50b9beb9`. `rootFRR.Stop` now returns before all daemon, symlink and directory operations when no slot lock was acquired. This closes the contention cleanup blocker and makes repeat cleanup non-destructive after release. `TestRootFRRCleanupWithoutSlotPreservesHolderFiles` holds a temporary slot lock, rejects a contender, calls unowned cleanup, and verifies holder marker and matching symlink preservation. No real daemon or namespace is used by this regression.

There are no remaining R4 source findings. The verification limitation above remains: compiled/race tests and mandatory unchanged quick gate require separate successful evidence before merge.

Verdict: **APPROVE** (source ownership and shared-host safety; no execution pass claimed).
