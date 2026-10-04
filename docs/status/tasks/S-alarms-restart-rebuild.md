# Recovery note — 2026-10-04

The historical report below preserves previous commands and outcomes. Its historical PASS/BLOCKED claims do not certify current main or current host readiness. This recovery adapts names to NGFW; current results and deferred acceptance are recorded in eight-review-recovery-20261004.md.

# S-alarms-restart-rebuild — API restart forgets active alarms (D-218)

Branch `task/S-alarms-restart-rebuild` · slot 10 (`w10`, API port 4000, DB `ngfw_w10`) · base `main@55a18e0f` ·
fixes `F-dashboard-prom-alarms-host-questions.md` Q4. Files touched: only `apps/api/src/features/dashboard-prom-alarms/**`
and `docs/status/tasks/S-alarms-restart-rebuild*`.

## What was built
`apps/api/src/features/dashboard-prom-alarms/alarms.service.ts`
- `start()` is now `rebuildState()` → `reload()` → `openStats()`. **`rebuildState()`** reads
  `select rule, instance, raised_at from alarm where state = 'active'` and seeds the engine:
  `state.set(stateKey(rule, instance), { since: raisedAt.getTime(), raised: true })`. That is the same key and shape
  `evaluate()` keeps (the prompt calls the key function `ruleKey`; in `engine.ts` it is `stateKey`). It logs
  `rebuilt N active alarm(s) from the database`. Because the state is seeded before the stats stream opens, the first
  sample is checked against the rebuilt state: a healthy sample clears the row and notifies once. A still-violating
  sample raises nothing new (no INSERT attempt, no `raised` webhook).
- **Orphans** are cleared by the existing `pruneRules` path. Because the rebuild runs before `reload()`, rebuilt rows
  whose rule is gone or disabled are pruned like any other state. `reload()` now passes a reason to `persistClear`:
  `rule-removed` (the rule is not in the running config) or `rule-disabled` (`enabled: false`). The same reasons now
  appear when a commit removes or disables a rule while the API runs.
- **`persistClear(rule, instance, reason?)`**: the conditional `UPDATE … WHERE state = 'active' RETURNING` makes a
  clear happen at most once per row. Only the caller that flips the row records the event, publishes and notifies.
  With a reason:
  - the `ALARM_CLEARED` system event gets `data.reason`, and its message gets the suffix
    `— cleared: its rule is no longer configured` or `— cleared: its rule is disabled`
  - the `alarm.events` bus message and the webhook payload carry `reason`
  - a removed rule has no targets left, so it sends no webhook (no webhook storm). A disabled rule still notifies its
    targets once, the same as the live disable path.
  Without a reason (the condition ended), all payloads are unchanged.

`apps/api/src/features/dashboard-prom-alarms/alarms.service.test.ts` (new, 6 tests). The test uses:
- an in-memory `alarm` table behind the three Drizzle chains the service uses (`select…where`,
  `update…set…where…returning`, `insert…values…onConflictDoNothing…returning` with the `alarm_active_uq` rule).
  Conditions are rendered with drizzle's own `PgDialect` and evaluated; any other condition shape throws.
- a fake agent `streamStats` stream, a spy on the webhook `deliver()`, and the real `Bus`.

## How it was verified

### 1. Own unit tests (D-210a): pass
`pnpm --filter @ngfw/api exec vitest run src/features/dashboard-prom-alarms/alarms.service.test.ts src/features/dashboard-prom-alarms/engine.test.ts`
(through `/root/NGFW/tools/heavy.sh`, D-224; at the branch tip, after `pnpm --filter @ngfw/api typecheck` → `typecheck exit=0`):
```
 RUN  v3.2.7 /root/ngfw-wt/S-alarms-restart-rebuild/apps/api

 ✓ src/features/dashboard-prom-alarms/engine.test.ts (6 tests) 22ms
 ✓ src/features/dashboard-prom-alarms/alarms.service.test.ts (6 tests) 125ms

 Test Files  2 passed (2)
      Tests  12 passed (12)
   Start at  21:08:50
   Duration  102.30s (transform 96.87s, setup 0ms, collect 108.52s, tests 147ms, environment 2ms, prepare 1.75s)

vitest exit=0
```

