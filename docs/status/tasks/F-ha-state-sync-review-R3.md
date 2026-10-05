# F-ha-state-sync independent R3 contracts/API review

Current exact source:834eed6fb94497321aece0f3b77d9fec055afe71 (remote042b66ea40d60fab5d702fb4045394c826e2f16d; tree128eff8f5cbd4e954d167d034257c71dd586c57f). Current verdict:APPROVE. Current BLOCKER0/MAJOR0/MINOR0. Original finding/evidence below is preserved as history.

Date:2026-10-05 UTC. Reviewer:/root/review_r3_api_contracts, no product authorship.
Exact local source:48e5af12fd6a42323f3c83d04129eb6563cef650. Snapshot:/dev/shm/r3-021ue61v/ha.
Reference main:492c0156e38ee73acca235527d7979b447067bb8.

## Findings

Historical48e5af12f:BLOCKER0/MAJOR1/MINOR0.

HaCluster.StateSync4/5, ActionRequest13 and the new HaSyncState RPC/messages are additive; existing stateSync flag names/defaults remain unchanged. Old stored HA configurations remain schema-readable; an enabled EI operational request now needs explicit listener, failover and dedicated-interface semantic checks with pointers. The REST state route is authenticated and reports absent observations/counters as null, process-local uint64 resyncCount as a decimal string, and NAT44-ED/ACL/IPsec unsupported kinds explicitly. The resync action requires admin metadata, adds an audit resource before dispatch, waits up to20 seconds for the native15-second completion budget, and rejects an ended/failed stream rather than inventing completion. Flush has explicitly native queued-update semantics in the agent contract and is not mislabeled as session deletion. No unsupported API, packet counters, SA continuity or two-node proof is promised. No existing route/schema/proto number is renamed or reused.

Both source histories contain the required contract-prefixed commits and task contract notes. Node uses agent gRPC, never VPP directly. Generated artifacts were produced normally, without manual edits.

## Actual independent verification

Commands ran with private executable RAM TMPDIR/GOTMPDIR/GOCACHE and private RAM pnpm store/Turbo cache; heavy semaphore preserved.

- `tools/heavy.sh pnpm install --frozen-lockfile --store-dir /dev/shm/r3-021ue61v/store --prefer-offline`:PASS.
- `tools/heavy.sh pnpm exec turbo run build --filter=@ngfw/api^... --concurrency=2`:6 successful tasks.
- `tools/heavy.sh pnpm exec turbo run gen --filter=@ngfw/api-client --concurrency=2`:9 successful tasks.
- Independent git blob-hash comparison of normally regenerated tracked Go/TS proto, API-client types and YANG outputs:31 of31 matched the exact source.
- Focused schema tests:3 passed.

HA controller boundary tests are queued separately; T2 execution reported independently.

The generator emits existing unrelated OIDC callback OpenAPI response warnings; generated tracked bytes nevertheless match. Complete quick CI remains the manager/T1 gate, not claimed here. T2 uses actual isolated PostgreSQL/Valkey with a clearly identified fake agent; data-plane execution is R4/T3 scope.

## MAJOR R3-HA-01: authorization denial becomes an upstream failure

`apps/api/src/agent/agent.client.ts:715` omits `GrpcStatus.PERMISSION_DENIED`; `:757` converts it to502. The new `apps/agent/internal/agent/rpc_ha_sync.go` explicitly returns PermissionDenied when the agent is not the globals owner. An authenticated admin calling `POST /api/v1/actions/ha/sync/resync` therefore receives502, despite a deliberate ownership/authorization refusal. Readonly/operator APIguard403 is correct; this is the second agent authorization boundary. It misclassifies a permanent policy refusal as an upstream failure and breaks the route's documented forbidden response.

Reproduced twice through the unchanged API/gRPC boundary with actual private PostgreSQL18/Valkey and a fake agent returning the same gRPC code as the real agent. First wave:19 passed/1 failed; focused reproduction:1 failed/19 skipped. `application/problem+json` is retained. The security boundary itself still refuses mutation; no bypass was observed.

Fix: translate this intended permission refusal to403 with a distinct permission problem (globally if consistent with existing error policy, or explicitly for this HA action), and add a regression using the actual AgentClient action transport rather than only direct controller mocks. Preserve409 precondition and503 unavailable mappings.

```text
AssertionError: expected502 to be403
Expected:403
Received:502
Tests1 failed|19 passed(20)
Focused reproduction:Tests1 failed|19 skipped(20)
```

The earlier R3 APPROVE published in59834acac10685b9da4a74ff118bdf328feb1ba6 is superseded by this actual API result.

**Historical48e5af12f verdict:BLOCK; superseded by the verified fix below.**

## Actual successful contract/unit output

```text
Tests3 passed(3)
Duration5.47s
Tests4 passed(4)
Duration15.28s
Tasks9 successful,9 total
Time1m38.733s
HA normally regenerated tracked artifacts:31 matched:31 differences:[]
```

## R3-HA-01 verified closure on834eed6fb

Fresh developer added the explicit403`agent-permission-denied` branch to agentProblem and fixed generic permission detail rather than exposing agent-private details. No existing proto/schema/client shape or number changed. Other gRPC statuses retain existing mappings. The independent reviewer did not write this product fix.

Actual unchanged API→AgentClient→gRPC transport against private real PostgreSQL18 and Valkey reproduced intended nonglobals-owner403twice on separate fresh fixture runs. Permission problem remains RFC9457with grpcCodePERMISSION_DENIED; detail does not disclose globals-owner internals. Actual PostgreSQL audit records failure/status403/after:null; no success completion. Inactive observations remain409FAILED_PRECONDITION with failed audit; disconnected agent remains503with failed audit. Both mappings replayed twice. Injected completed action retains success audit and exact uint64decimal max.

Independent author regression command:
`tools/heavy.sh pnpm --filter @ngfw/api exec vitest run src/agent/action-transport.test.ts`
```text
Tests7 passed(7)
Duration15.31s
```
This includes partial stream followed by403/409/503/502error, failed audit context, and ended stream without done.

First real API wave:21HA/config cases PASS, concurrent writes PASS, plus reviewer-only scalar-response JSONparse error at restart. Corrected reviewer fixture reads whole system object, recreates actual API against retained owned PostgreSQL/Valkey, observes committed hostname and authenticated HAstate after restart. Targeted second replay:
`... pnpm exec vitest run -c .reviewer-r3/config.ts -t 'denies nonadmins|reloads committed|arbitrates actual|preserves inactive|translates unavailable|completes resync'`
```text
Test Files2 passed(2)
Tests6 passed|17 skipped(23)
Duration42.68s
ha T2exit0
OWN fixture processes stopped; shared services untouched.
```
The17cases are intentionally name-filtered, already passing in the first21-case regression; no environment/integration skip is counted as proof. Total23unique T2scenarios passed across commands. Native two-node continuity remains outside this APIgrade. Separate R6UI finding is not covered or cleared here.

**Current R3 verdict:APPROVE.**
