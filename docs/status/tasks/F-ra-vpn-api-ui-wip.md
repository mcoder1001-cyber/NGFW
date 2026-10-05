# RA API/UI parallel implementation checkpoint

Branch: codex/ra-api-ui-20261005. Base local08964bd58 / published131bdfbc19128178f876f2a6336986bb97ac83b8 (identical product tree).
Owned: additive Dataplane RA RPC contracts and normal generated outputs, AgentClient RA adapters; apps/api/src/features/ra-vpn/**; apps/web/src/domains/vpn/ra-vpn/**; en/fa RA locales and VPN tab/i18n hunks; API-client generation.
Engine author retains privileged lifecycle, namespaces, TAPs, ACL attachment, private engine, credentials, agent RPC implementations.

Completed: paired exact additive RPC names and field numbers with engine author. Contracts expose capabilities, bounded opaque cursor sessions with 64-bit counters, readback-confirmed disconnect; no process identifiers or secrets.
Tests: normal proto generation pending completion. No consumer implementation or runtime support claimed.
Remaining: real API adapters/controller/audit, wizard and operational capability gates, session table/admin disconnect, en/fa, actual transport/controller/UI tests and API-client generation.
Next: finish normal proto generation, publish contract checkpoint before implementing consumers.
