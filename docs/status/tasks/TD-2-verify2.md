# TD-2 — verify round 2 (verdict)

Reviewer: independent verify agent (did not write this code). Envelope `TD-2.verify3.md` (manager ngfw-46, 14:00), continuing
`TD-2-verify2-partial.md`. Branch `task/TD-2` @ `899e919`. The product code is fix round 2 (`7ab9c83..84210b7`); `899e919` only adds
the partial verify note. Base `main` @ `fc0fe68`. Slot 7 (`w7`, DB `vrx_w7`, Valkey db 7). I read TD-2-verify.md (V1–V5),
the "Fix round 2" section of TD-2.md, TD-2-review.md, TD-2-questions.md Q6/Q7, TD-2-contract.md and LOG D-097/D-100/D-102/D-105.
I also re-read the round-2 code diff `ae52906..HEAD -- apps/api/src` and the new file `test/e2e/td2-verify.e2e.test.ts`.
I changed no product or test code. The negative controls restored the round-1 `apps/api/src` temporarily and put HEAD back afterwards (pasted below).

## Summary of the runs

| # | run | result |
|---|---|---|
| 1 | `TMPDIR=/tmp/g-td2 tools/ci.sh --base main` (short TMPDIR, per the manager's correction) | **CI GATE PASSED**, wall time 4m55s, turbo 30/30, generated-output gate clean, gitleaks clean, agent `make` ok, log `/root/ngfw-wt/logs/ci/TD-2-20260924-144845-457906` |
| 1a | `TMPDIR=/tmp/claude-0/ci-tmp-TD-2v tools/ci.sh --base main` (the envelope's first TMPDIR, run before the correction) | **CI GATE PASSED**, wall time 9m38s, log `/root/ngfw-wt/logs/ci/TD-2-20260924-140127-121647` |
| 2 | `eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`, run 3 times (14:14, 14:22, 14:28) | **red all 3 times**: 3, 3 and 4 failed / 57–58 passed / 3 skipped. Every failure is in `td2-verify.e2e.test.ts`, and each run starts with `V1 — 40 runs` hitting the 30 s default `testTimeout`. The other files pass every time (config 15, stream 3, td2 14, td2-review 11, auth 10). See finding **T1**. |
| 2a | the same suite with `--testTimeout=180000 --reporter=verbose` (14:35) | **61 passed, 3 skipped (64)**. This matches the worker's pasted numbers. V1 took 19.2 s. |
| 2b | `td2-verify.e2e.test.ts` alone, verbose (the worker's isolated command, 14:26) | **8 passed (8)**. V1 took 16.5 s. |
| 3 | negative controls: `td2-verify.e2e.test.ts` against `git checkout ae52906 -- apps/api/src` | **7 failed, 1 passed (8)**. V2 ×3, V3 and V5 fail on the round-1 code for the behavioural reason each test targets. V1 and V4 fail too. After `git checkout HEAD -- apps/api/src`, `git status` is clean. |

Product verdict: every V1–V5 fix holds. None of the 8 runs above broke a security assertion on the HEAD code. Even in the red runs,
V1 finished in the background with `working afterwards 0, api_key rows left 0` (run 3). The red suite is a test-harness defect
(T1), not a product defect. It must still be fixed before merge, because `tools/ci.sh full` runs this suite (`pnpm -r run test:integration`).

## 1. CI — `TMPDIR=/tmp/g-td2 tools/ci.sh --base main` (HEAD 899e919)
```
START 2026-09-24T14:48:45+03:30 HEAD 899e919 TMPDIR=/tmp/g-td2
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/TD-2
branch    task/TD-2 @ 899e919   (base: main)
tools     node v22.23.2 · pnpm 12.5.1 · go1.26.0 · buf 1.73.0 · golangci-lint 2.13.2 (pinned) · gitleaks 8.30.1 (pinned)
caches    pnpm store /root/.local/share/pnpm/store/v11 · turbo /root/.cache/vrx-turbo · go /root/.cache/go-build
logs      /root/ngfw-wt/logs/ci/TD-2-20260924-144845-457906

== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  packages/api-client/src/generated/schema.d.ts
ok — contract commit(s) on the branch:
  7ab9c83 contract(api-client): regenerate — users password 200 body gains discardedCandidate (TD-2 verify V5)
  097e014 contract(api-client): regenerate — /auth/password documents 429 (TD-2)
  d7c83e2 contract(api-client): regenerate — users password, /health schema, safe-text patterns, lock ownerKey, redacted secret changes (TD-2)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify round 2 — partial (stopped at manager handover)
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings

== tools (golangci-lint, gitleaks) ==
golangci-lint 2.13.2
gitleaks 8.30.1

== install (pnpm --frozen-lockfile --prefer-offline) ==
Lockfile is up to date, resolution step is skipped Done in 316ms using pnpm v12.5.1

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~303525 bytes (303.52 KB) in 862ms no leaks found

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m4.196s

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	2.907s; ok  	ngfw/agent/internal/agent	15.844s; ok  	ngfw/agent/internal/contracttest	6.465s; ok  	ngfw/agent/internal/descriptors/abf	1.797s; ok  	ngfw/agent/internal/descriptors/acl	1.935s; ok  	ngfw/agent/internal/descriptors/adl	1.873s; ok  	ngfw/agent/internal/descriptors/af_packet	6.239s; …

== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.066s;
integration tests inside these modules skip here (VRX_INTEGRATION unset); 'tools/ci.sh full' runs them on the CI slot

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m57s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   2m07s
  apps/agent: make lint test build                   0m40s
  test/ Go modules, unit mode (test/integration/smoke)   0m03s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify round 2 — partial (stopped at manager handover)
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
  mode quick · wall time 4m55s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-144845-457906

CI GATE PASSED
EXIT 0 2026-09-24T14:53:40+03:30
```
Run 1a (long TMPDIR, before the manager's correction) ended the same way:
```
  mode quick · wall time 9m38s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-140127-121647

CI GATE PASSED
EXIT 0 2026-09-24T14:11:05+03:30
```

## 2. api e2e, slot 7

### Runs 1–3 as committed (`eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`)
Run 1 (14:14; other slots' CI running, 1-min load about 15–19 on 32 cores):
```
 ❯ test/e2e/td2-verify.e2e.test.ts (8 tests | 3 failed) 219376ms
   × … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows 30038ms
     → Test timed out in 30000ms.
   ✓ … > V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can  549ms
   ✓ … > V2 … > commit: old sessions, refresh chain and API keys end; old password 401, new 200; audited via: config  1016ms
   ✓ … > V2 … > confirmed commit: sessions live while pending, end at confirm; a NEW user with a hash is no reset  3536ms
   ✓ … > V2 … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows  28273ms
   ✓ … > V3 — Valkey fails after the commit: 200, old access token and refresh chain still refused, audited  9041ms
   × … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone 30009ms
     → Test timed out in 30000ms.
   × … > V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered) 30004ms
     → Test timed out in 30000ms.
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 32587ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 10947ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 10392ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 8248ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 12607ms
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
 FAIL  test/e2e/td2-verify.e2e.test.ts > TD-2 verify fixes e2e (round 2, D-102)
Error: Hook timed out in 60000ms.
 ❯ test/e2e/td2-verify.e2e.test.ts:46:3
     46|   afterAll(async () => h?.close());
 Test Files  1 failed | 5 passed | 1 skipped (7)
      Tests  3 failed | 58 passed | 3 skipped (64)
e2e teardown: deleted 1484 Valkey keys vrx:w7:e2e:* in db 7
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 1 2026-09-24T14:21:56+03:30
```
Run 2 (14:22, load at start `2.05 11.64 19.00`):
```
   × … > V1 — 40 runs: … 30179ms
     → Test timed out in 30000ms.
   ✓ … > V1 — a session from before the reset cannot mint (401); … 14988ms
   × … > V2 … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows 30006ms
     → Test timed out in 30000ms.
   ✓ … > V3 — …  853ms
   ✓ … > V4 — …  1636ms
   × … > V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered) 569ms
     → expected 409 to be 200 // Object.is equality
 ❯ test/e2e/td2-verify.e2e.test.ts:428:7          (the lockkey API key's PATCH /config/interfaces/loop7777)
 Test Files  1 failed | 5 passed | 1 skipped (7)
      Tests  3 failed | 58 passed | 3 skipped (64)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 1 2026-09-24T14:26:07+03:30 load: 7.42 16.51 20.15
```
Run 3 (14:28, load at start `5.13 11.86 17.82`):
```
V1: 40 admin resets, 4 minting loops each: minted 465 keys (465 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
   × … > V1 — 40 runs: … 30033ms
     → Test timed out in 30000ms.
   × … > V2 … > 10 runs: … 30013ms
     → Test timed out in 30000ms.
   × … > V4 — … 30013ms
     → Test timed out in 30000ms.
   × … > V5 — … 30005ms
     → Test timed out in 30000ms.
Error: Hook timed out in 60000ms.
 ❯ test/e2e/td2-verify.e2e.test.ts:46:3
 Test Files  1 failed | 5 passed | 1 skipped (7)
      Tests  4 failed | 57 passed | 3 skipped (64)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 1 2026-09-24T14:35:18+03:30 load: 5.42 10.24 15.88
```
The `V1:` line is printed by the V1 test body. In run 3 it appeared during the V2 race test, so V1 kept running after vitest had
timed it out. It still reports 0 survivors and 0 rows.

### Run 2a — the same suite with a larger time budget (`… test:integration --testTimeout=180000 --reporter=verbose`)
```
START 2026-09-24T14:35:37+03:30 HEAD 899e919 load: 4.25 9.66 15.57
V1: 40 admin resets, 4 minting loops each: minted 570 keys (570 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
 ✓ … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows 19176ms
 ✓ … > V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can 333ms
V2 commit: {"me":401,"refresh":401,"apiKey":401,"keyRows":0,"oldLogin":401,"newLogin":200}
 ✓ … > V2 … > commit: old sessions, refresh chain and API keys end; old password 401, new 200; audited via: config 492ms
 ✓ … > V2 … > confirmed commit: sessions live while pending, end at confirm; a NEW user with a hash is no reset 1125ms
V2 race: 10 config-path resets: minted 188 (188 in flight during the commit) → working afterwards 0, rows left 0
 ✓ … > V2 … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows 6368ms
 ✓ … > V3 — Valkey fails after the commit: 200, old access token and refresh chain still refused, audited 271ms
V4: reset answered 500 after a deadlock
 ✓ … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone 1366ms
 ✓ … > V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered) 401ms
 Test Files  6 passed | 1 skipped (7)
      Tests  61 passed | 3 skipped (64)
e2e teardown: deleted 1571 Valkey keys vrx:w7:e2e:* in db 7
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 0 2026-09-24T14:40:39+03:30 load: 20.27 16.81 16.71
```
H2 in the same run: `H2: 60 runs, 120 hammered chains + … racing logins that got in → survivors 0` (green in all four full runs).

### Run 2b — `td2-verify.e2e.test.ts` alone (`tools/lab lock shared pnpm exec vitest run -c vitest.e2e.config.ts --reporter=verbose test/e2e/td2-verify.e2e.test.ts`)
```
V1: 40 admin resets, 4 minting loops each: minted 557 keys (557 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
 ✓ … > V1 — 40 runs: … 16534ms
 ✓ … > V1 — a session from before the reset cannot mint (401); … 291ms
V2 commit: {"me":401,"refresh":401,"apiKey":401,"keyRows":0,"oldLogin":401,"newLogin":200}
 ✓ … > V2 … > commit: … 435ms
 ✓ … > V2 … > confirmed commit: … 630ms
V2 race: 10 config-path resets: minted 189 (189 in flight during the commit) → working afterwards 0, rows left 0
 ✓ … > V2 … > 10 runs: … 5599ms
 ✓ … > V3 — … 241ms
V4: reset answered 500 after a deadlock
 ✓ … > V4 — … 1342ms
 ✓ … > V5 — … 372ms
 Test Files  1 passed (1)
      Tests  8 passed (8)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 0 2026-09-24T14:28:09+03:30
```

## 3. Negative controls — the new e2e against the round-1 API (`ae52906`)
```
$ git checkout ae52906 -- apps/api/src && git status --short
M  apps/api/src/auth/auth.service.ts
M  apps/api/src/auth/tokens.service.test.ts
M  apps/api/src/auth/tokens.service.ts
M  apps/api/src/commit/commit.service.test.ts
M  apps/api/src/commit/commit.service.ts
M  apps/api/src/common/principal.ts
M  apps/api/src/datastore/pg-repo.ts
M  apps/api/src/datastore/repo.ts
M  apps/api/src/db/schema.ts
M  apps/api/src/testing/memory-repo.ts
M  apps/api/src/users/users.controller.ts
M  apps/api/src/users/users.service.ts
$ tools/lab lock shared pnpm exec vitest run -c vitest.e2e.config.ts --reporter=verbose --testTimeout=180000 test/e2e/td2-verify.e2e.test.ts
START 2026-09-24T14:41:40+03:30 apps/api/src @ ae52906, tests @ 899e919
V1: 40 admin resets, 4 minting loops each: minted 2944 keys (2892 by requests in flight during the reset) → working afterwards 372, api_key rows left 372
 × … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows 178340ms
   → expected 372 to be +0 // Object.is equality
 ✓ … > V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can 7524ms
V2 commit: {"me":200,"refresh":200,"apiKey":200,"keyRows":1,"oldLogin":401,"newLogin":200}
 × … > V2 … > commit: old sessions, refresh chain and API keys end; old password 401, new 200; audited via: config 35929ms
   → expected { me: 200, refresh: 200, …(4) } to deeply equal { me: 401, refresh: 401, …(4) }
 × … > V2 … > confirmed commit: sessions live while pending, end at confirm; a NEW user with a hash is no reset 11943ms
   → expected 200 to be 401 // Object.is equality
V2 race: 10 config-path resets: minted 466 (421 in flight during the commit) → working afterwards 486, rows left 2673
 × … > V2 … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows 22340ms
   → expected 486 to be +0 // Object.is equality
 × … > V3 — Valkey fails after the commit: 200, old access token and refresh chain still refused, audited 179ms
   → expected 500 to be 200 // Object.is equality
V4: reset answered 500 after a deadlock
 × … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone 1373ms
   → expected false to be true // Object.is equality
 ❯ test/e2e/td2-verify.e2e.test.ts:395:33
    395|     expect(await visibleAtCall).toBe(true);
 × … > V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered) 195ms
   → expected { Object (self, passwordSet, ...) } to deeply equal { Object (passwordSet, self, ...) }
-   "apiKeysKept": 2,
    "apiKeysRevoked": [],
-   "keepApiKeys": true,
    "passwordSet": true,
    "self": false,
 ❯ test/e2e/td2-verify.e2e.test.ts:408:34
      Tests  7 failed | 1 passed (8)
e2e teardown: deleted 158 Valkey keys vrx:w7:e2e:* in db 7
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 1 2026-09-24T14:48:29+03:30
$ git checkout HEAD -- apps/api/src && echo RESTORED && git status --short
RESTORED
$ git status --porcelain | wc -l
0
```
What each failure shows:
- **V2**: with the round-1 code, a config-path hash change leaves the old access token, refresh cookie and API key alive (`me/refresh/apiKey 200`, `keyRows 1`). The confirm path is the same (old token 200 after confirm). The race leaves 486 working keys. This is exactly the verify-1 probe P2.
- **V3**: round 1 answers **500** when the revocation script fails after the commit. HEAD answers 200 and still refuses the old session.
- **V5**: round 1 audits `{passwordSet, self, apiKeysRevoked: []}` with no `keepApiKeys` or `apiKeysKept`.
- **V1**: 372 working keys (the worker's own control showed 314).
- **V4**: against the unmodified round-1 code, the aborted-reset half of the test passes, because round 1 deadlocks on the candidate row
  before it reaches `replaceInflightHash`. The second half catches the defect: on a successful reset, `replaceInflightHash` ran
  before the hash was visible to another connection. The worker's control patched the call position differently; the result is the same.
- The one test that passes on round 1 (`a session from before the reset cannot mint`) is covered there by round 1's in-process
  access-token revocation. It is a regression guard, not a control.

## V1–V5

| finding | status | evidence (this round) |
|---|---|---|
| V1 keys minted during a reset survive | **fixed** | `createApiKey` runs one transaction: `SELECT credential_gen … FOR SHARE` (conflicts with the reset's `NO KEY UPDATE`), a re-check (`sessionCurrent` for a JWT; the key row still exists for an ApiKey), then `INSERT` (`auth.service.ts:249-294`). Login matches `credential_gen` in its conditional `UPDATE`; refresh continues a chain only under the column's current value. e2e 0 survivors / 0 rows in runs 2a, 2b and (in the background) 3; the negative control gives 372. |
| V2 config-path reset bypasses D-097 | **fixed** (D-102, D-105) | `pg-repo.ts` `syncUsers`: an existing user's changed hash, in the promote transaction, bumps the generation, clears the lockout, deletes the keys and releases their locks; `commit.service.ts` `configResets` revokes and audits `via: config` after the commit. A new user, or the same hash staged again, is not a reset. A confirmed commit resets at confirm. e2e green; the negative control fails ×3. |
| V3 Valkey fails after the commit | **fixed** | `tokens.service.ts` `revokeUser`: the in-process revocation and `bus.sessions` come first; Valkey runs in `try/catch` → `persisted:false` → audit `revocationPersisted:false`, answer 200. The old refresh chain is refused by the PostgreSQL generation even though `rtfam` survives. e2e green; the negative control answers 500. |
| V4 inflight hash before commit | **fixed** | `users.service.ts`: `replaceInflightHash` is called after `db.transaction` resolves, inside `exclusive`. The lock order is app_user → api_key → candidate → pending. e2e green; the negative control fails at the visibility check. |
| V5 opt-out and discard audited | **fixed** | `users.controller.ts:72-89`: `keepApiKeys`, `apiKeysKept`, `discardedCandidate` in the audit row; `discardedCandidate` in the 200 body. e2e green; the negative control fails. |

## Checklist (REVIEW-PROMPT)
1. **Contract.** `git diff --name-only main...HEAD` over the contract paths lists only `packages/api-client/src/generated/schema.d.ts`.
   Round 2 adds 2 lines (`discardedCandidate: boolean` in the `POST /users/{name}/password` 200 body) in its own commit
   `7ab9c83 contract(api-client): …`, which TD-2-contract.md lists. The change is additive, on a route that exists only on this branch. CI's generated-output gate is clean.
   Migration `0003_td2_credential_gen.sql` is additive (`ADD COLUMN … DEFAULT 0 NOT NULL`). Main has only 0000/0001, so there is no number collision.
   `git merge-tree --write-tree main HEAD` gives `merge-tree clean vs main fc0fe68`.
2. **Real verification.** The e2e runs on the host PostgreSQL 18.6 and Valkey (db 7), with only the in-process fake agent for Apply. It asserts on DB rows, audit rows, Valkey keys and HTTP results.
3. **Restart safety / 4. VPP provenance.** Not applicable: round 2 touches no agent, VPP or binapi file (`git diff --name-only ae52906..HEAD -- apps/web apps/agent tools packages/schema packages/proto` → 0).
   L3 (revocations survive an API restart) is still green.
5. **Shared host.** Everything used slot 7: `vrx_w7`, Valkey db 7 keys `vrx:w7:e2e:*`. Every run's teardown printed `ok nothing named vrx_w7 / vrx_w7 remains`.
   After the runs, `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0` and `dbsize` → `0`. No server was started, and nothing listens on 3700/5700/9171.
6. **Security.** No `exec`, `child_process` or shell in the diff. The new log lines carry only the user id or username and the error message. No hash reaches an audit row: the e2e asserts `argon2id` never appears in `audit_log`.
7. **Transaction semantics.** A promote that fails after `syncUsers` rolls back the generation and key deletes with it, and `configResets` runs only after the commit. The V4 abort leaves the hash, the inflight copy and the sessions alone.
8–10. **UI / scope / i18n.** Round 2 has no web changes and no scope creep: every change maps to V1–V5 or D-102.
11. **Tests actually run.** CI: matches. e2e: the pasted `61 passed | 3 skipped` reproduces **only** with a larger time budget (2a) or file by file (2b). The committed suite was red 3 of 3 times → T1.

## Findings (by severity)

### T1 — Medium (required before merge; test-only): the new e2e file overruns the 30 s default timeout on a shared host, and one overrun cascades into 3–5 failures and a hung `afterAll`
`apps/api/test/e2e/td2-verify.e2e.test.ts`:
- `:138` (`V1 — 40 runs`) and `:287` (`V2 … 10 runs`) have no timeout argument, so they run under `vitest.e2e.config.ts` `testTimeout: 30_000`.
- `:80-135` `mintDuring` sets `stop = true` only on the happy path (`:127`).
- `:369-379` V4 synchronises with `sleep(300)` and then waits for `p` inside an open transaction that holds `config_candidate` FOR UPDATE and the target's `app_user` row, with no `lock_timeout`.

**Failure scenario (observed 3 of 3 times today).**
1. V1's wall time is 16.5–19.2 s on a quiet host. It went over 30 s in every committed-config run while other slots' CI was running (load 10–20 on 32 cores).
2. Vitest fails the test, but its body keeps running. The 40-run loop keeps resetting `minter` through `commits.exclusive`, and in run 2 the V2 race loop keeps staging and committing as admin.
3. **V4 hangs.** A background reset or commit holds the commit mutex and waits for the candidate row, which V4's helper transaction holds. V4's reset waits for the mutex, and the helper transaction waits on V4's reset in JavaScript. PostgreSQL cannot see this cycle, so it never breaks it. V4 times out, and the helper transaction stays open.
4. **V5 fails.** It times out (runs 1 and 3), or it gets `409` because the background loop holds the candidate lock (run 2, `:428`).
5. **`afterAll` hangs** (`Hook timed out in 60000ms`).

`tools/ci.sh full` runs `pnpm -r run test:integration`, so merging this as is would turn the manager's full gate red or flaky on main.
The product behaviour is not affected: every run that completed, including the backgrounded V1 in run 3, reports 0 survivors and 0 rows.

**Fix (test only, about 15 lines; no product change, no contract change):**
1. Give the two loop tests an explicit budget, `it('V1 — 40 runs …', fn, 180_000)` and `it('10 runs …', fn, 120_000)`, or lower V1 to 20 runs.
2. In `mintDuring`, wrap the trigger in `try { … } finally { stop = true; await Promise.all(loops); }`. Also add a file-level `aborted` flag, set in `afterEach` (or `onTestFinished`), that the loops and the `for (run…)` bodies check, so a timed-out test stops its work.
3. In V4:
   - start the helper transaction with `SET LOCAL lock_timeout = '10s'`;
   - replace `sleep(300)` with a poll of `pg_stat_activity` (`wait_event_type = 'Lock'` for the reset's backend) before the `update app_user`;
   - `Promise.race` `p` against a timeout, so the helper transaction always ends.
4. Evidence to add: the full `test:integration` run 3 times in a row, green with the committed config, while another slot's CI is running.

(Optional, same pattern, pre-existing from round 1: `td2-review.e2e.test.ts` `H2 — 60 runs` also relies on the 30 s default. It took 13–20 s here.)

### Info
- **Q6 is answered by D-105.** An admin who changes their own hash through the config API ends their own sessions and keys. The code matches (`commit.service.ts` `configResets`, no `keep`).
- **`syncUsers` reads the "before" hashes without a row lock.** This is correct only because the commit mutex serialises the hash writers of one API process. It is the existing single-process assumption; note it for any future multi-process API.
- **The `config.password-reset` audit rows carry `sourceIp: null`.** The committer's IP is on the commit's own audit row, not on the per-user reset rows. This is cosmetic.
- **The V4 test depends on PostgreSQL choosing the reset as the deadlock victim.** When the reset wins instead, the test fails fast with 200 ≠ 500, which is not a false pass. T1's fix item 3 also makes this deterministic.
- **Left for TD-4 (D-100):** "disable bumps the generation". It now fits into `syncUsers` next to the D-102 reset (worker's Q7).

## Verdict
The V1–V5 fixes are correct and proven by tests that fail without them (the negative controls above). The contract change is additive and properly labelled, and the CI gate passes.
The new e2e file is not yet merge-safe on this shared host (T1). The fix is test-only; after it, a 3× green full `test:integration` run is the merge evidence.

**APPROVE WITH CHANGES**