### 2. The same tests fail on the unfixed service
I ran the new test file against `HEAD~1`'s `alarms.service.ts` (temporarily checked out, then restored). All 6 fail
with the bug's symptoms:
```
   × … > start() rebuilds the raise state of every active row, keyed and shaped as evaluate keeps it 91ms
   × … > the first healthy sample after a restart clears the still-active row and sends exactly one cleared webhook 1021ms
   × … > a still-violating sample after a restart raises nothing new; the row clears once the condition ends 10ms
   × … > rows whose rule was removed while the API was down are cleared once each, with a reason and no webhook 11ms
   × … > a rule disabled while the API was down: its row clears once with reason rule-disabled, its target gets one webhook 4ms
   × … > a rule removed by a commit while running still clears its raised alarm, now with reason rule-removed 11ms
AssertionError: expected Map{} to deeply equal Map{ 'cpu\u0000' => { …(2) }, …(1) }
AssertionError: expected 'active' to be 'cleared' // Object.is equality
AssertionError: expected [ 'insert' ] to deeply equal []
AssertionError: expected [ [ 'gone', 'wan0', 'active' ], …(2) ] to deeply equal [ [ 'gone', 'wan0', 'cleared' ], …(2) ]
AssertionError: expected 'active' to be 'cleared' // Object.is equality
AssertionError: expected [ { rule: 'cpu', instance: '' } ] to deeply equal [ { rule: 'cpu', instance: '', …(1) } ]
      Tests  6 failed (6)
```

### 3. Live, once, on slot 10 (real ngfw-api + PostgreSQL `ngfw_w10`)
Setup:
- the real API (`node apps/api/dist/main.js`, this branch built with `tsc`) on `127.0.0.1:4000`
- DB `ngfw_w10`: schema reset first, as the e2e harness does, then migrated at boot
- the repo's `FakeAgent` (`apps/api/src/testing/fake-agent.ts`, run with `tsx`) on `/run/ngfw-test/w10/fake-agent.sock`;
  its StreamStats sends worker CPU 1.5 % every 2 s, which is a healthy sample for the rule below
- a webhook receiver on `127.0.0.1:4051` (slot sub-port 4000+51)

Secrets were sourced from `/run/ngfw-test/w10/{pg,api}.env` into the environment and were never printed. The live run
used the build of `aa915672`. The later commit `65cdd7c8` only changes formatting and a type-only `export`, and the
transpiled JavaScript of both versions compares identical (`cmp`).

