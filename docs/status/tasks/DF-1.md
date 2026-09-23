# DF-1 — Descriptors: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode

Branch `task/DF-1` (worktree `/root/ngfw-wt/DF-1`, slot 2 / prefix `w2`), base main@030f404, main merged
in at bf4c137 (40 commits: P09 ci.sh, P03 proto). Continued on 2026-09-24 after the first worker stalled
following 51073b1; that continuation found and fixed four Retrieve drift bugs with a new restart
simulation (below).

## Object types (20 descriptors, 7 packages)

| Package (`internal/descriptors/…`) | Descriptors | Doc |
|---|---|---|
| `interface` (Go `iface`) | `interface.subinterface`, `interface.admin-state`, `interface.mtu`, `interface.mac-address`, `interface.promisc`, `interface.rx-mode`, `interface.rx-placement` + shared interface-reference layer (`dump.go`: tag-based resolution, `KeyFor`, `RegisterKind`) | docs/agent/descriptors/interface.md |
| `bond` | `bond.bond`, `bond.member` | docs/agent/descriptors/bond.md |
| `l2` | `l2.bridge-domain`, `l2.bridge-domain-member`, `l2.xconnect`, `l2.fib-entry`, `l2.flags`, `l2.vlan-tag-rewrite` | docs/agent/descriptors/l2.md |
| `l3xc` | `l3xc.l3xc` | docs/agent/descriptors/l3xc.md |
| `memif` | `memif.socket`, `memif.memif` | docs/agent/descriptors/memif.md |
| `tapv2` | `tapv2.tap` | docs/agent/descriptors/tapv2.md |
| `af_packet` (Go `afpacket`) | `af-packet.host-interface` | docs/agent/descriptors/af_packet.md |

Each has KeyOf / Dependencies / Create / Update (in place or `ErrRecreate`) / Delete / Retrieve (full
dump, owner-filtered, decoded into the same proto type, Meta filled as Create does), a `Register(r, c,
owner)` per package, table-driven unit tests on `internal/vpp/fake` (via the stateful
`interface/ifacetest` fake) and a host integration test under the shared lab lock. `TestRegisterAllDF1`
registers all seven packages into one `scheduler.MapRegistry` (no duplicate/invalid names). There is no
agent-wide registry list on main yet (P05 wires `Register` calls); nothing outside my packages was edited.

## Verification (real output)

### CI gate — `tools/ci.sh --base main`
```
== apps/agent: make lint test build ==
ok  	ngfw/agent/internal/agent	1.146s; ok  	ngfw/agent/internal/contracttest	1.407s; ok  	ngfw/agent/internal/descriptors/af_packet	1.091s; ok  	ngfw/agent/internal/descriptors/bond	1.102s; ok  	ngfw/agent/internal/descriptors/interface	1.073s; ok  	ngfw/agent/internal/descriptors/l2	1.102s; ok  	ngfw/agent/internal/descriptors/l3xc	1.107s; ok  	ngfw/agent/internal/descriptors/memif	1.073s; ok  	ngfw/agent/internal/descriptors/tapv2	1.110s; ok  	ngfw/agent/internal/renderers	1.415s; ok  	ngfw/agent/internal/scheduler	1.090s; ok  	ngfw/agent/internal/vpp	1.054s;

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   0m15s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m13s
  apps/agent: make lint test build                   0m18s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 0m55s · logs /root/ngfw-wt/logs/ci/DF-1-20260924-003318-746437

CI GATE PASSED
```
(golangci-lint: `0 issues.`; this run fixed 152 findings the new P09 gate reports — method docs, gosec
nolints with reasons on the fixed-argv rig helper, De Morgan, `clear` shadowing.)

### Unit tests (fake VPP) — `go test -count=1 ./internal/descriptors/...`
```
ok  	ngfw/agent/internal/descriptors/af_packet	0.020s
ok  	ngfw/agent/internal/descriptors/bond	0.019s
ok  	ngfw/agent/internal/descriptors/interface	0.019s
?   	ngfw/agent/internal/descriptors/interface/ifacetest	[no test files]
ok  	ngfw/agent/internal/descriptors/l2	0.024s
ok  	ngfw/agent/internal/descriptors/l3xc	0.019s
ok  	ngfw/agent/internal/descriptors/memif	0.019s
ok  	ngfw/agent/internal/descriptors/tapv2	0.019s
```

