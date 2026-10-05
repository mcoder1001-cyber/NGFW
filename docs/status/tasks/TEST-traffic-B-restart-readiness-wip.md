# Fresh DHCP readiness repair

Own branch/worktree codex/traffic-restart-readiness-20261005 /root/ngfw-wt/traffic-restart-readiness-20261005. Base8e1c41d59; ownedfiles in envelope. Two observed immediate postrestartrollback503 failures remain reproducible source-fixture blocker; noFLAKYclassification.

Implemented context-bound authenticated GET /api/v1/state/dhcp/relays readiness. Real product controller awaits agent.retrieve(['services']); it is notcachedhealth/config. Predicate requires exact to-kea item applied plus nonempty actual config/retrieved. Only read-only polls (200ms existingfixture cadence) within one shared existing30s recoverydeadline; adds fourth condition after direct VPP proxy/client andKea recovery. HTTP requests use that deadline. Single strict POSTrollback and all existing owner/hash/revision/notApplied/warning/negative guards unchanged. No productionfile edits.

Actual: focused race Go tests (7top-level including wholeHTTPnumericowner/10wiretype/partial/unsupported/default and new unavailable->applied plus7unready/malformed/foreign/drift/blockedrequest negatives) PASS1.554s. New positive uses authenticated GET only, refuses neverready and expires blockedrequest under shared context. Python17PASS and Go vet results finalized below. gitdiffcheckclean. No livepacket/all7/completequick claimed.

Next manager publish coherentcommit, independent R1/R4preflight, then actualDHCPfirst and one frozenfullall7/newsource fullquick followed independentT3 sequential cleanup. Alldeveloperownedcommands finish beforehandoff.

Actual completed focused commands/output:

```text
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C test/topology/kea-dhcp-relay test -race -count=1 -run '^(TestRelayReadiness.*|TestCommitNumericOwner|TestCandidateOwnerWireContract|TestCommitAcceptance|TestBaselineWarningGuard|TestInactiveBaselineRequiresRecognizedExplicitDefault)$' -v ./...
--- PASS: TestCommitAcceptance (0.03s)
--- PASS: TestBaselineWarningGuard (0.00s)
--- PASS: TestInactiveBaselineRequiresRecognizedExplicitDefault (0.01s)
--- PASS: TestCommitNumericOwner (0.01s)
--- PASS: TestCandidateOwnerWireContract (0.05s)
--- PASS: TestRelayReadinessThroughLiveAPI (0.21s)
--- PASS: TestRelayReadinessRefusesUnreadyAndBoundsRequests (0.16s)
(unavailable, foreign-name, drift, cached-config-only, empty-retrieval, malformed, blocked-request all PASS)
PASS
ok ngfw/test/topology/kea-dhcp-relay 1.554s
tools/heavy.sh env GOMAXPROCS=2 GOFLAGS=-p=2 go -C test/topology/kea-dhcp-relay vet ./...
exit0 (no output)
python3 -m unittest discover -s test/topology/traffic-b -p test_scenario.py -v
Ran 17 tests in 1.109s
OK
git diff --check
exit0 (no output)
```

All owned developer commands finished; no livefixture processes/resources were created. Remaining actualDHCP/all7/fullquick/independentreviews are notDone. Managerpublish remoteSHA pending.
