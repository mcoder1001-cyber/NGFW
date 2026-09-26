# F-vlan-qinq — 802.1Q sub-interfaces and QinQ (802.1ad)

Branch `task/F-vlan-qinq` (worktree `/root/ngfw-wt/F-vlan-qinq`), slot 5 (`w5`, API 3500, web 5500, metrics 9151, DB
`vrx_w5`, rig 10.5.{1,2}.0/24; fix round 1 on slot 12 `w12`, D-128 — see "Fix round 1" at the end), data path
**af_packet** (D-010). Base: `task/W-seed`@8b7558e (speculative, D-114/D-120);
`task/W-seed`@df67a8e (TD-5 rings/quiesce, D-113 rig, P08 fix round 2) merged at 18:12 on the manager's A1 instruction,
before any host run. Worked 17:27–19:15; fix round 1 19:38–19:50 and (after a usage-limit stop at 20:57) 22:41–23:05.

## What was built

| area | what | files |
|---|---|---|
| schema | no rule was missing: 9 QinQ cases pass against the existing `SubinterfaceSchema`, `interfaces.vlan-unique`, `interfaces.subinterface-mtu` | `packages/schema/src/semantic/interfaces-qinq.test.ts` (new) |
| agent | **defect found and fixed** (Q1): removing an enabled sub-interface while its parent stays (a rollback, or *remove* + commit in the drawer) ended `ROLLED_BACK` — the delete plan had no edge from the attributes to the sub-interface and deleted `delete_subif` first. Fix: `SubinterfaceDescriptor.ProvidedKeys` = `interface/<parent>.<id>`. QinQ builder/assembler cases on the fake VPP | `apps/agent/internal/descriptors/interface/subinterface.go` (defect hunk), `apps/agent/internal/desired/interfaces_qinq_test.go` (new) |
| API | nothing new (as the prompt says); e2e proves `/state/interfaces` rows with `parent`, `vlanId`, `innerVlanId`, `dot1ad`, the duplicate-stack 400 and the rollback | `apps/api/test/e2e/vlan-qinq.e2e.test.ts` (new) |
| UI | `SubinterfaceTable` extracted from P08's drawer: Encapsulation (`dot1q 100` / `dot1ad 200 · dot1q 100`), Inner VLAN, live Admin/Link, Addresses; a live stack that differs from the candidate is shown under it; exact-match caption; en + fa (`vlan-qinq` namespace); tag-stack formatter + table Vitests | `apps/web/src/domains/interfaces/subinterfaces/**`, `apps/web/src/locales/{en,fa}/vlan-qinq.json` (new) |
| topology | `test/topology/vlan-qinq` (own module, P08 pattern): commit via API on `host-w5w0`, duplicate 400, VPP API rows, Retrieve == desired, vppctl, CLI, opt-in packets, restart simulation, rollback, cleanup; screenshot run | `test/topology/vlan-qinq/**` (new) |
| docs | user page with REST + real CLI output; V-new (af_packet loses the 802.1ad TPID) | `docs/user/interfaces/vlan-qinq.md`, `docs/user/interfaces/img/vlan-qinq-*.png` (new), `docs/vpp-code-track.md` (appended) |

### Shared hunks (hotspots; each is append-only / one region)
| id | file | hunk |
|---|---|---|
| W5 | `apps/web/src/domains/interfaces/InterfaceDrawer.tsx` | the sub-interface block (title + add button + error + `<Table>`, old lines 269–323) replaced by `<SubinterfaceTable … />` (13 lines); imports: `AddIcon` and `TableHead` removed (now unused here), one `import { SubinterfaceTable }` line after `./Sparkline` |
| W3 | `apps/web/src/i18n.ts` | 5 inserted lines, each group directly under its `// wave-A: F-vlan-qinq` anchor: 2 imports, `'vlan-qinq'` in `NAMESPACES`, `'vlan-qinq': enVlanQinq` (en) and `faVlanQinq` (fa) |
| D1 | `docs/user/interfaces/basics.md` | line 28 "QinQ is not supported by this release." → a link to `vlan-qinq.md` (line 102 "Not in this release" left to the manager) |
| A3-defect | `apps/agent/internal/descriptors/interface/subinterface.go` | `ProvidedKeys` (+15 lines), proving test `TestQinQDeleteWhileParentStays` (fails without it, below). `desired/interfaces.go` untouched |
| A7 | `docs/vpp-code-track.md` | appended `### V-new (F-vlan-qinq)` (00-CONTEXT FAST MODE rule for VPP-code items; the manager numbers it) |
| — | `docs/user/interfaces/img/vlan-qinq-*.png` | 5 new images next to my page (not in the owned list; new files, no conflict) |

## Acceptance — evidence

- [x] **Through the API on `host-w5w0`: `.100` = dot1q 100, `.200` = dot1ad 200 + dot1q 100** (plus `.300` = dot1q 300 +
      dot1q 30), each with an address; `Retrieve()` == desired; VPP shows both tag stacks and addresses — run 1 below
- [x] **Agent-restart simulation** (agent stopped, the sub-interfaces deleted via binapi, addresses first) → all back with
      their addresses **1.46 s** after the agent start; reconcile 1.332 s by agent log timestamps
- [x] **Rollback deletes the sub-interfaces** — results: each sub-interface's admin state and address before it (the Q1 fix
      on the real VPP); Retrieve has no sub-interface; `sw_interface_dump`/`vppctl show interface` have no `host-w5w0.<id>`
- [x] **Duplicate `(dot1ad, vlanId, innerVlanId)` → 400 problem+json with the pointer to the second entry** — real stack
      (run 1) and API e2e
- [x] **UI screenshots en + fa** of the drawer with the QinQ row — below (headless Chrome, not Playwright)
- [x] **`tools/ci.sh --base main` green** — below (after re-runs of the flaky contract guard, Q6)
- Packet level (optional): dot1q and dot1q-in-dot1q answered pings through VPP (run 1); dot1ad cannot work on the
  af_packet path (VPP bug, V-new). **VPP crashed once during my second run's packet phase (Q0): the trigger was the
  test's own `vppctl show trace`** (NULL formatter of a recycled interface tx node, review H1, D-128), not the frames.
  Fix round 1 removed every packet-trace call; the opt-in phase (`VRX_QINQ_PACKETS=1`, D-126/D-128) proves the path with
  ping + per-sub-interface counter deltas and sends no 802.1ad frame (lab limit, V-new).

