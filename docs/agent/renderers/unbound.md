# Unbound renderer — desired state ↔ rendered config

Package `apps/agent/internal/renderers/unbound` (RF-3). Unbound 1.24.2. File: `/etc/unbound/unbound.conf`
(`text/template` + strict escaping). Details and decisions: the package README.

| `services.dns.resolvers.<name>` | `unbound.conf` |
|---|---|
| (enabled resolvers) | one instance; `# resolver "<name>": "<description>"` comment lines |
| `vrf` | must be equal for all resolvers |
| `listen[] {address, port}` | `interface: <ip>@<port>`; `port:` = lowest listen port |
| `accessControl[] {prefix, action}` | `access-control: <network> <action>` |
| `forwarders[]` | `forward-zone: name: "."` + `forward-addr: <ip>@<port>[#<tlsServerName>]`, `forward-tls-upstream` |
| `forwardZones[] {zone, forwarders, forwardFirst}` | `forward-zone:` `name`, `forward-addr`, `forward-first`, `forward-tls-upstream` |
| `localZones[] {zone, type}` | `local-zone: "<zone>." <type>` (global with one resolver; inside `view: <resolver>` + `interface-view` with several) |
| `localZones[].records[] {name, type, ttlSec, data}` | `local-data: '<name>. <ttl> IN <type> <data>'` (TXT `\DDD`-escaped) |
| `dnssec.enabled` / `trustAnchorAuto` | `auto-trust-anchor-file` / `trust-anchor-file` / `module-config: "iterator"`; `harden-dnssec-stripped` |
| `cache {minTtlSec, maxTtlSec, prefetch, msgCacheMb, rrsetCacheMb}` | `cache-min-ttl`, `cache-max-ttl`, `prefetch`, `msg-cache-size`, `rrset-cache-size` |
| `threads` | `num-threads` |
| `qnameMinimisation`, `hideIdentity`, `hideVersion`, `logQueries` | same-named directives |
| — (agent) | `directory`, `chroot: ""`, `username`, `pidfile`, `do-daemonize: no`, syslog or `logfile`, `harden-glue`, `harden-below-nxdomain`, `remote-control:` unix `control-interface`, `control-use-cert: no`, `tls-cert-bundle` when TLS is used |
| `vppCache` | not rendered (VPP dns plugin, DF-8) |

Apply: `unbound-control reload_keep_cache`. Retrieve: `status`, `stats_noreset`, `list_forwards`,
`list_stubs`, `list_local_zones`, `list_local_data`. CLI equivalent: none yet.

## In the agent (F-unbound-chrony-syslog)

Singleton descriptor `unbound.config/vrx` (`renderers/unbound/descriptor.go`, D-109 d), domain `services`:

| | |
|---|---|
| Value | `unbound.Input(services.dns)` — the resolvers (enabled or not); no object without a resolver |
| Create / Update | Render → Validate (`unbound-checkconf`) → Apply; `*ActionRequired{start\|restart}` is logged, not a failure |
| Delete | the idle rendering (loopback only, no resolver) |
| Retrieve | `unbound.conf` → embedded input (`# vrx-input: <base64 of the deterministic protobuf>`) → re-rendered → byte-equal: the Value; else a drift `*structpb.Struct` (re-applied); a file without the line (Debian's default) is not ours and never reported |
| Pending (DnsState) | a persisted restart request (D-079) until unbound's process started after it; `start` while not running with resolvers |
| Ownership | `RecordsNoOwnership()` (TD-11b): the embedded input is the ownership record |

Paths: the globals owner (the product agent) uses `ProductPaths()`; every other agent `PathsUnder(/run/vrx-test/<owner>/unbound,
3<slot>53)` (loopback only, `VRX_HOST_SERVICES_DIR` overrides the base), prepared on the first write — registration touches no file.
