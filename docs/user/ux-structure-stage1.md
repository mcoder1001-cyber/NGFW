# Navigation structure — stage 1

This branch adapts the organization of the existing NGFW screens to the reference frontend supplied at `/frontend`. The current theme, forms, tables, permissions, configuration transactions and API contracts are retained.

| Navigation section | Existing screens |
| --- | --- |
| Routing | VRFs, static routes, FIB, neighbors, policy routing, Multi-WAN, MPLS, multicast |
| Dynamic routing | OSPF, IS-IS/RIP, BFD, redistribution, BGP |
| Routing objects | Prefix lists, route maps, usage and order |
| Firewall / NAT | Policies, standalone NAT, existing security screens |
| Objects | Overview, addresses/groups, services/groups, schedules, zones, tags |
| Tools | Existing capture/delay simulator, direct access to Ping |

Policies opens `/firewall/policies`. The legacy `/firewall/acl` URL still opens the same page and keeps tab/list parameters. A policy set is the existing named L3/L4 access list; its rules and interface/zone attachments retain their existing meaning. MACIP and host ACL remain technically distinct.

Objects links open existing tabs under `/firewall/objects`; existing URLs and in-page tabs remain usable. Query-aware navigation highlights one destination even when several sidebar links use the same page.

Prefix lists and route maps are now directly accessible under `/routing/objects?tab=prefix-lists|route-maps`, using the same editors and `routing.policy` configuration as before. Existing BGP tabs and `/routing/policy` links remain available for compatibility. Static route IPv4/IPv6 filtering and combined IS-IS/RIP screens are retained because the new product already handles them in shared editors.

NAT remains an independent configuration page. The reference's SNAT/SPAT Policy tab is not added. Reference zone-pair policies, zone Route/Bridge modes and Intra Zone Blocking require a product-model decision and are outside this structural stage.

The branch is for owner review. It is neither merged to main nor deployed; owner approval is required before integration.
