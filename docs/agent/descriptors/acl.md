# acl plugin descriptors (DF-4, WBS D5.2)

Package `apps/agent/internal/descriptors/acl` — reconciler descriptors for the VPP 26.06 `acl` plugin: L3/L4 ACLs
(stateless and stateful/reflect), per-interface in/out ACL lists, ethertype whitelists, L2 MACIP ACLs and their
binding, the global hit-counter switch, and a typed reader for the per-rule hit counters in the stats segment.
Message names come only from `apps/agent/binapi/acl` (+ `acl_types`, `ip_types`, `ethernet_types`, `interface`).

## Key contract (what DF-2 / F-* build against)

| Object | Descriptor name | Key | Helper | Meta (runtime handle) |
|---|---|---|---|---|
| ACL | `acl.acl` | `acl.acl/<name>` | `acl.KeyACL(name)` | `acl.Meta{ACLIndex uint32}` |
| MACIP ACL | `acl.macip-acl` | `acl.macip-acl/<name>` | `acl.KeyMacipACL(name)` | `acl.MacipMeta{ACLIndex uint32}` |
| interface ACL lists | `acl.interface-binding` | `acl.interface-binding/<ifname>` | `acl.KeyInterfaceBinding(ifname)` | `acl.BindingMeta{SwIfIndex}` |
| ethertype whitelist | `acl.etype-whitelist` | `acl.etype-whitelist/<ifname>` | `acl.KeyEtypeWhitelist(ifname)` | `acl.BindingMeta{SwIfIndex}` |
| MACIP binding | `acl.macip-interface-binding` | `acl.macip-interface-binding/<ifname>` | `acl.KeyMacipBinding(ifname)` | `acl.MacipBindingMeta{SwIfIndex, ACLIndex}` |
| counters switch | `acl.stats-enable` | `acl.stats-enable/global` | `acl.KeyStatsEnable` | none |

- `<name>` is the object id a human uses (`lan-in`), never an index. The VPP tag is `"<owner>:<name>"`
  (`vpp.OwnerTag`; owner = `VRX_OWNER`, tests: `VRX_TEST_PREFIX`), ≤ 63 bytes; it is how Retrieve attributes ACLs.
- Another plugin that needs the **acl_index** of `acl.acl/<name>` (DF-2 `abf.policy`) depends on `acl.KeyACL(name)`
  (mandatory) and resolves the index with `acl.LookupIndex(ctx, client, owner, name)` (acl_dump + tag match) —
  Meta is private to the owning descriptor. Update never changes the index (`acl_add_replace` on the same index).
- `Register(registry, client, owner, opts...)` registers all six descriptors; `acl.WithInterfaceKey(f)` sets the
  interface key scheme used for the optional interface dependency (default `interface/<ifname>` until DF-1 is merged).

## Desired-state values

The P03 proto contract has no ACL messages yet, so values are `*structpb.Struct` documents built with the typed
specs in `spec.go` (`acl.ACL{...}.Proto()`, `acl.InterfaceBinding{...}.Proto()`, …) and read back with
`acl.FromProto` / `acl.InterfaceBindingFromProto` / …. **Always build values with `.Proto()`**: every field is
emitted (also zeros and empty lists) and text is canonical, so `proto.Equal(desired, Retrieve())` is a correct diff.
Swapping to the real proto type later touches `spec.go` only.

