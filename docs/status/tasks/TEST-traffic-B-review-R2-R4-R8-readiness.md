# Independent bounded readiness fixture review

Frozen source43cfd1c9217163745246fb8816399756e70ad71b, exact remote db8d25a37c285789722c6a7da54c7def8612de98. Reviewer isolated branch/worktree codex/traffic-final-r4-t3-20261005, /root/ngfw-wt/traffic-final-r4-t3-20261005. Only reviewer docs changed; reviewer never authored this fixture fix. Five-file delta from8e: three DHCP test files and author recovery/envelope. No independent live T3 started; manager must first complete actual positive DHCP/fullall7 and release after cleanup.

## R2 security

Authenticated request uses existing protected GET /api/v1/state/dhcp/relays, bearer from the existing private API client. No credential/environment representation, token logging, authorization rule, session/secret/privilege boundary or fixture licence/resolver changes. Real controller independently inspected: @Protected, getRunning plus agent.retrieve(['services']) on every request; applied state computed by exact normalized running configuration versus actual retrieved relay. This is live API-agent reachability rather than cached process/health state. Fixed to-kea name must be applied with nonempty config/retrieved. Request context reaches http.NewRequestWithContext, constraining headers/body/request to the shared recovery deadline despite unchanged older client120s maximum. No unbounded new timeout or authenticated mutation in polling. Repeated503 is rejected readiness, not swallowed by rollback. Gitleaks on touched DHCP directory scanned76.75KB no leaks. Prior source security review carries only byte-identical product/API/privileged/private guard code, not an old whole-task acceptance verdict.

R2 verdict: **APPROVE**, bounded source delta, zero findings.

## R4 dataplane/shared host

No VPP API bindings/materializers, process ownership, private namespace/peer/PID guards, relay/ports/licence/secret fixture or daemon/systemd change. Existing proxy/client/Kea recovery assertions remain, now additionally requiring actual authenticated API-agent Retrieve of the restored relay. Original30s recovery budget remains shared; no globals/shared-VPP write, trace or management interface operation added. Actual single strict rollback POST and its hash/applied/warning/empty-state assertions unchanged. No POST retry, ignored failure or relaxed baseline. Independent live packet acceptance must rerun on new source; old source's positive T3 cannot substitute.

R4 verdict: **APPROVE**, bounded source delta, zero findings.

## R8 recovery/tooling

Fixture previously advanced after agent/VPP/Kea process recovery while API gRPC connection could still be unavailable. New fourth condition waits for that exact read-only agent-backed API readiness inside the existing30s context. Context cancellation bounds a blocked HTTP request and200ms polling; unavailable/missing/drift/foreign/malformed/empty responses cannot signal readiness. Timeout still fails existing recovery test; subsequent rollback mutation still runs exactly once. Owned cleanup/stdin/log modes/partialDB protections unchanged. Original repeat rollback503 failures remain recorded as reproducible BLOCK, not FLAKY; fixing the readiness precondition is not an assertion/timeout waiver. Optional earlier diagnostic write-bound MINOR remains outside this narrow delta.

R8 verdict: **APPROVE**, bounded source delta, zero new BLOCKER/MAJOR.

## Actual independent verification

    GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -run 'Test(RelayReadiness|Commit|CandidateOwner|ConfigDigest|Baseline|Inactive)' -count=1 -race -v
    --- PASS: TestCommitAcceptance
    --- PASS: TestConfigDigestCanonical
    --- PASS: TestBaselineWarningGuard
    --- PASS: TestInactiveBaselineRequiresRecognizedExplicitDefault
    --- PASS: TestCommitNumericOwner
    --- PASS: TestCandidateOwnerWireContract
    --- PASS: TestRelayReadinessThroughLiveAPI (0.21s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests (0.16s)
    PASS
    ok ngfw/test/topology/kea-dhcp-relay 1.571s

Real HTTP readiness test asserts GET path/bearer,503 then ready; seven negative subcases unavailable, foreign name, drift, cached config-only, empty retrieval, malformed items and blocked request all respect20ms context deadlines. Existing ten numeric identity wire cases, six partial/unsupported/invalid-commit cases, exact inactive/default/warning guards and whole six-request commit/readback test remain PASS with race enabled. No live DB/agent or host network needed.

    python3 -m unittest discover -s test/topology/traffic-b -v
    Ran 17 tests in 0.953s
    OK
    gitleaks dir --config .github/gitleaks.toml --redact test/topology/kea-dhcp-relay
    scanned ~76752 bytes (76.75 KB) in91.1ms
    no leaks found

Scoped git diff8e..43c for apps/packages/deploy/tools and traffic-b private tooling is empty. Source remains frozen during checks; all owned commands finished, no active fixture/process. No actual positive DHCP/all7 run, full quick or fresh independent T3 claimed here.

Combined bounded verdict: **APPROVE** for starting actual source-pinned validation. Overall acceptance remains blocked until fresh unchanged all7, full mandatory quick, independent T3, final docs/reviews and hosted/main gates pass. Preserve old8e independent T3 PASS and both root aggregate FAILs as distinct historical facts, no debt waiver.
