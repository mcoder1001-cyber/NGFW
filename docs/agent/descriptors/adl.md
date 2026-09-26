# adl descriptors (DF-2, WBS D2.4 allow/deny lists)

Package `apps/agent/internal/descriptors/adl`. Messages from `apps/agent/binapi/adl` only.

| Object | Descriptor / key | Create / Update / Delete | Retrieve | Dependencies | Notes |
|---|---|---|---|---|---|
| ADL on an interface | `adl.interface` / `adl.interface/<ifname>` | `adl_interface_enable_disable`; Update → `ErrRecreate` | **presence** via `feature_is_enabled` (arc `device-input`, feature `adl-input`, `plugins/adl/adl.c`) on this owner's interfaces, **confirmed** (V23 a, below) | `interface/<ifname>`; `adl.allowlist/<ifname>` (optional: ordering) | Default `Register`. The adl API itself has no dump. |
| allow-list binding | `adl.allowlist` / `adl.allowlist/<ifname>` | two-call sequences (below): Create `(1,1,1)` then `(ip4,ip6,0)`; Delete `(!ip4,!ip6,1)` then `(0,0,0)`; Update → `ErrRecreate`; `default_adl` is refused (`ErrDefaultADL`) | **none — write-only** (`df2.ErrRetrieveUnsupported`) | `interface/<ifname>`, `vrf/<fib_id>` | **Not in the default `Register`** — only `RegisterWriteOnly(r, c, owner, WithBootStore(…), WithAllowlistClaims(…))`, for a reconciler implementing D-063. Applied-once record (D-076) keyed by the D-080 boot identity **and** the sw_if_index: a resync on the same VPP instance sends nothing, a VPP restart re-adds once. Removing it from the desired state does not disable it after an agent restart. |

## Ownership on untagged interfaces (review H3)
Objects on interfaces tagged `"<owner>:<name>"` are ours. Physical ports (DPDK NICs) carry no tag: an object created on
an untagged interface is recorded by its **key** in the claim store (`df2.WithClaims(store)`, DF-4's `acl.ClaimStore`
interface; `df2.FileClaimStore` persists it in the agent state dir) and Retrieve reports it only while claimed. Interfaces
tagged by another owner — and `local0` — are refused with `df2.ErrForeignInterface`. Dependencies use the interface
alias key `interface/<name>` (D-065, DF-1).

Registration: `adl.Register` = adl.interface; `adl.RegisterWriteOnly` = adl.allowlist (D-063 reconciler only). The product
agent (F-rpf-adl-pbr, `subsystems/rpf_adl_pbr.go`) passes the persisted "acl" claim store to both and the owner's BootStore
to the allow-list.

## V23 (a): `feature_is_enabled` answers true for VPP errors (F-rpf-adl-pbr fix)
`vl_api_feature_is_enabled_t_handler` stores `vnet_feature_is_enabled`'s negative `VNET_API_ERROR_*` in a bool: an unknown
arc or feature and a sw_if_index beyond the arc's config vector ("certainly not enabled" in VPP's own comment) all read
as enabled (host: an unknown feature on a fresh loopback reads `true`). Retrieve counts a `true` for adl-input only when
1. the control query `feature_is_enabled(device-input, ethernet-input, idx)` is false — the arc's end node is registered as
   a feature but never part of a config's feature list, so it is true only when the query failed (index out of range), and
2. `feature_is_enabled(device-input, adl-input, 0)` (local0, never ADL) is false — VPP knows the adl-input feature
   (probed once per Retrieve).

Otherwise the `true` was an error, and ADL is not on (index beyond the vector: nothing configured; plugin unknown:
nothing can be). Unit: `TestInterfaceRetrieveV23` (fake modelling the error encoding); host: `TestADLRetrieveV23OnHost`.

## The allow-list call sequences (V-new, F-rpf-adl-pbr)
`adl_allowlist_enable_disable` touches all three ADL families (ip4, ip6, default = non-IP) on every call: a set flag adds
one more allow-list feature (not idempotent: repeats stack), a clear flag removes one — and removing one that is not
configured stores config index `~0` for that family (`vnet_config_del_feature` returns `~0` and `adl.c` keeps it), which
`adl-input` dereferences for the next packet of that family (crash vector while adl-input is on; source analysis, not
reproduced on the shared VPP). The default family's allow-list node is a stub that does not free the frame's buffers. The
descriptor therefore only sends sequences that never remove an unconfigured family and never leave the default on:

| state before | call 1 | call 2 | state after (ip4, ip6, default instances) |
|---|---|---|---|
| none (0,0,0) | Create `(1,1,1)` → (1,1,1) | `(ip4, ip6, 0)` | (2·ip4, 2·ip6, 0) — a checked family holds two identical instances |
| (2·ip4, 2·ip6, 0) | Delete `(!ip4, !ip6, 1)` → (1,1,1) | `(0,0,0)` | (0,0,0) |

adl.interface depends (optionally) on adl.allowlist: the allow-list is configured while adl-input is still off, adl-input
is switched off before the allow-list is removed, and a changed allow-list (recreate) re-creates adl.interface around it.

Known window (review L3): two paths run remove+add while adl-input may be on — Create after an agent restart when the
recorded value on the same index differs (the "desired changed while the agent was down" branch), and the scheduler's
undo of a failed recreate (it does not wrap dependents). Between the two calls the default family holds one instance,
so a non-IP frame arriving in that window reaches the leaking stub; no path stores `~0`. Accepted (rare, bounded to one
call gap); the fix would be to switch adl-input off around those two paths.

## Deletes re-verify identity (D-071, fix round 2)
A Delete that acts on a stored `sw_if_index` first re-dumps the interfaces (`df2.SkipDelete`): the index must still name the
object's interface and that interface must still be ours (own tag, or untagged and the key claimed). Otherwise nothing is
sent to VPP — our object went with the interface, or the index now belongs to someone else — and only the claim is dropped.
