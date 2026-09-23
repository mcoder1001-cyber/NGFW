# Task: DF-3 — Descriptors for VPP plugins: nat44_ed (nat), nat44_ei, nat64, nat66, det44, map, cnat, pnat   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for every NAT family in VPP 26.06 (WBS D4.1–D4.6),
against the scheduler interface published by P05a and the generated bindings in `apps/agent/binapi/`. No API, no UI — pure
agent-side building blocks that `F-nat44-ed-sessions` and the other NAT F-* tasks will wire up later.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/nat44_ed/`, `binapi/nat44_ei/`, `binapi/nat64/`, `binapi/nat66/`, `binapi/npt66/`, `binapi/det44/`, `binapi/map/`, `binapi/dslite/`,
  `binapi/cnat/`, `binapi/pnat/`, `binapi/nat_types/` — **the only source of message names and fields**; verify every name below, never guess
- `prompts/features/F-nat44-ed-sessions.md` — the consumer; its descriptor names (`nat44-enable`, `nat44-interface-feature`, `nat44-address-pool`,
  `nat44-static-mapping`, `nat44-timeouts`) are the ones you deliver
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (nat44-ed, nat44-ei, nat64, nat66, npt66, det44, map, dslite, cnat, pnat)
- `docs/lab/host-vrx-a.md` — `npt66_plugin.so` is **not loaded** → its descriptor gets an integration test marked `skip-unless-plugin-loaded`
  (skip predicate: govpp `CheckCompatiblity` on the npt66 messages returns unknown-message). All other NAT plugins are loaded.
- `docs/lab/shared-host-rules.md` — NAT pools only in `10.<N>.0.0/16`, tables `<N>000–<N>999`, tags `w<N>-*`

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-3.md` first, then build (estimate: 34 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **nat44_ed** (`binapi/nat44_ed`): `nat44-enable` (nat44_ed_plugin_enable_disable: sessions, flags static-mapping-only/connection-tracking/out2in-dpo,
  inside/outside vrf), `nat44-interface-feature` (nat44_interface_add_del_feature in/out; nat44_interface_add_del_output_feature for output/
  hairpin), `nat44-address-pool` (nat44_add_del_address_range: first/last in `10.<N>…`, vrf, twice-nat flag), `nat44-interface-address`
  (nat44_add_del_interface_addr), `nat44-static-mapping` (nat44_add_del_static_mapping_v2: local/external ip+port or external sw_if_index,
  protocol, vrf, flags addr-only/twice-nat/self-twice-nat/out2in-only, pool ip, tag), `nat44-identity-mapping` (nat44_add_del_identity_mapping),
  `nat44-lb-static-mapping` (nat44_add_del_lb_static_mapping + nat44_lb_static_mapping_add_del_local), `nat44-timeouts` (nat_set_timeouts or
  nat44_ed_set_timeouts — verify which exists; Retrieve nat_get_timeouts), `nat44-forwarding` (nat44_forwarding_enable_disable), `nat44-vrf-table`
  / `nat44-vrf-route` (nat44_ed_add_del_vrf_table / _vrf_route if generated). Retrieve: nat44_interface_dump, nat44_interface_output_feature_dump,
  nat44_address_dump, nat44_interface_addr_dump, nat44_static_mapping_dump, nat44_identity_mapping_dump, nat44_lb_static_mapping_dump,
  nat44_show_running_config, nat44_ed_vrf_tables_v2_dump. `nat44-sessions` = Retrieve-only state (nat44_user_session_v3_dump or newest;
  page in the agent, never one giant message); `nat44_del_session` / `nat44_ed_del_session` = action helper, not a descriptor.
- **nat44_ei** (`binapi/nat44_ei`): `nat44ei-enable`, `nat44ei-interface-feature` (+ output feature), `nat44ei-address-pool`, `nat44ei-interface-address`,
  `nat44ei-static-mapping`, `nat44ei-identity-mapping`, `nat44ei-timeouts`, `nat44ei-forwarding`, `nat44ei-ipfix` (nat44_ei_ipfix_enable_disable);
  Retrieve via the matching `nat44_ei_*_dump` + nat44_ei_show_running_config; sessions via nat44_ei_user_session_dump (Retrieve-only, paged).
- **nat64** (`binapi/nat64`): `nat64-enable` (nat64_plugin_enable_disable), `nat64-prefix` (nat64_add_del_prefix, per vrf), `nat64-pool`
  (nat64_add_del_pool_addr_range), `nat64-interface` (nat64_add_del_interface inside/outside), `nat64-static-bib` (nat64_add_del_static_bib),
  `nat64-timeouts` (nat64_set_timeouts / nat64_get_timeouts). Retrieve: nat64_prefix_dump, nat64_pool_addr_dump, nat64_interface_dump,
  nat64_bib_dump; nat64_st_dump = session state (Retrieve-only).
- **nat66** (`binapi/nat66`): `nat66-enable` (nat66_plugin_enable_disable), `nat66-interface` (nat66_add_del_interface), `nat66-static-mapping`
  (nat66_add_del_static_mapping, vrf). Retrieve: nat66_interface_dump, nat66_static_mapping_dump.
- **npt66** (`binapi/npt66`, NPTv6, plugin not loaded): `npt66-binding` (npt66_binding_add_del: sw_if_index, internal/external prefix). Retrieve: check
  for a dump; if none, partial + questions file. Integration test: skip-unless-plugin-loaded.
- **det44** (`binapi/det44`): `det44-enable` (det44_plugin_enable_disable: inside/outside vrf), `det44-interface` (det44_interface_add_del_feature),
  `det44-map` (det44_add_del_map: in_addr/plen ↔ out_addr/plen), `det44-timeouts` (det44_set_timeouts / det44_get_timeouts). Retrieve:
  det44_interface_dump, det44_map_dump; det44_session_dump = state; det44_close_session_in/out = action helpers.
- **map** (`binapi/map`, D4.5 MAP-E/MAP-T/LW4o6): `map-domain` (map_add_domain_v? newest: ip4_prefix, ip6_prefix, ip6_src, ea/psid bits/offset,
  mtu, flags translation/rfc6052, tag; map_del_domain), `map-rule` (map_add_del_rule: psid → ip6_dst; LW4o6 = MAP-E with rules),
  `map-params` (singleton composite over map_param_set_fragmentation / _icmp / _icmp6 / _security_check / _traffic_class / _tcp; Retrieve
  map_param_get), `map-interface` (map_if_enable_disable, is_translation). Retrieve: map_domains_dump, map_rule_dump.
- **dslite** (`binapi/dslite`, D4.5 DS-Lite, if generated): `dslite-aftr-addr` (dslite_set_aftr_addr / dslite_get_aftr_addr), `dslite-b4-addr`
  (dslite_set_b4_addr / dslite_get_b4_addr), `dslite-pool` (dslite_add_del_pool_addr_range; dslite_address_dump).
- **cnat** (`binapi/cnat`, D4.6): `cnat-translation` (cnat_translation_update: vip endpoint, paths/backends with flags, ip proto, lb type default/
  maglev, policy; cnat_translation_del), `cnat-snat-addresses` (cnat_set_snat_addresses / cnat_get_snat_addresses — singleton), `cnat-snat-policy`
  (cnat_set_snat_policy: none/if-pfx/k8s), `cnat-snat-interface` (cnat_snat_policy_add_del_if: table include/exclude/k8s), `cnat-snat-exclude-prefix`
  (cnat_snat_policy_add_del_exclude_pfx). Retrieve: cnat_translation_dump; cnat_session_dump = state; cnat_session_purge = action helper.
- **pnat** (`binapi/pnat`): `pnat-binding` (pnat_binding_add_v2 or pnat_binding_add: match/rewrite tuples; pnat_binding_del), `pnat-attachment`
  (pnat_binding_attach / pnat_binding_detach: sw_if_index, attachment input/output). Retrieve: pnat_bindings_get, pnat_interfaces_get.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
Every `*-enable` is the root of its family; `*-interface-feature` → interface + enable; `*-address-pool` → enable (+ `vrf/<id>`, Optional) ·
static mapping → enable + pool when the external address is a pool address, or the external interface when `external_sw_if_index` is used;
twice-NAT mapping additionally → a twice-nat pool · lb mapping → pool · identity mapping → interface or enable · timeouts / forwarding → enable ·
nat64-static-bib → nat64-pool + nat64-prefix · nat64/nat66/det44/npt66 interface objects → interface + enable · det44-map → enable ·
map-rule → map-domain · map-interface → interface (+ domain, Optional) · dslite-pool → dslite-aftr-addr · cnat-translation → cnat-snat-addresses
(Optional) · cnat-snat-interface → interface · pnat-attachment → pnat-binding + interface.

### Shared-host rules specific to NAT (mandatory)
Plugin enable/disable, timeouts, forwarding, snat addresses and map-params are **global singletons** on the one VPP. Your integration test must:
read them first (`*_show_running_config` / `*_get_*`); treat "already enabled with a compatible mode" as converged; **never disable a plugin
you did not enable**; never switch nat44 ED↔EI (mutually exclusive) while foreign objects exist (any pool outside `10.<N>.0.0/16` or tag not
`w<N>-*` → `t.Skip` with the reason); restore every global you changed in `t.Cleanup`. Interface features only on your prefixed loopbacks.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / indices).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your prefix (other workers' objects exist on the same VPP). Use loopbacks/tables/dummy objects; never touch `local0` or anything
   unprefixed; clean up in `t.Cleanup`. Session dumps are asserted only for shape (they may be empty — no traffic here).
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations.

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-3-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (all mapping flags, vrf ids, protocol, tag); a descriptor without Retrieve is not done.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012). No packet tests (F-nat44-ed-sessions owns them).

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{nat44_ed,nat44_ei,nat64,nat66,npt66,det44,map,dslite,cnat,pnat}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{nat44_ed,nat44_ei,nat64,nat66,npt66,det44,map,dslite,cnat,pnat}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed for every family; `npt66` test output shows the `skip-unless-plugin-loaded` reason
- [ ] `vppctl show nat44 addresses` / `show nat44 static mappings` / `show nat64 bib all` / `show det44 mappings` / `show cnat translation` pasted with your
      prefixed objects, then empty after delete; globals you touched shown restored

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-* feature wiring (session browser, kill-session RPC, pool utilisation), performance, startup.conf
changes. Not yours: ACL (DF-4), interfaces/tables (DF-1/P05), IPFIX exporter object (DF-8 — you only flip `*_ipfix_enable_disable`), ALG/session
helpers beyond VPP defaults, HA session sync, 464XLAT CLAT side, hairpinning tuning. No binapi regeneration, no VPP restart, no `local0`.
