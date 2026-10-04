# TEST-traffic-C — wave-C traffic scenario: MPLS/SRv6, VRRP failover, QoS policer (review 6.6a, D-125)   (prepend 00-CONTEXT.md)
Start only when: board dep F-ha-state-sync is merged (ready today) AND, for the steps named below, S-vrrp-product-fixes (failover by
commit; until then F1 makes `enabled:false` fail 422 with a spurious failover), S-capture-retention-stop + F-capture-trace-host
(capture rider), F-mpls-ldp-host (optional LDP step). Runs ONCE in a **manager window**, VPP otherwise idle (V22b VRRP engine and IGMP
run alone, launch queue §3); envelope: slot `w<N>`, daemon-owner **keepalived** (+ frr only for LDP). Wave C closes when this is green.

## Goal
One scripted run over the wave-C packet features back to back on the af_packet rig, configured through the product (slot API → agent),
tcpdump/counter evidence; observability features ride on the same traffic. One VPP only: VRRP failover = VPP VR vs a keepalived fixture
in a slot netns (what F-vrrp-config-sync-host proved possible), not two VPP routers — say so in the status file.

## Read first
`test/topology/vrrp/host.sh` (step 5: lan bridge, keepalived node B in `ns-w<N>-kb`, pinger), `docs/status/tasks/F-vrrp-config-sync-host.md`
§6 + questions F1–F4, `apps/agent/internal/agent/rpc_mpls_srmpls_integration_test.go` (table-0 rule), `.../srv6_integration_test.go`
(`TestSrv6GlobalsOnHost` = tech-debt TD-H18), `test/topology/{srv6/stack.sh,qos-flat/qos_test.go,ipfix-sflow/collector.go}`,
`test/topology/nat44-ed-sessions/` (slot stack + netns traffic helpers), `docs/lab/shared-host-rules.md` §2/§3/§11, D-167, D-175.

## Files you own
`test/topology/traffic-c/**` (driver; call/copy the pieces above, never edit them) · `docs/status/tasks/TEST-traffic-C*`.

## Steps (each: API commit → `applied`, `notApplied: []`, no `agent.unsupported-field`; traffic; evidence; rollback)
1. **MPLS** (F-mpls-srmpls): MPLS on `host-w<N>w0`, a slot-VRF route 10.<N>.98.0/24 via 10.<N>.2.2 with an out-label; ping from ns-lan.
   Table 0 only inside the globals window: created only if absent, deleted again, before/after diff identical (F-mpls-srmpls-host C4).
   Evidence = labelled frames in ns-wan (`tcpdump -e`: ethertype 0x8847 + label); no reply expected (never load kernel modules).
2. **SRv6** (F-srv6): IPv6 on the rig through the document (fd00:<slot hex>::/32), H.Encaps policy steering 10.<N>.160.0/24 to a SID
   routed to ns-wan; encap source = global → set and restore inside the window; run `TestSrv6GlobalsOnHost` in the same window
   (closes TD-H18). Evidence: IPv6 + SRH (routing header type 4, segment list) around the inner IPv4 in ns-wan.
3. **VRRP failover** (host.sh step-5 shape): VPP VR prio 200 (accept mode) vs keepalived prio 100; `ping -D -i 0.2` to the VIP;
   failover by commit `enabled:false` (after S-vrrp-product-fixes; otherwise `vrrp proto stop` as the host row, labelled simulation),
   return by commit; longest outage ≤ 3 s; VIP ARP moves to/from 00:00:5e:00:01:<vrid>.
4. **QoS policer** (F-qos-flat): policer `w<N>-gold` on the lan interface input with a low CIR; send above it from ns-lan (ping
   `-s 1200 -i 0.01` or a python UDP sender — counts only); conform/exceed/violate rise, wan tcpdump count < sent.
5. **Riders on the same traffic**: IPFIX records at the collector in ns-wan (exporter 0 is global: window, restore); product capture
   (`POST /api/v1/actions/capture` while pinging → download → `tcpdump -r`; one pcap per VPP; after the capture rows); IGMP join from
   ns-lan → `show igmp groups` + the mFIB entry; slot agent `/metrics` interface counters rise.
6. **Not exercised — write the reason**: LISP (V14 window, no peer), LB (loopback-only host proof), host stack, SNMP, HA state sync
   (needs a second VPP → INTEGRATE-E2E), LDP unless F-mpls-ldp-host merged.

## Evidence (pasted in docs/status/tasks/TEST-traffic-C.md; full output `TEST-traffic-C-evidence/*.txt`, D-175; no .pcap)
- tcpdump text from the rig netns for every step (MPLS label, SRH, VIP ARP + ping gaps with timestamps, policer drop count).
- `timeout 10 vppctl`: `show mpls fib table <t>`, `show ip fib table <T> 10.<N>.98.0/24`, `show sr policies`, `show sr steering-policies`,
  `show vrrp vr`, `show policer` (+ `GET /api/v1/state/services/qos/policers`), `show ipfix exporter`, `show igmp groups`,
  `show ip mfib table <T>`; keepalived node-B log lines (MASTER/BACKUP); collector stats; capture `tcpdump -r` head.
- NRestarts before/after every step; window start/end; every global changed (before value, after value, restored value).

## Cleanup
Rollback after every step; keepalived fixture and every other process stopped by PID; `tools/lab rig down w<N>` (never
`vppctl delete host-interface` by hand, V24/D-101); globals back to their before values (diff pasted); residue: nothing with `w<N>` in
`show interface`, `show vrrp vr`, `show sr policies`, `show policer`, MPLS table 0 state = before; `ip netns list | grep w<N>` empty.

## Out of scope
Product-code fixes (failure → `docs/status/tasks/TEST-traffic-C-questions.md` with evidence and a proposed fix row; continue) ·
two-VPP HA/VRRP (INTEGRATE-E2E) · performance or rate claims (policer = counters only) · screenshots · schema/proto · kernel modules.

## Rules
Slot prefix everywhere; never restart or kill VPP; `timeout 10` on every vppctl; packet trace banned (D-128; no manual `pcap trace` — step 5's product capture is the only capture);
lab lock `flock -s` for the run only, globals only `flock -x /run/lock/ngfw-globals.lock` (D-167), never `flock -x` on the lab lock;
every go build/test/vet, tsc, vitest, vite/pnpm build and pnpm install runs through `tools/heavy.sh` (D-224; never a server or daemon);
D-210a: the driver's self-test passes (`bash -n` + `--dry-run`, or go vet + a parser unit test; paste it), no full suite;
`tools/ci-slot.sh --base main` green at the end; commit the driver and docs/status/tasks/TEST-traffic-C.md with real output.
