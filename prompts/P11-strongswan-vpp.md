# Task P11 — strongSwan with VPP data plane + IPsec site-to-site   (prepend 00-CONTEXT.md)

## Goal
IPsec site-to-site tunnels where **IKE runs in strongSwan and ESP runs in VPP**, driven
declaratively by vrx-agent. The GPL daemon renderers already exist (RF-1…RF-4, incl. RF-2 strongSwan); this task builds the
vrx-strongswan package with the kernel-vpp plugin and wires RF-2 + DF-5 into the agent end to end. P12 runs in parallel.

## Read first
Intel "VPP-SSwan and Linux-CP" tech guide, VPP 26.06 `ipsec` docs,
strongSwan `kernel-vpp` plugin README (in the VPP repo under `extras/strongswan/vpp_sswan`).

## Already built — use, do not rebuild (prep-waveA check, 2026-09-24, main + task/P08)
- **RF-2** `apps/agent/internal/renderers/strongswan/` + `docs/agent/renderers/strongswan.md` + README: swanctl.conf / secrets rendering with
  strict escaping, VICI load/unload (no shell), owner-prefix scoping on a shared charon (D-089), restart detection
  (`State.Restarted`, `AckRestart`), `DaemonConfig.Plugins` (you pass the kernel-vpp list), the `swantest` harness (stock debs unpacked
  under `/run/vrx-test/<p>/`, D-083). Its Retrieve returns `structpb` until you add the IPsec state message.
- **DF-5** `apps/agent/internal/descriptors/{ipsec,vpn}/` + `docs/agent/descriptors/ipsec.md`: `ipsec.spd`, `.spd-interface`, `.spd-entry`,
  `.sa`, `.tunnel-protect`, `.itf`, globals `backend`/`async-mode`, `ipsec.NewCharonSweeper` (D-096: charon id range, stop → sweep → start →
  `AckRestart`), keyed secret references (`hmac:`), `vpn.Resolver`. DF-5 Q13 / D-089: `AckRestart(ctx, since)` must ack only the charon start
  time you observed.
- **P02c** schema `packages/schema/src/domains/vpn.ts` (`vpn.ipsec.{settings,proposals,tunnels}` incl. `engine`, `underlayVrf`,
  `routeBased.ipipInterface` → `tunnels.ipip`) + `semantic/vpn.ts` rules; proto `IpsecConfig`/`IpsecTunnel`/`IpsecProposal`.
- **P08** `apps/agent/internal/subsystems/`: `Wiring.IPsecOptions()` (persisted BootStore, D-096 keyer, globals flag) — "P11 adds the secret
  resolver and the id range when it registers the family"; builders in `internal/desired/`, hook in `agent/projection.go`. No renderer is
  called by the agent yet and no API→agent secret channel exists (see the envelope).
- The kernel-vpp plugin is **policy-based only** (SPD/SA/routes via the VPP C API; no `ipsec_tunnel_protect`/`if_id`), expects IKE to reach
  charon through a linux-cp pair (`linux_cp`/`linux_nl` are loaded since D-060; DF-8 `lcp.itf-pair` descriptor), and was tested upstream
  only with strongSwan 5.9.5/5.9.6.

## Build exactly this
1. **Build**: `deploy/debian/vrx-strongswan/` (one package, used by the lab and the product) building strongSwan (5.9.x — the version
   `vpp_sswan` was tested with; say so if you pick 6.x) with `--enable-swanctl --enable-vici --enable-systemd --enable-openssl
   --enable-eap-*` plus the **out-of-tree** kernel-vpp plugin `libstrongswan-kernel-vpp.so` compiled from a copy of
   `/root/vpp/extras/strongswan/vpp_sswan` (there is no `--enable-kernel-vpp` configure switch; never run its Makefile in place — it
   downloads into and writes under `/root/vpp/build-root` and installs into `/usr`), against VPP 26.06 headers/libs **from a staging sysroot**:
   `dpkg -x /root/vpp/build-root/vpp-dev_*.deb $WORK/sysroot` (+ `libvppinfra-dev`), `CPPFLAGS=-I$WORK/sysroot/usr/include`,
   `LDFLAGS=-L$WORK/sysroot/usr/lib/x86_64-linux-gnu`. **Never `dpkg -i vpp-dev` on this host while handover is pending.** The resulting
   `vrx-strongswan` .deb is **not installed** on this host (D-083: no package installs; strongSwan is not installed at all) — tests run
   the built charon from its unpacked tree under `/run/vrx-test/<p>/` (the `swantest` pattern). Package name `vrx-strongswan`,
   `Provides: strongswan`. Build inputs on the host: `libsystemd-dev`, `libssl-dev`, autotools present; `bison`/`flex`/`gperf` absent → use
   the release tarball (pre-generated parsers, pinned sha256, fetched into the build dir outside git), not the GitHub archive the
   `vpp_sswan` Makefile pulls. Artefacts stay outside git (F-vpp-debs pattern). Patches, if any, live in `deploy/strongswan/patches/`
   (they are GPL — document this in `NOTICE`).
