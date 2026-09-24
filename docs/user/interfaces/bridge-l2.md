# Bridging: bridge domains, cross-connects, VLAN tag rewrite, time-range MAC filter

**Screen:** Interfaces → **Bridging** (`/interfaces/bridging`). **REST:** configuration through the generic pointer routes
(`/api/v1/config/routing/l2/…`, `/api/v1/config/interfaces/<if>/l2`); live state `GET /api/v1/state/l2/bridge-domains`
and `GET /api/v1/state/l2/bridge-domains/{id}/macs?page&pageSize`. **CLI:** `vrx set|merge|delete routing l2 …`,
`vrx set|merge interfaces <if> l2 …` (no `show` command for bridge state yet — use the REST route).

The data plane is VPP's L2 switching: bridge domains (`bridge_domain_add_del_v2`) with members, a BVI and split-horizon
groups, L2 cross-connects (`sw_interface_set_l2_xconnect`), L3 cross-connects (the `l3xc` plugin), VLAN tag rewrite on L2
ports (`l2_interface_vlan_tag_rewrite`) and the `mactime` time-range source-MAC filter.

## The model — two halves

| where | what |
|---|---|
| `interfaces.<if>.l2`, `interfaces.<if>.subinterfaces.<id>.l2` | the L2 role of that (sub-)interface: `bridgeDomain` (name), `shg` (split-horizon group 0–255), `bvi`, `uuFwd`, `tagRewrite`, `macFilter` |
| `routing.l2.bridgeDomains.<name>` | a bridge domain: `id` (VPP id 1–16777215 — tunnels attach by this number), `flood`, `uuFlood`, `forward`, `learn` (all default on), `arpTerm` (off), `macAgeMin` (0 = no aging), `staticMacs[]` |
| `routing.l2.xconnects.<rx>` | one direction of an L2 cross-connect: every frame received on `<rx>` goes out of `tx` (a bidirectional cross-connect is two records) |
| `routing.l2.l3xc.<rx>` | an L3 cross-connect: every IPv4 / IPv6 packet received on `<rx>` is forwarded via `ipv4Paths` / `ipv6Paths` (next hop, interface, VRF, weight, preference), bypassing the FIB |
| `routing.l2.macFilters.<name>` | a device of the time-range MAC filter: `mac`, `action` (`allow` / `drop`), weekly `ranges` (`days`, `start`, `end`) |

Rules checked before anything is applied (400 problem+json with the JSON pointer of the offending field):

- an interface is in at most one bridge domain **or** cross-connect — the second membership is reported
  (e.g. `/routing/l2/xconnects/host-w7l0` for a bridge member that is also a cross-connect rx);
- a bridged port (except the BVI) and an L2 cross-connect rx have no addresses, no DHCP client, no unnumbered and stay in
  the `default` VRF;
- one BVI per bridge domain, and the BVI is a loopback (`loop<N>`); one uu-fwd port per bridge domain;
- `shg`, `bvi`, `uuFwd` need a `bridgeDomain`; a VLAN tag rewrite needs an L2 port (a non-BVI member or a cross-connect
  rx), and it cannot pop more tags than the sub-interface matches;
- bridge-domain ids are unique; static MACs are unique per domain and reached through one of its members;
- cross-connect rx ≠ tx, both configured; l3xc paths: at least one, next hop of the list's family, known interface and VRF;
- MAC-filter ranges: `HH:MM`, end after start (24:00 = midnight; split a range that crosses midnight), MACs unique;
  the filter runs on parent (hardware) interfaces only.

## Example: a LAN bridge with a routed BVI and a bridged VLAN

`loop720` is the gateway of the bridged segment; `host-w7l0` carries untagged frames and its VLAN 100 sub-interface joins
the same domain with the tag popped (`pop-1`) and in split-horizon group 1:

```jsonc
// PATCH /api/v1/config  (application/merge-patch+json)
{
  "interfaces": {
    "loop720":   { "enabled": true, "ipv4": ["10.7.20.1/24"], "l2": { "bridgeDomain": "w7-lan", "bvi": true } },
    "host-w7l0": { "enabled": true, "l2": { "bridgeDomain": "w7-lan" },
                   "subinterfaces": { "100": { "vlanId": 100, "enabled": true,
                                               "l2": { "bridgeDomain": "w7-lan", "shg": 1, "tagRewrite": { "op": "pop-1" } } } } }
  },
  "routing": { "l2": { "bridgeDomains": {
    "w7-lan": { "id": 7001, "macAgeMin": 5, "staticMacs": [{ "mac": "02:07:00:00:70:01", "interface": "host-w7l0" }] }
  } } }
}
```

After the commit VPP shows (from the host integration test):

