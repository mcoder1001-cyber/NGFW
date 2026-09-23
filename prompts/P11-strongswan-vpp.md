# Task P11 — strongSwan with VPP data plane + IPsec site-to-site   (prepend 00-CONTEXT.md)

## Goal
IPsec site-to-site tunnels where **IKE runs in strongSwan and ESP runs in VPP**, driven
declaratively by vrx-agent. This is the first task that adds a GPL daemon renderer — get
the pattern right, FRR (P12) copies it.

## Read first
Intel "VPP-SSwan and Linux-CP" tech guide, VPP 26.06 `ipsec` docs,
strongSwan `kernel-vpp` plugin README (in the VPP repo under `extras/strongswan/vpp_sswan`).

## Build exactly this
1. **Build**: `deploy/debian/vrx-strongswan/` (one package, used by the lab and the product) building strongSwan 5.9.x with `--enable-kernel-vpp --enable-socket-vpp
   --disable-kernel-netlink --enable-swanctl --enable-vici --enable-eap-*` against
   `vpp-dev` 26.06. Package name `vrx-strongswan`, `Provides: strongswan`. Patches, if any,
   live in `deploy/strongswan/patches/` (they are GPL — document this in `NOTICE`).
2. **Schema/proto** (this task IS allowed to open a `contract` PR, separately, first):
   `vpn.ipsec.proposals{name → ike{encr,integ,dh}, esp{encr,integ,dh?}}`,
   `vpn.ipsec.tunnels{name → localAddr, remoteAddr, localId, remoteId, auth{psk(secretRef)|cert},
   proposal, localTs[], remoteTs[], ikeVersion 1|2, dpd, natT, mode tunnel|transport,
   rekey, startAction, vrf, routeBased{ipipInterface} }`. PSKs are `secretRef`s, never inline.
3. **Agent renderer pattern** (`internal/renderers/strongswan`): desired state →
   `/etc/swanctl/conf.d/vrx.conf` + `secrets` (0600) rendered from templates with strict
   escaping; validate with `swanctl --load-all --noprompt` in dry mode when possible;
   apply with `swanctl --load-all` (via VICI socket, **not** shell); `Retrieve` reads
   `swanctl --list-sas` JSON via VICI and VPP `ipsec_sa_dump`/`ipsec_spd_dump` from binapi.
   Route-based tunnels: create the VPP `ipip` tunnel + protect it (`ipsec_tunnel_protect_update`)
   so traffic steering is by route, not SPD policies. Events: SA up/down → `StreamEvents`.
4. **API**: config via pointer routes; `GET /api/v1/state/ipsec/sas` (SPI, cipher, bytes,
   rekey time, child SAs), `GET /api/v1/state/ipsec/tunnels`. Secrets: `POST /api/v1/secrets`
   returns a ref; GET never returns material.
5. **UI**: Tunnel list with status chips, wizard (site-to-site PSK, IKEv2, route-based) +
   advanced SchemaForm; SA inspection drawer; en+fa.
6. **Topology test**: VRX-A ↔ VRX-B (`tools/lab up tri`) + hosts behind each; commit tunnel on
   both; ping host-to-host; `tcpdump` on the inter-VRX link shows **ESP only**; kill vpp on A →
   tunnel re-established without API calls; rollback removes SAs (Retrieve). Second test:
   VRX ↔ the `peer-sswan` VM (stock strongSwan, kernel XFRM) to prove interop.

## Acceptance
- [ ] ESP visible on the wire, plaintext not; `vppctl show ipsec sa` shows byte counters increasing
- [ ] `grep -rn "exec.Command" apps/agent/internal/renderers/strongswan` is empty (VICI only)
- [ ] PSK never appears in logs, API responses, or the rendered file's world-readable parts

## Out of scope
Certificates/PKI UI (F-pki-basic), remote-access/EAP, IKEv1 (allowed in schema, not tested), QAT.