### Host run 1 — `test/topology/vlan-qinq/run.sh -run TestVlanQinqTopology` (18:20, NRestarts 0 → 0, VPP pid 8760)
Historical evidence: this run used the first test version, which still called `vppctl show trace` (banned since, D-128;
the trace blocks below are kept as recorded). The committed test's run is under "Fix round 1".
Rig print lines, repeated `/state` fields and trace middles trimmed (`…`); nothing else edited.
```
NRestarts before: 0 pid 8760 18:20:27.519141885
=== RUN   TestVlanQinqTopology
    qinq_test.go:273: systemctl show vpp -p NRestarts (before) = 0
    qinq_test.go:273: rig up w5 (slot 5, path af_packet)          (…)
    qinq_test.go:273: rig wan side handed to the agent: sw_interface_add_del_address sw_if_index=7 (host-w5w0) del_all=true → ok
    qinq_test.go:273: rig wan side handed to the agent: af_packet_delete host_if_name=w5w0 (tag "") → ok
    qinq_test.go:273: started vrx-agent pid 1872471 (log /run/vrx-test/w5/qinq/agent.log)
    qinq_test.go:273: started vrx-api pid 1872554 (log /run/vrx-test/w5/qinq/api.log)
=== RUN   TestVlanQinqTopology/commit
    qinq_test.go:291: commit parent → applied revision 1
    qinq_test.go:300: commit with a duplicate tag stack → 400 content-type problem+json; body {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic","warnings":[],"detail":"semantic validation failed","instance":"/api/v1/config/commit","errors":[{"pointer":"/interfaces/host-w5w0/subinterfaces/201/vlanId","message":"VLAN dot1ad 200.100 is already used by sub-interface host-w5w0.200"}]}
    qinq_test.go:308: candidate diff: {"baseRevision":1,"changes":[{"op":"add","pointer":"/interfaces/host-w5w0/subinterfaces/100","to":{"vrf":"default","ipv4":["10.5.100.1/24"],"ipv6":[],"dot1ad":false,"vlanId":100,"enabled":true,"description":"dot1q 100"}},{"op":"add","pointer":"/interfaces/host-w5w0/subinterfaces/200","to":{"vrf":"default","ipv4":["10.5.200.1/24"],"ipv6":[],"dot1ad":true,"vlanId":200,"enabled":true,"description":"QinQ dot1ad 200 + dot1q 100","innerVlanId":100}},{"op":"add","pointer":"/interfaces/host-w5w0/subinterfaces/300","to":{…,"dot1ad":false,"vlanId":300,…,"innerVlanId":30}}]}
    qinq_test.go:310: commit sub-interfaces → applied revision 2 txn 589e0014-27e0-41dd-8fb1-cd025caeeb8e
    qinq_test.go:311: results (12, in order) summary={"created":12,"deleted":0,"failed":0,"reverted":0,"unchanged":4,"updated":0}:
           0 create ok   interface.subinterface/host-w5w0.100
           1 create ok   interface.subinterface/host-w5w0.200
           2 create ok   interface.subinterface/host-w5w0.300
           3 create ok   interface/host-w5w0.100
           4 create ok   interface-ip/host-w5w0.100/10.5.100.1/24
           5 create ok   interface.admin-state/host-w5w0.100
           6 create ok   interface/host-w5w0.200
           7 create ok   interface-ip/host-w5w0.200/10.5.200.1/24
           8 create ok   interface.admin-state/host-w5w0.200
           …  (the same three for .300)
    qinq_test.go:318: sw_interface_dump host-w5w0.100: sw_if_index=3 sup=7 sub_id=100 sub_number_of_tags=1 sub_outer_vlan_id=100 sub_inner_vlan_id=0 sub_if_flags=ONE_TAG|EXACT_MATCH tag="w5:host-w5w0.100"
    qinq_test.go:318: sw_interface_dump host-w5w0.200: sw_if_index=5 sup=7 sub_id=200 sub_number_of_tags=2 sub_outer_vlan_id=200 sub_inner_vlan_id=100 sub_if_flags=TWO_TAGS|DOT1AD|EXACT_MATCH tag="w5:host-w5w0.200"
    qinq_test.go:318: sw_interface_dump host-w5w0.300: sw_if_index=1 sup=7 sub_id=300 sub_number_of_tags=2 sub_outer_vlan_id=300 sub_inner_vlan_id=30 sub_if_flags=TWO_TAGS|EXACT_MATCH tag="w5:host-w5w0.300"
    qinq_test.go:336: /state/interfaces host-w5w0.100: kind=subinterface parent=host-w5w0 state={"adminUp":true,…,"innerVlanId":0,"ipv4":["10.5.100.1/24"],…,"linkUp":true,…,"managed":true,…,"type":"sub-interface","vlanId":100,…} config(Retrieve)={"description":"dot1q 100","dot1ad":false,"enabled":true,"ipv4":["10.5.100.1/24"],"vlanId":100,"vrf":"default"} running={"description":"dot1q 100","dot1ad":false,"enabled":true,"ipv4":["10.5.100.1/24"],"ipv6":[],"vlanId":100,"vrf":"default"} hasPendingChange=false
    qinq_test.go:336: /state/interfaces host-w5w0.200: kind=subinterface parent=host-w5w0 state={"adminUp":true,"description":"QinQ dot1ad 200 + dot1q 100","innerVlanId":100,"ipv4":["10.5.200.1/24"],…,"linkUp":true,…,"managed":true,…,"parent":"host-w5w0",…,"swIfIndex":5,…,"type":"sub-interface","vlanId":200,…} config(Retrieve)={"description":"QinQ dot1ad 200 + dot1q 100","dot1ad":true,"enabled":true,"innerVlanId":100,"ipv4":["10.5.200.1/24"],"vlanId":200,"vrf":"default"} running={…same + "ipv6":[]} hasPendingChange=false
    qinq_test.go:336: /state/interfaces host-w5w0.300: … state={…"innerVlanId":30,…"vlanId":300…} config(Retrieve)={"description":"dot1q 300 + dot1q 30","dot1ad":false,"enabled":true,"innerVlanId":30,"ipv4":["10.5.30.1/24"],"vlanId":300,"vrf":"default"} …
    qinq_test.go:350: vppctl show interface host-w5w0.100 host-w5w0.200 host-w5w0.300:
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
        host-w5w0.100                     3      up           0/0/0/0
        host-w5w0.200                     5      up           0/0/0/0
        host-w5w0.300                     1      up           0/0/0/0
    qinq_test.go:351: vppctl show interface address:
        host-w5w0 (up):
          L3 10.5.2.1/24
        host-w5w0.100 (up):
          L3 10.5.100.1/24
        host-w5w0.200 (up):
          L3 10.5.200.1/24
        host-w5w0.300 (up):
          L3 10.5.30.1/24
=== RUN   TestVlanQinqTopology/packets
    qinq_test.go:362: V19 guard: classify_table_by_interface host-w5w0.200 sw_if_index=5 l2=~0 ip4=~0 ip6=~0
    qinq_test.go:362: V19 guard: acl_interface_list_dump host-w5w0.200: 0 ACLs          (… the same for host-w5w0, .100, .300)
    qinq_test.go:362: V19 guard: ipsec_spd_interface_dump: no SPD on the rig interfaces
    qinq_test.go:362: V19 guard: reset write-only ip4/ip6 classify table and l2 in/out tables of host-w5w0.200 to ~0   (… all four)
    qinq_test.go:364: vrx-vpp-preflight: exit <nil>
        V19 pre-flight ok: no classify binding or classify DPO points at a missing table (0 warning(s))
    qinq_test.go:367: netns ns-w5-wan: w5w1.100 (802.1Q 100) 10.5.100.2/24
    qinq_test.go:367: netns ns-w5-wan: w5w1.200.100 (802.1ad 200 + 802.1Q 100) 10.5.200.2/24
    qinq_test.go:367: netns ns-w5-wan: w5w1.300.30 (802.1Q 300 + 802.1Q 30) 10.5.30.2/24
    qinq_test.go:377: ping 10.5.100.1 over host-w5w0.100 (dot1q 100) from ns-w5-wan: ok=true
        3 packets transmitted, 3 received, 0% packet loss, time 605ms
    qinq_test.go:382: vppctl show trace (our echo request over host-w5w0.100):
        05:18:05:267748: af-packet-input
            tpacket3_hdr: … vlan 100 vlan_tpid 33024
        05:18:05:267808: ethernet-input
          IP4: 02:4f:57:07:18:8f -> 02:fe:52:b9:ec:0a 802.1q vlan 100
        05:18:05:267823: ip4-input
          ICMP: 10.5.100.2 -> 10.5.100.1
        …  ip4-lookup → ip4-receive → ip4-icmp-echo-request → ip4-load-balance
        05:18:05:267858: ip4-rewrite
          tx_sw_if_index 3 dpo-idx 12 : ipv4 via 10.5.100.2 host-w5w0.100: mtu:9000 next:26 flags:[] 024f5707188f02fe52b9ec0a810000640800 …
        05:18:05:267862: host-w5w0-output
          host-w5w0.100 flags 0x13180005
          IP4: 02:fe:52:b9:ec:0a -> 02:4f:57:07:18:8f 802.1q vlan 100
    qinq_test.go:377: ping 10.5.200.1 over host-w5w0.200 (QinQ dot1ad 200 + dot1q 100) from ns-w5-wan: ok=false
        3 packets transmitted, 0 received, +3 errors, 100% packet loss, time 614ms
    qinq_test.go:382: vppctl show trace (our echo request over host-w5w0.200):
        05:18:07:791417: af-packet-input
            tpacket3_hdr:
              status 0x20000051 len 46 snaplen 46 mac 82 net 96
              sec 0x6ab538fd nsec 0x2d7b9f3a vlan 200 vlan_tpid 34984
        05:18:07:801484: ethernet-input
          ARP: 02:4f:57:07:18:8f -> ff:ff:ff:ff:ff:ff 802.1q vlan 200 802.1q vlan 100
        05:18:07:801506: error-drop
          rx:host-w5w0
        05:18:07:801550: drop
          ethernet-input: unknown vlan
    qinq_test.go:377: ping 10.5.30.1 over host-w5w0.300 (dot1q 300 + dot1q 30) from ns-w5-wan: ok=true
        3 packets transmitted, 3 received, 0% packet loss, time 633ms
    qinq_test.go:382: vppctl show trace (our echo request over host-w5w0.300):
        05:18:10:148308: af-packet-input
            tpacket3_hdr: … vlan 300 vlan_tpid 33024
        05:18:10:148335: ethernet-input
          IP4: 02:4f:57:07:18:8f -> 02:fe:52:b9:ec:0a 802.1q vlan 300 802.1q vlan 30
        05:18:10:148346: ip4-input
          ICMP: 10.5.30.2 -> 10.5.30.1
        …
        05:18:10:148366: ip4-rewrite
          tx_sw_if_index 1 dpo-idx 13 : ipv4 via 10.5.30.2 host-w5w0.300: mtu:9000 next:26 flags:[] 024f5707188f02fe52b9ec0a8100012c8100001e0800 …
        05:18:10:148369: host-w5w0-output
          host-w5w0.300 flags 0x23180005
          IP4: 02:fe:52:b9:ec:0a -> 02:4f:57:07:18:8f 802.1q vlan 300 802.1q vlan 30
=== RUN   TestVlanQinqTopology/restart-safety
    stack_test.go:199: stopped vrx-agent pid 1872471
    qinq_test.go:399: simulated loss: sw_interface_add_del_address sw_if_index=3 (host-w5w0.100) del_all=true → ok
    qinq_test.go:399: simulated loss: sw_interface_add_del_address sw_if_index=5 (host-w5w0.200) del_all=true → ok
    qinq_test.go:399: simulated loss: sw_interface_add_del_address sw_if_index=1 (host-w5w0.300) del_all=true → ok
    qinq_test.go:399: simulated loss: delete_subif sw_if_index=3 (host-w5w0.100, tag "w5:host-w5w0.100") → ok
    qinq_test.go:399: simulated loss: delete_subif sw_if_index=5 (host-w5w0.200, tag "w5:host-w5w0.200") → ok
    qinq_test.go:399: simulated loss: delete_subif sw_if_index=1 (host-w5w0.300, tag "w5:host-w5w0.300") → ok
    qinq_test.go:407: sw_interface_dump after the loss: none of [host-w5w0.100 host-w5w0.200 host-w5w0.300] exists; host-w5w0 stays (sw_if_index 7)
    qinq_test.go:409: GET /state/interfaces while the agent is stopped → 503 {"type":"https://vrx.dev/problems/agent-unavailable","title":"Agent unavailable","status":503,"grpcCode":"UNAVAILABLE",…
    qinq_test.go:413: started vrx-agent pid 1876723 (log /run/vrx-test/w5/qinq/agent.log)
    qinq_test.go:428: agent started at +0s; all three sub-interfaces back in VPP with their addresses at +1.46s (no config API call)
    qinq_test.go:435: agent log: {"time":"2026-09-24T18:21:47.290456151+03:30","level":"INFO","msg":"vrx-agent starting","version":"dev","pid":1876723,"owner":"w5","socket":"/run/vrx-test/w5/agent.sock","vpp_api":"/run/vpp/api.sock"}
    qinq_test.go:440: agent log: {"time":"2026-09-24T18:21:47.623855557+03:30","level":"INFO","msg":"VPP boot identity","owner":"w5","component":"subsystems","identity":"a93c0e7a-40b7-4755-9b0b-07eefa24137d/8760/1176214","complete":true,"expired_claims":0}
    qinq_test.go:440: agent log: {"time":"2026-09-24T18:21:48.622361238+03:30","level":"INFO","msg":"reconcile done","owner":"w5","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing"],"status":"APPLY_STATUS_APPLIED","summary":"created:12  unchanged:4","reapplied":0,"duration":997968186,"err":""}
    qinq_test.go:438: agent log: {"time":"2026-09-24T18:21:48.622480353+03:30","level":"INFO","msg":"resync finished","owner":"w5","status":"APPLY_STATUS_APPLIED","summary":"created:12  unchanged:4"}
    qinq_test.go:446: reconcile after simulated loss: 2026-09-24T18:21:47.290456151+03:30 → 2026-09-24T18:21:48.622480353+03:30 = 1.332s (agent log timestamps)
    qinq_test.go:453: after recovery host-w5w0.100: sw_if_index=1 sup=7 sub_id=100 sub_number_of_tags=1 sub_outer_vlan_id=100 sub_inner_vlan_id=0 sub_if_flags=ONE_TAG|EXACT_MATCH tag="w5:host-w5w0.100" ipv4=[10.5.100.1/24]
    qinq_test.go:453: after recovery host-w5w0.200: sw_if_index=5 sup=7 sub_id=200 sub_number_of_tags=2 sub_outer_vlan_id=200 sub_inner_vlan_id=100 sub_if_flags=TWO_TAGS|DOT1AD|EXACT_MATCH tag="w5:host-w5w0.200" ipv4=[10.5.200.1/24]
    qinq_test.go:453: after recovery host-w5w0.300: sw_if_index=3 sup=7 sub_id=300 sub_number_of_tags=2 sub_outer_vlan_id=300 sub_inner_vlan_id=30 sub_if_flags=TWO_TAGS|EXACT_MATCH tag="w5:host-w5w0.300" ipv4=[10.5.30.1/24]
    qinq_test.go:458: vppctl show interface address (after recovery):
        host-w5w0.100 (up):
          L3 10.5.100.1/24
        host-w5w0.200 (up):
          L3 10.5.200.1/24
        host-w5w0.300 (up):
          L3 10.5.30.1/24
=== RUN   TestVlanQinqTopology/rollback
    qinq_test.go:470: POST /config/rollback/1 → status applied revision 3
    qinq_test.go:474: results (9, in order) summary={"created":0,"deleted":9,"failed":0,"reverted":0,"unchanged":4,"updated":0}:
           0 delete ok   interface.admin-state/host-w5w0.300
           1 delete ok   interface.admin-state/host-w5w0.200
           2 delete ok   interface.admin-state/host-w5w0.100
           3 delete ok   interface-ip/host-w5w0.300/10.5.30.1/24
           4 delete ok   interface.subinterface/host-w5w0.300
           5 delete ok   interface-ip/host-w5w0.200/10.5.200.1/24
           6 delete ok   interface.subinterface/host-w5w0.200
           7 delete ok   interface-ip/host-w5w0.100/10.5.100.1/24
           8 delete ok   interface.subinterface/host-w5w0.100
    qinq_test.go:499: Retrieve after rollback: host-w5w0 config={"description":"F-vlan-qinq parent (rig wan)","enabled":true,"ipv4":["10.5.2.1/24"],"promiscuous":false,"vrf":"default"} (rows for [host-w5w0.100 host-w5w0.200 host-w5w0.300]: none)
    qinq_test.go:510: sw_interface_dump after rollback: 0 interface(s) named host-w5w0.* []
    qinq_test.go:515: vppctl show interface (after rollback):
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
        host-w5l0                         6      up          1500/0/0/0     rx packets                     3   (…)
        host-w5w0                         7      up          9000/0/0/0     rx packets                    46   (…)
        local0                            0     down          0/0/0/0       drops                          4
=== RUN   TestVlanQinqTopology/cleanup-through-api
    qinq_test.go:526: commit (parent deleted) → applied revision 4
    qinq_test.go:537: sw_interface_dump, everything with prefix w5 after the cleanup commit (rig down removes the rig's own lan side): [host-w5l0 (tag "")]
=== NAME  TestVlanQinqTopology
    stack_test.go:199: stopped vrx-api pid 1872554
    stack_test.go:199: stopped vrx-agent pid 1876723
    qinq_test.go:188: pg-test drop w5: <nil>
        drop   database vrx_w5
        drop   role vrx_w5
        ok     nothing named vrx_w5 / vrx_w5 remains
    qinq_test.go:255: rig down: <nil>
        rig down w5
          delete vpp host-w5l0
          delete netns ns-w5-lan (and its veth peer)
          delete netns ns-w5-wan (and its veth peer)
        rig: down
    qinq_test.go:270: systemctl show vpp -p NRestarts (after) = 0
--- PASS: TestVlanQinqTopology (37.12s)
    --- PASS: TestVlanQinqTopology/commit (2.98s)
    --- PASS: TestVlanQinqTopology/packets (9.80s)
    --- PASS: TestVlanQinqTopology/restart-safety (4.14s)
    --- PASS: TestVlanQinqTopology/rollback (0.83s)
    --- PASS: TestVlanQinqTopology/cleanup-through-api (0.57s)
PASS
ok  	ngfw/test/topology/vlan-qinq	37.158s
NRestarts after: 0 pid 8760 18:21:55.79288362
```
The packet phase of run 1 used the first version of the test (dot1ad devices included, IPv6 still on the VLAN devices,
hence `ip6` drops in the counters, and **`vppctl show trace`**, which the committed test no longer calls: that command
crashed VPP in run 2, Q0/D-128). The committed phase is opt-in (`VRX_QINQ_PACKETS=1`), sends no 802.1ad frame and
asserts per-sub-interface rx/tx counter deltas instead of a trace; the committed test's host run is in "Fix round 1".