### Integration on the host VPP — `VRX_INTEGRATION=1 VRX_TEST_PREFIX=w2 VRX_SLOT=2 VRX_VPP_TABLE_BASE=2000 go test -count=1 -v ./internal/descriptors/...` (excerpt)
```
af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50 name:"w2-af50"  host_if_name:"w2-af50"
af-packet.host-interface: Retrieve == desired: af-packet.host-interface/w2-af50p name:"w2-af50p"  host_if_name:"w2-af50p"  mode:MODE_IP
--- PASS: TestHostInterfaceOnHost (0.99s)
bond.bond: Retrieve == desired: bond.bond/w2-bond20
bond.bond: Retrieve == desired: bond.bond/w2-bond21
bond.member: Retrieve == desired: bond.member/w2-bond20/w2-tap20
bond.member: Retrieve == desired: bond.member/w2-bond20/w2-tap21
--- PASS: TestBondOnHost (1.08s)
interface.admin-state: Retrieve == desired: interface.admin-state/loop201 interface:"interface.loopback/loop201"
interface.mtu: Retrieve == desired: interface.mtu/loop201 interface:"interface.loopback/loop201" mtu:1500 ip4:1400
interface.mac-address: Retrieve == desired: interface.mac-address/loop201 interface:"interface.loopback/loop201" mac:"02:02:00:00:c9:01"
interface.promisc: Retrieve == desired: interface.promisc/w2-tap1 interface:"tapv2.tap/w2-tap1"
interface.rx-mode: Retrieve == desired: interface.rx-mode/w2-tap1 interface:"tapv2.tap/w2-tap1" mode:RX_MODE_KIND_INTERRUPT
integration_test.go:113: skip: no workers on host (startup.conf cpu { } runs the main core only; sw_interface_set_rx_placement needs a worker)
interface.subinterface: Retrieve == desired: interface.subinterface/loop201.100 parent:"interface.loopback/loop201" sub_id:100 outer_vlan:100 exact_match:true
--- SKIP: TestAttributesOnHost/rx-placement (0.00s)
l2.bridge-domain: Retrieve == desired: l2.bridge-domain/2010 id:2010  flood:true  uu_flood:true  forward:true  learn:true  mac_age:3
l2.bridge-domain-member: Retrieve == desired: l2.bridge-domain-member/2010/w2-tap10 bridge_domain:2010  interface:"tapv2.tap/w2-tap10"  shg:1
l2.bridge-domain-member: Retrieve == desired: l2.bridge-domain-member/2010/loop210 bridge_domain:2010  interface:"interface.loopback/loop210"  port_type:PORT_TYPE_BVI
l2.fib-entry: Retrieve == desired: l2.fib-entry/2010/02:02:00:00:0a:01 bridge_domain:2010  mac:"02:02:00:00:0a:01"  interface:"tapv2.tap/w2-tap10"  static:true
l2.fib-entry: Retrieve == desired: l2.fib-entry/2010/02:02:00:00:0a:02 bridge_domain:2010  mac:"02:02:00:00:0a:02"  filter:true
l2.flags: Retrieve == desired: l2.flags/w2-tap10 interface:"tapv2.tap/w2-tap10"  forward:true  flood:true  uu_flood:true  arp_term:true
l2.xconnect: Retrieve == desired: l2.xconnect/w2-tap12 rx:"tapv2.tap/w2-tap12"  tx:"tapv2.tap/w2-tap13"
l2.xconnect: Retrieve == desired: l2.xconnect/w2-tap13 rx:"tapv2.tap/w2-tap13"  tx:"tapv2.tap/w2-tap12"
interface.subinterface: Retrieve == desired: interface.subinterface/w2-tap11.100 parent:"tapv2.tap/w2-tap11"  sub_id:100  outer_vlan:100  inner_vlan:200  dot1ad:true  exact_match:true
l2.vlan-tag-rewrite: Retrieve == desired: l2.vlan-tag-rewrite/w2-tap11.100 interface:"interface.subinterface/w2-tap11.100"  op:VTR_OP_POP_2
--- PASS: TestL2OnHost (1.75s)
l3xc.l3xc: Retrieve == desired: l3xc.l3xc/w2-tap30/ip4 interface:"tapv2.tap/w2-tap30"  paths:{next_hop:"10.2.30.253"  interface:"interface.loopback/loop230"  weight:1}  paths:{next_hop:"10.2.30.254"  interface:"interface.loopback/loop230"  weight:1}
--- PASS: TestL3xcOnHost (0.84s)
memif.socket: Retrieve == desired: memif.socket/2040 id:2040  filename:"/run/vrx-test/w2/memif/w2-memif40.sock"
memif.memif: Retrieve == desired: memif.memif/w2-memif40 name:"w2-memif40"  id:40  socket:2040
--- PASS: TestMemifOnHost (0.62s)
tapv2.tap: Retrieve == desired: tapv2.tap/w2-tap40 name:"w2-tap40"  id:240  host_if_name:"w2-tap40"  host_ip4_prefix:"10.2.40.1/24"  host_ip6_prefix:"fd00:2:40::1/64"  host_mtu:1400  rx_ring_size:512  tx_ring_size:256  gso:true (sw_if_index {SwIfIndex:4})
--- PASS: TestTapOnHost (0.44s)
ok  	ngfw/agent/internal/descriptors/af_packet	1.010s
ok  	ngfw/agent/internal/descriptors/bond	1.114s
ok  	ngfw/agent/internal/descriptors/interface	2.476s
ok  	ngfw/agent/internal/descriptors/l2	1.786s
ok  	ngfw/agent/internal/descriptors/l3xc	0.871s
ok  	ngfw/agent/internal/descriptors/memif	0.639s
ok  	ngfw/agent/internal/descriptors/tapv2	0.465s
```
Each test deletes in `t.Cleanup` and asserts Retrieve no longer shows the key.

