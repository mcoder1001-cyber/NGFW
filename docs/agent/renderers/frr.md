# FRR renderer — desired state ↔ rendered directives (RF-1 framework)

Code: `apps/agent/internal/renderers/frr` (README there: layout, commands, FRR 10.7 facts, test harness).
Files: `/etc/frr/frr.conf` and `/etc/frr/vtysh.conf`, `frr:frr 0640`, written atomically; applied with
`frr-reload.py --reload` (diff against the running config, no daemon restart); validated with `vtysh -C -f` on a
staged copy; dry-run diff with `frr-reload.py --test`.

## frr.conf layout (section order)

| order | section | content |
|---|---|---|
| 0 | globals | `frr version 10.7.1`, `frr defaults traditional`, `hostname`, `log syslog informational`, `service integrated-vtysh-config` |
| 100 | vrfs | `vrf <name>` + that VRF's static routes + `exit-vrf` |
| 200 | interfaces | `interface <name>` / ` description …` / `exit` |
| 300 | static | default-VRF `ip route` / `ipv6 route` |
| 400–899 | registered protocol sections | P12 `router bgp`, route-maps, prefix-lists; F-* `router ospf`, … |
| — | `end` | |

Sections are separated by `!`; an absent section emits nothing.

## Mapping

| desired state (JSON path) | rendered | validation |
|---|---|---|
| `system.hostname` | `hostname <h>` (omitted when unset) | `[A-Za-z0-9][A-Za-z0-9.-]{0,62}` |
| `vrfs.<name>` (not `default`) | `vrf <name>` … `exit-vrf` | `[A-Za-z0-9_.-]{1,15}` |
| `vrfs.<name>.id`, `.description` | not rendered (VPP tables; kernel VRF ↔ table is P12) | — |
| `interfaces.<vpp name>.description` | `interface <linux name>` / ` description <text>` / `exit` | name: Linux name via the interface mapper (unmapped → skipped); text: printable ASCII, 1–80, no leading `!`/`#`, no leading/trailing/double blanks — written verbatim (FRR takes the rest of the line as one token; nothing reaches a shell) |
| `routing.static[i]` ownership (D-072) | rendered **only** when `frr.StaticOwnedByFRR` (default: the explicit flag, stand-in `routing.static[i].frr: true`); otherwise the agent programs it in VPP and FRR never sees it | — |
| `routing.static[i]` with `vrf` unset/`default` | `ip route` / `ipv6 route` at top level | prefix via `net/netip`, masked (`10.12.201.7/24` → `10.12.201.0/24`) |
| `routing.static[i]` with `vrf: X` | inside `vrf X` block | VRF name as above |
| `routing.static[i].nextHops[j].address` | `… <prefix> <address>` | `net/netip`, no zone, same family as the prefix; not unspecified/multicast/loopback; link-local only with an interface |
| `routing.static[i].nextHops[j].interface` | `… <prefix> [<address>] <linux name>` | mapped + Linux name rule; not IP-shaped; not (an abbreviation of) an `ip route` keyword (`Null0`, `blackhole`, `bl`, `reject`, `tag`, …); unmapped = error |
| `routing.static[i].blackhole` | `… <prefix> blackhole` | `nextHops` must be empty |
| `routing.static[i].tag` *(stand-in, D-055)* | `… tag <T>` (omitted when 0) | integer 0–4294967295 |
| `routing.static[i].distance` | trailing `<D>` (omitted when 1/unset) | 1–255 |
| `routing.static[i].nextHops[j].weight`, `.description` | not rendered (staticd has no equivalent) | — |
| several next hops | one line per next hop (ECMP), sorted | — |

Line format follows FRR's `show running-config` (`ip route P NH [IF] tag T D`) so a re-apply is an empty diff.

Secrets: sections resolve `<kind>/<name>` references through the renderer's `SecretResolver` (D-051/D-072); every
output (Retrieve, DryRun, errors) masks them, frr-reload.py logs at `critical` only.

## vtysh.conf

```
! vtysh.conf — rendered by vrx-agent (RF-1); frr-reload.py requires the integrated config.
service integrated-vtysh-config
```

## State (Retrieve)

`Retrieve()` returns a `structpb.Struct` (no FRR state message in the proto yet — RF-1-questions Q2):

| key | source |
|---|---|
| `version` | `show version` |
| `runningConfig` | `show running-config`, normalised (no banner, comments, blank lines, `frr version`, `end`) and secrets masked (`<redacted>`) — for drift |
| `vrfs` | `show vrf` (text; 10.7 has no JSON variant) → `[{name, active, id, table}]` |
| `staticRoutes` | `show ip route vrf all static json` + `show ipv6 route vrf all static json`, stream-decoded, compact entries |
| `summary` | `show ip[v6] route vrf all summary json` → `"ipv4/<vrf>/<protocol>"` → RIB count |
| `interfaces` | `show interface vrf all json` |
| registered keys | `RegisterStateReader` (P12: `bgpSummary` = `show bgp summary json`) |

## Events (1 Hz polling)

| poller | key → value | proto mapping |
|---|---|---|
| `routes` | `ipv4/<vrf>/<protocol>`, `ipv6/<vrf>/<protocol>` → RIB count (summary commands only) | `EVENT_KIND_UNSPECIFIED`, attributes `source=frr poller key old new` |
| `interfaces` | `<ifname>` → `up`/`down` | `EVENT_KIND_LINK_UP` / `LINK_DOWN` with `interface` |
| registered (`RegisterPoller`) | protocol-defined | `EVENT_KIND_UNSPECIFIED` + attributes |

CLI equivalent: `vtysh -c "show running-config"`, `vtysh -c "show ip route json"`.
