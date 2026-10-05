# Independent fixed DHCP review checkpoint

Source eb1a634da6858e3c6599e9586044b5030309f39b, own isolated branch/worktree codex/traffic-final-r4-t3-20261005, /root/ngfw-wt/traffic-final-r4-t3-20261005. Product source unchanged by reviewer. Independent live T3 remains held pending author campaign cleanup and explicit manager release.

R4/R8 static inspection: DHCP acceptance now requires applied concrete revision, empty notApplied, no unsupported result outside the narrow baseline inventory, candidate ownership/hash persistence and baseline rollback. Baseline exclusions match exact inactive schema defaults and unchanged values plus exact observed warning membership; changed DHCP/interface, enabled/customized/missing/unknown defaults and novel warnings refuse. Dispatcher requires two concrete commit markers and rollback, not a driver PASS marker alone. No VPP binding, production materializer, privileged helper, ownership/process cleanup, host services or shared endpoint change. Prior tagged fixture/relay/session safeguards retained.

**[other: R1] BLOCKER — actual numeric lock owner rejected**, test/topology/kea-dhcp-relay/stack_test.go commit. New code asserts ownerId is a string. Actual API LockOut uses z.number().int(), datastore lock types use number and datastore tests report ownerId1. A correct actual lock decodes float64 in this Go JSON client and the helper immediately fails candidate ownership absent before committing. Fix acceptance to validate the actual numeric contract, positive integral identity and unchanged repeated lock identity; retain strict candidate proof. The tests below exercise actual commit with real HTTP response twice; broader guard unit tests do not reach this path.

Actual independent checks:

    python3 -m unittest discover -s test/topology/traffic-b -v
    Ran 17 tests in 1.308s
    OK

    GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -run 'Test(CommitAcceptance|ConfigDigestCanonical|BaselineWarningGuard|InactiveBaselineRequiresRecognizedExplicitDefault)$' -count=1 -v
    --- PASS: TestCommitAcceptance (six actual HTTP cases)
    --- PASS: TestConfigDigestCanonical
    --- PASS: TestBaselineWarningGuard
    --- PASS: TestInactiveBaselineRequiresRecognizedExplicitDefault (six baseline defaults)
    PASS
    ok ngfw/test/topology/kea-dhcp-relay 0.041s

Independent retained overlay probe .scratch/r4-dhcp-owner-probe_test.go maps only a virtual test into the package, not product code. Real httptest returns the actual API lock contract locked:true, owner:admin, ownerId:1, ownerKeyId:null. It calls actual api.commit; no real database, daemon or privileged operation.

    GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C test/topology/kea-dhcp-relay test -overlay=/root/ngfw-wt/traffic-final-r4-t3-20261005/.scratch/r4-dhcp-owner-overlay.json -run '^TestR4RealNumericLockOwner$' -count=2 -v
    === RUN TestR4RealNumericLockOwner
    r4_owner_probe_test.go:5: candidate ownership absent
    --- FAIL: TestR4RealNumericLockOwner (0.01s)
    === RUN TestR4RealNumericLockOwner
    r4_owner_probe_test.go:5: candidate ownership absent
    --- FAIL: TestR4RealNumericLockOwner (0.00s)
    FAIL
    exit status 1
    FAIL ngfw/test/topology/kea-dhcp-relay 0.031s

Original3c native collision FAIL,3e weak DHCP historical result and cancelled8f no-PASS evidence are preserved. No source acceptance on eb is inferred from guard-unit PASS. Parent notified immediately. All owned commands finished, no live T3/VPP/network/realDB operation occurred.

Verdict: R4/R8 static boundaries have no new graded finding, but **task acceptance BLOCKED by other:R1 numeric-owner failure**, reproducible twice. Mandatory actual all7 independent T3 cannot claim PASS on this checkpoint. Focused corrected-source recheck required; no waiver from conditional FINISH ruling.
