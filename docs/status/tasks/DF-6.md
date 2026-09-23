# DF-6 — Descriptors: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (SRv6 + SR-MPLS), lisp

Branch `task/DF-6` · worktree `/root/ngfw-wt/DF-6` · slot 11 (`w11`, tables/VNIs/TEIDs/labels 11000–11999,
addresses 10.11.0.0/16 + fd11::/16) · WBS D6.6, D6.7, D6.8, D2.8 (SR part).

## What was built

Numbering scheme (host tests): tunnel instances `11<NN>` (gre/ipip/vxlan 1101…), names `w11-*`, tables `11000+i`
(`h.Table(i)`), VNIs 11100–11201, TEIDs 11300–11401, PPPoE session ids 11500+, L2TP session ids 11400+, SR-MPLS labels
11600–11800, LISP VNI 11100, SIDs/BSIDs `fd11:<net>::<host>`, loopbacks `loop11<NN>`.

35 object types in 10 packages under `apps/agent/internal/descriptors/` (+ shared `df6/`, `df6/df6test/`), each with
KeyOf/Dependencies/Create/Update(or ErrRecreate)/Delete/Retrieve, a `Register`, unit tests on a stateful fake and a host
check. Tables per plugin in `docs/agent/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,sr,sr_mpls,lisp,df6}.md`.

| Plugin | Object types (descriptor names) | Retrieve |
|---|---|---|
| gre | `gre.tunnel` (L3/TEB/ERSPAN, p2p/p2mp) | full |
| ipip | `ipip.tunnel`, `ipip.sixrd` | full / write-only (dump lacks 6RD prefixes) |
| vxlan | `vxlan.tunnel`, `vxlan.bypass` | full / write-only |
| vxlan_gpe | `vxlan-gpe.tunnel`, `vxlan-gpe.bypass` | full / write-only |
| gtpu | `gtpu.tunnel` (+ tteid in-place update), `gtpu.forward`, `gtpu.bypass` | full / full / write-only |
| l2tp | `l2tp.tunnel` (+ cookies in-place update; **no delete in VPP**), `l2tp.interface-enable`, `l2tp.lookup-key` | full / write-only / write-only |
| pppoe | `pppoe.session`, `pppoe.cp` | full / write-only |
| sr | `sr.localsid`, `sr.policy`, `sr.steering`, `sr.encap-source`, `sr.encap-hop-limit` | full ×3 / write-only ×2 |
| sr_mpls | `sr-mpls.policy`, `sr-mpls.steering`, `sr-mpls.endpoint-color` | partial: write-only (no dump; see Q5) |
| lisp | `lisp.enable`, `lisp-gpe.enable`, `lisp.locator-set`, `lisp.locator`, `lisp.local-eid`, `lisp.map-resolver`, `lisp.map-server`, `lisp.remote-mapping`, `lisp.adjacency`, `lisp.eid-table-map`, `lisp.pitr`, `lisp-gpe.fwd-entry` | full ×11 / write-only (V9) |

Task-prompt names folded: `gtpu-tteid` → `gtpu.tunnel` Update (`gtpu_tunnel_update_tteid`); `l2tpv3-cookies` →
`l2tp.tunnel` Update (`l2tpv3_set_tunnel_cookies`). `srv6-mobile` not built (Q6).

Shared machinery (`df6`): IfDescriptor (tagged interfaces), Bypass/Toggle, Singleton, Keyed (untagged objects with a
Scope), `RequireTable`, typed errors; **no descriptor ever sends a delete for an object VPP no longer has**, and
referenced tables are checked before sending (VPP 26.06 crashes or leaks otherwise — V8/Q1, Q7).

## How it was verified (real output)

### Unit tests (fakes), `cd apps/agent && go test -count=1 ./internal/descriptors/...`
```
?   	ngfw/agent/internal/descriptors/df6	[no test files]
?   	ngfw/agent/internal/descriptors/df6/df6test	[no test files]
ok  	ngfw/agent/internal/descriptors/gre	0.026s
ok  	ngfw/agent/internal/descriptors/gtpu	0.017s
ok  	ngfw/agent/internal/descriptors/ipip	0.021s
ok  	ngfw/agent/internal/descriptors/l2tp	0.020s
ok  	ngfw/agent/internal/descriptors/lisp	0.025s
ok  	ngfw/agent/internal/descriptors/pppoe	0.016s
ok  	ngfw/agent/internal/descriptors/sr	0.026s
ok  	ngfw/agent/internal/descriptors/sr_mpls	0.021s
ok  	ngfw/agent/internal/descriptors/vxlan	0.028s
ok  	ngfw/agent/internal/descriptors/vxlan_gpe	0.021s
```

