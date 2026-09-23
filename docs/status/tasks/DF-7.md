# DF-7 — Descriptors: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls

Branch `task/DF-7` (worktree `/root/ngfw-wt/DF-7`, slot 10, prefix `w10`, tables 10000–10999, addresses 10.10/16).
Main merged in (DF-1 resolver D-069, D-071, D-076). Status: **done, not merged**; open points in `DF-7-questions.md`.

## What was built

32 object types in 9 packages under `apps/agent/internal/descriptors/` plus a shared package `df7`:

| Plugin | Object types (R = Retrieve from a dump, W = write-only D-063, G = VPP-global, globals owner only D-071) | Doc |
|---|---|---|
| policer | `policer.policer` R · `policer.interface` W · `policer.bind` W · `policer.classify` W | `docs/agent/descriptors/policer.md` |
| qos | `qos.record` R · `qos.store` R · `qos.egress-map` R · `qos.mark` R | `qos.md` |
| lb | `lb.conf` W+G · `lb.vip` W · `lb.as` W · `lb.intf-nat` W | `lb.md` |
| span | `span.mirror` R | `span.md` |
| lldp | `lldp.global` W+G · `lldp.interface` W (Create verified via `lldp_dump`) | `lldp.md` |
| bfd | `bfd.auth-key` R · `bfd.udp-session` R · `bfd.echo-source` R+G · events | `bfd.md` |
| vrrp | `vrrp.vr` R · `vrrp.vr-peers` R · `vrrp.vr-track-interface` R · `vrrp.vr-state` R · events | `vrrp.md` |
| igmp | `igmp.interface` W · `igmp.listen` R · `igmp.group-prefix` W+G · `igmp.proxy-device` W · `igmp.proxy-downstream` W · events | `igmp.md` |
| mpls | `mpls-table` R · `mpls-interface` R · `mpls-route` R · `mpls-ip-bind` W · `mpls-tunnel` R | `mpls.md` (key contract `mpls-table/<id>`, `mpls-interface/<name>` for DF-6) |

Every write-only type is write-only because VPP 26.06 has no usable dump (reasons per type in the docs; four of
them — lb vip/as, policer.classify, igmp.group-prefix — have a dump that is broken, evidenced below).

