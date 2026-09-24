# F-neighbors-ra — ARP/ND table, static neighbours, proxy-ARP/ND, IPv6 RA, DAD

Branch `task/F-neighbors-ra` (slot 9, `w9`), base `task/W-seed@8b7558e` (speculative, D-114/D-120), `task/W-seed@df67a8e`
merged at 18:10 on the manager's safety update (TD-5 rings/quiesce, D-113 rig fix) → `5a6fe26`. Contract details:
`F-neighbors-ra-contract.md`; questions and decisions with options: `F-neighbors-ra-questions.md` (Q1–Q10).

## What
**Schema** (`packages/schema/src/domains/ext/neighbors-ra.ts`, rules in `semantic/neighbors-ra.ts`):
`interfaces.<if>.{ipv6Ra, proxyArp, proxyNd}` (and on sub-interfaces), `vrfs.<name>.proxyArpRanges`, `routing.neighbors{static,
ipv4Limits, ipv6Limits, dad}`. Absent/default = off everywhere (`ipv6Ra` defaults = VPP's fresh state: suppressed, 600/200/150 s);
no default is added to existing objects. Refinements: RA min ≤ 0.75 × max, max 4–1800, lifetime 0 or > max and ≤ 9000,
prefix lifetimes ≥ 1 and preferred ≤ valid, proxy-ARP range IPv4 low ≤ high, MAC unicast. Semantic rules: IPv6 needed under a
real RA config / proxy ND, SLAAC prefixes /64, unique proxy-ND addresses / ranges / static neighbours, static neighbour in a
connected subnet of its interface (or its unnumbered source; fe80::/10 when it has IPv6; never its own address).

**Proto** (numbers from wave-A-hotspots §2): Interface 15–17, Subinterface 13–15, Vrf 4, RoutingConfig 10, `rpc ListNeighbors`,
`ActionRequest.arp_flush` (4), `EVENT_KIND_NEIGHBOR_CHANGED` (10); proto.md §11 documents all three.

**Agent** (DF-2 descriptors used name for name, D-104):
- `desired/neighbors_ra.go`: projection onto `ip6-nd.ra-config` (only when not VPP's default state — DF-2's Retrieve omits
  those), `ip6-nd.ra-prefix` (`NormalizeRaPrefix`), `arp.proxy-interface`, `arp.proxy-range`, `ip-neighbor.neighbor`,
  `ip-neighbor.config` (globals owner only, both families always, VPP defaults when unset), `ip6-nd.dad` (globals owner
  only), `ip6-nd.proxy` (only with `VRX_DF2_PROXY_ND=1`). Leaves a slot agent does not apply → `agent.unsupported-field`
  warnings (which also keep them out of `/state/drift`). Assembler adds the retrieved leaves back (sorted lists; the stored
  document's "off" leaves reported in their off state).
- `subsystems/neighbors_ra.go`: `ipneighbor/ip6nd/arp.Register` with `df2.WithClaims(Wiring.KeyedClaims("acl"))` (persisted,
  D-080), `arp` table range = `SlotIDRange()` (slot 9000–9999; nil for the product agent), `RegisterGlobals` only for
  `Env.GlobalsOwner` (D-071), `RegisterProxyNd` only on opt-in (D-064); the neighbour-event watcher.
- `actions/neighbors-ra`: lister (`ip_neighbor_dump` per nameable interface and family, VRF from `sw_interface_get_table`,
  filter/sort/page on the agent), flush (learned entries deleted one by one — `ip_neighbor_flush` would delete the
  configuration's static entries too), `want_ip_neighbor_events_v2` per interface, 1 Hz coalescer. Never `sw_if_index` ~0
  or 0 (VPP maps 0 to "all" for watchers).
- `rpc_neighbors_ra.go`: `ListNeighbors`, the `arp_flush` Action case (flush "all" = the stored configuration's interfaces).
- DF-2 gap fix (`descriptors/ip_neighbor`): Retrieve dumped `sw_if_index ~0` (every slot's table); now per candidate
  interface. Named test `TestRetrieveNeverDumpsAllInterfaces` fails on the old code (below).
- `coretest/neighbors_ra.go`: agent-level fake of ip_neighbor / ip6_nd / ip6_dad / arp with VPP's semantics (IP6_NOT_ENABLED,
  toggle-style RA config, ~0 counting).

**API** (`apps/api/src/features/neighbors-ra`): `NeighborsRaController` — `GET /api/v1/state/neighbors` (replaces the 501
stub; paged `page/pageSize`, filters `vrf/interface/family/state/search`, sort) and `POST /api/v1/actions/arp-flush
{interface?, family?}` (static route beating `:action`; operator+; audited with resource `arp-flush/<if|*>`, before/after;
an interface the agent refuses → 400 with pointer `/interface`); `AgentClient.listNeighbors/arpFlush`; fake behaviour;
topic `neighbor.events` + EventKind case.

**Web** (`apps/web/src/domains/routing/neighbors-ra`): *Routing → Neighbours* — live table (ServerDataGrid, agent-side paging,
polled every 5 s and invalidated by `neighbor.events`), filters, *Flush…* with a confirm dialog, *Static entries and limits*
(SchemaForm over `routing.neighbors`, P08 `dropPhantomOptionals`); the drawer's per-interface group *IPv6 RA, proxy ARP/ND*
(schema group `neighbors-ra`, strings merged into the `interfaces` namespace). en + fa.

**Docs**: `docs/user/routing/neighbors-ra.md` (screen, examples, REST, CLI, VPP CLI, screenshots en+fa);
`docs/agent/descriptors/ip_neighbor.md` (per-interface Retrieve, flush).

## Shared hunks (hotspots; every insert directly under the `wave-A: F-neighbors-ra` anchor unless noted)
| file | hunk |
|---|---|
| A1 `apps/agent/internal/subsystems/subsystems.go` | `Domains`: 4 lines (Interfaces), 1 (VRFs), 3 (Routing); `Register`: `if err := w.registerNeighborsRa(r)…` (3 lines); **`Connected`: 2 lines after `w.dhcpClient.Reconnected()` — no anchor there (Q2)** |
| A2 `apps/agent/internal/agent/projection.go` | `desired.NeighborsRa(p, ds, in, vrfID)` in `project()`; `desired.AssembleNeighborsRa(ds, kvs, in, stored, nameOf)` in `assemble()` |
| A4 `apps/agent/internal/agent/server.go` | `case *vrxv1.ActionRequest_ArpFlush:`; the `Action` parameter `_` → `stream` (F-vrf-static-ecmp needs the same) |
| A6 `apps/agent/internal/descriptors/core/coretest/fakevpp.go` | **one line in `New()`: `v.installNeighborsRa()`** (existing coretest files are read-only, but without the hook P08's service tests fail on the new families' dumps) |
| C1 `packages/schema/src/domains/{interfaces,vrfs,routing}.ts` | key lines (3 + 3 in interfaces.ts, 1 in vrfs.ts, 1 in routing.ts); **one import line per file after the last import — no import anchors (Q4)** |
| C2/C3 `packages/schema/src/{semantic/index.ts,index.ts}` | import + spread; `export * from './domains/ext/neighbors-ra.js'` |
| C5 `packages/proto/vrx/v1/dataplane.proto` | RPC, EventKind 10, oneof 4, Interface 15–17, Subinterface 13–15, Vrf 4, RoutingConfig 10; messages in the `// ----- F-neighbors-ra -----` section |
| C6 `docs/contracts/proto.md` | `### F-neighbors-ra: ListNeighbors`, `…: ActionRequest.arp_flush (4)`, `…: EventKind.EVENT_KIND_NEIGHBOR_CHANGED (10)` |
| C4 | `packages/proto/test/fixtures/neighbors-ra-full.json` (new); **`packages/proto/test/desired-state.test.ts`: one expectation lists `proxyArpRanges: []` (Q6)** |
| C7 generated | `apps/agent/gen/**`, `packages/proto/gen/ts/**`, `packages/api-client/src/generated/schema.d.ts`, `apps/cli/internal/api/operations_gen.go` (regenerated; `docs/user/cli/reference.md` unchanged) |
| P1 `apps/api/src/app.module.ts` | import, `...neighborsRaFeature.controllers`, `...neighborsRaFeature.providers` |
| P2 `apps/api/src/state/state.controller.ts` | the `GET neighbors` 501 block deleted in one hunk (blank separator kept) |
| P4 `apps/api/src/agent/agent.client.ts` | 5 type imports (ActionDone/ActionOutput aliased `NeighborsRa…` so F-vrf-static-ecmp's imports cannot collide); `listNeighbors`, `arpFlush` |
| P5 `apps/api/src/testing/fake-agent.ts` | `listNeighbors: neighborsRaFake(this).listNeighbors,`; **one import line after the last import (no anchor)** |
| P6 `apps/api/src/infra/bus.ts`, `telemetry/relay.service.ts` | `'neighbor.events',`; `case EventKind.EVENT_KIND_NEIGHBOR_CHANGED: return 'neighbor.events';` |
| W1 `apps/web/src/router.tsx` | `routing/neighbors` lazy route |
| W2 `apps/web/src/nav/nav.ts`, `nav.test.ts` | routing-group NavItem `neighbors` (labelKey `neighbors-ra:nav`); `'neighbors'` in the available list |
| W3 `apps/web/src/i18n.ts` | en/fa imports + the side-effect import of `interface-strings` (drawer strings), `'neighbors-ra'` in `NAMESPACES`, `en`, `fa` |
| W5 | none: the RA/proxy fields need nothing beyond their SchemaForm group |
| DF-2 (gap-only) | `apps/agent/internal/descriptors/ip_neighbor/neighbor.go` Retrieve per interface + `neighbor_shared_test.go`; `docs/agent/descriptors/ip_neighbor.md` |

## How verified

### Host check on the real VPP — `test/topology/neighbors-ra/run.sh -run TestNeighborsRaHost` (slot 9, 19:24)
Prefixed loopbacks `loop901` (VRF `w9red` = table 9001) and `loop902`, no rig, **no packets** (learned entries injected with
`ip_neighbor_add_del` flags NONE, the entry the data plane creates after ARP/ND). NRestarts checked before/after.
```
    neighbors_ra_test.go:99: systemctl show vpp -p NRestarts (before) = 1
    neighbors_ra_test.go:122: base revision 1
    neighbors_ra_test.go:129: commit with a static neighbour outside every connected subnet → 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"tier":"semantic","warnings":[],"detail":"semantic validation failed","instance":"/api/v1/config/commit","errors":[{"pointer":"/routing/neighbors/static/0/ip","message":"10.9.200.5 is outside every connected subne…
    neighbors_ra_test.go:154: commit nra-feature: revision 2, warnings [… {"message":"duplicate address detection is VPP-wide: only the globals owner applies it (D-071); not applied by this agent","pointer":"/routing/neighbors/dad","rule":"agent.unsupported-field"} …]
    neighbors_ra_test.go:161: agent Retrieve (our objects):
        { "interfaces": {
            "loop901": { "enabled": true, "ipv4": ["10.9.1.1/24"], "ipv6": ["2001:db8:9:1::1/64"], "vrf": "w9red", "promiscuous": false,
              "ipv6Ra": { "suppress": false, "managed": true, "other": true, "lifetimeSec": 1800, "maxIntervalSec": 600, "minIntervalSec": 200,
                "prefixes": { "2001:db8:9:1::/64": { "validSec": 86400, "preferredSec": 14400, "offLink": false, "noAutoconfig": false } } },
              "proxyArp": true },
            "loop902": { "enabled": true, "ipv4": ["10.9.2.1/24"], "ipv6": ["2001:db8:9:2::1/64"], "vrf": "default", "promiscuous": false } },
          "vrfs": { "w9red": { "id": 9001, "proxyArpRanges": [ { "low": "10.9.3.10", "high": "10.9.3.20" } ] } },
          "routing": { "neighbors": { "static": [
                { "interface": "loop901", "ip": "10.9.1.50", "mac": "02:00:00:00:91:50", "noFibEntry": false },
                { "interface": "loop902", "ip": "2001:db8:9:2::50", "mac": "02:00:00:00:92:50", "noFibEntry": true } ] } } }
    neighbors_ra_test.go:171: vppctl show ip neighbors (ours):
               .3787                10.9.1.50                  S    02:00:00:00:91:50 loop901
               .3782            2001:db8:9:2::50              SN    02:00:00:00:92:50 loop902
    neighbors_ra_test.go:176: vppctl show ip6 interface loop901:
          …
          Advertised Prefixes:
              prefix 2001:db8:9:1::, length 64
            …
            ND router advertisements are sent every 600.0 seconds (min interval is 200.0)
            ND router advertisements live for 1800 seconds
            Hosts use stateless autoconfig for addresses
    neighbors_ra_test.go:181: vppctl show arp proxy:
        Proxy arps enabled for:
        Fib_index 1   10.9.3.10 - 10.9.3.20
    neighbors_ra_test.go:183: vppctl show interface features loop901 (arp arc): ["  arp-proxy\r"]
```
`/state/drift` had no change under our interfaces, VRF or `/routing/neighbors` (asserted). DAD was **not** applied by the
slot agent (D-071): a warning, and Retrieve has no `dad`.

Live table, flush, audit (same run):
```
    neighbors_ra_test.go:197: GET /api/v1/state/neighbors?interface=loop901 → {"page":1,"pageSize":100,"total":4,…,"items":[{"interface":"loop901","ip":"10.9.1.20","mac":"02:00:00:00:91:20","family":"ipv4","state":"dynamic",…,"vrf":"w9red","tableId":9001},…,{"interface":"loop901","ip":"10.9.1.50",…,"state":"static",…},{"interface":"loop901","ip":"2001:db8:9:1::20",…,"family":"ipv6","state":"dynamic",…}]}
    neighbors_ra_test.go:202: GET …?state=dynamic&family=ipv4&sort=ip&page=2&pageSize=1 → {"page":2,"pageSize":1,"total":2,…,"items":[{"interface":"loop901","ip":"10.9.1.21",…}]}
    neighbors_ra_test.go:207: POST /api/v1/actions/arp-flush {interface:loop901} → {"deleted":3,"interfaces":1,"summary":"deleted 3 learned entries on 1 interfaces","lines":["loop901 ipv4: deleted 2 learned entries","loop901 ipv6: deleted 1 learned entries"]}
    neighbors_ra_test.go:215: vppctl show ip neighbors after the flush (ours):
               .5492                10.9.1.50                  S    02:00:00:00:91:50 loop901
               .5487            2001:db8:9:2::50              SN    02:00:00:00:92:50 loop902
               .1391                10.9.2.30                  D    02:00:00:00:92:30 loop902
    neighbors_ra_test.go:217: POST /api/v1/actions/arp-flush {} (every configured interface) → {"deleted":1,"interfaces":2,…}
    neighbors_ra_test.go:222: POST /api/v1/actions/arp-flush {interface:local0} → 400 {"type":"https://vrx.dev/problems/bad-request",…,"errors":[{"pointer":"/interface","message":"agent: invalid request: interface \"local0\": iface: no owned interface with this id: \"local0\" (owner \"w9\")"}]}
    neighbors_ra_test.go:230: audit (newest first): {"items":[{"id":11,…,"action":"POST /api/v1/actions/arp-flush","resource":"arp-flush/local0","before":{"family":"","interface":"local0"},"after":null,"result":"failure","status":400},{"id":10,…,"resource":"arp-flush/*","before":{"family":"","interface":""},"after":{"deleted":1,"exitCode":0,"interfaces":2},"result":"success","status":200},…
```
Restart safety (agent stopped; static neighbours, RA prefix + RA config, proxy-ARP interface and range deleted via binapi,
dependents only — the interfaces stay; agent started; no config API call):
```
    neighbors_ra_test.go:245: simulated loss: static neighbours, RA prefix + RA config, proxy-ARP interface and range deleted via binapi (agent stopped)
    neighbors_ra_test.go:260: agent log: {"time":"2026-09-24T19:24:59.764754699+03:30","level":"INFO","msg":"vrx-agent listening","owner":"w9",…}
    neighbors_ra_test.go:260: agent log: {"time":"2026-09-24T19:24:59.766366671+03:30","level":"INFO","msg":"reconcile start","owner":"w9","txn_id":"","mode":"resync","domains":["interfaces","vrfs","routing"]}
    neighbors_ra_test.go:260: agent log: {"time":"2026-09-24T19:24:59.818260287+03:30","level":"INFO","msg":"reconcile done","owner":"w9","txn_id":"","mode":"resync",…,"status":"APPLY_STATUS_APPLIED","summary":"created:6  unchanged:12","reapplied":1,"duration":51951645,"err":""}
    neighbors_ra_test.go:266: restart safety: everything back 0.25 s after the agent start (no config API call)
```
Rollback to the base revision:
```
    neighbors_ra_test.go:272: POST /api/v1/config/rollback/1 → {"status":"applied","revision":{"id":3,…,"kind":"rollback",…},…,"results":[{"key":"arp.proxy-interface/loop90…
    neighbors_ra_test.go:274: agent Retrieve after the rollback (our objects):
        { "interfaces": {
            "loop901": { "enabled": true, "ipv4": ["10.9.1.1/24"], "ipv6": ["2001:db8:9:1::1/64"], "vrf": "w9red", "promiscuous": false },
            "loop902": { "enabled": true, "ipv4": ["10.9.2.1/24"], "ipv6": ["2001:db8:9:2::1/64"], "vrf": "default", "promiscuous": false } },
          "vrfs": { "w9red": { "id": 9001 } },
          "routing": {} }
    neighbors_ra_test.go:282: after the rollback — show ip neighbors (ours): []; show arp proxy: ""; arp-proxy feature on loop901: []
    neighbors_ra_test.go:283: after the rollback — show ip6 interface loop901:  … Advertised Prefixes: (none) …
    neighbors_ra_test.go:320: cleanup commit → 200 {"status":"applied",…,"comment":"nra-cleanup",…}
    neighbors_ra_test.go:323: cleanup: loop901/loop902 in VPP: false; show interface lines: []; show ip neighbors (ours): []
    harness_test.go:59: pg-test drop w9: <nil>   drop database vrx_w9 · drop role vrx_w9 · ok nothing named vrx_w9 / vrx_w9 remains
    neighbors_ra_test.go:102: systemctl show vpp -p NRestarts (after) = 1
--- PASS: TestNeighborsRaHost (17.33s)
ok  	ngfw/test/topology/neighbors-ra	17.383s
```
(NRestarts was already 1 from the 18:41 crash, before this task's first host call — Q7.)

### Neighbour events on the real VPP — `VRX_INTEGRATION=1 go test -run TestNeighborWatchOnHost ./internal/subsystems/`
The product agent publishes these only once TD-8 wires `Env.Publish` (Q3); this drives the same `RunNeighborWatch`:
```
=== RUN   TestNeighborWatchOnHost
    neighbors_ra_integration_test.go:64: show ip neighbor-watcher (lines naming loop971 or pid 7238241):
        Key: [loop971, 0.0.0.0]
        [pid:7238241, client:-2147483646]
    neighbors_ra_integration_test.go:92: event: kind=EVENT_KIND_NEIGHBOR_CHANGED interface=loop971 message="neighbours on loop971: 1 added" attributes=map[added:1 removed:0 updated:0]
    neighbors_ra_integration_test.go:104: event: kind=EVENT_KIND_NEIGHBOR_CHANGED interface=loop971 message="neighbours on loop971: 1 removed" attributes=map[added:0 removed:1 updated:0]
--- PASS: TestNeighborWatchOnHost (0.55s)
ok  	ngfw/agent/internal/subsystems	0.620s
NRestarts=1 (before and after)
```

### DF-2 gap fix, negative control
```
$ go test -count=1 ./internal/descriptors/ip_neighbor/            # with the fix
ok  	ngfw/agent/internal/descriptors/ip_neighbor	0.042s
$ git stash push internal/descriptors/ip_neighbor/neighbor.go; go test -run TestRetrieveNeverDumpsAllInterfaces ./internal/descriptors/ip_neighbor/
    neighbor_shared_test.go:34: ip_neighbor_dump for sw_if_index 4294967295 (all = 4294967295)
FAIL
```

### Unit and e2e suites (fake VPP / fake agent)
- schema: `vitest` 38 files, 1230 tests (was 1214; +16 `semantic/neighbors-ra.test.ts`); proto: 70 tests incl. the
  `neighbors-ra-full.json` fixture in both corpus tests; Go `contracttest` (drift guard both ways, strict decode of the fixture).
- agent: `actions/neighbors-ra` (lister filters/sort/paging, flush keeps statics, never ~0/0, vanished entries, coalescer),
  `desired` (projection non-owner/owner/proxy-ND, assembler round trip), `agent/project_neighbors_ra_test.go` (Apply →
  Retrieve == canonical → idempotent → rollback; restart simulation; globals only for the owner; proxy ND opt-in;
  validation pointer; ListNeighbors + arp_flush RPCs), `subsystems` (registration, id range, watcher, reconnect); all
  with `-race`; golangci-lint 0 issues on the touched packages.
- api: `features/neighbors-ra/neighbors-ra.controller.test.ts` (6) and `test/e2e/neighbors-ra.e2e.test.ts` (4, host
  PostgreSQL + Valkey + fake agent, slot DB created and dropped):
```
   ✓ neighbors-ra e2e (PostgreSQL + fake agent) > commits RA, proxy-ARP and static neighbours through the generic pointer routes
   ✓ … > static neighbour outside every connected subnet → 400 problem+json with the pointer (acceptance)
   ✓ … > GET /state/neighbors: the agent table, filtered and paged (readonly may read)
   ✓ … > POST /actions/arp-flush is this feature’s route: RBAC, body validation, audited, agent errors as problems
      Tests  4 passed (4)
```
- web: `NeighborsPage.test.tsx` (7), locale parity, nav; full web suite 15 files / 102 tests.

### Screenshots — `TestNeighborsRaScreenshots` (production build under `vite preview`, real API + agent + VPP)
Headless Chrome-for-Testing + playwright-core from the npx cache (Playwright is not installed; nothing installed or
committed; script in the session scratchpad — the P07a/P07b/P08 approach). The flush was done through the UI:
```
    shots_test.go:100: screenshots:
        neighbors-table-fa.png  html dir/lang=rtl/fa  gridRows=11  pageErrors=0
        neighbors-flush-confirm-fa.png  html dir/lang=rtl/fa  gridRows=11  pageErrors=0
        neighbors-static-fa.png  html dir/lang=rtl/fa  gridRows=0  pageErrors=0
        interface-drawer-ra-fa.png  html dir/lang=rtl/fa  gridRows=4  pageErrors=0
        interface-drawer-ra-group-fa.png  html dir/lang=rtl/fa  gridRows=4  pageErrors=0
        neighbors-table-en.png  html dir/lang=ltr/en  gridRows=11  pageErrors=0
        neighbors-flush-confirm-en.png  html dir/lang=ltr/en  gridRows=11  pageErrors=0
        neighbors-flush-done-en.png  html dir/lang=ltr/en  gridRows=6  pageErrors=0
        neighbors-static-en.png  html dir/lang=ltr/en  gridRows=0  pageErrors=0
        interface-drawer-ra-en.png  html dir/lang=ltr/en  gridRows=4  pageErrors=0
        interface-drawer-ra-group-en.png  html dir/lang=ltr/en  gridRows=4  pageErrors=0
    shots_test.go:105: vppctl show ip neighbors after the UI flush of loop901 (ours):
             44.0948                10.9.1.50                  S    02:00:00:00:91:50 loop901
             43.2788                10.9.2.30                  D    02:00:00:00:92:30 loop902
             42.5765                10.9.2.31                  D    02:00:00:00:92:31 loop902
             42.5734            2001:db8:9:2::31               D    02:00:00:00:92:31 loop902
             44.0924            2001:db8:9:2::50              SN    02:00:00:00:92:50 loop902
    shots_test.go:107: audit (newest first): {"items":[{"id":7,…,"action":"POST /api/v1/actions/arp-flush","resource":"arp-flush/loop901",…
--- PASS: TestNeighborsRaScreenshots (54.29s)
```
In the docs: `docs/user/routing/img/neighbors-ra-{table-en,table-fa,flush-confirm-en,flush-done-en,static-en,
interface-drawer-ra-group-en,interface-drawer-ra-group-fa}.png`.

### CI
`TMPDIR=/tmp/g-w9 tools/ci.sh --base main` on `c9858d8` (the code is unchanged after it; later commits are this file only).
This worktree's `tools/ci.sh` predates main's D-127 fix: its contract guard (the first step) failed 3/4 `check` runs and
8/8 full runs on the SIGPIPE race although the branch carries its `contract(…)` commits (Q8). The passing run is this
worktree's own `tools/ci.sh` with exactly main's D-127 hunk (`7edac8c`) applied to a temporary copy
(`patch -p2 < <(git show 7edac8c -- tools/ci.sh)`, `ROOT` from `git rev-parse`, every step unmodified; `tools/ci.sh` itself
is not edited):
```
== VRX CI gate: quick ==
branch    task/F-neighbors-ra @ c9858d8   (base: main)
== contract guard: HEAD vs main ==
ok — contract commit(s) on the branch:
  65fbe25 contract(schema): per-interface neighbour fields ordered proxy ARP, proxy ND, then the RA block (x-vrx-ui order only)
  744392e contract(proto): EVENT_KIND_NEIGHBOR_CHANGED comment lists the updated/interfaces attributes (comment only)
  f1fccf2 contract(schema): ipv6Ra prefix lifetimes are at least 1 s
  61412e5 contract(proto): ListNeighbors, arp_flush, neighbour event
  19935ee contract(schema): neighbours, RA, proxy-ARP/ND
  (+ W-seed/P08's own contract commits)
== generate + generated-output gate ==            (clean)
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src · no Dockerfile/compose · no kill-by-pattern · no secret-shaped strings · gitleaks: no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    4 cached, 30 total Time:    4m15.782s
== apps/agent: make lint test build ==            golangci-lint 0 issues · go test -race: ok=91 FAIL=0 no-test-files=161
== apps/cli: make lint test build ==              ok (7 packages)
== test/ Go modules, unit mode ==
test/topology/neighbors-ra: gofmt ok · go vet ok · ok  	ngfw/test/topology/neighbors-ra	0.017s;
  warnings: commit subject not in Conventional Commits form: review(W-seed): verify   (W-seed's commit)
  mode quick · wall time 8m01s · logs /root/ngfw-wt/logs/ci/F-neighbors-ra-20260924-194156-2874995
CI GATE PASSED
```

## Acceptance
- [x] `vppctl show ip neighbors` lists the static entries (S / SN); `vppctl show ip6 interface loop901` shows the RA timers,
      lifetime and the advertised prefix; `show arp proxy` + the `arp-proxy` feature; agent Retrieve == the committed leaves
- [x] Agent-restart simulation → static neighbours, RA and proxy-ARP back 0.25 s after the agent start (log excerpt)
- [x] Rollback removes static neighbours, RA config, proxy entries (Retrieve pasted, vppctl empty)
- [x] Static neighbour outside every connected subnet → 400 problem+json, pointer `/routing/neighbors/static/0/ip`
- [x] UI screenshots of the live neighbour table against the real endpoint (en + fa/RTL), flush confirm, flush done
      (through the UI, audited), RA/proxy group in the interface drawer (en + fa)
- [x] `tools/ci.sh --base main` — see *CI* (the D-127 guard flake, Q8)

## Out of scope (not built)
Proxy ND by default (V12: opt-in only); DAD auto-remove (plugin not loaded); DHCPv6/PD; static routes / FIB / ping; ARP
termination in bridge domains; linux-cp neighbour sync; ND-based uRPF; a `vrx show ip neighbors` CLI command (apps/cli is
not this task's; the operations are in the generated table). Live events in the product agent need TD-8 (Q3).

## Cleanup
Every process stopped by PID by the tests (agent, API, vite preview); lab lock held only during runs; `vrx_w9` dropped by
the harness; no `loop9xx` / w9 neighbour, RA or proxy object left (cleanup lines above); no VPP-wide setting was changed
(slot agent, D-071); `apps/agent/bin`, every `dist/` and `/run/vrx-test/w9/nra` removed after the CI run (below, final check:
`vppctl show interface | grep -c loop9xx` = 0, `show ip neighbors` = 0, `show arp proxy` empty, no watcher on loop9xx,
no listener on 3900/5900/9191, `/run/vrx-test/w9/pg.env` gone).