### CLI equivalent — the same test, run 2 (18:40, commit step passed before the crash of Q0)
```
    qinq_test.go:383: $ vrx configure show interfaces host-w5w0 subinterfaces set
        set interfaces host-w5w0 subinterfaces 100 description "dot1q 100"
        set interfaces host-w5w0 subinterfaces 100 dot1ad false
        …
        set interfaces host-w5w0 subinterfaces 200 description "QinQ dot1ad 200 + dot1q 100"
        set interfaces host-w5w0 subinterfaces 200 dot1ad true
        set interfaces host-w5w0 subinterfaces 200 enabled true
        set interfaces host-w5w0 subinterfaces 200 innerVlanId 100
        set interfaces host-w5w0 subinterfaces 200 ipv4 10.5.200.1/24
        set interfaces host-w5w0 subinterfaces 200 ipv6 []
        set interfaces host-w5w0 subinterfaces 200 vlanId 200
        set interfaces host-w5w0 subinterfaces 200 vrf default
        …
    qinq_test.go:383: $ vrx show interfaces host-w5w0.200
        Interface host-w5w0.200 (retrieved 2026-09-24T15:11:05.412Z)
          description "QinQ dot1ad 200 + dot1q 100";
          dot1ad true;
          enabled true;
          innerVlanId 100;
          ipv4 [ 10.5.200.1/24 ];
          vlanId 200;
          vrf default;
```

