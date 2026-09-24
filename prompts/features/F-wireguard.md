# Task: F-wireguard — WireGuard interfaces, peers and keys   (prepend 00-CONTEXT.md)

## Goal
Implement **WireGuard** end to end in FAST MODE: `wg<N>` interfaces with a private-key reference, listen port, addresses, peers
(public key, allowed IPs, endpoint, keepalive, optional PSK), live handshake status, and a key-generation helper that never shows the
private key. Reference: TNSR "WireGuard"; VPP plugin `wireguard` (WBS D6.5 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/vpn.ts` — `vpn.wireguard.interfaces.<name>{enabled, instance, vrf, underlayVrf, listenAddress, listenPort,
  privateKeyRef (key/<name>), address[], mtu, peers.<name>{publicKey, presharedKeyRef?, endpoint, allowedIps[], persistentKeepaliveSec}}`
  and proto `WireguardInterface` / `WireguardPeer` — extend only additively on `contract/F-wireguard`
- DF-5 (branch `task/DF-5` until merged — `git show task/DF-5:<path>`): `apps/agent/internal/descriptors/wireguard/`
  (`wireguard.interface`, `wireguard.peer`, write-only `wireguard.async-mode`, `peer.Events()`), `docs/agent/descriptors/wireguard.md`
  (secret reference forms `x25519:<pub>` / `sha256:<hex>`, canonical allowed-ips, VPP-wide public-key uniqueness), DF-5-questions Q3 (src_ip
  dependency), Q8 (who wires peer events into StreamEvents)
- `apps/agent/binapi/wireguard/` — `wireguard_interface_create`, `wireguard_peer_add_v2`, `wireguard_peers_v2_dump`, `want_wireguard_peer_events`
- `docs/decisions/LOG.md` D-051 (refs; secrets never in GET/logs), D-063 (async-mode write-only), D-065 (`interface/wg<N>` alias), D-071
  (async mode is a global → globals owner only), D-082 (globals lock in tests)
- `docs/lab/shared-host-rules.md` — listen ports `20000+100·slot+10/+11`, never 51820 on the shared host

## Scope — build exactly this
1. **Schema**: semantic rules — `publicKey` unique across all interfaces (VPP-wide rule); allowedIps masked and non-overlapping inside one
   interface; `listenPort` unique per underlay VRF; `privateKeyRef` must exist (API check, D-051); endpoint family = listen family.
2. **Agent**: projection `vpn.wireguard` → `wireguard.interface/wg<instance>` + `interface-ip` addresses + MTU + `wireguard.peer/…`
   (+ routes for allowedIps via core `ip.route` when `routeAllowedIps` is set — add that flag on the contract branch, default false);
   resolve `key/<name>` → the DF-5 `x25519:` reference through the agent secret resolver. Wire `peer.Events()` into StreamEvents
   (handshake up/dead). ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, prefixed objects, slot ports).
3. **API**: config via pointer routes; `GET /api/v1/state/vpn/wireguard` (per peer: last handshake, established/dead, rx/tx bytes, endpoint
   learnt); `POST /api/v1/actions/vpn/wireguard/keypair` → generates a key pair, stores the private key as a secret (`POST /secrets` path)
   and returns `{ref, publicKey}` only. OpenAPI; regenerate `packages/api-client`.
4. **UI**: WireGuard page — interfaces list with peers sub-table, status chips from the WS events, "generate key pair" button, peer
   config export (client `.conf` with the **client's** private key only when generated in the same session and never stored in the UI);
   en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/wireguard.md` — site-to-site and road-warrior example, CLI equivalent.
Files you own: `apps/agent/internal/descriptors/wireguard/**`, `docs/agent/descriptors/wireguard.md`, `apps/agent/internal/agent/project_wireguard*.go`,
`apps/agent/internal/actions/wireguard/**`, `apps/api/src/features/wireguard/**`, `apps/web/src/domains/vpn/wireguard/**`,
`apps/web/src/locales/*/wireguard.json`, `docs/user/vpn/wireguard.md`, `test/topology/wireguard/**`.
Shared files: one-line appends only (app.module.ts, router/nav, agent registry, StreamEvents topic list).

## Acceptance (paste the evidence)
- [ ] After commit `Retrieve()` == desired and `vppctl show wireguard interface` / `show wireguard peer` list them (private key and
      mac-key **redacted** from the pasted output — the CLI prints them)
- [ ] Handshake evidence: a kernel `wg` peer in `ns-<p>-wan` (optional packet test, not required) or at least the peer event
      `established` reaching the UI; record which one you ran
- [ ] Agent-restart simulation → interface and peers back within 30 s (log excerpt)
- [ ] Rollback removes interfaces and peers (Retrieve empty)
- [ ] Duplicate public key on two interfaces → 400 problem+json with `pointer` to the second peer
- [ ] No private key or PSK in logs, GET, fixtures, status files; `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
IPsec in any form (P11, F-ikev2-native, F-ra-vpn); PKI (F-pki); dynamic routing over wg (F-ospf / P12); VPP-generated keys
(`generate_key` stays false — DF-5); async crypto mode tuning; HA key sync (F-ha-state-sync); tunnel dashboards (F-dashboard-prom-alarms);
a mobile-client QR provisioning portal.

## Open questions to surface, not to decide silently
DF-5 Q3 (dependency on the address carrying `listenAddress`) — pick (a) no dependency unless ordering fails in the integration test.
Whether allowed-IPs should auto-install routes by default (TNSR does not) — default off, flag it.
