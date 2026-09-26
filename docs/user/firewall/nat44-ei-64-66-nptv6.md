# NAT44-EI, NAT64, NAT66 and NPTv6

Next to NAT44-ED ([nat44.md](nat44.md)), VRX drives four more VPP translators, all under `nat` in the configuration
document and through the usual candidate → diff → commit → rollback cycle. **UI:** *Firewall → NAT*, tabs **NAT44-EI**,
**NAT64**, **NAT66**, **NPTv6** (after the NAT44-ED tabs). **CLI:** `vrx set nat …` / `vrx merge nat …` (below).

| translator | configuration | VPP plugin | what it is for |
|---|---|---|---|
| NAT44-EI | `nat` with `mode: "ei"` | `nat44_ei` | IPv4 NAT with endpoint-independent mapping (full cone) |
| NAT64 | `nat.nat64` | `nat64` | IPv6-only clients reaching IPv4 servers (stateful, RFC 6146); the PLAT of 464XLAT |
| NAT66 | `nat.nat66` | `nat66` | stateless 1:1 IPv6 address translation |
| NPTv6 | `nat.nptv6.bindings[]` | `npt66` | stateless IPv6 prefix translation (RFC 6296) |

## NAT44-EI vs NAT44-ED

`mode` chooses the NAT44 flavour; the NAT44 fields (`inside`, `outside`, `outputFeature`, `pools`, `staticMappings`,
`identityMappings`, `forwarding`, `timeouts`, `insideVrf`, `outsideVrf`) are the same for both and are edited on the
Outbound, Static & port forwards and Pools tabs whatever the mode.

| | ED (`mode: "ed"`, default) | EI (`mode: "ei"`) |
|---|---|---|
| session key | full 5-tuple: one outside address:port serves many destinations | inside address:port: one outside address:port per inside endpoint, whatever the destination |
| reachability | only the destination a host talked to answers | any remote host can reach a mapped inside endpoint (full cone, friendlier to peer-to-peer and some games/VoIP) |
| twice-NAT, self twice-NAT, out-to-in-only, load-balanced mappings | yes | **no** — rejected with `400` and a pointer (rule `nat.mode-ed-features`) |
| port forward external address, identity mapping address with a port | any address | **a pool address** (nat44-ei reserves the port on a pool address; rule `nat.ei-port-forward-pool`), unless `staticMappingOnly` or an interface pool |
| `sessionLimit` | applied | not applied (nat44-ei takes it from startup.conf; a warning) |
| `staticMappingOnly`, `connectionTracking` | not supported by VPP (warning) | applied |

ED and EI are **mutually exclusive** on one VPP: switching `mode` removes the ED objects and enables EI in one commit
(the EI tab has a *Switch to EI* button that writes only `mode`). The NAT44-EI tab shows the EI session browser
(server-side paging, the same filters as ED) with a per-row kill; an EI session is found by its inside endpoint, so the
kill needs only protocol, inside address/port and VRF.

```json
{ "nat": {
  "mode": "ei",
  "inside": ["host-w4l0"], "outside": ["host-w4w0"],
  "pools": [{ "name": "pat", "range": "10.4.2.100-10.4.2.103" }],
  "staticMappings": [{ "name": "web", "protocol": "tcp",
    "local": { "ip": "10.4.1.2", "port": 80 }, "external": { "ip": "10.4.2.103", "port": 8080 } }]
} }
```

## NAT64 (and the PLAT side of 464XLAT)

```json
{ "nat": { "nat64": {
  "enabled": true,
  "inside": ["lan0"], "outside": ["wan0"],
  "prefixes": [{ "prefix": "64:ff9b::/96" }],
  "pools": [{ "range": "198.51.100.1-198.51.100.14" }],
  "staticBibs": [{ "protocol": "tcp", "inside": { "ip": "2001:db8:1::10", "port": 443 },
                   "outside": { "ip": "198.51.100.1", "port": 443 } }]
} } }
```