### Idempotency + restart simulation — `TestRestartSimulationOnHost`
One desired state with every DF-1 type (28 objects, owner `w2r` so parallel packages of the same slot do
not show up in the owner-wide diff), applied in `Dependencies()` topological order, then: same agent
re-Retrieve → plan; new API connection + fresh descriptors (empty memory) → Retrieve, Meta compared with
what Create returned → plan; re-apply → plan; delete everything with the restarted agent → Retrieve.
```
restart_integration_test.go:285: same agent, same desired state: plan is empty (28 objects)
restart_integration_test.go:309: restarted agent: Retrieve rebuilt 26 objects with equal Meta; plan = ["create interface.mac-address/loop260" "create interface.promisc/w2-tap63"] (VPP cannot report promisc / a configured MAC: re-applied idempotently)
restart_integration_test.go:323: restarted agent, second apply: plan is empty
restart_integration_test.go:343: restarted agent deleted 28 objects; Retrieve for owner w2r is empty
--- PASS: TestRestartSimulationOnHost (1.40s)
```
Its first run failed and exposed four real drift bugs that per-descriptor round-trip tests could not
catch (each would have caused a Delete in every plan, and the first also broke forwarding):
```
second apply by the same agent is not empty: ["delete interface.mtu/w2-tap62.100" "delete interface.rx-mode/w2-af60" "delete l2.fib-entry/2060/02:02:00:00:3c:01" "delete l2.flags/loop260"]
```
plus, found by a probe on the host: `interface.mtu` reported every fresh interface (`{9000,0,0,0}`) and
its Delete wrote `{0,0,0,0}` to hardware interfaces. The fixes are in 85588b5 (see Decisions D2–D5).

