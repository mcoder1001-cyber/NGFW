# kea — Kea DHCPv4/v6 + control-agent renderer (RF-3, WBS D7.1)

Installed version decides the design: **Kea 3.0.3** (`kea-dhcp4 -V`). Kea 3.0 still ships
`kea-ctrl-agent` (deprecated in favour of HTTP control sockets in the servers); the agent does not
need it — it talks to the servers' unix control sockets directly — so the control agent is
rendered and validated for the remote/UI path only and reloaded when its file changes and it runs.

| step | how |
|---|---|
| Render | `services.dhcp.servers` → typed Go structs → `encoding/json` (HTML escaping off, 2-space indent). No text templates. Files: `kea-dhcp4.conf`, `kea-dhcp6.conf` (always both; a family without enabled servers is an idle config with no interfaces), `kea-ctrl-agent.conf`; mode 0640, owner `_kea:_kea` in the product. |
| Validate | `kea-dhcp4 -t`, `kea-dhcp6 -t`, `kea-ctrl-agent -t` on a staged copy. Kea's checker verifies that a subnet's `interface` exists, so with `Paths.Netns` (tests) the DHCP checkers run under `ip netns exec` (test runner only). |
| Apply | snapshot → atomic write → `config-set` (rendered file content as arguments) over each running server's unix socket → ctrl-agent `config-reload` if its file changed and it answers. Failure → snapshot restored, previous config `config-set` again. A server that is not running: idle config → nothing; active config → `*ActionRequired{Unit: kea-dhcpN-server, Action: start}` (files stay written). `config-write` is **not** used: the file on disk already is the rendered config (config-write would replace it with Kea's canonical dump and break byte-level drift detection). |
| Retrieve | `status-get`, `config-get` (hash removed), `statistic-get-all`, `lease4/6-get-page` (1000 per page, max 100 000, `leasesTruncated`) → `structpb.Struct {"dhcp4":…, "dhcp6":…}`. `ConfigDrift(rendered, config-get)` is the normalised subset diff used by the integration test. |
| Events | poll `statistic-get-all` at 1 Hz: running, `pkt4/6-received`, `pkt4-ack-sent`, `pkt6-reply-sent`, per-subnet/pool assigned/total addresses. |

## Mapping decisions

- One kea-dhcp4 (and one kea-dhcp6) process serves every enabled server of its family. Server
  settings (lease time, T1/T2, authoritative, global options) are pushed down to each subnet.
  All servers of a family must share one VRF (else `ErrInvalid`); per-VRF instances are a later
  extension (vdom.md #5 keeps the document ready).
- Subnet ids are FNV-32a of `<server>/<subnet>` (stable when other subnets change; leases
  reference them), linear probing on collision.
- `user-context.vrx` carries server/subnet/reservation names and descriptions back through
  `config-get`. Kea's JSON is byte-oriented (it re-emits non-ASCII bytes as `\u00XX`), so free
  text is made printable ASCII with Go escape syntax (`☃` → `☃`, `\` → `\\`) and
  `DecodeText` restores it exactly.
- Typed options: v4 `routers`(3) `domain-name-servers`(6) `domain-name`(15) `ntp-servers`(42)
  `domain-search`(119); v6 `dns-servers`(23) `sntp-servers`(31) `domain-search`(24, the domain
  name leads it — DHCPv6 has no domain-name option). Custom options: a Kea standard code
  (probed list for 3.0.3) uses Kea's definition; any other code with `0x…` data is sent as raw
  hex (`csv-format: false`, stored without `0x` as Kea reports it); other text gets a string
  `option-def` `vrx-<code>`. Option data: printable ASCII, ≤ 255 (v4) / 1024 (v6).
- Interfaces: `WithInterfaceMapper` maps VPP names to Linux names (default identity; names must
  be Linux interface names). With a `DesiredState` the v4 binding is `<if>/<addr>` when the
  interface has an address inside one of the subnets. `Paths.InterfacePrefix` (tests: `w6-`)
  refuses anything else — a test rendering can never bind `ens192`.
- `dhcp-socket-type` is rendered only for `udp` (`raw` is Kea's default and `config-get` omits it).
- Kea 3.0 path restrictions: control sockets, lease files and logs must live in
  `/run/kea`, `/var/lib/kea`, `/var/log/kea` unless `KEA_CONTROL_SOCKET_DIR`,
  `KEA_DHCP_DATA_DIR`, `KEA_LOG_FILE_DIR` say otherwise; `NewRunner`/`Env` set them from `Paths`,
  and the socket directory must be mode 0750 or stricter.

## Not rendered

DHCP relay (VPP, DF-8), DDNS, HA hooks, SQL lease backends, client classes, shared networks,
option spaces beyond dhcp4/dhcp6 (all outside RF-3).

## Tests

`go test ./internal/renderers/kea/` — goldens in `testdata/` (`-update` rewrites), hostile
strings, argv of the checkers, Apply/rollback with a fake controller. Integration
(`VRX_INTEGRATION=1`): `kea_integration_test.go` — `ns-<prefix>-a` with veth `<prefix>-a`
(10.<slot>.10.1/24), kea-dhcp4/6 inside it, kea-ctrl-agent on 127.0.0.1:3<slot>80, all children
killed by PID.