An IPv6-only client reaches the IPv4 host `203.0.113.5` at `64:ff9b::cb00:7105` (the IPv4 address in the last 32 bits of
the /96); VPP translates the packet to IPv4 from a pool address and back. Prefix lengths are the RFC 6052 ones
(32/40/48/56/64/96, host bits zero); one prefix per VRF. `64:ff9b::/96` is the well-known prefix; use a network-specific
prefix from your own space if the clients must reach private IPv4 addresses (RFC 6052 §3.1).

**DNS64.** Clients learn the synthesized addresses from a DNS64 resolver (RFC 6147) that answers AAAA queries for
IPv4-only names with the NAT64 prefix + the A record. Configure the resolver the clients use (for example Unbound's
`dns64` module) with the same prefix; NAT64 itself does not touch DNS, and DNS64 in VRX's own resolver is a separate
feature of the DNS service.

**464XLAT.** NAT64 is the provider-side translator (PLAT) of 464XLAT (RFC 6877): a customer CLAT translates the host's
IPv4 to IPv6 towards the NAT64 prefix, and this NAT64 translates it back to IPv4. Nothing extra is configured on VRX; the
CLAT side on a VRX is a MAP-T domain (*NAT → MAP*).

**VRFs.** VPP's NAT64 is multi-tenant on the inside only: the prefix is chosen by the inside interface's IPv6 VRF, the
outside is always the default VRF, and the translated IPv4 packet is routed in the inside interface's IPv4 VRF (add a
route there towards the outside when the inside is in a tenant VRF).

**A NAT64 tenant VRF cannot be deleted until VPP restarts.** VPP 26.06 never releases the IPv6 table locks that a NAT64
prefix or static BIB entry takes on a non-default VRF, even after you remove them or disable NAT64 (pools are not
affected). Until VPP restarts:
- a commit that deletes that VRF fails its verification and is rolled back as a whole: none of its other changes are
  applied;
- a rollback to a revision without that VRF fails the same way;
- a confirmed commit that added such a VRF cannot be reverted automatically when the confirmation times out: the revert
  fails, the router stays on the new configuration and reports DEGRADED, and it retries on every resync.

