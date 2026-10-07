# R1 independent frozen PPPoE review

Verdict: APPROVE the bounded state/lifecycle repair at
`4bcf9f4977e2a9c248548de23cf552e4bcef94da`, tree
`7d914836662eb129ada7ac33459b97d4ce449b28`. No BLOCKER/MAJOR in this assigned scope.
Full F-pppoe-client acceptance remains unsatisfied; this is not main-merge authority.

Original stale-family findings resolved: ReadSessionState clears withdrawn IPv4
address/peer/DNS before combining families and gates IPv6 on configuration.
TestPppoeIPv6MirrorFollowsHookState executes the rendered IPv4 down hook carrying
old IPCP values, checks fake-FIB withdrawal while IPv6 persists, then restoration,
renumbering, IPv6 down and removal. StaleStateIgnoredWhenOff checks observe and
operator State. Reconnect serializes with transaction exclusion/runtime mutex,
invalidates both families, and has an exclusion regression.

Original publication/cancellation finding resolved: collection is outside the lock,
but generation check/publication share the revocation lock. Action locking
serializes up/down/stop; DHCP events are generation-gated. Bounded pidfd TERM/KILL
and exit verification precede handle deletion/hook replacement. CollectionRevocationBarrier
pauses collection, observes revocation and releases collection. OwnedShutdownAndLateEvent
requires both processes gone and rejects a real late event from the fixture child.
Apply covers removal/off/mode/credentials; natural loss covers hook/link/pppd loss.
These execute rendered helper code with private IP/sysctl/client substitutions.

Setup/client controls cover missing/invalid executable, sysctl failure, failed state
storage and asynchronous child exit. Writable-state errors reach ReadState as
failed with LastError; failed storage returns nonzero. Invalid/foreign process
controls fail without signals to the control process. PartialMirrorFailureIsTrackedAndWithdrawn
verifies compensation tracking after route error, not atomic VPP mirroring.

MINOR — apps/agent/internal/descriptors/pppoe/client.go:117 accepts arbitrary IPv6
prefix lengths/non-unicast addresses at the Mirror boundary. Production ReadIPv6
filters input and HostAddrs emits /128, so no reachable hostile-RA regression is
demonstrated. Harden validation or document its internal caller contract.

Consumer inspection: off/slaac/dhcpv6/defaults, proto field numbers and REST state
shapes unchanged; ipv6 remains a string through API to drawer. Schema/renderer
both reject IPv6 MTU below1280. Stored low-MTU configs can fail validation; the
contract record discloses this. TS execution/regeneration NOTRUN independently.

Actual commands in assigned reviewer worktree, product paths byte-equal freeze:

```text
cd apps/agent
../../tools/heavy.sh timeout 300s go test -race -p 2 -count=1 -timeout 240s ./internal/renderers/pppoe ./internal/descriptors/pppoe ./internal/subsystems -run 'Pppoe|PPPoE|Client|ReadState|ReadIPv6|IPv6|DHCP6|Render|Apply|Secrets'
ok  ngfw/agent/internal/renderers/pppoe 41.846s
ok  ngfw/agent/internal/descriptors/pppoe 1.309s
ok  ngfw/agent/internal/subsystems 3.045s
exit 0
git diff --quiet 4bcf9f4977e2a9c248548de23cf552e4bcef94da HEAD -- apps deploy packages tools test
exit 0
git diff --check
(no output; exit 0)
tools/ci.sh check --base origin/main
ok — contract commit(s) on the branch:
  39abb388f contract(pppoe): IPv6 help matches the client; IPv6 needs MTU >= 1280
ok: gitleaks — scanned ~695169 bytes (695.17 KB) in 805ms no leaks found
board valid: 212 tasks; read-only validation
ok: 30 developer slots + CI slot 12; 964 ports, 32 id ranges, no collision; tools/lab env verified for 31 slots
check PASSED (0m14s)
```

Full unchanged hosted quick is manager-owned by explicit owner assignment; no
local full quick PASS asserted. Real packaged dhcpcd privsep descendants, live
dial/reconnect/rollback/traffic and screenshots remain unverified here. Discovery
and absent client encapsulation are unsupported product functionality, not lab-only
deferrals. No product edits or shared-host changes by this independent reviewer.
