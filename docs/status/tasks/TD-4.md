# TD-4: auth hardening follow-ups from TD-2 (D-100 (1)–(3))

branch `task/TD-4` · worktree `/root/ngfw-wt/TD-4` · slot 8 (`w8`) · base `task/TD-2@4391a44` (speculative, D-114),
`task/TD-2@ff9811f` merged in, then **main merged after TD-2 landed** (`e16b0f0`, TD-2 squash `7082cc6`/`f3037ab`) ·
**Fix round 1 (review H1/M1/L1, D-124): code tip `30454b9`, CI GATE PASSED 19:16, whole API e2e 72 passed | 3 skipped — see "Fix round 1" below.** Round 0: code tip `e16b0f0 Merge branch 'main' into task/TD-4 (TD-2 squash 7082cc6/f3037ab, D-112)`. CI (below) ran on this tip; after it only `docs/status/tasks/TD-4*` changed. **CI GATE PASSED** (17:37–17:42, second run; the first run at 17:29 failed only in `@ngfw/api#test`, where all 11 files hit vitest's `[vitest-worker]: Timeout calling "fetch"` before any test ran (turbo's parallel tasks on top of other workers' gates, load ≈ 25–40); nothing changed between the two runs, and the same suite passes standalone, 102/102, below)

Work was interrupted once by a usage-limit stop (16:40, salvage commit `fad03d3`); this file was written by the continuing
worker. Everything below was re-run after the stop.

## What was built

| D-100 | Rule | Where |
|---|---|---|
| (1) login transport | `POST /auth/login` from a non-loopback peer over plain HTTP → **403 `tls-required`** (problem+json). The check runs **first**: before the per-IP rate limiter, the user lookup and argon2, so it counts no failed login and does not touch the lockout. Audited `auth.login`, `result: failure`, `status: 403`, `after: {reason: 'tls-required'}`, `user_id: null`, username only. Loopback and TLS unchanged. OpenAPI documents the 403 (summary unchanged). | `apps/api/src/auth/transport.ts` (new: `isLoopback`, `secureTransport`, `tlsRequired`, moved from `users.service.ts`, which now imports them), `auth.controller.ts` (`login` passes `secureTransport(req)`), `auth.service.ts` (`login`) |
| (2) step-up | `ApiKeyBody.current` (optional string 1–1024, the same shape as `PasswordBody.current`). **JWT caller:** transport check first (403 `tls-required`), then missing → 400 pointer `/current`; wrong → 403 `forbidden` via `registerFailure` (counts toward the lockout); the locking guess and every later attempt → 403 `locked`, no argon2, no `api_key` row. **API-key caller:** no `current` needed; a `current` it does send is checked the same way (**fix round 1: refused with 400 `current-not-allowed-with-api-key`, never checked — D-124**). `checkCurrent` reads hash **and** `credential_gen` in one row read, runs argon2 **outside** any transaction, and refuses a stale JWT session (401) before argon2 so it never feeds the lockout. Inside TD-2's `FOR SHARE` transaction (V1 ordering kept) the generation must still equal the checked one (401 otherwise), the account must not have been locked meanwhile (403 `locked`) and must not be disabled (401). Audit `after: {name, role, via: 'jwt'｜'apikey'}` — never the password. Summaries of `Auth_login` / `Auth_createApiKey` unchanged; the rule is in the field and 403 descriptions. | `auth.controller.ts` (`createApiKey`), `auth.service.ts` (`createApiKey`, `checkCurrent`, `accountLocked`) |
| (3) disable ends sessions | `syncUsers` (pg + memory): an **existing** user whose `disabled` flips false → true in the promoted snapshot gets `credential_gen + 1` in the promote transaction and is returned next to the D-102 resets with `reasons: ['disabled']`. After the commit `configResets` calls `tokens.revokeUser(id, gen)` (access tokens, refresh chains, WebSockets end now) and writes **one** `config.user-disabled` row (`resource: user/<name>`, `after: {disabled: true, via: 'config', revision, txnId}`). Confirmed commit: at **confirm** (the hook runs after promote, as D-102). No bump for a re-enable, a user created disabled, or the same flag staged again. Hash change + disable in one commit: **one** bump, one `revokeUser`, **two** rows (the D-102 `config.password-reset` row byte-identical). A disable alone does not clear `failed_logins`/`locked_until` and **does not delete API keys**: they are refused while the account is disabled (`authenticateApiKey` checks `app_user.disabled`) and work again after a re-enable (e2e below). | `datastore/repo.ts` (`PasswordReset.reasons`), `datastore/pg-repo.ts` + `testing/memory-repo.ts` (`syncUsers`), `commit/commit.service.ts` (`configResets`) |
| callers | CLI `api-key create`: with a `login`/`session` credential it sends `current` from `--password-file`, else prompts `Current password: ` without echo (`a.readSecret`); no terminal → usage error naming a terminal, `--password-file` or an API key; an API-key credential sends nothing. REPL e2e answers the prompt (the "password never on the terminal" check still holds). `live.sh` sends `current`. SDK doc example updated. The JWT key bodies of the api e2e carry `current`. | see minimal hunks |

### Minimal hunks outside the owned files (each one only what the prompt allows)
- `apps/api/src/datastore/repo.ts` — `PasswordReset.reasons` + two doc comments (the reset type only)
- `apps/api/src/datastore/pg-repo.ts` — `PgConfigTx.syncUsers` only (reads `disabled` with the hashes; disable-only bump)
- `apps/api/src/testing/memory-repo.ts` — `syncUsers` only
- `apps/api/src/commit/commit.service.ts` — `configResets` only (the post-promote hook); `commit.service.test.ts` — 3 new cases
- `apps/api/test/e2e/{auth,td2,td2-review,td2-verify}.e2e.test.ts` — `current` added to the JWT key-creation bodies, nothing else
- `apps/cli/internal/cli/cmd_op.go` — `apiKeyCreate` only; `apps/cli/internal/cli/cli_test.go` — `TestAPIKeyCreateStepUp`
- `apps/cli/test/e2e/repl_e2e_test.go` — answers the prompt (2 lines + comment)
- `test/topology/sdk-terraform-ansible/live.sh` — one line (`"current"`)
- `docs/user/system/sdk-terraform-ansible.md` — the key-creation example
- generated, never hand-edited: `packages/api-client/src/generated/schema.d.ts` (`contract(api-client)` commit `37b48e9` + `TD-4-contract.md`),
  `apps/cli/internal/api/operations_gen.go`, `sdk/python/vrx/_generated/*` (TD-4's `current` plus TD-2's un-regenerated drift, see questions 6)

## Out of scope (not built)
- The product nginx rule "`/api` is never proxied from plain :80" (D-100 (1)) belongs to **P10**: `transport.ts` states the assumption, nothing
  else changed. `tools/app` does exactly that kind of relay today (questions 2).
- Users page / apps/web (no API-key screen exists; questions 5), revocation on role demotion or user deletion (questions 3), the committer's own
  session on the config path, per-request Valkey revocation, schema single-line patterns (D-100 (4)), MFA/RADIUS/password policy, migrations,
  env vars, any transport bypass switch.

## Evidence

### Unit — `pnpm --filter @ngfw/api test` (new: `auth/transport.test.ts` ×3, `commit.service.test.ts` D-100 (3) ×3 on the memory repo)
```
 ✓ src/auth/transport.test.ts (3 tests) 33ms
 ✓ src/commit/commit.service.test.ts (24 tests) 6711ms
 Test Files  11 passed (11)
      Tests  102 passed (102)
```
The three `commit.service.test.ts` cases: disable → bump + `revokeUser(id, 1)` + one `config.user-disabled` row, then the same flag staged again
and a re-enable → nothing, a new disable after the re-enable → `revokeUser(id, 2)`, a user created disabled → gen 0; hash + disable → one bump,
two rows (D-102 row unchanged, no hash in the rows); confirmed commit → nothing while pending, bump + revoke at confirm, `revocationPersisted: false`
recorded when Valkey fails.

CLI: `make -C apps/cli … test` green (below); `go test -count=1 -run TestAPIKeyCreateStepUp -v ./internal/cli/` → `--- PASS: TestAPIKeyCreateStepUp`
(API key: no `current`, no prompt; session + `--password-file`: `current` in the body, never on the terminal; session, no terminal, no file:
usage error naming the three ways out, no request sent, no key file).

### e2e — whole API suite on slot 8 (17:12–17:19, load 36 → 15)
```
$ eval "$(tools/lab env 8)"
$ tools/lab lock shared pnpm --filter @ngfw/api test:integration
V1: 40 admin resets, 4 minting loops each: minted 381 keys (381 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
V2 race: 10 config-path resets: minted 99 (99 in flight during the commit) → working afterwards 0, rows left 0
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 74677ms
H2: 60 runs, 120 hammered chains + 48 racing logins that got in → survivors 0
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 45562ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 79566ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 23274ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 12697ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 65030ms
(1) after 5 remote plain-HTTP logins: failed_logins 1, locked_until null
(2) wrong current ×3: [{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":1,"locked":false},{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":2,"locked":false},{"status":403,"type":"https://vrx.dev/problems/locked","failed":0,"locked":true}]
(2) key creation racing a generation change: 401
(3) after the disable commit: {"me":401,"refresh":401,"apiKey":401,"ws":4403,"genBump":1,"keyRows":1,"login":401}
(3) after the re-enable commit: {"genBump":1,"login":200,"oldToken":401,"apiKey":200,"auditRows":1}
(3) confirmed commit: pending {"me":200,"apiKey":200} → confirmed {"me":401,"apiKey":401}
secrets: 17 passwords checked against {"audit":54,"events":6} rows and 1 API log lines → found in: []
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests) 29326ms
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
 Test Files  7 passed | 1 skipped (8)
      Tests  71 passed | 3 skipped (74)
e2e teardown: deleted 1286 Valkey keys vrx:w8:e2e:* in db 8
ok     nothing named vrx_w8 / vrx_w8 remains
```
TD-2's 61 tests + TD-4's 10 pass; the 3 agent-int tests skip. TD-2's V1 race is still `0 working keys, 0 rows`.

What the 10 TD-4 tests assert (`apps/api/test/e2e/td4-auth-hardening.e2e.test.ts`):
- **(1)** one real loopback failure first (so "unchanged" is a visible 1), then 5 remote plain-HTTP logins with right and wrong passwords → all 403
  `tls-required`; `failed_logins` stays 1, `locked_until` null, no `rl:login:192.0.2.10:*` key; 5 audit rows `{user_id: null, username, source_ip:
  192.0.2.10, after: {reason: 'tls-required'}, status: 403}`; loopback login → 200 (clears the one failure).
- **(2)** JWT without `current` → 400 `/current`; remote plain-HTTP JWT caller → 403 `tls-required`; right `current` → 201, the key authenticates,
  audited `via: 'jwt'`; wrong ×3 → 403, 403, 403 `locked` (`failed_logins` 1, 2, then locked), then the right password → 403 `locked`, no new
  `api_key` row; API-key caller without `current` → 201, audited `via: 'apikey'`; a stale JWT → 401 before argon2, `failed_logins` untouched;
  a generation change committing between the check and the insert → 401, no row; no password in `audit_log`, `system_event` or the API log (every
  Nest log level recorded, plus stdout/stderr).
- **(3)** user X holds an access token, a refresh cookie, an open WebSocket and an API key; the admin commits `disabled: true` → access 401 at once,
  refresh 401, WebSocket closed 4403, API key 401, gen +1, the key row kept, one `config.user-disabled` row; re-enable → login 200, the old access
  token still 401, the API key works again, no second row; confirmed commit → alive while pending, cut off at confirm; hash change + disable in one
  commit → one bump, rows `config.password-reset` (D-102, unchanged) + `config.user-disabled`, a new user created disabled → gen 0.

(An earlier run at 16:59 was stopped by me: the host load was 147 on 32 cores and td2-verify/td2-review failed on **timeouts only**; its DB and
739 Valkey keys were removed by hand — `pg-test.sh drop w8`, `unlink` by `vrx:w8:e2e:*`.)

### Negative control for (3) — TD-2's `syncUsers` patched back, restored with `git checkout`
```
$ git show ff9811f:apps/api/src/datastore/pg-repo.ts > apps/api/src/datastore/pg-repo.ts   # TD-2 syncUsers
 apps/api/src/datastore/pg-repo.ts | 60 +++++++++++++--------------------------
 1 file changed, 19 insertions(+), 41 deletions(-)
$ tools/lab lock shared pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/td4-auth-hardening.e2e.test.ts -t 'commit disabled'
 RUN  v3.2.7 /root/ngfw-wt/TD-4/apps/api
create role vrx_w8
create database vrx_w8 (owner vrx_w8)
check  vrx_w8 as vrx_w8 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w8/pg.env (0600) · DSN <redacted>
(3) after the disable commit: {"me":200,"refresh":401,"apiKey":401,"ws":4403,"genBump":0,"keyRows":1,"login":401}
 ❯ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests | 1 failed | 9 skipped) 26132ms
   × TD-4 auth hardening e2e (D-100) > (3) disabling an account through the config API ends its sessions at once > commit disabled: true → access token, refresh, WebSocket and API key end now; one audit row; re-enable → login 200, the old token stays dead, the key works again 823ms
     → expected { me: 200, refresh: 401, …(5) } to deeply equal { me: 401, refresh: 401, …(5) }
 FAIL  test/e2e/td4-auth-hardening.e2e.test.ts > TD-4 auth hardening e2e (D-100) > (3) disabling an account through the config API ends its sessions at once > commit disabled: true → access token, refresh, WebSocket and API key end now; one audit row; re-enable → login 200, the old token stays dead, the key works again
AssertionError: expected { me: 200, refresh: 401, …(5) } to deeply equal { me: 401, refresh: 401, …(5) }
- Expected
+ Received
  {
    "apiKey": 401,
-   "genBump": 1,
+   "genBump": 0,
    "keyRows": 1,
    "login": 401,
-   "me": 401,
+   "me": 200,
    "refresh": 401,
    "ws": 4403,
  }
 ❯ test/e2e/td4-auth-hardening.e2e.test.ts:410:21
 Test Files  1 failed (1)
      Tests  1 failed | 9 skipped (10)
   Start at  17:22:16
   Duration  74.03s (transform 39.12s, setup 0ms, collect 45.03s, tests 26.13s, environment 4ms, prepare 276ms)
e2e teardown: deleted 7 Valkey keys vrx:w8:e2e:* in db 8
drop   database vrx_w8
drop   role vrx_w8
ok     nothing named vrx_w8 / vrx_w8 remains
Error: ERR_PNPM_RECURSIVE_EXEC_FIRST_FAIL
  × "pnpm recursive exec" failed in /root/ngfw-wt/TD-4/apps/api
exit=1
$ git checkout apps/api/src/datastore/pg-repo.ts && git status --porcelain
 M apps/api/test/e2e/td4-auth-hardening.e2e.test.ts
```
With TD-2's `syncUsers` the disable commit leaves the **old access token at 200** (`genBump: 0`). Refresh and the API key were already refused by
TD-2 (both read `app_user.disabled`), and TD-2's relay closes the WebSocket on its `usersChanged` re-check; the access token is exactly the gap
D-100 (3) closes. (A first negative run before the test change below timed out waiting for that WebSocket close — `websocket close: no answer
within 10000 ms` — so the test now compares a socket that stays open as a value instead of throwing; commit `f955f48`, positive run below.)
```
$ tools/lab lock shared pnpm --filter @ngfw/api exec vitest run -c vitest.e2e.config.ts test/e2e/td4-auth-hardening.e2e.test.ts   (after f955f48)
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests) 6108ms
 Test Files  1 passed (1)
      Tests  10 passed (10)
ok     nothing named vrx_w8 / vrx_w8 remains
```

### Generated outputs
```
$ pnpm gen
@ngfw/web:gen: cache bypass, force executing 5592e00acdf12396
@ngfw/web:gen: $ echo 'web: nothing to generate'
@ngfw/web:gen: web: nothing to generate

 Tasks:    6 successful, 6 total
Cached:    0 cached, 6 total
  Time:    2m25.911s 

$ git status --porcelain packages/api-client/src/generated packages/api-client/openapi.json
(end porcelain)
$ make -C apps/cli gen docs lint test
make: Entering directory '/root/ngfw-wt/TD-4/apps/cli'
go run ./cmd/vrx-opgen -in ../../packages/api-client/openapi.json -out internal/api/operations_gen.go
vrx-opgen: internal/api/operations_gen.go written
go run ./cmd/vrx-docgen -out ../../docs/user/cli/reference.md
vrx-docgen: ../../docs/user/cli/reference.md written
ok: apps/cli talks only to the REST API
go vet ./...
0 issues.
go test -race -count=1 ./...
?   	ngfw/cli/cmd/vrx	[no test files]
?   	ngfw/cli/cmd/vrx-docgen	[no test files]
?   	ngfw/cli/cmd/vrx-opgen	[no test files]
ok  	ngfw/cli/internal/api	1.814s
?   	ngfw/cli/internal/api/opgen	[no test files]
ok  	ngfw/cli/internal/cli	4.730s
ok  	ngfw/cli/internal/cpath	1.174s
ok  	ngfw/cli/internal/jschema	1.273s
?   	ngfw/cli/internal/lineedit	[no test files]
ok  	ngfw/cli/internal/render	1.208s
ok  	ngfw/cli/internal/safe	1.143s
ok  	ngfw/cli/test/e2e	1.165s
make: Leaving directory '/root/ngfw-wt/TD-4/apps/cli'
make exit=0
$ git status --porcelain apps/cli docs/user/cli
(end porcelain)
$ sdk/gen.sh --check --openapi packages/api-client/openapi.json
python sdk: 43 operations, 357 models, 1 write-only + 22 secret-ref pointer patterns → /root/ngfw-wt/TD-4/sdk/python/vrx/_generated
terraform: vrx_interface 6 JSON names, 1 helpers; 1 write-only + 22 secret-ref pointer patterns, 4 keyed arrays → internal/provider
gen: clean — sdk/python/vrx/_generated sdk/terraform/internal/provider/zz_*_gen.go
sdk exit=0
$ git status --porcelain
(end)
```

### CLI REPL e2e on slot 8 (lock held only for the run; agent + API built from the merged tip `e16b0f0`)
```
$ systemctl show vpp -p NRestarts
NRestarts=0
$ deploy/dev/pg-test.sh create w8
create role vrx_w8
create database vrx_w8 (owner vrx_w8)
check  vrx_w8 as vrx_w8 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w8/pg.env (0600) · DSN <redacted>
$ tools/lab lock shared bash -c 'apps/cli/test/devstack.sh start && make -C apps/cli e2e; apps/cli/test/devstack.sh stop'
agent  pid 1385189 owner w8 socket /run/vrx-test/w8/agent.sock
api    pid 1385208 http://127.0.0.1:3800 (admin password in /run/vrx-test/w8/admin.pw)
make: Entering directory '/root/ngfw-wt/TD-4/apps/cli'
VRX_INTEGRATION=1 go test -count=1 -v ./test/e2e/
=== RUN   TestREPLConfirmedCommitAutoRevertAndRBAC
--- PASS: TestREPLConfirmedCommitAutoRevertAndRBAC (13.44s)
=== RUN   TestReviewFixesOnTheRealStack
    review_e2e_test.go:36: commit: 7 error: 400 invalid query
          at /comment (comment): control characters (C0/C1, DEL), line/paragraph separators (U+2028/U+2029) and bidi overrides (U+202A–U+202E, U+2066–U+2069) are not allowed
--- FAIL: TestReviewFixesOnTheRealStack (2.54s)
FAIL
FAIL	ngfw/cli/test/e2e	16.005s
FAIL
make: *** [Makefile:38: e2e] Error 1
make: Leaving directory '/root/ngfw-wt/TD-4/apps/cli'
api stopped (pid 1385208)
agent stopped (pid 1385189)
removed slot secrets and logs from /run/vrx-test/w8 (use stop --keep to keep them)
exit=2
$ deploy/dev/pg-test.sh drop w8
drop   database vrx_w8
drop   role vrx_w8
ok     nothing named vrx_w8 / vrx_w8 remains
$ systemctl show vpp -p NRestarts
NRestarts=0
$ vppctl show interface | grep loop8 (after)
(none)
```
`TestREPLConfirmedCommitAutoRevertAndRBAC` (the TD-4 hunk: `api-key create … file …` → `Current password: ` answered without echo; the
transcript check `the password appeared on the terminal` holds) **passes**. `TestReviewFixesOnTheRealStack` **fails, not because of TD-4**: it
commits a comment containing ESC/OSC bytes (P13 H1), which TD-2's safe-text rule on `comment` now refuses with 400 (`commit.controller.ts`,
`common/text.ts` — untouched by TD-4, `git diff main` on them is empty). Questions 7.
The first REPL run (17:26, agent binary built at 16:07 from the pre-merge tree, i.e. without TD-5) failed earlier, at the confirmed commit:
`create interface.loopback/loop801: sanitize loop801 (sw_if_index 6, create): no clean sw_if_index obtained (VPP V19 quarantine): placeholder cap
reached before every freed classify table index was resurrected (16 placeholders)` — the shared VPP's freed classify indexes vs the old cap;
with main's agent (TD-5, D-105 cap) it passed. NRestarts 0 → 0 in both runs, no `loop8*` left.

