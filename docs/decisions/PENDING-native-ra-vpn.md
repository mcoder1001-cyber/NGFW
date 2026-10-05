# PENDING: native remote-access VPN capability

- raised:2026-10-05 by F-ra-vpn
- decision: **pending product owner**
- parked functionality: operational IKEv2/EAP road-warrior authentication, client-address pools/DNS/split routes, sessions/disconnect and packet acceptance. This is a source/architecture gap, not a lab-only gate.

DEC-ipsec-route-based requires VPP-native IKEv2 for the assigned site-to-site route-based module and excludes the retired NGFW strongSwan/kernel-vpp path from that assignment. It explicitly says: "Remote access/EAP, HA SA sync and crypto-offload tuning are separate work and are not expanded by this decision." It does not by itself choose a remote-access engine. For this recovery run the manager explicitly directs native-only development and no revival of the retired daemon/engine without separate architecture/security authorization; the old RA prompt assumes the now-retired P11 kernel-vpp dependency. The pinned VPP26.06 source declares only RSA_SIG and SHARED_KEY_MIC auth methods (`src/plugins/ikev2/ikev2.h`, foreach_ikev2_auth_method); generated `apps/agent/binapi/ikev2` exposes profile authentication, IDs, transforms, selectors, tunnel binding and SA actions, with no EAP method/credential-server or client-pool/virtual-IP configuration RPC. EAP_ONLY_AUTHENTICATION notify enumeration is not an implemented EAP server API. NGFW native projection explicitly requires a fixed IPv4 peer, bound point-to-point IPIP tunnel and PSK; certificate/EAP road-warrior session/address-negotiation machinery is absent. No agent configuration-only change can provide the required EAP-MSCHAPv2/EAP-TLS/EAP-RADIUS server and per-client pool assignment on that contract.

| Option | Work/cost now | Reversal / risks |
| --- | --- | --- |
| Keep native-only; park operational RA until approved engine exposes EAP/virtual-IP/pool/session support | Implement explicit failclosed validation and capability UI/API now; engine work separately scoped, effort unknown until design | Preserves current security/ownership model; RA unavailable and must be reported incomplete |
| Separately authorize an independently designed strongSwan route-based RA gateway/engine | Architecture/security/licensing/build/package review, tunnel/route/SA ownership design and full interoperable packet/rollback acceptance before activation | Crosses current engine/security boundary; cannot reuse unreviewed retired kernel-vpp behavior or silently activate a daemon |

Recommendation: keep native-only and explicitly park operational RA pending an engine capability decision. Continue compatible schema validation and an authenticated capability page/API that prevents misleading enabled-profile commits. Disabled profiles may remain inactive editable configuration drafts, never a claim of a listener or connected users. Existing native site-to-site IPsec and the other ready tasks continue.

The old strongSwan prompt is not authority to revive the retired dataplane. No new VPP C, daemon/package, privilege boundary or operational EAP implementation is introduced by the bounded compatibility work. The product owner can review this concrete proposal without delaying unrelated development.
