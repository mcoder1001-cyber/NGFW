# F-vrrp-config-sync completion — 2026-10-04

Implementation complete for the manager's recovery envelope; integration is owned by the manager. Branch `codex/ha-complete-running-20261004`, isolated worktree `/root/.codex/worktrees/8c19/developers/HA-complete`, original base `4c8d1b247`.

Final frozen product source: `ee3bff260d09694651f84a4e4bdb90a77566a991`, tree `59e3c96bc71c153df08c01a3650799c19e240842`. GitHub connector published exactly that tree as remote checkpoint `abb451f226729b947764541f4ee941354e2ce230` on the same named branch; SHA/tree comparison succeeded. CLI push returned HTTP 403; no CLI publication is claimed. Earlier published snapshots are retained in remote ancestry. Independent packaging reviewer approved both core source `a9f8f24dc525896f34e2e086bbe21c1da451c6ab` and the final five-file classification followup `ee3bff260` without modifying HA product source. Review reports live in that reviewer's branch and manager queue.

## Completed behavior

- Additive generated `VrrpState` RPC, owner-scoped native dumps, actual native `WatchEvents` transitions, gated keepalived notify/live-dump observation through the existing cached renderer, and bounded shutdown-aware event polling. Unknown/unavailable state never becomes a fabricated Master.
- Live VRRP API and role/priority/master-interval display. Cluster peer table shows revision/lag, observed role and sync errors; operator Force sync. en/fa keys have matching shape. Failed refetch discards stale role/status display and disables the force button.
- Peer HTTPS is pinned to the configured SHA256 leaf certificate, checked for validity before any request is written. Authentication binds origin, source revision, timestamp, nonce and document with cluster-key HMAC SHA256. HTTPS ingress uses the existing trusted-hop `requestProtocol` helper. Requests have an absolute eight-second deadline, bounded response and shutdown cancellation; no redirect or HTTP fallback.
- API-to-API confirmed-running sync through the ordinary validation/dry-run/apply/promote engine, tagged `cluster-sync`. Dirty/locked candidates, pending confirmations and degraded/reconciling state refuse incoming commits. Loop suppression skips automatic relaying of cluster revisions. Automatic writer authority requires every enabled configured VR to be present, error-free and Master; mixed/unknown/missing roles fail closed. Explicit Force sync chooses the authoritative sender.
- Receiver restores its node-local cluster, management credentials/TLS, hostname, setup and hardware/dataplane settings plus its own extra exclusions. No secret values are synchronized; missing receiver references fail by reference name only.
- Accepted source revision provenance is persisted atomically in the ordinary revision kind/comment. Under the same cross-process commit lock, the latest exact-origin stamp rejects stale or equal source revisions, including after restart. The query uses exact prefix comparison, not SQL wildcard matching.
- Cluster config is correctly classified as API-managed; dataplane drift continues to show unrelated `/ha/vrrp` changes. Enabled NAT/IPsec/ACL state-sync controls retain individual explicit unsupported warnings.

## Actual focused verification

All listed commands exited zero after required workspace dependencies were built. Initial missing-dependency/generated-output errors were fixed; they are not counted as successful verification.

```text
pnpm --filter @ngfw/schema exec vitest run src/semantic/vrrp-config-sync.test.ts
Test Files 1 passed; Tests 2 passed

pnpm --filter @ngfw/api exec vitest run src/features/vrrp-config-sync src/auth/transport.test.ts src/auth/route-guard.test.ts
Test Files 7 passed; Tests 38 passed
# This run preceded the two independent-review corrections. Their final regressions follow:
pnpm --filter @ngfw/api exec vitest run src/features/vrrp-config-sync/service.test.ts
Test Files 1 passed; Tests 11 passed (including durable stale floor and mixed/missing/error/disabled authority)
pnpm --filter @ngfw/api exec vitest run src/features/vrrp-config-sync/classification.test.ts src/state/drift-defaults.test.ts
Test Files 2 passed; Tests 6 passed

pnpm --filter @ngfw/web exec vitest run src/domains/system/ha/HaPage.test.tsx
Test Files 1 passed; Tests 6 passed

node test/topology/vrrp-config-sync/transport-smoke.mjs
PASS: real disposable HTTPS; HMAC authenticated; matching pin delivers; wrong pin sends zero HTTP requests

pnpm --filter @ngfw/api typecheck
pnpm --filter @ngfw/web typecheck
# both passed; final API typecheck also passed after source-floor/authority/classification changes

pnpm --filter @ngfw/api exec eslint src/features/vrrp-config-sync src/commit/commit.service.ts src/agent/agent.client.ts src/testing/fake-agent.ts
pnpm --filter @ngfw/web exec eslint src/domains/system/ha
pnpm --filter @ngfw/schema exec eslint src/semantic/vrrp-config-sync.ts src/domains/ha.ts
# passed; Node emits the existing root package module-type warning, no lint findings

go -C apps/agent vet ./internal/agent ./internal/subsystems ./internal/descriptors/vrrp
# passed

go -C apps/agent test -race ./internal/agent ./internal/subsystems ./internal/descriptors/vrrp -run 'Vrrp|VRRP|Keepalived' -count=1
ok ngfw/agent/internal/agent 2.191s
ok ngfw/agent/internal/subsystems 1.690s
ok ngfw/agent/internal/descriptors/vrrp 1.272s

go -C apps/agent test ./internal/agent -run '^TestHaCluster|^TestVrrp' -count=1
ok ngfw/agent/internal/agent 0.397s

tools/ci.sh check --base 4c8d1b247
check PASSED; gitleaks no leaks; contract and forbidden-pattern guards passed
```

Full/hosted CI was explicitly waived by the existing owner authorization recorded in `next-five-review-plan-20261004.md`; no full CI success is claimed. Generated sources came from the normal pinned `turbo run gen --filter=@ngfw/api-client` scheduler. Baseline stale generated YANG for existing OSPF/setup source also regenerated; the manager regenerates the combined integration tree. Initial failed generation before dependencies were installed was restored immediately in a forward commit, preserving history.

## Laboratory acceptance deferred — NOT RUN

The two-node live PostgreSQL/API/agent config-sync fixture, packet failover within three seconds, keepalived live tracking, restart-recreation timing and real en/fa screenshots were not run. No shared VPP/daemon/package state was changed. Only a disposable loopback TLS server was started and it and its temporary certificate material were cleaned on exit. These remaining lab-only acceptance items belong in the manager's central deferred-acceptance record; they are not claimed as passing tests.

## Manager integration notes

The manager owns the Event17 → `vrrp.events` telemetry mapping, preserving native IPsec and routing events already being integrated. This branch adds the topic and native publishers; the polling UI already consumes the real state endpoints. Agent startup contributes exactly one watcher to the wait group, added to the manager's other native watcher. Preserve reviewed history/checkpoint ancestry under D112 and regenerate combined contracts. Source scope excludes secret value re-encryption, NAT/ACL/IPsec session replication and historical optional keepalived stand-ins outside the manager's recovery envelope. Next command: apply the approved classification followup `ee3bff260` to the manager integration tree, regenerate/review that tree and perform the authorized sequential merge.
