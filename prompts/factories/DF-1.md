# Task: DF-1 — Descriptors for VPP plugins: bond, l2 (bridge, xconnect), memif, tap, host-interface/af_packet, subinterface, admin-state, mtu, rx-mode   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for the interface-family and L2 object
types listed above (WBS D1.2, D1.4, D1.5, D1.6), against the scheduler interface published by P05a and the generated
bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that P08 and the F-* feature tasks
(F-vlan-qinq, bonding, bridging) will wire up later. P05 core owns `loopback`, `interface-ip`, `vrf`, `static-route`;
everything else interface-shaped is yours.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/` or P05's `core/loopback`) — reuse its
  interface key scheme (`interface/<name>` + `sw_if_index` in `Meta`); never invent a second name→index map
- `apps/agent/binapi/interface/`, `binapi/bond/`, `binapi/l2/`, `binapi/l3xc/`, `binapi/memif/`, `binapi/tapv2/`, `binapi/af_packet/`,
  `binapi/interface_types/` — **the only source of message names and fields**; verify every name below in the package, never guess
- VPP 26.06 docs for the plugins: https://s3-docs.fd.io/vpp/26.06/ (bond, l2, memif, tapv2, af_packet)
- `docs/lab/host-vrx-a.md` — all plugins here are loaded; but the host runs `cpu { }` (main core only): anything that needs a
  worker index (rx-placement to a worker) gets an integration test marked `skip: no workers on host`
- `docs/lab/shared-host-rules.md` — `VRX_TEST_PREFIX=w<N>`, loopbacks `loop<N>xx`, host-interfaces/veths/taps `w<N>-*`, table range

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-1.md` first, then build (estimate: 16 object types × Create/Update/Delete/Retrieve
+ table-driven unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **interface** (`binapi/interface`): `admin-state` (sw_interface_set_flags), `mtu` (sw_interface_set_mtu per-protocol array and/or
  hw_interface_set_mtu — decide, document), `mac-address` (sw_interface_set_mac_address), `promisc` (sw_interface_set_promisc),
  `rx-mode` (sw_interface_set_rx_mode: polling/interrupt/adaptive, per queue or all), `rx-placement` (sw_interface_set_rx_placement —
  skip-on-host, see above). Retrieve: sw_interface_dump (+ sw_interface_rx_placement_dump). These are *attribute* descriptors keyed on the
  parent interface key; Update in place, never recreate the interface.
- **subinterface** (`binapi/interface`, `binapi/l2`): `subinterface` (create_subif with dot1q / dot1ad / exact-match / default / untagged /
  outer+inner vlan; delete_subif; create_vlan_subif is the 802.1q shortcut — pick one path), `vlan-tag-rewrite` (l2_interface_vlan_tag_rewrite:
  push/pop/translate for QinQ stacking). Retrieve: sw_interface_dump decoded `sub_*` fields. Update = ErrRecreate.
- **bond** (`binapi/bond`): `bond` (bond_create2: mode lacp/xor/round-robin/active-backup/broadcast, load-balance l2/l23/l34, numa-only,
  enable-gso, id; bond_delete), `bond-member` (bond_add_member / bond_detach_member, is_passive, is_long_timeout). Retrieve:
  sw_bond_interface_dump, sw_member_interface_dump. Update of mode/lb = ErrRecreate.
- **l2** (`binapi/l2`): `bridge-domain` (bridge_domain_add_del_v2: flood, uu-flood, forward, learn, arp-term, arp-ufwd, mac-age; id from your
  table range), `bridge-domain-member` (sw_interface_set_l2_bridge: port type normal/bvi/uu-fwd, shg for split-horizon; unbind = set l3
  via sw_interface_set_l3? — verify the correct "remove from bridge" message), `l2-xconnect` (sw_interface_set_l2_xconnect, both directions
  as two objects or one composite — document), `l2-fib-entry` (l2fib_add_del static/filter/bvi), `l2-flags` (l2_flags learn/forward/flood/
  uu-flood/arp-term on an interface), `mac-age` if not part of the bridge-domain object (bridge_domain_set_mac_age). Retrieve:
  bridge_domain_dump, l2_xconnect_dump, l2_fib_table_dump.
- **l3xc** (`binapi/l3xc`, plugin loaded): `l3xc` (l3xc_update / l3xc_del with paths; l3xc_dump). Depends on interface + FIB table (P05 `vrf`).
- **memif** (`binapi/memif`): `memif-socket` (memif_socket_filename_add_del_v2; file under `/run/vrx-test/w<N>/`), `memif` (memif_create_v2:
  role master/slave, mode ethernet/ip/punt, ring-size, buffer-size, secret, hw-addr; memif_delete). Retrieve: memif_socket_filename_dump,
  memif_dump.
- **tap** (`binapi/tapv2`): `tap` (tap_create_v3: host_if_name `w<N>-tapX`, host namespace, host mac/ip4/ip6/bridge, mtu, rx/tx ring sizes,
  gso/csum flags; tap_delete_v2). Retrieve: sw_interface_tap_v2_dump. Update = ErrRecreate.
- **host-interface / af_packet** (`binapi/af_packet`): `host-interface` (af_packet_create_v3: host_if_name `w<N>-*`, mode ethernet/ip, flags qdisc-bypass/
  cksum-gso/version-2, rx/tx frame counts; af_packet_delete). Retrieve: af_packet_dump. The Linux veth must exist first — in tests create it with
  the P04 rig (`tools/lab rig`) or a fixed-argv `ip link add w<N>-… type veth peer …` helper; delete it in `t.Cleanup`.

### Dependencies (declare in `Dependencies()`, test the ordering with the fake)
admin-state / mtu / mac / promisc / rx-mode / rx-placement → the interface key they decorate (loopback from P05, tap, host-interface, bond,
memif, subinterface) · subinterface → parent interface · vlan-tag-rewrite → subinterface · bond-member → bond + member interface ·
bridge-domain-member → bridge-domain + interface · l2-xconnect → both interfaces · l2-fib-entry → bridge-domain (+ interface, optional) ·
l2-flags → interface · l3xc → interface + FIB table (`vrf/<id>` key from P05 core; Optional=false) · memif → memif-socket ·
host-interface → nothing in VPP (Linux veth is a precondition the test creates) · tap → nothing.
Publish the exact key strings in `docs/agent/descriptors/interface.md` — DF-2…DF-8 depend on `interface/<name>`.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your prefix (other workers' objects exist on the same VPP). Use prefixed loopbacks/taps/bridge ids from your table range; never
   touch `local0`, `ens192` or anything unprefixed; clean up in `t.Cleanup`. Tap and host-interface tests create Linux netdevs on the host —
   name them `w<N>-*`, never assign management addresses, delete them even when the test fails.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations.

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-1-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (sub-interface tags, bond mode/lb, bridge flags, memif role/socket); a descriptor without
  Retrieve is not done.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012).
- Interface keys are shared with P05 core: use the README convention, resolve `sw_if_index` from `Meta`, never by name lookup at apply time.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{interface,bond,l2,l3xc,memif,tapv2,af_packet}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{interface,bond,l2,l3xc,memif,tapv2,af_packet}` is empty (rig helper for veths excepted, fixed argv only)
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed for every plugin above; `rx-placement` test shows the `skip: no workers` reason
- [ ] `vppctl show interface` / `show bridge-domain <id> detail` / `show bond` pasted for your prefixed objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-* feature wiring (F-vlan-qinq, P08 vertical slice), performance/rx-queue tuning, worker
placement policy, startup.conf changes. Not yours: `loopback` / `interface-ip` / `vrf` / `static-route` (P05 core), neighbours/ND (DF-2),
tunnels incl. GRE TEB / VXLAN bridging (DF-6), SPAN / LLDP / QoS on interfaces (DF-7), linux-cp pairs (DF-8), DPDK/vmxnet3 interfaces
(no data NICs exist), "time-range MAC filter" (control-plane scheduling, F-*). No binapi regeneration, no VPP restart, no `local0`.
