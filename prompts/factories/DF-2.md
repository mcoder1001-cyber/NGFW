# Task: DF-2 — Descriptors for VPP plugins: ip_neighbor, ip6_nd (RA, DAD), urpf, abf, classify   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for the neighbour-database, IPv6 ND, uRPF/allow-deny,
ACL-based forwarding and classifier object types (WBS D2.3, D2.4, D2.7), against the scheduler interface published by P05a and the
generated bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that P08 and the F-* tasks wire up later.
IP tables, routes and interface addresses are **P05 core** — you consume their keys, you do not build them.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`) — key scheme `interface/<name>`, `vrf/<id>`
- `apps/agent/binapi/ip_neighbor/`, `binapi/arp/`, `binapi/ip6_nd/`, `binapi/urpf/`, `binapi/adl/`, `binapi/abf/`, `binapi/classify/`,
  `binapi/ip_session_redirect/`, `binapi/ip_types/`, `binapi/fib_types/` — **the only source of message names and fields**; verify every
  name below in the package, never guess
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/ (ip_neighbor, ip6-nd, urpf, adl, abf, classify, ip_session_redirect)
- `docs/lab/host-vrx-a.md` — `ip6_dad_autoremove` is **not loaded** → its descriptor gets an integration test marked `skip-unless-plugin-loaded`
  (skip predicate: govpp `CheckCompatiblity` on that plugin's messages returns unknown-message). abf/urpf/adl/classify are loaded.
- `docs/lab/shared-host-rules.md` — prefix `w<N>`, table range `<N>000–<N>999`, addresses in `10.<N>.0.0/16`, IPv6 test prefixes documented in your status file

## Scope — build exactly this
Enumerate the object types in `docs/status/tasks/DF-2.md` first, then build (estimate: 17 object types × Create/Update/Delete/Retrieve
+ unit test with the fake client + one integration check under the shared lock with your slot prefix):

### Object types per plugin (binapi package → messages to look for)
- **ip_neighbor** (`binapi/ip_neighbor`): `neighbor` (ip_neighbor_add_del: static / no-fib-entry / no-adj-fib flags, ip4+ip6, per interface),
  `neighbor-config` (ip_neighbor_config per AF: max_number, max_age, recycle — global per AF, singleton key). Retrieve: ip_neighbor_dump per
  interface + AF (filter to your prefixed interfaces), ip_neighbor_config_get. `ip_neighbor_flush` is an action helper, not a descriptor.
- **arp / proxy-ARP** (`binapi/arp`): `proxy-arp-range` (proxy_arp_add_del: table id + lo/hi in `10.<N>.…`), `proxy-arp-interface`
  (proxy_arp_intfc_enable_disable). Retrieve: proxy_arp_dump, proxy_arp_intfc_dump.
- **ip6_nd** (`binapi/ip6_nd`): `ra-config` (sw_interface_ip6nd_ra_config: suppress, managed, other, ll_option, send_unicast, cease, default_router,
  lifetime, initial/max/min intervals), `ra-prefix` (sw_interface_ip6nd_ra_prefix: prefix, valid/preferred lifetimes, no_advertise, off_link,
  no_autoconfig, no_onlink), `proxy-nd` (ip6nd_proxy_add_del + ip6nd_proxy_enable_disable), `dad` (26.06 DAD: look for `ip6_dad*` / `*dad*`
  messages in `binapi/ip6_nd` and in a dedicated package — if only the not-loaded `ip6_dad_autoremove` plugin exposes it, write it with the skip
  predicate; if no API exists, questions file). Retrieve: sw_interface_ip6nd_ra_dump, ip6nd_proxy_dump. `ip6nd_send_router_solicitation` = action, skip.
- **urpf** (`binapi/urpf`): `urpf` (urpf_update_v2: mode off/loose/strict, AF, direction rx/tx, sw_if_index, table id). Retrieve: urpf_interface_dump.
- **adl** (`binapi/adl`, D2.4 allow/deny lists): `adl-interface` (adl_interface_enable_disable), `adl-allowlist` (adl_allowlist_enable_disable: fib id,
  ip4/ip6, default-cop). Retrieve: check for a dump; if none exists, questions file + mark partial in the doc table.
- **abf** (`binapi/abf`): `abf-policy` (abf_policy_add_del: policy_id from your table range, acl_index resolved from the `acl/<name>` key
  metadata, fib paths), `abf-interface-attach` (abf_itf_attach_add_del: policy, sw_if_index, priority, is_ipv6). Retrieve: abf_policy_dump,
  abf_itf_attach_dump. abf_plugin_get_version = health check only.
- **classify** (`binapi/classify`): `classify-table` (classify_add_del_table: nbuckets, memory, skip/match vectors, mask bytes, next/miss table,
  current_data flag/offset; keep mask as opaque bytes in the proto), `classify-session` (classify_add_del_session: match bytes, hit_next_index,
  opaque, advance, action, metadata), `classify-interface-ip-table` (classify_set_interface_ip_table), `classify-interface-l2-tables`
  (classify_set_interface_l2_tables), `input-acl` / `output-acl` (input_acl_set_interface / output_acl_set_interface: ip4/ip6/l2 tables).
  Retrieve: classify_table_ids → classify_table_info, classify_session_dump, classify_table_by_interface. Tag tables via `classify-table` name in
  your prefix (store the mapping in Meta; the API has no tag field — document how you attribute tables to owners).
- **ip_session_redirect** (`binapi/ip_session_redirect`, D2.7): `session-redirect` (ip_session_redirect_add_v2 / ip_session_redirect_del: table
  index, match, opaque index, paths, is_punt, af). Retrieve: check for a dump; if none, questions file + partial.

### Dependencies (declare in `Dependencies()`, test ordering with the fake)
neighbor → interface (+ `vrf/<id>` optional) · neighbor-config → none · proxy-arp-range → `vrf/<id>` · proxy-arp-interface → interface ·
ra-config / ra-prefix → interface (ra-prefix additionally → the matching `interface-ip` key, Optional=true) · proxy-nd → interface ·
dad → interface · urpf → interface + `vrf/<id>` · adl-interface → interface · adl-allowlist → `vrf/<id>` · abf-policy → `acl/<name>`
(**DF-4's key** — agree the exact string via `docs/agent/descriptors/acl.md`; until DF-4 merges, tests create the ACL as a fixture directly
through `binapi/acl` with a prefixed tag) + next-hop interfaces (Optional) · abf-interface-attach → abf-policy + interface ·
classify-session → classify-table · classify-interface-* / input-acl / output-acl → classify-table(s) + interface · session-redirect → classify-table.

### Deliverables per object type
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`,
   `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index / table index).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`, `VRX_INTEGRATION=1`, `flock -s /run/lock/vrx-lab.lock`): create → Retrieve shows it →
   delete → Retrieve shows nothing. **Every object name/tag/table id carries your `VRX_TEST_PREFIX` / slot range**; Retrieve-based assertions
   filter by your prefix (other workers' objects exist on the same VPP). Use prefixed loopbacks (`loop<N>xx`, created via binapi as fixtures)
   and tables in your range; never touch `local0` or anything unprefixed; clean up in `t.Cleanup`. `neighbor-config` is global: set, read back,
   restore the previous value in `t.Cleanup`; never leave it changed.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations (incl. "no dump" cases).

## Rules
- Message names come from binapi. `apps/agent/binapi/` is **manager-owned** (generated for all plugins by P04, D-014): if a message is missing,
  write `docs/status/tasks/DF-2-questions.md` naming the plugin and continue with the rest; never edit `tools/binapi-gen.sh` or
  `apps/agent/binapi/` in your branch.
- Retrieve must decode *everything* the diff needs (all RA flags and timers, classify mask/match bytes, abf paths); a descriptor without
  Retrieve is not done. Where VPP has no dump, say so in the doc table and the questions file — never fake Retrieve from cached desired state.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`. No VPP restarts (D-012).

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/{ip_neighbor,arp,ip6_nd,urpf,adl,abf,classify,ip_session_redirect}/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/{ip_neighbor,arp,ip6_nd,urpf,adl,abf,classify,ip_session_redirect}` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed; `dad` test output shows the `skip-unless-plugin-loaded` reason
- [ ] `vppctl show ip neighbors` / `show ip6 interface loop<N>xx` / `show urpf` / `show abf policy` / `show classify tables` pasted for your prefixed
      objects, then empty after delete

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-* feature wiring, performance, startup.conf changes. Not yours: `vrf` / `static-route` /
`interface-ip` / `loopback` (P05 core), ACL descriptors (DF-4 — you only depend on its key), policer-classify (DF-7), MPLS/SR (DF-6/DF-7),
IGMP/multicast (DF-7), Auto-SDL (host-stack session layer, later), BFD (DF-7), linux-cp (DF-8). No binapi regeneration, no VPP restart,
no `local0`, no global `ip neighbor flush` / `clear` on the shared VPP.
