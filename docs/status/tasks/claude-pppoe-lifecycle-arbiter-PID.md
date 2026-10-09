# PPPoE PID/proc consistency — independent A2 ruling

Reviewed source: local `a673d340`, published PR210 `a939827119cf6645f77fa9555296e6a070df551b`, equal tree `9f1c79084ba9edc9bb8be14ac468e128c520c984`.

Independent private-child controls twice found that a process PID differs from its procfs self-directory identity. Numeric procfs child paths were either absent or belonged to an unrelated process, while owned pidfds correctly observed alive/dead transitions. Private namespace repair was refused with `Operation not permitted`; no host or production files were changed and no unrelated process was signalled.

The unchanged `TestIPv6PinnedParentRejectsRecycledNumericIdentity` was independently repeated with `-count=2`. Both runs FAIL (0.20s and 0.17s), including the retained-writer assertion and owned-shutdown cleanup. The test's numeric-proc observer can declare a still-live owned writer gone in this inconsistent environment, so the resulting lifecycle observation is invalid. This establishes environment contamination; it neither proves product success nor dismisses all possible source defects.

Command: `go -C apps/agent test -race -count=2 -v -run '^TestIPv6PinnedParentRejectsRecycledNumericIdentity$' ./internal/renderers/pppoe` (trusted Go1.26, bounded build concurrency and read-only modules).

```text
FAIL TestIPv6PinnedParentRejectsRecycledNumericIdentity
recycled numeric identity retained writer: phase:"up"
fixture cleanup: pppoe: owned IPv6 shutdown failed
FAIL (both repeated runs)
```

Ruling: procfs-dependent ownership and lifecycle acceptance is unresolved. Preserve actual failures. Re-run unchanged focused tests twice on an isolated Linux runner with aligned procfs/PID namespace. No ownership preflight or assertion may be weakened. Per owner instruction, complete CI waits until all task work is complete and runs once on the final combination.