### Screenshots — `TestVlanQinqScreenshots` (18:33, production build under `vite preview`, real API + agent + VPP; NRestarts 0 → 0)
Headless Chrome-for-Testing 153 + playwright-core 1.63 from the npx cache, driven by `test/topology/vlan-qinq/shots.mjs`
(committed, so it does not get lost like P08's F6; nothing installed). `.300` has an uncommitted inner-tag change (30 → 31),
so its row shows the stack VPP still has.
```
NRestarts before: 0 pid 8760 18:33:23.82894721
    shots_test.go:73: screenshots:
        vlan-qinq-drawer-en.png  html dir/lang=ltr/en  rows=["host-w5w0.100 dot1q 100 — Up Up 10.5.100.1/24","host-w5w0.200 dot1ad 200 · dot1q 100 100 Up Up 10.5.200.1/24","host-w5w0.300 dot1q 300 · dot1q 31 in VPP: dot1q 300 · dot1q 30 31 Up Up 10.5.30.1/24"]  pageErrors=0
        vlan-qinq-dialog-en.png  title="Sub-interface host-w5w0.200"  pageErrors=0
        vlan-qinq-drawer-fa-rtl.png  html dir/lang=rtl/fa  rows=["host-w5w0.100 dot1q 100 — فعال فعال 10.5.100.1/24","host-w5w0.200 dot1ad 200 · dot1q 100 100 فعال فعال 10.5.200.1/24","host-w5w0.300 dot1q 300 · dot1q 31 در VPP: dot1q 300 · dot1q 30 31 فعال فعال 10.5.30.1/24"]  pageErrors=0
--- PASS: TestVlanQinqScreenshots (83.29s)
ok  	ngfw/test/topology/vlan-qinq	83.881s
NRestarts after: 0 pid 8760 18:36:49.958760728
```
![QinQ table, en](../../user/interfaces/img/vlan-qinq-table-en.png)
![QinQ table, fa](../../user/interfaces/img/vlan-qinq-table-fa.png)
![drawer, en](../../user/interfaces/img/vlan-qinq-drawer-en.png)
![drawer, fa/RTL](../../user/interfaces/img/vlan-qinq-drawer-fa-rtl.png)
![QinQ edit dialog, en](../../user/interfaces/img/vlan-qinq-dialog-en.png)

The dialog still shows P08's help texts "802.1Q tag (single-tag sub-interfaces)" / "inner tag for QinQ (not supported by
this release)" — they live in `interfaces.json`, which the envelope forbids me (Q5, replacement strings proposed).

### Unit / e2e (real output)
```
$ cd packages/schema && npx vitest run src/semantic/interfaces-qinq.test.ts --reporter=verbose
 ✓ … > accepts an 802.1ad outer tag with an 802.1Q inner tag and keeps the tag stack as written
 ✓ … > accepts 802.1Q-in-802.1Q (two dot1q tags) next to a single-tag sub-interface on the same outer VLAN
 ✓ … > accepts the same outer tag once as dot1q and once as dot1ad on one parent (distinct tag stacks)
 ✓ … > accepts the same tag stack on two different parents
 ✓ … > rejects a duplicate (dot1ad, vlanId, innerVlanId) with the pointer to the second entry
 ✓ … > rejects a duplicate dot1q-in-dot1q stack, and "second" follows the numeric order of the ids
 ✓ … > rejects innerVlanId without vlanId (schema: the outer tag is required)
 ✓ … > rejects out-of-range inner tags (1–4094, like the outer tag)
 ✓ … > rejects a QinQ sub-interface MTU above the parent MTU, with the pointer to the sub-interface MTU
      Tests  9 passed (9)

$ cd apps/agent && go test -count=1 -v -run QinQ ./internal/desired/
--- PASS: TestQinQBuilderObjects (0.03s)
--- PASS: TestQinQRoundTripOnFake (0.10s)
--- PASS: TestQinQDeleteWhileParentStays (0.03s)
ok  	ngfw/agent/internal/desired	0.212s
$ go test -count=1 ./internal/descriptors/interface/ ./internal/agent/ ./internal/scheduler/ ./internal/subsystems/
ok  	ngfw/agent/internal/descriptors/interface	0.060s
ok  	ngfw/agent/internal/agent	7.602s
ok  	ngfw/agent/internal/scheduler	0.061s
ok  	ngfw/agent/internal/subsystems	0.049s

# discrimination: the same tests with the pre-fix subinterface.go (go test -overlay, nothing committed)
$ go test -count=1 -overlay overlay.json -run QinQ ./internal/desired/
--- FAIL: TestQinQRoundTripOnFake (0.40s)
    interfaces_qinq_test.go:388: apply q8: APPLY_STATUS_ROLLED_BACK delete interface.admin-state/host-w5w0.300: sw_interface_set_flags: VPPApiError: Invalid sw_if_index (-2) results=[key:"interface.subinterface/host-w5w0.300"  op:APPLY_OPERATION_DELETE  code:OBJECT_RESULT_CODE_REVERTED …
--- FAIL: TestQinQDeleteWhileParentStays (0.06s)
    interfaces_qinq_test.go:416: apply d2: APPLY_STATUS_ROLLED_BACK delete interface.admin-state/host-w5w0.200: sw_interface_set_flags: VPPApiError: Invalid sw_if_index (-2) results=[key:"interface.subinterface/host-w5w0.200"  op:APPLY_OPERATION_DELETE  code:OBJECT_RESULT_CODE_REVERTED …
FAIL	ngfw/agent/internal/desired	0.556s

$ eval "$(tools/lab env 5)"; cd apps/api && npx vitest run -c vitest.e2e.config.ts test/e2e/vlan-qinq.e2e.test.ts
 ✓ test/e2e/vlan-qinq.e2e.test.ts (4 tests) 5767ms
   ✓ … > commits dot1q 100 and dot1ad 200 + dot1q 100 and lists both with parent, vlanId, innerVlanId and dot1ad  618ms
   ✓ … > a duplicate (dot1ad, vlanId, innerVlanId) on one parent is a 400 problem+json with the pointer to the second entry  389ms
   ✓ … > a rollback to the revision without sub-interfaces removes both rows; the parent stays  357ms
      Tests  4 passed (4)
drop   database vrx_w5
ok     nothing named vrx_w5 / vrx_w5 remains

$ cd apps/web && npx vitest run src/domains/interfaces src/locales
 ✓ src/locales/locales.test.ts (12 tests)
 ✓ src/domains/interfaces/subinterfaces/tagStack.test.ts (6 tests)
 ✓ src/domains/interfaces/model.test.ts (5 tests)
 ✓ src/domains/interfaces/subinterfaces/SubinterfaceTable.test.tsx (4 tests)
 ✓ src/domains/interfaces/InterfacesPage.test.tsx (7 tests)            (P08's drawer tests, unchanged, on the swapped table)
      Tests  34 passed (34)
```