### No shell-outs
```
$ grep -rn "vppctl\|exec.Command" internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,sr,sr_mpls,lisp}
(no output, exit 1)
```

### Host VPP (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w11 VRX_SLOT=11 VRX_VPP_TABLE_BASE=11000 VRX_DF6_HOLD=6`, shared lab lock)
Each package's `OnHost` tests were run one package at a time; `systemctl show vpp -p NRestarts` stayed `2` before and
after every run (the 2 restarts are the gtpu incident, Q1). gtpu with `VRX_DF6_GTPU_HOST=1`, lisp with `VRX_DF6_LISP_HOST=1`.
"re-apply plan … empty=true" is the idempotency check: Retrieve after apply diffed against the same desired state
(`df6test.PlanFor`, scheduler semantics) — applying the same desired state twice yields an empty plan.
```
### gre
    tunnel_integration_test.go:36: re-apply plan gre.tunnel: create=0 update=0 delete=0 (empty=true)
    tunnel_integration_test.go:58: retrieved 4 gre tunnels: [{gre.tunnel/gre1104 instance:1104  mode:MP  src:"10.11.1.1" {2}} {gre.tunnel/gre1103 instance:1103  type:ERSPAN  src:"10.11.1.1"  dst:"10.11.1.4"  session_id:5 {7}} {gre.tunnel/gre1102 instance:1102 
--- PASS: TestTunnelOnHost (6.03s)
### ipip
    ipip_integration_test.go:42: re-apply plan ipip.tunnel: create=0 update=0 delete=0 (empty=true)
    ipip_integration_test.go:64: retrieved 3 ipip tunnels: [{ipip.tunnel/ipip1103 instance:1103  src:"10.11.2.1"  mode:MP {7}} {ipip.tunnel/ipip1102 instance:1102  src:"10.11.2.1"  dst:"10.11.2.3"  table_id:11002  flags:4 {13}} {ipip.tunnel/ipip1101 instance:1
--- PASS: TestTunnelOnHost (6.04s)
### vxlan
    vxlan_integration_test.go:43: re-apply plan vxlan.tunnel: create=0 update=0 delete=0 (empty=true)
    vxlan_integration_test.go:65: retrieved 3 vxlan tunnels: [{vxlan.tunnel/vxlan_tunnel1101 instance:1101  src:"10.11.3.1"  dst:"10.11.3.2"  vni:11100 {4}} {vxlan.tunnel/vxlan_tunnel1102 instance:1102  src:"10.11.3.1"  dst:"10.11.3.3"  vni:11101  encap_vrf_id
--- PASS: TestTunnelOnHost (6.04s)
### vxlan_gpe
    vxlan_gpe_integration_test.go:40: re-apply plan vxlan-gpe.tunnel: create=0 update=0 delete=0 (empty=true)
    vxlan_gpe_integration_test.go:62: retrieved 2 vxlan-gpe tunnels: [{vxlan-gpe.tunnel/w11-gpe1 name:"w11-gpe1" local:"10.11.5.1" remote:"10.11.5.2" vni:11200 protocol:IP4 encap_vrf_id:11005 decap_vrf_id:11005 {10}} {vxlan-gpe.tunnel/w11-gpe2 name:"w11-gpe2" 
--- PASS: TestTunnelOnHost (6.03s)
### gtpu
    gtpu_integration_test.go:56: re-apply plan gtpu.tunnel: create=0 update=0 delete=0 (empty=true)
    gtpu_integration_test.go:78: retrieved 2 gtpu tunnels: [{gtpu.tunnel/w11-gtpu2 name:"w11-gtpu2" src:"10.11.4.1" dst:"10.11.4.3" encap_vrf_id:11004 decap_next:IP4 teid:11301 pdu_extension:true qfi:5 {1}} {gtpu.tunnel/w11-gtpu1 name:"w11-gtpu1" src:"10.11.4.
    gtpu_integration_test.go:86: retrieved forward: [{gtpu.forward/w11-gtpufw name:"w11-gtpufw" dst:"10.11.4.9" forwarding_type:2 encap_vrf_id:11004 decap_next:L2 {6}}]
--- PASS: TestTunnelOnHost (6.04s)
### l2tp
--- PASS: TestL2tpOnHost (0.02s)
    l2tp_integration_test.go:42: l2tpv3 tunnels of w11 before: 0
    l2tp_integration_test.go:44: l2tp.tunnel create skipped: VPP has no l2tpv3 delete, a created tunnel would outlive the test on the shared host (set VRX_DF6_L2TP_CREATE=1 to opt in)
--- SKIP: TestL2tpTunnelOnHost (0.02s)
### pppoe
--- PASS: TestCpOnHost (6.02s)
--- SKIP: TestSessionOnHost (0.01s)
### sr-localsid
    sr_integration_test.go:72: re-apply plan sr.localsid: create=0 update=0 delete=0 (empty=true)
    sr_integration_test.go:72: retrieved 5 sr.localsid: [{sr.localsid/fd11:51::5 sid:"fd11:51::5"  behavior:END_DT6  lookup_table:11009 <nil>} {sr.localsid/fd11:51::4 sid:"fd11:51::4"  behavior:END_DT4  lookup_table:11008 <nil>} {sr.localsid/fd11:51::3 sid:"fd
--- PASS: TestLocalSidOnHost (6.04s)
### sr-policy
    sr_integration_test.go:87: re-apply plan sr.policy: create=0 update=0 delete=0 (empty=true)
    sr_integration_test.go:87: retrieved 2 sr.policy: [{sr.policy/fd11:b::2 bsid:"fd11:b::2"  type:SPRAY  fib_table:11010  sid_lists:{sids:"fd11:54::1"  weight:1} <nil>} {sr.policy/fd11:b::1 bsid:"fd11:b::1"  encap:true  sid_lists:{sids:"fd11:51::1"  sids:"fd1
--- PASS: TestPolicyOnHost (6.02s)
### sr-steering
    sr_integration_test.go:105: re-apply plan sr.steering: create=0 update=0 delete=0 (empty=true)
    sr_integration_test.go:105: retrieved 2 sr.steering: [{sr.steering/ipv6/0/fd11:12::/64 traffic_type:IPV6  prefix:"fd11:12::/64"  bsid:"fd11:b::3" <nil>} {sr.steering/ipv4/11011/10.11.12.0/24 traffic_type:IPV4  prefix:"10.11.12.0/24"  table_id:11011  bsid:"
--- PASS: TestSteeringOnHost (6.04s)
### sr_mpls
    sr_mpls_integration_test.go:58: sr-mpls policy 11600 + steering 10.11.13.0/24/11012 created, observed, deleted
--- PASS: TestPolicySteeringOnHost (6.03s)
--- SKIP: TestEndpointColorOnHost (0.00s)
### lisp
    lisp_integration_test.go:33: before: lisp=false gpe=false
    lisp_integration_test.go:87: observed lisp.locator-set/w11-ls1: name:"w11-ls1"
    lisp_integration_test.go:89: re-apply plan lisp.locator-set: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.locator/w11-ls1/loop1114: locator_set:"w11-ls1"  interface:"loop1114"  priority:1  weight:10
    lisp_integration_test.go:89: re-apply plan lisp.locator: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.eid-table-map/l3/11100: vni:11100  dp_table:11013
    lisp_integration_test.go:89: re-apply plan lisp.eid-table-map: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.local-eid/11100/10.11.14.0/24: vni:11100  eid:"10.11.14.0/24"  locator_set:"w11-ls1"
    lisp_integration_test.go:89: re-apply plan lisp.local-eid: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.map-resolver/10.11.14.100: address:"10.11.14.100"
    lisp_integration_test.go:89: re-apply plan lisp.map-resolver: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.map-server/10.11.14.101: address:"10.11.14.101"
    lisp_integration_test.go:89: re-apply plan lisp.map-server: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.remote-mapping/11100/10.11.15.0/24: vni:11100  eid:"10.11.15.0/24"  rlocs:{address:"10.11.14.2"  priority:1  weight:1}
    lisp_integration_test.go:89: re-apply plan lisp.remote-mapping: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp.adjacency/11100/10.11.15.0/24/10.11.14.0/24: vni:11100  reid:"10.11.15.0/24"  leid:"10.11.14.0/24"
    lisp_integration_test.go:89: re-apply plan lisp.adjacency: create=0 update=0 delete=0 (empty=true)
    lisp_integration_test.go:87: observed lisp-gpe.fwd-entry/11100/10.11.17.0/24/10.11.14.0/24: vni:11100  dp_table:11013  reid:"10.11.17.0/24"  leid:"10.11.14.0/24"  pairs:{local:"10.11.14.1"  remote:"10.11.14.9"  weight:1}
--- PASS: TestLISPOnHost (6.06s)
```

