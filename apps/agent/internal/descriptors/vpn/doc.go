// Package vpn holds what the DF-5 descriptor packages (ipsec, ikev2, wireguard) share: the
// desired-state proto types (pb/), the secret-reference contract and its Resolver, address
// canonicalisation, the programme's cross-plugin key conventions (interface/<name>, vrf/<id>)
// and the interface table used to translate names to sw_if_index and to read owner tags.
//
// # Secrets
//
// No desired-state value, key, Meta, log line or error produced by these packages ever contains
// key material. A secret field holds a *reference* that is derived from the material:
//
//	sha256:<hex>          symmetric material (SA keys, IKEv2 PSK, WireGuard preshared key)
//	x25519:<base64 pub>   a WireGuard private key, referenced by its public key
//
// Create resolves the reference through a Resolver (the agent's secret store; tests use
// MapResolver) and verifies that the material matches the reference (Resolve). Retrieve rebuilds
// the reference from what VPP exposes — VPP returns SA keys (ipsec_sa_v5_dump), the IKEv2 PSK
// (ikev2_profile_dump) and the WireGuard preshared key (wireguard_peers_v2_dump) in its dumps,
// and the WireGuard public key (wireguard_interface_dump) — hashes it and zeroes the buffer. The
// scheduler's proto.Equal diff therefore works on secrets without ever seeing them, applying the
// same desired state twice yields an empty plan, and the comparison survives an agent restart.
// A key change produces a different reference and is handled as ErrRecreate (SAs, WireGuard) or an
// in-place setter re-issue (IKEv2 PSK).
//
// The binapi request and details messages that carry material (ipsec_sad_entry_add_v2,
// ipsec_sa_v5_details, ikev2_profile_set_auth, ikev2_profile_details, wireguard_interface_create,
// wireguard_peer_add_v2, wireguard_peers_v2_details) are never formatted with %v or passed to a
// logger by these packages; errors name the message, not its content.
package vpn
