# D-234: native IPsec peer certificate trust and global local key

- raised: 2026-10-04 by F-ikev2-native/P11 integration
- decision: owner approved option 1 in chat on 2026-10-04; implement explicit peer leaf pinning and one shared local key/certificate identity. Certificate configurations remain fail-closed until the reviewed implementation is merged.
- affected tasks: F-ikev2-native, P11 (current route-based scope); development resumed.

## Context

DEC-ipsec-route-based supersedes the old strongSwan build requirements. The native PSK module and SA events are implemented. Current certificate contract names a local certificate and optional remoteCa. The pinned VPP plugin verifies authentication with an explicitly supplied peer leaf certificate; it does not implement remoteCa chain verification. Its local private key is a plugin-wide singleton, shared by profiles. Enabling the current certificate shape without an explicit peer trust model would change its meaning.

## Concrete alternatives for review

1. Add an explicit peer certificate reference/pin beside the existing local certificate and remoteCa fields. Provision the full peer certificate required by VPP and verify its expected identity before installation. Keep CA semantics explicit: reject unsupported CA-only trust; never reinterpret remoteCa as a leaf. Restrict native profiles to one shared local private key/certificate identity and reject conflicting keys before any VPP write. References travel through the existing sealed cache; private values never enter desired state, events or logs. Implement provisioning, ownership, rollback/restart and secret-rotation tests after approval.
2. Retain CA-chain-based per-tunnel certificate semantics. This needs additional VPP trust/global-key architecture rather than enabling the current plugin API under the existing contract. Keep certificate configurations rejected meanwhile.

Both alternatives preserve working native PSK. Approval does not itself enable a peer pin or global-key write; implementation and verification are required. Cost estimates require the selected trust/identity boundary; the global limitation is a source constraint, not a deferred lab check.

## Recommendation and continuing work

Option 1 is approved by the owner: «موافقم برای توسعه سریع تر این کار روبکن و مشکل اتصال گواهی رو حل کن». Implement peer leaf pinning, explicit identity checks, one shared local identity, conflict rejection before writes and sealed-reference provisioning; do not claim CA-chain verification. Routing/tunnel/events, reviewed HA/WAN, isolated VPP artifact compilation and packaging checks continue. The owner answer satisfies decision-policy item 4 for this bounded trust/key boundary; unrelated host privileges and installation remain unchanged.

## Exact runtime trust semantics

The pinned VPP implementation verifies IKE AUTH using the public key extracted from the explicitly configured peer leaf certificate and checks the configured IKE identity. It does not compare an on-wire certificate to the configured DER, present the local leaf certificate, or validate an on-wire chain. A reissued peer certificate using the same public key remains compatible. Local validation checks the configured leaf validity/identity and the local RSA key/certificate pair; this is peer-certificate **public-key** pinning, not CA-chain or exact-certificate-byte verification. Interoperability peers must be configured to trust the local public key independently.
