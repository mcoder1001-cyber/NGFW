# Independent R1 numeric owner preflight

Source8e1c41d59ef177e81dfec3ed095c317c20b36dd5, tree da4c9d727dde9981c923a49864974d659051d995, remote d6a2ef2df0fc3b40c06bb227ed68fa9b72f51092. Own worktree /root/ngfw-wt/traffic-r1-numeric-20261005; branch codex/traffic-r1-numeric-20261005; 2026-10-05. Reviewer owns reports only and ran no live host/network campaign.

Prior findings are closed for this bounded preflight: real LockOut numeric owner1 now reaches actual api.commit and emits numeric candidate_owner1, stable checked numeric ownership, canonical candidate/running SHA and concrete revision42. Independent positive full-helper HTTP lifecycle passed twice under race detector, with exactly six requests in order: GET config, GET lock, GET candidate, GET lock, POST commit, GET config. Expected state comes from submitted fixture; no readback-derived comparison exclusion. Before/candidate/after documents differ where expected; stored baseline hash equals submitted candidate.

JSON contract audit: LockOut.ownerId is number.int nullable, JSON Go decoding is float64. candidateOwner requires locked=true, HTTP200, positive integral safe integer<=9007199254740991. Both observed locks use that validator and compare numerically. Python proof decoder requires exact int type, rejecting bool/string/fraction/zero/unsafe IDs. Revision is an actual positive integral JSON number. Candidate hashes derive canonical JSON document maps; no fabricated source state. Proof payload numeric1 serializes to JSON1, matching the Python consumer. Initial DHCP baseline and primary commit, strict rollback and actual proof extraction remain mandatory. Marker-only acceptance refuses.

D237 baseline exception audit: only listed exact recognized inactive RootConfig defaults can be exempt; before/candidate values must match exactly, and subsequent warning objects must equal established warning objects. Missing/empty/enabled/custom/future shapes refuse. External AAA/custom TLS/flowprobe are unused in those values; API TLS is not described as disabled. Original unsupported changed DHCP field remains refused.

Commands/output independently observed:

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -race -run '^(TestCommitAcceptance|TestConfigDigestCanonical|TestBaselineWarningGuard|TestInactiveBaselineRequiresRecognizedExplicitDefault|TestCommitNumericOwner.*)$' -count=2 -v
```

Selected actual output:

```text
=== RUN   TestCommitNumericOwner
    commit_guard_test.go:152: TRAFFIC_B_DHCP_REST_PROOF={"baseline_warnings":[],"candidate_owner":1,"candidate_sha256":"f26bf440a00413b02d25edf321d655c616d8c72030101118668657110cfeb3f9","notApplied":[],"revision":42,"status":"applied","txn":"kea-base","warnings":[]}
--- PASS: TestCommitNumericOwner (0.02s)
=== RUN   TestCommitNumericOwner
    commit_guard_test.go:152: TRAFFIC_B_DHCP_REST_PROOF={"baseline_warnings":[],"candidate_owner":1,"candidate_sha256":"f26bf440a00413b02d25edf321d655c616d8c72030101118668657110cfeb3f9","notApplied":[],"revision":42,"status":"applied","txn":"kea-base","warnings":[]}
--- PASS: TestCommitNumericOwner (0.02s)
PASS
ok  	ngfw/test/topology/kea-dhcp-relay	1.179s
```

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -run '^TestCandidateOwnerWireContract$' -count=1 -v
```

Actual output:

```text
=== RUN   TestCandidateOwnerWireContract
=== RUN   TestCandidateOwnerWireContract/numeric
=== RUN   TestCandidateOwnerWireContract/display-name
=== RUN   TestCandidateOwnerWireContract/boolean
=== RUN   TestCandidateOwnerWireContract/fractional
=== RUN   TestCandidateOwnerWireContract/zero
=== RUN   TestCandidateOwnerWireContract/negative
=== RUN   TestCandidateOwnerWireContract/unsafe
=== RUN   TestCandidateOwnerWireContract/missing
=== RUN   TestCandidateOwnerWireContract/null
=== RUN   TestCandidateOwnerWireContract/unlocked
--- PASS: TestCandidateOwnerWireContract (0.02s)
    --- PASS: TestCandidateOwnerWireContract/numeric (0.00s)
    --- PASS: TestCandidateOwnerWireContract/display-name (0.00s)
    --- PASS: TestCandidateOwnerWireContract/boolean (0.00s)
    --- PASS: TestCandidateOwnerWireContract/fractional (0.00s)
    --- PASS: TestCandidateOwnerWireContract/zero (0.00s)
    --- PASS: TestCandidateOwnerWireContract/negative (0.00s)
    --- PASS: TestCandidateOwnerWireContract/unsafe (0.00s)
    --- PASS: TestCandidateOwnerWireContract/missing (0.00s)
    --- PASS: TestCandidateOwnerWireContract/null (0.00s)
    --- PASS: TestCandidateOwnerWireContract/unlocked (0.00s)
PASS
ok  	ngfw/test/topology/kea-dhcp-relay	0.044s
```

Original unsupported changed DHCP warning replay now uses real numeric owner1 and reaches rejecting api.commit twice (review-only Go overlay, source untouched):

```sh
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -overlay /tmp/traffic-r1-numeric-warning-overlay.json -run '^TestReviewCommitUnsupportedWarningAcceptance$' -count=2 -v
```

Actual output:

```text
=== RUN   TestReviewCommitUnsupportedWarningAcceptance
    stack_test.go:529: commit review-unsupported-dhcp: unsupported changed or unrecognized field /services/dhcp/relays/to-kea
--- FAIL: TestReviewCommitUnsupportedWarningAcceptance (0.01s)
=== RUN   TestReviewCommitUnsupportedWarningAcceptance
    stack_test.go:529: commit review-unsupported-dhcp: unsupported changed or unrecognized field /services/dhcp/relays/to-kea
--- FAIL: TestReviewCommitUnsupportedWarningAcceptance (0.00s)
FAIL
exit status 1
FAIL	ngfw/test/topology/kea-dhcp-relay	0.046s
```

The replay intentionally fails because helper t.Fatal refuses the bad response; exit1 is expected negative-test evidence, not a repository regression. Independent repository guard tests above pass. Python `python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py -v` actual result17 tests in .858s, OK.

Previous BLOCK reports remain archived on own branches codex/traffic-correctness-pinned-20261005 at8b1a254de1715d923954b3b1b1e900ccc46b944a and codex/traffic-correctness-repair-20261005 at8cdc9b398e1fe26ae53387afe91de6dc26609650. Their interrupted quick runs stay NOT PASS.

Verdict: **APPROVE — bounded R1 preflight only**. Both specific prior blockers closed. Complete unchanged quick and final immutable all-seven actual REST campaign/independent T3 remain mandatory; no full task acceptance or T1 PASS claimed here. All owned preflight commands finished and source clean before adding reports. No full quick has started on this source, per manager instruction to require actual DHCP preflight first.