### vppctl, prefixed objects during the restart simulation (`VRX_DF1_HOLD=20`, excerpt)
```
vpp# show interface
              Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
BondEthernet260                   11    down         9000/0/0/0
host-w2-af60                      10    down         9000/0/0/0
local0                            0     down          0/0/0/0
loop260                           3      up        1500/1400/0/0
memif2060/60                      12    down         9000/0/0/0
tap260                            4     down         9000/0/0/0
tap261                            2     down         9000/0/0/0
tap262                            7      up          9000/0/0/0     rx packets                     2
tap262.100                        13     up           0/0/0/0
tap263                            9     down         9000/0/0/0
tap264                            5     down         9000/0/0/0
vpp# show bridge-domain 2060 detail
  BD-ID   Index   BSN  Age(min)  Learning  U-Forwrd   UU-Flood   Flooding  ARP-Term  arp-ufwd Learn-co Learn-li   BVI-Intf
  2060      1      3      5         on        on       flood        on       off       off        0    16777216   loop260
           Interface           If-idx ISN  SHG  BVI  TxFlood        VLAN-Tag-Rewrite
            loop260              3     3    0    *      *                 none
          tap262.100             13     1    1    -      *                 pop-1
  BD-Tag: w2r:2060
vpp# show bond details
BondEthernet260
  mode: lacp
  load balance: l34
  number of active members: 0
  number of members: 2
    tap260
    tap261
  device instance: 0
  interface id: 260
  sw_if_index: 11
  hw_if_index: 11
vpp# show memif
sockets
  id  listener    filename
  0   no          /run/vpp/memif.sock
  206 yes (1)     /run/vrx-test/w2/memif-restart/w2-memif60.sock
interface memif2060/60
  socket-id 2060 id 60 mode ethernet
vpp# show l2fib bd_id 2060
    Mac-Address     BD-Idx If-Idx BSN-ISN Age(min) static filter bvi         Interface-Name
 02:02:00:00:3c:01    1      3      0/0      no      *      -     *              loop260
 02:02:00:00:3c:02    1      13     0/0      no      *      -     -            tap262.100
vpp# show interface rx-placement
Thread 0 (vpp_main):
 node tap-input:
    tap261 queue 0 (polling)
    tap260 queue 0 (polling)
    tap264 queue 0 (polling)
    tap262 queue 0 (polling)
    tap263 queue 0 (interrupt)
 node af-packet-input:
    host-w2-af60 queue 0 (interrupt)
vpp# show l3xc
l3xc:[0]: tap263
      path:[27] pl-index:30 ip4 weight=1 pref=0 attached-nexthop:  oper-flags:resolved,
        10.2.60.254 loop260
vpp# show mode
l2 bridge loop260 bd_id 2060 bvi shg 0
l2 xconnect tap264 host-w2-af60
l2 xconnect host-w2-af60 tap264
l3 BondEthernet260
l3 memif2060/60
l2 bridge tap262.100 bd_id 2060 shg 1
```
(`show memif` truncates the socket id column: 2060 → `206`. `show mode` also listed other workers'
`gtpu_tunnel*`, omitted.) `show tap` showed w2-tap60…64 (`name "w2-tap6x"`, qsz 256).

### vppctl after the test (everything deleted)
```
vpp# show interface
              Name               Idx    State  MTU (L3/IP4/IP6/MPLS)     Counter          Count
local0                            0     down          0/0/0/0
vpp# show bridge-domain 2060 detail
show bridge-domain: No such bridge domain 2060
vpp# show bond
interface name   sw_if_index  mode          load balance  active members members
vpp# show memif
sockets
  id  listener    filename
  0   no          /run/vpp/memif.sock

vpp# show l3xc
$ ip -br link | grep -c '^w2-'
0
```

### Forbidden patterns — `grep -rn "vppctl\|exec.Command" internal/descriptors/{interface,bond,l2,l3xc,memif,tapv2,af_packet}`
Only the fixed-argv veth rig helper in two test files (`/usr/sbin/ip link add|set|del <w2-…>`, marked
`ALLOW:` + `//nolint:gosec // G204`) and three `t.Logf` hints naming a `vppctl show` command for the
operator; no `vppctl` call and no `exec.Command` in non-test code.
```
internal/descriptors/interface/restart_integration_test.go:166:	_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() //nolint:gosec // G204 ALLOW: leftover of an aborted run
internal/descriptors/interface/restart_integration_test.go:168:		if out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput(); err != nil { //nolint:gosec // G204 ALLOW: fixed argv rig helper
internal/descriptors/interface/restart_integration_test.go:172:	t.Cleanup(func() { _ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() }) //nolint:gosec // G204 ALLOW: cleanup
internal/descriptors/bond/integration_test.go:78:	t.Logf("bond %s (BondEthernet%d) with members %v configured; vppctl show bond details", lacp.Name, lacp.Id, members)
internal/descriptors/memif/integration_test.go:69:	t.Logf("memif socket %d %s + master memif configured; vppctl show memif", sock.Id, sock.Filename)
internal/descriptors/l2/integration_test.go:127:	t.Logf("bridge-domain %d with %s, %s, %s(BVI) and %s.100 configured; vppctl show bridge-domain %d detail", bdID, tap10, tap11, loopKey, tap11, bdID)
internal/descriptors/af_packet/integration_test.go:24:		out, err := exec.Command("/usr/sbin/ip", args...).CombinedOutput() //nolint:gosec // G204 ALLOW: fixed argv rig helper
internal/descriptors/af_packet/integration_test.go:29:	_ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() //nolint:gosec // G204 ALLOW: leftover from an aborted run
internal/descriptors/af_packet/integration_test.go:31:	t.Cleanup(func() { _ = exec.Command("/usr/sbin/ip", "link", "del", name).Run() }) //nolint:gosec // G204 ALLOW: cleanup
```

