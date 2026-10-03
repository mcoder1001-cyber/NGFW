# IPsec scope: route-based only

Date: 2026-10-03. Authority: product owner's instruction in chat: «در مستندات ipsec رو به ماژول route base تغییر بده. فقط همین را پیاده سازی کنند».

This is the current implementation scope for IPsec. It supersedes the policy-based fallback and strongSwan/kernel-vpp build requirements in earlier P11 prompts, reviews and schedule exports. Earlier evidence remains historical; no completed implementation or test is asserted by this decision.

## Required module

Implement site-to-site **route-based IPsec**, with VPP as the ESP data plane. Use the existing `F-ikev2-native` path (`engine: vpp-ikev2`, IKEv2) as the implementation path; strongSwan may be an interoperability test peer. This engine choice follows the available native route-based task and avoids the policy-only kernel-vpp integration. The owner's explicit requirement is route-based-only.

- One logical tunnel interface is referenced by `routeBased.ipipInterface` in the existing contract. IPIP src/dst, overlay VRF and underlay VRF must agree with the IPsec configuration.
- VPP's IKEv2 profile negotiates keys and CHILD_SAs and binds the tunnel interface using the supported API. Establish inbound/outbound SA protection on that interface; do not create a second set of manually owned SAs that competes with the IKE plugin.
- Routing/FIB determines which prefixes use the protected interface. IKE traffic selectors still constrain the negotiated CHILD_SA; they do not replace routes.
- Provide tunnel address/state, routes, proposal, PSK/certificate references, SA state/counters, rekey/DPD, agent restart recovery and ordered rollback. Secrets remain references and never enter logs or state responses.
- Implement agent, validation, API, CLI and UI under one IPsec module. Reject missing route-based binding and unsupported policy/transport configurations with field pointers; do not fall back silently.

## Excluded implementation

Policy-based/SPD user configuration, transport-mode VPN, IKEv1, strongSwan/kernel-vpp/socket-vpp on the NGFW side, and the old P11-pkg build are outside this assignment. Existing generic low-level descriptors and historical strongSwan code need not be deleted. Remote access/EAP, HA SA sync and crypto-offload tuning are separate work and are not expanded by this decision.

## Acceptance

A real IKEv2 peer negotiates with VPP. Bidirectional ICMP and TCP traverse the protected interface; FIB routes and tunnel/SA counters prove the path. Underlay capture contains ESP or NAT-T-protected data and no inner plaintext (IKE/ARP/ND control traffic is expected). Route removal stops that prefix entering the tunnel. Rekey, agent restart, peer loss/recovery and rollback are exercised; rollback removes owned profiles, protection/SAs, routes and tunnel objects in dependency order. Evidence must distinguish an IKE plugin-owned SA from an agent-owned object and redact keys.

Use a disposable VPP and slot-scoped peers. Do not restart the shared system VPP or install daemon packages on the host to satisfy this module.

Implementation status is tracked in the [2026-10-03 report](../status/tasks/ipsec-prerequisites-2026-10-03.md). The native PSK path is wired and has disposable API/packet evidence; certificate provisioning and other unverified capabilities remain separate follow-up work. The shared appliance has not received the required patched VPP plugin. See [implementation prompt](../../prompts/features/F-ikev2-native.md) and [topology procedure](../../test/topology/ipsec/README.md).