### `vppctl show …` while the prefixed objects exist, then after delete
```
$ vppctl show gre tunnel (objects present)
[0] instance 1104 src 10.11.1.1 dst 0.0.0.0 fib-idx 0 sw-if-idx 2 payload L3 multi-point 
[1] instance 1103 src 10.11.1.1 dst 10.11.1.4 fib-idx 0 sw-if-idx 7 payload ERSPAN point-to-point session 5 l2-adj-idx 10 
[2] instance 1102 src 10.11.1.1 dst 10.11.1.3 fib-idx 2 sw-if-idx 16 payload TEB point-to-point l2-adj-idx 3 
[3] instance 1101 src 10.11.1.1 dst 10.11.1.2 fib-idx 0 sw-if-idx 13 payload L3 point-to-point 
$ vppctl show gre tunnel (after delete)
No GRE tunnels configured...

$ vppctl show ipip tunnel (objects present)
[0] 6rd src 10.11.2.1 ip6-pfx fd11:6d::/32 table-ID 0 sw-if-idx 5 flags [none] dscp CS0
[1] instance 1103 p2mp src 10.11.2.1 table-ID 0 sw-if-idx 7 flags [none] dscp CS0
[2] instance 1102 src 10.11.2.1 dst 10.11.2.3 table-ID 11002 sw-if-idx 13 flags [encap-copy-dscp ] dscp CS0
[3] instance 1101 src 10.11.2.1 dst 10.11.2.2 table-ID 0 sw-if-idx 2 flags [none] dscp EF
$ vppctl show ipip tunnel (after delete)
No IPIP tunnels configured...

$ vppctl show vxlan tunnel (objects present)
[0] instance 1101 src 10.11.3.1 dst 10.11.3.2 src_port 4789 dst_port 4789 vni 11100 fib-idx 0 sw-if-idx 4 encap-dpo-idx 6 
[1] instance 1102 src 10.11.3.1 dst 10.11.3.3 src_port 14789 dst_port 14789 vni 11101 fib-idx 3 sw-if-idx 3 encap-dpo-idx 0 
[2] instance 1103 src 10.11.3.1 dst 239.11.11.11 src_port 4789 dst_port 4789 vni 11102 fib-idx 0 sw-if-idx 10 encap-dpo-idx 10 mcast-sw-if-idx 7 
$ vppctl show vxlan tunnel (after delete)
No vxlan tunnels configured...

$ vppctl show vxlan-gpe (objects present)
[0] lcl 10.11.5.1 rmt 10.11.5.2 lcl_port 4790 rmt_port 4790 vni 11200 fib-idx 3 sw-if-idx 10 decap-next-protocol ip4 fib-idx 3 
[1] lcl 10.11.5.1 rmt 10.11.5.3 lcl_port 14790 rmt_port 14790 vni 11201 fib-idx 3 sw-if-idx 3 decap-next-protocol ethernet 
$ vppctl show vxlan-gpe (after delete)
No vxlan-gpe tunnels configured.

$ vppctl show gtpu tunnel (objects present)
[0] src 10.11.4.1 dst 10.11.4.3 teid 11301 tteid 11301 encap-vrf-id 11004 sw-if-idx 1 encap-dpo-idx 16 decap-next-ip4 pdu-enabled qfi 5 
[1] src 10.11.4.1 dst 10.11.4.2 teid 11300 tteid 11400 encap-vrf-id 0 sw-if-idx 8 encap-dpo-idx 37 decap-next-l2 pdu-disabled 
[2] src 10.11.4.9 dst 127.0.0.128 teid 0 tteid 0 encap-vrf-id 11004 sw-if-idx 6 encap-dpo-idx 26 decap-next-l2 forwarding unknown-teid 
$ vppctl show gtpu tunnel (after delete)
No gtpu tunnels configured...

$ vppctl show sr localsids (objects present)
SRv6 - My LocalSID Table:
=========================
	Address: 	fd11:51::5/128
	Behavior: 	DT6 (Endpoint with decapsulation and specific IPv6 table lookup)
	Table: 11009
	Good traffic: 	[0 packets : 0 bytes]
	Bad traffic:  	[0 packets : 0 bytes]
--------------------
	Address: 	fd11:51::4/128
	Behavior: 	DT4 (Endpoint with decapsulation and specific IPv4 table lookup)
	Table: 	11008
	Good traffic: 	[0 packets : 0 bytes]
	Bad traffic:  	[0 packets : 0 bytes]
--------------------
	Address: 	fd11:51::3/128
	Behavior: 	DX6 (Endpoint with decapsulation and IPv6 cross-connect)
	Iface:  	loop1108
	Next hop: 	fd11:8::3
	Good traffic: 	[0 packets : 0 bytes]
	Bad traffic:  	[0 packets : 0 bytes]
--------------------
	Address: 	fd11:51::2/128
	Behavior: 	X (Endpoint with Layer-3 cross-connect)
	Iface:  	loop1108
	Next hop: 	fd11:8::2
	Good traffic: 	[0 packets : 0 bytes]
	Bad traffic:  	[0 packets : 0 bytes]
--------------------
	Address: 	fd11:51::1/128
	Behavior: 	End
	PSP: 	True
	Good traffic: 	[0 packets : 0 bytes]
	Bad traffic:  	[0 packets : 0 bytes]
--------------------
$ vppctl show sr localsids (after delete)
SRv6 - My LocalSID Table:
=========================

$ vppctl show sr policies (objects present)
SR policies:
[0].-	BSID: fd11:b::2
	Behavior: SRH insertion
	Type: Spray
	FIB table: 11010
	Segment Lists:
  	[0].- < fd11:54::1,  > weight: 1
-----------
[1].-	BSID: fd11:b::1
	Behavior: Encapsulation
	EncapSrcIP: fd11:a::1
	Type: Default
	FIB table: 0
	Segment Lists:
  	[2].- < fd11:51::1, fd11:52::1,  > weight: 1
  	[1].- < fd11:53::1,  > weight: 5
-----------
$ vppctl show sr policies (after delete)
SR policies:

$ vppctl show sr steering-policies (objects present)
SR steering policies:
Traffic		SR policy BSID
L3 fd11:12::/64	fd11:b::3
L3 10.11.12.0/24	fd11:b::3
$ vppctl show sr steering-policies (after delete)
SR steering policies:
Traffic		SR policy BSID

$ vppctl show sr mpls policies (objects present)
SR MPLS policies:
[0].-	BSID: 11600
	TE disabled
	Type: Default
	Segment Lists:
  	[0].- < 11700, 11701,  > 
-----------
$ vppctl show sr mpls policies (after delete)
SR MPLS policies:

$ vppctl show lisp status
feature: enabled
gpe: enabled
$ vppctl show lisp locator-set
Locator-set     Locator        Priority         Weight     
w11-ls1               4               1              10
<remote-1>      10.11.14.2               1               1
<remote-2>      10.11.14.2               1               1
<remote-3>      10.11.14.2               1               1
<remote-4>      10.11.14.2               1               1
<remote-5>      10.11.14.2               1               1
$ vppctl show lisp eid-table
$ vppctl show lisp eid-table map l3
    VNI       VRF   
     0         0    
   11100     11013  
$ vppctl show lisp adjacencies vni 11100
leid                                     reid
[11100] 10.11.14.0/24                    [11100] 10.11.15.0/24
$ vppctl show lisp map-resolvers
10.11.14.100
$ vppctl show gpe entry
VNI:11100 VRF:11013 EID: 10.11.14.0/24 -> 10.11.17.0/24  [index:0]
 via:
  weight:1 adj:[ vni: 11100, remote-RLOC: 10.11.14.9, LISP L3 sub-interface index: 0, LISP tunnel index: 0]
VNI:11100 VRF:11013 EID: 0.0.0.0/0 -> 10.11.15.0/24  [index:1]
 via:
  weight:1 adj:[ vni: 11100, remote-RLOC: 10.11.14.2, LISP L3 sub-interface index: 0, LISP tunnel index: 1]
$ vppctl show lisp status
feature: disabled
gpe: disabled
$ vppctl show lisp locator-set
Locator-set     Locator        Priority         Weight     
<remote-1>      10.11.14.2               1               1
<remote-2>      10.11.14.2               1               1
<remote-3>      10.11.14.2               1               1
<remote-4>      10.11.14.2               1               1
<remote-5>      10.11.14.2               1               1
$ vppctl show lisp eid-table

```
(`show lisp eid-table` prints nothing on this VPP even while EIDs exist; the EIDs are shown by the Retrieve output
above and `show lisp adjacencies` / `show gpe entry`. The `<remote-N>` locator sets are VPP's leak, Q9. The
sr-mpls snapshot was taken right after `sr_mpls_policy_add`, before the second list's `sr_mpls_policy_mod`.)

### CI gate
See the end of this file.

## Out of scope / left undone
- API, UI, schema, F-*/P11 wiring, scheduler (P05) — none built. No descriptor registry list exists yet to add to;
  `Register` per package is the entry point (signatures in Q10).
- `srv6-mobile`, SRv6 proxy behaviours (binapi not generated), uSID behaviours, SR path tracing, color-based SR-MPLS
  steering, LISP map-register HMAC keys, LISP control-plane semantics beyond the messages listed, NSH EIDs, iOAM,
  hardware offload (`vxlan_offload_rx`, `gtpu_offload_rx`), PPPoE daemons.
- Host coverage gaps (all with reasons in the tests): `l2tp.tunnel` create (no delete; opt-in
  `VRX_DF6_L2TP_CREATE=1`), `pppoe.session` create (needs PPPoE discovery traffic), `sr-mpls.endpoint-color` (global
  side effects, no un-assign), `lisp.pitr` (changes the global LISP mode), singleton globals (encap source/hop limit,
  l2tp lookup key: no getter, never changed on the shared host).

## Open questions — `docs/status/tasks/DF-6-questions.md`
Q1 V8 gtpu crash (incident, guarded) · Q2 l2tp no delete · Q3 pppoe needs discovery · Q4 write-only policy for P05 ·
Q5 SR-MPLS no dump · Q6 srv6 proxies/mobile · Q7 IP-table-delete leak (incident, repaired) · Q8 V9 GPE path details
msg id · Q9 LISP leftovers (`<remote-N>` sets, `lisp_gpe*` interfaces) · Q10 Register signatures.

## Decisions taken (for the LOG)
1. Write-only descriptors return `df6.ErrRetrieveUnsupported` rather than cached desired state (options: cache /
   partial decode / write-only) — write-only is the only honest option; the reconciler policy is P05's (Q4).
2. Deletes are idempotent and pre-checked everywhere (options: send and map errors / pre-check) — pre-check, because
   VPP 26.06 handlers crash or leak on failed deletes (V8, unchecked `fib_table_find`).
3. SR policy `encap_src` mandatory for encap, forbidden for insert (options: allow empty + global dependency / require)
   — required, because VPP substitutes the unreadable global and the diff would never converge.
4. SR-MPLS segment lists and LISP RLOCs / GPE pairs have a canonical sorted order (VPP does not keep configured order).
5. gtpu / lisp host tests opt-in (manager rule after the gtpu incident; LISP toggles a global).
6. `srv6-mobile` not built (T3; no delete + ownership clash with `sr.policy`) — questions file instead.

## CI gate — `tools/ci.sh --base main` on commit 4e8b26c (the code tree of this report; this file is docs-only on top)
```
== VRX CI gate: quick ==
== contract guard: HEAD vs main ==
== tools (golangci-lint, gitleaks) ==
== install (pnpm --frozen-lockfile --prefer-offline) ==
== generate + generated-output gate ==
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~499525 bytes (499.52 KB) in 1.25s no leaks found 
== lint · typecheck · unit tests · build (turbo) ==
== apps/agent: make lint test build ==
ok  	ngfw/agent/internal/agent	1.153s
ok  	ngfw/agent/internal/descriptors/gre	1.096s
ok  	ngfw/agent/internal/descriptors/gtpu	1.090s
ok  	ngfw/agent/internal/descriptors/ipip	1.101s
ok  	ngfw/agent/internal/descriptors/l2tp	1.086s
ok  	ngfw/agent/internal/descriptors/lisp	1.122s
ok  	ngfw/agent/internal/descriptors/pppoe	1.088s
ok  	ngfw/agent/internal/descriptors/sr	1.099s
ok  	ngfw/agent/internal/descriptors/sr_mpls	1.089s
ok  	ngfw/agent/internal/descriptors/vxlan	1.103s
ok  	ngfw/agent/internal/descriptors/vxlan_gpe	1.089s
ok  	ngfw/agent/internal/renderers	1.426s; 
== test/ Go modules, unit mode (test/integration/smoke) ==
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot
== summary (quick) ==
CI GATE PASSED
```
