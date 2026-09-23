# Task: F-kea-dhcp-relay — Kea DHCPv4/v6 server, VPP DHCP relay and DHCP client   (prepend 00-CONTEXT.md)

## Goal
DHCP end to end (WBS D7.1, D7.2 in `plan/wbs.csv`): Kea DHCPv4/v6 servers with subnets, pools, reservations and option sets plus a
lease browser; VPP's DHCP relay/proxy per VRF; the DHCPv4 client on an interface (and DHCPv6 IA_NA/PD client, write-only).
Reference: TNSR "DHCP server (Kea), DHCP relay, DHCP client"; VPP plugin `dhcp` (+ `dhcp6_ia_na_client_cp`, `dhcp6_pd_client_cp`).

## Inputs to read first
- `packages/schema/src/domains/services.ts` — `services.dhcp{servers{<name>}, relays{<name>}}` exists (DhcpServer/Subnet/Pool/Reservation/
  Option/Relay); the client lives on the interface: `interfaces.<name>.dhcpClient` (`interfaces.ts`, D-050) — extend only additively
- `apps/agent/internal/renderers/kea/` (RF-3, merged) + `docs/agent/renderers/kea.md` + its README — reuse; Kea is driven over its own UNIX
  control sockets (`<run>/kea4.sock`, `kea6.sock`, root-only dir, 0600), **no kea-ctrl-agent** (D-079); `config-set` per server, no restart
- `apps/agent/internal/descriptors/dhcp/` (DF-8, merged) + `docs/agent/descriptors/dhcp.md` — `dhcp.proxy`, `dhcp.proxy-vss`, `dhcp.client`,
  write-only `dhcp.dhcp6-client`/`dhcp6-pd-client`/`dhcp6-pd-address`/`dhcp6-duid` (D-063/D-076), `ClientDescriptor.Leases`, `WatchLeases`
- `apps/agent/binapi/dhcp/` — `dhcp_proxy_config`, `dhcp_proxy_set_vss`, `dhcp_proxy_dump`, `dhcp_client_config`, `dhcp_client_dump` (verified)
- `docs/lab/shared-host-rules.md` — daemon ownership: Kea runs only as a per-slot test instance (own conf/run/lib dirs, own netns), never the
  system `kea-dhcp4-server` unit; D-077 Q7 (relay is per-VRF, not a global)

## Contract changes
Only if needed (e.g. relay option-82/remote-id, client classes): commit on `contract/F-kea-dhcp-relay` (schema + proto + drift guard),
`docs/status/tasks/F-kea-dhcp-relay-contract.md`, tell the manager in the questions file, continue. No reshaping.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/kea/**`, `apps/agent/internal/descriptors/dhcp/**`, `docs/agent/renderers/kea.md`,
`docs/agent/descriptors/dhcp.md`, `apps/agent/internal/agent/project_kea_dhcp_relay*.go`, `apps/api/src/features/kea-dhcp-relay/**`,
`apps/web/src/domains/services/kea-dhcp-relay/**`, `apps/web/src/locales/*/kea-dhcp-relay.json`, `docs/user/services/kea-dhcp-relay.md`,
`test/topology/kea-dhcp-relay/**`. Shared files: one-line appends only (agent registry, `app.module.ts`, router/nav).
1. **Schema** (semantic rules on the contract branch only if missing): pools inside their subnet and non-overlapping; reservations inside the
   subnet and outside pools (or flagged), unique MAC/DUID per subnet; relay `sourceAddress` configured on an interface of `serverVrf`;
   one Kea VRF per family (renderer rule); `dhcpClient` excludes static IPv4 addresses on the same interface.
2. **Agent**: project `services.dhcp.servers` → Kea renderer, `services.dhcp.relays` → `dhcp.proxy` (+ `dhcp.proxy-vss` when set),
   `interfaces.<n>.dhcpClient` → `dhcp.client`; interface refs are `interface/<name>` (D-065/D-069). Renderer Retrieve (`config-get`,
   `status-get`) and descriptor Retrieve cover every object; lease reads via `lease4/6-get-page` with server-side paging.
3. **API**: config via pointer routes; `GET /api/v1/state/dhcp/leases?server&page&filter` (paged, never the whole file),
   `GET /api/v1/state/dhcp/relays` (from Retrieve), `GET /api/v1/state/interfaces/{name}/dhcp-client` (lease, from `Leases`).
4. **UI**: DHCP page with tabs Servers / Subnets & Pools / Reservations / Relays / Leases (ServerDataGrid); client toggle stays in the
   interface drawer (link only); pool utilisation bar; en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/services/kea-dhcp-relay.md` — LAN server, relay to a central server, WAN DHCP client; CLI equivalent.

## Acceptance (paste the evidence)
- [ ] Test Kea instance (slot dirs/netns) answers a `dhclient`/`perfdhcp` request from `ns-<p>-lan`; lease appears in the API lease page
- [ ] `vppctl show dhcp proxy` lists the relay with the prefixed VRF; `vppctl show dhcp client` shows the client on a slot interface
- [ ] Agent-restart simulation → Kea config re-applied and relay/client recreated within 30 s (log excerpt)
- [ ] Rollback removes relay + client (Retrieve) and the server's subnets (`config-get`)
- [ ] Pool outside its subnet → 400 problem+json with a `pointer` to the pool
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
Unbound/VPP DNS cache (F-unbound-chrony-syslog), NTP, DHCP failover/HA pairs and Kea HA hook (F-ha-state-sync), DDNS, Kea database
backends (memfile only), kea-ctrl-agent (D-079), IPv6 RA/SLAAC (F-neighbors-ra), DHCPv6-PD downstream delegation UI beyond the write-only
client, interface addressing itself (P08).

## Open questions to surface, not to decide silently
Relay on an interface that also runs a Kea server in the same VRF — refuse or allow? DHCPv6 client objects are write-only (no dump): how the
UI shows their status (CLI-only `show dhcp6 clients`) — propose "configured, state unknown".
