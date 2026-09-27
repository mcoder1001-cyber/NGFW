# FRR renderer — `rip` section (F-isis-rip)

Package `apps/agent/internal/renderers/frr/rip`, order 420 (docs/status/wave-BC-numbers.md; `ripng` 430 is reserved for
this package). Registers the `rip` section from `init()`; wired by the blank import in
`apps/agent/internal/subsystems/isis_rip.go`; `desired.FRRDoc` / `AssembleFRR` carry `routing.rip`.

## Canonical form
```
router rip [vrf <name>]
 version 2
 default-metric <1-16>                 (when set)
 network <IPv4 prefix>                 (document order)
 network <linux-if>                    (every interface under interfaces, sorted by VPP name)
 passive-interface <linux-if>
 redistribute <connected|static|ospf|isis|bgp> [metric 0-16] [route-map R]
exit
```
RIP has no interface-level lines, so no interface-lines producer is registered.

## Validation (errors at the document pointer)
Networks: IPv4 prefix, host bits clear, no duplicates. Default metric 1–16, redistribute metric 0–16, redistribute
into itself, route-map names, unmapped / hostile / colliding interface names, VRF name.

## Not built
- RIPng (`ripng`, 430): `routing.ripng` does not exist in the contract.
- `version` leaf (always 2), authentication.
- State: FRR 10 has no JSON for `show ip rip status` / `show ip rip`, and the framework's state readers accept only
  `show … json`; RIP routes are visible through the framework route readers (`proto=rip`).
