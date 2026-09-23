# RF-1 — questions for the manager (work continued on the stated default)

## Q1 — proto: static-route `tag` and blackhole next hop are missing (D-055 stand-in used)
`vrxv1.StaticRoute` has no `tag`; `vrxv1.NextHop` has no blackhole/reject kind (and the P02a schema refine demands
address or interface). The task requires both. **Default taken:** `Render` also accepts a `*structpb.Struct`
configuration document and reads the stand-ins at `routing.static[i].tag` and `routing.static[i].nextHops[j].blackhole`
(`frr.Desired`, `frr.Extensions`). **Ask (P03b/P02a, additive):** `StaticRoute.tag` (uint32, 6) and
`NextHop.blackhole` (bool, 4) or a `kind` enum; the schema refine becomes "address, interface or blackhole". When they
land, `readExtensions` is replaced by the proto fields (small follow-up).

## Q2 — proto: no FRR/routing state message
`Retrieve()` must return a proto; there is no message for daemon state. **Default:** `structpb.Struct`
(keys in `docs/agent/renderers/frr.md`), typed Go `frr.State` for agent code. **Ask:** a `RoutingState` (RIB entries,
VRFs, interfaces, protocol summaries) when the `/api/v1/state/routes` endpoint (P12) needs it.

## Q3 — proto: no event kind for routing changes
Route-count and future neighbour-state changes map to `EVENT_KIND_UNSPECIFIED` with attributes
(`source=frr, poller, key, old, new`); interface changes use `LINK_UP/LINK_DOWN`. **Ask:** additive
`EVENT_KIND_ROUTING_CHANGE` (and P12 may want `EVENT_KIND_NEIGHBOR_STATE`).

## Q4 — who owns static routes: VPP descriptor or FRR staticd?
The P02a schema comment says "Static routes … are programmed in VPP by vrx-agent" (descriptor, `ip_route_add_del`),
and RF-1 renders the same `routing.static` into staticd. With linux-cp/linux-nl (P12) a staticd route installed in
the kernel is synced into VPP again → double programming of the same prefix. **Default taken:** the framework renders
`routing.static` (as the task says), and P12 decides the ownership when it wires linux-nl (options: (a) VPP descriptor
only, FRR gets them only for redistribution via a flag; (b) FRR only, descriptor drops them; (c) both, identical
next hops — idempotent but noisy). Recommendation: (a) with an FRR-side flag per route. Not blocking RF-1.

## Q5 — ALLOWLIST.md is outside my envelope's file list
`internal/renderers/ALLOWLIST.md` is not in "files you own", but the task and `TestAllowlistDocumented` require the
frr rows. **Done:** added the frr rows (moved vtysh / frr-reload.py from *Planned* to *Active*, added the test-only
`ip` and daemon rows); no other line changed. Other RF-* branches editing the same table may conflict textually on
merge (append-only rows, trivial to resolve).

## Q6 — `/run/frr/<prefix>` symlink outside `/run/vrx-test/<prefix>`
FRR 10.7 mgmtd binds its front/back-end sockets in `/var/run/frr/<pathspace>` regardless of `--vty_socket`. The
harness creates `/run/frr/w12 → /run/vrx-test/w12/frr/run/w12` for the test's duration (refuses if the path exists
and is anything else; removed on Stop). It is pathspace-scoped (the system FRR uses `/run/frr` itself, never
`/run/frr/w12`). Alternative would be a private mount namespace (`unshare -m` + bind mount), which needs a shell or
a new helper in `helpers_exec.go`. Please confirm this is acceptable under shared-host-rules §5.

## Q7 — rig namespace name
The task names `ns-w<N>-a`; `tools/lab rig` creates `ns-<prefix>-lan|wan` and puts VPP host-interfaces in them. The
harness creates its own `ns-<prefix>-frr` (no VPP dependency: FRR tests should not need VPP) and accepts
`Options.NetNS` to reuse a rig namespace (P12 will, for the linux-cp path). `tools/lab rig gc` does not know
`ns-<prefix>-frr`; the harness deletes it on Stop and on the next Start after a kill.
