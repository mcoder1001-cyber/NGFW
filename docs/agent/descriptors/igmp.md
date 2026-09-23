# igmp descriptors (DF-7, WBS D2.9)

Package `apps/agent/internal/descriptors/igmp` — VPP IGMP plugin. Messages only from `apps/agent/binapi/igmp`.
DF-7 conventions: see `policer.md`. (Multicast FIB routes and PIM are F-* / P12, not here.)

| Object type | Key | Create / Update / Delete | Retrieve | Notes |
|---|---|---|---|---|
| `igmp.interface` | `igmp.interface/<if>` | `igmp_enable_disable` host/router; mode change = ErrRecreate; disable | **write-only** | VPP answers a repeated enable (and a disable of a disabled interface) with `-1` → success (idempotent resync); disabling also removes the interface from its proxy device |
| `igmp.listen` | `igmp.listen/<if>/<group>` | `igmp_listen` INCLUDE with sources (Update = new list, in place) / INCLUDE with no sources (leave) | `igmp_dump` (sources per group; VPP sends each twice — deduped) | host-mode interfaces only; ≥ 1 source (SSM); VPP 26.06 does not implement EXCLUDE listens; groups on interfaces this process configured as router mode are skipped (`Modes`), since `igmp_dump` also lists learned groups |
| `igmp.group-prefix` (**global**) | `igmp.group-prefix/<prefix>` | `igmp_group_prefix_set` SSM / ASM (removes) | **write-only** | the SSM range list is per VPP → `igmp.RegisterGlobals` only (D-071) |
| `igmp.proxy-device` | `igmp.proxy-device/<vrf>` | `igmp_proxy_device_add_del` (upstream = host-mode interface in that VRF) | **write-only** | VPP keeps an existing device (idempotent) |
| `igmp.proxy-downstream` | `igmp.proxy-downstream/<vrf>/<if>` | `igmp_proxy_device_add_del_interface` (router-mode interface) | **write-only** | `-1` on a repeated add = already downstream |

Events (`StreamEvents`): `igmp.WatchEvents` — `want_igmp_events` + `igmp_event` → `Event{Interface, Group, Source,
Filter}` for interfaces with this owner's `igmp.interface`. Host-mode static joins produce none (the host run logs
that); events come from router-mode learning. Action helper: `igmp.ClearInterface` (`igmp_clear_interface`).

Dependencies: listen → `igmp.interface/<if>` + `interface/<if>`; proxy-device → `vrf/<id>` + `interface/<up>` +
`igmp.interface/<up>`; downstream → `igmp.proxy-device/<vrf>` + `interface/<if>` + `igmp.interface/<if>`.

## VPP 26.06 quirk

`igmp_group_prefix_dump` sends its details with the `igmp_details` message id (igmp_api.c
`igmp_ssm_range_walk_dump`); govpp fails with `unexpected message: *igmp.IgmpDetails` (host probe) → write-only.

## FIB entries

`igmp.interface` adds mFIB entries (general query / report) in the interface's multicast table and removes them on
disable. No unicast FIB entries.

## Mode drift (review L4)

`igmp_enable_disable` answers `-1` both for "already enabled" and "enabled in the other mode"; there is no getter
for the mode. Create treats `-1` as "exists": accepted only on our tagged interface or with our live claim
(otherwise `ErrNotOurs`, review M1), but a mode changed underneath (host ↔ router) is not detected. Mode changes
through the descriptor are ErrRecreate (disable + enable), so drift only comes from outside the agent.

## Host test gated (D-087)

The host test ran together with the VRRP host test when VPP crashed (SIGSEGV in `ip4_options_node_fn`, IGMP
router-alert packets looped back on loopbacks; DF-7-questions Q9). It runs only with `VRX_DF7_IGMP_HOST=1`, alone,
in a manager window.
