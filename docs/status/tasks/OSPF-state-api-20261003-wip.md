# Bounded OSPFv2 state API checkpoint

Branch `codex/ospf-state-api-20261003`, isolated `/root/.codex/worktrees/0b16/developers/OSPF-state-api`, base `b2c214b5`. Contract first `ca0e30bf`; final local implementation checkpoint is this document's commit. Root owns publication, shared app.module registration and generated outputs.

Owned ONLY new `apps/api/src/features/ospf/{dto.ts,index.ts,ospf.controller.ts,state.ts,ospf.controller.test.ts,state.test.ts}` and this report. Backlog evidence: F-ospf status lists API state as absent; existing agent ospf/state.go registers ospfNeighbors scoped reader, and AgentClient.routingState carries owner automatically. No proto/runtime FRR/VPP, shared API module, SDK/CLI, HTTPS, P10/P11 or ISO files modified.

Implemented standalone GET `/api/v1/state/ospf` controller using ONLY fixed `ospfNeighbors` reader and empty RIB prefixes. Public schema selects VRF/router ID/address/interface/state/priority with nullable unknowns; unfamiliar states become Unknown. Current/multi-VRF and legacy/single-instance FRR forms follow existing agent fixtures. Raw reader JSON, extra fields, other readers and provider errors are never returned. Generic partial-observation warning preserves upstream error visibility without diagnostic disclosure. Missing/invalid readers and stopped FRR are explicit unavailable causes, not fabricated healthy empty inventory. Mapped agent RPC failures propagate. Observed `{}` is valid empty inventory.

Bounds: at most1MiB reader bytes,128 instances,2000 router keys per instance/total rows inspected; public response100 sorted neighbors plus truncation flag. Malformed or excessive input yields explicit unavailable and no partial healthy rows. No caller-provided reader/command/prefix selectors or mutation. Existing global authenticated readonly GET convention retained.

New regressions exercise public projection/redaction, both FRR shapes, unknown metadata, empty vs missing/stopped, seven malformed payloads, sorted truncation, byte/instance/work limits, timestamp unknown, fixed reader requests, mapped errors. Minimal Nest/Fastify fixture with real existing AuthGuard checks route registration, unauthenticated401 before agent calls and readonly200; test auth provider is mocked, not a new policy. Query selectors are ignored. Full application registration remains root-owned and is not claimed present in this source tree.

Actual worker checks: Prettier write and git diff --check passed; source inspected against existing agent fixtures and neighboring BGP/auth conventions. Automated tests/lint/typecheck NOT RUN per root finite-lane ownership. No database/network/host reads, transport calls or full suites launched by worker.

Root integration: import ospfFeature from `./features/ospf/index.js`, add `...ospfFeature.controllers` and `...ospfFeature.providers` under F-ospf anchors in app.module.ts. Regenerate API schema/client and run unchanged complete quick gate. No new proto required.

Finite focused commands after compatible dependencies/builds are prepared:

```sh
pnpm --filter @ngfw/api exec vitest run src/features/ospf/state.test.ts src/features/ospf/ospf.controller.test.ts --maxWorkers=1
pnpm --filter @ngfw/api exec eslint src/features/ospf
pnpm --filter @ngfw/api typecheck
```

Remaining current feature scope: root wiring/generated verification and independent review; live FRR/VRF observations, OSPF neighbor UI, OSPFv3/auth/Event20/LSDB belong separate coordinated tasks and are not implemented or claimed by this bounded neighbor endpoint.
