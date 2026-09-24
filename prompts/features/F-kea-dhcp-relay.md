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
- **P08** (vertical slice) **already projects the DHCPv4 client**: `apps/agent/internal/desired/interfaces.go` maps
  `interfaces.<if>.dhcpClient` → `dhcp.client` and assembles it back, `subsystems.go` registers `dhcp.NewClient` in `Domains["interfaces"]`
  and calls its `Reconnected()` from the VPP-connect hook. Do not re-project it; you only add the lease read (a `Wiring` accessor in your
  own `subsystems/kea.go`) and the relay/server paths. P08 patterns: builders in `internal/desired/`, registration + `Domains` in
  `subsystems/subsystems.go`, the hook in `agent/projection.go`, read-only state RPCs like `InterfaceState`. `Domains["services"]` is shared
  (F-rpf-adl-pbr `autoSdl`, F-unbound-chrony-syslog, later F-snmp/F-qos-flat/F-ipfix-sflow): whoever lands first adds it, the others append.
- **Agent integration fact (checked 2026-09-24):** the agent's Apply/DryRun/Retrieve/Resync path runs only the descriptor scheduler; no
  renderer is called anywhere in the agent (`apps/agent/internal/agent/service.go`). Default (same as F-host-acl-nftables, log it as a
  decision with options): wrap each Kea renderer in one singleton scheduler descriptor (e.g. `kea.dhcp4/vrx`, `kea.dhcp6/vrx`: Create/Update
  = Render → Validate → Apply, Delete = idle config, Retrieve = `config-get` normalised), registered under `Domains["services"]` — no change
  to the agent core. Only if the manager has put a shared renderer stage on main when you start, use that instead.
- `apps/agent/binapi/dhcp/` — `dhcp_proxy_config`, `dhcp_proxy_set_vss`, `dhcp_proxy_dump`, `dhcp_client_config`, `dhcp_client_dump` (verified)
- `docs/lab/shared-host-rules.md` — daemon ownership: Kea runs only as a per-slot test instance (own conf/run/lib dirs, own netns), never the
  system `kea-dhcp4-server` unit (installed, disabled — leave it so); D-077 Q7 (relay is per-VRF, not a global)

## Contract changes
Only if needed (e.g. relay option-82/remote-id, client classes): commit on `contract/F-kea-dhcp-relay` (schema + proto + drift guard),
`docs/status/tasks/F-kea-dhcp-relay-contract.md`, tell the manager in the questions file, continue. No reshaping.

## Scope — build exactly this
Files you own: `apps/agent/internal/renderers/kea/**`, `apps/agent/internal/descriptors/dhcp/**` (gap-only: P08 calls `dhcp.NewClient`,
`WithInterfaceKey`, `Reconnected` — keep those signatures), `docs/agent/renderers/kea.md`, `docs/agent/descriptors/dhcp.md`,
`apps/agent/internal/desired/{kea,dhcp_relay}*.go`, `apps/agent/internal/subsystems/kea*.go`, `apps/agent/internal/agent/rpc_kea*.go`,
`apps/api/src/features/kea-dhcp-relay/**`, `apps/api/test/e2e/kea-dhcp-relay*`, `apps/web/src/domains/services/kea-dhcp-relay/**`,
`apps/web/src/locales/*/kea-dhcp-relay.json`, `docs/user/services/kea-dhcp-relay.md`, `test/topology/kea-dhcp-relay/**`.
Shared files: registration hunks only, listed in your PR (`subsystems.go` `Domains["services"]` + Register lines, `projection.go` hook,
proto rpc/field numbers from the manager, `agent.client.ts`, `fake-agent.ts`, `app.module.ts`, router/nav, `i18n.ts`, the services page's
tab registry). `apps/agent/internal/desired/interfaces.go` stays P08's (the client projection is done).
1. **Schema**: these rules **already exist** — test them, do not re-add: pools inside their subnet, same family, non-overlapping; reservations
   inside the subnet, one address reserved once, one MAC/DUID once (`DhcpSubnetSchema` refinements); subnets unique / non-overlapping per
   VRF (`services.dhcp-subnets-unique`) and inside an interface prefix (`services.dhcp-subnet-within-interface-prefix`); relay
   `sourceAddress` configured in `serverVrf ?? vrf` (`services.bind-address-configured`). **Add only** (own file, contract branch):
   reservations outside pools (or flagged); one Kea VRF per family (renderer rule); `dhcpClient` excludes static IPv4 addresses on the same
   interface (only the unnumbered exclusivity exists today).
2. **Agent**: builders `desired/kea*.go` (`services.dhcp.servers` → the Kea renderer descriptor(s), see Inputs) and `desired/dhcp_relay*.go`
   (`services.dhcp.relays` → `dhcp.proxy` + `dhcp.proxy-vss` when set) with assemblers; interface refs are `interface/<name>`
   (D-065/D-069). `interfaces.<n>.dhcpClient` → `dhcp.client` is P08's (done). Renderer Retrieve (`config-get`, `status-get`) and descriptor
   Retrieve cover every object; lease reads via `lease4/6-get-page` with server-side paging.
3. **API**: config via pointer routes; `GET /api/v1/state/dhcp/leases?server&page&filter` (paged, never the whole file),
   `GET /api/v1/state/dhcp/relays` (from Retrieve), `GET /api/v1/state/interfaces/{name}/dhcp-client` (lease, from `Leases`).
4. **UI**: DHCP page with tabs Servers / Subnets & Pools / Reservations / Relays / Leases (ServerDataGrid); client toggle stays in the
   interface drawer (link only); pool utilisation bar; en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/services/kea-dhcp-relay.md` — LAN server, relay to a central server, WAN DHCP client; CLI equivalent.

## Acceptance (paste the evidence)
- [ ] Test Kea instance (slot dirs/netns) answers a `dhclient` request from `ns-<p>-lan` (`perfdhcp` is not installed on the host; run
      `dhclient -sf /bin/true` with `-lf`/`-pf` under `/run/vrx-test/<p>/` — dhclient-script under `ip netns exec` would rewrite the host's
      `/etc/resolv.conf`); lease appears in the API lease page
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
