# PPPoE lifecycle repair WIP

Branch: codex/pppoe-lifecycle-20261008; base 4d4723f.
Current source checkpoint is resolved by git rev-parse HEAD. Publication pending.
Owned files: PPPoE renderer lifecycle.go/lifecycle_test.go, supervisor.go/tests,
templates/hook6.tmpl/ipv6.tmpl, testdata/two-sessions.golden; subsystem
pppoe.go/pppoe_watch_test.go; this envelope/WIP.

Completed source: StopIPv6 persists an admission fence even before PID publication.
Supervision stops old pppd while fenced, replaces/invalidates files, rotates admission
then starts replacement. Hook captures admission before waiting on action lock;
old token replay after reopen is rejected. Parent process is opened once with pidfd,
passed directly to refresher and polled for original lifetime, never numeric PID
relookup. Regression controls cover late up with/without PID, token replay after
reopen and simulated recycled numeric identity after original parent death.

Actual local verification: git diff --check clean; rendered Python AST syntax passed.
Go/gofmt unavailable, so Go controls and golden require hosted verification. Local
process fixture failed at ipv6-parent-unavailable. No successful lifecycle
execution claimed. No host
mutation or real daemon execution performed.

Remaining: full unchanged mandatory hosted quick, independent source/security review,
resolve any review/test findings. Whole feature is not complete: product discovery
and LAN encapsulation remain unsupported; real lifecycle acceptance remains owed.
Next: publish small source repair PR and run unchanged mandatory hosted quick.
