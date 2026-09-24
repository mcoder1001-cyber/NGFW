# Task: DF-7 — Descriptors for VPP plugins: policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for the services and control-protocol object types VPP
implements natively — policers and QoS marking (WBS D7.8), the load-balancer plugin (D7.9), SPAN mirroring (D1.10), LLDP (D1.7), BFD (D3.8),
VRRP (D9.1), IGMP (D2.9) and core MPLS (D2.8) — against the scheduler interface published by P05a and the generated bindings in
`apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that the QoS / HA / multicast / MPLS F-* tasks wire up later.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/policer/`, `binapi/policer_types/`, `binapi/qos/`, `binapi/lb/`, `binapi/lb_types/`, `binapi/span/`, `binapi/lldp/`, `binapi/bfd/`, `binapi/vrrp/`,
  `binapi/igmp/`, `binapi/mpls/`, `binapi/fib_types/` — **the only source of message names and fields**; verify every name below, never guess
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (policer, qos, lb, span, lldp, bfd, vrrp, igmp, mpls)
- `docs/lab/host-vrx-a.md` — all plugins above are loaded; **no workers** on the host → `policer_bind` to a worker index gets an integration test
  marked `skip: no workers on host`
- `docs/lab/shared-host-rules.md` — prefix `w<N>`, tables/labels/VR ids from your slot range, addresses `10.<N>.0.0/16`; BFD/VRRP/IGMP only on prefixed loopbacks

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-7.md` first, then build (estimate: 30 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **policer** (`binapi/policer`): `policer` (policer_add / policer_update / policer_del — or the older policer_add_del; name `w<N>-*`; cir/eir/cb/eb, rate
  type kbps/pps, round type, type 1r2c/1r3c/2r3c-2698/2r3c-4115/2r3c-mef5cf1, color-aware, conform/exceed/violate actions with dscp), `policer-interface`
  (policer_input_v2 / policer_output_v2: attach by index in/out), `policer-bind` (policer_bind_v2 to worker — skip on host), `policer-classify`
  (policer_classify_set_interface: ip4/ip6/l2 table indices → DF-2's `classify-table` key). Retrieve: policer_dump_v2 (or policer_dump), policer_classify_dump.
  policer_reset = action helper.
- **qos** (`binapi/qos`): `qos-record` (qos_record_enable_disable: interface, source ext/vlan/mpls/ip), `qos-store` (qos_store_enable_disable: interface, source,
  value), `qos-egress-map` (qos_egress_map_update: map id, 4×256 rows; qos_egress_map_delete), `qos-mark` (qos_mark_enable_disable: interface, output source,
  map id). Retrieve: qos_record_dump, qos_store_dump, qos_egress_map_dump, qos_mark_dump.
- **lb** (`binapi/lb`, T3 — minimal): `lb-conf` (lb_conf: ip4/ip6 src addresses, sticky buckets, flow timeout — global singleton, read/restore in tests), `lb-vip`
  (lb_add_del_vip_v2 or newest: prefix, protocol, port, encap gre4/gre6/l3dsr/nat4/nat6, dscp, target port, new flows table), `lb-as` (lb_add_del_as: vip, as
  address, flush), `lb-intf-nat` (lb_add_del_intf_nat4 / _nat6). Retrieve: lb_vip_dump (or v2), lb_as_dump. lb_flush_vip = action helper.
- **span** (`binapi/span`): `span-mirror` (sw_interface_span_enable_disable: src, dst, state disabled/rx/tx/both, is_l2; dst may be a GRE ERSPAN tunnel from
  DF-6 — depend on the interface key only). Retrieve: sw_interface_span_dump (l2 and l3 variants).
- **lldp** (`binapi/lldp`): `lldp-global` (lldp_config: system name `w<N>-…`, tx hold, tx interval — global singleton; read-only against the shared VPP unless
  unset), `lldp-interface` (sw_interface_set_lldp: port description, mgmt ip4/ip6/oid, enable). Retrieve: if binapi has no lldp dump, mark `partial`,
  name it in `DF-7-questions.md`; never fake Retrieve from desired state.
- **bfd** (`binapi/bfd`): `bfd-auth-key` (bfd_auth_set_key: conf key id, auth type, key = secret → `VRX_TEST_PSK_<id>`-style fixtures, never logged; bfd_auth_del_key),
  `bfd-udp-session` (bfd_udp_add / bfd_udp_mod / bfd_udp_del: interface, local/peer addr, desired min tx/rx, detect mult, auth key + bfd key id; flags via
  bfd_udp_session_set_flags admin up/down; bfd_udp_auth_activate/deactivate), `bfd-echo-source` (bfd_udp_set_echo_source / bfd_udp_del_echo_source; Retrieve
  bfd_udp_get_echo_source). Retrieve: bfd_udp_session_dump (or v2 with state), bfd_auth_keys_dump. Events: want_bfd_events → session state → `StreamEvents`.
- **vrrp** (`binapi/vrrp`): `vrrp-vr` (vrrp_vr_add_del — or vrrp_vr_update where generated: interface, vr id from slot range, priority, interval, flags preempt/
  accept/unicast/ipv6, addresses in `10.<N>…`), `vrrp-vr-peers` (vrrp_vr_set_peers: unicast peers), `vrrp-vr-track-interface` (vrrp_vr_track_if_add_del: tracked
  interface + priority decrement), `vrrp-vr-state` (vrrp_vr_start_stop as a desired bool `running`). Retrieve: vrrp_vr_dump, vrrp_vr_peer_dump,
  vrrp_vr_track_if_dump. Events: want_vrrp_vr_events → master/backup transitions → `StreamEvents`.
- **igmp** (`binapi/igmp`): `igmp-interface` (igmp_enable_disable: interface, mode host/router), `igmp-listen` (igmp_listen: static join, group + sources on
  interface, filter include/exclude), `igmp-group-prefix` (igmp_group_prefix_set: SSM/ASM range), `igmp-proxy-device` (igmp_proxy_device_add_del: vrf,
  upstream interface), `igmp-proxy-downstream` (igmp_proxy_device_add_del_interface). Retrieve: igmp_dump, igmp_group_prefix_dump. Events: want_igmp_events.
  igmp_clear_interface = action helper.
- **mpls** (`binapi/mpls`): `mpls-table` (mpls_table_add_del: table id from slot range, name `w<N>-*`), `mpls-interface` (sw_interface_set_mpls_enable), `mpls-route`
  (mpls_route_add_del: table, local label, eos, is_multicast, paths with label stacks), `mpls-ip-bind` (mpls_ip_bind_unbind: label ↔ ip prefix in `vrf/<id>`),
  `mpls-tunnel` (mpls_tunnel_add_del: paths + out labels, l2-only, multicast). Retrieve: mpls_table_dump, mpls_interface_dump, mpls_route_dump, mpls_tunnel_dump.
  Publish `mpls-table/<id>` and `mpls-interface/<name>` keys in `docs/agent/descriptors/mpls.md` — DF-6 SR-MPLS depends on them.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
policer-interface / policer-bind → policer (+ interface) · policer-classify → interface + `classify-table/…` (DF-2 key; fixture until merged) · qos-record /
qos-store → interface · qos-mark → interface + qos-egress-map · lb-vip → lb-conf (Optional) · lb-as → lb-vip · lb-intf-nat → interface · span-mirror → src + dst
interfaces · lldp-interface → lldp-global + interface · bfd-udp-session → interface + `interface-ip` of the local address (Optional) + bfd-auth-key (when set) ·
bfd-echo-source → interface · vrrp-vr → interface (+ `interface-ip`, Optional) · vrrp-vr-peers / -track-interface / -state → vrrp-vr (+ tracked interface) ·
igmp-listen → igmp-interface · igmp-proxy-downstream → igmp-proxy-device + igmp-interface · igmp-proxy-device → `vrf/<id>` + interface · mpls-interface →
interface · mpls-route → mpls-table (+ next-hop interfaces, Optional) · mpls-ip-bind → mpls-table + `vrf/<id>` · mpls-tunnel → next-hop interfaces.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / policer index / vr id).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding; event decoding
   for bfd/vrrp/igmp).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions filter
   by your prefix (other workers' objects exist on the same VPP). Use prefixed loopbacks/tables; never touch `local0` or anything unprefixed; clean up in
   `t.Cleanup`. BFD sessions stay `down` (no peer) and VRRP VRs may be started on a loopback (adverts never leave the host) — assert configuration and
   state shape, not protocol behaviour. Globals (lb-conf, lldp-global) are read, restored if touched.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations (+ event shapes for bfd/vrrp/igmp).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing, write
  `docs/status/tasks/DF-7-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (policer rates/actions, egress map rows, VR flags/addresses, MPLS paths and label stacks); a descriptor
  without Retrieve is not done — `partial` only where VPP has no dump, documented.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012). No packet tests.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{policer,qos,lb,span,lldp,bfd,vrrp,igmp,mpls}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed for every plugin; `policer-bind` test shows the `skip: no workers` reason; `mpls-table` key contract documented
- [ ] `vppctl show policer` / `show qos egress map` / `show lb vips` / `show span` / `show bfd sessions` / `show vrrp vr` / `show igmp config` / `show mpls fib table <id>`
      pasted for your prefixed objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-* feature wiring, performance, startup.conf changes. Not yours: classify tables (DF-2 — you consume the key),
ACL (DF-4), GRE/ERSPAN tunnels (DF-6 — SPAN only references the interface), SR-MPLS policies (DF-6), keepalived path of VRRP (RF-4), FRR BFD/PIM
integration (P12/F-*), multicast FIB routes `ip_mroute_add_del` and BIER (multicast F-*), hierarchical/HQoS shaping and per-interface queues (no
26.06 mainline API), LLDP neighbour table UI. No binapi regeneration, no VPP restart, no `local0`.
