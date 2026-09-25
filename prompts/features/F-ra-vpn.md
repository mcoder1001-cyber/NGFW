# Task: F-ra-vpn — remote-access VPN (IKEv2 + EAP, client pools)   (prepend 00-CONTEXT.md)

## Goal
Remote-access VPN end to end in FAST MODE: road-warrior clients (Windows/macOS/iOS/Android native IKEv2, strongSwan client) connect
with EAP-MSCHAPv2 (local users), EAP-TLS/pubkey (client CA) or EAP-RADIUS, receive an address from a client pool plus DNS and split-
tunnel routes; IKE in strongSwan, ESP in VPP (kernel-vpp, P11). Reference: TNSR "IPsec remote access (mobile clients)"; strongSwan
swanctl `pools` + `eap-*` plugins (WBS D6.9 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/vpn.ts` — `vpn.remoteAccess.<name>{enabled, localAddr, localId, vrf, underlayVrf, auth eap-mschapv2|eap-tls|
  eap-radius|pubkey, certificate, clientCa, proposal, pools[{name, prefix, dns[]}], splitTunnel[], users[{username, passwordRef}],
  radius{servers[{address, port, secretRef}]}, dpd, rekey}` + `semantic/vpn.ts` (pool overlap, refs); proto `RemoteAccessProfile`
- P11 (merged before you start): `apps/agent/internal/renderers/strongswan/` (RF-2 + P11) and `docs/agent/renderers/strongswan.md` — the
  model already renders `pools { <name> { addrs; dns } }` and `authorities`; **you extend this renderer** through new files
  (`ra*.go`, `templates/vrx-ra*.tmpl`) plus a few named hunks in P11's files (envelope list) — nobody else edits it while you run
- F-pki (merged): certificate/CA files materialised by `apps/agent/internal/pki` — you reference names, you do not write PEM files
- `apps/agent/binapi/ipsec/` for SA state (`ipsec_sa_v5_dump`) — via P11's state code, not new descriptors
- `docs/decisions/LOG.md` D-051 (password/psk refs; EAP secrets rendered only into the 0600 secrets file), D-040, D-067 (IKEv1 + GCM IKE rejected),
  D-083/D-089 (stock strongSwan client = debs unpacked under `/run/vrx-test/<slot>/`, owner-prefix scoping on a shared charon);
  `docs/decisions/PENDING-secret-channel.md` (EAP/RADIUS secrets need the API→agent channel: use what P11 merged, else a fixture resolver);
  `docs/vpp-code-track.md` V6 (vpp_sswan vs strongSwan 6.x — use the version P11 pinned)

## Scope — build exactly this
1. **Schema**: semantic rules (most exist) — pool prefixes do not overlap each other, interface subnets or other profiles' pools;
   `eap-tls`/`pubkey` need `clientCa`; `eap-radius` needs ≥ 1 server; `eap-mschapv2` needs ≥ 1 user; usernames unique; server
   certificate required. Missing fields (e.g. per-user static IP, RADIUS accounting) → `contract(schema|proto): …` commits on your task
   branch, additive only (no `contract/` branch; numbers from `docs/status/wave-BC-numbers.md`).
2. **Agent**: extend the strongSwan renderer — one `connections.ra-<name>` per profile (`remote_addrs = %any`, `pools`, `send_certreq`,
   `eap_id = %any`, child `local_ts = splitTunnel or 0.0.0.0/0,::/0`), `secrets.eap-<user>` from resolved refs (0600 file, never logged),
   `eap-radius` plugin section in `strongswan.conf` with the resolved shared secret; VPP side: client-pool routes resolved via the route
   the kernel-vpp plugin installs (verify, do not add a second programmer — D-072 spirit). State: connected users (identity, virtual IP,
   uptime, bytes) from VICI `list-sas`; action: disconnect a user (`terminate` over VICI). ONE integration check (`VRX_INTEGRATION=1`).
3. **API**: config via pointer routes; `GET /api/v1/state/vpn/remote-access/sessions` (paged),
   `POST /api/v1/actions/vpn/remote-access/sessions/{id}/disconnect` (state routes are read-only — architecture rule 8);
   OpenAPI; regenerate `packages/api-client`.
4. **UI**: Remote access page — profile wizard (auth method → pools → split tunnel → users/RADIUS), connected-users grid with disconnect,
   client configuration hints per OS; en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/ra-vpn.md` — EAP-MSCHAPv2 and EAP-TLS examples, Windows/macOS/strongSwan client steps, CLI equivalent.
Files you own (the envelope's list wins): new files `apps/agent/internal/renderers/strongswan/{ra*.go,templates/vrx-ra*.tmpl,testdata/ra-*}`
(named hunks in P11's renderer files and an appended section in `docs/agent/renderers/strongswan.md`), `apps/agent/internal/{desired,subsystems}/ra_vpn*.go`,
`apps/agent/internal/agent/rpc_ra_vpn*.go`, `apps/agent/internal/actions/ra-vpn/**`, `apps/api/src/features/ra-vpn/**`, `apps/web/src/domains/vpn/ra-vpn/**`,
`apps/web/src/locales/*/ra-vpn.json`, `docs/user/vpn/ra-vpn.md`, `test/topology/ra-vpn/**`.
Shared files: one-line appends only; keep every existing P11 S2S test green (you now own the renderer, you do not change S2S behaviour).

## Acceptance (paste the evidence)
- [ ] Packet-level (path recorded: `af_packet` rig): a strongSwan client in `ns-<p>-wan` connects with EAP-MSCHAPv2, gets a pool address,
      pings a host behind `ns-<p>-lan`; `tcpdump` on the inter-namespace veth shows **ESP only**; `vppctl show ipsec sa` counters increase
- [ ] EAP-TLS client with a cert issued by the F-pki internal CA connects; a revoked/foreign cert is refused (charon log excerpt)
- [ ] EAP-RADIUS: rendered config + golden files only (no RADIUS server on the host, no package installs) — say so
- [ ] Agent-restart simulation → connections reloaded within 30 s, clients can reconnect without API calls (log excerpt)
- [ ] Rollback unloads the RA connection and pools (VICI `list-conns`/`get-pools` output, not assumption)
- [ ] Overlapping pools → 400 problem+json with a `pointer` to the second pool
- [ ] No EAP password / RADIUS secret in logs, GET, rendered world-readable files, fixtures, status (grep); `tools/ci.sh --base main` green

## Out of scope (do not build)
OpenVPN / SSL-VPN (WBS "optional" — not in this build); site-to-site tunnels and the base renderer pattern (P11); native IKEv2 responder
(F-ikev2-native); CA/CSR/cert issuance (F-pki); RADIUS/TACACS for **admin** login (F-aaa); per-user firewall policy (F-acl); client
software packaging; IKEv1/XAuth; HA SA sync (F-ha-state-sync); tunnel dashboards (F-dashboard-prom-alarms).

## Open questions to surface, not to decide silently
Whether pool routes in VPP come from kernel-vpp or must be programmed by the agent (verify on the rig; if the agent must, it becomes the
single programmer). Local EAP users: stored in the config document as refs (as modelled) vs the API user table — keep the model.
