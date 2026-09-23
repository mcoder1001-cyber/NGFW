# Task P12 — FRR + linux-cp framework, BGP basic   (prepend 00-CONTEXT.md)

## Goal
Dynamic routing: FRR runs on the Linux side, VPP's `linux-cp` plugin mirrors VPP
interfaces into Linux and `linux-nl` syncs the kernel FIB into VPP. Deliver the framework
plus BGP with neighbours, address-families, prefix-lists and route-maps. OSPF/BFD are
separate F-tasks that reuse this framework.

## Read first
VPP 26.06 `linux-cp` and `linux-nl` plugin docs; FRR 10 docs (`frr-reload.py`, JSON
show commands); `docs/01-architecture.md` AD-2; P11's renderer pattern (copy it).

## Precondition (checked first)
`vppctl show plugins` must list `linux_cp_plugin.so` and `linux_nl_plugin.so`. On this host they are **on disk but not loaded**
(`docs/lab/host-vrx-a.md`). Do **not** edit `/etc/vpp/startup.conf` yourself: this task is `parked_on: handover`; when the manager
unparks it the plugins are enabled via the D0.6 generator. If you are spawned and they are still missing, write
`docs/status/tasks/P12-questions.md` and do everything below that does not need them (renderer, schema, API, UI, tests with `t.Skip`).

## Build exactly this
1. **Lab**: FRR is installed on this host next to VPP (same kernel); VPP runs with `linux-cp` + `linux-nl` plugins enabled; agent creates LCP pairs (`lcp_itf_pair_add_del_v2`) for
   every routed VPP interface (tap named `vpp<idx>`/or the interface name), syncs MTU/state
   both ways, handles the netns.
2. **Contract PR first** (`contract` label): `routing.bgp{asn, routerId, neighbors{ip →
   remoteAs, description, password(secretRef), ebgpMultihop, updateSource, afi{ipv4Unicast,
   ipv6Unicast → {enabled, routeMapIn, routeMapOut, prefixListIn, prefixListOut, nextHopSelf,
   softReconfig}}, bfd}, networks[], redistribute{connected,static}, gracefulRestart}`,
   `routing.policy{prefixLists{name → [{seq, action, prefix, ge, le}]}, routeMaps{name →
   [{seq, action, match{prefixList, community, asPath}, set{localPref, med, community, nextHop}}]}}`.
   Proto messages to match.
3. **Agent renderer** (`internal/renderers/frr`): render `/etc/frr/frr.conf` (bgpd + zebra +
   staticd sections; `vtysh.conf`), validate with `vtysh -C -f`, apply with
   `frr-reload.py --reload` (diff-based, no restart), `Retrieve` from `vtysh -c "show bgp
   summary json"`, `show bgp ipv4 unicast json`, `show ip route json`; events on neighbour
   state change (poll 1 Hz for now). Both vtysh invocations go through a **fixed argv, no
   shell**, allow-listed in the CI grep gate.
4. **FIB verification**: routes learned by FRR must appear in VPP (`ip_route_dump` via binapi).
   `GET /api/v1/state/routes?vrf=&proto=bgp` reads VPP FIB and annotates with FRR source.
5. **API/UI**: BGP global + neighbours (state, uptime, prefixes rx/tx, flaps), prefix-list and
   route-map editors (SchemaForm with array widgets), redistribution toggles; en+fa.
6. **Topology test** (single host): VRX ↔ two FRR instances running in network namespaces on the veth rig (`zebra/bgpd -N <ns>` with their own
   config dirs — never the system FRR unit), eBGP, each announcing 100 prefixes; after
   commit: `vppctl show ip fib` contains all 200; withdraw on peer → gone from VPP within 5 s;
   apply route-map denying half → 100 remain; agent-restart simulation (and, after handover, the manager's `kill -9 vpp`) → LCP pairs +
   BGP sessions recover and FIB is repopulated without API involvement; rollback of the whole BGP config → sessions torn
   down cleanly, FIB empty of BGP routes.

## Acceptance
- [ ] Routes present in **VPP** FIB, not only in `vtysh` (this is the whole point)
- [ ] `frr-reload.py` used for changes; FRR never restarted on a config edit
- [ ] Link down on a VPP interface propagates to the Linux pair within 1 s and BGP notices

## Out of scope
OSPF, IS-IS, RIP, BFD (F-tasks), full-table performance, VPNv4, BGP over IPsec.