### CI — `TMPDIR=/tmp/g-w8 tools/ci.sh --base main`
```
Thu Sep 24 05:37:47 PM +0330 2026
 17:37:47 up  7:50,  2 users,  load average: 22.21, 36.92, 37.76
e16b0f0 Merge branch 'main' into task/TD-4 (TD-2 squash 7082cc6/f3037ab, D-112)
$ TMPDIR=/tmp/g-w8 tools/ci.sh --base main
...
== contract guard: HEAD vs main ==
ok — contract commit(s) on the branch:
  37b48e9 contract(api-client): regenerate — login 403 tls-required, API-key step-up current (TD-4, D-100)
  (+ TD-2's three contract commits from its merged history)
...
Tasks:    30 successful, 30 total Cached:    23 cached, 30 total Time:    2m2.037s
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m34s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   2m03s
  apps/agent: make lint test build                   0m47s
  apps/cli: make lint test build                     0m11s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify round 2 — partial (stopped at manager handover)
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
  mode quick · wall time 4m46s · logs /root/ngfw-wt/logs/ci/TD-4-20260924-173747-1495299

CI GATE PASSED
exit=0
Thu Sep 24 05:42:33 PM +0330 2026
 17:42:33 up  7:55,  2 users,  load average: 42.20, 36.32, 36.77
```