```jsonc
// acl.acl
{"name": "lan-in", "rules": [
  {"action": "permit",          // "deny" | "permit" | "reflect" (permit + session, stateful)
   "src": "10.0.0.0/8", "dst": "0.0.0.0/0",   // canonical CIDR, both v4 or both v6; AnyV4 "0.0.0.0/0", AnyV6 "::/0"
   "proto": 6,                                // IP protocol, 0 = any (ports ignored)
   "src_port_first": 0, "src_port_last": 65535,   // TCP/UDP port range; ICMP: type range (0–255)
   "dst_port_first": 80, "dst_port_last": 80,     // TCP/UDP port range; ICMP: code range
   "tcp_flags_mask": 0, "tcp_flags_value": 0}]}   // pkt.flags & mask == value; 0/0 = any
// acl.macip-acl
{"name": "l2-guard", "rules": [{"action": "permit", "src_mac": "02:00:00:00:00:01",
  "src_mac_mask": "ff:ff:ff:ff:ff:ff", "src": "10.10.1.0/24"}]}
// acl.interface-binding                       // complete ordered lists; both empty = unbound
{"interface": "loop1040", "input": ["lan-in", "lan-guard"], "output": ["lan-out"]}
// acl.etype-whitelist                         // strictly ascending ethertypes
{"interface": "loop1040", "input": [2054, 35020], "output": [2054]}
// acl.macip-interface-binding                 // one MACIP ACL per interface, inbound
{"interface": "loop1040", "acl": "l2-guard"}
// acl.stats-enable
{"enabled": true}
```

Canonical forms enforced by `Validate()` (a clear error instead of a perpetual Update): prefixes ==
`netip.Prefix.Masked().String()`; MACs == `net.HardwareAddr.String()` (lower-case, colons); no duplicate ACL per
direction; ethertypes strictly ascending; port/type ranges `first ≤ last`; ≤ 255 ACLs / ethertypes per interface.
VPP stores rules exactly as sent (acl.c `acl_add_list` / `copy_acl_rule_to_api_rule`), so the codec in
`rules.go` round-trips byte-identically (tested for prefix lengths 0–32/0–128, ports 0–65535, TCP flags, ICMP
ranges and wildcards).

## Object type ↔ VPP messages

| Object | Create | Update | Delete | Retrieve | Ownership filter | Dependencies |
|---|---|---|---|---|---|---|
| `acl.acl` | `acl_add_replace` (`acl_index=~0`, `tag`, `r[]`) | `acl_add_replace` on `Meta.ACLIndex` (index kept, bindings/ABF stay valid); name change → `ErrRecreate` | `acl_del` | `acl_dump` (`acl_index=~0`) → `acl_details` | `tag` parses as `<owner>:<name>` | none |
| `acl.macip-acl` | `macip_acl_add_replace` (`acl_index=~0`) | `macip_acl_add_replace` on index | `macip_acl_del` | `macip_acl_dump` (`~0`) → `macip_acl_details` | tag | none |
| `acl.interface-binding` | `acl_interface_set_acl_list` (`acls` = input ++ output, `n_input`) | same message (reorder = update) | same message with empty list | `acl_interface_list_dump` (`~0`) → `acl_interface_list_details`; `sw_interface_dump` for index→name | non-empty list **and** every ACL in it is ours | interface (optional, `WithInterfaceKey`), every listed `acl.acl` (mandatory) |
| `acl.etype-whitelist` | `acl_interface_set_etype_whitelist` (`whitelist` = input ++ output, `n_input`) | same | same with empty list | `acl_interface_etype_whitelist_dump` (`~0`) → `…_details` | interface tag parses as `<owner>:…` | interface (optional) |
| `acl.macip-interface-binding` | `macip_acl_interface_add_del` (`is_add=1`) | `macip_acl_interface_add_del` add with the new index (VPP unapplies the old one) | `is_add=0` | `macip_acl_interface_list_dump` (`~0`) → `…_details` | the bound MACIP ACL is ours | interface (optional), `acl.macip-acl` (mandatory, for ordering — see caveat) |
| `acl.stats-enable` | `acl_stats_intf_counters_enable` (`enable=1`) via raw stream, see caveat | same | **no-op** (never disables) | last value applied by this process (VPP has no getter) | n/a (global) | none |
| hit counters (`acl.StatsReader`, not a descriptor) | – | – | – | stats segment `/acl/<acl_index>/matches` (combined counters, one slot per rule + 1 spare, per worker) via `adapter.StatsAPI.DumpStats`; ACL names via `acl_dump` | tag | reads `acl.acl` objects |
| health (`acl.GetPluginInfo`) | – | – | – | `acl_plugin_get_version`, `acl_plugin_get_conn_table_max_entries` | – | – |

