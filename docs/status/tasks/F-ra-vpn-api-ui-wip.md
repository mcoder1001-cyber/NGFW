# RA API/UI parallel implementation checkpoint

Branch: codex/ra-api-ui-20261005. Worktree: /dev/shm/ra-api-ui-20261005. Base local08964bd58 / published131bdfbc19128178f876f2a6336986bb97ac83b8 (identical treec5f1211c10b8c4139c1081732f11ac16527c341f).

Owned: additive Dataplane RA RPC contracts and normal generated Go/TS/YANG outputs; schema outerPolicy structural field19; AgentClient RA adapters; apps/api/src/features/ra-vpn/**; narrow required FakeAgent F-ra-vpn declarations (UNIMPLEMENTED default); apps/web/src/domains/vpn/ra-vpn/**; en/fa RA locales and VPN tab/i18n hunks; API-client and required CLI operation normal generation; docs/user/vpn/ra-vpn.md.
Engine author retains privileged lifecycle, namespaces, TAPs, ACL attachment/readback, private engine, credentials, semantic validators and agent RPC implementations. No shared host package/service/VPP changes or cache deletion performed.

## Durable contracts

- RPCs: localb41a0ffb4 / remote51ddd0a6ad5e4eac240cdc9f2fc5bf6b11862476, exacttree69dbaf8dbf0f7f23c1c377c7be9b021ae8bd0536.
- Outer policy19: local4f8ba6ab6 / remotee495dada55d4d28a594f6084693bc1cdd001a61a; normal schema/proto/YANG generation PASS, exact tree verified before publication.
- Configured owner boundary: localbdc4a75cff911da5192b21e143db157ac18b45db / remoted70837b56db9e60e34c357a5a45cfbe00688f4ee. Capabilityrequestowner1, sessionsrequestowner4, disconnectrequestowner3. AgentClient injects configured owner; HTTP never selects it. Engine author agreed checkOwner before every handler lookup.
- Consumer WIP: locala6a0aa3fe / remote83e9b07a1e43dc62fe2108f8aeb2bd6f4c399ef8, exact tree verified. It predates closure fixes described below and is not final.

## Completed consumer code

Real AgentClient adapters; protected capability and paged session observations; admin-only disconnect using readback and normal mutation audit. False removal is409/failure audit. RA-specific error wrapper retains HTTP403/409/503 and gRPC code while withholding daemon diagnostics. Observations are structurally bounded, corrupt/foreign profile observations are502 with static diagnostics. Counters remain generated uint64 decimal strings across actual gRPC/HTTP, including18446744073709551615. Existing forceLong=string generator setting unchanged.

Common dependency: exact nine-line HA permission-denied mapping from local52c2b274, preserving403 and withholding permission text. No HA features copied.

UI: four-step canonical-schema wizard for public endpoint/identities/VRFs/auth/cert/CA/proposal, explicit outer and inner transit plus both ingress/egress existing ACL references, pools/DNS/split routes, EAP/RADIUS reference-only credentials and timers. Operational capability and supported auth gate enabled submissions. Disabled drafts remain editable. Session grid preserves exact counter text, opaque cursor page, role-gated administrator confirmation and scoped disconnect. Recursive en/fa labels and user client/API/CLI docs included.

## Actual checks

- Normal proto/schema/YANG/OpenAPI/API-client/CLI generation PASS.
- All API unit tests:104files/694tests PASS125.68s on consumer code before the configured-owner follow-up. No actual PostgreSQL/Valkey integration or appliance acceptance claimed.
- Targeted actual Unix gRPC→AgentClient→Nest HTTP:14tests PASS542ms/runner19.81s after owner injection; maxuint64, malformed IDs/bounds/queries, role/admin/unauth audit, foreign/stale conflict, unverified removal, runtime failures, diagnostic withholding. Latest capability consistency follow-up PASS14tests504ms/runner16.75s; configured owner injection observed for capabilities, sessions and disconnect.
- UI unit suite:8tests PASS20.95s/runner34.92s, including mounted four-step disabled-draft save, nonoperational enable refusal, exact counters/admin confirmation, readonly and no unsupported polling. Scripted unit HTTP fixture is explicitly not an actual-endpoint screenshot. Latest mounted Persian nested-label case and unchanged branding guard PASS:9UI+3branding tests, runner39.22s.
- API and web typechecks PASS after owner injection and recursive locale schema changes. Final frozen-source reruns PASS.
- Scoped API lint PASS. Scoped web lint initially found missing translation identifiers then an unused local; fixed without suppressions, latest scoped lint PASS.
- First unchanged full web suite:102files PASS,1file failed;605tests PASS,1failed/431.84s. Real failure was strongSwan client name in a locale hint violating the existing product branding guard. Both locale hints now say other IKEv2 clients; no tests/assertions changed or weakened. Scoped branding+UI9 tests PASS39.22s. Full unchanged web rerun completed PASS103files/607tests431.06s. Startedc1e9a343d; latest UI default/enum delta happened in final phase. This is prior-run provenance, not final aggregate exact-tree quick proof. Latest target12cases separately PASS40.12s.

## Delegated source complete; overall RA task remains unfinished

All owned source is complete and product-frozen at localfccb1f5b7f37d96307e64138dbbb6136152fcc2d / tree6e41f8dbb4c8762b575ed3b58bcff86ffce5a22f. Final tests and lint/typechecks are green with exact provenance in F-ra-vpn-api-ui-test-receipt.json. Only bounded consumer completion is claimed. Owner-default follow-up accepts a draft only when enabled is explicitlyfalse, because canonical omitted-enabled defaults true; FA includes translated authentication and DPD option labels. Then hand code to engine author and root for final actual backend integration, independent R1/R2/R3/R6/R7/R8 reviews, actual-endpoint screenshot and complete quick gate after latest-main integration. Privileged runtime/lifecycle/policy and full packet acceptance remain engine author's explicit unfinished scope. Neither this WIP nor local unit passes mark the overall RA task complete.

Closure checkpoint localb62dcabb0 / remotefd2728dde1f41966270b2dfd697927f5f1841203 preserves all consumer ownership/privacy/activation code and generated CLI additions. Later locale-hint and Persian test-harness correction is a separate committed checkpoint.

Next manager command: fetch codex/ra-api-ui-20261005 and merge its published consumer checkpoint into the engine integration branch, preserving contracts/history. Assign production controller/projection/RPC runtime and fresh R3/T2 plus R6/T4 independently. After actual runtime integration, perform normal generation and the unchanged complete quick gate on the final latest-main tree. This worker releases its slot; it is no longer an active developer after handoff.
