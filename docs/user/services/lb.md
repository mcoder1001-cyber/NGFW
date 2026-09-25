# Load balancer (VPP lb plugin) — tier T3

**Screen:** *Services → Load balancer* (`/services?tab=lb`). **REST:** configuration through the generic routes
(`/api/v1/config/services/lb`, e.g. `PATCH /api/v1/config/services` with `{"lb": …}`), live state
`GET /api/v1/state/lb/vips`, action `POST /api/v1/actions/lb/vips/{name}/flush`. **CLI:** the generic configuration
commands (`docs/user/cli/reference.md`): `vrx show configuration services lb`, `vrx set services lb vips web prefix
198.51.100.10/32`, `vrx merge services lb '<json>'`, then `vrx commit`; the live state and the flush have no curated
CLI command in this release (REST operations `Lb_vips`, `Lb_flush`).

The load balancer spreads **new flows** to a **virtual IP (VIP)** over a set of **application servers (AS)** with
VPP's Maglev-style consistent hashing (the *new-flows table*); established flows stay on their server through a
per-worker sticky table. The packet reaches the server

- **gre4 / gre6** — inside a GRE tunnel (outer source = `settings.ip4Source` / `ip6Source`); the server decapsulates
  and answers the client directly (direct server return);
- **l3dsr** (IPv4 VIPs) — unchanged except for the DSCP (`dscp`), the server recognises the VIP by it and answers
  directly;
