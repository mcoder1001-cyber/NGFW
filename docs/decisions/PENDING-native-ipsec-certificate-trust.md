# PENDING: native IPsec peer certificate trust and global local key

- raised: 2026-10-04 by F-ikev2-native/P11 integration
- decision: pending owner answer; native certificate authentication remains fail-closed
- parked tasks: F-ikev2-native, P11 (current route-based scope)

## Context

DEC-ipsec-route-based supersedes the old strongSwan build requirements. The native PSK module and SA events are implemented. Current certificate contract names a local certificate and optional remoteCa. The pinned VPP plugin verifies authentication with an explicitly supplied peer leaf certificate; it does not implement remoteCa chain verification. Its local private key is a plugin-wide singleton, shared by profiles. Enabling the current certificate shape without an explicit peer trust model would change its meaning.

## Concrete alternatives for review

1. Add an explicit peer certificate reference/pin beside the existing local certificate and remoteCa fields. Provision the full peer certificate required by VPP and verify its expected identity before installation. Keep CA semantics explicit: reject unsupported CA-only trust; never reinterpret remoteCa as a leaf. Restrict native profiles to one shared local private key/certificate identity and reject conflicting keys before any VPP write. References travel through the existing sealed cache; private values never enter desired state, events or logs. Implement provisioning, ownership, rollback/restart and secret-rotation tests after approval.
2. Retain CA-chain-based per-tunnel certificate semantics. This needs additional VPP trust/global-key architecture rather than enabling the current plugin API under the existing contract. Keep certificate configurations rejected meanwhile.

Both alternatives preserve working native PSK. No peer pin or global-key write has been enabled. Cost estimates require the selected trust/identity boundary; the global limitation is a source constraint, not a deferred lab check.

## Recommendation and continuing work

Option1 is the bounded available plugin implementation, subject to explicit owner acceptance of peer pinning and shared local key identity. The owner question is pending. Routing/tunnel/events, reviewed HA/WAN, isolated VPP artifact compilation and packaging checks continue. Decision policy always-PENDING item4 applies to this new trust/secret boundary.
