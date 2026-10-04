# TEST-traffic-B — wave-B traffic scenario with FRR/strongSwan peers in netns (review 6.6a, D-125)   (prepend 00-CONTEXT.md)
Start only when: board deps F-ikev2-native, F-pki, F-bfd-redistribution are merged (ready today) AND, for the phases named below,
F-tunnels-host (GRE/VXLAN real-engine driver), F-isis-rip-host, F-det44-cnat-fix, and P11-pkg + P11-host for phase 1a (kernel-vpp
charon; `test/topology/ipsec/run.sh` is NOT IMPLEMENTED until then; P11-pkg is HELD by D-223, an owner action — while held, run 1b
only and record 1a as owed). Runs ONCE in a **manager window** (no other host row, no `tools/ci.sh full`); envelope: slot `w<N>`,
daemon-owner **frr + strongswan + kea** (slot instances only, left stopped); this row is the only root-netns zebra (D-119 M3).
Wave B closes when this row is green (docs/12 S4 gate, docs/11 §5).

## Goal
One scripted run over every wave-B feature back to back, traffic from `ns-w<N>-lan` (or the tunnel's inner netns) with tcpdump
evidence, every phase configured through the product (slot API → agent) and rolled back before the next phase starts.

## Read first
`test/topology/ipsec/README.md` (+ P11-host's driver once merged), `test/topology/wireguard/stack.sh`, `test/topology/kea-dhcp-relay/`,
`test/topology/frr-linuxcp/run-fib-root.sh`, `test/topology/ospf/run.sh` (`NGFW_OSPF_FIB=root`), `test/topology/det44/`,
`docs/status/tasks/S-tunnels-contract-evidence/t1-slot.sh` / F-tunnels-host's driver, `test/topology/nat44-ed-sessions/` (slot stack +
netns traffic helpers), `docs/lab/shared-host-rules.md` §2/§3/§11, `docs/decisions/PENDING-secret-channel.md`, D-167, D-175, D-211.

## Files you own
`test/topology/traffic-b/**` (orchestrator; call the drivers above, never edit them) · `docs/status/tasks/TEST-traffic-B*`.

## Phases (each: API commit → `applied`, `notApplied: []`, no `agent.unsupported-field`; traffic; evidence; rollback)
1. **IPsec**: (a) P11 kernel-vpp charon (`NGFW_STRONGSWAN_PATHSPACE=w<N>`) vs a stock charon peer in netns; (b) F-ikev2-native (VPP
   IKEv2) vs the same peer; one of them authenticates with an F-pki certificate. IKE/NAT-T on the slot ports of the ipsec README, never
   500/4500. PSKs only as `NGFW_TEST_PSK_TEST-traffic-B_*` through the test-build resolver (PENDING-secret-channel). Ping behind→behind.
2. **WireGuard** (stack.sh pattern: tap to a kernel peer netns, `-tags ngfwtestsecrets`): handshake, ping through the tunnel.
3. **GRE + VXLAN**: tunnels via the API (instance set) to kernel `gre`/`vxlan` peers in `ns-w<N>-wan`; ping across each inner subnet.
4. **BGP → FIB, OSPF → FIB**: the root-mode harnesses, one at a time, `flock -x /run/lock/ngfw-globals.lock` inside the shared lab lock.
   They prove `lcp-rt-dynamic` routes but send no packet: the traffic step pings/TCPs from ns-lan to a learned prefix served in
   ns-wan. If the harness tears down before you can send, write a hold-before-cleanup hook request (their tests live in
   `apps/agent/internal/agent/*_topology_integration_test.go`, not yours) to the questions file and paste the FIB-only proof.
5. **DHCP relay**: dhclient in ns-lan leases from the slot Kea (`NGFW_KEA_MODE=test`, ns-wan) through VPP's relay.
6. **Rest of wave B**, one smoke step each: det44/DS-Lite/CNAT (det44 enable only in the globals window, V9; DS-Lite pool deletes
   banned, D-211 — record the pool left), IS-IS adjacency + one route (OSI punt in the window), BFD session up with the FRR peer;
   unbound (`dig` from ns-lan), chrony (`chronyc sources`), syslog (one line received) — or "not run" with the reason.

## Evidence (per phase; full output `TEST-traffic-B-evidence/*.txt`, D-175; no .pcap, no key material — `<redacted>`)
- tcpdump in the rig netns: ESP/UDP-encap only, no plaintext (IPsec); UDP to the WG port only; proto 47 / UDP 4789 around the inner
  ICMP (GRE/VXLAN); relayed DISCOVER with giaddr = the VPP lan address (DHCP); the learned prefix's packets in ns-wan (BGP/OSPF).
- `timeout 10 vppctl`: `show ipsec sa` counters before/after (keys redacted), `show wireguard peer`, `show gre tunnel`, `show vxlan
  tunnel`, `show ip fib <prefix>` (+ load-balance counter), `show dhcp proxy`, `show lcp`; FRR `show ip bgp summary` /
  `show ip ospf neighbor`; `swanctl --list-sas` of the slot charon; `GET /api/v1/state/{ipsec/sas,tunnels}`.
- NRestarts before/after each phase; window start/end; every globals window (start/end, what changed, restored value).

## Cleanup
Rollback after every phase; daemons (charon ×2, FRR, Kea, peers) stopped by PID, units never touched; `tools/lab rig down w<N>`;
residue pasted: no SA/SPD/tunnel/LCP pair of ours (`show ipsec sa`, `show ipsec spd`, `show lcp`), no FRR route in the root kernel
table (`ip route show proto bgp`, `… proto ospf`), `ip netns list | grep w<N>` empty, `/run/ngfw-test/w<N>/` daemon dirs removed.

## Out of scope
Product-code fixes (failure → `docs/status/tasks/TEST-traffic-B-questions.md` with evidence and a proposed fix row; next phase
continues) · strongSwan packaging (P11-pkg) · tri-VM topology (INTEGRATE-E2E) · remote-access VPN (F-ra-vpn) · performance ·
screenshots · schema/proto changes · system daemon units · any VPP global outside a recorded window.

## Rules
Slot prefix everywhere; never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128); lab lock `flock -s` for
the run only, globals only `flock -x /run/lock/ngfw-globals.lock` (D-167), never `flock -x` on the lab lock; every go build/test/vet,
tsc, vitest, vite/pnpm build and pnpm install runs through `tools/heavy.sh` (D-224; never a server or daemon); D-210a: the orchestrator's
self-test passes (`bash -n` + `--dry-run` listing the phases, or go vet + a parser unit test; paste it), no full suite;
`tools/ci-slot.sh --base main` green at the end; commit the driver and docs/status/tasks/TEST-traffic-B.md with real output.