Commit preview (dry run) warns at the prefix's or static BIB entry's `vrf` (rule `nat.nat64-tenant-vrf`: "this VRF cannot
be deleted until VPP restarts"). Keep NAT64 in the default VRF unless you need tenants, and plan a VPP restart before
removing a tenant VRF that has carried NAT64.

The NAT64 tab shows the live session table (IPv6 client, IPv4 pool endpoint, IPv4 remote and its IPv6 form), paged on
the server and refreshed every 30 s or with *Refresh*. VPP has no NAT64 session delete, so there is no kill.

## NAT66

```json
{ "nat": { "nat66": { "enabled": true, "inside": ["lan0"], "outside": ["wan0"],
  "staticMappings": [{ "local": "fd00:1::66", "external": "2001:db8:66::66" }] } } }
```

Each static mapping translates one IPv6 address to another in both directions; an interface is on one side only. The
NAT66 tab shows the mappings with their live status (whether what the agent reads back from VPP matches the running
configuration).

## NPTv6 (RFC 6296)

```json
{ "nat": { "nptv6": { "bindings": [
  { "description": "site A", "interface": "wan0", "internal": "fd00:4:10::/48", "external": "2001:db8:20::/48" }
] } } }
```

The binding sits on the **outside** interface: packets leaving it from the internal prefix leave with the external prefix,
packets arriving for the external prefix are delivered to the internal one. The translation is stateless and
checksum-neutral: one 16-bit word (the subnet word for prefixes up to /48) is adjusted, so `fd00:4:10::2` appears as, for
example, `2001:db8:20:ffef::2`. Rules: both prefixes have the same length, at most /64, no host bits; one binding per
interface.

VPP cannot list NPTv6 bindings (no dump in VPP 26.06): the agent applies every binding of the running configuration and
re-applies it on every resync, but cannot read it back. The NPTv6 tab therefore shows the running bindings marked
*configured (not readable)*; check the data plane with `vppctl show npt66 bindings`. A binding removed while the agent was
stopped stays in VPP until VPP restarts.

## Plugin-wide settings, restarts

The NAT44-EI enable (VRFs, flags), timeouts and forwarding, and the NAT64 / NAT66 enables are VPP-wide; the system's own
agent sets them. Sessions are state: they are lost when VPP restarts or when the agent recreates NAT objects after a
data-plane loss. The configuration comes back on its own within seconds after an agent or VPP restart.

## REST

| route | what |
|---|---|
| `PATCH /api/v1/config/nat` (merge patch), `GET /api/v1/config/diff`, `POST /api/v1/config/commit` | configuration (all four) |
| `GET /api/v1/state/nat/ei/sessions?page&pageSize&inside&outside&external&port&protocol&vrf` | NAT44-EI sessions (pageSize ≤ 1000) |
| `POST /api/v1/actions/nat/ei/sessions/kill` `{protocol, insideAddress, insidePort, vrf?}` | delete one EI session (operator, audited; 404 when gone) |
| `GET /api/v1/state/nat/nat64/sessions?page&pageSize&protocol` | NAT64 sessions |
| `GET /api/v1/state/nat/nptv6` | running NPTv6 bindings with the write-only marker |

```sh
curl -s -X PATCH -H "Authorization: Bearer $TOKEN" -H 'content-type: application/merge-patch+json' \
  -d '{"nptv6":{"bindings":[{"interface":"wan0","internal":"fd00:4:10::/48","external":"2001:db8:20::/48"}]}}' \
  https://vrx/api/v1/config/nat
curl -s -X POST -H "Authorization: Bearer $TOKEN" 'https://vrx/api/v1/config/commit?comment=nptv6'
curl -s -H "Authorization: Bearer $TOKEN" 'https://vrx/api/v1/state/nat/nat64/sessions?pageSize=100'
curl -s -X POST -H "Authorization: Bearer $TOKEN" -H 'content-type: application/json' \
  -d '{"protocol":"tcp","insideAddress":"10.4.1.2","insidePort":40001}' https://vrx/api/v1/actions/nat/ei/sessions/kill
```

## CLI

```text
vrx set nat mode ei
vrx merge nat '{"nat64":{"enabled":true,"inside":["lan0"],"outside":["wan0"],"prefixes":[{"prefix":"64:ff9b::/96"}],"pools":[{"range":"198.51.100.1-198.51.100.14"}]}}'
vrx merge nat '{"nat66":{"enabled":true,"inside":["lan0"],"outside":["wan0"],"staticMappings":[{"local":"fd00:1::66","external":"2001:db8:66::66"}]}}'
vrx merge nat '{"nptv6":{"bindings":[{"interface":"wan0","internal":"fd00:4:10::/48","external":"2001:db8:20::/48"}]}}'
vrx show configuration diff
vrx commit comment "nat64 nat66 nptv6"
vrx show configuration nat
```

The CLI has no session commands yet; use the REST routes above or the UI.

## What happens on the data plane

| configuration | VPP (check with) |
|---|---|
| EI `inside` / `outside` / `pools` / `staticMappings` | `vppctl show nat44 ei interfaces`, `show nat44 ei addresses`, `show nat44 ei static mappings`; sessions `show nat44 ei sessions detail` |
| NAT64 | `vppctl show nat64 interfaces`, `show nat64 prefix`, `show nat64 pool`, `show nat64 bib all`, `show nat64 session table all` |
| NAT66 | `vppctl show nat66 interfaces`, `show nat66 static mappings` |
| NPTv6 | `vppctl show npt66 bindings` |

A rollback removes the interfaces, pools, prefixes, mappings and bindings of the rolled-back revision; the plugins
themselves stay enabled.