### CI gate — `TMPDIR=/tmp/g-w5 tools/ci.sh --base main`
Runs at 18:46 and 18:48 failed on things this branch does not touch: the contract guard's SIGPIPE flake (Q6), then under
host load ~35 two timeouts in unchanged files (`packages/schema/src/semantic/acl.test.ts` "Test timed out in 5000ms",
`apps/web/src/flows.test.tsx` login heading) — both pass alone (13/13, 6/6). Run 3 (HEAD c06785c; only this status file
uncommitted), untrimmed except the per-package `ok` lists:
```
$ TMPDIR=/tmp/g-w5 tools/ci.sh --base main
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/F-vlan-qinq
branch    task/F-vlan-qinq @ c06785c   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
logs      /root/ngfw-wt/logs/ci/F-vlan-qinq-20260924-185951-2149999
WARN uncommitted changes in the worktree — the gate checks the working tree, but only commits get merged:
      ?? docs/status/tasks/F-vlan-qinq.md

== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  apps/agent/gen/vrx/v1/dataplane.pb.go
  apps/agent/gen/vrx/v1/dataplane_grpc.pb.go
  packages/api-client/src/generated/schema.d.ts
  packages/proto/gen/ts/vrx/v1/dataplane.ts
  packages/proto/vrx/v1/dataplane.proto
  packages/schema/src/domains/interfaces.ts
  packages/schema/src/domains/routing.ts
  packages/schema/src/domains/services.ts
  packages/schema/src/domains/vrfs.ts
  packages/schema/src/index.ts
  packages/schema/src/semantic/index.ts
  packages/schema/src/semantic/interfaces-qinq.test.ts
ok — contract commit(s) on the branch:
  6ce08c2 contract(api-client): /state/interfaces items[].config — null also on candidate-only rows (description; P08 re-review R1, D-118)
  5c6e1f8 contract(wave-A): anchors
  f6fbdf3 contract(api-client): /state/interfaces items[].config is the Retrieve view again, running config in new items[].running, actual dropped (P08 F1, D-105; additive); CLI operations table regenerated (F2)
  c02aa32 contract(api-client): regenerate — /state/interfaces merged items (state/config/actual/counters/hasPendingChange), /state/interfaces/{name}/counters (P08, additive)
  51b7c42 contract(proto): InterfaceState RPC — live interface table for /state/interfaces (additive, P08)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(W-seed): verify

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~738179 bytes (738.18 KB) in 2.9s no leaks found

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    22 cached, 30 total Time:    5m16.74s

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.901s; ok  	ngfw/agent/cmd/vrx-vppcheck	1.850s; ok  	ngfw/agent/internal/agent	10.735s; …

== apps/cli: make lint test build ==
ok  	ngfw/cli/internal/api	1.468s; ok  	ngfw/cli/internal/cli	2.084s; … ok  	ngfw/cli/test/e2e	1.124s;

== test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces test/topology/vlan-qinq) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.023s;
test/topology/interfaces: gofmt ok · go vet ok · ok  	ngfw/test/topology/interfaces	0.025s;
test/topology/vlan-qinq: gofmt ok · go vet ok · ok  	ngfw/test/topology/vlan-qinq	0.025s;

== summary (quick) ==
  mode quick · wall time 10m04s · logs /root/ngfw-wt/logs/ci/F-vlan-qinq-20260924-185951-2149999

CI GATE PASSED
```
The only file of mine under the contract paths is the test `packages/schema/src/semantic/interfaces-qinq.test.ts`; no
schema, proto or generated file changed on this branch (the listed ones are P08's and W-seed's, with their contract commits).

## Incident (Q0) — root cause: `vppctl show trace` (corrected in fix round 1)
18:41:08 VPP `SIGSEGV, PC 0x0` during the packet phase of my second topology run (NRestarts 0 → 1; systemd restarted VPP,
pid 8760 → 2006833; core in `/var/lib/systemd/coredump/`). The review unwound the core (vpp-dbg symbols): the fault is in
the CLI process — `cli_show_trace_buffer` → `format_vlib_trace` (`vlib/trace.c:159-162`) calls a NULL `format_buffer` for a
stale trace record whose tx node belonged to a deleted interface and was recycled by one without a trace formatter. The
trigger was my test's own `show trace` after the ping, **not** the 802.1ad frames, the V19 classify reset or V24 — my
first reading was wrong. Fix round 1: no packet trace in any test of mine; the VPP-code item and the host-wide ban are
TD-20's (D-128). Timeline in `F-vlan-qinq-questions.md` Q0.

## Out of scope / not done
- Non-exact-match / default / untagged / any sub-interfaces, tag rewrite, L2 sub-interfaces (F-bridge-l2), bonding,
  linux-cp mirrors, DPDK path — as the prompt says. The exact-match question is recorded (Q4).
- `packages/schema/examples/vlan-qinq-*.json`: not added — `examples.test.ts` rejects unknown prefixes (Q3).
- The fake agent reports `innerVlanId: 0` (Q2) — the e2e overrides the live row; the real agent reports it (fake VPP + host).
- `interfaces.json` help strings (Q5) and `basics.md` line 102 ("Not in this release: … QinQ") — manager, after the wave.
- The general delete-order gap for other creators (af_packet/loopback/bond attributes on an observe-only alias) — Q1 (b).
- A final clean host run of the packet-free topology test after the crash: done in fix round 1 (slot 12, NRestarts 1 → 1).

## Decisions taken (options → choice, why)
| # | decision | options | why |
|---|---|---|---|
| D-VQ-1 | Fix the delete-order defect in DF-1's sub-interface descriptor (`ProvidedKeys`) | (a) descriptor provides its alias key (b) scheduler follows deps through observe-only objects (c) tolerant admin-state delete | (a) is inside the A-list file the envelope allows for a proven defect and fixes the order (not just the symptom); (b) is P05's scheduler — proposed in Q1 |
| D-VQ-2 | Topology test hands over only the rig's **wan** side (the parent); the lan side stays the rig's | (a) wan only (b) both, as P08 | fewer af_packet deletes on the shared VPP (V24, D-107) |
| D-VQ-3 | Add `.300` (dot1q-in-dot1q) to the acceptance pair `.100`/`.200` | (a) 3 stacks (b) exactly the 2 | proves the second QinQ flavour on real VPP at no extra churn |
| D-VQ-4 | Packet phase opt-in, without VPP packet trace (fix round 1) and without 802.1ad frames | (a) opt-in, counters instead of trace, no S-tag (b) keep the trace (c) remove the phase | corrected rationale: the crash was `show trace` (review H1, D-128), so the trace goes and ping + per-sub-interface counter deltas prove the stack (b is banned); opt-in is the manager's rule (D-126/D-128); no S-tag because an 802.1ad frame cannot reach a dot1ad sub-interface on af_packet (V-new) — a lab limit, not safety; (c) loses the dot1q/QinQ packet proof |
| D-VQ-5 | Encapsulation column shows the **candidate** stack; a differing live stack (numbers from `state`, type from `config`, D-105) goes under it | (a) candidate + live drift line (b) live only (c) candidate only | the table edits the candidate; hiding what VPP runs during a pending tag change would mislead |
| D-VQ-6 | Screenshot script committed as test code (`test/topology/vlan-qinq/shots.mjs`), chrome/playwright-core from env paths | (a) commit the script (b) scratch only (P08) | P08's F6: the scratch script was lost |
| D-VQ-7 | Tag notation `dot1q 100` / `dot1ad 200 · dot1q 100` is not translated (VPP/TNSR CLI notation, shown LTR); TPID tooltips and headers are | (a) technical notation (b) localized words | it is the notation users type in the CLI and see in VPP |

## Cleanup
Checked 19:12 (after the last host run at 18:41 and the CI gate):
```
processes of slot 5 (vrx-agent / vrx-api / vite on 3500/5500, VRX_OWNER=w5): 0      (every one stopped by PID in the test cleanups)
lab lock holders: F-object-model's topology run only (pids of /root/ngfw-wt/F-object-model/…) — none of mine
deploy/dev/pg-test.sh list | grep w5: 0                                              (vrx_w5 dropped by every run: "ok nothing named vrx_w5 / vrx_w5 remains")
ip netns list | grep ns-w5: 0 · ip -br link | grep ^w5: 0                             (rig down in every run)
vppctl show interface | grep w5: 0 — VPP (restarted at 18:41:26, pid 2006833, NRestarts=1) shows only local0 for me
```
`apps/{agent,cli}/bin`, `apps/{api,web}/dist` and `packages/*/dist` removed. `/run/vrx-test/w5` stays `drwxr-xr-x`; I left
`/run/vrx-test/w5/qinq/{agent,api}.log` of the crashed run for Q0 (0600, no secrets; the CLI password file removed, and the
test now deletes it itself). Scratch only (my scratchpad, never in the worktree): the Chrome-for-Testing copy, run logs.

## Fix round 1 (review 674b03e: H1, M1, M2, L1, L2) — slot 12 (`w12`, API 4200, metrics 9221, DB `vrx_w12`, rig 10.12/16; D-128)