Registration order (`Register`): acl, macip-acl, interface-binding, etype-whitelist, macip-interface-binding,
stats-enable — the scheduler's deterministic tie-break.

## Stats segment paths (discovered on the host, `vpp_get_stats ls` / `StatsReader.ListPaths`)

| Path | Type | Meaning |
|---|---|---|
| `/acl/<acl_index>/matches` | combined counter vector, `[worker][slot]` `(packets, bytes)` | slot *i* = hits of rule *i* of that ACL; VPP registers the vector when the ACL is created (`validate_and_reset_acl_counters`) and allocates `rule_count + 1` slots; the data plane increments it only while `acl.stats-enable` is on. `StatsReader.ReadOwned` sums workers and truncates to the rule count. |
| `/err/acl-plugin-in-ip4-fa/*`, `/err/acl-plugin-out-ip6-fa/*`, `/err/acl-plugin-in-nonip-l2/*`, … | error counters | plugin node counters (permit/deny/sessions) — global, not per ACL; not read by this package |

There is no per-interface or per-MACIP-ACL hit counter in the stats segment.

## Caveats and limitations

- **stats-enable reply id.** VPP 26.06 `acl.c` answers `acl_stats_intf_counters_enable` with `REPLY_MACRO
  (VL_API_ACL_DEL_REPLY)`, i.e. the reply arrives as `acl_del_reply`. The generated `Invoke` rejects the mismatched
  id, so `acl.EnableCounters` sends the request on a raw `NewStream` and accepts either reply type. A VPP fix would be
  a one-line C change — not allowed in this plan (configuration-only fallback in place; recorded for
  `docs/vpp-code-track.md` by the manager).
- **No getter for the counters flag** (only `vppctl show acl-plugin tables` prints "Stats counters enabled for
  interface ACLs"). Retrieve of `acl.stats-enable` reports what this process applied; after an agent restart the
  scheduler enables once more (idempotent on VPP). The descriptor **never disables** the flag — on the shared VPP
  another owner may rely on it, and `enabled: false` is recorded but not sent.
- **Reflect / stateful ACLs**: `reflect` permits and creates a 5-tuple session so the return flow is permitted on
  the *same* interface in the *other* direction; sessions live in the plugin's connection table whose size is a
  `startup.conf` knob (`acl-plugin { connection count max N }`, read via `GetPluginInfo`) — out of scope here.
  Reflect has no meaning for MACIP rules (rejected by `Validate`).
- **Rule order is semantic**: desired order == sent order == `acl_details` order == Retrieve order (tested).
- **Bindings have no tag.** An interface whose ACL list mixes this owner's and a foreign owner's ACLs is not
  reported (never touched); a whitelist on an untagged interface (`local0`, a physical port DF-1 has not tagged) is
  not ours either.
- `acl_del` fails with `ACL_IN_USE_INBOUND/OUTBOUND/BY_LOOKUP_CONTEXT` while bound — the mandatory binding →
  ACL dependency makes the scheduler unbind first (and ABF policies, DF-2, must depend on `acl.KeyACL`).
- **`macip_acl_del` of a bound MACIP ACL succeeds** and unapplies it from every interface itself (acl.c
  `macip_acl_del_list`; verified on the host) — unlike `acl_del`, which fails with `ACL_IN_USE_INBOUND (-142)`.
  The binding → MACIP ACL dependency therefore only orders operations; a binding vanishes with its ACL.
- `acl_interface_set_acl_list` rejects an ACL listed twice in one direction (`ENTRY_ALREADY_EXISTS`) and unknown
  indexes (`NO_SUCH_ENTRY`); `Validate` catches the former before the call.
- Not built here (out of scope): API/UI/schema, classifier `input_acl_set_interface` (DF-2), ABF (DF-2), conn-table
  sizing, packet-level tests (F-*), `acl_plugin_use_hash_lookup_set` (left at VPP default).