2. **Schema/proto**: the config model **exists** (P02c, see above). Open a `contract/P11` branch only for additions: tunnel/profile names
   `max(60)` (D-089), the IPsec state message + SA up/down event kind (RF-2 Q, D-083), and whatever the secret channel needs if the manager
   assigns it to you. PSKs are `secretRef`s, never inline. Numbers come from the manager, never "next free".
3. **Agent wiring** (the renderer and the descriptors exist — RF-2, DF-5): builder `internal/desired/ipsec*.go` (`vpn.ipsec` → RF-2
   renderer input + DF-5 objects where VPP state is ours), charon lifecycle (plugin list = kernel-vpp, charon id range disjoint from the
   descriptors', D-096 order stop → sweep → start → `AckRestart(since)`), `Retrieve` = RF-2 `list-sas` via VICI + DF-5 dumps.
   Route-based tunnels: the upstream kernel-vpp plugin cannot protect an IPIP interface — either a strongSwan-side patch in
   `deploy/strongswan/patches/` or the tunnel stays policy-based in this task; `routeBased.ipipInterface` names an IPIP tunnel from
   `tunnels.ipip` (projected by F-tunnels). Decide with options in the questions file, do not decide silently. Events: SA up/down →
   `StreamEvents`.
4. **API**: config via pointer routes; `GET /api/v1/state/ipsec/sas` (SPI, cipher, bytes,
   rekey time, child SAs), `GET /api/v1/state/ipsec/tunnels`. Secrets: `POST /api/v1/secrets`
   returns a ref; GET never returns material.
5. **UI**: Tunnel list with status chips, wizard (site-to-site PSK, IKEv2, route-based) +
   advanced SchemaForm; SA inspection drawer; en+fa.
6. **Topology test** (single host, `VRX_INTEGRATION=1`, shared lock): VRX (this host's VPP + your agent) ↔ a **stock strongSwan (charon)
   running inside a network namespace** on the veth rig with kernel XFRM (IKE/NAT-T on the slot's ports, never 500/4500: VPP's ikev2
   plugin claims 500/4500 inside VPP as soon as any IKEv2 profile exists — DF-5 port scheme `20000+100·slot+…`); commit the tunnel; ping from a second namespace behind VPP to a
   host behind the peer namespace; `tcpdump` on the inter-namespace veth shows **ESP only**; agent-restart simulation → SAs re-established
   without API calls; rollback removes SAs (Retrieve). The VRX-A↔VRX-B `tri` test is deferred to INTEGRATE-E2E when VMs exist.

## Acceptance
- [ ] ESP visible on the wire, plaintext not; `vppctl show ipsec sa` shows byte counters increasing
- [ ] `grep -rn "exec.Command" apps/agent/internal/renderers/strongswan` shows only the allow-listed fixed-argv runner (VICI for load/state)
- [ ] PSK never appears in logs, API responses, or the rendered file's world-readable parts

## Out of scope
Certificates/PKI UI (F-pki), remote-access/EAP, IKEv1 (allowed in schema, not tested), QAT. Use, do not rebuild: RF-2 renderer, DF-5
descriptors/sweeper, P02c schema, P08 wiring. WireGuard (F-wireguard), VPP-native IKEv2 (F-ikev2-native), IPIP tunnel creation
(`tunnels.ipip`, F-tunnels), the linux-cp pair model in the config (P12 — a test fixture pair via DF-8's descriptor is fine).