### Cleanup
```
$ ps -eo pid,args | grep -E "TD-4|/run/vrx-test/w8|:3800" (my processes)
(none)
$ ss -ltn | grep -E ":(3800|5800|9181)\b"
(none)
$ deploy/dev/pg-test.sh list
(no vrx_w8)
$ valkey-cli -n 8 --scan --pattern 'vrx:w8:*' | wc -l
9
$ ls /run/vrx-test/w8
ls: cannot access '/run/vrx-test/w8': No such file or directory
$ systemctl show vpp -p NRestarts
NRestarts=0
$ vppctl show interface | grep -c "^ *loop8"
0
$ valkey-cli -n 8 --scan --pattern 'vrx:w8:cli:*' | xargs -r valkey-cli -n 8 unlink   # refresh keys the two devstack runs left
9
$ valkey-cli -n 8 --scan --pattern 'vrx:w8:*' | wc -l
0
```
The git-ignored build outputs were deleted: `apps/{api,web}/dist`, `packages/{api-client,proto,schema,ui-kit}/dist`, `apps/agent/bin`, `apps/cli/bin`.
Every process I started (two vitest runs, two devstack stacks) was stopped by PID; the lab lock is not held.

## Fix round 1 (review `d30d503` APPROVE WITH CHANGES · envelope `TD-4.fix1.md`, manager ngfw-46 · slot 8)

