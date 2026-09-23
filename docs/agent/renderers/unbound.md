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
