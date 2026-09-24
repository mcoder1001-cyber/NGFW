# Kea renderer — desired state ↔ rendered config

Package `apps/agent/internal/renderers/kea` (RF-3). Kea 3.0.3. Files: `/etc/kea/kea-dhcp4.conf`,
`/etc/kea/kea-dhcp6.conf` (JSON, `encoding/json`); no kea-ctrl-agent (D-079). Details and
decisions: the package README.

| `services.dhcp` (document) | Kea (`Dhcp4` / `Dhcp6`) |
|---|---|
| `servers.<name>` enabled, `family: ipv4` / `ipv6` | merged into the one kea-dhcp4 / kea-dhcp6 config of its family; disabled servers are omitted |
| `servers.<name>.vrf` | must be equal for all servers of a family (one process, one namespace) |
| `servers.<name>.interfaces[]` | `interfaces-config.interfaces` (Linux names via the interface mapper; `if/addr` when the desired interface has an address in a subnet); a server with one interface also sets `subnetN[].interface` |
| `servers.<name>.leaseTimeSec` (default 3600) | `subnetN[].valid-lifetime` of its subnets (v6: also `preferred-lifetime`) |
| `servers.<name>.renewTimerSec` / `rebindTimerSec` | `subnetN[].renew-timer` / `rebind-timer` |
| `servers.<name>.authoritative` | `subnet4[].authoritative` |
| `servers.<name>.options[]` | appended to each subnet's `option-data` unless the subnet sets the code |
| `servers.<name>.description` | `subnetN[].user-context.vrx.server-description` (ASCII-escaped) |
| `servers.<name>.subnets.<sub>` | `subnetN[]` with `id` = the id Kea already runs for `server/sub` (read back from the applied file), else FNV-32a(`server/sub`) salted `#1`, `#2`… on collision; `user-context.vrx.{server,subnet,description}` |
| `…subnet`, `pools[]` | `subnet` (masked), `pools[].pool` = `first-last` |
| `…gateway` (v4) | option `routers` (3) |
| `…dnsServers[]` | v4 `domain-name-servers` (6), v6 `dns-servers` (23) |
| `…ntpServers[]` | v4 `ntp-servers` (42), v6 `sntp-servers` (31) |
| `…domainName` | v4 `domain-name` (15); v6: first entry of `domain-search` |
| `…domainSearch[]` | v4 `domain-search` (119), v6 `domain-search` (24) |
| `…leaseTimeSec` | `valid-lifetime` of that subnet |
| `…options[]` `{code, data, alwaysSend}` | `option-data {code, space, data, always-send}`; unknown code + `0x…` → `csv-format: false`; unknown code + text → `option-def {vrx-<code>, string}` |
| `…reservations.<r>` `{mac\|duid, ip, hostname, options}` | `reservations[] {hw-address\|duid, ip-address (v4) / ip-addresses (v6), hostname, option-data, user-context.vrx.reservation}` |
| — (agent) | `control-sockets` unix `<run>/kea4.sock` / `kea6.sock`; `lease-database` memfile `<lib>/leases4.csv` + `lfc-interval`; `hooks-libraries` `libdhcp_lease_cmds.so` (found under the hooks dir); `loggers` to `<log>/kea-dhcpN.log` |
| `servers.<name>` relay, client classes, shared networks | not rendered (relay is VPP, DF-8) |

Apply: `config-set` per running server (no restart). Retrieve: `status-get`, `config-get`,
`statistic-get-all`, `lease4/6-get-page`.

## In the agent (F-kea-dhcp-relay)

No renderer stage exists in the agent core, so each daemon is **one singleton scheduler descriptor** (D-109 d),
`kea.dhcp4/vrx` and `kea.dhcp6/vrx` in `Domains["services"]` (`renderers/kea/descriptor.go`):

| | |
|---|---|
| Value | `kea.Input(document, family)`: a `vrx.v1.DesiredState` with that family's servers (enabled or not, as configured) and, for DHCPv4, the IPv4 addresses of the interfaces they name. A family without servers has no object (Delete → idle config) |
| Render | per family (`RenderFamily`); the input is embedded in the rendered config as top-level `user-context.vrx.input` (base64 of the deterministic protobuf, plus `servers` names) |
| Create / Update | render → `kea-dhcp<N> -t` → atomic write + `config-set`; a stopped daemon with an active config is a start request (logged; `Status` reports `action_required = "start"` on every read — D-079), not a failure |
| Delete | the idle configuration of the family |
| Retrieve | `config-get` (running) or the file the daemon loads at start (stopped) → embedded input → re-rendered → `ConfigDrift` against what the daemon has: equal → the input is the Value; different → a `structpb` drift Value (never equal, the reconciler re-applies). No embedded input (idle, or a foreign/commented `/etc/kea` file) → no object: never deleted or rewritten |
| Dependencies | `interface/<name>` of every server interface (optional) |

Modes (`subsystems/kea.go`, `VRX_KEA_MODE`): `product` (default; ProductPaths, no interface mapper until linux-cp — a
server interface is refused), `test` (`TestPaths(owner)`, checkers in `VRX_KEA_NETNS` through `ip netns exec`,
`VRX_KEA_IFMAP` explicit VPP→Linux map, no `if/addr` bindings), `off`.

State (`DhcpLeases` RPC, `rpc_kea.go`): `Status` (running, active, start request, reload age, per-subnet pool usage from
`statistic-get-all` named through the subnets' `user-context`) and `LeasePage` (`lease4/6-get-page` in pages of 1000, at
most 100 000 per family, filtered by server subnets and text, sorted, one page returned).

CLI equivalent: `vrx set|merge services dhcp …` + `vrx commit`; state through the REST routes
(`/api/v1/state/dhcp/leases`, operation id `KeaDhcpRelay_leases`).