main merged first (`7adbb9a`, clean: TD-6, board/LOG, D-120..D-123). Then:

| finding | fix | where |
|---|---|---|
| **H1** step-up: no throttle before argon2, parallel guesses out-run the lockout, two `locked` texts act as an oracle | (a) In `createApiKey`, after the transport rule and the 400s and **before** `checkCurrent`/argon2: `tokens.hit('pwset:<user id>', 60) > VRX_PASSWORD_RATE_PER_MIN` → **429 `rate-limited`** (Valkey INCR, so it holds for parallel requests; keyed by the account id, not by key or session). The budget is the one `setPassword` already spends per caller (`pwset:<caller id>`): one per-account bound for both routes that check a current password, so a stolen JWT cannot double its guesses by alternating routes (D-TD4-8). No new env var. (b) `accountLocked()` has ONE body; the lock that a wrong guess causes now answers exactly like a guess refused while locked and like the transaction re-check. (c) e2e `td4-stepup-burst.e2e.test.ts` (new, own harness: `VRX_PASSWORD_RATE_PER_MIN=5`, `VRX_LOGIN_MAX_FAILURES=3`): 30 parallel step-ups from one JWT, right guess at #20; every argon2 verification is counted through a pass-through `vi.mock` wrapper of `verifyPassword`; the budget is `5 × minute windows touched` (a burst across a minute boundary gets two). Asserts: checks ≤ budget, the rest 429, one body for every `locked`, an `api_key` row only if the right guess got 201, only known answers, the audit reasons match the answers, no guess in `audit_log`. | `auth.service.ts` (`createApiKey`, `checkCurrent`, `accountLocked`), `test/e2e/td4-stepup-burst.e2e.test.ts` |
| **M1** failed key creations audited without a reason | The controller wraps the service call; on a `ProblemError` it sets `req.audit.after = {name, via, reason: <problem slug>}` and rethrows (`bad-request`, `tls-required`, `forbidden`, `locked`, `rate-limited`, `unauthorized`, `current-not-allowed-with-api-key`). Slug only: no detail, no password. td4 (2) audit assertions extended (the four rows of the JWT case, the five of the lockout case, the three refused API-key rows). | `auth.controller.ts` (`createApiKey`), `td4-auth-hardening.e2e.test.ts` |
| **L1 → D-124** (manager) | A `current` sent by an **API-key** caller → **400 `current-not-allowed-with-api-key`** (pointer `/current`), never checked: no argon2, no lockout count, no key. The transport rule still answers first (a password over remote plain HTTP → 403 `tls-required`). e2e: right and wrong `current` give byte-identical 400s, `failed_logins` stays 0, key rows unchanged. D-TD4-1 marked superseded. | `auth.service.ts`, `auth.controller.ts` (`current` description), `td4-auth-hardening.e2e.test.ts`, `docs/user/system/sdk-terraform-ansible.md` |

