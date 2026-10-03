# Native/schema/proto merge review — 2026-10-03

Read-only comparison against locally available `origin/main` **29f9cae41bb1f1120918a85e92126c1aed49edff**. No fetch, product edits, plan changes or other-worktree mutation. This report reviews integration risks; it does not assert a completed merge or refreshed upstream head.

## Merge blockers to resolve

**P1 — Preserve newly merged PKI/notification contracts when resolving shared-file conflicts.** Relative to this upstream head, the current working proto lacks `PkiFileState` and its messages, `PkiKeySpec`, `PkiCsr`, `PkiIssued`, `PkiCa.key_spec=5/issued=6`, `PkiCertificate.csr=7/issued=8`, and `ManagementConfig.notifications=7` with its notification message graph. The current schema likewise lacks upstream PKI public metadata and `management.notifications`, and would remove their fixtures/parsed-document coverage. `apps/api/src/app.module.ts` differs by dropping upstream PKI, notifications and OSPF registration. These are older-base differences, not intended native work. Taking the current entire file would regress released features. Integrate native additions onto the latest source and retain upstream module registration, contracts, exports, fixtures and tests; regenerate all consumers from that combined source. Do not hand-merge generated TS/Go clients.

**P1 — System identity response tags cannot be overwritten with the current older contract.** Upstream `SystemIdentityStateResponse` has `owner=1`, timestamp `retrieved_at=2`, `hostname=3`, optional double `uptime_seconds=5`, optional scalar `kernel_hostname=6`, and resolver/error fields7–11. Current source changes field1 to `mechanism`, field2 to scalar `static_hostname`, field5 to uint64 and changes cardinality/type/meaning of fields6–11. This is a wire compatibility violation, independent of the native additions. Preserve upstream identity service/response semantics, or deliberately append new fields with new numbers and adapt callers; do not reuse existing tags. A compile-only gate cannot establish this compatibility.

These blockers concern integration choices. They do not imply that the native PSK implementation itself removed current upstream features before merge.

## Native additions and shared secret boundaries

The proposed native tags are free at the reviewed upstream head: `ApplyRequest.secret_bundle=7`, `DryRunRequest.secret_bundle=5`, `ActionRequest.ikev2=11` and `EVENT_KIND_IPSEC_SA_CHANGED=12`. The native action SPI types remain uint64/uint32 with the API preserving IKE decimal-string precision. Keep those tags and add `SecretBundle`/`Ikev2Action` without renumbering old messages or oneof members. An additive proto change still requires matching product API/agent implementation for native capability; old agents do not acquire native PSK support by ignoring unknown request fields.

Native `SecretDeliveryService` selects only enabled `vpp-ikev2` PSK references. Retain that selection when combining it with PKI and notifications: it must not silently ship SMTP passwords, webhook tokens, CA signing keys or certificate material through this PSK path. Notifications remain API-owned; their secret-reference-only DesiredState contract and dispatch recovery must survive. Upstream `rpc_pki.go` explicitly reports unavailable materialization; the new sealed PSK cache does not complete that materializer or supply VPP peer trust/global signing-key mapping.

Merge validation should include descriptor wire-breaking comparison against the reviewed upstream baseline, schema/proto drift, regenerated-output gates, PKI and populated-notification parsed-document round trips, module/route presence, and the native secret-channel tests. The earlier native review's focused race checks and real packet/cache proofs remain useful but were run on the pre-integration workspace; they do not validate the combined merge source. No new heavy check was run for this comparison.

## Scope that must remain open

The native route-based PSK milestone is reviewed and its direct-agent uniqueness finding was fixed. Overall `F-ikev2-native` remains running: certificate authentication needs an explicit peer-certificate/CA-trust contract and plugin-global private-key provisioning. IPv6 overlay and NAT traversal remain unverified; full VPP-process restart negotiation is not covered by the proven agent restart. Local responder CHILD rekey stays explicitly refused.

