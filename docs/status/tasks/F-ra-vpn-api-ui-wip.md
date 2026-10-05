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
- Targeted actual Unix gRPC→AgentClient→Nest HTTP:14tests PASS542ms/runner19.81s after owner injection; maxuint64, malformed IDs/bounds/queries, role/admin/unauth audit, foreign/stale conflict, unverified removal, runtime failures, diagnostic withholding. Latest capability consistency follow-up pending final rerun.
- UI unit suite:8tests PASS20.95s/runner34.92s, including mounted four-step disabled-draft save, nonoperational enable refusal, exact counters/admin confirmation, readonly and no unsupported polling. Scripted unit HTTP fixture is explicitly not an actual-endpoint screenshot. Persian mounted nested labels added, pending rerun.
- API/web typechecks PASS before latest nonstructural follow-ups; final reruns pending.
- Scoped API lint PASS. Scoped web lint initially found missing translation identifiers then an unused local; fixed without suppressions, latest scoped lint PASS.
- Full unchanged web suite running; do not claim pass until completion.

## Remaining and next command

Run latest focused API14/UI9 cases, final typechecks/lint, inspect unchanged full web result, publish closure checkpoint. Then hand code to engine author and root for final actual backend integration, independent R1/R2/R3/R6/R7/R8 reviews, actual-endpoint screenshot and complete quick gate after latest-main integration. Privileged runtime/lifecycle/policy and full packet acceptance remain engine author's explicit unfinished scope. Neither this WIP nor local unit passes mark the overall RA task complete.
