# Task: F-wireguard — WireGuard interfaces, peers and keys   (prepend 00-CONTEXT.md)

## Goal
Implement **WireGuard** end to end in FAST MODE: `wg<N>` interfaces with a private-key reference, listen port, addresses, peers
(public key, allowed IPs, endpoint, keepalive, optional PSK), live handshake status, and a key-generation helper that never shows the
private key. Reference: TNSR "WireGuard"; VPP plugin `wireguard` (WBS D6.5 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/vpn.ts` — `vpn.wireguard.interfaces.<name>{enabled, instance, vrf, underlayVrf, listenAddress, listenPort,
  privateKeyRef (key/<name>), address[], mtu, peers.<name>{publicKey, presharedKeyRef?, endpoint, allowedIps[], persistentKeepaliveSec}}`
  and proto `WireguardInterface` / `WireguardPeer` — extend only additively on `contract/F-wireguard`
- DF-5 (merged): `apps/agent/internal/descriptors/wireguard/` (`wireguard.interface`, `wireguard.peer`, write-only
  `wireguard.async-mode`, `peer.Events()`; entry point `wireguard.Register(r, client, owner, WithSecrets, WithKeyer, WithGlobalsOwner)`),
  `docs/agent/descriptors/wireguard.md` (secret reference forms `x25519:<pub>` / `hmac:<hex>` — keyed, D-096; canonical allowed-ips,
  VPP-wide public-key uniqueness), DF-5-questions Q3 (src_ip dependency), Q8 (who wires peer events into StreamEvents). TD-3 (merged)
  added the interface sanitizer call to `wireguard.interface` Create — keep it.
- **P08** (vertical slice) patterns you extend: builders in `apps/agent/internal/desired/`, registration + `Domains` in
  `apps/agent/internal/subsystems/subsystems.go` (`Wiring.VPNKeyer()` = the D-096 fingerprint key, `Wiring.BootStore()`, `Env.GlobalsOwner`;
  `IPsecOptions()` is the pattern for a `WireguardOptions()`), the hook in `apps/agent/internal/agent/projection.go`, and the read-only
  state RPC pattern (`InterfaceState`). P08 registers **no** DF-5 family yet; `Domains["vpn"]` is shared with P11 and F-ikev2-native —
  whichever lands first adds it, the others append. Read `docs/status/vertical-slice.md`.
- **Secret material fact (checked 2026-09-24):** no RPC or proto field carries secret material from the API to the agent (D-040 strips
  secret leaves; `vpn.Resolver` exists only as an interface + the test `vpn.MapResolver`). P08 leaves "the secret resolver" to P11. See
  open questions — do not invent a channel silently.
- `apps/agent/binapi/wireguard/` — `wireguard_interface_create`, `wireguard_peer_add_v2`, `wireguard_peers_v2_dump`, `want_wireguard_peer_events`
- `docs/decisions/LOG.md` D-051 (refs; secrets never in GET/logs), D-063 (async-mode write-only), D-065 (`interface/wg<N>` alias), D-071
  (async mode is a global → globals owner only), D-082 (globals lock in tests), D-096 (keyed fingerprints)
- `docs/agent/descriptors/wireguard.md` → test listen ports `20000+100·slot+10/+11`, never 51820 on the shared host (DF-5 port scheme)

## Scope — build exactly this
1. **Schema**: these rules **already exist** (P02c, `packages/schema/src/semantic/vpn.ts`) — test them, do not re-add:
   `vpn.wireguard-unique` (instance; listen socket = underlayVrf + listenAddress + listenPort — VPP allows several interfaces on one port;
   duplicate public key inside one interface; the same allowed-IP prefix on two peers of one interface), `vpn.wireguard-address-overlap`,
   `vpn.local-address-configured` (listenAddress), `vpn.vrf-exists`; `privateKeyRef`/`presharedKeyRef` existence is P06's generic
   `secrets.ref-exists` (D-051). **Add only** (own file `semantic/wireguard.ts`, contract branch): `publicKey` unique across **all**
   interfaces (VPP-wide, DF-5); allowedIps without host bits and not overlapping (not only equal) inside one interface; endpoint family =
   listen family.
2. **Agent**: builder `apps/agent/internal/desired/wireguard*.go` (P08 pattern): `vpn.wireguard` → `wireguard.interface/wg<instance>` +
   `interface-ip` addresses + MTU + `wireguard.peer/…` (+ routes for allowedIps via core `ip.route` when `routeAllowedIps` is set — add that
   flag on the contract branch, default false), plus the assembler back to `WireguardConfig`; register the family from an owned
   `subsystems/wireguard.go` with `WithKeyer(Wiring.VPNKeyer())`, `WithGlobalsOwner(env.GlobalsOwner)` and the secret resolver; resolve
   `key/<name>` → the DF-5 `x25519:` reference through the agent secret resolver (it does not exist yet — open questions). Wire
   `peer.Events()` into StreamEvents (handshake up/dead; subsystems has no event sink yet — add one field to `subsystems.Env`, or use
   the one a wave-A task added). ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, prefixed objects, slot ports).
3. **API**: config via pointer routes; `GET /api/v1/state/vpn/wireguard` (per peer: last handshake, established/dead, rx/tx bytes, endpoint
   learnt); `POST /api/v1/actions/vpn/wireguard/keypair` → generates a key pair, stores the private key as a secret (`POST /secrets` path)
   and returns `{ref, publicKey}` only. OpenAPI; regenerate `packages/api-client`.
4. **UI**: WireGuard page — interfaces list with peers sub-table, status chips from the WS events, "generate key pair" button, peer
   config export (client `.conf` with the **client's** private key only when generated in the same session and never stored in the UI);
   en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/wireguard.md` — site-to-site and road-warrior example, CLI equivalent.
Files you own: `apps/agent/internal/descriptors/wireguard/**` (gap-only), `docs/agent/descriptors/wireguard.md`,
`apps/agent/internal/desired/wireguard*.go`, `apps/agent/internal/subsystems/wireguard*.go`, `apps/agent/internal/agent/rpc_wireguard*.go`,
`apps/agent/internal/actions/wireguard/**`, `apps/api/src/features/wireguard/**`, `apps/api/test/e2e/wireguard*`,
`apps/web/src/domains/vpn/wireguard/**`, `apps/web/src/locales/*/wireguard.json`, `docs/user/vpn/wireguard.md`, `test/topology/wireguard/**`.
**Read-only:** `apps/agent/internal/descriptors/vpn/**` (DF-5 helpers shared with P11 and F-ikev2-native; a change goes to the questions file).
Shared files: registration hunks only, listed in your PR (`subsystems.go` `Domains["vpn"]` + one Register line, `projection.go` hook,
`proto` rpc/EventKind/field numbers from the manager, `agent.client.ts`, `fake-agent.ts`, `app.module.ts`, `infra/bus.ts` topic,
router/nav, `i18n.ts`, the vpn page's tab registry).

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
**Secret channel (blocking for the product path):** the agent cannot get a WireGuard private key or PSK today (see Inputs). Use the
channel P11 (or a manager-designated row) provides if it is on main when you start; otherwise build everything else, run the host check
with a slot-local `vpn.MapResolver` fixture (keys = `VRX_TEST_PSK_<id>`-derived test vectors), keep the end-to-end step open and write it
in the questions file — never put key material into `DesiredState`, a status file or a log.