## Acceptance checklist
- [x] unit + integration green for all seven packages (above)
- [x] no `vppctl` / `exec.Command` except the fixed-argv test rig helper
- [x] same desired state applied twice → empty plan (restart simulation log)
- [x] object ↔ message table per plugin (docs/agent/descriptors/*.md); rx-placement skips with `skip: no workers on host`
- [x] `vppctl show interface` / `show bridge-domain 2060 detail` / `show bond details` with prefixed objects, then empty after delete

## Out of scope / left undone
- Wiring into the reconciler / agent registry (P05), API/UI, F-* features, P08 — not built.
- `interface.rx-placement` is unit-tested only; the host has no workers (skip with reason).
- Not modelled because VPP cannot report them back (they would never round-trip): tap host MAC, queue
  counts and host gateways; memif ring/buffer size, queue counts, secret; af_packet flags/frame sizes;
  bond enable_gso. Documented per plugin.
- Models are agent-internal `.proto` stand-ins (D-055); P03b adds the domain leaf messages, then a small
  follow-up switches the descriptors to them.
- Loopback is P05's; tests create prefixed loopbacks directly with the same tag scheme.

## Open questions — docs/status/tasks/DF-1-questions.md
- Q1 FIB-table dependency key for l3xc: `vrf/<id>` (prompt) vs P05's final name (one constant).
- Q2 **Interface key scheme**: DF-1 uses the creator's full key (`tapv2.tap/w2-tap10`,
  `interface.loopback/loop201`, …), as the P05a contract routes keys by descriptor name; DF-2…DF-6
  branches build `interface/<name>`, which no descriptor produces. Needs a manager decision before P08.
- Q3 promisc / configured MAC are not readable from VPP: one idempotent re-apply after an agent restart;
  should P05 persist an owner table for them instead?

## Decisions (for the LOG)
- D1 Interface references are full creator keys; resolution by owner tag (`"<owner>:<name>"`) in one
  `sw_interface_dump`, checked against the device class — not a second name→index map. Create cannot see
  dependency Meta under the P05a contract, so it resolves by tag; Update/Delete use Meta. (Options: full
  keys / generic `interface/<name>` alias / single `interface` descriptor — see Q2.)
- D2 `interface.mtu` = per-protocol sw MTU (`sw_interface_set_mtu`), not `hw_interface_set_mtu`. The
  object exists only while the MTUs differ from VPP's creation default (`{link_mtu,0,0,0}`, sub-interface
  `{0,0,0,0}`); Delete restores that default; L3 mtu 0 is rejected. (Options: report always → perpetual
  deletes; report non-zero → same; default-relative ✔.)
- D3 `interface.rx-mode` is relative to the device-class default (polling; interrupt for af-packet);
  desiring the default is an error; Delete restores it.
- D4 `interface.mac-address` / `interface.promisc` are reported only for what the running agent set
  (VPP has no configured-vs-default MAC and no promisc readback); one re-apply after restart (Q3).
- D5 `l2.flags` is the per-interface mask relative to VPP's join default (all on, learning off for the
  BVI); the static BVI fib entry VPP installs for the BVI's own MAC is part of the member object, not an
  `l2.fib-entry`.
- D6 `l2.xconnect` = one object per direction (rx → tx), keyed on rx.
- D7 Sub-interfaces via `create_subif` only (not `create_vlan_subif`), one decoding path.
- D8 memif socket ownership = file directly in the owner's socket dir (`/run/vrx/memif`, tests
  `/run/vrx-test/<owner>/memif`); bridge domains by `bd_tag`; l3xc by owned rx interface.
- D9 Merged main into the task branch (bf4c137) so the branch is gated by the current P09 `tools/ci.sh`.