**Run 1**: commit an alarm rule and a webhook target through the API:
```
PATCH /api/v1/config/management -> 200
POST /api/v1/config/commit -> 200 {"status":"applied","revision":{"id":1,…,"author":"admin","comment":"S-alarms-restart-rebuild verify",…},…,"summary":{"created":0,"updated":7,"deleted":0,"unchanged":0,"failed":0,"reverted":0},…}
GET /api/v1/config (running) -> 200; management.alarms =
{ "rules": { "cpu": { "op": "gt", "forSec": 0, "metric": "worker_cpu_percent", "enabled": true, "targets": ["ops"],
                      "severity": "warning", "threshold": 90 } },
  "targets": { "ops": { "url": "http://127.0.0.1:4051/hook", "kind": "webhook" } } }
 alarm_rows
------------
          0
webhooks so far: 0
API run 1 (pid 197563) stopped
```
**While the API is down**, insert two active alarms: `cpu` (its rule exists) and `gone/wan0` (its rule is not configured):
```
INSERT 0 2
 id | rule | instance | state  |            raised_at             | cleared_at
----+------+----------+--------+----------------------------------+------------
  1 | cpu  |          | active | 2026-10-01 20:52:32.568059+03:30 |
  2 | gone | wan0     | active | 2026-10-01 20:52:32.568059+03:30 |
```
**Run 2 (the restart)**: the API log, the webhook receiver, then psql:
```
[Nest] 199122  - 10/01/2026, 8:57:48 PM     LOG [alarms] rebuilt 2 active alarm(s) from the database
ngfw-api listening on http://127.0.0.1:4000 (docs at /api/docs)
--- webhook receiver (1 POSTs):
{"method":"POST","url":"/hook","body":{"kind":"cleared","rule":"cpu","instance":"","metric":"worker_cpu_percent","severity":"warning","message":"worker_cpu_percent = 97 (threshold 90)","at":"2026-10-01T17:27:50.643Z"}}

 id | rule | instance |  state  |            raised_at             |          cleared_at
----+------+----------+---------+----------------------------------+-------------------------------
  1 | cpu  |          | cleared | 2026-10-01 20:52:32.568059+03:30 | 2026-10-01 20:57:50.558+03:30
  2 | gone | wan0     | cleared | 2026-10-01 20:52:32.568059+03:30 | 2026-10-01 20:57:48.487+03:30

 id |                ts                | severity |     code      |                                          message                                          |                              data
----+----------------------------------+----------+---------------+-------------------------------------------------------------------------------------------+----------------------------------------------------------------
  2 | 2026-10-01 20:57:48.519437+03:30 | info     | ALARM_CLEARED | interface_link_down on wan0 = 1 (threshold 1) — cleared: its rule is no longer configured | {"rule": "gone", "reason": "rule-removed", "instance": "wan0"}
  3 | 2026-10-01 20:57:50.640891+03:30 | info     | ALARM_CLEARED | worker_cpu_percent = 97 (threshold 90)                                                    | {"rule": "cpu", "instance": ""}
```
The orphan was cleared at boot with its reason and no webhook. `cpu` was cleared by the first healthy sample, about
2 s after boot, with exactly one `cleared` webhook. About 27 s later (roughly 13 more healthy samples) the API view
showed no further webhooks:
```
GET /api/v1/state/alarms -> 200
  #2 gone/wan0 cleared raisedAt=2026-10-01T17:22:32.568Z clearedAt=2026-10-01T17:27:48.487Z
  #1 cpu/- cleared raisedAt=2026-10-01T17:22:32.568Z clearedAt=2026-10-01T17:27:50.558Z
GET /api/v1/state/dashboard -> 200 {"alarms":{"active":0,"bySeverity":{"info":0,"warning":0,"critical":0}},"agent":{"reachable":true}}
20:58:17
webhook POSTs received so far: 1
```
**Run 3 (another restart)**: nothing left to rebuild or clear. One `cleared` per row:
```
ngfw-api listening on http://127.0.0.1:4000 (docs at /api/docs)
webhook POSTs total: 1
  state  | count
---------+-------
 cleared |     2
 alarm_cleared_events
----------------------
                    2
```
**Run 4 (contrast, unfixed code)**: only `dist/features/dashboard-prom-alarms/alarms.service.js` was swapped for a
`transpileModule` of `HEAD~1`'s source. I checked the method first: transpiling the fixed source reproduces the `tsc`
output except the decorator-metadata guard form, which resolves to the same classes at runtime. Then I inserted a fresh
active `cpu` row (id 3):
```
dist now UNFIXED (rebuildState refs: 0)
INSERT 0 1
listening seen at 21:01:15
checked at 21:01:32
ngfw-api listening on http://127.0.0.1:4000 (docs at /api/docs)
webhook POSTs total: 1
 id | rule | instance |  state  |            raised_at             |          cleared_at
----+------+----------+---------+----------------------------------+-------------------------------
  1 | cpu  |          | cleared | 2026-10-01 20:52:32.568059+03:30 | 2026-10-01 20:57:50.558+03:30
  2 | gone | wan0     | cleared | 2026-10-01 20:52:32.568059+03:30 | 2026-10-01 20:57:48.487+03:30
  3 | cpu  |          | active  | 2026-10-01 20:55:56.855963+03:30 |
```
After 15 s of healthy samples, row 3 was still `active` and no webhook had arrived. That is the Q4 bug.
**Run 5 (fixed `dist` restored, `cmp` checked)**: the stranded row is cleared about 2 s after boot, with one webhook:
```
[Nest] 210665  - 10/01/2026, 9:02:14 PM     LOG [alarms] rebuilt 1 active alarm(s) from the database
ngfw-api listening on http://127.0.0.1:4000 (docs at /api/docs)
--- webhook receiver (2 POSTs total):
{… "at":"2026-10-01T17:27:50.643Z"}}   (run 2)
{"method":"POST","url":"/hook","body":{"kind":"cleared","rule":"cpu","instance":"","metric":"worker_cpu_percent","severity":"warning","message":"worker_cpu_percent = 97 (threshold 90)","at":"2026-10-01T17:32:16.109Z"}}
  3 | cpu  |          | cleared | 2026-10-01 20:55:56.855963+03:30 | 2026-10-01 21:02:16.052+03:30
  4 | 2026-10-01 21:02:16.105383+03:30 | ALARM_CLEARED | worker_cpu_percent = 97 (threshold 90)                                                    | {"rule": "cpu", "instance": ""}
```
**Cleanup**:
- every process I started was stopped by PID: the API, the fake agent and its `tsx` child, and the hook receiver
- ports 4000 and 4051 are free, and the fake-agent socket is removed (`FakeAgent.stop()`)
- the 7 Valkey keys under my prefix `ngfw:w10:alarmsverify:` in db 10 were deleted
- no secret key file was created
- the `ngfw_w10` data from the run is left in place (the e2e global setup recreates the database)

