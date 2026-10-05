# DEC: independent strongSwan remote-access VPN

- raised:2026-10-05 by F-ra-vpn
- decision: **approved product owner, 2026-10-05**
- implementation not yet delivered: operational IKEv2/EAP road-warrior authentication, client-address pools/DNS/split routes, sessions/disconnect and packet acceptance. This is a source/architecture gap, not a lab-only gate.

DEC-ipsec-route-based requires VPP-native IKEv2 for the assigned site-to-site route-based module and excludes the retired NGFW strongSwan/kernel-vpp path from that assignment. It explicitly says: "Remote access/EAP, HA SA sync and crypto-offload tuning are separate work and are not expanded by this decision." It does not by itself choose a remote-access engine. For this recovery run the manager explicitly directs native-only development and no revival of the retired daemon/engine without separate architecture/security authorization; the old RA prompt assumes the now-retired P11 kernel-vpp dependency. The pinned VPP26.06 source declares only RSA_SIG and SHARED_KEY_MIC auth methods (`src/plugins/ikev2/ikev2.h`, foreach_ikev2_auth_method); generated `apps/agent/binapi/ikev2` exposes profile authentication, IDs, transforms, selectors, tunnel binding and SA actions, with no EAP method/credential-server or client-pool/virtual-IP configuration RPC. EAP_ONLY_AUTHENTICATION notify enumeration is not an implemented EAP server API. NGFW native projection explicitly requires a fixed IPv4 peer, bound point-to-point IPIP tunnel and PSK; certificate/EAP road-warrior session/address-negotiation machinery is absent. No agent configuration-only change can provide the required EAP-MSCHAPv2/EAP-TLS/EAP-RADIUS server and per-client pool assignment on that contract.

| Option | Work/cost now | Reversal / risks |
| --- | --- | --- |
| Keep native-only; park operational RA until approved engine exposes EAP/virtual-IP/pool/session support | Implement explicit failclosed validation and capability UI/API now; engine work separately scoped, effort unknown until design | Preserves current security/ownership model; RA unavailable and must be reported incomplete |
| Separately authorize an independently designed strongSwan route-based RA gateway/engine | Architecture/security/licensing/build/package review, tunnel/route/SA ownership design and full interoperable packet/rollback acceptance before activation | Crosses current engine/security boundary; cannot reuse unreviewed retired kernel-vpp behavior or silently activate a daemon |

Recommendation: keep native-only and explicitly park operational RA pending an engine capability decision. Continue compatible schema validation and an authenticated capability page/API that prevents misleading enabled-profile commits. Disabled profiles may remain inactive editable configuration drafts, never a claim of a listener or connected users. Existing native site-to-site IPsec and the other ready tasks continue.

The old strongSwan prompt is not authority to revive the retired dataplane. No new VPP C, daemon/package, privilege boundary or operational EAP implementation is introduced by the bounded compatibility work. The product owner can review this concrete proposal without delaying unrelated development.


## Owner authorization and resulting boundary

Owner answer, relayed by the manager on 2026-10-05: «طراحی و پیاده‌سازی موتور مستقل strongSwan برای VPN دسترسی از راه دور، با بازبینی امنیتی و تست کامل».
This authorizes a separately designed strongSwan RA engine and its security
boundary. It does not restore the retired kernel-vpp implementation or change
native VPP site-to-site ownership. The earlier native-only recommendation above
records the proposal history; the selected option is the independent engine.

Each enabled profile owns one Linux network namespace, a private charon daemon,
root-only VICI socket and credentials, XFRM interface/SA/policies, and two VPP
TAP-v2 links. The outer TAP is in the selected underlay VRF and carries IKE/ESP;
the inner TAP is in the selected protected VRF and carries decrypted traffic.
Explicit transit address pairs are part of the additive configuration contract;
no hidden allocation or reuse of an operator interface is allowed. The public
localAddr is a dedicated routed /32 in the namespace and must not overlap an
existing interface address or native site-to-site listener. VPP owns the route
to that endpoint and every client-pool route through the corresponding TAP.
Linux routes protected split destinations only through the inner TAP and routes
client pools only through the owned XFRM interface. No namespace NIC or alternate
route reaches protected LANs directly. Reply traffic traverses the selected VPP
VRF/policy before returning through XFRM. TAP ACL attachment remains explicit;
RA never installs an implicit allow rule. Native S2S configurations and SAs are
outside this engine's ownership and are never unloaded or flushed.

Daemon configuration admits only the required plugins; kernel-netlink is scoped
to the private namespace. Secret references resolve through the existing sealed
API-to-agent channel; credentials are root-only and never appear in state/logs.
Session IDs identify only this engine's owned connection/unique IKE SA; a
terminate request must prove membership before invoking VICI. Restart recovers
owned daemon/configuration without mutating foreign namespaces or sockets.
Rollback unloads the profile, terminates its SAs, removes owned routes/TAPs and
private files, and verifies readback. Tests use disposable namespaces and daemon
roots; no shared sysctl, host charon/service/package, or shared VPP mutation.

Operational acceptance must cover EAP-MSCHAPv2 and EAP-TLS authentication,
foreign/revoked certificate rejection, client pool/DNS/split routes, ESP-only
outer captures, protected VRF forwarding/replies, ACL denial, lifecycle/restart,
owned rollback and session disconnect. RADIUS renderer tests are distinct from
live RADIUS interoperability. The old requirement for VPP crypto-SA counters is
superseded: this design decrypts in the isolated Linux XFRM engine; verify its SA
counters plus VPP transit-interface counters and packet captures instead.
Until runtime and acceptance pass, enabled profiles remain failclosed and the
capability endpoint must honestly report unavailable. Owner authorization is not
an operational completion claim.


Private daemon privilege split: the agent creates its private namespace binding
and owned VPP topology. A constrained `ngfw-ra@<hashed-instance>` unit enters
that binding via `NetworkNamespacePath`; charon receives only CAP_NET_ADMIN,
CAP_NET_BIND_SERVICE and CAP_IPC_LOCK. It receives no CAP_SYS_ADMIN. The fixed
helper verifies the namespace inode against its root-owned manifest and rejects
the host namespace before setting namespace-only forwarding sysctls and routes.
The agent's ProtectKernelTunables remains enabled. Private daemon socket mode
is0600, verified with peer credentials and the managed daemon PID before VICI.

Outer IKE sockets carry mark1; CHILD ESP packets use set_mark_out1. A namespace
rule directs only that mark to an owned outer routing table/default through the
underlay TAP, while unmarked decrypted traffic uses protected routes through the
inner TAP. Client-pool reply routes use the XFRM interface, dropping without a
matching SA. Numeric RADIUS server host routes use the outer TAP explicitly.
This prevents full-tunnel inner defaults from routing IKE/ESP back into the
protected path and does not add a host or VPP policy exemption.
