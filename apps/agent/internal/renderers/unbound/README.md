# unbound — Unbound resolver renderer (RF-3, WBS D7.3)

Installed: **Unbound 1.24.2**, trust anchor from `dns-root-data` (`/usr/share/dns/root.key`).

| step | how |
|---|---|
| Render | `services.dns.resolvers` → `templates/unbound.conf.tmpl` (`text/template` + strict helpers) → one `unbound.conf` (0640, `root:unbound` in the product). |
| Validate | `unbound-checkconf <staged unbound.conf>` — parses every directive, local-data RR, view and forward block; requires the trust-anchor file to exist. |
| Apply | snapshot → atomic write → `unbound-control -c <conf> reload_keep_cache` over the unix control socket (`control-use-cert: no`). Failure → restore + reload the old file. Not running: idle config → nothing; resolvers present → `*ActionRequired{Unit: unbound, Action: start}`. |
| Retrieve | `unbound-control status`, `stats_noreset`, `list_forwards`, `list_stubs`, `list_local_zones`, `list_local_data` → typed `State` → `structpb.Struct`. |
| Events | poll `stats_noreset` at 1 Hz: running, `total.num.queries`, cache hits/misses, request list. |

## Escaping (00-CONTEXT rule 9)

- Every string reaches the file through a helper: `quoted` (paths, zone names, view names,
  resolver comments), `network` (access-control), `sockaddr` / `fwdaddr` (re-validated
  `ip@port[#tls-name]`), `rr` (local-data, single-quoted), `ident` (view names in
  interface-view). Enums (zone types, ACL actions, record types) come from allow-lists.
- DNS names are validated with the schema's `dnsName` pattern and written absolute/lower-case;
  RR data is rebuilt from typed parts (addresses via `net/netip`, MX/SRV numbers ≤ 65535).
- TXT data (printable ASCII ≤ 1024) is split into ≤ 255-byte character-strings; everything but
  letters, digits, space and inert punctuation is `\DDD`-escaped, and the first letter of any
  `include` is escaped too, so the token `include` never appears in the file from user data
  (`assertNoInjection` checks every golden). A quote, newline or `\ninclude: /etc/passwd`
  anywhere else is rejected (`ErrInvalid`).
- Resolver descriptions appear only in a leading comment, quoted.

## Mapping decisions

- One Unbound instance serves every enabled resolver. One resolver: local zones are global.
  Several resolvers: each resolver's local zones go into `view: <resolver>` (`view-first: yes`)
  bound to its listen addresses with `interface-view`; forward zones, access control and
  instance settings (threads, cache, DNSSEC, qname minimisation, hide-*, log queries) are
  global and must agree (else `ErrInvalid`); all resolvers share one VRF.
- A resolver's default `forwarders` become `forward-zone "."`; `forward-tls-upstream` is per
  zone, so the forwarders of a zone must agree on TLS; `tlsServerName` → `ip@port#name`, and
  `tls-cert-bundle` is rendered when any zone uses TLS.
- DNSSEC: auto → `auto-trust-anchor-file` (writable copy, product `/var/lib/unbound/root.key`),
  static → `trust-anchor-file /usr/share/dns/root.key`, off → `module-config: "iterator"`.
- `stub-zone:` is supported by Unbound but the schema has no stub zones (questions Q4); Retrieve
  still reports `list_stubs`.
- `Paths.LoopbackOnly` (tests) refuses any listen address that is not loopback.

## Tests

Goldens (`testdata/*.golden`, `-update`), hostile strings in every user field, argv, Apply with
a recording runner. Integration (`VRX_INTEGRATION=1`): `unbound -d -c <cfg>` on
127.0.0.1:3<slot>53 under /run/vrx-test/<prefix>/unbound; `list_forwards`, `net.Resolver`
lookups (A and a hostile TXT that must round-trip verbatim), change + reload + rollback.