OpenAPI (summaries unchanged): `POST /auth/api-keys` documents a **429** (the per-account budget) and the `current` description says
"rate-limited per account … Not allowed with `Authorization: ApiKey` (400 `current-not-allowed-with-api-key`, never checked)".
Callers: the CLI sends `current` only with a `login`/`session` credential (unchanged, so it never reaches the D-124 400) and
creates one key per REPL e2e run; `live.sh` creates one key from a JWT. The CLI REPL e2e was therefore not re-run (no CLI or
devstack change in this round).

Commits: `8180648 fix(api): TD-4 review H1/M1/L1 …` · `220bf32 contract(api-client): regenerate — key step-up 429, current refused
with an API key (TD-4 fix 1, D-124)` (+ `TD-4-contract.md` "Fix round 1") · `30454b9 docs(user): …` · this status commit.

### H1 negative control — the burst e2e against the pre-fix code (`7adbb9a` = review tip `d30d503` + main; 18:35, load ≈ 13–22)
```
$ eval "$(tools/lab env 8)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration test/e2e/td4-stepup-burst.e2e.test.ts
H1 step-up burst (right guess at #20, MAX_FAILURES 3, VRX_PASSWORD_RATE_PER_MIN 5, 2 minute window(s)):
  18× 403 locked | the current password is wrong; the account is now locked  ← #0,11,12,14,15,16,17,18,19,21,22,23,24,25,26,27,28,29
  10× 403 locked | the account is locked after too many failed password checks  ← #1,2,3,4,5,6,7,8,9,20
  2× 403 forbidden | the current password is wrong  ← #10,13
H1 summary: {"argon2Checks":21,"rateLimited":0,"lockedAnswers":28,"distinctLockedBodies":2,"rightGuess":"403 locked","keyRows":0} (budget 10)
   × TD-4 step-up burst (review H1) > 30 parallel step-ups from one JWT (right guess at #20): ≤ 5 argon2 checks per minute window, 429 for the rest, one body for every locked answer 43888ms
     → expected { checksWithinBudget: false, …(4) } to deeply equal { checksWithinBudget: true, …(4) }
-   "checksWithinBudget": true,
+   "checksWithinBudget": false,
    "keyOnlyIfRightGot201": true,
-   "oneLockedBody": true,
+   "oneLockedBody": false,
    "onlyKnownAnswers": true,
-   "restRateLimited": true,
+   "restRateLimited": false,
 Test Files  1 failed (1)
      Tests  1 failed (1)
ok     nothing named vrx_w8 / vrx_w8 remains
```
21 guesses checked by argon2 (7× the lockout of 3), no 429, the right guess (#20) inside the "locked" set of the other text: the
review's reproduction, now as a committed test. The burst took 44 s (21 argon2 runs on a loaded host).

### H1 after the fix — both td4 files (18:42)
```
$ eval "$(tools/lab env 8)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration test/e2e/td4-stepup-burst.e2e.test.ts test/e2e/td4-auth-hardening.e2e.test.ts
H1 step-up burst (right guess at #20, MAX_FAILURES 3, VRX_PASSWORD_RATE_PER_MIN 5, 1 minute window(s)):
  2× 403 forbidden | the current password is wrong  ← #0,1
  3× 403 locked | the account is locked after too many failed password checks  ← #2,3,4
  25× 429 rate-limited | too many password checks; try again in a minute  ← #5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29
H1 summary: {"argon2Checks":5,"rateLimited":25,"lockedAnswers":3,"distinctLockedBodies":1,"rightGuess":"429 rate-limited","keyRows":0} (budget 5)
 ✓ test/e2e/td4-stepup-burst.e2e.test.ts (1 test) 2456ms
(2) wrong current ×3: [{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":1,"locked":false},{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":2,"locked":false},{"status":403,"type":"https://vrx.dev/problems/locked","failed":0,"locked":true}]
secrets: 17 passwords checked against {"audit":55,"events":6} rows and 1 API log lines → found in: []
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests) 23080ms
 Test Files  2 passed (2)
      Tests  11 passed (11)
ok     nothing named vrx_w8 / vrx_w8 remains
```
5 argon2 checks (= the budget), 25× 429, one `locked` body; the right guess was throttled, so no key row. The burst takes 0.4 s instead of 44 s.

### Unit — `pnpm --filter @ngfw/api test` (18:46)
```
 Test Files  11 passed (11)
      Tests  102 passed (102)
```

### e2e — whole API suite on slot 8 (19:16–19:20, load ≈ 23–28)
The first full run (18:47–18:59, load 52–68 from other workers' gates) had 5 failures, all `Test timed out in 30000ms` on
argon2-heavy tests (`auth.e2e` lockout ×10, four td4 cases whose own output lines had already printed) — the host-load class of
D-121/TD-12; the same tests passed standalone at 18:42 and in the re-run below, with no change in between.
```
$ eval "$(tools/lab env 8)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration
19:16:16
 19:16:16 up  9:28,  2 users,  load average: 27.57, 33.34, 36.05
30454b9 docs(user): API-key creation — per-account check budget, `current` refused with an API key (TD-4 fix 1, D-124)
(1) after 5 remote plain-HTTP logins: failed_logins 1, locked_until null
(2) wrong current ×3: [{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":1,"locked":false},{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":2,"locked":false},{"status":403,"type":"https://vrx.dev/problems/locked","failed":0,"locked":true}]
(2) key creation racing a generation change: 401
(3) after the disable commit: {"me":401,"refresh":401,"apiKey":401,"ws":4403,"genBump":1,"keyRows":1,"login":401}
(3) after the re-enable commit: {"genBump":1,"login":200,"oldToken":401,"apiKey":200,"auditRows":1}
(3) confirmed commit: pending {"me":200,"apiKey":200} → confirmed {"me":401,"apiKey":401}
secrets: 17 passwords checked against {"audit":55,"events":6} rows and 1 API log lines → found in: []
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests) 7547ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 14453ms
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 29376ms
V1: 40 admin resets, 4 minting loops each: minted 362 keys (362 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 39508ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 27090ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11639ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 12125ms
H1 step-up burst (right guess at #20, MAX_FAILURES 3, VRX_PASSWORD_RATE_PER_MIN 5, 1 minute window(s)):
  2× 403 forbidden | the current password is wrong  ← #0,1
  3× 403 locked | the account is locked after too many failed password checks  ← #2,3,4
  25× 429 rate-limited | too many password checks; try again in a minute  ← #5,6,7,8,9,10,11,12,13,14,15,16,17,18,19,20,21,22,23,24,25,26,27,28,29
H1 summary: {"argon2Checks":5,"rateLimited":25,"lockedAnswers":3,"distinctLockedBodies":1,"rightGuess":"429 rate-limited","keyRows":0} (budget 5)
 ✓ test/e2e/td4-stepup-burst.e2e.test.ts (1 test) 3222ms
agent integration test skipped: VRX_INTEGRATION is not 1
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
 Test Files  8 passed | 1 skipped (9)
      Tests  72 passed | 3 skipped (75)
e2e teardown: deleted 1327 Valkey keys vrx:w8:e2e:* in db 8
ok     nothing named vrx_w8 / vrx_w8 remains
exit=0
19:20:02
 19:20:02 up  9:32,  2 users,  load average: 23.09, 29.38, 34.04
```
TD-2's 61 + TD-4's 10 + the H1 burst 1 = 72 pass; the 3 agent-int tests skip. The V1 race is still `working afterwards 0, api_key rows left 0`.

### Generated outputs (after `8180648`)
```
$ pnpm gen
 Tasks:    6 successful, 6 total
$ git status --porcelain
 M packages/api-client/src/generated/schema.d.ts        → committed as 220bf32 contract(api-client) (+ TD-4-contract.md)
   (diff: the `current` description, and a new 429 response on Auth_createApiKey — additive)
$ make -C apps/cli gen docs lint test
vrx-opgen: internal/api/operations_gen.go written
vrx-docgen: ../../docs/user/cli/reference.md written
ok: apps/cli talks only to the REST API
0 issues.
ok  	ngfw/cli/internal/api	2.531s
ok  	ngfw/cli/internal/cli	2.292s
ok  	ngfw/cli/test/e2e	1.154s
make exit=0
$ git status --porcelain
(empty: operations_gen.go and reference.md unchanged)
$ sdk/gen.sh --check --openapi packages/api-client/openapi.json
python sdk: 43 operations, 357 models, 1 write-only + 22 secret-ref pointer patterns → /root/ngfw-wt/TD-4/sdk/python/vrx/_generated
terraform: vrx_interface 6 JSON names, 1 helpers; 1 write-only + 22 secret-ref pointer patterns, 4 keyed arrays → internal/provider
gen: clean — sdk/python/vrx/_generated sdk/terraform/internal/provider/zz_*_gen.go
sdk exit=0
```

### CI — `TMPDIR=/tmp/g-w8 tools/ci.sh --base main` (19:02–19:16, on `30454b9`; after it only `docs/status/tasks/TD-4*` changed)
The first run (19:00) stopped in the contract guard with a false "CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT": the guard's
`git log … | grep -q` pipeline under `pipefail` loses to SIGPIPE when the newest `contract(` commit is near the top (here line 2 of
45; 110 of 200 local runs of the same pipeline were false negatives). Nothing changed between the two runs; details and the one-line
fix in questions 10.
```
Thu Sep 24 07:02:07 PM +0330 2026
 19:02:07 up  9:14,  2 users,  load average: 62.75, 46.81, 37.20
30454b9 docs(user): API-key creation — per-account check budget, `current` refused with an API key (TD-4 fix 1, D-124)
$ TMPDIR=/tmp/g-w8 tools/ci.sh --base main
...
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  packages/api-client/src/generated/schema.d.ts
ok — contract commit(s) on the branch:
  220bf32 contract(api-client): regenerate — key step-up 429, current refused with an API key (TD-4 fix 1, D-124)
  37b48e9 contract(api-client): regenerate — login 403 tls-required, API-key step-up current (TD-4, D-100)
  7ab9c83 contract(api-client): regenerate — users password 200 body gains discardedCandidate (TD-2 verify V5)
  097e014 contract(api-client): regenerate — /auth/password documents 429 (TD-2)
  d7c83e2 contract(api-client): regenerate — users password, /health schema, safe-text patterns, lock ownerKey, redacted secret changes (TD-2)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-4): verdict
      review(TD-2): verify round 2 — partial (stopped at manager handover)
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
...
Tasks:    30 successful, 30 total Cached:    12 cached, 30 total Time:    3m26.434s  
...
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   4m00s
  forbidden patterns (+ gitleaks)                    0m07s
  lint · typecheck · unit tests · build (turbo)   3m28s
  apps/agent: make lint test build                   0m36s
  apps/cli: make lint test build                     0m15s
  test/ Go modules, unit mode (test/integration/smoke)   0m05s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m09s
  warnings:
    - uncommitted changes in the worktree — the gate checks the working tree, but only commits get merged:
       M docs/status/tasks/TD-4-questions.md
       M docs/status/tasks/TD-4.md
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-4): verdict
  mode quick · wall time 13m50s · logs /root/ngfw-wt/logs/ci/TD-4-20260924-190207-2206013
CI GATE PASSED
exit=0
Thu Sep 24 07:15:57 PM +0330 2026
 19:15:57 up  9:28,  2 users,  load average: 22.87, 33.00, 36.01
```

### Cleanup (19:21)
```
$ ps -eo pid,args | grep -E "ngfw-wt/TD-4|/run/vrx-test/w8|:3800" (my processes)
(none)
$ ss -ltn | grep -E ":(3800|5800|9181)\b"
(none)
$ deploy/dev/pg-test.sh list
(no vrx_w8)
$ valkey-cli -n 8 --scan --pattern 'vrx:w8:*' | wc -l
0
$ ls /run/vrx-test/w8
ls: cannot access '/run/vrx-test/w8': No such file or directory
$ systemctl show vpp -p NRestarts
NRestarts=1
$ flock -n -x /run/lock/vrx-lab.lock true  # nobody (me included) holds the lab lock
held by someone
```
The lab lock is held by other slots' runs; no process of mine exists (the three vitest runs and two CI runs ended; nothing was left
running between them). `NRestarts=1` (0 in round 0): this round started no agent and created no VPP object (the api e2e use the
in-process fake agent), so the restart is not from TD-4. Build outputs deleted: `apps/{api,web}/dist`,
`packages/{api-client,proto,schema,ui-kit}/dist`, `apps/agent/bin`, `apps/cli/bin`.

## Decisions taken (worker; none needed a LOG entry — all inside D-100's text)
| id | Decision | Options | Why |
|---|---|---|---|
| D-TD4-1 | ~~A `current` sent by an **API-key** caller is checked too (transport rule, argon2, lockout), not ignored~~ **superseded by the manager's D-124 (fix round 1): 400 `current-not-allowed-with-api-key`, never checked** (the transport rule still answers first) | (a) ignore it (b) 400 (c) check it | A password in a body is a password: it must not cross plain HTTP, and a guess must never be free. D-124: a role-capped key must not test its owner's password. |
| D-TD4-2 | Locked account on step-up → 403 `locked` (own problem type) without argon2; a stale JWT → 401 **before** argon2, never counted | 401 for both · 403 for both | The prompt asks 403 `locked`; a stale session could not mint anyway and must not be a lockout lever against the account. |
| D-TD4-3 | A disable alone bumps the generation only: no lockout reset, no API-key deletion | treat it as a D-102 reset | A disable is not a credential change; the prompt keeps keys (refused while disabled). |
| D-TD4-4 | The key transaction also refuses a disabled account (401), from the same `FOR SHARE` row read | rely on the auth-time check | An API-key request authenticated just before the disable commit could otherwise still mint. No extra query. |
| D-TD4-5 | One `PasswordReset` per user with `reasons: ('password'｜'disabled')[]` instead of a sibling list | a second return list + second hook | One bump and one `revokeUser` for hash + disable by construction; the D-102 audit row stays byte-identical. |
| D-TD4-6 | TD-2's generated drift (Python SDK, `operations_gen.go`) regenerated and committed here (`chore(sdk)`/`chore(cli)`) | leave it dirty | The prompt requires clean generated outputs; the merge with main was clean on these files. |
| D-TD4-7 | Merge with main: conflicted API files take ours (TD-2 + TD-4), `TD-2.md` takes main's | hand-merge each hunk | main's TD-2 content is byte-identical to `task/TD-2@ff9811f` for every conflicted API file; check: `git diff main` of the merge == `git diff ff9811f f955f48` on apps/packages/sdk/test/docs/user. |
| D-TD4-8 | (fix round 1) The step-up spends the **same** per-account budget as password changes (`pwset:<user id>`), not a sibling `keystep:` key | shared · sibling | One bound on current-password checks per account per minute across both routes: a stolen JWT cannot double its guesses by alternating them. Cost: an admin who made 5 password sets in a minute waits for the next minute to mint a key (429). |

## Open questions → `docs/status/tasks/TD-4-questions.md`
1 re-enable keeps API keys (default kept) · 2 plain-HTTP relays: `tools/app` :8080 → vite → 127.0.0.1:3000 · 3 demotion/deletion revocation gap ·
4 CLI help/reference text outside the hunk · 5 no API-key screen in apps/web · 6 TD-2 generated drift · 7 P13's `TestReviewFixesOnTheRealStack`
broken by TD-2's comment rule · 8 other speculative branches: old agent binaries hit the 16-placeholder cap on the shared VPP.
Fix round 1: 9 the self-service password route still has two `locked` wordings (bounded by `pwset:`, not changed) · 10 `tools/ci.sh`
contract guard false negative (`grep -q` + `pipefail` → SIGPIPE), measured 110/200 on this branch.
