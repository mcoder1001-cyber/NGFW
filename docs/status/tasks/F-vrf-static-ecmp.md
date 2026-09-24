# F-vrf-static-ecmp — VRFs, static routes, ECMP, FIB browser, ping/traceroute

Branch `task/F-vrf-static-ecmp` (worktree `/root/ngfw-wt/F-vrf-static-ecmp`, slot 2, base `task/W-seed@8b7558e`, merged
`task/W-seed@df67a8e` at 18:05 on the manager's A1 safety update). Speculative base (D-114/D-120): **not merged with main**.

## What was built

| layer | what |
|---|---|
| contract (schema) | `vrfs.<name>.sourceSelect?[]{prefix, interface}`, `routing.static[].nextHops[].vrf?`, `routing.static[].viaFrr?` (sub-schemas `domains/ext/vrf-static-ecmp.ts`); rules `routing.vrf-static-ecmp-nexthop-vrf`, `routing.vrf-static-ecmp-single-path-weight`, `vrfs.vrf-static-ecmp-source-select-{interface-exists,unique}`; the reused rules tested (`vrfs.id-unique`, `routing.static-unique`, `routing.vrf-exists`) |
| contract (proto) | `Vrf.source_select = 3`, `StaticRoute.via_frr = 7`, `NextHop.vrf = 4`, `rpc ListRoutes` + `ListRoutes{Request,Response,Entry,Path}`, `VrfSourceSelect`; proto.md §11 section |
| agent: core | `RoutePath.next_hop_table` (core_model.proto 4): encode (path `table_id`), decode (dump `table_id` of a recursive path), dependencies (`vrf/<nh table>`), sort tie-break; VRF Retrieve skips `<owner>:svs:*` tables |
| agent: svs (new) | `svs.table/<id>`, `svs.interface/<if>`, `svs.route/<table>/<prefix>` (docs/agent/descriptors/svs.md): named tables via `ip_table_add_del`, enablements from `svs_dump`, routes from `ip_route_v2_dump` of the `svs` source + a boot-keyed applied-once record for the selected table (D-076/D-080; never desired state) |
| agent: wiring | `subsystems/vrf_static_ecmp.go`: svs registration with the persisted BootStore, the D-072 selector (`viaFrr`) through `sync.Once`, `SvsRange()` (slot range top / product high range); `desired/vrf_static_ecmp.go`: builder (svs objects, viaFrr notice) + assembler |
| agent: RPCs | `ListRoutes` (`rpc_vrf_static_ecmp.go` → `internal/actions/vrf-static-ecmp/fib.go`: one streamed dump per family through a 64k-reply stream, filters vrf/family/prefix/source (source pushed to VPP), bounded heap of offset+limit); Action `ping` (VPP ping plugin, default VRF only, count × interval ≤ 5 s, serialised) and `traceroute` → UNIMPLEMENTED |
| API | `GET /api/v1/state/routes` moved to `VrfStaticEcmpController` (agent-side paging, filters, paths/DPO detail; operationId `State_routes` kept for the CLI; all-VRF listing for `vrx show ip route`); `POST /api/v1/actions/:action` = Action bridge (ping/traceroute; agent INVALID_ARGUMENT → 400 with the field's pointer); `AgentClient.listRoutes` / `runAction`; fake behaviour in `features/vrf-static-ecmp/fake.ts` |
| UI | `/routing/vrfs` (VRF list with live FIB count, schema form incl. source VRF select), `/routing` tabs: static routes grid (live status) + ECMP path editor, FIB browser (ServerDataGrid, agent-paged), ping; en + fa |
| docs | `docs/user/routing/vrf-static-ecmp.md`, `docs/agent/descriptors/svs.md`, `docs/vpp-code-track.md` V-new |

## Acceptance — evidence (pasted real output)

All host runs on vrx-a, slot 2 (`w2`, tables 2000–2999, 10.2.0.0/16), shared lab lock, `NRestarts` read before and after
every run (1 → 1: the only restart, 18:41:08, was the `show trace` crash of D-128, not this task — Q7).

### 1. `vppctl show ip fib table <id> <prefix>` shows both weighted paths; Retrieve == desired

Full stack (`test/topology/vrf-static-ecmp/run.sh`, 19:35, real vrx-agent + vrx-api + rig): the committed ECMP default route of
VRF `red` (table 2021), next hops 3:1 — VPP's load-balance has 4 buckets, 3 to `.2` and 1 to `.3`:

```
vse_test.go:118: vppctl show ip fib table 2021 0.0.0.0/0:
w2:red, fib_index:1, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[interface:1, API:1, recursive-resolution:3, ]
0.0.0.0/0 fib:1 index:33 locks:3
  API refs:1 src-flags:added,contributing,active,
    path-list:[62] locks:2 flags:shared, uPRF-list:39 len:1 itfs:[4, ]
      path:[65] pl-index:62 ip4 weight=3 pref=1 recursive:  oper-flags:resolved,
        via 10.2.221.2 in fib:1 via-fib:20 via-dpo:[dpo-load-balance:22]
      path:[62] pl-index:62 ip4 weight=1 pref=1 recursive:  oper-flags:resolved,
        via 10.2.221.3 in fib:1 via-fib:48 via-dpo:[dpo-load-balance:50]

  default-route refs:1 entry-flags:drop, src-flags:added,
    path-list:[39] locks:1 flags:drop, uPRF-list:27 len:0 itfs:[]
      path:[39] pl-index:39 ip4 weight=1 pref=0 special:  cfg-flags:drop,
        [@0]: dpo-drop ip4

 forwarding:   unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:35 buckets:4 uRPF:39 to:[0:0]]
    [0-2] [@14]: dpo-load-balance: [proto:ip4 index:22 buckets:1 uRPF:70 to:[0:0]]
          [0] [@3]: arp-ipv4: via 10.2.221.2 loop221
    [3] [@14]: dpo-load-balance: [proto:ip4 index:50 buckets:1 uRPF:21 to:[0:0]]
          [0] [@3]: arp-ipv4: via 10.2.221.3 loop221
```

Next hop resolved in another VRF (`nextHops[].vrf: default` → path table 0) and the blackhole in `blue`, source VRF select:

```
vse_test.go:122: vppctl show ip fib table 2021 10.2.60.0/24:
w2:red, fib_index:1, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[interface:1, API:1, recursive-resolution:3, ]
10.2.60.0/24 fib:1 index:47 locks:2
  API refs:1 src-flags:added,contributing,active,
    path-list:[16] locks:2 flags:shared, uPRF-list:14 len:1 itfs:[1, ]
      path:[28] pl-index:16 ip4 weight=1 pref=1 recursive:  oper-flags:resolved,
        via 10.2.2.2 in fib:0 via-fib:45 via-dpo:[dpo-load-balance:49]

 forwarding:   unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:47 buckets:1 uRPF:14 to:[0:0]]
    [0] [@14]: dpo-load-balance: [proto:ip4 index:49 buckets:1 uRPF:16 to:[0:0]]
          [0] [@5]: ipv4 via 10.2.2.2 host-w2w0: mtu:1500 next:4 flags:[] ee452936f91002fe8136312c0800
vse_test.go:123: vppctl show ip fib table 2022 10.2.70.0/24:
w2:blue, fib_index:2, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[API:1, ]
10.2.70.0/24 fib:2 index:43 locks:2
  API refs:1 src-flags:added,contributing,active,
    path-list:[77] locks:2 flags:shared, uPRF-list:64 len:0 itfs:[]
      path:[22] pl-index:77 ip4 weight=1 pref=1 special:  cfg-flags:drop,
        [@0]: dpo-drop ip4

 forwarding:   unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:45 buckets:1 uRPF:64 to:[0:0]]
    [0] [@0]: dpo-drop ip4
vse_test.go:124: vppctl show svs:
Source VRF select interface to fib-index mappings:
 ipv4
  loop222 -> 3
 ipv6
  loop222 -> 3
```

Retrieve vs running through the API (drift view, `GET /api/v1/state/drift`) — no change:

```
vse_test.go:126: GET /api/v1/state/drift → {"subsystems":["interfaces","vrfs","routing"],"changes":[],"ignored":[{"pointer":"/management","rule":"agent.unimplemented-domain"},{"pointer":"/nat","rule":"agent.unimplemented-domain"},{"pointer":"/routing","rule":"agent.unsupported-field"},{"pointer":"/services","rule":"agent.unimplemented-domain"},{"pointer":"/system","rule":"agent.unimplemented-domain"},{"pointer":"/vpn","rule":"agent.unimplemented-domain"}]}
```

Descriptor level (`svs_integration_test.go`, 19:45, core + svs on the host VPP, 16 objects incl. IPv6 next hop in table 0,
svs entries IPv4 + IPv6):

```
svs_integration_test.go:119: vppctl show ip fib table 2960 10.2.50.0/24:
w2s:svs:2960, fib_index:2, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[API:1, ]
10.2.50.0/24 fib:2 index:71 locks:2
  svs refs:1 entry-flags:exclusive, src-flags:added,contributing,active,
    path-list:[19] locks:2 flags:exclusive, uPRF-list:73 len:0 itfs:[]
      path:[42] pl-index:19 ip4 weight=1 pref=0 exclusive:  oper-flags:resolved, cfg-flags:exclusive,
        [@0]: src-address,unicast lookup in w2s:red

 forwarding:   unicast-ip4-chain
  [@0]: dpo-load-balance: [proto:ip4 index:73 buckets:1 uRPF:73 to:[0:0]]
    [0] [@15]: src-address,unicast lookup in w2s:red
svs_integration_test.go:108: apply: {Created:16 Updated:0 Deleted:0 Unchanged:0 Failed:0 Reverted:0} in 157.844405ms
svs_integration_test.go:109: Retrieve == desired (16 objects)
svs_integration_test.go:155: resync after the simulated loss: {Created:4 Updated:0 Deleted:0 Unchanged:12 Failed:0 Reverted:0} in 25.825911ms
svs_integration_test.go:156: Retrieve == desired (16 objects)
svs_integration_test.go:171: V15 probe: table 2021 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
svs_integration_test.go:171: V15 probe: table 2022 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
svs_integration_test.go:171: V15 probe: table 2960 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
--- PASS: TestVrfStaticEcmpOnHost (0.58s)
ok  	ngfw/agent/internal/descriptors/svs	0.620s
```

### 2. FIB browser: 100 k prefixes in the slot's table, page of 1000

`fib_integration_test.go` (19:43–19:45): 100 000 API routes inside 10.2.0.0/16 in table 2100 (`w2f:fib`), then `ListRoutes`
through the lister the RPC uses. Only the page crosses gRPC (35–38 KB for 1000 routes); `total` is exact (100 000 + VPP's 5).

```
fib_integration_test.go:103: injected 100000 prefixes into table 2100 in 45.847094623s
fib_integration_test.go:119: ListRoutes offset=0 limit=1000 prefix="" source="": 1000 routes of total 100005 in 814ms; gRPC answer 34840 bytes (first 0.0.0.0/0, last 10.2.2.57/32)
fib_integration_test.go:119: ListRoutes offset=50000 limit=1000 prefix="" source="": 1000 routes of total 100005 in 977ms; gRPC answer 37526 bytes (first 10.2.125.201/32, last 10.2.128.99/32)
fib_integration_test.go:119: ListRoutes offset=99000 limit=1000 prefix="" source="": 1000 routes of total 100005 in 1.118s; gRPC answer 37691 bytes (first 10.2.253.100/31, last 10.2.255.254/31)
fib_integration_test.go:119: ListRoutes offset=0 limit=1000 prefix="10.2.200.0/24" source="": 384 routes of total 384 in 790ms; gRPC answer 14447 bytes (first 10.2.200.0/31, last 10.2.200.255/32)
fib_integration_test.go:119: ListRoutes offset=1000 limit=1000 prefix="" source="API": 1000 routes of total 100000 in 823ms; gRPC answer 34992 bytes (first 10.2.2.59/32, last 10.2.4.118/31)
fib_integration_test.go:76: cleanup: 100000 routes deleted in 1 pass(es), then table 2100, in 41.066274383s
--- PASS: TestFIBBrowser100kOnHost (91.59s)
ok  	ngfw/agent/internal/actions/vrf-static-ecmp	91.683s
```

Timing: 0.79–1.12 s per page on the loaded shared host (load average ≈ 20–30); an earlier run at load ≈ 50 took 1.1–1.9 s with
one 22 s outlier. The time is VPP walking and sending the whole table (`ip_route_v2_dump` has no cursor, V-new (d)); FAST
MODE: recorded, not tuned. Through the API (topology run): `GET /api/v1/state/routes?vrf=red&pageSize=1000 → 14 routes of 14 in 34ms`.
The first run of this test also found that govpp drops dump replies under load (Q8) — the lister now reads through a
64k-reply stream; the test's cleanup re-dumps until no API route is left before it deletes the table (V15, Q7).

### 3. Agent-restart simulation → VRFs + routes back within 30 s

Topology run: the agent (PID we started) stopped; behind its back, via binapi, the svs enablement, svs entries, the API routes of
tables 2021/2022 and the tables themselves deleted (routes before tables); agent started again:

```
vse_test.go:187: restart simulation: agent restarted, VRFs + routes + svs back=true in 359ms; agent log excerpt:
{"time":"2026-09-24T19:36:44.217719016+03:30","level":"INFO","msg":"VPP binary API connected","owner":"w2","component":"vpp","socket":"/run/vpp/api.sock"}
{"time":"2026-09-24T19:36:44.241123785+03:30","level":"INFO","msg":"reconcile start","owner":"w2","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing"]}
{"time":"2026-09-24T19:36:44.291838954+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"vrf/2022"}
{"time":"2026-09-24T19:36:44.297790041+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"ip.route/2021/0.0.0.0/0"}
{"time":"2026-09-24T19:36:44.303946131+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"ip.route/2021/10.2.60.0/24"}
{"time":"2026-09-24T19:36:44.305177302+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"ip.route/2022/10.2.70.0/24"}
{"time":"2026-09-24T19:36:44.310640109+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"svs.table/2960"}
{"time":"2026-09-24T19:36:44.31255064+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"svs.interface/loop222"}
{"time":"2026-09-24T19:36:44.316442594+03:30","level":"INFO","msg":"created","owner":"w2","component":"scheduler","key":"svs.route/2960/10.2.50.0/24"}
{"time":"2026-09-24T19:36:44.372903045+03:30","level":"INFO","msg":"reconcile done","owner":"w2","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing"],"status":"APPLY_STATUS_APPLIED","summary":"created:7 unchanged:10","reapplied":1,"duration":131753477,"err":""}
{"time":"2026-09-24T19:36:44.3729789+03:30","level":"INFO","msg":"resync finished","owner":"w2","status":"APPLY_STATUS_APPLIED","summary":"created:7 unchanged:10"}
vse_test.go:191: after the restart, vppctl show ip fib table 2021 0.0.0.0/0:
w2:red, fib_index:1, flow hash:[src dst sport dport proto flowlabel ] epoch:0 flags:none locks:[interface:1, API:1, recursive-resolution:3, ]
0.0.0.0/0 fib:1 index:33 locks:3
  API refs:1 src-flags:added,contributing,active,
    path-list:[18] locks:2 flags:shared, uPRF-list:66 len:1 itfs:[4, ]
      path:[69] pl-index:18 ip4 weight=3 pref=1 recursive:  oper-flags:resolved,
        via 10.2.221.2 in fib:1 via-fib:44 via-dpo:[dpo-load-balance:46]
      path:[72] pl-index:18 ip4 weight=1 pref=1 recursive:  oper-flags:resolved,
        via 10.2.221.3 in fib:1 via-fib:14 via-dpo:[dpo-load-balance:16]
```

(`vrf/2021` is not re-created: `loop221` keeps VPP's table alive by its binding, the VRF's name is intact, and the resync's
Reapplier re-asserts its API lock — `reapplied:1`.)

### 4. Rollback removes routes then tables, no stray /32 (V15)

Topology run, `POST /api/v1/config/rollback/1` (the baseline with only the loopbacks): the tables are gone (`show ip fib table`
prints nothing), no API route is left in `default`, and each table id re-created afterwards holds exactly VPP's 5 defaults —
nothing leaked into the next table that reuses the FIB index:

```
vse_test.go:196: rollback to rev 1 → {"status":"applied","revision":{"id":3,"createdAt":"2026-09-24T16:06:44.810Z","authorId":1,"author":"admin","comment":"rollback to revision 1","parentId":2,"hash":"75cfe1b9ae10bf4a6688c8265ada0e0c4c2aec3a18271491e9f2870579e754b6","txnId":"895b6231-6621-4198-95a2-13fc2e115847","kind":"rollback","secretVersions":null},"txnId":"895b6231-6621-4198-95a2-13fc2e115847","results":[{"key":"svs.route/2960/10.2.50.0/24","op":"delete","pointer":"","subsystem":"vrfs","code":"ok","message":""},{"key":"svs.interface/loop222","op":"delete","pointer":"","subsystem":"vrfs","code":"ok","message":""},{"key":"svs.table/2960","op":"delete","pointer":"","subsystem":"vrfs","code":"ok","message":""},{"key":"interface-ip.table/loop221","op":"delete","pointer":"","subsystem":"interfaces","code":"ok","message":""},{"key":"interface-ip/loop221/10.2.221.1/24","op":"recreate","pointer":"/interfaces/loop221/ipv4/0","subsystem":"interfaces","code":"ok","message":""},{"key":"ip.route/2022/10.2.70.0/24","op":"delete","pointer":"","subsystem":"routing","code":"ok","message":""},{"key":"ip.route/2021/10.2.60.0/24","op":"delete","pointer":"","subsystem":"routing","code":"ok","message":""},{"key":"ip.route/2021/0.0.0.0/0","op":"delete","pointer":"","subsystem":"routing","code":"ok","message":""},{"key":"vrf/2022","op":"delete","pointer":"","subsystem":"vrfs","code":"ok","message":""},{"key":"vrf/2021","op":"delete","pointer":"","subsystem":"vrfs","code":"ok","message":""}],"warnings":[{"pointer":"/management","message":"management is not implemented by this agent build (Health.subsystems) and is not applied","rule":"agent.unimplemented-domain"},{"pointer":"/nat","message":"nat is not implemented by this agent build (Health.subsystems) and is not applied","rule":"agent.unimplemented-domain"},{"pointer":"/routing","message":"routing protocols and policy are rendered by RF-1 (FRR), not by this agent build","rule":"agent.unsupported-field"},{"pointer":"/services","message":"services is not implemented by this agent build (Health.subsystems) and is not applied","rule":"agent.unimplemented-domain"},{"pointer":"/system","message":"system is not implemented by this agent build (Health.subsystems) and is not applied","rule":"agent.unimplemented-domain"},{"pointer":"/vpn","message":"vpn is not implemented by this agent build (Health.subsystems) and is not applied","rule":"agent.unimplemented-domain"}],"notApplied":[],"summary":{"created":0,"updated":1,"deleted":9,"unchanged":8,"failed":0,"reverted":0},"sync":{"state":"in-sync","reason":"","txnId":null,"since":"1970-01-01T00:00:00.000Z"}}
vse_test.go:198: after rollback, vppctl show ip fib table 2021:
vse_test.go:198: after rollback, vppctl show ip fib table 2022:
vse_test.go:198: after rollback, vppctl show ip fib table 2960:
vse_test.go:201: after rollback, API routes in default: {"page":1,"pageSize":100,"total":0,"vrf":"default","tableId":0,"retrievedAt":"2026-09-24T16:06:44.927Z","items":[]}
vse_test.go:204: V15 probe: table 2021 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
vse_test.go:204: V15 probe: table 2022 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
vse_test.go:204: V15 probe: table 2960 re-created holds 5 entries: 0.0.0.0/0(src 20) 240.0.0.0/4(src 1) 224.0.0.0/4(src 1) 0.0.0.0/32(src 20) 255.255.255.255/32(src 20)
```

### 5. Next hop in an undeclared VRF → 400 problem+json with `pointer`

```
vse_test.go:135: commit with a next hop in an undeclared VRF → 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic","warnings":[],"detail":"semantic validation failed","instance":"/api/v1/config/commit","errors":[{"pointer":"/routing/static/0/nextHops/0/vrf","message":"VRF 'nope' does not exist"}]}
```

(Same in the API e2e suite: `apps/api/test/e2e/vrf-static-ecmp.e2e.test.ts`, 5/5 passed, and at DryRun/Apply in the agent:
`routing.static.next-hop-vrf`.)

### 6. Ping from the UI returns replies from a rig address

The screenshot script (headless Chrome of P07a/P07b/P08, outside the product code) ran the Ping tab against the real stack:
`ping (en): PING 10.2.2.2 (default VRF): 3 packets transmitted, 1 received, 67% packet loss` (fa: the same). Through the API:

```
vse_test.go:146: POST /api/v1/actions/ping 10.2.2.2 → {"action":"ping","lines":["PING 10.2.2.2 (default VRF): 5 packets transmitted, 1 received, 80% packet loss"],"done":{"summary":"PING 10.2.2.2 (default VRF): 5 packets transmitted, 1 received, 80% packet loss","exitCode":0,"stats":{"transmitted":"5","received":"1","loss_pct":"80"}}}
vse_test.go:151: ping in VRF red → 400 {"type":"https://vrx.dev/problems/bad-request","title":"Bad request","status":400,"detail":"invalid action argument: vrf: ping in VRF \"red\": VPP's ping API has no table and always pings from the default VRF (docs/vpp-code-track.md V-new (F-vrf-static-ecmp))","instance":"/api/v1/actions/ping","errors":[{"pointer":"/vrf","message":"invalid action argument: vrf: ping in VRF \"red\": VPP's ping API has no table and always pings from the default VRF (docs/vpp-code-track.md V-new (F-vrf-static-ecmp))"}]}
```

Replies arrive from the rig peer 10.2.2.2 (`ns-w2-wan`); VPP's ping **API** under-counts them on a busy binary API (1–2 of 3
here; `vppctl ping` from VPP's CLI process gets 3/3) — V-new (b), not fixable without VPP code.

Screenshots (`docs/status/tasks/F-vrf-static-ecmp-screens/`, en + fa/RTL): `vrfs-list`, `vrf-form` (source VRF select),
`static-routes`, `ecmp-editor`, `fib-browser`, `ping-result`.

![ECMP editor](F-vrf-static-ecmp-screens/ecmp-editor-en.png)
![FIB browser, Persian](F-vrf-static-ecmp-screens/fib-browser-fa-rtl.png)
![Ping result](F-vrf-static-ecmp-screens/ping-result-en.png)

## Tests

| suite | where | result |
|---|---|---|
| schema rules (new + reused) | `packages/schema/src/semantic/vrf-static-ecmp.test.ts` | 12/12; whole schema suite 1226/1226 |
| proto round-trip / drift corpus | `packages/proto/test`, `apps/agent/internal/contracttest` (+ fixture `vrf-static-ecmp-full.json`) | pass |
| core route (ECMP weights, next-hop table, blackhole, deps, sort) | `apps/agent/internal/descriptors/core/route_vrf_static_ecmp_test.go` | pass (fake) |
| svs descriptors | `apps/agent/internal/descriptors/svs/svs_test.go` (6 tests, coretest model with VPP's duplicate-add behaviour) | pass |
| agent end to end (projection → reconcile → Retrieve, viaFrr skip + notice, validation, ListRoutes/Action RPCs) | `apps/agent/internal/agent/project_vrf_static_ecmp_test.go` | pass |
| lister + ping | `apps/agent/internal/actions/vrf-static-ecmp/vse_test.go` | pass |
| API e2e (PostgreSQL + fake agent) | `apps/api/test/e2e/vrf-static-ecmp.e2e.test.ts` | 5/5 |
| web model | `apps/web/src/domains/routing/vrf-static-ecmp/model.test.ts` | 4/4 |
| host: descriptors | `svs_integration_test.go`, `fib_integration_test.go` (`VRX_INTEGRATION=1`, one package at a time) | PASS (above) |
| host: full stack | `test/topology/vrf-static-ecmp/run.sh` (+ screenshots) | PASS 78.6 s |

## Shared hunks (append-only under `wave-A: F-vrf-static-ecmp` unless stated)

| id | file | hunk |
|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` | `svsTableName`, `svsInterfaceName`, `svsRouteName` in `Domains[VRFs]`; `registerVrfStaticEcmp(r, w)` in `Register()` |
| A2 | `apps/agent/internal/agent/projection.go` | `desired.VrfStaticEcmp(p, ds, in, vrfID, subsystems.SvsRange())` in `project()`; `desired.AssembleVrfStaticEcmp(ds, kvs, nameOf)` in `assemble()`; **in P08's `routing.static` blocks** (allowed this wave): the next-hop-VRF lines in project (→ `RoutePath.NextHopTable`) and assemble (→ `NextHop.Vrf`) |
| A4 | `apps/agent/internal/agent/server.go` | `Action`'s stream parameter named (`_` → `stream`, needed by every case); `case ActionRequest_Ping` / `ActionRequest_Traceroute` |
| A6 | `apps/agent/internal/descriptors/core/coretest/vrf_static_ecmp.go` | new file (svs, fib_source_dump, ping, `ip_route_v2_dump` with `src` — installed only by `InstallVrfStaticEcmp`) |
| A7 | `docs/vpp-code-track.md` | `### V-new (F-vrf-static-ecmp)` appended (no anchor in the file) |
| C1 | `packages/schema/src/domains/{vrfs,routing}.ts` | `sourceSelect: vrfSourceSelect`, `vrf: nextHopVrf`, `viaFrr: staticRouteViaFrr` under the anchors **+ one import line each, outside the anchor** (Q5) |
| C2/C3 | `packages/schema/src/semantic/index.ts`, `packages/schema/src/index.ts` | import + spread; `export *` |
| C4 | `packages/proto/test/fixtures/vrf-static-ecmp-full.json` | new file; **`packages/proto/test/desired-state.test.ts`**: `toEqual` → `toMatchObject` on the `vrfs['customer-a']` line (Q6) |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | `rpc ListRoutes` under the service anchor; `Vrf.source_select = 3`, `StaticRoute.via_frr = 7` under their anchors; `NextHop.vrf = 4` (no anchor; NextHop is touched only by this task); messages in `// ----- F-vrf-static-ecmp -----` |
| C6 | `docs/contracts/proto.md` | `### F-vrf-static-ecmp: ListRoutes` |
| C7 | generated | `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go`, `docs/user/cli/reference.md` (regenerated, never hand-edited) |
| P1 | `apps/api/src/app.module.ts` | import + `...vrfStaticEcmpFeature.controllers` / `.providers` |
| P2 | `apps/api/src/state/state.controller.ts` | the `routes()` handler removed (one hunk, blank separator kept) + its now-unused `RoutesQuery`/`RouteOut` consts, `routesOf()` and the `canonicalPrefix` import (prettier collapsed that import to one line) |
| P3 | `apps/api/src/actions/actions.controller.ts` | owned this wave: the Action bridge |
| P4 | `apps/api/src/agent/agent.client.ts` | 5 type imports; `listRoutes()`, `runAction()` (generic, for F-neighbors-ra / F-nat44-ed-sessions) |
| P5 | `apps/api/src/testing/fake-agent.ts` | `...vrfStaticEcmpFake(this)` + one import line; **the base `action` stub removed** (TS2783 duplicate key; the feature fake answers every other action UNIMPLEMENTED — Q10) |
| W1 | `apps/web/src/router.tsx` | `/routing/vrfs` → `VrfsPage`, `/routing` → `RoutingPage` |
| W2 | `apps/web/src/nav/nav.ts`, `nav.test.ts` | `'vrfs'`, `'routing'` in `BUILT_DOMAINS` and in the expected `available` list |
| W3 | `apps/web/src/i18n.ts` | en/fa imports, namespace, resources |
| — | `apps/web/src/App.test.tsx` | the "not yet available" example moves from `/routing/vrfs` (built now) to `/system/management` (one hunk, not owned; needed) |

## Out of scope (not built)

BGP/OSPF/IS-IS/RIP and FRR rendering of `viaFrr` routes (P12 / F-bfd-redistribution: the flag and the selector are here);
neighbours/ARP (F-neighbors-ra); uRPF/ABF/PBR (F-rpf-adl-pbr); MPLS labels; multicast routes; traceroute without a Linux path
(UNIMPLEMENTED + V-new); VRF leaking via route maps (only the next hop in another VRF); the CLI `ping` body fix (Q9); the core
descriptor README row for `next_hop_table` (`core/README.md` is not an owned file — `core_model.proto` documents the field).

## Decisions taken (options → choice)

| # | decision | options | chosen, why |
|---|---|---|---|
| 1 | svs table creation | (a) `svs_table_add_del` (+ boot record to balance its counted lock) (b) named `ip_table_add_del` only | (b): idempotent single API lock, ownership by name, no underflow risk; svs route/enable only need the table to exist |
| 2 | svs route readback (selected table not in the dump) | (a) write-only D-063 (b) dump existence + boot-keyed applied-once record (c) echo desired | (b): existence from VPP, attribute from what this agent applied on this VPP instance; unknown → re-program; (c) is forbidden |
| 3 | svs table ids | (a) new config field (contract reshape) (b) agent allocation | (b): hashed per interface in the top of the slot range / 4294967040–4294967294, skipping declared VRF ids |
| 4 | new schema fields' presence | (a) `viaFrr` default false (b) optional without default | (b): existing documents parse identically, no drift on Retrieve, no change to P08/P02 tests |
| 5 | next-hop VRF canonical form | (a) always set (b) only when it differs and the hop is address-only | (b) + semantic rule refusing the route's own VRF / an interface hop — Retrieve can then equal running |
| 6 | ping | (a) API ping with limits (b) CLI-in-band ping | (a): `cli_inband` would run the CLI ping loop inside the API process and swallow other clients' socket events; limits: default VRF only, count × interval ≤ 5 s, serialised; traceroute UNIMPLEMENTED (prompt default) |
| 7 | FIB paging | (a) agent pages a full dump per request (bounded heap) (b) cache snapshots | (a): FAST MODE (no performance work), memory bounded by the window; 64k-reply stream against govpp drops (Q8) |
| 8 | `/state/routes` | (a) new route (b) replace in place | (b) with operationId `State_routes` kept (the CLI binds by it), old item fields kept, new fields added, all-VRF listing when `vrf` is absent |
| 9 | UI writes | (a) PUT `vrfs/<name>`, `routing/static` (b) merge patch of the root key | (b): the API reads a percent-encoded `/` as part of one pointer segment |

## Open questions

See `F-vrf-static-ecmp-questions.md`: Q1 contract numbers (NextHop 4 to add to §2), Q2/Q3 traceroute and VRF-aware ping
(defaults taken), Q4 `examples.test.ts` sibling regex, Q5/Q6 hunks outside anchors, Q7 slot-2 timeline around the 18:41 crash,
Q8 govpp dump drops (every descriptor), Q9 CLI ping body, Q10 fake Action dispatch, Q11 routing tabs location.
