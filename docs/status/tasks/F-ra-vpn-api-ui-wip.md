# RA API/UI parallel implementation checkpoint

Branch: codex/ra-api-ui-20261005. Base local08964bd58 / published131bdfbc19128178f876f2a6336986bb97ac83b8 (identical product tree).
Owned: additive Dataplane RA RPC contracts and normal generated outputs, AgentClient RA adapters; apps/api/src/features/ra-vpn/**; apps/web/src/domains/vpn/ra-vpn/**; en/fa RA locales and VPN tab/i18n hunks; API-client generation.
Engine author retains privileged lifecycle, namespaces, TAPs, ACL attachment, private engine, credentials, agent RPC implementations.

Completed: paired exact additive RPC names and field numbers with engine author. Contracts expose capabilities, bounded opaque cursor sessions with 64-bit counters, readback-confirmed disconnect; no process identifiers or secrets.
Tests: normal proto generation pending completion. No consumer implementation or runtime support claimed.
Remaining: real API adapters/controller/audit, wizard and operational capability gates, session table/admin disconnect, en/fa, actual transport/controller/UI tests and API-client generation.
Next: finish normal proto generation, publish contract checkpoint before implementing consumers.

## Consumer work in progress

Published contracts: 51ddd0a6ad5e4eac240cdc9f2fc5bf6b11862476 (RPCs, exact local b41a0ffb4 tree); e495dada55d4d28a594f6084693bc1cdd001a61a (outerPolicy19/schema/normal Go/TS/YANG, exact local4f8ba6ab6 tree).
Real API consumers implemented: actual AgentClient unary adapters, protected capability/session routes, admin-only disconnect and existing mutation audit. Narrow FakeAgent required-interface declarations return UNIMPLEMENTED by default; actual tests own an explicit Unix gRPC server.
Common dependency: exact nine-line HA permission-denied mapping from local52c2b274, preserving HTTP403 and withholding remote permission text. No HA features copied.
Actual test result: ten actual Unix gRPC→AgentClient→HTTP cases PASS509ms (runner17.90s), including maximum uint64 decimal18446744073709551615 preserved, invalid bounds/IDs, role/auth rejection, foreign/stale conflicts, failure/success audit, unavailable transport and withheld denial details. API build and normal OpenAPI/API-client generation PASS.
UI wizard/session consumer code exists but not yet verified; do not label complete. UI package dependency builds/typecheck running. Remaining: meaningful UI/model/en/fa tests, scoped lint, all API test/typecheck, truthful artifact/runtime integration against engine author's final code and screenshot actual endpoint.
Next: finish UI dependency build, run web typecheck then actual unit tests; fix only owned consumers; publish this unfinished checkpoint.
