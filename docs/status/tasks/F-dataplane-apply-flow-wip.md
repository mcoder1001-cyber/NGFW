# Durable checkpoint
Branch: codex/ready-dataplane-apply-20261004; PR #162.
Remote checkpoint before this status: aa73e52122aca992dbecafd170134a96fbb8bdb1.
Source and generated contract complete; reviews igp + wan_pppoe APPROVE.
Tests and scope: F-dataplane-apply-flow.md. Remaining: final unchanged quick output,
expected-head squash merge, main CI, board merged; live appliance restart remains deferred.
Exact next command: tail -40 /root/ngfw-wt/apply-quick-bounded.log

2026-10-04 final source checkpoint: local 97ff17912, remote
 e02d61e8dba0bf5a35875cd34bb305d0215f5f98. Go prerequisite fixes validate
stored VRRP IDs/priorities before narrowing, repair existing lint diagnostics,
and add invalid stored-ID regression coverage. Targeted agent/desired tests PASS;
Go lint reports zero issues. Independent integration reviewer approved these fixes.
Hosted previous quick: 34/35 turbo jobs passed, final web failure was a stale
L2TPv3 warning assertion; corrected without reducing coverage. All six tunnel
page tests now PASS. Latest unchanged complete hosted quick run 37215516046
is pending; no merge authorized by a green gate yet. Local bounded quick log
/root/ngfw-wt/apply-quick-bounded.log still running. Lab acceptance remains deferred.

Additional complete hosted gate 37215793248 passed frontend turbo then revealed
37 existing Go lint diagnostics. Checkpoint local8b8d44490/remote0d8ac5a8 fixes
public API comments, bounded selected WAN weight narrowing and native certificate
snapshot read-file Close error propagation. Independent integration review APPROVE;
all six affected Go package race suites PASS. Full lint and a fresh complete hosted
quick remain mandatory; preceding failed runs are not PASS. Reviewed history remains
in codex/archive/F-dataplane-apply-flow-reviewed-20261004.

Whole-agent race completed: all packages passed except the existing renderer
allowlist documentation test (ripngd was already in the isolated test harness,
but absent from its test-only documented list). The row is corrected at
localad5ae4095/remotef7bbb3a; independent review APPROVE and focused race test PASS.
Complete affected renderers package rerun pending. Full golangci-lint: zero issues.
No production binary allowlist was expanded. Latest full hosted gate must run on
the final single-commit tree; cancelled intermediate runs are not acceptance.

Final consumer generation: local762f55d82/remote6f875d594bd9a3098e82abde9e32127af157da55
regenerates the CLI operations table from final OpenAPI (two new apply endpoints
and nine existing omitted operations); independent generated-contract review APPROVE.
CLI complete lint/race-test/build PASS. All test/ Go modules: formatting, vet and
unit tests PASS. Affected renderers complete race rerun PASS after the test-only
RIPng documentation fix. Fake-host full apply-startup harness is running at
/root/ngfw-wt/apply-startup-full-harness.log. Source is frozen for the final
D112 single-commit complete hosted quick gate; only an actual new gate failure
may reopen source. No intermediate cancelled run is counted as PASS.
