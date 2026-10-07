# R5 independent frozen PPPoE review

Verdict: APPROVE bounded lifecycle repair on frozen4bcf9f4977e2a9c248548de23cf552e4bcef94da,
tree7d914836662eb129ada7ac33459b97d4ce449b28. No BLOCKER/MAJOR in assigned scope.
Original leaked-refresher finding resolved by verified bounded shutdown,
generation revocation and serialized action/publication. Private rendered-helper
controls passed with race detection; exact commands/output in review-R1 report.
Direct-child/refresher exits precede replacement/removal. This does not certify
packaged dhcpcd descendant lifecycle or production scale.

MINOR — apps/agent/internal/subsystems/pppoe_watch.go:69 and :86 observe mirrored
sessions twice; observe at :120 rereads state6 after ReadSessionState. For S sessions
with A addresses, ClientMirror per-address dumps can scan roughly O(S*A^2) address
rows each reassertion, plus S interface dumps. Consolidate observation and batch
family dumps when session counts grow. No breaking threshold or throughput failure
was measured; no specific PPPoE scale target is claimed. Watcher uses lifetime
context, one-second ticker and bounded poll; helper subprocesses have two-second
timeouts and paced refresh. No new unbounded cache/history found.
No benchmark or shared-VPP test requested/performed.
