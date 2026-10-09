# PPPoE lifecycle repair — R5 performance and scale

Reviewed source: `18889fed1dc410fdb77679585b4c5a2bbcd2c31c` in
`/workspace/scratch/9baf7442ffbf/ngfw-pppoe`, against `main`.
Includes `37de7f5` pending-removal recovery and `7debd6a` late-parent-hook fix.
Source review only; no benchmark, throughput claim, real daemon, VPP, or host
mutation. No product source or git commits written by this reviewer.

## Findings

### Resolved MAJOR — historical host-interface tombstones grew without bound

Locations: `apps/agent/internal/renderers/pppoe/lifecycle.go:33,55-57,72`,
`apps/agent/internal/renderers/pppoe/supervisor.go:170-194`.

Removal intentionally leaves `.ipv6.blocked` and `.ipv6.admission` outside
`sessionFiles`; `CompleteIPv6Transition` removes only `.ipv6.pending`.
For each distinct removed host interface, at least the new blocked/admission
artifacts remain indefinitely. Repeated transitions of the same name overwrite
fixed files, but churn through distinct valid names has no retention bound.
The new pending recovery inventory uses `os.ReadDir(StateDir)`, materializing
and sorting the entire directory on every Apply, including these tombstones.
Thus disk/inodes and reconciliation scan memory/work grow with historical host
names, rather than active sessions or pending transitions. At 100,000 removed
names there are at least 200,000 new retained directory entries even with zero
active sessions; this is a cardinality example, not measured latency or a
claimed failure threshold. The source offers no history limit or reclamation
protocol.

Fix: provide safe bounded retirement after old writers/hooks cannot act, or
separate pending inventory from retained fencing records and bound the latter
with a safety-preserving protocol. Do not blindly remove fences or TTL them:
that would reopen the admission race this repair addresses. Alternatively,
the manager may explicitly accept this narrow retention limitation in
`docs/tech-debt.md` with an owner and date, as permitted by the review rules.

## Positive source observations and limits

- New per-refresh parent liveness is a constant-size zero-timeout pidfd select,
  replacing numeric-PID `/proc` lookup. No new subprocess or all-session scan
  is added to the approximately two-second refresh iteration. Existing two
  `ip` observations per session/tick remain, each with a two-second timeout.
- The parent pidfd is captured once, inherited only by the refresher, and closed
  by both launcher and refresher on their respective cleanup paths. Stop's
  verified pidfds and DHCP child pidfd have finally-path closure. No new
  persistent goroutine or growing per-tick map/list is evident.
- Existing lock retries sleep 20 ms and have an eight-second deadline; TERM/KILL
  polls have two-second limits each; Go StopIPv6 has a 20-second command bound.
  Cleanup is bounded per session, not an overall constant-time bulk guarantee.
- Changed-session sorting adds O(n log n) work to Apply, not to the refresh
  tick. Pending stat and fixed file operations are O(n). The removed nested
  restart-inference scans improve source complexity. Durable pending evidence
  reuses one pathname per host and is cleared after successful transition.
- Startup/failure retry, full quick CI, real lifecycle acceptance, and large
  session scale remain outside this source-only review. No benchmark requested
  under FAST MODE's prohibition on performance work.

Actual read-only check executed in the source worktree:

```text
$ git diff --check main...HEAD
[no output; exit 0]
$ git rev-parse HEAD
f7aadb6ab7357a73ef259623d7f0e2a2e01f1803
```

## Verification round — 18889fed

The final change resolves the new retention/scan growth: pending transitions
now live in a dedicated `ipv6-transitions` directory. `installedHostIfs` scans
that directory rather than all state files. Removed-session admission/fence
files are deleted only after the shutdown/removal sequence and successful
`daemon-reload`; the pending entry remains until retirement succeeds, allowing
cleanup failure to retry. Successful removal deletes its pending entry too.
Recreation generates a fresh random token. Fixed host-path artifacts are
therefore bounded by active/incomplete transitions rather than all historical
removed hosts. Preexisting lock artifacts are unchanged and are not enumerated
by the new pending inventory.

The changed regression test asserts the removed admission/fence and pending
paths are absent after recovery. This assertion was read, not executed here.
Read-only `git diff --check main...HEAD` again returned exit 0 without output;
`git rev-parse HEAD` returned
`18889fed1dc410fdb77679585b4c5a2bbcd2c31c`. Other timed-loop/process-handle
observations above remain applicable. This is an R5 resource review, not R2/R4
race acceptance or evidence of real lifecycle execution.

Verdict: **APPROVE** — 0 open BLOCKER, 0 open MAJOR, 0 MINOR. The previous
MAJOR is resolved at the exact final SHA. Applies only to R5; hosted quick,
other reviewer approvals, and real acceptance remain separate.