- **nat4 / nat6** — destination-NATed to `server:targetPort`; the return traffic is translated back on the interfaces
  listed in `natInterfaces` (VPP's lb NAT in2out feature). `srvType` *clusterip* uses the VIP as SNAT source,
  *nodeport* the global source address.

What this is **not** (WBS D7.9 is tier T3, "solved outside the product by HAProxy/nginx"): no health checks (a server
stays in rotation until you remove it), no L7 (HTTP/TLS) proxying, no weights, no session persistence beyond the
sticky table / `srcIpSticky`. NAT44-ED load-balanced static mappings (`nat.loadBalancedMappings`) and CNAT VIPs are
other features.

## Configuration (`services.lb`)

| member | meaning |
|---|---|
| `settings.ip4Source`, `ip6Source` | outer GRE source / nodeport SNAT source (VPP defaults 255.255.255.255 and ffff:…:ffff — set them for GRE) |
| `settings.flowBuckets`, `flowTimeoutSec` | sticky-table buckets per worker (power of two) and idle flow timeout; unset = keep VPP's value |
| `vips.<name>.prefix` | the VIP (a network prefix, usually /32 or /128; installed in the default table) |
| `protocol`, `port` | `any` (every port, no `port`) or `tcp`/`udp` with a `port` |
| `encap` | `gre4`, `gre6`, `l3dsr`, `nat4`, `nat6` |
| `dscp` | l3dsr only (0–63) |
| `srvType`, `targetPort`, `nodePort` | nat4/nat6 only; `targetPort` required; `nodePort` is accepted but not used by VPP 26.06 |
| `newFlowsTableLength` | Maglev table size, a power of two (default 1024) |
| `srcIpSticky` | hash on the client address only |
| `servers[] {address, flushOnDelete}` | application servers (IPv4 for gre4/l3dsr/nat4, IPv6 for gre6/nat6); `flushOnDelete` also drops the server's sticky entries when it is removed |
| `natInterfaces[] {interface, family}` | the lb NAT in2out feature (needed by nat4/nat6 VIPs) |

Rules checked before commit (400 problem+json with a pointer): the encapsulation matches the VIP family (l3dsr and
nat4 need an IPv4 VIP, nat6 an IPv6 VIP) and the servers' family (**a gre4 VIP with an IPv6 server is refused at
`/services/lb/vips/<name>/servers/<i>/address`**); a port needs tcp/udp; powers of two; unique (prefix, protocol,
port); per prefix either one all-port VIP or per-port VIPs of one encapsulation (VPP's rule); NAT VIPs need a port, a
target port and a `natInterfaces` entry of their family; two NAT VIPs may not share a (server, target port) pair; VIPs
in 0.0.0.0/8 are reserved; NAT interfaces must exist. `settings` is VPP-global: only the *globals owner* agent (the
product agent on a real box) applies it.

### Example: GRE (direct server return)

```json
PATCH /api/v1/config/services
{ "lb": {
    "settings": { "ip4Source": "192.0.2.1" },
    "vips": {
      "web": { "prefix": "198.51.100.10/32", "protocol": "tcp", "port": 443, "encap": "gre4",
               "servers": [ { "address": "10.0.10.11" }, { "address": "10.0.10.12", "flushOnDelete": true } ] } } } }
```

The servers need a GRE tunnel back to `192.0.2.1` that decapsulates into the VIP (the VIP configured on a loopback of
the server) and a default route for the answers.

### Example: L3DSR

```json
{ "lb": { "vips": { "dns": { "prefix": "198.51.100.53/32", "encap": "l3dsr", "dscp": 10,
                             "servers": [ { "address": "10.0.10.21" }, { "address": "10.0.10.22" } ] } } } }
```

Each server maps DSCP 10 to the VIP (for example an iptables/nftables rule that DNATs DSCP-10 packets to the local
VIP address) and answers directly.

### VPP CLI equivalent (what the agent does through the binary API)

```
lb conf ip4-src-address 192.0.2.1
lb vip 198.51.100.10/32 protocol tcp port 443 encap gre4 new_len 1024
lb as 198.51.100.10/32 protocol tcp port 443 10.0.10.11 10.0.10.12
lb vip 198.51.100.53/32 encap l3dsr dscp 10
lb as 198.51.100.53/32 10.0.10.21 10.0.10.22
lb set interface nat4 in <interface>
show lb vips verbose
```

## Live state and flush

The table lists the VIPs of the candidate with their state as VPP holds it (refreshed every 30 s and with *Refresh* —
reading VPP's lb tables is serialised in the agent):

- **active** — in VPP with a server in use; **no server in use** — in VPP without one; **missing in VPP** — the agent
  created it on this VPP instance but VPP does not list it; **not applied** — not created on the running VPP instance
  (not committed yet, refused by the agent, or VPP restarted and the agent has not re-applied it yet);
- **removed copies** — see below; the server sub-table (expand the row) lists every server VPP holds for the VIP,
  *in use* or *removed*.

**Flush** empties the VIP's sticky flow table (`lb_flush_vip`): established flows are re-hashed over the current
servers (use it after removing a server without `flushOnDelete`). It is refused (409) unless the VIP is active.

## Caveats (VPP 26.06, `docs/vpp-code-track.md` V20)

- **Write-only.** VPP cannot report lb objects correctly (`lb_vip_details` loses the protocol and the table length),
  so the agent never reads the configuration back: the configuration view (`Retrieve`) shows no `services.lb` and the
  drift view ignores it; the live state above is a separate view. After a VPP restart the agent re-applies the VIPs on
  its resync.
- **Removed VIPs linger.** Deleting a VIP or a server — and every change of a VIP, which is delete + add — leaves the
  old entry in VPP as *removed* (and each server's `/32` tracking entry in table 0) until VPP's lb garbage collection.
  The API never runs it; the globals-owner agent triggers it about 65 s after the last lb delete (a constant CLI
  command through the binary API, D-090). On shared lab VPPs the test agents never do, so removed entries stay until a
  VPP restart.
- **NAT VIPs and the garbage collection.** VPP keys the NAT mapping of a NAT VIP by (server, target port) only, so a
  changed NAT VIP and its removed predecessor share one mapping; the agent skips the garbage collection while such a
  pair exists (collecting it would break the live VIP and can crash VPP).
- `nodePort` is ignored by VPP 26.06; `show lb vips` reports the NAT target port byte-swapped (the state view swaps it
  back).
