# F-ra-vpn WIP
Branch codex/ready-f-ra-vpn-20261005, base7b507db5, remote checkpoint pending.
Completed: read current native-only route-based decision and obsolete RA prompt; inspected pinned VPP source/generated APIs. Auth methods only RSA_SIG and SHARED_KEY_MIC, no EAP/profile pools/virtual-IP API; NGFW native projection only fixed peers. Existing remoteAccess branch merely emits unsupported warning, allowing silent no-op risk.
Remaining: enabled-profile failclosed schema+agent; authenticated honest capability API/UI; disabled draft handling; exact blocker/options PENDING; tests, unchanged quick gate, PR and independent review. Full RA transport/auth/pool/session functionality cannot be delivered on the approved engine today.
Owned: current ready envelope; no host state changes.
Actual tests: none for RA yet. Current failure: missing approved native EAP/address-assignment path; engineering/source blocker, not deferred laboratory acceptance.
Next command: implement semantic/ra-vpn.ts and desired/ra_vpn.go failclosed gates.

2026-10-05 owner decision: independent strongSwan RA engine authorized; DEC-independent-ra-vpn replaces pending proposal. Boundary: per-profile private namespace/charon/XFRM plus outer/inner VPP TAPs, explicit transit addresses and selected VRF/policy handoff; native S2S unaffected. Existing enabled-profile refusal remains until operational engine verified. Next command: commit additive transit contract and tests, then implement secure renderer/runtime descriptors and RPCs. RA is not delivered.
