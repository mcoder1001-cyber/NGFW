# Wave-C product and packet acceptance

`execute.py` implements the leased live transaction. Offline checks never grant a
window or claim packet acceptance:

```sh
python3 test/topology/traffic-c/execute.py --slot 14 --dry-run
python3 -m unittest discover -s test/topology/traffic-c -v
tools/heavy.sh go -C test/topology/traffic-c/globals vet ./...
tools/heavy.sh go -C test/topology/traffic-c/globals test -count=1 ./...
tools/heavy.sh go -C test/topology/traffic-c/globals build -o traffic-c-globals .
```

The manager must provision a dedicated slot API/agent/datastore, admin API key in
`NGFW_HOST_ACCESS_TOKEN`, a committed empty configuration, the canonical
`tools/lab env N` environment, and a global-owner agent for the quiet window.
No existing slot namespaces/interfaces may exist. The manager creates
`/run/ngfw-test/wN/traffic-c` root-owned0700 and
`/run/ngfw-test/wN/traffic-c-lease.json` root-owned0600, unaliased regular file:

```json
{
  "task": "TEST-traffic-C", "slot": 14, "prefix": "w14",
  "boot_id": "<actual current boot UUID>", "expires_unix": 0,
  "lease_id": "<32 lowercase hex characters>",
  "daemon_owner": "keepalived", "globals_owner": true, "quiet": true
}
```

The expiry must be in the future within two hours. The manager must first verify
an otherwise idle VPP and exclusive keepalived ownership. The example is invalid
until actual identities/expiry are supplied; the driver never creates its lease.
Do not start a global-owner agent before its manager window.

```sh
NGFW_INTEGRATION=1 NGFW_TRAFFIC_C_HOST=1 \
  python3 test/topology/traffic-c/execute.py --slot 14
```

One VPP is used; its VRRP peer is keepalived in an owned namespace, bridged inside
the owned LAN namespace. Failover/return use real product commits and timestamped
ping/ARP evidence. The driver uses labelled SR-MPLS steering plus a next-hop
label route: the current static-route schema has no direct `outLabels` field.

Each stage commits via the API, rejects partial apply/unsupported leaves/drift,
collects real WAN packet text and relevant CLI/counter state, then rolls back to
the rig baseline. QoS compares actual sent UDP count, WAN tcpdump count, socket
received count, and all three policer counter increases. Riders use a live IPFIX
collector, IGMPv3 INCLUDE peer, product capture download decoded with tcpdump,
and owned-interface Prometheus counter deltas. TD-H18 runs its existing actual
host test and rejects skipped proof. No packet trace or VPP restart is used. All authenticated API/download requests reject redirects and ignore proxy environment. TERM enters finally cleanup; every spawned group receives final SIGKILL after the graceful accounting interval, even when its leader exited.

The globals helper saves exporter0, flowprobe parameters, SR source/hop limit
and table0 readback. It restores exact values through generated binary API and
requires the runner's inherited exclusive globals lock. The lab lock stays
shared. The helper refuses preexisting flowprobe interface bindings.

**Current product constraint:** a globals-owner slot agent cannot adopt an
existing foreign MPLS table0. This executor refuses any preexisting table0
before creating the rig rather than changing its owner or removing it. Therefore
this combined run requires an otherwise idle VPP where table0 is absent at
entry (for example a manager-provisioned private VPP). Before/after table0 is then
identically absent. Existing-table0 shared-host acceptance remains explicit in
the task questions; no live shared host run has been performed.

Evidence stays private under the allocated run directory. Packet captures must
never be committed. Cleanup stops only freshly spawned process groups, rolls
back the original product revision, lowers the peers before `tools/lab rig down`,
restores globals, and checks namespace/interface/feature residue. A failed
restoration retains evidence and never emits PASS. Original expiry/revocation
checks remain active; an expired lease can require manager recovery of a down
rig. The importable `driver.py` contains parser/transaction primitives; its old
entry point remains an explicitly offline foundation plan.
