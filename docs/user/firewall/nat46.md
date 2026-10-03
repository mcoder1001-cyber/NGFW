# NAT46 — IPv4 clients to IPv6-only servers

NAT46 lets IPv4 clients reach a server that has only IPv6. Each **mapping** gives one IPv6 server an IPv4 **service
address**; IPv4 clients connect to that address and the router translates every packet to IPv6 and back (stateless
SIIT, RFC 7915, 1:1). On the IPv6 side a client appears as an address inside the **client prefix**, a /96 that embeds
the client's IPv4 address in its last 32 bits (RFC 6052) — so the server's logs and firewall still see each client.

**UI:** *Firewall → NAT*, tab **NAT46** (after NPTv6). **CLI:** `ngfw merge nat …` (below). **REST:** the generic
`/api/v1/config/nat` routes, plus two read-only routes.

> Stateless 1:1 only. A pool of IPv4 addresses shared by many IPv6 servers (stateful NAT46) is not available: the
> data plane has no such translator (decision D-160).

## Configuration

| field | meaning |
|---|---|
| `clientPrefix` | IPv6 /96 IPv4 clients appear under (default `64:ff9b::/96`, RFC 6052's well-known prefix). Host bits must be zero; route it to the router on the IPv6 side. |
| `interfaces` | interfaces translation runs on — list **both** the IPv4-facing and the IPv6-facing interface |
| `mappings[]` | `name`, `ipv4` (service address), `ipv6` (the server), optional `mtu` of the IPv6 path (1280–65535; IPv4 packets bigger than MTU−20 are fragmented) |

```json
{ "nat": { "nat46": {
  "clientPrefix": "2001:db8:46::/96",
  "interfaces": ["lan0", "wan0"],
  "mappings": [
    { "name": "web", "ipv4": "198.51.100.80", "ipv6": "2001:db8:10::80" },
    { "name": "mail", "ipv4": "198.51.100.25", "ipv6": "2001:db8:10::25", "mtu": 1500 }
  ] } } }
```

With this, an IPv4 client `203.0.113.7` that opens `198.51.100.80:443` reaches `[2001:db8:10::80]:443`, and the server
sees the connection from `2001:db8:46::cb00:7107`.

Commit refuses (400, with a pointer to the field):

- a client prefix that is not a /96 or has host bits set;
- the same IPv4 service address, IPv6 server or mapping name twice; a server inside the client prefix;
- mappings without interfaces, or an interface that does not exist;
- a service address inside a MAP domain's IPv4 prefix (`nat.map`) — both share one translation table;
- a NAT46 interface that `nat.map` uses for MAP-E (encapsulation); MAP-T on the same interface is fine and shared.

Mapping names are 1–54 characters; the router keeps each mapping as a MAP-T domain named `nat46-<name>`, so a
`nat.map` domain may not use the `nat46-` prefix.

## Checking it

- `GET /api/v1/state/nat/nat46` — the running mappings with their data-plane domain names.
- `GET /api/v1/state/nat/nat46/client?ipv4=203.0.113.7` — the IPv6 address a client appears as (also on the tab:
  *Client address*).
- On the router: `vppctl show map domain` lists `nat46-web` with `ip4-pfx 198.51.100.80/32`, `ip6-pfx
  2001:db8:10::80/128`, `ea-bits-len 0`, `ip6-src 2001:db8:46::/96`.

There is no session table: stateless translation keeps no state.

## CLI

```text
ngfw merge nat '{"nat46":{"clientPrefix":"2001:db8:46::/96","interfaces":["lan0","wan0"],"mappings":[{"name":"web","ipv4":"198.51.100.80","ipv6":"2001:db8:10::80"}]}}'
ngfw show configuration diff
ngfw commit comment "nat46 web"
ngfw show configuration nat
```
