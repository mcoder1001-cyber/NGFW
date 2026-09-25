# F-kea-dhcp-relay — Kea DHCPv4/v6 server, VPP DHCP relay, DHCP client (WBS D7.1, D7.2)

Branch `task/F-kea-dhcp-relay` (worktree `/root/ngfw-wt/F-kea-dhcp-relay`), slot 2 (`w2`, table range 2000–2999),
daemon-owner: kea (slot-local test instance only; `kea-dhcp4-server`/`kea-dhcp6-server` stay disabled and stopped,
`/etc/kea` untouched, no kea-ctrl-agent).

## What was built

| layer | what |
|---|---|
| contract | `rpc DhcpLeases` + `DhcpLeasesRequest/Response`, `DhcpLease`, `DhcpSubnetUsage`, `DhcpServerStatus`, `DhcpClientLease` (F-kea-dhcp-relay proto section; DhcpRelay 9–10 unused); four semantic rules in `packages/schema/src/semantic/kea-dhcp-relay.ts` — see `F-kea-dhcp-relay-contract.md` |
| agent | `Domains["services"]`: `kea.dhcp4/vrx`, `kea.dhcp6/vrx` (RF-3's renderer as one singleton descriptor per daemon, D-109 d), `dhcp.proxy`, `dhcp.proxy-vss`, `dhcp.relay` (relay records); builders/assemblers `desired/kea.go`, `desired/dhcp_relay.go`; `subsystems/kea.go` (modes product/test/off, relay scope = slot table range, DHCP runtime); `agent/rpc_kea.go` (`DhcpLeases`); Kea `Status`/`LeasePage`; ownership declarations for the TD-11b guard; **TD-11b Q3**: `dhcp.client` claims before the VPP add |
| API | `GET /api/v1/state/dhcp/leases?server&family&page&pageSize&filter`, `GET /api/v1/state/dhcp/relays`, `GET /api/v1/state/interfaces/{name}/dhcp-client` (`features/kea-dhcp-relay`), fake `DhcpLeases`, e2e; OpenAPI → api-client, CLI operation table regenerated |
| UI | Services → DHCP tab (Servers / Subnets & Pools / Reservations / Relays / Leases), schema-driven edit dialogs, pool utilisation bars, server-paged lease grid, Refresh button (D-132), en + fa |
| docs | `docs/user/services/kea-dhcp-relay.md`, `docs/agent/renderers/kea.md`, `docs/agent/descriptors/dhcp.md`, `renderers/kea/README.md` |
| test | `test/topology/kea-dhcp-relay` (host VPP + Kea + dhclient), unit tests (renderer/descriptor/status, relay record, claim order, projection round trip, semantic rules), API e2e |

(evidence sections follow below)
