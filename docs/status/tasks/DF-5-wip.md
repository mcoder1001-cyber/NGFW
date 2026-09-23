# DF-5 — WIP log (descriptors: ipsec, ikev2, wireguard)

- 2026-09-23 read: 00-CONTEXT, WORKER-OPS, DF-5.md, shared-host-rules, host-vrx-a, LOG D-012/014/030/038, envelope,
  descriptors/README, scheduler/descriptor.go, vpp/{client,tag,fake,vpptest}, binapi ipsec/ipsec_types/ikev2/ikev2_types/
  wireguard/tunnel_types/ipip/interface/ip_types. Host: VPP 26.06 on /run/vpp/api.sock, ikev2 + wireguard plugins loaded,
  ipsec core, no workers, protoc + protoc-gen-go v1.36.12 present.
- VPP semantics verified read-only in /root/vpp/src (no edits): ipsec_spd_interface_details carries the SPD *pool index*,
  not the spd_id (limitation, see questions); ipsec_sa_v5_dump returns key material; ikev2_profile_dump returns the PSK;
  wireguard_peers_v2_dump returns the preshared key; wireguard_interface_dump returns the private key only with
  show_private_key (never used). These drive the secret-reference design.
- Plan: shared `descriptors/vpn` (proto types, secret refs + resolver, address/interface helpers), then ipsec (7 objects),
  ikev2 (3 + state helper), wireguard (3 + events), unit tests on the fake, one integration check per object type, docs.
- 2026-09-24 CONTINUE (respawned on host): salvaged ipsec/ (cc002dc) checked — builds, vet clean, unit tests green,
  TestIpsecOnHost green against host VPP with prefix w4 (all 8 descriptors; backend dump empty on 26.06, async skipped).
  ipsec = done. Next: ikev2 (profile, local-key, sleep-interval, + liveness singleton, SA state helper), wireguard.
- ikev2 VPP findings (read-only /root/vpp/src/plugins/ikev2): transform/id/auth fields are raw u8 in binapi, value
  tables come from ikev2.h foreach_* macros (IANA numbers); ikev2_profile_dump returns the PSK (auth.data) and the
  rsa-sig cert path (+NUL); responder hostname is not dumped; ipsec_udp_port can only be set when unset; udp_encap /
  natt_disabled are set-only; liveness is global (period, max_retries), no getter; profile add registers VPP-internal
  UDP 500/4500 (refcounted, VPP stack only — not a host socket); govpp decodes id.data (string[64]) up to the first NUL,
  so ip4/ip6 ids containing a zero byte are truncated in the dump.
- 2026-09-24 ikev2 done: profile (composite, changed-setters-only Update, ErrRecreate on un-settable removals),
  local-key / liveness (last-applied cache), sleep-interval (ikev2_get_sleep_interval), SA state helper
  (ikev2_sa_v3_dump + ikev2_child_sa_v2_dump, derived keys zeroed), action helpers. Liveness added as its own
  singleton (message has no profile name). Unit tests green (15), TestIkev2OnHost green on host (w4).
  Next: wireguard.
