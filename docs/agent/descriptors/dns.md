# dns plugin descriptors (DF-8, WBS D7.3)

Package `apps/agent/internal/descriptors/dns` — VPP's caching DNS resolver/proxy: the enable switch and the upstream
name servers, plus resolve action helpers. Message names only from `apps/agent/binapi/dns`.
`dns.Register(registry, client)` registers both (name servers first).

| Object type | Key | VPP messages | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `dns.name-server` | `dns.name-server/<ip>` | `dns_name_server_add_del` is_add=1 / 0 | **write-only** (`ErrRetrieveUnsupported`) | re-apply (address is the key) | — |
| `dns.enable` (singleton) | `dns.enable/global` | `dns_enable_disable` enable=1 / 0 | **write-only** | in place | — (see ordering) |

Actions (not desired state): `ResolveName(ctx, client, name)` (`dns_resolve_name`; name validated to DNS characters,
never a shell), `ResolveIP(ctx, client, addr)` (`dns_resolve_ip`).

## Notes and limitations
- **No dump, no getter** in `dns.api` (enable_disable, name_server_add_del, resolve_name, resolve_ip only): both
  descriptors are write-only (D-063). Create is idempotent (VPP ignores a duplicate server; enable twice is fine);
  `NAME_SERVER_NOT_FOUND` on delete = already gone.
- **Ordering is reversed vs. the DF-8 prompt:** VPP refuses `dns_enable_disable(1)` with `NO_NAME_SERVERS` while no
  server is configured. `dns.enable` therefore has no dependency and `Register` puts `dns.name-server` first (the
  scheduler breaks ties by registration order); deletes run in reverse (disable first).
- `dns_resolve_name` replies only after the upstream answers or VPP's retries give up: pass a ctx with a deadline
  (a host run without one blocked for minutes against unreachable upstreams).
- VPP CLI bug seen in the evidence: `show dns servers` prints the IPv6 list from the IPv4 vector (`fd00:5::53` is shown
  as `a05:3501::`, the bytes of `10.5.53.1`); the API state is right.
- Ownership: servers have no tag; production owns the resolver. The singleton is VPP-global; nobody else on the
  shared host uses VPP's resolver (Unbound is RF-3's), the host test enables and disables it.
