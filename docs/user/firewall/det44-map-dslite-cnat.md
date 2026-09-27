# CGNAT (DET44), MAP-E / MAP-T / LW4o6, DS-Lite, 464XLAT and CNAT

Carrier-grade and transition translators, all under `nat` in the configuration document and applied through the
usual candidate → diff → commit → rollback cycle.

> **Status of this build.** The agent programs and retrieves every translator below. The REST state/actions
> (DET44 session browser and lookup, CNAT sessions), the NAT page tabs (CGNAT, MAP, CNAT, PNAT) and PNAT (`nat.pnat`)
> are **not in this build yet** (see docs/status/tasks/F-det44-map-dslite-cnat.md). Configure through the configuration
> document / `vrx merge nat …` for now.

| translator | configuration | VPP plugin | what it is for |
|---|---|---|---|
| DET44 | `nat.det44` | `det44` | deterministic CGNAT: each subscriber gets a fixed outside address and port block, no per-session logging needed |
| MAP-E / MAP-T / LW4o6 | `nat.map` | `map` | stateless IPv4-over-IPv6 border relay (RFC 7597 / 7599 / 7596) |
| DS-Lite | `nat.dslite` | `dslite` | IPv4-in-IPv6 softwire with NAT on the AFTR (RFC 6333) |
| 464XLAT | `nat.nat64` (PLAT) + `nat.map` MAP-T (CLAT) | `nat64`, `map` | IPv4 apps over an IPv6-only access (RFC 6877) — no object of its own |
| CNAT | `nat.cnat` | `cnat` | service VIPs load-balanced to backends, plus a source-NAT policy |

## DET44 — deterministic CGNAT and sizing

```json
"det44": {"enabled": true, "inside": ["ge0/0/1"], "outside": ["ge0/0/0"],
  "mappings": [{"inside": "100.64.0.0/22", "outside": "203.0.113.0/28"}]}
```

Every inside host of a mapping shares an outside address with `2^(outside length − inside length)` others and gets a
fixed port block: **ports per host = 64512 / 2^(outside length − inside length)** (ports 1024–65535). `/22 → /28`: 64
hosts per address, 1008 ports each. VPP allows at most 2^15 hosts per address; the outside prefix may not be larger than
the inside one, and inside prefixes may not overlap (all refused with a pointer).

Because the mapping is algorithmic, a CGNAT log only needs the mapping itself: the outside address:port of a subscriber
(and back) is computable. Caveats:

- **V9 — det44 is never disabled.** VPP 26.06 crashes when det44 is disabled. Removing `nat.det44` (or rolling back)
  removes the interfaces and mappings, but the plugin stays enabled and idle until the next VPP restart. Changing
  `insideVrf` / `outsideVrf` after det44 was enabled is refused for the same reason.
- `det44` enable and timeouts are VPP-wide: on a shared VPP only the globals owner sets them.

## MAP-E vs MAP-T vs LW4o6

| | MAP-E | MAP-T | LW4o6 |
|---|---|---|---|
| data plane | IPv4 encapsulated in IPv6 | IPv4 translated to IPv6 (stateless NAT46) | IPv4 encapsulated in IPv6 |
| `domains[].ipv6Source` | BR address, /128 | DMR prefix, /64 or /96 | BR address, /128 |
| subscriber addressing | EA bits embedded in the IPv6 prefix | EA bits embedded | per-subscriber rules: `eaBitsLength: 0` + `rules[] {psid, ipv6Destination}` |
| interface `mode` | `map-e` | `map-t` | `map-e` |