| item | what I did | commit |
|---|---|---|
| H1 | `test/topology/vlan-qinq`: no `trace add` / `show trace` anywhere (grep below). The opt-in packet phase (`VRX_QINQ_PACKETS=1`) now proves the dot1q and dot1q-in-dot1q path with the answered ping **plus the rx/tx packet counter deltas of exactly the pinged sub-interface** (`vppctl show interface <subs>` before/after, parsed by `parseIfCounters`, unit-tested by `TestParseIfCounters`; counters are never cleared on the shared VPP). Q0, the test header, the Acceptance line, the run-1 note, the Incident section and D-VQ-4 now name the real trigger (`show trace` → `format_vlib_trace` NULL formatter of a recycled interface tx node), not 802.1ad frames / V24 | abe6d50, this docs commit |
| H1 V-item | **not added by me**: TD-20's envelope owns the `docs/vpp-code-track.md` V-new for the `format_vlib_trace` NULL guard (plus the `tools/ci.sh` ban and `shared-host-rules.md` §11); a second row from this branch would duplicate it at merge (FR1-a below). TD-20's ban patterns (`task/TD-20:tools/ci.sh` `do_trace_ban`), run over the 20 non-doc files this branch changed since W-seed: 0 hits | — |
| M1 | nothing on the branch (fix envelope). Main has D-128b since 7e2c272 (22:41): the guard drops `*.test.ts` / `_test.go` from the changed contract files. Simulated with that filter on the review's case (W-seed as the base): raw `git diff --name-only df67a8e HEAD -- <CONTRACT_PATHS>` = only `packages/schema/src/semantic/interfaces-qinq.test.ts`; after the filter **empty** → "no contract files changed". Against main today the branch's own guard passes through W-seed/P08's `contract(…)` commits in `mb..HEAD` (CI below). No fake contract commit | — |
| M2 | the committed test ran once on the host, packet-free (`VRX_QINQ_PACKETS` unset), at abe6d50: PASS, NRestarts 1 → 1 (below). Test code is unchanged since then (`git diff --stat abe6d50 HEAD`: `tagStack.ts` + my two status files only), so I did not run it again. CI at the final HEAD (below) | — |
| L1 | the unreachable `proto = "802.1ad"` branch removed; `vlanDevices` builds only 802.1Q stacks and says why (V-new, a lab limit) | abe6d50 |
| L2 | `TagStackFields` = `Loose<Pick<SubinterfaceConfig, 'vlanId' \| 'innerVlanId' \| 'dot1ad'>>`, `TagStackRow` = `Pick<LiveState, 'vlanId' \| 'innerVlanId'>` + `Loose<Pick<InterfaceItem, 'config' \| 'running'>>` — a contract rename breaks the build | 5cd5010 |
| L3 | nothing on this branch (the fake's `innerVlanId`, Q2, belongs to the P5 owner) | — |

No trace call left in my tests (the only hits are comments naming the ban):
```
$ grep -rnE 'trace' test/topology/vlan-qinq/*.go
test/topology/vlan-qinq/qinq_test.go:13://	                  <sub>` deltas, never a clear). No VPP packet trace on the shared VPP (D-128): VPP's trace formatter
test/topology/vlan-qinq/vpp_test.go:5:// packet trace on the shared VPP, D-128). Copied from P08's test/topology/interfaces and extended with sub-interface rows
$ cd test/topology/vlan-qinq && gofmt -l . ; go vet ./... && go test -count=1 ./...
ok  	ngfw/test/topology/vlan-qinq	0.025s
$ go test -count=1 -run TestParseIfCounters -v .
--- PASS: TestParseIfCounters (0.00s)
$ cd apps/web && npx tsc --noEmit -p tsconfig.json; echo $?      →  0
$ npx vitest run src/domains/interfaces src/locales               →  Test Files  5 passed (5) · Tests  34 passed (34)
$ npx eslint src/domains/interfaces/subinterfaces/ && npx prettier --check src/domains/interfaces/subinterfaces/*.ts   →  0 problems · All matched files use Prettier code style!
```

### Host run — the committed test, packet-free (19:43, slot 12, HEAD abe6d50)
`eval "$(tools/lab env 12)"; test/topology/vlan-qinq/run.sh -run 'TestVlanQinqTopology|TestParseIfCounters'`. Trimmed:
the rig's key/value print lines, the `/state` rows' repeated fields and `running` copy, two CLI `set` blocks, the diff
body (`…`); nothing else edited. The 3 `ip6` drops on `host-w12l0` are the rig's own lan-side link-local noise between
`rig up` and the test setting the peers down — no packet was sent by the test.
```
NRestarts before: 1 pid 2006833 19:43:48.505195089 HEAD abe6d50 VRX_QINQ_PACKETS=<unset>
=== RUN   TestVlanQinqTopology
    qinq_test.go:309: systemctl show vpp -p NRestarts (before) = 1
    qinq_test.go:309: rig up w12 (slot 12, path af_packet)
          create netns ns-w12-lan
          create veth w12l0 <-> ns-w12-lan:w12l1
          create vpp host-w12l0 (af_packet on w12l0)
          set    addr host-w12l0 10.12.1.1/24
          create netns ns-w12-wan
          create veth w12w0 <-> ns-w12-wan:w12w1
          create vpp host-w12w0 (af_packet on w12w0)
          set    addr host-w12w0 10.12.2.1/24
        rig: up
    qinq_test.go:309: rig wan side handed to the agent: sw_interface_add_del_address sw_if_index=1 (host-w12w0) del_all=true → ok
    qinq_test.go:309: rig wan side handed to the agent: af_packet_delete host_if_name=w12w0 (tag "") → ok
    qinq_test.go:309: create role vrx_w12
        create database vrx_w12 (owner vrx_w12)
        check  vrx_w12 as vrx_w12 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
        ok     env /run/vrx-test/w12/pg.env (0600) · DSN postgres://vrx_w12:<redacted>@127.0.0.1:5432/vrx_w12
    qinq_test.go:309: started vrx-agent pid 2902187 (log /run/vrx-test/w12/qinq/agent.log)
    qinq_test.go:309: started vrx-api pid 2902216 (log /run/vrx-test/w12/qinq/api.log)
=== RUN   TestVlanQinqTopology/commit
    qinq_test.go:327: commit parent → applied revision 1
    qinq_test.go:336: commit with a duplicate tag stack → 400 content-type problem+json; body {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic","warnings":[],"detail":"semantic validation failed","instance":"/api/v1/config/commit","errors":[{"pointer":"/interfaces/host-w12w0/subinterfaces/201/vlanId","message":"VLAN dot1ad 200.100 is already used by sub-interface host-w12w0.200"}]}
    qinq_test.go:344: candidate diff: {"baseRevision":1,"changes":[{"op":"add","pointer":"/interfaces/host-w12w0/subinterfaces/100","to":{"vrf":"default","ipv4":["10.12.100.1/24"],"ipv6":[],"dot1ad":false,"vlanId":100,"enabled":true,"description":"dot1q 100"}},{"op":"add",… (the three adds, as in run 1)
    qinq_test.go:346: commit sub-interfaces → applied revision 2 txn 16ff3b11-c8dd-4952-b768-a994a22c251b
    qinq_test.go:347: results (12, in order) summary={"created":12,"deleted":0,"failed":0,"reverted":0,"unchanged":4,"updated":0}:
           0 create ok   interface.subinterface/host-w12w0.100
           1 create ok   interface.subinterface/host-w12w0.200
           2 create ok   interface.subinterface/host-w12w0.300
           3 create ok   interface/host-w12w0.100
           4 create ok   interface-ip/host-w12w0.100/10.12.100.1/24
           5 create ok   interface.admin-state/host-w12w0.100
           6 create ok   interface/host-w12w0.200
           7 create ok   interface-ip/host-w12w0.200/10.12.200.1/24
           8 create ok   interface.admin-state/host-w12w0.200
           9 create ok   interface/host-w12w0.300
          10 create ok   interface-ip/host-w12w0.300/10.12.30.1/24
          11 create ok   interface.admin-state/host-w12w0.300
    qinq_test.go:354: sw_interface_dump host-w12w0.100: sw_if_index=9 sup=1 sub_id=100 sub_number_of_tags=1 sub_outer_vlan_id=100 sub_inner_vlan_id=0 sub_if_flags=ONE_TAG|EXACT_MATCH tag="w12:host-w12w0.100"
    qinq_test.go:354: sw_interface_dump host-w12w0.200: sw_if_index=10 sup=1 sub_id=200 sub_number_of_tags=2 sub_outer_vlan_id=200 sub_inner_vlan_id=100 sub_if_flags=TWO_TAGS|DOT1AD|EXACT_MATCH tag="w12:host-w12w0.200"
    qinq_test.go:354: sw_interface_dump host-w12w0.300: sw_if_index=4 sup=1 sub_id=300 sub_number_of_tags=2 sub_outer_vlan_id=300 sub_inner_vlan_id=30 sub_if_flags=TWO_TAGS|EXACT_MATCH tag="w12:host-w12w0.300"
    qinq_test.go:372: /state/interfaces host-w12w0.100: kind=subinterface parent=host-w12w0 state={"adminUp":true,"innerVlanId":0,"linkUp":true,"managed":true,…,"vlanId":100,…} config(Retrieve)={"description":"dot1q 100","dot1ad":false,"enabled":true,"ipv4":["10.12.100.1/24"],"vlanId":100,"vrf":"default"} running=(same + "ipv6":[]) hasPendingChange=false
    qinq_test.go:372: /state/interfaces host-w12w0.200: kind=subinterface parent=host-w12w0 state={"adminUp":true,"innerVlanId":100,"linkUp":true,"managed":true,…,"vlanId":200,…} config(Retrieve)={"description":"QinQ dot1ad 200 + dot1q 100","dot1ad":true,"enabled":true,"innerVlanId":100,"ipv4":["10.12.200.1/24"],"vlanId":200,"vrf":"default"} running=(same + "ipv6":[]) hasPendingChange=false
    qinq_test.go:372: /state/interfaces host-w12w0.300: kind=subinterface parent=host-w12w0 state={"adminUp":true,"innerVlanId":30,"linkUp":true,"managed":true,…,"vlanId":300,…} config(Retrieve)={"description":"dot1q 300 + dot1q 30","dot1ad":false,"enabled":true,"innerVlanId":30,"ipv4":["10.12.30.1/24"],"vlanId":300,"vrf":"default"} running=(same + "ipv6":[]) hasPendingChange=false
    qinq_test.go:386: vppctl show interface host-w12w0.100 host-w12w0.200 host-w12w0.300:
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
        host-w12w0.100                    9      up           0/0/0/0
        host-w12w0.200                    10     up           0/0/0/0
        host-w12w0.300                    4      up           0/0/0/0
    qinq_test.go:387: vppctl show interface address:
        host-w12w0 (up):
          L3 10.12.2.1/24
        host-w12w0.100 (up):
          L3 10.12.100.1/24
        host-w12w0.200 (up):
          L3 10.12.200.1/24
        host-w12w0.300 (up):
          L3 10.12.30.1/24
    qinq_test.go:394: $ vrx configure show interfaces host-w12w0 subinterfaces set
        set interfaces host-w12w0 subinterfaces 200 description "QinQ dot1ad 200 + dot1q 100"
        set interfaces host-w12w0 subinterfaces 200 dot1ad true
        set interfaces host-w12w0 subinterfaces 200 enabled true
        set interfaces host-w12w0 subinterfaces 200 innerVlanId 100
        set interfaces host-w12w0 subinterfaces 200 ipv4 10.12.200.1/24
        set interfaces host-w12w0 subinterfaces 200 ipv6 []
        set interfaces host-w12w0 subinterfaces 200 vlanId 200
        set interfaces host-w12w0 subinterfaces 200 vrf default
        …  (the same seven lines for 100 and 300)
    qinq_test.go:394: $ vrx show interfaces host-w12w0.200
        Interface host-w12w0.200 (retrieved 2026-09-24T16:14:36.840Z)
          description "QinQ dot1ad 200 + dot1q 100";
          dot1ad true;
          enabled true;
          innerVlanId 100;
          ipv4 [ 10.12.200.1/24 ];
          vlanId 200;
          vrf default;
        Counters (2026-09-24T16:14:36.817Z)
          drops 0;
          errors 0;
          name host-w12w0.200;
          punts 0;
          rxBytes 0;
          rxMisses 0;
          rxPackets 0;
          swIfIndex 10;
          txBytes 0;
          txPackets 0;
=== RUN   TestVlanQinqTopology/packets
    qinq_test.go:404: packet phase is opt-in: VRX_QINQ_PACKETS=1 (D-126/D-128)
=== RUN   TestVlanQinqTopology/restart-safety
    stack_test.go:199: stopped vrx-agent pid 2902187
    qinq_test.go:450: simulated loss: sw_interface_add_del_address sw_if_index=9 (host-w12w0.100) del_all=true → ok
    qinq_test.go:450: simulated loss: sw_interface_add_del_address sw_if_index=10 (host-w12w0.200) del_all=true → ok
    qinq_test.go:450: simulated loss: sw_interface_add_del_address sw_if_index=4 (host-w12w0.300) del_all=true → ok
    qinq_test.go:450: simulated loss: delete_subif sw_if_index=9 (host-w12w0.100, tag "w12:host-w12w0.100") → ok
    qinq_test.go:450: simulated loss: delete_subif sw_if_index=10 (host-w12w0.200, tag "w12:host-w12w0.200") → ok
    qinq_test.go:450: simulated loss: delete_subif sw_if_index=4 (host-w12w0.300, tag "w12:host-w12w0.300") → ok
    qinq_test.go:458: sw_interface_dump after the loss: none of [host-w12w0.100 host-w12w0.200 host-w12w0.300] exists; host-w12w0 stays (sw_if_index 1)
    qinq_test.go:460: GET /state/interfaces while the agent is stopped → 503 {"type":"https://vrx.dev/problems/agent-unavailable","title":"Agent unavailable","status":503,"grpcCode":"UNAVAILABLE","detail":"agent: No connection establishe…
    qinq_test.go:464: started vrx-agent pid 2906345 (log /run/vrx-test/w12/qinq/agent.log)
    qinq_test.go:479: agent started at +0s; all three sub-interfaces back in VPP with their addresses at +0.41s (no config API call)
    qinq_test.go:486: agent log: {"time":"2026-09-24T19:44:39.088056045+03:30","level":"INFO","msg":"vrx-agent starting","version":"dev","pid":2906345,"owner":"w12","socket":"/run/vrx-test/w12/agent.sock","vpp_api":"/run/vpp/api.sock"}
    qinq_test.go:491: agent log: {"time":"2026-09-24T19:44:39.113332783+03:30","level":"INFO","msg":"VPP boot identity","owner":"w12","component":"subsystems","identity":"a93c0e7a-40b7-4755-9b0b-07eefa24137d/2006833/3203894",…
    qinq_test.go:491: agent log: {"time":"2026-09-24T19:44:39.342589091+03:30","level":"INFO","msg":"reconcile done","owner":"w12","mode":"resync","status":"APPLY_STATUS_APPLIED","summary":"created:12  unchanged:4","reapplied":0,"duration":229088337,"err":""}
    qinq_test.go:489: agent log: {"time":"2026-09-24T19:44:39.342645494+03:30","level":"INFO","msg":"resync finished","owner":"w12","status":"APPLY_STATUS_APPLIED","summary":"created:12  unchanged:4"}
    qinq_test.go:497: reconcile after simulated loss: 2026-09-24T19:44:39.088056045+03:30 → 2026-09-24T19:44:39.342645494+03:30 = 0.255s (agent log timestamps)
    qinq_test.go:504: after recovery host-w12w0.100: sw_if_index=4 sup=1 sub_id=100 sub_number_of_tags=1 sub_outer_vlan_id=100 sub_inner_vlan_id=0 sub_if_flags=ONE_TAG|EXACT_MATCH tag="w12:host-w12w0.100" ipv4=[10.12.100.1/24]
    qinq_test.go:504: after recovery host-w12w0.200: sw_if_index=10 sup=1 sub_id=200 sub_number_of_tags=2 sub_outer_vlan_id=200 sub_inner_vlan_id=100 sub_if_flags=TWO_TAGS|DOT1AD|EXACT_MATCH tag="w12:host-w12w0.200" ipv4=[10.12.200.1/24]
    qinq_test.go:504: after recovery host-w12w0.300: sw_if_index=9 sup=1 sub_id=300 sub_number_of_tags=2 sub_outer_vlan_id=300 sub_inner_vlan_id=30 sub_if_flags=TWO_TAGS|EXACT_MATCH tag="w12:host-w12w0.300" ipv4=[10.12.30.1/24]
    qinq_test.go:509: vppctl show interface address (after recovery):
        host-w12w0.100 (up):
          L3 10.12.100.1/24
        host-w12w0.200 (up):
          L3 10.12.200.1/24
        host-w12w0.300 (up):
          L3 10.12.30.1/24
=== RUN   TestVlanQinqTopology/rollback
    qinq_test.go:521: POST /config/rollback/1 → status applied revision 3
    qinq_test.go:525: results (9, in order) summary={"created":0,"deleted":9,"failed":0,"reverted":0,"unchanged":4,"updated":0}:
           0 delete ok   interface.admin-state/host-w12w0.300
           1 delete ok   interface.admin-state/host-w12w0.200
           2 delete ok   interface.admin-state/host-w12w0.100
           3 delete ok   interface-ip/host-w12w0.300/10.12.30.1/24
           4 delete ok   interface.subinterface/host-w12w0.300
           5 delete ok   interface-ip/host-w12w0.200/10.12.200.1/24
           6 delete ok   interface.subinterface/host-w12w0.200
           7 delete ok   interface-ip/host-w12w0.100/10.12.100.1/24
           8 delete ok   interface.subinterface/host-w12w0.100
    qinq_test.go:550: Retrieve after rollback: host-w12w0 config={"description":"F-vlan-qinq parent (rig wan)","enabled":true,"ipv4":["10.12.2.1/24"],"promiscuous":false,"vrf":"default"} (rows for [host-w12w0.100 host-w12w0.200 host-w12w0.300]: none)
    qinq_test.go:561: sw_interface_dump after rollback: 0 interface(s) named host-w12w0.* []
    qinq_test.go:566: vppctl show interface (after rollback):
                      Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
        host-w12l0                        2      up          1500/0/0/0     rx packets                     3
                                                                            rx bytes                     250
                                                                            drops                          3
                                                                            ip6                            3
        host-w12w0                        1      up          9000/0/0/0
        local0                            0     down          0/0/0/0
=== RUN   TestVlanQinqTopology/cleanup-through-api
    qinq_test.go:577: commit (parent deleted) → applied revision 4
    qinq_test.go:588: sw_interface_dump, everything with prefix w12 after the cleanup commit (rig down removes the rig's own lan side): [host-w12l0 (tag "")]
=== NAME  TestVlanQinqTopology
    stack_test.go:199: stopped vrx-api pid 2902216
    stack_test.go:199: stopped vrx-agent pid 2906345
    qinq_test.go:196: pg-test drop w12: <nil>
        drop   database vrx_w12
        drop   role vrx_w12
        ok     nothing named vrx_w12 / vrx_w12 remains
    qinq_test.go:291: rig down: <nil>
        rig down w12
          delete vpp host-w12l0
          delete netns ns-w12-lan (and its veth peer)
          delete netns ns-w12-wan (and its veth peer)
        rig: down
    qinq_test.go:281: systemctl show vpp -p NRestarts (after) = 1
--- PASS: TestVlanQinqTopology (23.38s)
    --- PASS: TestVlanQinqTopology/commit (5.00s)
    --- SKIP: TestVlanQinqTopology/packets (0.00s)
    --- PASS: TestVlanQinqTopology/restart-safety (2.68s)
    --- PASS: TestVlanQinqTopology/rollback (0.25s)
    --- PASS: TestVlanQinqTopology/cleanup-through-api (0.35s)
=== RUN   TestParseIfCounters
--- PASS: TestParseIfCounters (0.00s)
PASS
ok  	ngfw/test/topology/vlan-qinq	23.433s
exit 0
NRestarts after: 1 pid 2006833 19:44:42.514721925
```
Cleanup after the run:
```
vppctl show interface | grep -c w12: 0
ip netns | grep -c w12: 0
ip -br link | grep -c ^w12: 0
pg-test list w12: 0
procs VRX_OWNER=w12 / port 4200: 0
```

### CI gate at the final code HEAD — `TMPDIR=/tmp/g-w12 tools/ci.sh --base main` (22:45, HEAD fd604b2)
The branch's own `tools/ci.sh` failed first (22:44:35, logs `/root/ngfw-wt/logs/ci/F-vlan-qinq-20260924-224435-3281604`)
in the contract guard with "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT" although the branch carries five
`contract(…)` commits: the SIGPIPE flake (Q6, fixed on main by D-127). As D-127 says, I then ran **main's copy**
(`git show main:tools/ci.sh`, main @ 7e2c272, with D-128b) on this worktree. It also runs main's deploy/vpp step. Untrimmed
except the per-package `ok` lists:
```
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/F-vlan-qinq
branch    task/F-vlan-qinq @ fd604b2   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
logs      /root/ngfw-wt/logs/ci/F-vlan-qinq-20260924-224514-3284573

== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  apps/agent/gen/vrx/v1/dataplane.pb.go
  apps/agent/gen/vrx/v1/dataplane_grpc.pb.go
  packages/api-client/src/generated/schema.d.ts
  packages/proto/gen/ts/vrx/v1/dataplane.ts
  packages/proto/vrx/v1/dataplane.proto
  packages/schema/src/domains/interfaces.ts
  packages/schema/src/domains/routing.ts
  packages/schema/src/domains/services.ts
  packages/schema/src/domains/vrfs.ts
  packages/schema/src/index.ts
  packages/schema/src/semantic/index.ts
ok — contract commit(s) on the branch:
  6ce08c2 contract(api-client): /state/interfaces items[].config — null also on candidate-only rows (description; P08 re-review R1, D-118)
  5c6e1f8 contract(wave-A): anchors
  f6fbdf3 contract(api-client): /state/interfaces items[].config is the Retrieve view again, running config in new items[].running, actual dropped (P08 F1, D-105; additive); CLI operations table regen…
  c02aa32 contract(api-client): regenerate — /state/interfaces merged items (state/config/actual/counters/hasPendingChange), /state/interfaces/{name}/counters (P08, additive)
  51b7c42 contract(proto): InterfaceState RPC — live interface table for /state/interfaces (additive, P08)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(F-vlan-qinq): APPROVE WITH CHANGES — …
      review(W-seed): verify

== tools (golangci-lint, gitleaks) ==
golangci-lint 2.13.2
gitleaks 8.30.1

== install (pnpm --frozen-lockfile --prefer-offline) ==
Lockfile is up to date, resolution step is skipped Done in 221ms using pnpm v12.5.1

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~827513 bytes (827.51 KB) in 1.87s no leaks found

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    20 cached, 30 total Time:    4m42.982s

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.582s; ok  	ngfw/agent/cmd/vrx-vppcheck	1.876s; ok  	ngfw/agent/internal/agent	10.550s; …

== apps/cli: make lint test build ==
ok  	ngfw/cli/internal/api	2.414s; ok  	ngfw/cli/internal/cli	2.909s; …

== test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces test/topology/vlan-qinq) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.019s;
test/topology/interfaces: gofmt ok · go vet ok · ok  	ngfw/test/topology/interfaces	0.025s;
test/topology/vlan-qinq: gofmt ok · go vet ok · ok  	ngfw/test/topology/vlan-qinq	0.018s;
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot

== deploy/vpp: shellcheck + apply-startup fake-host harness ==
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
apply-startup harness: green (4 shards; 404 checks passed in the parallel run)

== summary (quick) ==
  mode quick · wall time 16m17s · logs /root/ngfw-wt/logs/ci/F-vlan-qinq-20260924-224514-3284573

CI GATE PASSED
```
With D-128b the guard no longer lists `packages/schema/src/semantic/interfaces-qinq.test.ts` (M1 closed on main's side).
The deploy/vpp step ran this branch's older `deploy/vpp` (from the W-seed base, no shard support, so every "shard" ran
the full harness). I did not touch those files. After this run the only commit is this status file. VPP stayed
pid 2006833, NRestarts 1 (CI quick does not touch VPP). Slot 12 is clean: no `w12` netns, link, VPP interface or listener
on 4200. The ignored build outputs (`apps/*/bin`, `apps/*/dist`, `packages/*/dist`) from this CI run are still in the
worktree: my `rm -rf` of them was not permitted in this session.

### Decisions (fix round 1)
| # | decision | options | why |
|---|---|---|---|
| FR1-a | No `format_vlib_trace` V-item from this branch | (a) leave it to TD-20 (b) append a second V-new as the review asked | the fix envelope (wins on scope) does not list it and TD-20's envelope owns exactly that append; (b) would put two rows for one bug into the manager's merge |
| FR1-b | Counter proof via `vppctl show interface <subs>` deltas | (a) parse `show interface` (b) govpp stats client on `/run/vpp/stats.sock` (c) tcpdump in the netns | (a) is read-only, already used by the test for evidence, and counts on the sub-interface's own sw_if_index, so it shows that ethernet-input classified the frames onto that exact tag stack; (b) adds a new client path to the test module; (c) proves only the netns side |
| FR1-c | Packet phase stays opt-in and was not run | — | fix envelope M2: `VRX_QINQ_PACKETS=1` is not set in this round; the counter assertions compile and the parser is unit-tested, their first host execution is for whoever enables the phase |
| FR1-d | CI at the final HEAD with main's `tools/ci.sh` (7e2c272) | (a) main's copy after the branch copy's guard flake (b) re-run the branch copy until the guard passes | D-127 says to run main's copy when the branch copy fails in the guard. Main's copy is also the one the pre-merge hook runs, and it includes D-128b |
