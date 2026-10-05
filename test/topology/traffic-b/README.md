# Wave-B composed traffic campaign

`run.py` sequentially runs the native route-based production IPsec PSK and certificate campaigns, REST WireGuard handshake/packets, REST GRE and L2 VXLAN to kernel peers, BGP/OSPF learned-prefix ICMP and exact TCP echo, and REST DHCP relay to slot Kea with DISCOVER/giaddr and actual lease evidence. Nothing restarts or reconfigures the shared VPP. Phase6 smoke checks remain explicitly not-run with individual reasons; `whole_wave_passed` never claims acceptance of those omissions.

Each primary phase gets a new private VPP and private `/run/vpp`, `/run/netns`, `/run/frr` and `/run/ngfw-test` mounts. FRR executes in a new network namespace. REST stack phases retain host loopback for PostgreSQL but use only slot-prefixed veths, daemons and ports. API stacks use a dedicated no-persistence Valkey at HTTP port+80, DB0; they never change host Valkey configuration or use another slot's database. Fixed logical slot8 IPsec names remain inside an entirely private network/mount namespace with a read-only extracted stock strongSwan root. They do not reserve or alter host slot8 resources.

The private child refuses direct invocation unless it observes a new mount namespace different from both its parent and PID1; a private network phase also verifies a new network namespace. Probes observe the private VPP PID/executable/mount namespace and compare mounted vs physical API socket identities. Diagnostic logs and scoped text captures are0600, evidence directories0700. No arbitrary JSON commands, shell expansion, shared packet trace, system daemon units, package installation, or management interfaces are involved.

## Run

From this worktree after the unchanged complete quick gate has built the API and production agent (the approved WireGuard tagged agent is built separately in owned scratch):

```sh
python3 test/topology/traffic-b/run.py --slot 27 --dry-run
python3 -m unittest discover -s test/topology/traffic-b -v
NGFW_INTEGRATION=1 python3 test/topology/traffic-b/run.py --slot 27 \
  --build-native-plugin --build-wg-test-agent --output "$PWD/.scratch/traffic-b-composed"
```

The native plugin builder copies the pinned reference API source before applying product patches and writes only the requested worktree output. The shared plugin and `/root/vpp` reference are untouched. Set `NGFW_TRAFFIC_STOCK_ROOT` to an already extracted stock peer root if the standard local fixture is unavailable. No peer package is installed as a host service.

Use `--phase bgp`, `--phase wireguard`, `--phase dhcp-relay`, `--phase ipsec`, `--phase ipsec-cert`, `--phase ospf`, or `--phase tunnels` for a bounded rerun; multiple phase flags preserve their specified order. Every output directory must be new. A selected subset can pass its checks but cannot report `primary_passed: true`. Any nonzero driver result, skipped integration test, missing executed acceptance marker or changed shared VPP identity fails that phase. Failure diagnostics stay private and the dispatcher continues independent phases after the owned wrapper cleans up.

## Evidence and limits

Historical pre-REST implementation: native IPsec uses the production agent and sealed secret delivery. Its unchanged packet drivers prove bidirectional ICMP/TCP, ESP/no plaintext, native rekey, route withdrawal/recovery, agent restart and owned rollback; the certificate peer uses stock default RFC7427 signatures. WireGuard/BGP/OSPF use existing product agent integration fixtures through gRPC with opt-in packet hooks; their REST candidate/commit acceptance is not established by those fixtures. DHCP and tunnels configure through the product REST API. Do not describe those historical runs as REST proof. The later REST bridge implementation and historical frozen3e campaign in [the task history](../../../docs/status/tasks/TEST-traffic-B-pre-readiness-history.md) supersede that wiring; the source-specific3e result was blocked on its independently identified DHCP warning/notApplied guard gap. Corrected43c adds strict ownership/default-warning guards and real API-agent readiness within the original recovery deadline; ROOT and independent T3 each executed all seven successfully. See docs/status/tasks/TEST-traffic-B.md for current-source evidence and remaining final CI/review gates.

BGP/OSPF packet probes assign a served /32 within an actually learned prefix on the WAN peer, add a peer-only return route to the LAN source, verify that exact learned FIB entry before/after, capture requests and replies inside WAN, and exchange an exact TCP payload. The peer return route supplies a fixture prerequisite; it does not install a VPP destination route. DHCP configures the relay source to the LAN address, captures DISCOVER with that exact giaddr, and obtains a lease with a foreground client using `/bin/true` as its script, followed by the existing lease/state/restart/rollback tests. Packet probes never use system resolv.conf.

Tunnel source locks the candidate and records its ownership, compares the canonical candidate hash with the committed running document and records the actual revision. It sends ICMP through kernel GRE/VXLAN peers, records scoped encapsulation text plus API/CLI state, rolls back to its concrete owned baseline after each phase, and refuses tunnel or namespace residue. A pristine API datastore gets an owned harmless VRF baseline rather than a fabricated revision0/null. Unrelated baseline warnings are recorded verbatim; unsupported changed tunnel/interface/routing-l2 fields or new warnings fail acceptance.

Raw upstream native captures and private peer diagnostics remain in ignored `.scratch`, never committed. Share only reviewed redacted text/JSON summaries. `scenario.accept` is an evidence envelope validator and does not turn arbitrary adapter assertions into packet acceptance; the live dispatcher calls only repository-owned built-in campaigns.

Every primary configuration now enters its owned API candidate and commit path.
The native and routing fixtures attach that API to their real agent only after
an actual owner-checked Retrieve and observed socket peer UID/PID, protected path,
expected fixture-parent or explicitly recorded owned agent-child relationship,
and matching private mount/network namespaces. Private-network stacks access
host PostgreSQL only through a root-only Unix relay; they reserve an absent
slot database/role, clean partial creation on failure, and never reuse foreign
resources. Helper/daemon stdin is detached from the REST control pipe.

Each owned API trusts only its own short-lived signed test licence. WireGuard
uses the task-authorized `ngfwtestsecrets` build and an owned0600 fixture; REST
still commits its configuration through real guards. This does not assert that
production WireGuard secret delivery is implemented. The production
HTTP422 remains historical evidence of the unimplemented production secret channel.
The native unsupported-warning prerequisite landed in separately reviewed PR192;
its exact hosted/main quick passed. The questions file preserves both histories. Actual nullable WireGuard
agent timestamps are retained; handshake acceptance also checks the kernel's
positive timestamp, inner ping and captured UDP traffic. TERM propagates bounded
cleanup through fixture-created process sessions and always tears down the owned
WireGuard rig even if rollback or API state checking fails.
