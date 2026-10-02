# Multi-WAN R3 correction verification

Original BLOCK report `multiwan-monitor-resume-review-R3.md` / commit `bf28e2c2` remains historical evidence. This verification supersedes that verdict for its single MAJOR finding only.

Reviewed correction `29e5e2deac10a59d7709b792db4812511ccc626d` in checkpoint `d746440fb1c7736620ab3a38cf469748f8054700`, tree `acfb223ee4772c301295979222e2c402f1924196`, based on main `c76774e8`. Independent worktree `/workspace/scratch/de92de7d9874/NGFW-wan-r3-verify`; branch `review/wan-monitor-r3-verify-20261002`. No product edits.

The runtime now maintains `memberSamples.up` and `memberSamples.since` under the same lock as monitor samples. After an observation, it recomputes the member's AND health and changes `since` only when that aggregate health changes. Snapshots directly export this member timestamp. Unobserved/never-up members retain nil; partial monitor changes while aggregate health remains down preserve the timestamp. Identical config preserves observations; a new identity replaces them. No protobuf/schema/API shape changed.

Independently ran in `apps/agent` with pinned Go, `GOTOOLCHAIN=local`, read-only existing module cache, writable `/tmp/dashboard-r2-gocache`, `GOFLAGS='-p=2 -buildvcs=false'`:

```text
go test -race -count=1 ./internal/multiwan -run '^TestRuntimeSinceTracksAggregateMemberTransitions$'
ok ngfw/agent/internal/multiwan 1.424s
```

Inspected the test assertions: initial partial success leaves nil, aggregate recovery creates a timestamp, aggregate down advances it, subsequent partial failure/recovery preserves it, same configuration preserves it, replacement identity resets it. The test observes actual asynchronous probe output before each assertion. `git diff c76774e8..d746440f --name-only -- packages/schema packages/proto apps/agent/gen packages/api-client` produced no output.

Also inspected the adjacent watcher change to a committed immutable `wanSaved` snapshot and captured generation-specific device mapping; it does not reshape the public RPC. R1 owns verification of persistence failure, restart and rollback behavior. Existing monitor-only limitations and empty `Active` remain accurate. No full quick or lab acceptance claim.

**Verdict: APPROVE.** R3 MAJOR member timestamp finding is resolved on the identified checkpoint. Hosted quick, other reviewer gates and any further product delta remain separate integration requirements.