The domain `mode` is a label for validation (VPP's `map_add_domain` has no mode); MAP runs only on the interfaces listed
in `map.interfaces` with their mode. `parameters.tcpMss` and `parameters.preResolve` are write-only in VPP 26.06 and are
not applied (a warning). MAP parameters are VPP-wide.

**Q3 default flip — existing documents.** `nat.map.parameters.securityCheck.enabled` and `trafficClass.copy` now
default to `true` (VPP's defaults). Documents already stored were parsed with the old defaults when saved, so they keep
an explicit `false`; only imports, restores and hand-written candidates that omit the leaves get `true`. There is no
migration: on an existing configuration, reset `securityCheck.enabled` / `trafficClass.copy` by hand if you want VPP's
defaults.

## DS-Lite

`dslite.aftr {ipv6, ipv4?}` sets the AFTR tunnel endpoint (the IPv4 address sources ICMP errors), `dslite.pools[]` the
IPv4 NAT pool. Configure either the AFTR or the B4 side. The AFTR and B4 addresses are VPP-wide (globals owner only)
and VPP cannot delete them: removing them resets them to `::` / `0.0.0.0`.

## 464XLAT recipe

The provider side (PLAT) is NAT64 (`nat.nat64` with the well-known or a network-specific /96 — see
[nat44-ei-64-66-nptv6.md](nat44-ei-64-66-nptv6.md)); the customer side (CLAT) is a MAP-T domain whose `ipv6Source` is
the PLAT's /96 and whose rule prefix covers the CLAT's IPv4 host address. No extra object exists.

## CNAT — service VIPs and SNAT policy

```json
"cnat": {
  "translations": [{"name": "web", "protocol": "tcp", "vip": {"ip": "198.51.100.10", "port": 80},
    "backends": [{"ip": "10.0.0.2", "port": 8080}, {"ip": "10.0.0.3", "port": 8080}], "lbType": "maglev"}],
  "snat": {"policy": "interface", "addresses": {"ipv4": "198.51.100.1"},
    "interfaces": [{"interface": "ge0/0/1", "table": "include-v4"}], "excludePrefixes": ["10.0.0.0/8"]}}
```

- A translation needs at least one backend of the VIP's address family (V10: VPP 26.06 crashes on an empty one).
- `snat.policy` `interface` / `k8s`, policy interfaces and excluded prefixes **need `snat.addresses`** (the default
  SNAT entry): without it the commit is refused with a pointer at `/nat/cnat/snat/addresses` (V10: VPP would crash).
- The cnat feature is enabled on every policy interface; the commit's validation report names those interfaces in an
  info notice (`nat.det44-map-dslite-cnat-cnat-interface-feature`).
- CNAT and NAT44-ED on the **same interface** are refused (no supported feature ordering in VPP 26.06).
- The SNAT policy, policy interfaces and excluded prefixes are write-only in VPP (not shown by Retrieve / drift);
  the SNAT addresses and policy are VPP-wide.

## PNAT — policy 1:1 NAT

```json
"pnat": {
  "bindings": [{"name": "dns", "match": {"proto": "udp", "dst": "198.51.100.53", "dport": 53},
    "rewrite": {"dst": "10.0.0.53", "dport": 5353}}],
  "attachments": [{"binding": "dns", "interface": "ge0/0/0", "point": "input"}]}
```

- IPv4 only. A match needs at least one field, a rewrite at least one; ports only with protocol `tcp`/`udp`.
- The match is the binding's identity: two bindings with the same match are refused. Binding names are labels only
  (the data plane keeps none); the applied state shows your names again, matched by the match, so no drift appears.
- All bindings on one interface and point must match the same fields (VPP refuses mixed masks).
- V11: the agent never looks up or detaches a PNAT flow while no interface has an attachment (that crashes VPP 26.06).

## Live state and actions (API / UI)

| what | API | role |
|---|---|---|
| one subscriber's DET44 sessions + its outside address and port block | `GET /api/v1/state/nat/det44/sessions?user=<ip>&page&pageSize` | readonly |
| DET44 lookup for CGNAT logging (inside → outside + ports, or outside + port → inside) | `POST /api/v1/actions/nat/det44/lookup` `{inside}` / `{outside, port}` | operator |
| close one DET44 session (audited) | `POST /api/v1/actions/nat/det44/sessions/close` `{direction: in\|out, address, port, externalAddress, externalPort}` | operator |
| CNAT sessions | `GET /api/v1/state/nat/cnat/sessions?page&pageSize` | readonly |
| purge the CNAT session table (VPP-wide: only the globals-owner agent does it; a test-slot agent answers 403) | `POST /api/v1/actions/nat/cnat/sessions/purge` | admin |

The NAT page has four tabs for this: **CGNAT** (DET44 + DS-Lite forms, the port-block calculator — enter the inside
and outside prefixes to see the sharing ratio, ports per host and the block of any host —, sessions of one subscriber
and the lookup), **MAP**, **CNAT** (form, sessions, purge for admins) and **PNAT**.

## CLI equivalent

```
vrx merge nat '{"det44":{"enabled":true,"inside":["ge0/0/1"],"outside":["ge0/0/0"],"mappings":[{"inside":"100.64.0.0/22","outside":"203.0.113.0/28"}]}}'
vrx commit
```

VPP's own view (read-only, for troubleshooting): `vppctl show det44 mappings`, `show map domain`, `show dslite aftr
endpoint`, `show dslite pool`, `show cnat translation`.
