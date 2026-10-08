# PPPoE lifecycle repair WIP

Branch: codex/pppoe-lifecycle-20261008; base 4d4723f.
Exact local source checkpoint: d08aae29b535671e37e5caeae6d1abc590d80101.
Checkpoint tree: e17623c58d191d9689f18111e4c34ae19fa5a58a. Remote source SHA: none (unpublished). Publication blocked by automatic review: repository payload/destination authorization
and trust/privacy were not established. No remote publication claimed.
Owned files: PPPoE renderer lifecycle.go/lifecycle_test.go, supervisor.go/tests,
templates/hook6.tmpl/ipv6.tmpl, testdata/two-sessions.golden; subsystem
pppoe.go/pppoe_watch_test.go; this envelope/WIP.

Completed source: StopIPv6 persists an admission fence even before PID publication.
Supervision stops old pppd while fenced, replaces/invalidates files, rotates admission
then starts replacement. Durable pending marker survives daemon-reload/restart
failures and triggers identical-config retry until successful restart. Hook captures admission before waiting on action lock;
old token replay after reopen is rejected. Parent process is opened once with pidfd,
passed directly to refresher and polled for original lifetime, never numeric PID
relookup. Regression controls cover late up with/without PID, token replay after
reopen and simulated recycled numeric identity after original parent death.

Actual local verification: git diff --check clean; rendered Python AST syntax passed; tools/ci.sh check --base main PASS
(with missing-gitleaks warning; this is not the hosted quick gate).
Go/gofmt unavailable, so Go controls and golden require hosted verification. Local
process fixture failed at ipv6-parent-unavailable. No successful lifecycle
execution claimed. No host
mutation or real daemon execution performed.

Remaining: full unchanged mandatory hosted quick, independent source/security review,
resolve any review/test findings. Whole feature is not complete: product discovery
and LAN encapsulation remain unsupported; real lifecycle acceptance remains owed.
Next: obtain owner authorization for exact reviewed source PR payload to verified
NGFW repository after automatic approval rejection; then publish and run unchanged
mandatory hosted quick. See pppoe-lifecycle-20261008.md for actual command transcript.
