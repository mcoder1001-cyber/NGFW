# Task: F-ikev2-native — VPP native IKEv2 responder path   (prepend 00-CONTEXT.md)

## Goal
Implement **IPsec tunnels negotiated by VPP's own IKEv2 plugin** (no strongSwan) end to end in FAST MODE: a tunnel with
`engine: "vpp-ikev2"` becomes an `ikev2.profile` (+ responder, transforms, traffic selectors, route-based tunnel interface),
the plugin negotiates the SAs, ESP runs in VPP. Responder-first (WBS note: "alternative to strongSwan for responder-only cases");
initiator = one explicit action. Reference: TNSR "IPsec (native IKE)"; VPP plugin `ikev2` + 26.06 crypto (WBS D6.3 in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/vpn.ts` — `vpn.ipsec.tunnels.<name>` with `engine: "vpp-ikev2"` already exists (IKEv2-only rule is in the
  schema; IKEv1 + GCM IKE proposals rejected, D-067), `vpn.ipsec.proposals`, `vpn.ipsec.settings{cryptoEngine, asyncCrypto}`; proto `IpsecTunnel`
- DF-5 (merged): `apps/agent/internal/descriptors/ikev2/`
  (`ikev2.profile`, write-only `ikev2.responder-hostname` / `ikev2.local-key` / `ikev2.liveness`, `ikev2.sleep-interval`, `SAs()` state helper,
  `InitiateSAInit` / `DeleteIKESA` / `DeleteChildSA` / `RekeyChildSA` actions), `descriptors/ipsec/` (`ipsec.itf`, `ipsec.tunnel-protect`),
  `descriptors/vpn/pb/vpn.proto`, `docs/agent/descriptors/{ikev2,ipsec}.md` and `docs/status/tasks/DF-5-questions.md` (Q2 id truncation, Q9)
- **P08 + P11 (merged)**: `Wiring.IKEv2Options()` in `apps/agent/internal/subsystems/subsystems.go` (persisted BootStore + D-096 keyer +
  globals flag — already tested), builders in `apps/agent/internal/desired/`, the hook in `projection.go`; P11's strongSwan builder skips
  `engine: vpp-ikev2` tunnels, owns `Domains["vpn"]`, the IpsecState RPC and SA EventKind, the IPsec tab and the stock-strongSwan unpack
  harness (D-083). The API→agent secret channel is `docs/decisions/PENDING-secret-channel.md` — use what P11 merged, else a fixture resolver
- `apps/agent/binapi/ikev2/` — the only source of names (`ikev2_profile_add_del`, `ikev2_set_responder`, `ikev2_set_ike_transforms`,
  `ikev2_set_esp_transforms`, `ikev2_set_tunnel_interface`, `ikev2_sa_v3_dump`, `ikev2_child_sa_v2_dump`, `ikev2_initiate_sa_init`)
- `docs/decisions/LOG.md` D-051 (refs `psk/<name>`, `cert/<name>`, `key/<name>`), D-063/D-076/D-080 (write-only + boot identity), D-065 (`interface/<name>`),
  D-071/D-082 (liveness, local key, sleep interval are plugin-wide → globals owner only, globals lock in tests)
- `docs/lab/shared-host-rules.md` (slot prefix, ports: ipsec-over-udp 20000+100·slot+1/+2), VPP docs https://s3-docs.fd.io/vpp/26.06/ → IKEv2

## Scope — build exactly this
1. **Schema**: semantic rules — `engine: vpp-ikev2` requires `routeBased.ipipInterface` or an `ipsec<N>` tunnel interface (native path is
   route-based only); `auth.method` psk|cert (cert needs `vpn.pki.certificates.<name>` → key path on the host, F-pki provides the file);
   ip-type ids with an inner zero octet (10.4.0.1) rejected with the DF-5 reason (govpp NUL truncation) — suggest an fqdn id; proposal
   transforms limited to what the plugin supports (map table from `ikev2.go`). Missing fields → `contract(schema|proto): …` commits on your
   task branch (additive; no `contract/` branch; numbers from `docs/status/wave-BC-numbers.md`).
2. **Agent**: projection `engine=vpp-ikev2` tunnel → `ikev2.profile/<name>` (+ `ipsec.itf`, route to the tunnel interface via core
   `ip.route`); translate D-051 refs to DF-5's keyed `hmac:` form (HMAC-SHA256 under the agent-local key, D-096 — never a plain sha256)
   through `Wiring.IKEv2Options()` and the agent secret resolver, never the plaintext into Value/logs.
   Globals (`ikev2.liveness`, `ikev2.local-key`, sleep interval) only when the agent is the globals owner (D-071). State: `SAs()` →
   IKE/child SAs with SPIs, transforms, bytes, uptime (derived keys are zeroed by DF-5 — keep it so). Actions: initiate, rekey child,
   delete IKE SA. ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, prefixed objects).
3. **API**: config via pointer routes; `GET /api/v1/state/ipsec/ikev2/sas`; `POST /api/v1/actions/ipsec/ikev2/{tunnel}/{initiate|rekey|delete-sa}`.
   OpenAPI; regenerate `packages/api-client`.
4. **UI**: the IPsec tunnel form shows an engine selector; native tunnels get a status column + SA drawer (SPI, transforms, counters,
   rekey actions); en + fa; screenshot against the real endpoint.
5. **Docs**: `docs/user/vpn/ikev2-native.md` — when to pick native vs strongSwan, example responder config, CLI equivalent, known limits.
Files you own (the envelope's list wins): `apps/agent/internal/descriptors/ikev2/**` (gap-only), `docs/agent/descriptors/ikev2.md`,
`apps/agent/internal/desired/ikev2*.go`, `apps/agent/internal/subsystems/ikev2*.go`, `apps/agent/internal/agent/rpc_ikev2*.go`,
`apps/agent/internal/actions/ikev2-native/**`, `apps/api/src/features/ikev2-native/**`, `apps/web/src/domains/vpn/ikev2-native/**`,
`apps/web/src/locales/*/ikev2-native.json`, `docs/user/vpn/ikev2-native.md`, `test/topology/ikev2-native/**`.
Shared files: one-line appends only (app.module.ts, router/nav, agent registry); `descriptors/ipsec/**` belongs to P11 — request changes via questions.

- **VPP dns crash rule (D-137/D-139/D-140, 2026-09-25):** the ikev2 plugin calls the dns plugin's resolver when `ikev2_initiate_sa_init` runs on a profile with a responder HOSTNAME, and VPP 26.06 crashes (NULL deref in ip4_sas) unless an IPv4 name server was added since VPP started. Accept a responder hostname only while the agent's `dns.Readiness` fact holds, or resolve the name in the agent and send an address; refuse otherwise at build time with a pointer. Never call dns.api directly.

## Acceptance (paste the evidence)
- [ ] Packet-level (path recorded: `af_packet` rig): a stock strongSwan initiator in `ns-<p>-wan` (debs unpacked under
      `/run/vrx-test/<p>/`, D-083 — never installed) negotiates with VPP's responder;
      ping from the peer's inner network to a host behind `ns-<p>-lan`; `tcpdump` on the inter-namespace veth shows **ESP only**;
      `vppctl show ikev2 sa` + `show ipsec sa` show the SA with counters increasing (PSK redacted from the pasted output)
- [ ] `Retrieve()` == desired for `ikev2.profile`; write-only objects re-applied once per VPP boot identity (D-076/D-080), not per resync
- [ ] Agent-restart simulation → profile back within 30 s, SA re-negotiated without API calls (log excerpt)
- [ ] Rollback removes profile, tunnel interface and SAs (Retrieve output)
- [ ] `engine: vpp-ikev2` + `ikeVersion: 1` or ip id `10.4.0.1` → 400 problem+json with a `pointer`
- [ ] PSK never in logs, GET, fixtures or status files (`grep` evidence); `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
strongSwan S2S and the swanctl renderer (P11); the ipsec SA/SPD descriptors themselves (P11/DF-5); PKI, CSR, CA (F-pki);
remote-access/EAP and client pools (F-ra-vpn); WireGuard (F-wireguard); policy-based (SPD) native tunnels; QAT/crypto offload tuning;
HA SA sync (F-ha-state-sync); tunnel dashboards (F-dashboard-prom-alarms); a full initiator UX with hostname resolution (DF-5 Q9).

## Open questions to surface, not to decide silently
IKE id truncation (DF-5 Q2) is a VPP/govpp code-track candidate — propose it as a V-item rather than working around it further.
Whether the globals owner should set liveness/sleep interval from `vpn.ipsec.settings` (needs contract fields) or keep VPP defaults.
