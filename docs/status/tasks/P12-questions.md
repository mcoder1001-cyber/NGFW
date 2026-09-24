# P12 — questions for the manager (work continues on the stated default; never waiting)

Slot 8 (`w8`, tables 8000–8999, rig 10.8.{1,2}.0/24), daemon-owner frr (slot instances only). Written 2026-09-24 23:40,
**before the first host run** (envelope coordination (4)).

## Q1 — linux-nl test plan (envelope coordination (4)) — asks for a manager window

Facts (VPP 26.06 source, `src/plugins/linux-cp/lcp_nl.c`, read-only):
- linux_nl opens its netlink socket **when the first LCP pair of the whole VPP is added** (`lcp_nl_pair_add_cb`, socket
  NULL → `lcp_nl_open_socket`), in the netns that is the **lcp default netns at that moment**
  (`lcp_get_default_ns_fd` + `setns`), and **closes it when the last pair is deleted** (`lcp_nl_pair_del_cb`).
- Today: no pair exists (`vppctl show lcp` lists none), default netns `<unset>` → the next first pair opens the socket in
  the **root** netns. The pair's own `netns` field only places the tap; it does not move the socket.
- Kernel table → VPP table: identity, except 254/255 → 0 (`lcp_router_table_k2f`); FRR routes (proto > static) get FIB
  source `lcp-rt-dynamic`, kernel/static ones `lcp-rt`.
- VPP→Linux: tap carrier follows the phy's **hardware link** always (`lcp_itf_pair_link_up_down` → `tap_set_carrier`);
  admin state and MTU are copied once at pair creation; later admin/MTU/address changes are synced VPP→Linux only with
  `lcp lcp-sync on` (a VPP global, no binary API: startup.conf `linux-cp { lcp-sync }`), which is off on this host.
  Linux→VPP (admin, MTU, addresses, routes, neighbours) is linux_nl's, from the netns its socket lives in.

So the VRX-side FRR's kernel routes reach VPP only if that FRR runs in the netns the socket opened in. Options:

| | how | FIB proof | host impact |
|---|---|---|---|
| **T1 (default, always run)** | VRX-side FRR (frrtest) in its own netns `ns-w8-frr`; LCP pairs `host-w8l0 ↔ w8-l0`, `host-w8w0 ↔ w8-w0` created **with that netns** (pair field, no global); tap addresses rendered by FRR (`interface w8-l0 / ip address …`, see Q5); peers = 2 frrtest instances in the rig's `ns-w8-lan` / `ns-w8-wan` | BGP sessions over the punt path, 200 prefixes in FRR's RIB, route-map half, withdraw, link-down → BGP notices, agent restart, rollback — **not** the VPP FIB (linux_nl listens in root) | none beyond the slot: no global, no root-netns daemon |
| **T2 (opt-in `VRX_P12_LINUXNL=1`, manager window)** | as T1, plus: `flock -x /run/lock/vrx-globals.lock`; refuse unless **no** LCP pair exists and default netns is unset; `lcp_default_ns_set ns-w8-frr` → the agent's first pair opens linux_nl's socket in `ns-w8-frr` → restore default netns to exactly the saved value (unset) at once; the socket stays in `ns-w8-frr` while any pair exists | full: routes in `ns-w8-frr`'s main table → VPP table 0 via linux_nl, prefixes only inside the slot's `10.8.0.0/16` (`10.8.64.0/25`… peer 1, `10.8.160.0/25`… peer 2), `ip_route_dump` / ListRoutes `source=lcp-rt-dynamic` counts 200 / 100 / 0 | the global is changed for < 1 s; while my pairs exist linux_nl hears only `ns-w8-frr` (nobody else depends on linux_nl today: P11 uses its fixture pair for IKE punt only). The globals lock is held **for the whole test**, so DF-8's host test (LockGlobals, shared) and any other pair creator waits; after my pairs are deleted the socket closes (no pair left) |
| T3 (alternative) | root-netns zebra/bgpd (a frrtest root mode, gap) with a kernel VRF `w8vrf` table 8001, taps in root enslaved to it; BGP `vrf w8vrf` → VPP table 8001 | full, table 8001 | a root-netns zebra is host-wide (startup sweep of FRR-proto kernel routes, sees ens192); one slot at a time; product-like only if the product runs FRR in root |

