# P11 follow-up — route-based IPsec only

## Current scope

The product owner changed the assignment on 2026-10-03: **implement only the route-based IPsec module**. Read [DEC-ipsec-route-based](../docs/decisions/DEC-ipsec-route-based.md), then [F-ikev2-native](features/F-ikev2-native.md). This replaces the former strongSwan/kernel-vpp build and policy-based site-to-site instructions in this prompt.

Use VPP native IKEv2 (`engine: vpp-ikev2`) with a protected tunnel interface and routes through that interface. Reuse the existing IKEv2/IPsec/tunnel descriptors. Complete declarative projection, validation, secret resolution, state, API/CLI/UI, rekey and restart/rollback behavior. Negotiated SAs belong to the IKE plugin; avoid conflicting agent-owned SAs.

No policy-based fallback, transport-mode VPN, IKEv1 or NGFW-side strongSwan plugin packaging is accepted for this assignment. StrongSwan is permitted as a test peer. Historical P11 implementation/review records remain evidence of the old scope, not instructions to implement it again.

## Acceptance

Follow the route-based packet, FIB, SA counter, route withdrawal, rekey, restart, peer recovery, rollback and secret checks in the decision and `test/topology/ipsec/README.md`. Use a disposable VPP and slot-scoped peers. Record unresolved contract/secret-channel requirements explicitly; do not claim completed forwarding from fake-client or configuration-only tests.
