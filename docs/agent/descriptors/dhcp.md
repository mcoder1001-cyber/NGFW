# dhcp plugin descriptors (DF-8, WBS D7.2)

Package `apps/agent/internal/descriptors/dhcp` — DHCPv4/v6 relay per VRF, relay VSS, DHCPv4 client, DHCPv6 IA_NA
and prefix-delegation clients, prefix-derived addresses and the DHCPv6 DUID. Message names only from
`apps/agent/binapi/{dhcp,dhcp6_ia_na_client_cp,dhcp6_pd_client_cp}`. `dhcp.Register(registry, client, owner, opts...)`
registers all seven; options: `WithInterfaceKey`, `WithVRFKey`, `WithVRFScope`.

Values are `*structpb.Struct` documents built from the typed specs in `spec.go` (`dhcp.Proxy{...}.Proto()`, D-055
stand-in; codec in `descriptors/dfkit`). Always build values with `.Proto()` — every field is emitted, addresses are
canonical (`net/netip`), so `proto.Equal(desired, Retrieve())` is a correct diff.

| Object type | Key | VPP messages (create / delete) | Retrieve | Update | Dependencies |
|---|---|---|---|---|---|
| `dhcp.proxy` (relay server) | `dhcp.proxy/<rx_vrf>/<server_vrf>/<server-ip>` | `dhcp_proxy_config` is_add=1 / 0 (v4 or v6 by address family) | `dhcp_proxy_dump` is_ip6=0 and 1, one KV per server | `ErrRecreate` (src is per rx VRF) | `vrf/<rx>`, `vrf/<server>` optional (VPP creates missing tables; VRF 0 omitted) |
| `dhcp.proxy-vss` | `dhcp.proxy-vss/<ip4\|ip6>/<vrf>` | `dhcp_proxy_set_vss` is_add=1 / 0 | VSS fields of `dhcp_proxy_details` | in place (`dhcp_proxy_set_vss`) | `vrf/<vrf>` optional |
| `dhcp.client` (DHCPv4 client) | `dhcp.client/<ifname>` | `dhcp_client_config` is_add=1 / 0 | `dhcp_client_dump` (owned interfaces) | `ErrRecreate` | `interface/<ifname>` (D-065) |
| `dhcp.dhcp6-client` (IA_NA) | `dhcp.dhcp6-client/<ifname>` | `dhcp6_client_enable_disable` | **write-only** (`ErrRetrieveUnsupported`) | re-apply | `interface/<ifname>` |
| `dhcp.dhcp6-pd-client` | `dhcp.dhcp6-pd-client/<ifname>` | `dhcp6_pd_client_enable_disable` | **write-only** | `ErrRecreate` (group change) | `interface/<ifname>` |
| `dhcp.dhcp6-pd-address` | `dhcp.dhcp6-pd-address/<ifname>/<group>/<addr>/<len>` | `ip6_add_del_address_using_prefix` | **write-only** | re-apply (all fields are key) | `interface/<ifname>`, `dhcp.dhcp6-pd-client/<ifname>` optional |
| `dhcp.dhcp6-duid` (singleton) | `dhcp.dhcp6-duid/global` | `dhcp6_duid_ll_set` / — (no reset in VPP: Delete is a no-op) | **write-only** | in place | — |

Status / actions (not desired state): `ClientDescriptor.Leases` (lease per owned interface from `dhcp_client_dump`),
`WatchLeases` (`dhcp_compl_event` → `StreamEvents`), `WatchDHCP6Replies` / `WatchDHCP6PDReplies`
(`want_dhcp6_reply_events` / `want_dhcp6_pd_reply_events` + the events, unsubscribed on ctx cancel),
`SendDHCP6ClientMessage` / `SendDHCP6PDClientMessage` (action helpers).

## Ownership (shared VPP)
- Interface-bound objects: the interface must carry the owner tag `<owner>:<id>` (`vpp.OwnerTag`); Create refuses
  others (`dfkit.ErrNotOwned`), Retrieve filters by tag. The DHCPv4 hostname should carry the owner prefix (tests: `w5-host`).
- Relay objects have no tag: they are owned through their **rx VRF**, which must be inside `WithVRFScope` (production:
  every VRF; tests: the slot's table range `N000–N999`).

## Notes and limitations
- **Write-only (D-063):** VPP 26.06 has no dump for the DHCPv6 clients, prefix addresses or the DUID. Retrieve returns
  `dfkit.ErrRetrieveUnsupported` (message text equals P05's `scheduler.ErrRetrieveUnsupported`); every Create is
  idempotent (repeated enable is accepted; `DUPLICATE_IF_ADDRESS` = success); Delete treats "already gone" as success.
  `show dhcp6 clients` exists as CLI only.
- A VSS is reported only while its VRF relays to at least one server (VPP puts it in `dhcp_proxy_details`); a VSS on a
  non-relaying VRF would be re-created on every reconcile — configure VSS only on proxied VRFs.
- DHCPv4 client: VPP copies hostname and client id with `strlen`; an empty hostname is rejected by `Validate`. A second,
  different client on the same interface fails (`INVALID_VALUE`); an identical re-apply succeeds (compared with the dump).
  The lease is status (`Leases`), never part of the Value. `pid` = agent PID (event demux).
- The DUID is VPP-global without getter or reset: the host test runs only with `VRX_DF8_DUID=1`.
- Evidence: `docs/status/tasks/DF-8.md` (`show dhcp proxy`, `show dhcpv6 proxy`, `show dhcp vss`, `show dhcp client`,
  `show dhcp6 clients` during the host run, empty after).