### 4. CI gate — PASSED
`TMPDIR=/tmp/g-w10 /root/NGFW/tools/ci-slot.sh --base main` (D-224 wrapper) on commit `ccf7048f`. This status-file
update is the only change after it (docs only):
```
ci-slot: /run/lock/ngfw-ci-1.lock + /run/lock/ngfw-heavy-1.lock after 0s (canary 0.33s, MemAvailable 15 GB) — tools/ci.sh --base main (log /root/ngfw-wt/logs/ci/gate-S-alarms-restart-rebuild-1001-211230.log)
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m11s
  generate + generated-output gate                   3m33s
  forbidden patterns (+ gitleaks)                    0m07s
  packet-trace ban on the shared VPP (D-128)         0m01s
  ip classify reset after every shell interface create (D-185)   0m01s
  slot resource scheme (1..32, no collisions)        0m03s
  typecheck · build (turbo — tests OFF)           9m18s
  apps/agent: go build + go vet (tests OFF)          1m03s
  apps/cli: make build (tests OFF)                   0m07s
  test/ Go modules, unit mode (…21 modules…)   0m22s
  warnings:
    - TESTS OFF (plan/NO-TESTS, D-210): compile-only gate — typecheck · build · go vet; no lint, no unit/integration tests, no apply-startup harness
    - control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:1:… // ALLOW: test-only openssl cert   (not this task's file; 3 lines)
    - deploy/vpp harness and integration skipped (tests OFF, D-210)
  mode quick · wall time 14m49s · logs /root/ngfw-wt/logs/ci/S-alarms-restart-rebuild-20261001-211230-239232

CI GATE PASSED
```

## Out of scope / not done
- `docs/user/dashboard/dashboard-prom-alarms.md` is not my file. A follow-up line for its owner: alarms survive an API
  restart; removing or disabling a rule clears its active alarms with a `reason` (`rule-removed` / `rule-disabled`) on
  the `ALARM_CLEARED` event, the `alarm.events` message and the webhook payload (a removed rule notifies nobody).
- An e2e case in `apps/api/test/e2e/dashboard-prom-alarms.e2e.test.ts` (not my file). The live slot run above covers it.
- A row whose rule still exists but **no longer matches** it (the rule's metric changed, or its `interface` filter now
  excludes the instance) is rebuilt but never sampled again. This gap already exists in the live commit path (prune only
  checks existence and `enabled`). Not addressed: the task scopes orphans to "rule no longer exists". Open question 1.
- `interface_link_down` is fed only by LINK_UP/LINK_DOWN events, with no initial snapshot. A rebuilt link alarm
  therefore clears on the next link-up event, not at boot. Unchanged and outside this task.

## Decisions (for the LOG)
- **Orphans go through the existing `pruneRules` path**, because the rebuild runs before `reload()`. There is no second
  clear path. Disabled rules count as orphans, exactly as in the live prune path. The reasons are `rule-removed` and
  `rule-disabled`.
- **The reason is not stored on the `alarm` row** (there is no column, and schema changes are out of scope). It goes on
  the `ALARM_CLEARED` system event (message suffix + `data.reason`), the `alarm.events` message and the webhook payload.
  All three are additive optional fields.
- **A rebuild failure fails `start()`** (boot), the same as `reload()` already does. A database that is unreachable at
  boot already fails earlier boot steps (migrations, bootstrap admin).
- Process: heavy steps (install, builds, tsc, vitest) ran through `/root/NGFW/tools/heavy.sh` and the gate through
  `/root/NGFW/tools/ci-slot.sh`, per the manager's D-224 message received mid-task.

## Open questions (none blocking)
1. Should a rebuilt row whose rule still exists but no longer matches it (metric changed, or the instance is outside
   the rule's `interface` filter) also be cleared, for example with reason `rule-changed`? The same gap applies when
   such an edit is committed live. If yes, it is a small follow-up in this feature directory.
