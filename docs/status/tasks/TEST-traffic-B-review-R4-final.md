# Independent final R4 traffic review

Frozen3c06cbdb1. Own branch codex/traffic-final-r4-t3-20261005, isolated worktree /root/ngfw-wt/traffic-final-r4-t3-20261005. Product edits forbidden, owned reviewer/test docs only. Static scope: REST bridge, real-agent attachment, SQL relay, signed fixture licence/tagged WG, consumers, private wrappers, late proof/session cleanup.

No static R4 findings. Generated VPP bindings and production materializers unchanged except separately reviewed minimal BUG warning removal. Existing agent ownership/preflight/Retrieve/secret/licence guards retained. REST attachment requires owner-checked Retrieve, protected peer UID/PID/parent and matching namespaces. Attached fixture agent is never stopped by API stack. DB/role absent checks and partial-create cleanup retained; private ValkeyDB0 never alters host configuration. Private namespace/VPP executable/socket guards refuse shared endpoint. Logical native/routing names/IDs remain private while physical API slot is reserved. Native plugin copies pinned reference sources into scratch, no shared source/plugin writes. No system units, trace, shared restart or management interface operations. Initial actual REST applied candidate and rollback evidence are mandatory; BGP/OSPF exact Retrieve comparisons retained with explicit API defaults.

Actual independent commands/output:

```text
python3 -m unittest discover -s test/topology/traffic-b -v
Ran 15 tests in 1.016s
OK
GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/trafficbtest
? ngfw/agent/internal/trafficbtest [no test files]
```

Actual dispatcher safe negative-evidence probe: mocked driver success markers plus owned scratch REST proof with empty events, no real packet/daemon operation:

```text
START bgp
END bgp failed
VERIFIED actual dispatcher refuses driver PASS when initial actual REST candidate evidence is absent
```

The suite executes owned descendant cleanup after session leader exit, SIGTERM unwind, foreign role refusal, partial-create cleanup, socket/path/namespace identity and unconditional WG rig cleanup. Slot28 env yields12800/16800/table28000; listener12800/12880/16800, namespacew28 and DBw28 checks clean. No live campaign begun, author retains27. No shared VPP/realDB/network mutation. Go compile is not packet proof.

Verdict: **APPROVE**, static R4 on3c06cbdb1, zero findings. Known author composed PSK evidence-file role collision is an actual separate acceptance failure; mandatory independent T3 waits for committed fix/repin and author completion. This approval does not waive packet acceptance or source-delta review.

## Fixed source3e940102 closure

Eight-file committed delta independently inspected: responder/initiator fixed labels and separate r/i DB/runtime variants; numeric runtime reconstruction and os.Root confine diagnostics to owned0700 evidence dirs. grpc.NewClient/Connect retains actual ten-second owner Retrieve before attachment. No API/production agent/resolver/plugin source delta, so prior artifact hashes remain valid. No new VPP/privilege/materializer change.

Actual command: python3 -m unittest discover -s test/topology/traffic-b -v

    Ran 16 tests in 1.176s
    OK

Actual command: GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/trafficbtest

    ? ngfw/agent/internal/trafficbtest [no test files]

Actual dispatcher safe negative probe supplies mocked driver success and valid responder proof but no initiator proof:

    START ipsec
    END ipsec failed
    VERIFIED actual dispatcher requires both responder and initiator REST proof; responder-only driver success fails

Static R4 APPROVE extends to fixed3e940102. Historical3c collision failure remains documented; independent live T3 waits author completion and explicit release.

Actual tagged resolver overlay command, product source unchanged:

    GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test -tags ngfwtestsecrets -overlay=/root/ngfw-wt/traffic-final-r4-t3-20261005/.scratch/r4-w28-overlay.json ./internal/subsystems -run 'Test(R4Slot28TaggedFixture|WireguardFixtureHookOnlyInTaggedFile)' -count=1 -v
    === RUN TestR4Slot28TaggedFixture
    --- PASS: TestR4Slot28TaggedFixture (0.00s)
    === RUN TestWireguardFixtureHookOnlyInTaggedFile
    --- PASS: TestWireguardFixtureHookOnlyInTaggedFile (0.26s)
    PASS
    ok ngfw/agent/internal/subsystems 0.363s

Probe verifies actual tagged resolver loads owned0600 key/w28tb-a test vector and refuses0644; logger discards output, no material logged. Only-tag hook guard passes. Licence statically inspected as owned private signed licence; actual slot28 API acceptance remains live T3 prerequisite, not claimed by resolver test. All owned commands finished; manager will publish checkpoint, then reactivate T3 after author cleanup release.
