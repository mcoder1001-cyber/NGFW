# cnat descriptors (DF-3, D4.6)

Package `apps/agent/internal/descriptors/cnat`. Bindings are in `apps/agent/binapi/cnat` (plugin `cnat_plugin.so`,
loaded on vrx-a) plus `binapi/feature`. Entry point: `cnat.Register(registry, client, owner)`. The carrier is
`*structpb.Struct` built from typed specs (D-055).

| Descriptor | Key id | Create / Delete | Update | Retrieve | Dependencies | Notes / limitations |
|---|---|---|---|---|---|---|
| `cnat.translation` | `<vip>/<proto>/<port>` | `cnat_translation_update` (VIP endpoint, ≥1 path, lb type, flags, is_real_ip) → Meta `{ID}` / `cnat_translation_del(id)` | in place: `cnat_translation_update` on the same (vip, port, proto) keeps the id | `cnat_translation_dump`, owned when the VIP address is in the owner's scope | optional `cnat.snat-addresses/global` | Address endpoints only (sw_if_index `~0`); interface-resolved endpoints are not modelled. Path flags are masked to `CNAT_EPT_NO_NAT`, because the other bits in the dump are internal tracker state. **Write-only in 26.06:** `flags` (alloc-port / no-return-session / no-client), `is_real_ip` and `flow_hash_config` are not in the details. They are kept in an in-process cache by id, so after an agent restart one in-place update re-applies them. `flow_hash_config` is not modelled. |
| `cnat.snat-addresses` | `global` | `cnat_set_snat_addresses` (ip4 / ip6, or an interface; no interface = sw_if_index `~0`, never 0 = local0) / the same message with all zeros = delete the default entry | recreate (the v1 setter cannot clear one family; VPP drops the policy, interface tables and excluded prefixes together with the entry, so dependents are recreated) | `cnat_get_snat_addresses` (retval FEATURE_DISABLED = none); owned when an address is in scope or the interface is owned | `interface/<name>` when interface-based | Global singleton (the entry in `CNAT_FIB_TABLE`). An interface-based entry reports only the interface, because its addresses are derived. The v2 message (`cnat_set_snat_addresses_v2`, per-FIB entries, flags) and `cnat_snat_addresses_dump` are not modelled: one default entry is enough for the NAT features. |
| `cnat.snat-policy` | `global` | `cnat_set_snat_policy` (none / if-pfx / k8s / dnat); Delete sets `none` | in place | **no getter**: in-process cache, reported only while the owned default entry exists | `cnat.snat-addresses/global` | Guarded: sent only after `cnat_get_snat_addresses` confirms a default entry exists (see hazards). |
| `cnat.snat-interface` | `<interface>/<table>` | `cnat_snat_policy_add_del_if` (table include-v4 / include-v6 / pod / host) | recreate | **no dump**: in-process cache, filtered by existing interfaces and the owned default entry | `cnat.snat-addresses/global`, `interface/<name>` | FEATURE_DISABLED on add → `ErrNoSnatDefault`. |
| `cnat.snat-exclude-prefix` | `<prefix>` | `cnat_snat_policy_add_del_exclude_pfx` | recreate | **no dump**: in-process cache (same rules) | `cnat.snat-addresses/global` | Guarded like snat-policy. |
| `cnat.interface-feature` | `<interface>` | `feature_cnat_enable_disable` | recreate | `feature_is_enabled(ip4-unicast, cnat-input-ip4)` on owned interfaces | `interface/<name>` | The message enables the cnat nodes on ip4/ip6 unicast and output together. |

Retrieve-only state and actions: `Sessions(offset, limit)` over `cnat_session_dump` (paged in the agent) and
`PurgeSessions` (`cnat_session_purge`, which is global).

**VPP 26.06 hazards (verified in `src/plugins/cnat`, guarded in code and in the fake):**
- `cnat_set_snat_policy` and `cnat_snat_policy_add_del_exclude_pfx` use `cnat_snat_policy_entry_get_default()`
  without a NULL check (`ASSERT` only), so with no default SNAT entry they dereference NULL and crash VPP. The
  descriptors call `cnat_get_snat_addresses` first and return `ErrNoSnatDefault`; a delete becomes a no-op.
- `cnat_translation_update` with `n_paths = 0` calls `vec_validate(paths, n_paths - 1)`, which underflows. The
  descriptor rejects this with `ErrNoPaths`.
- `cnat_set_snat_addresses` treats `sw_if_index = 0` as "use local0", so "no interface" must be sent as `~0`.

**Restart safety:** the snat-policy, snat-interface and snat-exclude-prefix objects have no VPP getter. After an agent
restart their caches are empty, so the scheduler re-creates them. That is idempotent for the policy and the interface
bitmap. For an excluded prefix it bumps a refcount in VPP: harmless, and it disappears with the default entry. Open
question Q6 in `DF-3-questions.md`.

Tests: `cnat_test.go` runs on the fake. It covers create, idempotent re-apply, in-place translation update, write-only
flags after a restart, foreign VIPs and a foreign SNAT entry, the crash guards (the fake fails the test if a guarded
message is sent without a default entry), and the ~0 interface. `cnat_integration_test.go` runs on the host: VIP
`10.9.47.1` tcp/80 and udp/53 (maglev), the cnat feature on `loop940`, SNAT `10.9.49.1` / `fd00:9::4901`, policy
if-pfx, `loop941` in include-v4, and excluded `10.9.50.0/24`. The default SNAT entry is removed again at the end.
