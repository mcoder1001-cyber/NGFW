# TEST-traffic-A — wave-A traffic scenario on the af_packet rig (review 6.6a, D-125)   (prepend 00-CONTEXT.md)
Start: all 12 deps merged (board 2026-10-01). Runs ONCE in a **manager window**: no other host row and no `tools/ci.sh full` on the
shared VPP; the envelope names the window and your slot `w<N>`. Wave A closes when this row is green (docs/12 S4 gate, docs/11 §5).

## Goal
One scripted scenario: real ping + TCP from `ns-w<N>-lan` to `ns-w<N>-wan` through every wave-A packet feature at once, configured
through the product (slot API → agent → VPP), tcpdump evidence: VLAN sub-if → bridge + BVI → VRF static/ECMP (+ static neighbour)
→ uRPF + PBR (ABF) → ACL on an object-model group → NAT44-ED, then the same path with NAT44-EI.

## Read first
`tools/lab` (rig up|down|gc), `docs/lab/shared-host-rules.md` §2/§11/§12, `test/topology/nat44-ed-sessions/` (stack_test.go + rig_test.go:
slot stack on a throwaway DB via `deploy/dev/pg-test.sh`, peers down until the V19 preflight; traffic_test.go: python hold client/server +
tcpdump in a netns), `test/topology/nat44-ei-64-66-nptv6/` (ED/EI exclusive: slot nat44 lock, globals lock shared, `edEnabled`/`ensureEI`),
`test/topology/{vlan-qinq,bridge-l2,vrf-static-ecmp,acl}/`, `docs/user/` pages, `docs/status/tasks/F-rpf-adl-pbr.md` (uRPF/ABF never had a packet).

## Files you own
`test/topology/traffic-a/**` (own module or bash; copy helpers, never import another topology module) · `docs/status/tasks/TEST-traffic-A*`.

## Topology (reference; keep the shape, adapt names — every object carries `w<N>`, tables `<N>0xx`, addresses in 10.<N>.0.0/16)
- lan: `ns-w<N>-lan` `w<N>l1.100` 10.<N>.10.2/24 (+ 10.<N>.9.9/32, spoof probe) ─ `host-w<N>l0.100` (dot1q 100) in bridge domain
  `w<N>-bd` with BVI `loop<N>10` 10.<N>.10.1/24 in VRF `w<N>-ta` (table <N>010); uRPF strict rx, ABF and ACL `in` on `loop<N>10`.
- wan: `host-w<N>w0.201` 10.<N>.21.1/24 and `.202` 10.<N>.22.1/24 ↔ `ns-w<N>-wan` `w<N>w1.201/.202` (.2), server 10.<N>.99.1/32 on lo.
- VRF `w<N>-ta`: 10.<N>.99.0/24 via 10.<N>.21.2 AND 10.<N>.22.2 (ECMP), static neighbour for 10.<N>.21.2; ABF: tcp dport 8001 →
  10.<N>.22.2 only; ACL: address group `w<N>-srv` {10.<N>.99.1}: permit icmp + tcp 8000-8001, deny tcp 8002; NAT44-ED inside
  `loop<N>10`, outside both sub-ifs, pool + return route as in nat44-ed-sessions.

## Steps
1. `eval "$(tools/lab env <N>)"`; NRestarts before; `tools/lab rig up w<N>`; peers down until `apps/agent/bin/ngfw-vpp-preflight`
   exits 0 (D-095). Interfaces the driver creates outside the agent get the ip4+ip6 classify reset first (D-185).
2. Slot stack (agent + API as `newStack`); commit the baseline (rig interfaces, VRF), then ONE commit of the whole chain via
   `POST /api/v1/config/commit` → paste `status: applied`, `notApplied: []`, no `agent.unsupported-field`; `/state/drift` clean.
3. Traffic: ping 10.<N>.99.1; 20 TCP flows to :8000 (distinct source ports); 5 to :8001; 3 to :8002; `ping -I 10.<N>.9.9 10.<N>.99.1`.
   Expected: ping, :8000, :8001 succeed; :8002 and the spoof fail.
4. NAT44-EI: one commit swapping ED for EI (locks as the EI test). If nat44-ed is enabled by another owner (e.g. tools/app) at window
   time, stop this step and record it — never touch it. Repeat ping + :8000 flows.
5. Rollback to the baseline revision → Retrieve holds no chain object; delete the rig interfaces through the API, veths down.

## Evidence (paste in docs/status/tasks/TEST-traffic-A.md; full output in `TEST-traffic-A-evidence/*.txt`, D-175; no .pcap committed)
- `tcpdump -nn -e` inside ns-w<N>-lan and ns-w<N>-wan: tag 100 on lan; source = pool address on wan; :8000 flows on BOTH vlan 201 and
  202 (ECMP), :8001 only on 202 (ABF); no :8002 SYN and no 10.<N>.9.9 packet on wan (ACL, uRPF).
- `timeout 10 vppctl` read-only shows: `show bridge-domain <id> detail`, `show ip fib table <N>010 10.<N>.99.0/24` (2 paths,
  load-balance `to:[packets:bytes]` before/after), `show ip neighbors`, `show abf attach loop<N>10`, `show acl-plugin acl`,
  `show errors` uRPF lines before/after, `show nat44 sessions` / `show nat44 ei sessions`, counters of the prefixed interfaces.
- NRestarts before/after every step, window start/end, the commit responses, the driver command line.

## Cleanup
Stack stopped by PID; `tools/lab rig down w<N>` (never `vppctl delete host-interface` by hand, V24/D-101); `pg-test.sh drop`; residue
pasted: `vppctl show interface | grep w<N>`, table <N>010 gone, no pool address of ours in NAT, `ip netns list | grep w<N>` empty.

## Out of scope
Product-code fixes (failure → `docs/status/tasks/TEST-traffic-A-questions.md` with evidence + a proposed fix row; continue) · bonding, GSO,
LLDP, SPAN, RA, host ACL/nftables (list as "not on the chain": no transit packet on a one-veth-per-side rig) · performance · screenshots ·
schema/proto changes · table-0 routes · other VPP globals.

## Rules
Slot prefix everywhere; never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128: no trace add/show/clear,
no pcap trace); lab lock `flock -s` for the run only (D-094), `flock -x /run/lock/ngfw-globals.lock` only around a global step (D-167);
every go build/test/vet, tsc, vitest, vite/pnpm build and pnpm install runs through `tools/heavy.sh` (D-224; never a server or daemon);
D-210a: the driver's own self-test passes (go vet + a unit test of its evidence parsers, or `bash -n` + `--dry-run`; paste it), no full
suite; `tools/ci-slot.sh --base main` green at the end; commit the driver and docs/status/tasks/TEST-traffic-A.md with real output.
