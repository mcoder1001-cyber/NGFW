# FRR renderer — `isis` section (F-isis-rip)

Package `apps/agent/internal/renderers/frr/isis`, order 470 (docs/status/wave-BC-numbers.md). Registers from `init()`:
the `isis` section, the `isis` interface-lines producer (S2), the state reader `isisNeighbors`
(`show isis vrf all neighbor json`) and the poller `isis-adjacencies`. Wired by the blank import in
`apps/agent/internal/subsystems/isis_rip.go`; `desired.FRRDoc` / `AssembleFRR` carry `routing.isis`.

## Canonical form
```
interface <linux-if>
 ip router isis vrx
 ipv6 router isis vrx
 isis circuit-type level-1|level-2-only|level-1-2   (when circuitType is set)
 isis network point-to-point                       (networkType point-to-point; broadcast renders nothing)
 isis metric <1-16777215>
 isis passive
 isis bfd
exit
router isis vrx [vrf <name>]
 is-type level-1|level-2-only|level-1-2            (level, default level-1-2)
 net <NET, lower case>
 metric-style wide
 redistribute ipv4|ipv6 <src> level-1|level-2 [metric N] [route-map R]
exit
```
Redistribution: one line per level the IS runs; IPv6 lines for connected, static and bgp only (ospf/rip have no IPv6
twin under the same name).

## Validation (errors at the document pointer, `frr.ErrInput`)
`net` required and an ISO NET (`AA(.AAAA){3,9}.00`); level / circuit type names; a circuit type outside the IS level
(level-1 IS with a level-2 or level-1-2 circuit, and the reverse); network type; metric range; unmapped or hostile
interface names; two VPP interfaces on one Linux interface; redistribute into itself; route-map names; VRF name.

## State
`ParseNeighbors` reads `{"vrfs":[{"vrf","areas":[{"area","circuits":[{"adj"|"system-id","interface","level","state"|"adj-state"}]}]}]}`
or a single instance's `{"areas":[…]}`; circuits without an adjacency are skipped. Poller key
`<vrf>|<area>|<system id>|<interface>|<level>` → state. The field names are taken from FRR source reading and are
**not verified against a live FRR 10.7**. `show isis database json` is not read (unbounded LSDB).

## Not built (needs an additive contract or a lab)
Per-family switch (`interfaces.<if>.{ipv4,ipv6}`), area/domain passwords, EventKind 21 adjacency events, the
`lcp.osi-proto` globals descriptor (OSI punt to the LCP tap — without it IS-IS PDUs never reach FRR on a real box).
