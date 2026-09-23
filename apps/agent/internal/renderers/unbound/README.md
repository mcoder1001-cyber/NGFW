# unbound — Unbound resolver renderer (RF-3, WBS D7.3)

Installed: **Unbound 1.24.2**, trust anchor from `dns-root-data` (`/usr/share/dns/root.key`).

| step | how |
|---|---|
| Render | `services.dns.resolvers` → `templates/unbound.conf.tmpl` (`text/template` + strict helpers) → one `unbound.conf` (0640, `root:unbound` in the product). |
| Validate | `unbound-checkconf <staged unbound.conf>` — parses every directive, local-data RR, view and forward block; requires the trust-anchor file to exist. |
| Apply | snapshot → atomic write → not running: idle → nothing, resolvers → `*ActionRequired{start}`; a **startup-only directive changed** (`interface`, `port`, `interface-view`, `username`, `chroot`, `directory`, `pidfile`, `do-daemonize`, `control-*`) → `*ActionRequired{restart}` persisted in `Paths.PendingFile` (reload does not reopen sockets — review H1, verified live); a pending restart whose request is newer than unbound's process → the same request again (M2); otherwise `unbound-control reload_keep_cache` + **convergence check** (rendered forward zones in `list_forwards`, global local zones with their type in `list_local_zones`, every `interface` accepts TCP). Reload or check failure → restore + reload the old file (on a detached context). |
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

## Paths (review M4, L5)

Product: `/etc/unbound/unbound.conf`, control socket `/run/unbound.ctl` and pidfile
`/run/unbound.pid` directly in `/run` (the packaged `unbound.service` has no `RuntimeDirectory`, so
`/run/unbound` does not exist; `Apply` also creates missing parent directories),
`/var/lib/unbound/root.key`, pending requests `/run/vrx/renderers/unbound.pending` (cleared by a
reboot, which restarts unbound anyway). `TestProductPaths` pins them and the integration test runs
`unbound-checkconf` on the staged product render. An idle instance binds `127.0.0.1@IdlePort`
(product 53, tests 3<slot>53 — never :53 on the shared host).

`unbound-control` output at the runner's capture limit is an error (`ErrOutputTruncated`);
`State` reports `localDataTruncated` instead of a silently partial `list_local_data` (review L1).
