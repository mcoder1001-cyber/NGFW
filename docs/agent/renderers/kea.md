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
`statistic-get-all`, `lease4/6-get-page`. CLI equivalent: none yet (API/CLI are later tasks).
