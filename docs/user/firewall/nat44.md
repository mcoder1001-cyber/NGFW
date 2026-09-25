# NAT44 (endpoint-dependent): outbound PAT, 1:1, port forwards and the session browser

VRX translates IPv4 with VPP's **NAT44-ED** plugin (`nat44-ed`, endpoint-dependent: every session is keyed by the full
5-tuple, so one outside address and port can serve many destinations). The configuration lives under `nat` in the
configuration document and goes through the usual candidate → diff → commit → rollback cycle; live sessions, pool
usage and the session kill are separate state and action routes. **UI:** *Firewall → NAT* (tabs Outbound, Static &
port forwards, Pools, Sessions). **CLI:** `vrx set nat …` / `vrx merge nat …` (see below).

This page covers `mode: "ed"`. NAT44-EI, NAT64, NAT66, NPTv6, DET44/CGNAT, DS-Lite, MAP and CNAT have their own pages;
until those features ship, the agent reports their subtrees as *not applied* (`agent.unsupported-field` warnings in
validation) and programs nothing for them.

## Concepts

| field | what it does in VPP |
|---|---|
| `inside`, `outside` | NAT44-ED feature on the interface (`nat44_interface_add_del_feature` inside / outside). An interface is on one side only. |
| `outputFeature` | NAT on the output path (post-routing) of the interface. |
| `pools[]` `{name, range, vrf?, twiceNat}` | Addresses translated sources are taken from (`nat44_add_del_address_range`, ≤ 1024 addresses per range). A pool without `vrf` belongs to the default VRF (table 0). |
| `pools[]` `{name, interface, twiceNat}` | Use the address of an interface, following it when it changes (DHCP WAN). |
| `staticMappings[]` | **1:1** when no ports are given (the whole address, every protocol); a **port forward** with `protocol` + `local.port` + `external.port`. `external` is exactly one of `ip`, `pool` (a range pool stands for its first address, an interface pool for the interface address) or `interface`. |
| `identityMappings[]` | An address (or interface address), optionally one protocol/port, that is never translated. |
| `loadBalancedMappings[]` | One external `ip:port` spread over weighted local endpoints. |
| `forwarding` | Forward traffic that matches no session and no mapping instead of dropping it. |
| `sessionLimit`, `insideVrf`, `outsideVrf`, `timeouts` | Plugin-wide VPP settings (see "Plugin-wide settings"). |
| `enabled` | Optional. Omitted = on as soon as any interface, pool or mapping is configured; `false` keeps the configuration but programs nothing. |

Traffic that matches no pool and no mapping is **dropped** by VPP unless `forwarding` is on (VPP's default; VRX adds no
policy of its own).

Rules checked before anything is applied (a violation is a `400 application/problem+json` with a JSON `pointer`):
inside/outside disjoint, interfaces exist, pool names unique, ranges ordered and **not overlapping** within one twice-NAT
class, and **not touching** either (`10.4.2.100-10.4.2.104` and `10.4.2.105-…` in one class and VRF must be written as
one range — VPP stores pool addresses one by one and reports them back merged, so two touching pools could never match
what is applied); a port on a mapping needs `protocol`; `staticMappingOnly` and `connectionTracking` are accepted but not
applied (VPP 26.06 answers *unsupported*).

## Scenario 1 — outbound PAT (many inside hosts behind a few public addresses)

```json
{
  "nat": {
    "inside": ["host-w4l0"],
    "outside": ["host-w4w0"],
    "pools": [{ "name": "pat", "range": "10.4.2.100-10.4.2.103" }]
  }
}
```

A host behind `host-w4l0` (10.4.1.2) that opens `10.4.2.2:8000` appears on the outside as one of `10.4.2.100–103`,
with a port VPP picked. VPP answers ARP for the pool addresses on the outside interface by itself (no proxy ARP needed).
Addresses of a pool inside the outside interface's subnet are preferred for destinations in that subnet.

## Scenario 2 — 1:1 (a whole address, both directions)

```json
{ "nat": { "staticMappings": [
  { "name": "one2one", "local": { "ip": "10.4.1.3" }, "external": { "ip": "10.4.2.111" } }
] } }
```

`10.4.1.3` leaves as `10.4.2.111` (ports kept), and anything sent to `10.4.2.111` reaches `10.4.1.3`.

## Scenario 3 — port forward

```json
{ "nat": { "staticMappings": [
  { "name": "web", "protocol": "tcp",
    "local": { "ip": "10.4.1.2", "port": 80 }, "external": { "ip": "10.4.2.110", "port": 8080 } }
] } }
```

