# PPPoE lifecycle repair WIP

## Published source checkpoint — supersedes earlier publication blocker

Owner explicitly authorized publication of this reviewed payload on 2026-10-08.
GitHub connector publication succeeded. Reviewed local e8fdb673 maps to remote
040e3535e8f4aad5e3f005bc7c78b759fc7632a6 with identical tree
98e78164a1359c37bb07789b1c2e5234d95da60a. All thirteen coherent checkpoint trees
were reproduced in the remote history; archive branch
codex/checkpoint-pppoe-reviewed-20261008 preserves that reviewed checkpoint.

Actual main417e8fcd0c82fae7906da0fba1c0622a8de017f7 integrated without conflicts;
local integration551931768a00b797659e56e281346e4ec4bfacc2 equals remote
bfa15151373472489b8a45ee99e4bbf1bfce4c12 at tree
583e01bdd7df86e9d70644d474eb106a749a5cf4. Upstream delta is ten TD19 paths;
PPPoE reviewed source unchanged. Draft PR210:
https://github.com/mcoder1001-cyber/NGFW/pull/210 . Hosted unchanged CI gate
37812633992 is in progress; not passed or merge-ready. No merge performed.

Next: verify unchanged complete hosted quick; repair actual failures and obtain
independent review of any executable delta. Root handles final integration/merge.
The earlier publication rejection below remains historical, resolved by explicit
owner authorization rather than a connector bypass.

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
