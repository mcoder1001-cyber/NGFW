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