A connection to `10.4.2.110:8080` from the outside reaches `10.4.1.2:80` (VPP's `nat44-ed-out2in` node translates it).
Use `"external": {"interface": "host-w4w0", "port": 8080}` to forward on the address of a DHCP-configured WAN.

## Session browser

![NAT sessions, 2 101 live sessions, server-side paging](../../status/tasks/F-nat44-ed-sessions-screens/nat-sessions-en.png)

*Firewall → NAT → Sessions* lists the live translations of this system, paged on the server (at most 256 rows per
page), filtered by inside / outside / external address, port, protocol and inside VRF, with **Kill** on each row (asks for
confirmation; TCP, UDP and ICMP sessions). The unfiltered grid refreshes every 30 s; with an address, port or protocol
filter it refreshes only when you press **Refresh**. The Pools tab shows per pool the number of sessions and a
utilisation bar (sessions ÷ (addresses × 64 512 ports) — an estimate: an endpoint-dependent session reuses a port for
different destinations); the NAT summary behind it is refreshed every 30 s.

![Pools with the utilisation bar](../../status/tasks/F-nat44-ed-sessions-screens/nat-pools-en.png)

```sh
# one page (page, pageSize ≤ 256) and filters
curl -s -H "Authorization: Bearer $TOKEN" 'https://vrx/api/v1/state/nat/sessions?page=1&pageSize=100&protocol=tcp&inside=10.4.1.2'
# {"page":1,"pageSize":100,"total":1,"totalUsers":1,"truncated":false,"items":[{"insideAddress":"10.4.1.2","insidePort":40001,
#   "outsideAddress":"10.4.2.101","outsidePort":1024,"externalAddress":"10.4.2.2","externalPort":8000,"protocol":"tcp","vrf":"default",…}]}

# totals and per-pool usage (joined with the pool names of the running configuration)
curl -s -H "Authorization: Bearer $TOKEN" https://vrx/api/v1/state/nat/summary

# kill one session: its 5-tuple and the inside VRF (default "default"); 404 when it is already gone.
# externalAddress/Port = the remote end as the inside host addresses it: for a twice-NAT session its
# externalNatAddress/Port (the UI does this for you)
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"protocol":"tcp","insideAddress":"10.4.1.2","insidePort":40001,"externalAddress":"10.4.2.2","externalPort":8000}' \
  https://vrx/api/v1/actions/nat/sessions/kill
```

Reading sessions needs the `readonly` role, killing one `operator`; every kill is in the audit log (resource
`nat/sessions/<protocol>/<inside>:<port>/<external>:<port>/<vrf>`).

**Cost on the data plane.** VPP lists sessions only per inside host, and every such listing walks its whole session
table while packet processing waits. So the agent bounds each request: one page visits at most 256 inside hosts, a
filter on the outside or external address, a port or the protocol looks through at most 256 hosts and 200 000 sessions
(`truncated: true` — the total is then a lower bound; narrow the filter, e.g. by inside address), and the summary's
per-pool / per-protocol breakdown visits at most 64 hosts (the totals are always complete). The agent computes the
summary at most once every 30 s, however many browsers are open (`retrievedAt` tells how old it is), and it runs one
such walk at a time: concurrent requests wait for each other instead of stalling VPP together.

**Overlapping inside addresses in several VRFs.** VPP's per-host session listing matches the inside address only, not
the VRF. When the same inside address is NATed in two VRFs, the browser shows each of those sessions once, but it may
show one under the wrong VRF (and a kill of such a row then answers 404). Filter by the inside address to see them all.

## Plugin-wide settings and restarts

`sessionLimit`, `insideVrf`, `outsideVrf`, `timeouts` and `forwarding` are VPP-wide. The system's own agent sets them;
changing `sessionLimit` or the VRFs needs the plugin empty (VPP re-enables it), so remove the NAT objects first.

**Sessions are not configuration.** They are lost when VPP restarts, when the pool address they use is removed, and
when the agent recreates NAT objects after a data-plane loss; clients reconnect and get new sessions. The configuration
itself comes back on its own: after a VPP or agent restart the agent re-applies the committed NAT configuration within
seconds.

## The same with REST

```sh
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'content-type: application/merge-patch+json' \
  -d '{"inside":["host-w4l0"],"outside":["host-w4w0"],"pools":[{"name":"pat","range":"10.4.2.100-10.4.2.103"}]}' \
  https://vrx/api/v1/config/nat
curl -s -H "Authorization: Bearer $TOKEN" https://vrx/api/v1/config/diff
curl -s -X POST -H "Authorization: Bearer $TOKEN" 'https://vrx/api/v1/config/commit?confirm=120&comment=nat'
curl -s -X POST -H "Authorization: Bearer $TOKEN" https://vrx/api/v1/config/commit/confirm
```

Lists (`pools`, `staticMappings`, …) are replaced as a whole by a merge patch; send the complete list.

## The same with the CLI

```text
vrx set nat inside host-w4l0
vrx set nat outside host-w4w0
vrx merge nat '{"pools":[{"name":"pat","range":"10.4.2.100-10.4.2.103"}]}'
vrx merge nat '{"staticMappings":[{"name":"web","protocol":"tcp","local":{"ip":"10.4.1.2","port":80},"external":{"ip":"10.4.2.110","port":8080}}]}'
vrx show configuration diff
vrx commit confirm 120 comment "nat"
vrx confirm
vrx show configuration nat
```

The CLI has no session command yet (`show nat sessions` is not in the command registry); use the REST calls above or
the UI.

## What happens on the data plane

| configuration | VPP (check with) |
|---|---|
| `inside` / `outside` | `vppctl show nat44 interfaces` → `host-w4l0 in`, `host-w4w0 out` |
| `pools` | `vppctl show nat44 addresses` → each address with its tenant VRF |
| `staticMappings` | `vppctl show nat44 static mappings` → `TCP local 10.4.1.2:80 external 10.4.2.110:8080 vrf 0`, `local 10.4.1.3 external 10.4.2.111 vrf 0` |
| sessions | `vppctl show nat44 sessions filter i2o saddr 10.4.1.2` |

A rollback removes the interface features, pools and mappings of the rolled-back revision; the plugin itself stays
enabled.