**Default taken:** T1 always; T2's code path is in the topology test behind `VRX_P12_LINUXNL=1` and refuses to run
without the conditions above. **Ask:** a manager window for one T2 run (≈ 10 min, slot 8; the test holds the globals lock
exclusively and prints `show lcp` / default netns before and after). T3 is not implemented (no frrtest root mode).
Product layout = either FRR in root with default netns unset, or TNSR-style `linux-cp { default netns dataplane }` +
FRR in that netns (startup.conf, F-startup-gen) — both work with the same agent code (the pair's `netns` leaf).

## Q2 — LCP pair leaf: `Interface` 22 `lcp` (chosen) vs `RoutingConfig` 12

`interfaces.<name>.lcp: {hostIfName?, hostIfType? (tap|tun, default tap), netns?}` — present = the agent creates the
linux-cp pair (DF-8 `lcp.itf-pair`). `hostIfName` default = the VPP name when it is a valid Linux name ≤ 15 bytes, else a
validation error asks for it (no silent truncation/rename). Parent interfaces only (sub-interfaces: no allocation, later).
Why the interface: the pair is a property of one interface (like TNSR's per-interface host pair), the W-seed anchor
sits there, and a routing-level list would duplicate interface names. `RoutingConfig` 12 stays reserved.

## Q3 — renderer stage (envelope coordination (1)): one singleton descriptor `frr.config/vrx` (default (a), D-109 d)

In `Domains["routing"]`; desired only when the document has FRR content (bgp, non-empty policy, a `viaFrr` static, or an
interface with `lcp` + address/description to render), so an agent without FRR (CI, product boxes without routing)
never calls vtysh. Value = the FRR-relevant subset of the document (secret *references* only, never values). Create/Update
= Render → Validate (`vtysh -C`) → Apply (`frr-reload.py --reload` + convergence check); Delete = apply the empty
framework config (tolerant when FRR is not running). Retrieve = the last applied value when `frr-reload.py --test` shows no
diff, a drift marker otherwise (→ Update re-applies), nothing after an agent restart (→ Create = idempotent re-apply,
frr-reload computes an empty diff; sessions are untouched). TD-13 (scheduler Validator stage) will move Validate earlier;
until then DryRun gets the render-time errors from the projection (Q4).

## Q4 — `passwordRef` before PENDING-secret-channel (envelope coordination (2))

Product wiring has no resolver: a neighbour or peer group with `passwordRef` is refused at validation time
(`routing.bgp-password-unavailable`, pointer `/routing/bgp/neighbors/<addr>/passwordRef`) with the text "BGP MD5 passwords
need the API→agent secret channel (PENDING-secret-channel); remove passwordRef or wait". Tests use a fixture resolver
(`VRX_TEST_PSK_P12_<n>`), and the redaction path is unit-tested (DryRun, errors, Retrieve).

## Q5 — who puts the VPP interface addresses on the Linux tap

With `lcp-sync` off (host global) VPP does not copy addresses to the tap, and BGP needs them to source its sessions.
Options: (a) the FRR renderer renders `interface <tap> / ip address A/L` for every LCP interface (zebra installs them;
linux_nl mirrors them back into VPP where they already exist — idempotent), through the S2 hook; (b) the agent sets them
by netlink (new privileged code, a netlink dependency — D4); (c) require `lcp-sync on` (a global, not on this host).
**Default (a)**: product-safe, no new dependency, frr-reload manages add/remove, and it keeps working with (c) enabled.

## Q6 — FRR→VPP route sync and TD-8 V1 (manager addendum)

The route sync is linux_nl's (AD-2): the agent programs no BGP route and registers **no** dynamic desired source, so TD-8's
per-source all-or-nothing (V1) cannot starve routes — linux_nl applies each netlink route on its own. The agent's part is
bounded reads (`show bgp … summary json`, per-neighbour counts, RIB **summary** — never a full-table dump; `/state/routes`
reads VPP through F-vrf-static-ecmp's streamed ListRoutes). If a later decision moves routes to an agent-side sync (e.g.
a box whose FRR netns cannot host linux_nl), it must wait for TD-8b's per-key quarantine.

## Q7 — non-default BGP VRF ↔ kernel VRF device

`routing.bgp.vrf: X` renders `router bgp N vrf X`; FRR needs a Linux VRF device `X` (table = the VPP table id) in its
netns with the LCP taps enslaved, and linux_nl maps that kernel table 1:1 to the VPP table. Nobody creates kernel VRF
devices today (no netlink/`ip` in the agent product allowlist). **Default:** P12 renders it and documents the
requirement; creating the device (agent netlink vs startup/netplan) is a follow-up question — the topology test uses the
default VRF (Q1). Semantic rule warns when bgp.vrf ≠ default.

## Q8 — `/state/routes?proto=bgp` (P2 shared hunk)

VPP only knows the FIB source (`lcp-rt` = kernel/static proto, `lcp-rt-dynamic` = routing daemons), not FRR's protocol.
**Default:** the API maps `origin` `lcp-rt*` → `frr`, and `proto=<p>` = ListRoutes `source=lcp-rt-dynamic` (or `lcp-rt`
for `static`/`kernel`) plus a per-page annotation from the agent's `RoutingState` RIB lookup of that page's prefixes
(bounded: one page ≤ 1000 prefixes). With BGP the only dynamic protocol in P12, `lcp-rt-dynamic` ≡ BGP; F-ospf adds its
protocol name to the same map.

## Q9 — contract numbers taken (wave-BC-numbers.md "Batch-2 follow-ons")

`Interface` **22** `lcp` (`InterfaceLcp`), `StaticRoute` **8** `tag`, `EventKind` **14** `EVENT_KIND_ROUTING_CHANGED`,
**15** `EVENT_KIND_BGP_NEIGHBOR_CHANGED`; RPC `RoutingState` (messages `RoutingState*`, `BgpState*`, `RoutingLcp*`) in
the `// ----- P12 -----` section. Nothing else. Committed first as `contract(schema|proto): …` on `task/P12`.

## Q10 — frrtest multi-instance (framework gap, envelope: gap-only)

The topology needs three FRR instances in one slot (VRX side + two peers). frrtest has one pathspace/lock/base per prefix
and requires `Options.NetNS` to contain the prefix; a second prefix (`w8p1`) would live outside `/run/vrx-test/w8`
(shared-host §5). **Done (gap):** `Options.Instance` → pathspace `<prefix><instance>`, base
`/run/vrx-test/<prefix>/frr-<instance>`, lock `/run/vrx-test/<prefix>/frr-<instance>.lock`, symlink `/run/frr/<pathspace>`;
the namespace check stays on the slot prefix. Existing callers unchanged.

## Q11 — seams S2/S3 (D-119 M4, confirmed)

S2 `frr.RegisterInterfaceLines(name, fn)`: protocol/LCP lines inside the framework's `interface X … exit` block (the block
is rendered when a description or registered lines exist). S3: projection.go's routing warning is table-driven, one row
per leaf with a `handled` flag; `bgp` and `policy` are handled by P12; `wave-BC` anchors for F-ospf, F-isis-rip,
F-bfd-redistribution sit in the table.
