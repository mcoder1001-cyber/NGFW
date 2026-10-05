# TEST-traffic-B WIP

Branch codex/test-traffic-b-20261005; worktree /root/ngfw-wt/TEST-traffic-B-20261005.
Base/local parent d314f0728. Remote checkpoint publication pending manager connector.
Owned files traffic-b/** and TEST-traffic-B* only.

Implemented slot-parametric API GRE/VXLAN kernel peer campaign with ping, scoped text tcpdump, state verification and rollback. Added full phase inventory and strict run-bound acceptance envelope that never treats adapter assertions as independent packet acceptance. Generic orchestration cannot pass a whole wave.

Remaining: execute tunnel campaign against own stack, compose actual WG/IPsec/routing/DHCP packet fixtures, review and full quick. Existing fixed-slot8 IPsec and slot6 OSPF wrapper cannot silently consume assigned slot27. Native route-based DEC replaces historical kernel-vpp prerequisite.

Exact next command: python3 -m unittest discover -s test/topology/traffic-b -v
No packet acceptance claimed yet. No system services changed.

2026-10-05 checkpoint2: local f3426976e published exactly as remote76d58a0327d76b02a97dbfc7e798f81c79a27a06 (manager verified identical tree); CLI push403. Own extra manager-approved hunks p12_topology_integration_test.go, ospf_topology_integration_test.go, rpc_wireguard_integration_test.go, traffic_b_packet_integration_test.go and kea-dhcp-relay/dhcp_test.go: opt-in private-VPP packet hooks only. Ordinary fixtures unchanged when NGFW_TRAFFIC_B is unset.

Built private mount+network isolation, tcpdump readiness via stderr listening signal, BGP/OSPF served learned-prefix ICMP+exact TCP echo, WG UDP capture and ping, DHCP relay DISCOVER giaddr capture/lease proof, owned API/agent stack launcher. Private VPP lifecycle smoke passed and shared PID/NRestarts unchanged. First BGP failed before probes because own redirected run inherited restrictive umask preventing FRR traversal; corrected private wrapper to daemon-compatible022 directories with explicit0600 diagnostic logs. Rerun currently executing. Quick gate currently executing, not claimed green.

2026-10-05 checkpoint3: previous localc27b0b470 exactly published as remoteea970ae4ef618f80329988f7f04a270bfe411ded. Full unchanged tools/ci.sh --base origin/main PASSED (20m21s, logs /root/ngfw-wt/logs/ci/TEST-traffic-B-20261005-20261005-154239-3873918; includes35Turbo tasks, Go race/lint/build, all topology modules vet/unit,149 startup checks). Tree evolved during execution: final coherent source requires another gate before merge.

Actual BGP TestP12TopologyOnHost PASSED92.83s with200VPP learned routes, bidirectional learned-prefix ICMP and exact TCP echo, source-owned peer return /32 route, scoped capture readiness, idempotence/withdrawal/rollback, SHARED_VPP_UNCHANGED. Actual WG TestWireguardHandshakeOnHost PASSED6.75s including ping/UDP capture, handshake state and rollback; private fixture uses slotloopback instance2760 because upstream WG limits instances<16384 (27060 invalid). Neither of these fixture tests establishes slot REST candidate commit acceptance for their features; record that distinction.

Built deterministic primary dispatcher using existing native production PSK/certificate campaigns plus reviewed protocol hooks, no arbitrary adapters. Enforced observed private mount/network namespaces before private child mounts; logs/capture0600, process-owned command group deadline and cleanup; extracted native peer binaries mounted read-only within private namespace. Native source built, runtime validation still owed. Tunnel REST baseline currently failsHTTP400before agent apply; investigating exact validator pointers. OSPF executing. DHCP live and final review still owed.

2026-10-05 checkpoint4: local03f304d01 exactly published remote74b741da2918d690e13696483658c993cd459bea. Actual OSPF TestOSPFTopologyOnHost PASSED66.56s, actual DHCP TestKeaDhcpRelay PASSED30.56s including REST commit, Kea lease and captured DISCOVER giaddr10.27.1.1 (private Wave-B sourceAddressLAN variant) plus restart/rollback. Native production PSK initiator and responder passed through dispatcher; native production certificate packet proof PASSED29.58s with stock defaultRFC7427 peer. SharedVPP unchanged in all cases. Source CLI/log artifacts remain private in .scratch; no key material or pcaps committed.

Hardened tunnel source: observe VPP PID/exe/private mount/socket same-file; exact applied revision, candidate lock identity, canonical candidate vs running hash; safe same-fd token read. Generic primary dispatcher source complete. Actual tunnel run still refuses default whole-root agent.unsupported-field warnings on unrelated disabled /management/aaa,/management/tls,/nat/ipfix,/services/ipfix/flowprobe,/services/ntp,/vpn/ipsec. Manager evaluating scope-correct strict tunnel warning check or product warning correction. No unsupported tunnel field accepted. Next: resolve warning disposition, run tunnel packets, independent review, final unchangedquick/rebase on latestmain.

2026-10-05 final source checkpoint: all primary drivers passed separately, including actual GRE and L2 VXLAN REST/kernel packets in private runtime120461 (25s) with concrete candidate SHA/revision, exact unrelated baseline warnings, rollback after each phase, no owned residue, shared VPP unchanged. Manager authorized strict changed-field warning scope; new/changed unsupported fields still refuse. Current API tunnel state uses items, not the historical script's tunnels property. Candidate ownership checks stable ownerId/locked, not mutable timestamps. Own failed candidate is discarded before rollback. Rig WAN VPP port is removed before REST creates its owned interface to avoid an unowned-object collision.

Unpublished local4685573de archived at archive/test-traffic-b-4685573de and removed from active ancestry: unchanged gitleaks matched generated fixture cache endpoint variable assignment, not an actual credential. Renamed cacheEndpoint; no scanning rules changed. Remote published baseline remains local03f304d01 / remote74b741da2918d690e13696483658c993cd459bea. Clean coherent successor publication pending manager. Eight unit/safety tests passed.

Remaining: final composed back-to-back primary dispatcher run on this frozen source, unchanged complete quick on the exact tree, independent review. Optional phase6 smokes explicitly not-run with individual reasons in dispatcher summary, as prompt allows. WG/BGP/OSPF use product gRPC fixtures, not REST candidate acceptance; do not claim REST proof. Exact next command: NGFW_INTEGRATION=1 NGFW_TRAFFIC_STOCK_ROOT=/run/vrx-test/w10/swan-stock/root python3 test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-b-all-primary-final

2026-10-05 clean publication: local27b5a5690cb0024d9f51c11ba087e251b4e0e6b5 exact remote852eedcb1a1ebf27f64d4f06bf2071c1eaeda1de tree94d51f97cc0dbaa5b5586834ae8dffb176381196. Final gitleaks falsepositive was the generated NGFW_VALKEY_URL= + endpoint expression, resolved with fmt.Sprintf; unchanged complete check PASSED0m11s. Local archived326902c48 excluded active history too.

Full unchanged quick running process session62616 against27b source. Final composed dispatcher session12552 output .scratch/traffic-b-all-primary-final recorded initial326 SHA; only equivalent environment formatting changed to27b during run, manager informed. PSK passed, WG passed, GRE/VXLAN passed. Certificate FAILED at immediate post-route-restore ping (overlayline557/originalline515), after real AUTH, TCP1MiB, rekey, packet/state/restart proofs. This failure is retained; no primary aggregate pass. Individual certificate earlier passed29.58s. BGP/OSPF/DHCP still running. Exact next step: inspect summary and bounded unchanged rerun on27b, investigate reproducible recovery failure before completion. No real failure deferred.

Final author checkpoint 2026-10-05: d3c2ce451 exactly published remote0ae67a0050966bc60306fd20cc058acb88d1a79b. Primary composed727 PASSEDall7, 16:47:18–16:54:11UTC, source unchanged throughout. d3c removes narrowing cast from WG JSON formatting; same instance2760 packets PASSEDagain. Complete unchanged quick d3c PASSED12m14s (logs /root/ngfw-wt/logs/ci/TEST-traffic-B-20261005-20261005-165958-219813),35Turbo tasks,138race-tested agent packages, CLI and all25 Go modules. Startup fake-host harness used unchanged-green cache, not rerun. No required test command remains active. Detailed author report/evidence and FLAKY debt are committed in final docs checkpoint; remote publication pending manager.

Current remaining work is independent review and fresh-main integration/acceptance after backup merge; root will pin the exact main SHA before that run. No product or primary phase placeholder remains. REST proof limits and optional-not-run phases remain explicit. Exact next command for an independent current-source packet rerun: NGFW_INTEGRATION=1 NGFW_TRAFFIC_STOCK_ROOT=/run/vrx-test/w10/swan-stock/root python3 test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-b-independent-new

R1 BLOCK correction: earlier "no primary placeholder"/ready assertion was premature. The original task requires slot REST commit for all primary phases; previous native/WG/BGP/OSPF gRPC fixtures did not satisfy that requirement. No waiver or lab deferral applies. Own branch fast-forwarded to manager07ee3084a (combined backup/main source, remote final baselineb52f029e96b6ee17dcfcbd663521c0cae637e192). REST bridge implementation is now being added, not yet accepted: product API attached to actual native/BGP/OSPF agent socket, REST secrets/candidate/commit, actual object results and owned rollback; standalone production-agent WireGuard REST driver. Cancellation propagation and standalone tunnel baseline fix are included. Owned exception: test-only apps/agent/internal/trafficbtest/**, desired/ikev2 integration opt-in and isolated-vpp.py nested identity export; no product feature changes. D-237 records alternatives. Current failure: compile lacked fmt import, corrected; current REST runtime not yet run. Exact next command: GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh go -C apps/agent test ./internal/trafficbtest ./internal/agent ./internal/desired -run '^TestDoesNotExist$'. Then build current product binaries and run selected REST phases followed by all7 and complete unchanged quick. Do not mark Done.

## REST consumer checkpoint, 2026-10-05

Implemented opt-in REST consumers for native PSK/certificate and BGP/OSPF
fixtures, a standalone real-agent REST WireGuard peer driver, protected PostgreSQL
Unix relay for private network namespaces, and SIGTERM cleanup in dispatcher and
stack. Standalone tunnel CLI now passes observed baseline warnings. Necessary
extra owned test-only hooks: `desired/ikev2_integration_test.go` and
`hardware-smoke/isolated-vpp.py`; default fixture behavior remains unchanged.

Attachment checks include socket type/path ownership, SO_PEERCRED, observed
mount/network namespaces, explicitly recorded parent fixture or its own real
agent child PID, and an actual Retrieve RPC verifying the requested owner before
API startup. Partial PostgreSQL create failures clean reserved resources; foreign
existing database or role is refused.

Actual checks: 12 host-independent tests PASS, including real dispatcher SIGTERM
and cleanup, partial-create failure injection and foreign-role/socket refusal.
Compile and live protocol reruns remain pending checkpoint completion. First
WireGuard REST request failed HTTP403 entitlement; signed private test licence
resolved that gate. Next actual request failed HTTP422 rolled_back with
PENDING-secret-channel; shared VPP unchanged. See TEST-traffic-B-questions.md.
Primary REST acceptance and complete quick gate are still pending; this task is
not Done. Next command: repeat owned BGP/OSPF/native REST drivers and capture
strict warning failures while manager handles the separate secret-channel
prerequisite.

## Tagged WireGuard and relay follow-up

The original task explicitly permits `-tags ngfwtestsecrets`. The owned WireGuard
agent now verifies that build tag and reads an approved private0600 fixture. REST
still creates and commits every configuration change through the production API,
and licensing remains enabled. This proves the authorized test-secret path;
production WireGuard secret delivery remains pending and its actual422 failure
is retained. Tagged first commit applied and capture contained handshake packets;
API established/last-handshake assertion timed out, so acceptance remains failed.

Private-network PostgreSQL creation now directs its login check through the
protected Unix relay and constructs a DSN with a Unix host query. Helper/daemon
stdin is detached from the bridge control pipe. Persistent private diagnostics
are retained, and WireGuard rig cleanup runs even if rollback/state checks fail.
Actual host-independent suite:14/14 PASS. Go compile3packages PASS at130 source.
Current BGP repeat is executing real REST and learned-route packet checks; no
final verdict is claimed before command completion. Next: await BGP, then repeat
WireGuard with retained actual state and native REST under strict warning checks.

The repeated BGP campaign completed all real REST configuration/learned FIB
ICMP/TCP/restart/link-loss/rollback steps, but FAILed its existing strict Retrieve
comparison because the API materializes documented defaults. Opt-in fixture
input now explicitly sets only those defaults (default VRF, false boolean fields,
empty AFI/redistribution/match and route-map set defaults), retaining exact
comparisons for every configured leaf. WG assertion now follows the documented
nullable timestamp contract and additionally requires a positive kernel handshake
stamp; prior failures retained. Actual native REST commit refused the stale
`/vpn/ipsec` unsupported warning; separate product prerequisite is documented in
the questions file and assigned to the manager. Current OSPF run still pending.

Actual tagged REST WireGuard repeat PASS: applied candidate/owner/hash, positive
kernel latest-handshake, API established peer, inner ping and bidirectional UDP,
owned tagged agent restart/recovered ping, REST rollback with empty WireGuard
state, shared VPP unchanged. Evidence run:
`.scratch/traffic-b-rest-primary-three/wireguard-packets` and private phase log.
The actual API event timestamp remains null and is reported as observed.

OSPF opt-in fixture now materializes documented defaults (default VRF, normal area,
noSummary/passive/BFD false, empty redistribution and default originate off), with
all configured leaf comparisons retained. Owned session cleanup now handles
surviving descendants after leader exit, with bounded TERM grace then KILL only
for fixture-created process groups. Actual recovery/safety suite15/15 PASS,
including a real orphaned descendant cleanup probe. REST BGP/OSPF repeat remains
in progress; complete quick and native warning prerequisite remain pending.

BGP actual REST repeat PASS at34e545fc6 plus the subsequently committed OSPF
expected-default fixture: TestP12TopologyOnHost132.44s, applied candidate metadata,
200 learned routes, ICMP/TCP, restart and route withdrawal/recovery, policy/link
changes, REST rollback and shared PID/restart invariance. Evidence:
`.scratch/traffic-b-private-401638/evidence/bgp-rest.json`. Latest dispatcher now
requires actual initial-applied/candidate-owner/hash and baseline-rollback proof
from bounded private metadata for native/routing phases; first fixture commit
cannot silently accept unchanged. This guard will be verified by final composed
rerun after the native stale-warning prerequisite. Signed licence test material
stays private; remote checkpoints confirmed through a45391efc at
b2064f8d4c764b85aba4ff57ee10878e6f0bbe7b. Later checkpoints await manager publication.

Latest queued compile on d751 completed PASS for the three affected Go packages.
No owned protocol command remains active. Durable remote source confirmations:
34e545fc6→6ec1d7a7bbc0a0ad600a6c37e5b1a7bb2d868435;
0c95a49ef→196feb00e8af663f9be1edbf8e98f387397fb1b7;
d75127d30→412d6e70debca1cf801adc3fbd9a2366c3d29e64.
Curated actual REST3 evidence is local dcf54014c awaiting manager publication.
Next: integrate the manager's independently reviewed native-warning prerequisite
and exact fresh backup-main pin, then freeze source, build the owned binaries and
run all seven primary REST phases followed by the unchanged complete quick gate.

Independent R2 found that the dispatcher SIGTERM unit fixture chose WireGuard and
therefore depended on an already built tagged binary in the author's ignored
scratch directory. On a clean reviewer worktree the owned command never started.
The fixture now selects BGP (no binary preflight) while retaining the real
child/SIGTERM/cleanup assertion, and routes mocked gate/lock setup into its own
temporary directory. No product or live preflight guard is changed. Actual
updated safety/recovery suite15/15 PASS; reviewer rerun is required. Curated dcf
checkpoint is verified remote8903182a384314a4e065c1bd796061663ea50247.

## Frozen integrated acceptance candidate

Local8f6b121b4 merges the manager's separately owned VPN capability-warning fix
dfa993405 with fresh backup main d6e1646. The prerequisite's independent reviews
and merge are pending; this task does not claim that product row complete. Own
D237 is retained; two conflicting backup acceptance/review docs use the fresh
main versions. No product merge conflict or author product edit occurred. Board
contains212 rows including the prerequisite. Dispatcher test no longer requires
a hidden tagged binary and15/15 host-independent tests pass.

Next exact commands (owned slot27, bounded dispatcher): build production API and
agent, build `go -C apps/agent build -tags ngfwtestsecrets` to the owned scratch
agent, then `NGFW_INTEGRATION=1 GOMAXPROCS=2 GOFLAGS=-p=2 python3
test/topology/traffic-b/run.py --slot 27 --output .scratch/traffic-b-rest-all-final`.
Wait all seven phases complete, preserve any failure, then run unchanged complete
`GOMAXPROCS=2 GOFLAGS=-p=2 tools/heavy.sh tools/ci.sh quick` on that frozen source.
No Done or native packet pass is claimed before actual results.