Native SA state is available through read-only RPC/REST polling. The current `EVENT_KIND_IPSEC_SA_CHANGED` publishers are the historical strongSwan watcher; no native SA watcher was found. Preserve notification schemas but do not claim native SA WebSocket/VPN-notification delivery from the enum/relay mapping alone. The stale P11 comments in `ipsec.controller.ts:10–20` and `vpn/ipsec/queries.ts:11` still describe VICI and a pending secret channel; update those claims when integrating the native primary state path.

Native-only engine selection and required IPIP binding intentionally change the configuration contract from upstream's strongSwan default. Existing strongSwan/policy-based documents need an explicit migration or a clear validation refusal; preserving upstream PKI metadata must not reintroduce an unsupported strongSwan fallback. Required VPP patches0002/0003 are product build inputs; merge does not authorize installation or restart of shared VPP.

Disposition: conditional approval for additive native integration, subject to the two P1 compatibility blockers and fresh combined-source validation. No deployment or full certificate/PKI acceptance approval is implied.

## Dedicated integration source check

Follow-up read-only inspection of `/root/.codex/worktrees/bbd9/NGFW/.scratch/merge-integration`, base **6db19a51311e9fca238e2595207361264aea3393**, after the no-commit checkpoint cherry-pick. Source proto/schema auto-merges retain upstream PKI metadata/messages/RPC, populated notification fields and the original SystemIdentity response wire layout. AppModule's automatic diff only adds SecretDeliveryService. The two whole-file overwrite risks above are therefore avoided in the inspected integration source; generator regeneration and combined-source validation remain necessary.

**P1 — Missing preexisting native/tunnel state foundations after cherry-pick.** The inspected integration proto contains SecretBundle/Ikev2Action additions but no `IpsecState` service method or request/response/IKE/CHILD/connection messages, no `EVENT_KIND_IPSEC_SA_CHANGED`, and no `TunnelState` service/messages. Those foundations existed on the older implementation base and were deleted upstream, so a checkpoint delta does not restore them. New `rpc_ikev2.go` already refers to missing state types; AppModule has no `ipsecFeature` registration. Resolve deleted/modified route and RPC files by restoring the reviewed native-only product path, and transplant its exact state contracts additively onto the preserved upstream proto. Restore TunnelState's reviewed contracts separately. Preserve upstream PKI/notifications/identity and regenerate every consumer; do not restore a strongSwan product fallback simply to satisfy old tests. Event12 can be restored for retained source/test consumers without claiming native event publication.

This follow-up is a source audit while conflicts remain unresolved, not a compile/test pass. Root owns conflict resolution and product changes. Conditional merge approval remains blocked on restoring these required foundations and passing the combined-source gates.


## Foundation restoration follow-up

Read-only reinspection confirms root restored additive IpsecState/TunnelState RPCs and messages, Event12, and native Action11 while retaining upstream PKI, notifications and SystemIdentity fields. AppModule registers the native feature; VPN tabs and both locales register its UI. AgentClient imports the state response and implements the owner-scoped paged state call. Previously absent UI model/query helpers and the disposable topology entrypoint are present. Schema semantic registration includes both VPN and IPsec validators.

Native subsystem registration, the protected-IPIP admin descriptor guard, projection/assembly hooks and static-route logical-tunnel resolution are present. Production Start attaches the stable sealed secret cache before constructing Service. strongSwan interoperability code is confined to internal/testpeer/strongswan and test imports; no product fallback was found. No further missing native registry foundation was identified in this pass.

The earlier missing-foundation blocker is resolved at source level. Generated consumers and other conflict resolutions still require the root combined-source gates; this review did not run another build. Restored state-contract comments still describe VICI/charon and should describe native profiles/SAs and intentionally unavailable legacy watcher fields. This documentation mismatch does not establish a runtime fallback. Overall certificate/native PKI acceptance and native SA event publication remain outside the completed PSK milestone.
