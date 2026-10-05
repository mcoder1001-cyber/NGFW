# F-ra-vpn WIP
Branch codex/ready-f-ra-vpn-20261005, base7b507db5, remote checkpoint pending.
Completed: read current native-only route-based decision and obsolete RA prompt; inspected pinned VPP source/generated APIs. Auth methods only RSA_SIG and SHARED_KEY_MIC, no EAP/profile pools/virtual-IP API; NGFW native projection only fixed peers. Existing remoteAccess branch merely emits unsupported warning, allowing silent no-op risk.
Remaining: enabled-profile failclosed schema+agent; authenticated honest capability API/UI; disabled draft handling; exact blocker/options PENDING; tests, unchanged quick gate, PR and independent review. Full RA transport/auth/pool/session functionality cannot be delivered on the approved engine today.
Owned: current ready envelope; no host state changes.
Actual tests: none for RA yet. Current failure: missing approved native EAP/address-assignment path; engineering/source blocker, not deferred laboratory acceptance.
Next command: implement semantic/ra-vpn.ts and desired/ra_vpn.go failclosed gates.

2026-10-05 owner decision: independent strongSwan RA engine authorized; DEC-independent-ra-vpn replaces pending proposal. Boundary: per-profile private namespace/charon/XFRM plus outer/inner VPP TAPs, explicit transit addresses and selected VRF/policy handoff; native S2S unaffected. Existing enabled-profile refusal remains until operational engine verified. Next command: commit additive transit contract and tests, then implement secure renderer/runtime descriptors and RPCs. RA is not delivered.

Contract checkpoint: RemoteAccessProfile.transport field17 explicit outer/inner point-to-point pairs plus optional IPv6 inner pair; generated Go/TS stubs. Schema validators reject wrong families/subnets/same endpoints, duplicate ownership, interface/client pool/transit overlaps, native listener collision, IPv6 pool without IPv6 handoff. Only explicitly routed transport skips historical interface-owned local-address rule. Runtime remains failclosed. Focused7 tests PASS5.75s; buf lint PASS; proto regeneration PASS. Next: implement standalone secure RA renderer+descriptor lifecycle, namespace and VPP TAP/routes, actual VICI readback/actions. API/UI drafts not complete.

Standalone private RA renderer implemented (not wired/operational): fresh
kernel-netlink plugin configuration, private VICI socket, route installation
explicitly off; IKEv2 EAP/user pools/DNS/split selectors/XFRM IDs/DPD/rekey;
strict EAP-TLS/pubkey revocation trust and bounded numeric RADIUS sources.
Sealed resolver values appear only in private file bytes; fmt/JSON serializers
redact all file content, resolver failure details never propagate. Three focused
Go renderer tests PASS0.081s. Existing S2S renderer untouched. Runtime ownership,
PKI snapshots/CRLs, namespace/TAP/routes/readback/restart/rollback/session actions,
packaging/API/UI and real packet acceptance remain to implement. Next command:
implement RA runtime descriptor and bounded VICI observation using private roots;
activation guard must remain until engine verified.

Bounded private VICI observation/action source added: exact singleton connection
ownership,1024-event buffer plus stable before/after stats to detect dropped
listings, pool VIP and XFRM ID validation, public-only identity/counters, exact
uint64 JSON strings, generation-derived opaque session IDs and owned termination
followed by readback. Unix dial checks root-only parent/socket mode and SO_PEERCRED
exact managed PID, with a deadline that bounds context-free subscription too.
Seven focused RA Go tests PASS (latest runtime listed in own disk log); includes
200-session listing beyond govici default buffer, incomplete/foreign/stale refusal
and real private Unix socket mode/PID checks. No operational engine wired yet.
Private privilege/route design documented: constrained unit uses no SYS_ADMIN;
IKE/ESP mark1 selects owned underlay routing table, decrypted traffic traverses
VPP inner VRF; full-tunnel routes cannot loop outer crypto into protected path.
Next: fixed namespace helper/unit, secure lifecycle/PKI staging, TAP/routes and
scheduler descriptor; then API/proto/UI and disposable interoperable acceptance.