```
$ vppctl show bridge-domain 7001 detail
  BD-ID   Index   BSN  Age(min)  Learning  U-Forwrd   UU-Flood   Flooding  ARP-Term  arp-ufwd ...  BVI-Intf
  7001      1      0      5         on        on       flood        on       off       off   ...  loop720
           Interface           If-idx ISN  SHG  BVI  TxFlood        VLAN-Tag-Rewrite
            loop720              4     1    0    *      *                 none
           host-w7l0             5     1    0    -      *                 none
         host-w7l0.100           7     1    1    -      *                 pop-1
  BD-Tag: w7:7001/w7-lan
```

The bridge-domain tag carries the owner and the record name, so the agent reports the domain under its name again.

## Example: an L2 cross-connect with a tag translation, and an L3 cross-connect

```jsonc
{
  "interfaces": { "host-w7w0": { "enabled": true, "subinterfaces": { "200": { "vlanId": 200, "enabled": true,
                    "l2": { "tagRewrite": { "op": "translate-1-1", "tag1": 300 } } } } },
                  "loop721": { "enabled": true, "ipv4": ["10.7.21.1/24"] } },
  "routing": { "l2": {
    "xconnects": { "host-w7w0": { "tx": "host-w7w0.200" }, "host-w7w0.200": { "tx": "host-w7w0" } },
    "l3xc": { "loop721": { "ipv4Paths": [{ "nextHop": "10.7.21.254", "interface": "loop721" }] } }
  } }
}
```

`vppctl show mode` then shows `l2 xconnect host-w7w0 host-w7w0.200` and back; `vppctl show l3xc` shows the path.

Tag-rewrite operations: `push-1`/`push-2` add tags (`tag1`, `tag2`; `dot1ad: true` pushes an 802.1ad outer tag),
`pop-1`/`pop-2` remove them, `translate-N-M` replace N tags by M. Sub-interfaces stay exact-match.

## Example: time-range MAC filter

```jsonc
{
  "interfaces": { "host-w7l0": { "l2": { "bridgeDomain": "w7-lan", "macFilter": true } } },
  "routing": { "l2": { "macFilters": {
    "kids-tablet": { "mac": "02:07:00:00:99:01", "action": "allow",
                     "ranges": [{ "days": ["mon", "tue", "wed", "thu", "fri"], "start": "16:00", "end": "20:00" }] },
    "old-camera":  { "mac": "02:07:00:00:99:02", "action": "drop" }
  } } }
}
```

On an interface with `macFilter: true`, a device with ranges and `allow` is admitted only inside its ranges; with `drop`
it is blocked only inside them; without ranges the action is permanent. Unknown MACs pass (VPP learns them as `mac-<mac>`
entries; a configured device replaces such an entry). **Clock:** VPP's mactime plugin evaluates ranges in its own clock,
set by the VPP start-up option `mactime { timezone_offset }` (default −5 hours with US daylight saving) — this release does
not render that option. Retrieve reports the ranges grouped per time window, days in `mon … sun` order.

## The same with the CLI

```
vrx merge routing l2 '{"bridgeDomains":{"w7-lan":{"id":7001,"macAgeMin":5}}}'
vrx merge interfaces loop720 l2 '{"bridgeDomain":"w7-lan","bvi":true}'
vrx merge interfaces host-w7l0 subinterfaces 100 l2 '{"bridgeDomain":"w7-lan","shg":1,"tagRewrite":{"op":"pop-1"}}'
vrx merge routing l2 xconnects '{"host-w7w0":{"tx":"host-w7w0.200"},"host-w7w0.200":{"tx":"host-w7w0"}}'
vrx delete interfaces host-w7l0 l2
vrx show configuration diff
vrx commit comment "bridging"
```

## Live state

`GET /api/v1/state/l2/bridge-domains` lists every bridge domain of the running configuration and of VPP: live flags,
members (port role, split-horizon group, tag rewrite), BVI, learned vs static MAC counts, the running record and whether
the candidate changes it. `GET /api/v1/state/l2/bridge-domains/{id}/macs?page=1&pageSize=100` pages through the L2 FIB
(ordered by MAC, at most 1000 per page). Both answer 501 when the agent predates the F-bridge-l2 RPCs.

## Restart safety and rollback

The agent re-creates bridge domains, members, tag rewrites, cross-connects, l3xc and the MAC filter when they disappear
behind its back (host test: back 0.21 s after the agent start). A rollback to a revision without the L2 model returns every
member to L3 and deletes the bridge domain, cross-connects, l3xc and MAC-filter devices of this system.

Not in this release: per-member L2 feature flags, ARP termination tables (F-neighbors-ra), VXLAN/GRE-L2 tunnels as members
beyond referencing any configured interface, EVPN, mactime traffic quotas.
