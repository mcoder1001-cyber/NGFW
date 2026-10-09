# PPPoE VLAN carrier source checkpoint

Branch: `codex/pppoe-vlan-20261008`. Base local `1b8c55df`, remote `8f6224f936662b35ec1c6b2c815aee7e5fb0c559`.

Published foundation: local `2d5963a6`, remote `0cd6e3186cb17c8dcbfdef100966297d667ded76`, tree `4c973b8f0b58c65c7976467b7783f9df99d91d42`. Follow-up adds explicit live QinQ/dot1ad and missing-inner regression coverage.

Owned changes: new descriptor/desired VLAN helpers and focused tests, semantic parent resolver and narrow PPP semantic changes, en/fa parent help. Existing runtime/projection integration files remain owned by carrier worker.

Completed foundation: explicit configured root/child resolution (canonical sub-ID), enabled/exclusive leaf, sibling isolation, one-tag and QinQ POP projection using existing scheduler-managed l2 descriptor; cloned daemon parent references; explicit VTR dependency; authoritative owned live child/tag validation and VTR readiness, raw TAP rewrite rejection. Schema accepts explicit VLAN/QinQ parents and rejects addressed selected leaf, disabled root, whole port with children. Existing UI interface-picker permits free text; en/fa help explains `eth0.100`.

Tests actually run: descriptor and desired `go test -race ... -run '^TestCarrierVLAN' -count=1` passed (1.039s / 1.061s). Schema focused existing+new PPP tests passed 15/15. Schema typecheck passed after fixes. Schema typecheck initially found synthetic-leaf missing promiscuous and optional proxyNd; both fixed. A mistaken package-script separator caused a full schema-package test run (not CI): 1685 passed, one duplicate disabled-parent assertion failed; corrected and focused 15/15 rerun passed. Aggregate/hosted CI not run. No native VPP, namespace or lab activation.

Remaining integration, owned by parent: wire helpers into full desired plan and daemon manifest/dependencies/sessions, admission and verified forwarding; extend live physical-root bond/LCP/L2 guards to VLAN root; combined graph regression and independent review. This checkpoint is not a claim that VLAN product integration is done.

## Exact caller contract

- `resolved, err := pppoedesc.ResolveCarrierParent(ifs, parentName)` before any plan emission. Use `resolved.Config` for leaf checks, `resolved.RootName` for root bond checks. Existing nil raw fixture must be explicitly Enabled true. Do not apply whole-port children rejection to `resolved.Root` for VLAN; helper already validates selected leaf.
- In carrier emit loop: resolve same validated parent then `PppoeCarrierVLAN(s, resolved, pt)`. This emits `l2.vlan-tag-rewrite/<parent>` only for explicit VLAN.
- `desired.Pppoe`: resolve parent instead of `ifs[parent]`; after storing logical PPP clone, call `pppoedesc.CopyCarrierParentReference(doc, ifs, parent)`. The helper deep-clones the root and selected child; multiple PPP children retain each selected reference, unrelated siblings excluded.
- Daemon Dependencies: `key, present, err := CarrierVLANDependency(doc, parent)`; append key only on present+nil error. Sessions/validation must reject resolver errors (Dependencies cannot return an error).
- Namespace admission: call `AdmitCarrierVLAN(ctx,c,owner,spec)` before namespace mutation. Existing raw L2/IP/LCP/bond checks remain necessary. For VLAN, extend physical-root bond/LCP/L2 checks using the live selected child's SupSwIfIndex; unrelated root children must remain allowed. Root addressing can remain independent.
- Forwarding readiness: `VerifyCarrierVLANReadiness(ctx,c,owner,spec)` after authoritative bilateral XC checks. Missing POP keeps carrier unready.
- Dependencies are subinterface alias -> namespace -> dedicated carrier TAP -> XC -> VTR -> daemon. Namespace MUST NOT depend on VTR (cycle). Helpers do not name standard tapv2 creator.

## VPP authoritative tag behavior

Reviewed upstream FDio/vpp `v26.06`, `src/vnet/l2/l2_vtr.c`, downloaded from https://raw.githubusercontent.com/FDio/vpp/v26.06/src/vnet/l2/l2_vtr.c . SHA256 `303ae4d256f4620323657dabc8aca7fa66e3d7a03ce66f95a69c6d5bb192236e`.

Input POP_1/POP_2 removes one/two tags. Lines 292–321 derive the symmetric output push and its dot1ad/outer/inner tags from the subinterface classification. No PUSH is configured on the raw TAP. `l2vtr_get` initializes push/tag arguments to zero for POP; desired POP therefore keeps PushDot1Q=false and Tag1/Tag2=0 so readback is stable. This proves source/API semantics, not physical packet delivery. Native PPP discovery/session packets over VLAN and QinQ remain lab acceptance.
