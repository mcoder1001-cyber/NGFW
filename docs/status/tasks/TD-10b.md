# TD-10b: API auth, session and audit hardening (REVIEW-2026-09-24 §2.3, D-125)

branch `task/TD-10b` · worktree `/root/ngfw-wt/TD-10b` · slot 5 (e2e run as prefix `w5b`, see questions 8) · base
`task/TD-4@c05183a` (speculative, D-114) · started 2026-09-24 23:41 · paused by the usage limit 01:0x–03:40, resumed.
Commits: `5c71be8b` (implementation), `e19d9102` (unit + first e2e), `62d53507` **`contract(api-client)`**
(additive: 409/503 responses, logout description — `TD-10b-contract.md`), `bd48f747` (remaining e2e, older e2e follow the
new lockout store), then this file. **Review `175bed1c`: APPROVE WITH CHANGES → Fix round 1 (C1, C2/M1+L5, C3, L1, L4, L7) at
the end of this file.**

## What was built (per review item)

| item | behaviour now | where |
|---|---|---|
| **2.3a lockout** | The LOGIN lockout is per **(user, credential generation, client address)** in Valkey (`lkf:`/`lk:` keys, one Lua script): a guesser locks the account only for its own address; IPv6 is keyed by its /64, IPv4-mapped as IPv4. The **last admin** — an enabled admin with no other enabled admin who could still log in from that address (not locked there, not locked account-wide), decided atomically together with the other admins' keys — is **throttled** 1, 2, 4 … ≤ 60 s instead of locked; a `LOGIN_THROTTLED` system_event (one per admin and minute). A locked/throttled attempt is refused without counting and with the same 401 as a wrong password (no oracle); a success passes an atomic gate (`admit`) so a parallel failure that locked the address wins (P06 H1). The account-wide lock in `app_user` stays for password checks made **inside** a session (TD-4 step-up, TD-2 own-password change) and never hits the last admin there (the per-account budget `pwset:<id>` still bounds its guesses). A password reset or disable (new generation) leaves every old lock behind. | `auth/lockout.ts` (new), `auth/auth.service.ts` (`login`, `registerFailure`) |
| 2.3a break-glass | **`vrx-authctl`** (root only): `unlock <user>` clears the account-wide lock and every per-address lock/throttle/counter of the user; `locks` lists what is in force; `rotate-jwt-key` (P06 below). Talks to PostgreSQL and Valkey directly with the API's settings (environment + `--env-file`), so it works when nobody can log in; every unlock writes an `auth.break-glass-unlock` audit row (`username: root (break-glass)`) and a `BREAK_GLASS_UNLOCK` system_event; never prints a setting. P10 packages it (questions 6). | `auth/break-glass.ts`, `auth/break-glass-cli.ts` (new), `deploy/sbin/vrx-authctl` (new, shellcheck clean) |
| **2.3b trusted proxy** | `VRX_TRUST_PROXY` (default `loopback`; addresses, `<ip>/<bits>`, `linklocal`/`uniquelocal`, `none`; `/0`, `true`, `*` refused) → Fastify `trustProxy`. `sourceIp` is the client behind the trusted proxy; `requestProtocol` takes the X-Forwarded-Proto entry of the **outermost trusted hop** (appending proxies cannot be fooled; no entry → `http`). `secureTransport` = the client's protocol is https **or the client itself** is loopback — so the rate limit, the lockout, the audit row and the transport rule see one client. vite `xfwd: true` (dev and `vite preview`). | `app.ts`, `common/principal.ts`, `auth/transport.ts`, `config.ts` (3 settings, additive), `apps/web/vite.config.ts` (one line), `tools/app` (banner) |
| **2.3c logout** | Logout ends **that** login session: refresh chain deleted and its access tokens refused at once (per-sid revocation, in memory + `atrevsid:<sid>` in Valkey, reloaded at boot); the user's other sessions stay. Works with the refresh cookie (web UI) and/or a Bearer token (CLI). A token the store does not know ends nothing (before: a forged `<family>.<junk>` deleted anyone's chain). Refresh-token reuse now also revokes the session's access tokens. | `auth/tokens.service.ts`, `auth/auth.service.ts`, `auth/auth.controller.ts` |
| 2.3c demotion/deletion | **PENDING-session-revocation option 1** (recommended; isolated hunk that follows the product owner's answer): `syncUsers` returns `demoted` (lower role → generation bumped) and `deleted` (row gone → generation + 1) resets; `configResets` already revokes every reset → old tokens, refresh chains and WebSockets end at the commit. A demoted user logs in again with the new role; promotions end nothing. | `datastore/pg-repo.ts` (`syncUsers` only), `datastore/repo.ts` (`PasswordReset.reasons` union only) |
| **2.3d max age** | `VRX_SESSION_MAX_SEC` (default **12 h**) — absolute from the login, next to the sliding `VRX_REFRESH_TTL_SEC`: the refresh chain records its login time (`rtfam = <uid>:<gen>:<t0>`), a refresh after the maximum is refused (`session-expired`), the refresh cookie, the docs cookie and the access token never outlive the session. Chains from before TD-10b start their clock at their first refresh. | `auth/tokens.service.ts` (Lua), `auth/auth.service.ts`, `auth/cookies.ts` |
| **2.3e audit** | `auth.logout` rows (user, address, via); `auth.refresh` failure rows with their reason and user (`refresh-token-reuse`, `session-ended`, `session-expired`, `user-deleted`, `disabled`, `locked`, `credentials-changed`); unknown tokens and **401 mutations** aggregated per client, route and minute (a row at the 1st, 10th, 100th … occurrence, ≤ 200 keys a minute then one overflow row per action) — not awaited, never delays a 401. An audit **write failure** increments `AuditService.writeFailures` and writes one `AUDIT_WRITE_FAILED` system_event a minute (running count, folded count, SQLSTATE); the log shows the driver's cause, not drizzle's query-with-parameters. **Privileged routes fail closed**: users/own password set, API-key create/delete, secret create/delete write their row BEFORE the action; if that insert fails → 503 `audit-unavailable`, nothing runs; the row is completed after (a crash leaves `after.incomplete`). | `audit/audit.service.ts`, `audit/audit.interceptor.ts`, `audit/system-events.service.ts` (log line), `auth/auth.guard.ts` |
| **2.3g /api/docs** | Option (a): login and refresh also set `vrx_docs` = the session's access token (httpOnly, SameSite=Strict, path `/api/docs`, lifetime ≤ token/session); the docs hook accepts it for GET/HEAD only; every other route reads only `Authorization`. The Swagger UI, its assets and `swagger-ui-init.js` (which inlines the spec) load in a browser that is logged in to the web UI; logout clears it and the per-sid revocation kills it. The CLI keeps `GET /api/docs-json` with its header. | `auth/cookies.ts` (new), `app.ts` (docs hook), `auth/auth.controller.ts` |
| P06 key ring | `VRX_JWT_KEY_FILE`: one key per line, newest signs (`kid` header = truncated hash), all verify; re-read on change (stat ≤ every 5 s; a bad file keeps the old ring and logs); kid-less tokens from before are tried against the ring. `vrx-authctl rotate-jwt-key` puts a new key on top, keeps the previous one, drops older ones (atomic write, 0600, owner kept, directory fsync). | `auth/tokens.service.ts`, `auth/break-glass.ts` |
| P06 key-file owner check | `checkKeyFile`: regular file (no symlink), owner = the API's uid or root, no group/other bits; refused at boot with a message naming the path and the problem, never the content. Applied to the JWT ring; the secret store's key is TD-10a's file (questions 5). | `auth/key-file.ts` (new) |
| manager addendum (TD-10a M2) | Password set (`POST /users/{name}/password`, `POST /auth/password`) waits **≤ 1 s** for the commit lock, then **409 `commit-busy`** (`retryAfterSec: 2`, TD-10a's body); the queued section is abandoned and changes nothing. Local bound until TD-10a's `userExclusive` is on main (questions 2). | `users/commit-busy.ts` (new), `users/users.service.ts` |

### Outside the owned files (each minimal)
- `apps/api/src/config.ts` — three new settings (`VRX_TRUST_PROXY`, `VRX_SESSION_MAX_SEC`, `VRX_JWT_KEY_FILE`) and two comments; env settings live nowhere else.
- `apps/api/src/datastore/repo.ts` — `PasswordReset.reasons` union + its comment (the PendingCommit hunk of TD-10a is untouched).
- `apps/web/vite.config.ts` — `xfwd: true` on the one `/api` proxy (the "vite xfwd" of the envelope lives there; tools/app runs `vite preview` with this config; no row owns the file).
- `apps/api/test/e2e/auth.e2e.test.ts`, `td4-auth-hardening.e2e.test.ts` — only the assertions that read the old account-wide LOGIN lock (`app_user.failed_logins/locked_until` after failed logins) now read the per-address keys; the break-glass replaces the test's manual `UPDATE … locked_until`.
- `packages/api-client/src/generated/schema.d.ts` — regenerated only (contract commit `62d53507`).

## Decisions (options for the manager's LOG)
| # | Decision | Options | Chosen, why |
|---|---|---|---|
| 1 | Lockout key | (a) every password check per (user, address) in Valkey; (b) LOGIN per (user, address), checks inside a session keep TD-4's account-wide lock, the last admin exempt from both; (c) per account, last admin only throttled | **(b)**: closes the anonymous DoS (the review's case) and keeps TD-2/TD-4's verified semantics for session-held checks (a stolen session may lock its own account, D-097/D-100). (c) keeps the DoS for every other user; (a) rewrites TD-2/TD-4 behaviour and tests for no gain against a session holder (already bounded by `pwset:`). Residual, documented: a guesser that shares the admin's address and guesses continuously keeps that address throttled most of the time (never locked; other addresses and the break-glass work); distributed guessing gets MAX_FAILURES per address per lockout window (plus the per-address rate limit; IPv6 per /64). |
| 2 | Last admin | throttle 1-2-4…≤60 s per address; or never count; or lock anyway | throttle, decided atomically with the other admins' keys (parallel bursts on two admins lock at most one — e2e) |
| 3 | Logout revocation | (a) refresh family only (before); (b) per-sid access revocation, memory + Valkey, reloaded at boot; (c) generation bump (ends all sessions of the user) | **(b)**: logout must not log the user out of their other devices; (c) is what a password change does. Plus: only a known token acts; reuse detection revokes the sid too. |
| 4 | `VRX_SESSION_MAX_SEC` | 8 h / **12 h** / 24 h / off | 12 h (a working day); min 5 s, no "off" (no bypass switch); the cookies and access token capped at the end; the Valkey records live 5 min longer so a late refresh is audited as `session-expired`, not as junk |
| 5 | Audit write failure | (a) fail open + event + counter; (b) breaker: after a failure privileged routes 503 until a probe write succeeds; (c) write-ahead row for privileged routes, fail closed | **(c)** for passwords, API keys, secrets (the routes that change who can do what): no such change without its row, even for the first failure; everything else (a). Cost: one extra UPDATE per privileged request. |
| 6 | `/api/docs` | (a) cookie auth; (b) loopback-only | **(a)**, as a docs cookie carrying the session's access token (the refresh cookie itself is scoped to /api/v1/auth, and presenting it rotates the chain — a docs tab would trip reuse detection and log the web UI out). (b) keeps the docs unusable for the product owner's desktop browser. |
| 7 | 401 audit volume | row per 401; aggregate; none | aggregate per (client, route, reason, minute) at powers of ten, ≤ 200 keys/minute + overflow |
| 8 | Refresh success audit | audit every refresh; failures only | failures only (a session refreshes every ~15 min; the login row starts it) |
| 9 | Trusted hop without X-Forwarded-Proto | trust as TLS (old assumption); not TLS | **not TLS** — P10's nginx must send it (questions 6) |

## Evidence

### 1. Every new test FAILS on the base first
The implementation files were put back to the base (`git restore --source=c05183a6 --worktree` on the 16 modified files, the
6 new ones moved aside), the TD-10b tests kept; then HEAD restored (tree clean afterwards).

Unit (`npx vitest run` of the six test files):
```
   × AuditService (TD-10b) > writeAggregated: one row at the 1st, 10th, 100th … occurrence per key and minute
   × AuditService (TD-10b) > a failed write is counted and reported as ONE system_event a minute (fail open: write() resolves)
   × AuditService (TD-10b) > begin (privileged write-ahead) throws when the row cannot be written; finish updates the outcome
   × route guard > TD-10b: every fail-closed (privileged) audit route exists — a renamed route cannot silently fail open
   × route guard > TD-10b: the docs cookie opens only /api/docs, never an API route; a forged one opens nothing   → expected 401 to be 200
 FAIL  src/auth/break-glass-cli.test.ts      Error: Cannot find module './break-glass-cli.js'
 FAIL  src/auth/tokens.keyring.test.ts       Error: Cannot find module './break-glass.js'
 FAIL  src/auth/transport.test.ts            TypeError: Cannot read properties of undefined (reading 'length')   (VRX_TRUST_PROXY unknown)
 FAIL  src/users/commit-busy.test.ts         Error: Cannot find module './commit-busy.js'
 Test Files  6 failed (6)
```
e2e on the host PostgreSQL + Valkey (`tools/lab lock shared pnpm --filter @ngfw/api test:integration test/e2e/td10b-*.e2e.test.ts`, prefix w5b):
```
 × lockout: a wrong-password burst from address A locks the user for A only — the owner logs in from B        → expected 401 to be 200
 × lockout: the LAST admin is throttled (1 s, 2 s …), never locked                                               → expected null to be 'throttle'
 × lockout: a session-held password check never locks the last admin account-wide                              → expected [ '403 locked', '403 locked', …(1) ] to deeply equal [ '403 forbidden', …(2) ]
 × lockout: two enabled admins … also under parallel bursts                                                     → expected null to be 'lock'
 × lockout: break-glass (root-only vrx-authctl) …                                                               → Cannot find module '../../src/auth/break-glass.js'
 × session-max: refreshes do not extend a session past its maximum age                                          → expected 900 to be less than or equal to 5
 × audit: 2.3b plain HTTP from another machine behind the proxy → 403 tls-required                               → expected 200 to be 403
 × audit: 2.3b the login rate limit is per client, not one global bucket                                         → expected 429 to be 401
 × audit: 2.3e unauthenticated mutations leave a bounded trace                                                   → expected null to deeply equal [ ObjectContaining{…}, …(2) ]
 × audit: 2.3e an audit write failure … is counted and becomes a system_event                                   → expected undefined to be NaN
 × audit: 2.3e privileged routes fail CLOSED                                                                     → expected 201 to be 503
 × audit: 2.3g /api/docs works in a browser (the docs cookie)                                                    → (no vrx_docs cookie set)
 × audit: manager addendum (TD-10a M2): password set behind a busy commit lock → 409 commit-busy               → expected 200 to be 409
 × session: 2.3c logout ends THAT session's access tokens at once                                              → expected { s1: 200, s2: 200 } to deeply equal { s1: 401, s2: 200 }
 × session: 2.3c the revocation survives an API restart                                                         → expected { id: 3, username: 'op', …(4) } to be null
 × session: 2.3c a Bearer logout (the CLI) ends that session too                                                → expected 200 to be 401
 × session: a forged `<family>.<junk>` logs nobody out                                                          → expected 401 to be 200
 × session: 2.3e refresh-token reuse: the family AND its access tokens die                                       → expected 200 to be 401
 × session: 2.3e refresh failures carry their reason; junk is aggregated                                         → TypeError: Cannot read properties of null (reading 'map')
 × session: option 1: a DEMOTED user's old token dies at once                                                   → expected { demoMe: 200, demoRefresh: 200, …(1) } to deeply equal { demoMe: 401, demoRefresh: 401, …(1) }
 × session: option 1: a DELETED user's access token dies at once                                                → expected 200 to be 401
 Test Files  4 failed (4)
      Tests  21 failed (21)
ok     nothing named vrx_w5b / vrx_w5b remains
```

### 2. The same tests on TD-10b — whole API e2e suite (04:21–04:25, load ≈ 36, slot 5 as `w5b`)
```
$ eval "$(tools/lab env 5)"; export VRX_TEST_PREFIX=w5b VRX_VALKEY_DB=5 VRX_HTTP_PORT=3550
$ tools/lab lock shared pnpm --filter @ngfw/api test:integration
create role vrx_w5b · create database vrx_w5b (owner vrx_w5b)
2.3b via the proxy from 203.0.113.10: plain HTTP 403 https://vrx.dev/problems/tls-required, TLS 200
2.3b 7 logins from P: 401,401,401,401,401,401,429; then one from Q: 401
2.3e 13 unauthenticated commits → 3 audit rows
2.3e fail-open: counter 0 → 1; event {"severity":"error","subsystem":"audit","data":{"code":"23514","action":"PATCH /api/v1/config/*","failuresTotal":1,"foldedSinceLastEvent":0}}
2.3e fail-closed: key creation 503 https://vrx.dev/problems/audit-unavailable; password set 503
2.3g docs with the cookie: page 200, init.js 200; without: 401
commit-busy: 409 https://vrx.dev/problems/commit-busy after 1086 ms (lock held 3 s)
 ✓ test/e2e/td10b-audit.e2e.test.ts (7 tests) 6797ms
2.3d max 5s: login {"expiresIn":5,"tokenLife":5,"refreshCookie":5,"docsCookie":5}; refresh at +1.2 s 200 {"expiresIn":4,"refreshCookie":4}; refresh at +5.7 s 401
 ✓ test/e2e/td10b-session-max.e2e.test.ts (1 test) 8182ms
2.3c after logout of session 1: {"s1":401,"s2":200}
forged logout → the victim's refresh 200, access token 200
demotion: {"demoMe":401,"demoRefresh":401,"promoMe":200}
deletion: the deleted user's token on GET /api/v1/config → 401
 ✓ test/e2e/td10b-session.e2e.test.ts (8 tests) 5266ms
2.3a victim after 3 failures from A: from A 401, from B 200
2.3a last admin from C: inside the throttle 401, after 1.2 s 200
2.3a last admin, 3 wrong step-ups: 403 forbidden, 403 forbidden, 403 forbidden; then the right one: 201
2.3a parallel bursts from F on both admins → ["lock","throttle"]
break-glass CLI: exit 0; unlocked 'victim' (id 3): account-wide lock was not set, 1 per-address lock/counter key(s) removed
 ✓ test/e2e/td10b-lockout.e2e.test.ts (5 tests) 7832ms
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 37443ms
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 30233ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 11877ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 12279ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11475ms
(2) wrong current ×3: [{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":1,"locked":false},{"status":403,"type":"https://vrx.dev/problems/forbidden","failed":2,"locked":false},{"status":403,"type":"https://vrx.dev/problems/locked","failed":0,"locked":true}]
secrets: 17 passwords checked against {"audit":56,"events":6} rows and 1 API log lines → found in: []
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests) 6453ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 7710ms
  2× 403 forbidden | the current password is wrong  ← #0,1
  25× 429 rate-limited | too many password checks; try again in a minute  ← #2,…,9,13,…,29
  3× 403 locked | the account is locked after too many failed password checks  ← #10,11,12
 ✓ test/e2e/td4-stepup-burst.e2e.test.ts (1 test) 2636ms
 Test Files  12 passed | 1 skipped (13)
      Tests  93 passed | 3 skipped (96)          (skipped: test/integration/agent.int.test.ts without VRX_INTEGRATION)
e2e teardown: … · drop database vrx_w5b · drop role vrx_w5b
ok     nothing named vrx_w5b / vrx_w5b remains
```
The TD-4 secrets scan (every password of that file against audit_log, system_event and the API log) still finds nothing
with TD-10b's new rows and log lines. The TD-4 step-up burst (H1) is unchanged.

### 3. Unit — `pnpm --filter @ngfw/api test`
```
 Test Files  15 passed (15)
      Tests  131 passed (131)
```
New: `auth/transport.test.ts` (real Fastify with the app's trustProxy: direct peers, forged headers from a remote peer,
the appending-proxy forgery, two trusted hops, a hop without X-Forwarded-Proto, `none`, the setting's grammar, clientKey),
`auth/tokens.keyring.test.ts` (rotation keeps the previous key, drops the one before; reload in a running process; kid-less
legacy tokens; a forged kid; refused files: mode 0644, symlink, foreign owner, empty, short line — never echoing a key; a
file that turns bad keeps the ring; per-sid revocation with Valkey down; exp capped at the session end),
`audit/audit.service.test.ts`, `users/commit-busy.test.ts` (409 after the wait, the abandoned section never runs; a started
section is awaited), `auth/break-glass-cli.test.ts` (usage, root only, no value echoed), `auth/route-guard.test.ts` +2.

### 4. The tool itself (wrapper → built dist)
```
$ VRX_API_DIST=$PWD/apps/api/dist deploy/sbin/vrx-authctl --help | head -3      → usage …   exit 0
$ setpriv --reuid=65534 … bash deploy/sbin/vrx-authctl locks                    → vrx-authctl: root only   exit 1
$ deploy/sbin/vrx-authctl rotate-jwt-key <scratch>/jwt.keys   (twice)
rotated …/jwt.keys: 1 key(s), the new signing key first (new file, owner root: chown it to the API user); the API reloads it within 5 s
rotated …/jwt.keys: 2 key(s), the new signing key first; the API reloads it within 5 s
$ stat -c '%a %U' …/jwt.keys → 600 root ; keys in the file → 2
$ shellcheck deploy/sbin/vrx-authctl tools/app → clean
```
(`unlock` and `locks` against a real database are in the lockout e2e above: the CLI's `main()` with the harness settings.)

### 5. Generated outputs and the contract
```
$ pnpm --filter @ngfw/api-client gen          → schema.d.ts: +409/+503 on Users_setPassword and Auth_password, +503 on
                                                 Auth_createApiKey/Auth_deleteApiKey, Auth_logout 204 description (55+/1−)
$ make -C apps/cli gen docs                   → operations_gen.go, docs/user/cli/reference.md: unchanged
$ sdk/gen.sh --openapi packages/api-client/openapi.json → python sdk / terraform: unchanged
```
Committed as `62d53507 contract(api-client): …` with `TD-10b-contract.md` (additive only).

### 6. CI — `TMPDIR=/tmp/g-w5 tools/ci.sh --base main` (04:08–04:21, load 35–51)
```
== contract guard: HEAD vs main ==
ok — contract commit(s) on the branch:
  62d53507 contract(api-client): TD-10b — 409 commit-busy and 503 audit-unavailable on the password/API-key routes, logout description (additive)
  c05183a6 contract(api-client): TD-4 auth hardening — … (the base)
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no secret-shaped strings
ok: gitleaks — scanned ~296804 bytes (296.80 KB) in 1.1s no leaks found
Tasks:    30 successful, 30 total Cached:    12 cached, 30 total Time:    3m12.089s
  apps/agent: make lint test build                   1m15s
  apps/cli: make lint test build                     0m19s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m02s
  mode quick · wall time 12m08s · logs /root/ngfw-wt/logs/ci/TD-10b-20260925-040851-2994039
CI GATE PASSED
```
After the CI run only `docs/status/tasks/TD-10b*` changed.

## Out of scope (not built)
- The `tools/app` listener (TLS or loopback bind): follows PENDING-tools-app-transport (questions 1). The banner says what
  works meanwhile (SSH tunnel) and how to break glass on the lab stack.
- `configResets` audit rows for demotion/deletion, memory-repo parity, the secret store's owner-check call site, TD-10a's
  `userExclusive` swap (questions 2–5); a root-side password reset (questions 7); P10's nginx headers and packaging
  (questions 6); `TokensService.hit` fail-open on an EXEC error (questions 9); a `docs/user/` page (P10 with the packaging).
- Web UI: nothing to change — its logout already sends the refresh cookie, and it never reads `vrx_docs`.

## Cleanup
Valkey `vrx:w5b:*` deleted by the e2e teardown, database and role `vrx_w5b` dropped ("nothing named vrx_w5b remains"),
`/run/vrx-test/w5b` absent; the worktree's `dist/` directories and `apps/agent/bin` removed (rebuild with
`pnpm --filter "@ngfw/api^..." run build` before an e2e re-run); no process of mine is running; ports 3000/8080/9101 and the
running `tools/app` were not touched; no pkill.

## Fix round 1 (review `175bed1c`, APPROVE WITH CHANGES → manager's list)

Worked on `task/TD-10b` after `175bed1c`, no rebase (the merger: `reset --soft c05183a6`, `rebase --onto main`). Commits:
`fe41a982` C1 · `89e5ee5f` M1+L5, L1, L4, L7 · `f3011e01` + `cb74843d` C3 · `37611df9` test flake + lint · this file.

| item | fix | test that fails on the old code |
|---|---|---|
| **C1** (H1) | `tools/app` starts the API with **`VRX_TRUST_PROXY=none`** (the API env line + a comment naming `docs/decisions/PENDING-tools-app-transport.md`): vite's loopback peer is the client again = main's behaviour; the listener hunk of the answer removes it. The banner line I added in round 0 ("remote logins are refused") would have become false; it now says passwords cross the LAN in clear (lab exception) and how to tunnel. Nothing else in tools/app changed; tools/app was not run. | `transport.test.ts` "review C1": vite's xfwd headers for a remote plain-HTTP browser under `none` → accepted, and the tools/app API line carries `VRX_TRUST_PROXY=none` — on the previous tools/app: `expected 'VRX_HTTP_HOST="$API_HOST" VRX_HTTP_PO…' to match /(^|\s)VRX_TRUST_PROXY=none(\s|\\|$)/` |
| **C2** (M1) | `rotateKeyFile(path, apiUser)`: the ring may belong to **root or the API user** (`VRX_API_USER`, default `vrx`, from /etc/passwd); owner and group kept, mode 0600; a foreign owner, a symlink or any group/other bit is still refused; a new ring is created for the API user. | `tokens.keyring.test.ts` "review M1" (ring 0600 owned by 65534, API user `nobody`) — old: `key file …/product: is owned by uid 65534; it must belong to uid 0 or root` |
| **L5** | `vrx-authctl rotate-jwt-key` writes an `auth.jwt-key-rotated` audit row (`root (break-glass)`; file, key count, new signing **kid**, owner uid — never a key) and a `JWT_KEY_RING_ROTATED` system_event; warns if the row cannot be written. | lockout e2e "review L5" — old: `expected [] to deeply equal [ { user_id: null, …(4) } ]` |
| **C3** | The demotion/deletion hunk (PENDING-session-revocation option 1) is ONE commit: `f3011e01` takes it out (datastore/pg-repo.ts + repo.ts back to the base, its two e2e cases leave td10b-session), `cb74843d` puts it back **byte-identical to the reviewed code** (`git diff --quiet 89e5ee5f -- apps/api/src/datastore/` → identical) with its own suite `td10b-session-revocation.e2e.test.ts`. Drop it: `git revert cb74843d`. Nothing else changed in it (the local `rank` nit, L10, stays for that reason). | the new suite at `f3011e01` (hunk out): `demotion: {"demoMe":200,"demoRefresh":200,"promoMe":200}` → `expected {…} to deeply equal { demoMe: 401, demoRefresh: 401, …(1) }`; `deletion: … → 200` → `expected 200 to be 401` · with `cb74843d`: 2 passed |
| **L1** | `registerFailure` takes a transaction-scoped advisory lock (single-bigint key space; TD-10a's commit lock uses the two-int space) before its UPDATE, so the "last admin" test sees the other admin's committed lock. | lockout e2e "review L1" (10 trials, two admins at MAX−1 failing in parallel) — old: `admins locked account-wide per trial: 2,2,1,2,2,1,1,2,2,1` → `expected 2 to be 1`; fixed: `1,1,1,1,1,1,1,1,1,1` |
| **L4** | `consumeRefresh` takes the token AND sets `rtused` in one Lua script (`CONSUME_SCRIPT`). | session e2e "review L4" (the `rtused` write delayed 300 ms, logout 100 ms into the refresh) — old: `{"logout":204,"refresh":200,"oldToken":200,"newChain":200,"newToken":200}`; fixed: `{"logout":204,"refresh":200,"oldToken":401,"newChain":401,"newToken":401}` |
| **L7** | `key-file.ts`: open once with `O_NOFOLLOW`, `fstat` and read through that descriptor (`readKeyFile`); the JWT ring and the rotation use it. | `key-file.swap.test.ts` (path check sees a clean 0600 file, the file read is 0644) — old: `expected [Function] to throw an error` |
| test flake | `td10b-audit` "rate limit per client" failed once at a minute boundary (fixed-window counter split: `7 logins from P: 401 ×7`); the test now waits out the last 10 s of a minute. | — |

### Hand-offs (not fixed here; owner in bold)
| review item | what | owner |
|---|---|---|
| M2 | accepted residual: distributed guessing gets VRX_LOGIN_MAX_FAILURES per address (IPv4) or per /64 per lockout window — account-wide soft backstop (addresses without a recent success of that user get the progressive throttle after N failures/h across all addresses) and a second IPv6 key at /56 or /48 | **SEC-auth** (tech-debt row) |
| L3 | P10's nginx: **overwrite** `X-Forwarded-Proto $scheme`, `X-Forwarded-For $proxy_add_x_forwarded_for`, set X-Forwarded-Host (the API reads no host/origin today — keep it so); dedicated nginx→API address (127.0.0.2) + `VRX_TRUST_PROXY=127.0.0.2` | **P10** (questions 6) |
| L6 | the `vrx_docs` cookie holds a full access token (HttpOnly/SameSite=Strict/Path=/api/docs, accepted only by the docs hook) — mint a docs-only token (`typ: 'docs'`) that `verifyAccess` refuses | **SEC-auth** (with P10's cookie/nginx review) |
| L2 | optional: count an attempt before argon2 (in-flight key), the probe could not exploit the gap | **SEC-auth** |
| L8 | `checkKeyFile` for `VRX_SECRET_KEY_FILE` (symlink refusal, clear message) once TD-10a is on main | **TD-22** |
| L9 | aggregated audit rows: a closing row per minute with the exact count and top-N sources | **SEC-auth** |
| L10 | `writeFailures` exported as a metric → **F-dashboard-prom-alarms / P10**; config-path user changes stay fail-open for the audit row (the revision row of the promote is their record) → **D-line text**; "last admin" counts an enabled admin without a usable password → **SEC-auth**; per-sid revocations reloaded only at boot (second API process) → known D-111 limit; secrets routes' 503 in OpenAPI → **contract follow-up after TD-10a + TD-10b** (whoever merges second) | as listed |
| questions 2 | `userExclusive` swap (mechanical, pre-reviewed) | whichever of **TD-10a/TD-10b merges second** |

### Evidence
Unit — `pnpm --filter @ngfw/api test` (typecheck and lint clean):
```
 Test Files  16 passed (16)
      Tests  134 passed (134)
```
The same new tests with the five changed implementation files put back to `175bed1c` (tests kept; HEAD restored afterwards):
```
 × key file: check and read are one descriptor (review L7) … → expected [Function] to throw an error
 × JWT key ring … review M1: root rotates the PRODUCT layout … → key file …/product: is owned by uid 65534; it must belong to uid 0 or root
 × lockout … review L1 … → expected 2 to be 1   (L1 admins locked account-wide per trial: 2,2,1,2,2,1,1,2,2,1)
 × lockout … review L5 … → expected [] to deeply equal [ { user_id: null, …(4) } ]
 × sessions … review L4 … → {"logout":204,"refresh":200,"oldToken":200,"newChain":200,"newToken":200}
```
e2e on HEAD — td10b ×5, td4 ×2, auth (slot 5 as `w5b`, ports 3500/3550 checked free, shared lab lock, 05:12–05:17):
```
$ eval "$(tools/lab env 5)"; export VRX_TEST_PREFIX=w5b VRX_VALKEY_DB=5 VRX_HTTP_PORT=3550
$ tools/lab lock shared pnpm --filter @ngfw/api test:integration test/e2e/td10b-{lockout,session,session-max,audit,session-revocation}.e2e.test.ts test/e2e/td4-{auth-hardening,stepup-burst}.e2e.test.ts test/e2e/auth.e2e.test.ts
L1 admins locked account-wide per trial (2 parallel failures at MAX-1): 1,1,1,1,1,1,1,1,1,1
L5 rotate: exit 0; rotated …/jwt.keys: 1 key(s), new signing key 35b1d675b844435b first, owner uid 65534; the API reloads it within 5 s
 ✓ test/e2e/td10b-lockout.e2e.test.ts (7 tests)
 ✓ test/e2e/td10b-session-max.e2e.test.ts (1 test)
secrets: 17 passwords checked against {"audit":56,"events":6} rows and 1 API log lines → found in: []
 ✓ test/e2e/td4-auth-hardening.e2e.test.ts (10 tests)
2.3b 7 logins from P: 401,401,401,401,401,401,429; then one from Q: 401
commit-busy: 409 https://vrx.dev/problems/commit-busy after 1206 ms (lock held 3 s)
 ✓ test/e2e/td10b-audit.e2e.test.ts (7 tests)
L4 logout racing a refresh: {"logout":204,"refresh":200,"oldToken":401,"newChain":401,"newToken":401}
 ✓ test/e2e/td10b-session.e2e.test.ts (7 tests)
 ✓ test/e2e/td4-stepup-burst.e2e.test.ts (1 test)
demotion: {"demoMe":401,"demoRefresh":401,"promoMe":200}
deletion: the deleted user's token on GET /api/v1/config → 401
 ✓ test/e2e/td10b-session-revocation.e2e.test.ts (2 tests)
 ✓ test/e2e/auth.e2e.test.ts (10 tests)
 Test Files  8 passed (8)
      Tests  45 passed (45)
ok     nothing named vrx_w5b / vrx_w5b remains
```
CI — `TMPDIR=/tmp/g-td10b tools/ci.sh --base main` on `37611df9` (05:11–05:17):
```
ok — contract commit(s) on the branch: 62d53507 contract(api-client): TD-10b … · c05183a6 contract(api-client): TD-4 …
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: gitleaks — scanned ~384837 bytes (384.84 KB) in 1.4s no leaks found
Tasks:    30 successful, 30 total Cached:    12 cached, 30 total Time:    2m47.435s
  warnings: commit subject(s) not in Conventional Commits form: review(TD-10b): APPROVE WITH CHANGES   (the reviewer's commit)
  mode quick · wall time 5m58s · logs /root/ngfw-wt/logs/ci/TD-10b-20260925-051144-4155108
CI GATE PASSED
```
After the CI run only `docs/status/tasks/TD-10b*` changed. Cleanup: `vrx_w5b` dropped and its Valkey keys deleted by the
teardown, the worktree's `dist/` and `apps/agent/bin` removed, no process of mine left; tools/app was never run, ports
3000/8080/9101 and `/run/vrx/agent.sock` untouched.
