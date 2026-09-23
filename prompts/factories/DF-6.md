# Task: DF-6 — Descriptors for VPP plugins: gre, ipip, vxlan, vxlan_gpe, gtpu, l2tp, pppoe, sr (srv6 + mpls), lisp   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for VPP's tunnel and overlay object types (WBS D6.6), Segment
Routing SRv6 + SR-MPLS (D6.7, D2.8 SR part) and LISP (D6.8), against the scheduler interface published by P05a and the generated bindings in
`apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that P11 (ipip + tunnel protect), SPAN (ERSPAN dst) and the tunnel F-*
tasks wire up later. Priority order = WBS value: gre, ipip, vxlan first; vxlan_gpe, gtpu, l2tp, pppoe next; sr then lisp last (T3 — leave
questions rather than polish).

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/gre/`, `binapi/ipip/`, `binapi/vxlan/`, `binapi/vxlan_gpe/`, `binapi/gtpu/`, `binapi/l2tp/`, `binapi/pppoe/`, `binapi/sr/`, `binapi/sr_types/`,
  `binapi/sr_mpls/`, `binapi/sr_pt/`, `binapi/srv6_*` (ad/am/as/mobile proxies, if generated), `binapi/lisp/`, `binapi/lisp_gpe/`, `binapi/lisp_types/`,
  `binapi/one/` (LISP "one" API, if generated), `binapi/tunnel_types/`, `binapi/mpls/` (fixture only) — **the only source of message names and
  fields**; verify every name below in the package, never guess
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (gre, ipip, vxlan, vxlan-gpe, gtpu, l2tp, pppoe, srv6, sr-mpls, lisp)
- `docs/lab/host-vrx-a.md` — every plugin above is loaded (all tunnel objects are created over prefixed loopbacks; no traffic leaves the host)
- `docs/lab/shared-host-rules.md` — prefix `w<N>`, tunnel endpoints in `10.<N>.0.0/16`, VNIs/TEIDs/session ids/BSIDs/labels from your slot range
  (document the numbering scheme in your status file), tables `<N>000–<N>999`

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-6.md` first, then build (estimate: 28 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **gre** (`binapi/gre`): `gre-tunnel` (gre_tunnel_add_del: type L3/TEB/ERSPAN, src/dst, outer table id, instance, session id for ERSPAN, mode
  p2p/p2mp). Retrieve: gre_tunnel_dump. Update = ErrRecreate. TEB tunnels are bridged by DF-1's `bridge-domain-member`, not here.
- **ipip** (`binapi/ipip`): `ipip-tunnel` (ipip_add_tunnel: src/dst, table id, instance, flags/dscp/mode; ipip_del_tunnel), `ipip-6rd` (ipip_6rd_add_tunnel
  / ipip_6rd_del_tunnel: ip6 prefix, ip4 prefix + src, security check, table ids). Retrieve: ipip_tunnel_dump. P11/DF-5 protect this interface.
- **vxlan** (`binapi/vxlan`): `vxlan-tunnel` (vxlan_add_del_tunnel_v3 or newest: src/dst or mcast + mcast sw_if_index, vni, encap vrf, decap next,
  instance, src port/dst port, is_l3), `vxlan-bypass` (sw_interface_set_vxlan_bypass per interface, ip4/ip6). Retrieve: vxlan_tunnel_v2_dump or newest.
  vxlan_offload_rx = hardware, skip.
- **vxlan_gpe** (`binapi/vxlan_gpe`): `vxlan-gpe-tunnel` (vxlan_gpe_add_del_tunnel_v2: local/remote, mcast sw_if_index, vni, protocol, encap/decap vrf),
  `vxlan-gpe-bypass` (sw_interface_set_vxlan_gpe_bypass). Retrieve: vxlan_gpe_tunnel_v2_dump. iOAM-over-GPE messages = out of scope.
- **gtpu** (`binapi/gtpu`): `gtpu-tunnel` (gtpu_add_del_tunnel_v2 or newest: src/dst, mcast, encap vrf, decap next, teid/tteid, pdu extension/qfi),
  `gtpu-tteid` (gtpu_tunnel_update_tteid), `gtpu-forward` (gtpu_add_del_forward), `gtpu-bypass` (sw_interface_set_gtpu_bypass). Retrieve:
  gtpu_tunnel_v2_dump. gtpu_offload_rx = hardware, skip.
- **l2tp** (`binapi/l2tp`): `l2tpv3-tunnel` (l2tpv3_create_tunnel: client/our address, local/remote session id + cookies, l2 sublayer, encap vrf),
  `l2tpv3-cookies` (l2tpv3_set_tunnel_cookies), `l2tpv3-enable` (l2tpv3_interface_enable_disable), `l2tpv3-lookup-key` (l2tpv3_set_lookup_key —
  global singleton, read-only in tests). Retrieve: sw_if_l2tpv3_tunnel_dump. If binapi has **no delete** message for the tunnel, Delete returns a
  documented error, the doc table records the limitation and `DF-6-questions.md` names it.
- **pppoe** (`binapi/pppoe`): `pppoe-session` (pppoe_add_del_session: session id, client ip, client mac, decap vrf), `pppoe-cp` (pppoe_add_del_cp:
  control-plane punt sw_if_index). Retrieve: pppoe_session_dump.
- **sr / SRv6** (`binapi/sr`): `sr-localsid` (sr_localsid_add_del: localsid, behavior end/end.x/end.t/end.dx2/dx4/dx6/dt4/dt6 + plugin behaviours if
  `binapi/srv6_ad|am|as` exist, fib table, nh, sw_if_index, end_psp), `sr-policy` (sr_policy_add_v2 / sr_policy_mod_v2 / sr_policy_del: bsid, sid lists +
  weights, type default/spray, fib table, encap/insert, encap source), `sr-steering` (sr_steering_add_del or v2: bsid or policy index, prefix + table
  or l2 sw_if_index), `sr-encap-source` / `sr-encap-hop-limit` (sr_set_encap_source / sr_set_encap_hop_limit — global singletons, read-only in tests).
  Retrieve: sr_localsids_dump, sr_policies_v2_dump (or newest), sr_steering_pol_dump; sr_localsids_with_packet_stats_dump = stats. `srv6-mobile`
  (D6.7 GTP functions): only if `binapi/srv6_mobile` is generated — sr_mobile_localsid_add_del / sr_mobile_policy_add; otherwise questions file.
- **sr_mpls** (`binapi/sr_mpls`): `sr-mpls-policy` (sr_mpls_policy_add / _mod / _del: bsid label, segments, weight, type), `sr-mpls-steering`
  (sr_mpls_steering_add_del: prefix + table, bsid, color/co-bits, vpn label), `sr-mpls-endpoint-color` (sr_mpls_policy_assign_endpoint_color). Retrieve:
  if binapi has **no** sr_mpls dump, derive from mpls_route_dump on the SR-MPLS label table, mark the descriptor `partial` in the doc table and
  name it in the questions file — never fake Retrieve from cached desired state. Needs an MPLS table + enabled interface (DF-7) as test fixture.
- **lisp** (`binapi/lisp`, `binapi/lisp_gpe`; minimal set, D6.8 is T3): `lisp-enable` (lisp_enable_disable — global; read/restore in tests), `lisp-locator-set`
  (lisp_add_del_locator_set), `lisp-locator` (lisp_add_del_locator: sw_if_index, priority, weight), `lisp-local-eid` (lisp_add_del_local_eid: eid, locator set,
  vni, key), `lisp-map-resolver` / `lisp-map-server` (lisp_add_del_map_resolver / _map_server), `lisp-remote-mapping` (lisp_add_del_remote_mapping:
  eid, rlocs), `lisp-adjacency` (lisp_add_del_adjacency), `lisp-eid-table-map` (lisp_eid_table_add_del_map: vni ↔ vrf/bd), `lisp-pitr` (lisp_pitr_set_locator_set),
  `lisp-gpe-enable` / `lisp-gpe-fwd-entry` (gpe_enable_disable, gpe_add_del_fwd_entry). Retrieve: lisp_locator_set_dump, lisp_locator_dump,
  lisp_eid_table_dump, lisp_map_resolver_dump, lisp_map_server_dump, lisp_adjacencies_get, lisp_eid_table_map_dump, gpe_fwd_entries_get.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
every tunnel → `vrf/<id>` of its encap/outer table (Optional when 0) · vxlan/vxlan-gpe/gtpu with mcast → the mcast interface · *-bypass → interface ·
gtpu-tteid / gtpu-forward → gtpu-tunnel · l2tpv3-cookies / l2tpv3-enable → l2tpv3-tunnel · pppoe-cp → interface · sr-localsid → `vrf/<id>` (+ interface
for dx behaviours) · sr-policy → none (encap source Optional) · sr-steering → sr-policy + `vrf/<id>` or l2 interface · sr-mpls-policy → `mpls-table`
(DF-7 key; fixture until merged) · sr-mpls-steering → sr-mpls-policy + `vrf/<id>` · lisp-locator → lisp-locator-set + interface · lisp-local-eid →
lisp-locator-set + lisp-enable · lisp-remote-mapping → lisp-enable · lisp-adjacency → lisp-remote-mapping + lisp-local-eid · lisp-eid-table-map →
`vrf/<id>` or `bridge-domain/<id>` (DF-1 key) · lisp-gpe-fwd-entry → lisp-gpe-enable.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / policy index).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your prefix / address range (other workers' tunnels exist on the same VPP). Endpoints on prefixed loopbacks in `10.<N>…`; never touch
   `local0` or anything unprefixed; clean up in `t.Cleanup`. Globals (encap source, lisp enable, l2tp lookup key) are read, not changed.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations (no-delete, no-dump cases explicit).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-6-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (endpoints, vni/teid/session ids, tables, sid lists and weights); a descriptor without Retrieve
  is not done — `partial` is allowed only where VPP offers no dump, and only when documented.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012). No packet tests.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,sr,sr_mpls,lisp}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{gre,ipip,vxlan,vxlan_gpe,gtpu,l2tp,pppoe,sr,sr_mpls,lisp}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed for every plugin; every `partial`/no-delete limitation listed in `DF-6-questions.md`
- [ ] `vppctl show gre tunnel` / `show ipip tunnel` / `show vxlan tunnel` / `show sr localsids` / `show sr policies` / `show lisp eid-table` pasted for your
      prefixed objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-*/P11 feature wiring, performance, startup.conf changes. Not yours: IPsec SA/SPD/tunnel-protect
(DF-5), bridging of TEB/VXLAN interfaces (DF-1), MPLS tables/routes/interface enable (DF-7 — fixture use only), SPAN/ERSPAN mirroring session
(DF-7), iOAM, hardware offload (vxlan/gtpu `offload_rx`), SR path tracing (`sr_pt`) unless trivially generated, LISP control-plane semantics
beyond the messages above, PPPoE client/server daemons. No binapi regeneration, no VPP restart, no `local0`.
