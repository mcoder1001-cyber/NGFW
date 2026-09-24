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
//	hmac:<hex>            symmetric material (SA keys, IKEv2 PSK, WireGuard preshared key):
//	                      HMAC-SHA256 under the agent-local fingerprint key (D-096, Keyer)
//	x25519:<base64 pub>   a WireGuard private key, referenced by its public key (a public value)
//
// The fingerprint key is a 0600 file of 32 random bytes in the agent's state dir, created on first
// use (LoadOrCreateKeyFile) and passed to every package with WithKeyer; it is never logged and the
// Keyer formats as "vpn.Keyer(hmac-sha256)". A weak PSK therefore cannot be guessed offline from
// desired state, plans, logs or test output. The key file is written crash-safe (temp file,
// fsync, link, directory fsync) and read without following symlinks. Unkeyed "sha256:"
// fingerprints are refused like any other malformed reference (D-096).
//
// A value where a reference belongs is checked against the reference grammar (CheckRef) before
// anything else; a malformed one — typically pasted plaintext — fails with a bare ErrBadRef and
// is never echoed. Redact is the only form in which references reach errors: anything that is
// not a well-formed reference becomes "<redacted>".
//
// Create resolves the reference through a Resolver (the agent's secret store; tests use
// MapResolver) and verifies that the material matches the reference (Resolve). Retrieve rebuilds
// the reference from what VPP exposes — VPP returns SA keys (ipsec_sa_v5_dump), the IKEv2 PSK
// (ikev2_profile_dump) and the WireGuard preshared key (wireguard_peers_v2_dump) in its dumps,
// and the WireGuard public key (wireguard_interface_dump) — keys it and zeroes the buffer. The
// scheduler's proto.Equal diff therefore works on secrets without ever seeing them, applying the
// same desired state twice yields an empty plan, and the comparison survives an agent restart.
// A key change produces a different reference and is handled as ErrRecreate (SAs, WireGuard) or an
// in-place setter re-issue (IKEv2 PSK).
//
// The binapi request and details messages that carry material (ipsec_sad_entry_add_v2,
// ipsec_sa_v5_details, ikev2_profile_set_auth, ikev2_profile_details, wireguard_interface_create,
// wireguard_peer_add_v2, wireguard_peers_v2_details) are never formatted with %v or passed to a
// logger by these packages; errors name the message, not its content. The packages zero every
// buffer they own; known limitation (review L1): govpp's own encode/decode buffers of those
// messages are not zeroed (outside DF-5's code — govpp item).
package vpn
