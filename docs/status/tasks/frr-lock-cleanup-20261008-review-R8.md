# R8 operability and packaging review — FRR lock cleanup

Reviewed local HEAD `88d97a9ca9914d673499b275f7c19752ffe648fe`, rechecked after inspection. Remote publication identity is a manager responsibility; this reviewer did not publish or independently query the remote.

Findings: no BLOCKER or MAJOR in R8 source scope. Failed base reset now retains the acquired cleanup handle, enabling registered cleanup to release the slot lock. Failed nonblocking acquisition closes the new descriptor, while root-harness Stop refuses cleanup without acquired ownership. This protects a holder's files and pathspace from a failed contender. Existing daemon stop order, ownership predicates and bounded termination remain unchanged. New regressions cover reset failure/reacquisition, contention descriptor counts, and holder preservation without starting services.

Actual commands executed:

```text
git diff --check
(no output; exit 0)

go -C apps/agent test -race -count=1 -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors|TestRootFRRCleanupWithoutSlotPreservesHolderFiles' ./internal/renderers/frr/frrtest ./internal/agent
/bin/bash: line 1: go: command not found
(exit 127)
```

No Go test success is claimed. Complete unchanged hosted quick and race regression execution remain required. No service, namespace, package, or daemon operation was executed.

Verdict: **APPROVE** for R8 source scope; required executable checks remain external gates.
