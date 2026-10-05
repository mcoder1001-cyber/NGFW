# F-ha-state-sync independent T2 API test

Current exact source:834eed6fb94497321aece0f3b77d9fec055afe71. Current verdict:PASS. Original failing-source evidence below is preserved as history.

Exact source:48e5af12fd6a42323f3c83d04129eb6563cef650. Date:2026-10-05 UTC. Assigned slot10, actual exclusive `/run/lock/ngfw-slot-10.lock`. All PostgreSQL18 data/WAL and Valkey fixture files in owned executable `/dev/shm`, only owned private processes on127.0.0.1:4002/4001. Shared PostgreSQL/Valkey/VPP services unchanged. Source snapshot:/dev/shm/r3-021ue61v/ha. Private fixture copies under apps/api/.reviewer-r3; tracked product source unchanged.

Command (within private fixture lifecycle):
`NGFW_TEST_PREFIX=w10 NGFW_SLOT=10 NGFW_HTTP_PORT=4000 PGHOST=127.0.0.1 PGPORT=4002 NGFW_PG_HOST=127.0.0.1 NGFW_PG_PORT=4002 ../../tools/heavy.sh pnpm exec vitest run -c .reviewer-r3/config.ts`

|Scenario|Observed|Result|
|---|---|---|
|Existing transactional config regression|15 passing cases including commit/rollback, confirmed auto-revert, 400pointers,404, RBAC,409singlewriter, secret output protection, audit, agent503|PASS|
|Unauthenticated state/read-only honest unsupported state|401/200; unknown counters null and ED/ACL/IPsec inactive unsupported|PASS|
|Readonly/operator resync|403before agent dispatch|PASS|
|Admin on nonglobals-owner|502instead of403; real agent likewise returns PermissionDenied|FAIL|
|EI missing explicit endpoints/dedicated interface|400with exact expected pointers|PASS|
|Injected active observed state resync|200onlyafterfake completion; uint64max remains exact decimal string, missed0, actual PostgreSQLaudit row successful|PASS|
|Agent unavailable|503problem-json|PASS|

First meaningful wave output:
```text
Test Files1 failed(1)
Tests1 failed|19 passed(20)
AssertionError:expected502 to be403
Duration41.94s
drop database ngfw_w10
drop role ngfw_w10
```

Reproduced independently with fresh private fixture:
`... pnpm exec vitest run -c .reviewer-r3/config.ts -t 'denies nonadmins'`
```text
Tests1 failed|19 skipped(20)
AssertionError:expected502 to be403
Duration28.38s
ok nothing named ngfw_w10/ngfw_w10 remains
OWN fixture processes stopped; shared services untouched.
```

An earlier reviewer setup mistakenly used role `read-only` instead of `readonly`; its beforeAll400was fixture-only and corrected without product/test assertion changes. A retry preflight hit TCP TIME_WAIT; adding SO_REUSEADDR to the private-port availability check allowed a fresh fixture, with no service mutation. These are not product failures.

R3-HA-01 records the reproducible product status-code failure. APIrestart/actual simultaneous writes supplemental cases are pending and will be reported separately. No real two-node continuity or VPP dataplane execution is claimed.

**Historical48e5af12f verdict:FAIL; fixed-source results below supersede it.**

Supplement:actual simultaneous distinct-principal candidate writes PASS. APIrestart reviewer fixture supplied a scalar string without JSON media type, receiving415before reaching restart. Corrected reviewer-only fixture changes the write to an object payload under `/config/system`; it was not rerun under manager stop instruction. This is reviewer setup, not an additional product finding. Actual APIrestart remains unverified.

All privatePG/Valkeyprocesses stopped, owned role/database dropped, exclusive slotlock released. Manager accepts R3-HA-01 and will use fresh arbiter/developer for the third-round policy; no reviewer product edits.

## Fixed-source verification on834eed6fb

Private real PostgreSQL18/Valkey fixture recreated under the same exclusive slot10lock. No shared DB/Valkey/VPP mutation. First source-fix wave passed21HA/config scenarios and simultaneous-write arbitration. Reviewer restart-response parsing expected JSONfrom a scalar leaf; actual APIreturned200and raw persisted value. Reviewer-only correction uses whole/systemobject and repeats restart assertion; product unchanged.

|Current scenario|Actual observed|Result|
|---|---|---|
|Nonglobals-owner admin resync, two fresh fixture runs|403agent-permission-denied, grpcCodePERMISSION_DENIED, generic non-sensitive detail, failed PostgreSQLaudit/status403/after:null|PASS|
|Readonly/operator role denial|403before action dispatch|PASS|
|Inactive endpoint resync, two runs|409FAILED_PRECONDITION; failure audit/after:null|PASS|
|Agent unavailable, two runs|503problem-json; failure audit/after:null|PASS|
|Injected completed resync, two runs|200only after fake completion, success audit, exact uint64max decimal string|PASS|
|Real APIapplication restart retaining DB/Valkey|Committed hostname persists, same authenticated token can read HAstate after APIrecreation|PASS|
|Actual simultaneous different-principal writes|Exactly200/409problem-json|PASS|
|Other original transactional/schema/config cases|21-case feature/configsuite passed allcases; no weakened assertions|PASS|

Actual commands in own copied source:
`... pnpm exec vitest run -c .reviewer-r3/config.ts`
`... pnpm exec vitest run -c .reviewer-r3/config.ts -t 'denies nonadmins|reloads committed|arbitrates actual|preserves inactive|translates unavailable|completes resync'`

```text
First wave:.reviewer-r3/ha.test.ts21tests PASS
First wave restart supplementary:1reviewer JSONparse failure/1concurrency PASS
Final focused replay:Test Files2 passed(2)
Tests6 passed|17 skipped(23)
Duration42.68s
ha T2exit0
drop database ngfw_w10
drop role ngfw_w10
ok nothing named ngfw_w10/ngfw_w10 remains
OWN fixture processes stopped; shared services untouched.
```
Seventeen cases intentionally filtered in second command already passed in first21-case suite. Twenty-three unique T2scenarios passed across commands; no actualnative two-node/VPP continuity claim. Separate actual AgentClientstream tests7passed, retaining403/409/503/502maps and preventing partial/ended-stream inventedcompletion.

R3-HA-01 is CLOSED for this exact source.

**Current T2 verdict:PASS.**