- `df7`: generic structpb codec for the typed specs (D-055), interface snapshot on DF-1's `iface` resolver (logical
  names, D-069) with the D-071 claim rule (own tag / never foreign / untagged only with a claim of the object key in
  `iface.Claims`), `Reresolve` for every Delete (never a stored index), `ApplyOnce`/`AppliedNow` boot-identity
  records (D-076), FIB path codec with MPLS label stacks, `IDRange`, `Watch` event plumbing,
  `ErrRetrieveUnsupported` (same text as the scheduler's sentinel).
- `df7/registry.Register(r, client, Config{Owner, GlobalsOwner, BFDSecrets, Options})` — the registry-list entry:
  28 descriptors, 32 for the globals owner.
- `df7/df7test`: fake with interface table + VPP boot identity (`Reboot()`), plan diff, host helpers
  (`RestartSimulation` with a loss leg, V15-safe address/table cleanup, `NoLeftovers`, `GlobalsOptIn`,
  `AlignedLoopback`).
- Events (StreamEvents): `bfd.WatchEvents`, `vrrp.WatchEvents`, `igmp.WatchEvents` (want_* registration with the
  process PID, owner-filtered decode, unregister on ctx end).
- Action helpers: `policer.Reset`, `lb.FlushVIP`, `igmp.ClearInterface`; state readers `lb.DumpVIPs`,
  `lldp.Neighbours`, `bfd.Sessions`, `vrrp.States`.

## How it was verified (real output, this host, 2026-09-24)

### Unit tests (fake VPP; models duplicate adds, counters, VPP restarts)

```
$ go test -count=1 ./internal/descriptors/{policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls,df7}/...
ok  	ngfw/agent/internal/descriptors/policer	0.026s
ok  	ngfw/agent/internal/descriptors/qos	0.034s
ok  	ngfw/agent/internal/descriptors/lb	0.022s
ok  	ngfw/agent/internal/descriptors/span	0.018s
ok  	ngfw/agent/internal/descriptors/lldp	0.023s
ok  	ngfw/agent/internal/descriptors/bfd	0.030s
ok  	ngfw/agent/internal/descriptors/vrrp	0.027s
ok  	ngfw/agent/internal/descriptors/igmp	0.025s
ok  	ngfw/agent/internal/descriptors/mpls	0.023s
ok  	ngfw/agent/internal/descriptors/df7	0.032s
?   	ngfw/agent/internal/descriptors/df7/df7test	[no test files]
ok  	ngfw/agent/internal/descriptors/df7/registry	0.022s

$ grep -rn "vppctl\|exec.Command" internal/descriptors/{policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls}; echo "grep exit: $?"
grep exit: 1
```

Covered per type: create request fields, idempotent re-apply (empty plan), update in place / ErrRecreate, delete,
dependencies, Retrieve decoding incl. other owners' objects filtered, untagged interfaces via claims, VPP errors,
D-076 duplicate adds (`policer.interface` / `lb.intf-nat` feature stacking stays 1 across 3 resyncs, re-added once
after `Reboot()`, no un-apply after a restart), index reuse on delete (policer at a reused pool index of another
owner is not deleted), event decoding (bfd/vrrp/igmp).

### Host run (VRX_INTEGRATION=1, shared lab lock, slot 10) — `systemctl show vpp -p NRestarts` 2 before and after

```
$ VRX_INTEGRATION=1 go test -count=1 -v -run OnHost ./internal/descriptors/{policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls}/
--- PASS: TestPolicerOnHost (0.06s)
    --- PASS: TestPolicerOnHost/update_in_place (0.00s)
    --- PASS: TestPolicerOnHost/interface_attach_(write-only) (0.01s)
    --- SKIP: TestPolicerOnHost/bind_to_worker (0.00s)
    --- PASS: TestPolicerOnHost/classify_(write-only) (0.01s)
    --- PASS: TestPolicerOnHost/restart_simulation (0.02s)
ok  	ngfw/agent/internal/descriptors/policer	0.094s
--- PASS: TestQoSOnHost (0.16s)
    --- PASS: TestQoSOnHost/update_in_place (0.01s)
    --- PASS: TestQoSOnHost/restart_simulation (0.07s)
    --- PASS: TestQoSOnHost/restart_simulation_(maps,_after_their_marks_are_gone) (0.02s)
ok  	ngfw/agent/internal/descriptors/qos	0.192s
--- PASS: TestLBOnHost (0.03s)
    --- SKIP: TestLBOnHost/conf_(write-only,_globals_owner_only) (0.00s)
ok  	ngfw/agent/internal/descriptors/lb	0.069s
--- PASS: TestSpanOnHost (0.07s)
    --- PASS: TestSpanOnHost/update_state_in_place (0.00s)
    --- PASS: TestSpanOnHost/restart_simulation (0.02s)
ok  	ngfw/agent/internal/descriptors/span	0.093s
--- PASS: TestLLDPOnHost (0.04s)
    --- SKIP: TestLLDPOnHost/global_(write-only,_globals_owner_only) (0.00s)
    --- PASS: TestLLDPOnHost/interface_(write-only,_verified_via_lldp_dump) (0.03s)
ok  	ngfw/agent/internal/descriptors/lldp	0.062s
--- PASS: TestBFDOnHost (0.13s)
    --- PASS: TestBFDOnHost/update_in_place (0.01s)
    --- PASS: TestBFDOnHost/events (0.00s)
    --- SKIP: TestBFDOnHost/echo_source (0.00s)
    --- PASS: TestBFDOnHost/restart_simulation (0.04s)
    --- PASS: TestBFDOnHost/restart_simulation_(keys,_after_their_sessions_are_gone) (0.02s)
ok  	ngfw/agent/internal/descriptors/bfd	0.158s
--- PASS: TestVRRPOnHost (0.18s)
    --- PASS: TestVRRPOnHost/events (0.00s)
    --- PASS: TestVRRPOnHost/peers_update_on_a_running_VR (0.01s)
    --- PASS: TestVRRPOnHost/track_priority_update (0.01s)
    --- PASS: TestVRRPOnHost/restart_simulation_+_update_without_a_pool_index (0.03s)
    --- PASS: TestVRRPOnHost/restart_simulation_with_loss_(state,_tracking) (0.04s)
    --- PASS: TestVRRPOnHost/restart_simulation_with_loss_(VRs,_after_their_children_are_gone) (0.02s)
ok  	ngfw/agent/internal/descriptors/vrrp	0.225s
--- PASS: TestIGMPOnHost (0.12s)
    --- PASS: TestIGMPOnHost/listen_update (0.01s)
    --- PASS: TestIGMPOnHost/proxy_(write-only) (0.01s)
    --- PASS: TestIGMPOnHost/group_prefix_dump_(read-only_probe) (0.00s)
    --- SKIP: TestIGMPOnHost/group_prefix_(write-only,_globals_owner_only) (0.00s)
    --- PASS: TestIGMPOnHost/restart_simulation (0.01s)
ok  	ngfw/agent/internal/descriptors/igmp	0.151s
--- PASS: TestMPLSOnHost (0.18s)
    --- PASS: TestMPLSOnHost/route_paths_update_in_place (0.01s)
    --- SKIP: TestMPLSOnHost/interface_+_ip_bind_(need_MPLS_table_0) (0.00s)
    --- PASS: TestMPLSOnHost/restart_simulation (0.07s)
    --- PASS: TestMPLSOnHost/restart_simulation_(tables,_after_their_routes_are_gone) (0.01s)
ok  	ngfw/agent/internal/descriptors/mpls	0.207s
```

Skip reasons (pasted from the same log):

```
skip: no workers on host (policer_bind → policer.bind: policer_bind w10:gold worker 0 enable=true: VPPApiError: Invalid worker thread (-89))
skip: lb.conf is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in
skip: lldp.global is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in
skip: bfd.echo-source is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in
skip: igmp.group-prefix (the SSM range list) is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in
mpls-interface Create without MPLS table 0: mpls-interface: sw_interface_set_mpls_enable 15 enable=true: VPPApiError: No such FIB / VRF (-3)
skip: MPLS table 0 (created/locked by mpls enable and label bindings) is VPP-global — only the globals owner sets it (D-071); VRX_DF7_GLOBALS=1 to opt in
```

Every host test does: create → Retrieve == desired (empty re-apply plan) → update in place → **restart simulation**
(fresh connection + fresh descriptor → empty plan → objects deleted behind the agent's back → plan = one Create per
object → recreated → empty plan) → delete → Retrieve shows nothing of ours. Examples:

```
restart simulation (qos.record): fresh connection, fresh descriptor
qos.record: re-apply plan for 2 desired object(s):
  (empty plan)
qos.record: plan after simulated loss of 2 object(s):
  create qos.record/loop1020/ip
  create qos.record/loop1020/vlan
qos.record: recreated 2 object(s)
restart simulation (mpls-route): fresh connection, fresh descriptor
mpls-route: re-apply plan for 3 desired object(s):
  (empty plan)
mpls-route: plan after simulated loss of 3 object(s):
  create mpls-route/10090/10090/eos
  create mpls-route/10090/10091/neos
  create mpls-route/10091/10091/eos
mpls-route: recreated 3 object(s)
mpls-route: re-apply plan for 3 desired object(s):
  (empty plan)
pool index found by the walk: {SwIfIndex:7 Index:1 HasIndex:true} (Create returned {SwIfIndex:7 Index:1 HasIndex:true})
bfd event: {Key:bfd.udp-session/loop1060/10.10.60.1/10.10.60.3 Interface:loop1060 Local:10.10.60.1 Peer:10.10.60.3 State:admin-down}
vrrp event: {Key:vrrp.vr/loop1070/10/ipv4 VR:{Interface:loop1070 VRID:10 IPv6:false} OldState:init NewState:backup}
live state: [{Key:vrrp.vr/loop1070/11/ipv4 State:backup Priority:50 MasterAdvCS:200} {Key:vrrp.vr/loop1070/10/ipv4 State:backup Priority:70 MasterAdvCS:100}]
loop1050: sw_if_index 14 hw_if_index 14 (found true)
created lldp.interface/loop1050 (meta {SwIfIndex:14})
created lldp.interface/loop1050 (meta {SwIfIndex:14})
deleted lldp.interface/loop1050
lldp_dump after delete: loop1050 not listed
ip_route_dump table 10080: nothing of 10.10.0.0/16 left
```

Evidence for the write-only decisions (same run):

```
policer_classify_dump(~0) with a table bound on loop1010: 0 details, err <nil>
policer_input_v2 probe: err=<nil>
lb_vip_dump: {Prefix:10.10.30.1/32 Port:80 Encap:gre4 DSCP:0 TargetPort:0 RawProtocol:0 RawFlowTableLength:0}
lb_vip_dump: {Prefix:10.10.30.2/32 Port:0 Encap:l3dsr DSCP:10 TargetPort:0 RawProtocol:0 RawFlowTableLength:0}
lb_vip_dump: {Prefix:10.10.30.3/32 Port:8080 Encap:nat4 DSCP:0 TargetPort:20480 RawProtocol:0 RawFlowTableLength:0}
(earlier run) igmp_group_prefix_dump (why it is not used): 0 details, err unexpected message: *igmp.IgmpDetails &{1 0.232.0.0 0.0.0.0}
```

### `vppctl show …` while the prefixed objects exist, then after delete (VRX_DF7_EVIDENCE_HOLD)

```
--- vppctl show policer (objects present)
Name "w10:bronze" type 1r2c cir 500 eir 0 cb 8000 eb 0
rate type pps, round type up
conform action mark-and-transmit EF, exceed action drop, violate action drop
Name "w10:gold" type 2r3c-2698 cir 1500 eir 3000 cb 16000 eb 32000
rate type kbps, round type closest
conform action transmit, exceed action mark-and-transmit AF11, violate action drop
--- vppctl show interface features loop1010 (objects present)
ip4-output:
  policer-output
ip4-unicast:
  policer-input
  ip4-not-enabled
--- vppctl show policer (after delete)
(empty)

--- vppctl show qos egress map (objects present)
 Map-ID:10002
  MPLS:[0,1,2,3,4,5,6,7,0,1,2,3,4,5,6,7,…]
 Map-ID:10001
  IP:[0,0,0,0,1,1,1,1,2,2,2,2,3,3,3,3,4,…]
--- vppctl show qos record (objects present)
 loop1020:
  VLAN
  IP
--- vppctl show qos store (objects present)
 loop1021:
  IP -> 46
--- vppctl show qos mark (objects present)
 loop1021:
  IP: map:1
--- vppctl show qos egress map / record / store / mark (after delete)
(empty)

--- vppctl show lb vips verbose (objects present; the three of this run)
 ip4-gre4 [18] 10.10.30.1/32
  protocol:6 port:80
  #as:2
    10.10.31.1 512 buckets   0 flows  dpo:59 used
    10.10.31.2 512 buckets   0 flows  dpo:39 used
 ip4-l3dsr [19] 10.10.30.2/32
  dscp:10
  #as:1
    10.10.31.3 1024 buckets   0 flows  dpo:69 used
 ip4-nat4 [20] 10.10.30.3/32
  protocol:17 port:8080
  type:clusterip port:36895 target_port:80
--- after delete: the same three are listed as "… removed" (VPP keeps them until its GC — Q1)

--- vppctl show interface span (objects present)
Source                           Destination                       Device       L2
loop1040                         loop1041                         (  both) (  none)
                                 loop1042                         (    tx) (  none)
loop1042                         loop1041                         (  none) (    tx)
--- vppctl show interface span (after delete)
(empty)

--- vppctl show bfd sessions (objects present)
     0     IPv4 address                               10.10.60.1           10.10.60.3
           Session state                                    Down                 Down
           Detect multiplier                                   4                    0
           Required Min Rx Interval (usec)                200000                    1
           Desired Min Tx Interval (usec)                 200000                    0
           Authentication config key ID                    10060
           Authentication BFD key ID                           2
     1     IPv4 address                               10.10.60.1           10.10.60.2
Number of configured BFD sessions: 2
--- vppctl show bfd keys (objects present)
     10060 Meticulous Keyed SHA1              2
--- after delete
Number of configured BFD sessions: 0
Number of configured BFD keys: 0

--- vppctl show vrrp vr (objects present)
[0] sw_if_index 15 VR ID 11 IPv4
   state Backup flags: preempt no accept yes unicast yes
   priority: configured 50 adjusted 50
   timers: adv interval 200 master adv 200 skew 160 master down 760
   addresses 10.10.70.252 10.10.70.253
   peer addresses 10.10.70.2 10.10.70.3
[1] sw_if_index 15 VR ID 10 IPv4
   state Backup flags: preempt yes accept no unicast no
   priority: configured 100 adjusted 70
   addresses 10.10.70.254
   tracked interfaces sw_if_index 14 priority 30
--- vppctl show vrrp vr (after delete)
(empty)

--- vppctl show igmp config (objects present)
interface: loop1080 mode: HOST proxy device: 10080
interface: loop1081 mode: ROUTER proxy device: 10080
interface: loop1082 mode: HOST
    232.10.0.1
        10.10.80.12 not-running
--- vppctl show ip mfib table 10080 (objects present)
(*, 224.0.0.1/32):  Interfaces: loop1082: Accept, loop1081: Accept, loop1080: Accept,
--- vppctl show igmp config / show ip mfib table 10080 (after delete)
(empty)

--- vppctl show mpls fib table 10090 (labels ≥ 16)
10090:eos/21 fib:0 index:25 locks:2
  API refs:1 src-flags:added,contributing,active,
      path:[71] … 10.10.90.2 loop1090
      path:[115] … 10.10.90.4 loop1090
10091:neos/21 fib:0 index:31 locks:2
      path:[93] … 10.10.90.3 loop1090
     path:93  labels:[[300 pipe ttl:0 exp:0]]
--- vppctl show mpls fib table 10091
10091:eos/21 fib:1 index:72 locks:2
      path:[114] pl-index:91 ip6 weight=1 pref=0 special:  cfg-flags:drop,
--- vppctl show mpls tunnel
[@0] mpls-tunnel0: sw_if_index:23 hw_if_index:22
        10.10.90.2 loop1090
     path:101  labels:[[500 pipe ttl:0 exp:0][600 pipe ttl:0 exp:0]]
--- after delete
No MPLS tunnels configured...
```

(`show lldp detail` during the hold of the evidence runs happened to catch runs where no aligned loopback existed;
the LLDP enable/disable on `loop1050` is shown by the test log above via `lldp_dump`.)

### CI gate

```
$ tools/ci.sh --base main
…
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m22s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m21s
  apps/agent: make lint test build                   0m12s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      merge main into task/DF-7 (D-069 resolver, D-071, D-076)
  mode quick · wall time 1m04s · logs /root/ngfw-wt/logs/ci/DF-7-20260924-014809-1576264

CI GATE PASSED
```

(The only warning is the subject of the `git merge main` commit; history is not rewritten.)

## FIB entries installed by DF-7 objects (P05 / V15 request)

- `mpls-table` creates an MPLS FIB (with VPP's reserved-label entries); `mpls-route` adds API label entries (deleted
  before the table); `mpls-ip-bind` adds a local label to an IP prefix and entries in MPLS table 0.
- `lb.vip` installs the VIP prefix in table 0; `lb.as` makes VPP track each AS address with a recursive-resolution
  entry in table 0 that only VPP's GC removes (Q1: `10.10.31.1-3/32` are left in table 0 from my lb runs).
- `igmp.interface` adds mFIB entries (224.0.0.1/2/22) in the interface's multicast table, removed on disable.
- policer, qos, span, lldp, bfd, vrrp (no accept-mode master) add no FIB entries.
- Host-test helpers only: interface addresses (removed before the loopback/table, with the stale ip-classify
  binding cleared first — Q3: that binding was the source of the stray `10.10.82.1/32`; `10.10.70.1/32` cleaned).

## Out of scope / not done

- API, UI, schema, F-* wiring, keepalived VRRP (RF-4), FRR BFD/PIM, multicast FIB routes, BIER, HQoS.
- Host runs of the VPP-global objects (`lb.conf`, `lldp.global`, `bfd.echo-source`, `igmp.group-prefix`) and of
  the objects needing MPLS table 0 — only with `VRX_DF7_GLOBALS=1` (D-071); unit-tested.
- `policer.bind` host run: no worker threads on the host (`INVALID_WORKER`, skip reason above).
- MPLS paths of proto mpls with a via-label / lookup table and uniform-mode labels: not reported by VPP's path
  encoder → rejected as non-canonical / pipe mode only.

## Decisions (for the LOG)

1. Values: typed Go specs encoded to `*structpb.Struct` through one generic JSON codec (`df7.Encode/Decode`, all
   fields `omitempty`, unknown fields rejected) — (a) hand-written per-field structpb as in DF-4, (b) generic codec →
   (b): one canonical form by construction, P03b swap touches the spec types only.
2. 13 write-only types (D-063), each with the reason in its doc; four have a dump that VPP 26.06 corrupts or
   mis-labels (lb vip/as, policer.classify, igmp.group-prefix) — evidence above.
3. Ownership: owner tag in name/tag fields (policer, MPLS table/tunnel, tunnel interface), interface ownership with
   the D-071 claim keyed by the object key for per-interface objects, id range for QoS egress maps and BFD conf keys.
4. Globals (D-071): `lb.conf`, `lldp.global`, `bfd.echo-source`, `igmp.group-prefix` only via `RegisterGlobals` /
   `registry.Config.GlobalsOwner`; host tests opt-in (`VRX_DF7_GLOBALS=1`). MPLS table 0 treated as global (Q5).
5. D-076/D-080: `policer.interface` and `lb.intf-nat` (feature stacking) apply once per D-080 boot identity and
   `<sw_if_index>/<name>` (`df7.ApplyOnce`, `dfkit.BootStore`, `df7.SetBootStore` for P05); reference-counted
   enables (qos record/store, mpls-interface) enable only when the dump does not list them; lb VIPs/ASes and MPLS
   table-0 labels carry ownership records written after our own add (fix round).
6. lb enum byte-swap workaround for VPP's missing `ntohl` (little-endian only) — (a) only gre4/clusterip VIPs,
   (b) workaround + V-item → (b).
7. VRRP Update after an agent restart walks the pool index (VPP's key check makes wrong indexes harmless) — (a)
   ErrRecreate (VR flap), (b) walk → (b); review L3 (cache) not done: Meta carries the index, the walk only runs
   without Meta. VRRP peers Delete is a no-op (VPP cannot clear peers; they go with the VR).
8. BFD key secrets only through a `Secrets` resolver, never in a Value; rotation = new conf-key id.
9. MPLS descriptor names hyphenated (`mpls-table`, …) to match DF-6's `mpls-table/<id>` contract.
10. Test-only `cli_inband` reads (`show hardware-interfaces` for the LLDP index guard, `show ip fib` diagnostics in
    `NoLeftovers`); never in a descriptor.

## Open questions

`docs/status/tasks/DF-7-questions.md` (Q1 lb GC/leftovers — numbers corrected, Q2 classify key, Q3 stale
ip-classify binding, Q4 interface-ip deps, Q5 MPLS table 0, Q6 V-item candidates, Q7 P05/P08 wiring —
`SetBootStore`, Q8 LLDP mismatch cannot be undone via the API, Q9 VPP crash 02:23 during my parallel host run).

## Review fixes (DF-7-review.md, fix round after `git merge main` incl. TD-1 `bootid`)

| Finding | Change | Evidence |
|---|---|---|
| H1 applied-once records | `df7/applied.go` on `dfkit.BootStore` + `dfkit.IdentitySource` (= `bootid.Current`, D-080 triple boot_id/PID/start time); per-interface value `<sw_if_index>/<name>` (`df7.IfaceValue`); `ApplyOnce` records only after success; policer Delete sends `apply=0` only with a matching record | `TestApplyOnce` (new PID, same PID + new start time, re-created interface, failed apply), `TestAttachments` (no `policer_input` un-apply after `RestartSamePID`, after loop1 re-created at index 12, after a failed apply) |
| H2 mpls table 0 | `mpls-route` in table 0: Create refuses an existing label without our record (`ErrNotOurs`), records after its add; Update/Delete/Retrieve only for recorded labels | `TestRouteSharedTable0`: SR-MPLS BSID, mpls-ip-bind, FRR/linux-cp, reserved entries neither reported nor deleted; Create/Update refused; failed add unrecorded; record expires with the VPP instance |
| M1 claim after add, no adoption | claims via `dfkit.Target.Claim` only after VPP accepted (policer, qos, span, lldp, bfd, vrrp, igmp, mpls, lb.intf-nat); "already exists" paths use `Target.Adopt` (ours only when tagged/claimed) | `TestInterfacesAndClaims`, `TestRecordStore`, `TestInterface` (mpls), `TestMirrorNoAdopt`, `TestInterfaceMismatchUndo` (lldp), `TestSessionFailedAddNoClaim` (bfd EEXIST), `TestVRExistsNoClaim` (vrrp ENTRY_ALREADY_EXISTS), `TestInterfaceNoAdopt` (igmp -1) |
| M2 lb enum order | after each add `lb_vip_dump` must show the requested type; otherwise delete, switch byte order, retry once, else `ErrEnumOrder` | `TestVIPEnumOrder` (fake with ntohl → adapts; wrong type in both orders → `ErrEnumOrder`, VIP removed, unrecorded) |
| M3 lb leftovers | lb host test opt-in `VRX_DF7_LB=1`; Q1 numbers corrected (26 removed VIPs, `#vips: 27 #ass: 24`, `10.10.31.1` refs:8 — wiped by the 02:23 restart); per-update leak in `lb.md` | host run below: `SKIP: TestLBOnHost` |
| M4 lb adoption / delete | VIP/AS `VALUE_EXIST` = success only with our record, else `ErrNotOurs`; Delete only of recorded objects; `NO_SUCH_ENTRY` = success | `TestVIP`, `TestASAndNat` |
| M5 policer index | Update re-verifies the stored index by name (`currentIndex`); `Reset(ctx, c, owner, name)` looks the index up | `TestPolicerLifecycle` (Meta index 0 reused by another owner → update goes to 7; gone → error) |
| M6 LLDP mismatch | **investigated, not undoable**: VPP's disable looks up `hw(arg)->sw_if_index`, so a disable with our index removes another interface's entry; Create now sends no disable, claims nothing, fails loudly (`ErrIndexMismatch … NOT undone`) — the first version of this fix sent that disable and was removed | `TestInterfaceMismatchUndo`; DF-7-questions Q8 |
| L1 counters | qos record/store, mpls-interface: one disable (mpls only while the dump lists it); Create never adds a second reference | `TestRecordStore`, `TestInterface` (mpls) |
| L2 / L4 / L3 | documented (`vrrp.md` accept-mode addresses + IGMP join, `igmp.md` mode drift; VRRP walk kept) | docs |
| L5 / L6 | not changed in this round: L5 proof gaps remain where host tests are opt-in (globals, lb, vrrp, igmp); L6 key ambiguity with `/` in names is open — no validation of `/` in interface / policer names added (needs a rule for all factories; proposal: reject `/` in spec validation) | — |
| Manager: D-082 | `df7test.GlobalsOptIn` holds `/run/lock/vrx-globals.lock` exclusively for the (sub)test | `df7/df7test/host.go` |
| Manager: VPP crash (D-087) | host tests one package at a time; `TestVRRPOnHost` / `TestIGMPOnHost` opt-in (`VRX_DF7_VRRP_HOST`, `VRX_DF7_IGMP_HOST`); trigger + backtrace in Q9 | below |
| Manager: TD-1 | fake answers `control_ping` (vpe_pid) and sets `dfkit.IdentitySource` to a `bootid.Identity`; no `iface.VPPIdentity` | build + tests |

### Unit tests

```
$ go test -count=1 ./internal/descriptors/{df7,policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls}/...
ok  	ngfw/agent/internal/descriptors/df7	0.027s
ok  	ngfw/agent/internal/descriptors/df7/registry	0.023s
ok  	ngfw/agent/internal/descriptors/policer	0.045s
ok  	ngfw/agent/internal/descriptors/qos	0.035s
ok  	ngfw/agent/internal/descriptors/lb	0.022s
ok  	ngfw/agent/internal/descriptors/span	0.029s
ok  	ngfw/agent/internal/descriptors/lldp	0.025s
ok  	ngfw/agent/internal/descriptors/bfd	0.027s
ok  	ngfw/agent/internal/descriptors/vrrp	0.030s
ok  	ngfw/agent/internal/descriptors/igmp	0.026s
ok  	ngfw/agent/internal/descriptors/mpls	0.032s
$ go test -count=1 -v -run '<fix-round tests>' …   (excerpt)
--- PASS: TestInterfacesAndClaims   --- PASS: TestApplyOnce        --- PASS: TestPolicerLifecycle
--- PASS: TestAttachments           --- PASS: TestRouteSharedTable0 --- PASS: TestVIP
--- PASS: TestVIPEnumOrder          --- PASS: TestASAndNat          --- PASS: TestRecordStore
--- PASS: TestInterfaceMismatchUndo --- PASS: TestMirrorNoAdopt     --- PASS: TestSessionFailedAddNoClaim
--- PASS: TestVRExistsNoClaim       --- PASS: TestInterfaceNoAdopt
```

### Host runs

First attempt (02:23, all nine packages in one `go test`, i.e. in parallel): VPP crashed (NRestarts 3 → 4) —
see Q9; I ran it that way, the crash came during my run. Re-run one package at a time, NRestarts checked around each:

```
$ for p in policer qos span lldp bfd mpls lb vrrp igmp; do NRestarts before; VRX_INTEGRATION=1 go test -count=1 -p 1 -v -run OnHost ./internal/descriptors/$p/; NRestarts after; done
== policer NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestPolicerOnHost (0.05s)  (bind: skip, no workers)
== qos     NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestQoSOnHost (0.13s)
== span    NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestSpanOnHost (0.07s)
== lldp    NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestLLDPOnHost (0.02s)   (global: opt-in skip)
== bfd     NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestBFDOnHost (0.07s)    (echo-source: opt-in skip)
== mpls    NRestarts before=4 … rc=0 NRestarts after=4   --- PASS: TestMPLSOnHost (0.20s)   (table 0: opt-in skip)
== lb      NRestarts before=4 … rc=0 NRestarts after=4   --- SKIP: TestLBOnHost  (VRX_DF7_LB=1 to opt in)
== vrrp    NRestarts before=4 … rc=0 NRestarts after=4   --- SKIP: TestVRRPOnHost (VRX_DF7_VRRP_HOST=1, D-087)
== igmp    NRestarts before=4 … rc=0 NRestarts after=4   --- SKIP: TestIGMPOnHost (VRX_DF7_IGMP_HOST=1, D-087)
$ vppctl show interface | grep -c 'loop10[0-9][0-9]'; vppctl show ip fib | grep -c '10\.10\.'; vppctl show lb vips
0
0
(empty)
```

### CI gate

```
$ tools/ci.sh --base main        (clean tree at 0ccb946)
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m27s
  apps/agent: make lint test build                   0m28s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m34s · logs /root/ngfw-wt/logs/ci/DF-7-20260924-024045-2138322

CI GATE PASSED
```
(warning only: non-conventional subjects of earlier merge/review commits.)
