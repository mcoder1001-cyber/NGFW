# Independent R1 restart readiness bounded preflight

Source43cfd1c9217163745246fb8816399756e70ad71b, tree83e9078b796539cc14929e3e1e2bca4c2ba5b3eb, remote db8d25a37c285789722c6a7da54c7def8612de98. Own branchcodex/traffic-r1-readiness-20261005/worktree /root/ngfw-wt/traffic-r1-readiness-20261005. Date2026-10-05. Owned reports only, no product edits or live host/network campaign. Prior8e source full quick31m49s PASS and twice-reproduced packet rollback503 remain preserved; this preflight does not relabel those failures FLAKY.

Verdict: **APPROVE — bounded R1 preflight**. The fixture now establishes the product API agent-readiness condition missing from the two reproduced rollback failures. Final corrected-source complete unchanged quick and actual composed packet/T3 acceptance remain mandatory; no T1 or task-wide PASS yet.

Diff audit: exactly three DHCP fixture Go files change, plus developer envelope/WIP docs. Existing numeric owner/safe-integer and stable observed lock, candidate/running SHA, concrete revision, unchanged-default warning guards, strict rollback/hash and packet/recovery assertions remain unchanged. `dhcp_test.go:728` starts context.WithTimeout30s immediately before the original recovery poll location (after startAgent), then uses remaining context deadline in waitFor. Readiness is a fourth condition after existing real VPP relay/client and Kea recovery. A blocked HTTP request inherits that same context; no additional30s window is introduced. The later single strict rollback POST at line774 remains unchanged. Only read-only GET requests poll, with deadline-bound200ms timers; no config mutation/POST retries/fixed synchronization sleep/timeout extension/guard weakening/product API change.

Actual API source audit: `apps/api/src/features/kea-dhcp-relay/kea-dhcp-relay.controller.ts:224` performs both running datastore read and `this.agent.retrieve(['services'])`; state applied is emitted only when canonical retrieved relay equals configured relay and is enabled. Thus the selected authenticated endpoint requires the actual API-to-agent RPC, rather than datastore-only health or lease metadata. Helper additionally requires exact to-kea name, applied state and nonempty config/retrieved objects; foreign/drift/null/empty/malformed responses refuse. This is static inspection of the product controller, not an actual running NestJS/VPP campaign by this reviewer.

Independent actual HTTP helper tests use httptest loopback responses, not a live product/VPP fixture. Recovery test exercises the real api.callContext/waitRelayReady helper with authenticated GET, first503 then applied200 and exactly two read-only requests. Seven negative cases test unavailable, foreign-name, drift, cached-config-only, empty retrieval, malformed items and blocked request cancellation; each retains its context deadline. Original full-helper numeric positive lifecycle and ten wiretype cases are also rerun. These unit HTTP probes do not substitute for manager actual DHCP preflight.

Command:

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -race -run '^(TestCommitAcceptance|TestConfigDigestCanonical|TestBaselineWarningGuard|TestInactiveBaselineRequiresRecognizedExplicitDefault|TestCommitNumericOwner|TestCandidateOwnerWireContract|TestRelayReadiness.*)$' -count=1 -v
```

Selected actual output:

```text
=== RUN   TestCommitNumericOwner
    commit_guard_test.go:152: TRAFFIC_B_DHCP_REST_PROOF={"baseline_warnings":[],"candidate_owner":1,"candidate_sha256":"f26bf440a00413b02d25edf321d655c616d8c72030101118668657110cfeb3f9","notApplied":[],"revision":42,"status":"applied","txn":"kea-base","warnings":[]}
--- PASS: TestCommitNumericOwner (0.02s)
=== RUN   TestRelayReadinessThroughLiveAPI
--- PASS: TestRelayReadinessThroughLiveAPI (0.21s)
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/unavailable
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/foreign-name
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/drift
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/cached-config-only
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/empty-retrieval
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/malformed
=== RUN   TestRelayReadinessRefusesUnreadyAndBoundsRequests/blocked-request
--- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests (0.16s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/unavailable (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/foreign-name (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/drift (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/cached-config-only (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/empty-retrieval (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/malformed (0.02s)
    --- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests/blocked-request (0.02s)
PASS
ok  	ngfw/test/topology/kea-dhcp-relay	1.559s
```

Eight top-level Go tests passed under race detector in1.559s, including prior guard coverage. Full output /tmp/traffic-r1-readiness-preflight.log.

Original changed-DHCP unsupported warning replay uses actual numeric owner1 and actual complete commit helper with review-only overlay, source unchanged. It remains refused twice, expected negative-probe exit1 (helper t.Fatal); not a repository regression:

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -overlay /tmp/traffic-r1-readiness-warning-overlay.json -run '^TestReviewCommitUnsupportedWarningAcceptance$' -count=2 -v
```

Actual output:

```text
=== RUN   TestReviewCommitUnsupportedWarningAcceptance
    stack_test.go:576: commit review-unsupported-dhcp: unsupported changed or unrecognized field /services/dhcp/relays/to-kea
--- FAIL: TestReviewCommitUnsupportedWarningAcceptance (0.01s)
=== RUN   TestReviewCommitUnsupportedWarningAcceptance
    stack_test.go:576: commit review-unsupported-dhcp: unsupported changed or unrecognized field /services/dhcp/relays/to-kea
--- FAIL: TestReviewCommitUnsupportedWarningAcceptance (0.00s)
FAIL
exit status 1
FAIL	ngfw/test/topology/kea-dhcp-relay	0.035s
```

Python command `python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py`:

```text
.................
----------------------------------------------------------------------
Ran 17 tests in 0.928s

OK
```

All owned preflight commands finished before report edits. Source was clean and frozen during probes. No full quick was started: manager must first pass actual corrected-source DHCP preflight, then release full unchanged quick/all-seven source-pinned tests. Earlier failed/interrupted branches/reports remain intact. This bounded approval does not waive any prior actual failure or complete the task.
