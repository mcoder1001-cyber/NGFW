# FRR renderer: `ospf` section (F-ospf)

Package `apps/agent/internal/renderers/frr/ospf`, registered from `init()` and blank-imported by
`apps/agent/internal/subsystems/ospf.go`. It plugs into the RF-1 framework like P12's `bgp` section.

| Piece | Name | What |
|---|---|---|
| Section | `ospf`, order 440 | `router ospf [vrf <v>]` … `exit` |
| Interface lines (seam S2) | `ospf` | `ip ospf …` inside the framework's `interface <linux-name>` block |
| State reader | `ospfNeighbors` | `show ip ospf vrf all neighbor json` |
| State reader | `ospfInterfaces` | `show ip ospf vrf all interface json` |
| Poller | `ospf-neighbors` | key `<vrf>\|<router id>\|<interface>` → FRR state (`Full/DR`, …); events use the framework's generic `EVENT_KIND_UNSPECIFIED` + attributes (no OSPF EventKind yet) |

## Rendered form

```
interface w8-l0
 ip ospf network point-to-point
 ip ospf cost 10
 ip ospf hello-interval 2
 ip ospf dead-interval 8
 ip ospf priority 0
 ip ospf area 0.0.0.0
 ip ospf bfd
 ip ospf passive
exit
!
router ospf
 ospf router-id 10.8.9.1
 redistribute connected metric 20
 redistribute static
 redistribute bgp route-map rm-in
 area 0.0.0.7 nssa
 area 51 stub no-summary
 default-information originate always
exit
```

Full example: `apps/agent/internal/renderers/frr/ospf/testdata/full.golden`.

## Rules (render-time, errors at the field's pointer)

- Area keys: decimal uint32 (no leading zeros) or a dotted quad; `0` and `0.0.0.0` together are refused. An interface's
  `area` must resolve to an area under `areas` (area 0 is not implied); the interface line spells the area as the
  `areas` key does (FRR keeps the format per area).
- `stub` / `nssa` refused for the backbone; `noSummary` refused on a normal area. Normal areas render no `area` line.
- Interfaces: VPP name → Linux name via the linux-cp mapper (`rc.MapInterface`); unmapped, unsafe names or two VPP
  interfaces on one Linux name are errors. Cost 1–65535, priority 0–255, dead interval must exceed hello.
- Redistribution in FRR route-type order (connected, static, rip, isis, bgp); `ospf` into itself refused; metric
  0–16777214; route-map names validated (existence is the schema's rule).
- `defaultInformationOriginate`: `on` → `default-information originate`, `always` → `… always`.

## Wiring outside the package

- `desired.FRRDoc` / `AssembleFRR` carry `routing.ospf` into / out of the `frr.config/ngfw` singleton.
- `projection.go`: the `ospf` routing leaf is `handled` (no more agent.unsupported-field warning).

## Not done / open

- **OSPFv3 (`ospf6`, order 450)**: no schema/proto model (`routing.ospf6`) exists; adding it is a contract change
  (RoutingConfig field 13 per wave-BC-numbers.md) that this task did not make. Nothing is registered for `ospf6`.
- **MD5 auth** (`OspfInterface.auth`, field 9): no model; `frr.RedactPatterns` already masks `ip ospf
  message-digest-key` / `authentication-key`.
- Line order inside the blocks follows FRR's `show running-config` from memory of the ospfd sources and is **not proven
  against a live FRR 10.7** here (frrtest/topology could not run). frr-reload compares per context, so an order mismatch
  would only show up as a cosmetic diff, but the convergence check should be run on the lab rig.
- The product FRR `daemons` file must enable `ospfd`.
