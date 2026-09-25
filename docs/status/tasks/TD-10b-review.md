# TD-10b review: API auth, session and audit hardening (REVIEW-2026-09-24 §2.3, D-125)

reviewer: an independent agent that did not write the code · branch `task/TD-10b` @ `1480e6b5` (code `bd48f747`, contract `62d53507`),
base `task/TD-4@c05183a6` (TD-4 is now on main as `10059d57`) · slot 5 as prefix `w5b` · 2026-09-25 04:20–04:50.
Read: the envelope, TD-10b.md, TD-10b-questions.md, TD-10b-contract.md, 00-CONTEXT, 01-architecture, 04-api-datamodel, the LOG
(D-091, D-097, D-100, D-102, D-111, D-124, D-125, D-134, D-135), decision-policy, PENDING-session-revocation,
PENDING-tools-app-transport, the whole diff `c05183a6..1480e6b5` (42 files, +3674/−299) and TD-10a's `userExclusive`/`CommitLock`.

## Verdict: APPROVE WITH CHANGES

The security core is correct and well proven. The lockout script is atomic. The trusted-proxy walk cannot be spoofed by a peer that is
not trusted. Logout ends the session by sid and survives a restart. The session max caps every credential. Privileged routes write
their audit row first and change nothing when that write fails. Every item fails on the base and passes here, and I re-ran it.

Three things stand between the branch and main:

- **C1 (H1): the lab.** Once merged, `tools/app` refuses remote logins. Pick one of two fixes:
  - one line in `tools/app` keeps today's lab behaviour (recommended), or
  - hold the merge until the product owner answers PENDING-tools-app-transport.
- **C2 (M1): key rotation.** `vrx-authctl rotate-jwt-key` refuses the production key file, which the `vrx` user owns (reproduced).
  The fix is small and belongs to TD-10b's own files.
- **C3: the demotion/deletion hunk.** It implements PENDING-session-revocation option 1, and that decision field is still empty.
  Decision policy §4 treats the auth/session model as always PENDING. Ask the product owner now. If the answer is "no", removing the
  hunk is mechanical.

After C1 and C2, a focused re-verify is enough: two tests that fail first, plus a `tools/app` diff. A full re-review is not needed.

## Evidence (reviewer runs)

```
$ pnpm --filter "@ngfw/api^..." run build && pnpm --filter @ngfw/api test && … typecheck && … lint
 Test Files  15 passed (15)      Tests  131 passed (131)      typecheck ok · lint ok · exit=0
$ eval "$(tools/lab env 5)"; VRX_TEST_PREFIX=w5b VRX_VALKEY_DB=5 VRX_HTTP_PORT=3550 (3500/3550 checked free first)
$ tools/lab lock shared pnpm --filter @ngfw/api test:integration td10b-{lockout,session,session-max,audit} td4-auth-hardening auth
2.3a victim after 3 failures from A: from A 401, from B 200
2.3a last admin from C: inside the throttle 401, after 1.2 s 200
2.3a parallel bursts from F on both admins → ["lock","throttle"]
2.3b via the proxy from 203.0.113.10: plain HTTP 403 …/tls-required, TLS 200
2.3b 7 logins from P: 401,401,401,401,401,401,429; then one from Q: 401
2.3c after logout of session 1: {"s1":401,"s2":200} · forged logout → the victim's refresh 200, access token 200
demotion: {"demoMe":401,"demoRefresh":401,"promoMe":200} · deletion: … → 401
2.3d max 5s: login {"expiresIn":5,"tokenLife":5,"refreshCookie":5,"docsCookie":5}; refresh at +1.2 s 200; at +5.7 s 401
2.3e 13 unauthenticated commits → 3 audit rows · fail-closed: key creation 503 …/audit-unavailable; password set 503
2.3g docs with the cookie: page 200, init.js 200; without: 401
commit-busy: 409 …/commit-busy after 1151 ms (lock held 3 s)
 Test Files  6 passed (6)      Tests  41 passed (41)
e2e teardown: deleted 161 Valkey keys vrx:w5b:e2e:* · drop database vrx_w5b · drop role vrx_w5b · ok nothing named vrx_w5b remains
```
Reviewer probe: last-admin throttle against a parallel burst, `MAX_FAILURES=3`, default rate limit. The probe was a temporary e2e
file, deleted after the run and never committed. After 3 sequential failures and the 1 s throttle, I sent 15 parallel logins from one
address, with the right password at #10:
```
PROBE burst of 15 (right password at #10) after the 1 s throttle expired: statuses [401 ×15];
      audit reasons {"bad-password-throttled":1,"throttled":14}; right guess → 401
```
Reviewer reproduction of M1. The file was owned by uid 65534 with mode 0600, `rotateKeyFile` ran as root, and the key was redacted:
```
ROTATE RESULT: key file …/jwt.keys: is owned by uid 65534; it must belong to uid 0 or root
```
`shellcheck deploy/sbin/vrx-authctl tools/app` → clean. After my runs the worktree is clean: I removed the `dist/` directories I built,
nothing named `w5b` remains, and I did not touch ports 3000/8080/9101 or tools/app.

**The pre-fix failures the worker pasted are credible.** The behavioural cases fail on the base for the right reason: the old account
lock (`expected 401 to be 200`), the old trust in loopback (`expected 200 to be 403`), `expected 201 to be 503`,
`expected {s1:200} … {s1:401}`, `expected 900 to be ≤ 5`, `expected 200 to be 409`. Four unit files fail only because a module does not
exist yet: break-glass-cli, keyring, commit-busy, and transport, which fails at setup. For new features that is acceptable, because an
e2e case adds a behavioural control for commit-busy (409) and for transport (403). The key ring has no behavioural control on the base,
which is fine for a new feature.

## Findings

### H1: after the merge, remote lab logins through `tools/app` get 403 `tls-required` (merge condition, not a code defect)
`apps/web/vite.config.ts:20` sets `xfwd: true`, `apps/api/src/config.ts` defaults `VRX_TRUST_PROXY=loopback`, and `tools/app:115-117`
starts the API without the setting. I read the proxy code in vite 7.3.6 (`dist/node/chunks/config.js:21433-21446`). It **appends**
`for`, `port` and `proto`, so the API sees `X-Forwarded-Proto: …,http` for the vite hop, and
`secureTransport = requestProtocol(req)==='https' || isLoopback(client)` is false. Every login from another machine then gets 403, and so
does every password set and API-key step-up. That is correct under D-100, and the worker predicted and reported it (questions 1). Refresh
has no transport check, so existing lab sessions continue until `VRX_SESSION_MAX_SEC`. Then the product owner's lab UI stops working
from the desktop.

**Fix (recommended, C1).** In `tools/app`, add `VRX_TRUST_PROXY=none` to the `start_svc api env …` line, with a comment that points to
PENDING-tools-app-transport, and correct the banner. The note it prints now ("logins from other machines are refused") would become wrong.

This reproduces **main's current behaviour exactly**, and I checked why. On main today the API has no `trustProxy`, the peer is vite on
127.0.0.1, and `isLoopback` accepts it. With `none`, `trustProxy` is false, `req.ips` is undefined, the protocol comes from the socket,
and the client is 127.0.0.1, so the request is accepted. No new security decision is taken: cleartext lab passwords remain the known,
recorded exception, and the listener hunk (option 1 or 2) removes the line.

The lab's side effect is the same as on main: every lab browser shares one lockout and rate-limit key, 127.0.0.1. The last-admin
throttle still applies. The alternative to C1 is to hold the merge until the product owner decides. With C1, the only PO-gated part left
is C3.

### M1: `vrx-authctl rotate-jwt-key` refuses a key file owned by the API user, which is the production layout
`apps/api/src/auth/break-glass.ts:166` calls `checkKeyFile(path)`, and `apps/api/src/auth/key-file.ts:17` defaults the allowed owner to
`process.geteuid()`, which is 0 for the root tool. The API runs as `vrx` (`docs/01-architecture.md:83`). A 0600 file therefore has to
belong to `vrx`, and the API's own check at boot requires exactly that. A vrx-owned file then fails the rotation (reproduced above).
Chowning the file to root does not help, because the API could no longer read it. The unit test (`tokens.keyring.test.ts:42-72`) passes
only because it runs as root on root-owned files. So the P06 tech-debt item "JWT key rotation", which TD-10b marks as done in
questions 10, does not work on a product box.

**Fix.** When the tool runs as root, accept the file's existing owner: lstat first, then `checkKeyFile(path, st.uid)`. Keep the checks
for regular file, not a symlink, and no group/other bits. Add a unit test that fails first: create the file, `chown 65534`, rotate, and
check that the owner is still 65534 and the mode is 0600.

### M2: nothing bounds guessing against one account across many addresses (a residual; record it and give it an owner)
`apps/api/src/auth/lockout.ts:45-48`, `:110-132` and `auth.service.ts:127` key the lock and the rate limit by `clientKey`: one IPv4
address, or one IPv6 /64. The worker documents this residual (decision 1), but it is larger than the text suggests:
- **IPv4.** Each IPv4 address gets 10 guesses per 15-minute window, plus 20 attempts a minute from the rate limit.
- **Last admin.** It gets 10 free guesses per address, then about 1 a minute.
- **IPv6.** Someone who holds a /48 has 65,536 /64 keys.

The only global bound left is argon2 throughput, a few thousand hashes a minute on the 4-thread pool, together with the 12-character
minimum. Before TD-10b the account lock bounded guessing to about 10 per 15 minutes globally, at the price of the DoS that review 2.3a
asked to remove, so the design choice is right.

**Recommendation (not a merge blocker).** Record the residual in the LOG line (below) and open a tech-debt row owned by SEC-auth or F-aaa:
- an account-wide soft backstop: when an account sees more than N failures an hour across all addresses, addresses without a recent
  successful login of that user get the progressive throttle (owners keep access, so there is no DoS);
- a second key at /56 or /48 for IPv6.

### L1: two admins can both be locked account-wide by concurrent in-session failures (race in the "last admin" SQL)
`apps/api/src/auth/auth.service.ts:217-230`. `last` is evaluated per row under READ COMMITTED. Suppose two stolen admin sessions each
reach `MAX` wrong step-ups at the same moment. Each UPDATE's `NOT EXISTS` sees the other admin still unlocked, so both get locked
account-wide for `VRX_LOGIN_LOCKOUT_SEC`. The precondition is two stolen admin sessions, which could already disable or demote every
admin through a config commit, and the break-glass clears the locks, so the risk is low.

**Fix (later).** Serialise admin lock decisions: `pg_advisory_xact_lock(<constant>)`, or `SELECT … FOR UPDATE` on the admin rows, in the
same transaction. Put it on the tech-debt list.

### L2: the gap between the argon2 check and the throttle check (theoretical; the probe could not exploit it)
`auth.service.ts:133` runs `verifyPassword` before `lockout.state()` (`:140`), and the failure is counted afterwards in `fail()`.
Attempts whose argon2 finishes in the same instant could all read `open`. The probe shows the bound holds in practice: 1 of 15
evaluated, and the right guess refused without an oracle. The rate limit bounds it as well.

**Optional hardening.** Count an attempt before argon2 (an in-flight key in `state()`, reconciled by `admit`).

### L3: the proxy-header assumptions P10 must honour
`apps/api/src/common/principal.ts:61-77`.
- **X-Forwarded-Proto.** `requestProtocol` takes the entry at `len − hops`. "A trusted hop without X-Forwarded-Proto counts as http"
  holds only when the client sent no entry of its own. A trusted hop that **passes the client's X-Forwarded-Proto through** without
  setting its own lets a forged `https` count. vite appends, so it is safe, and the two-hop unit test covers setting hops.
  - P10's nginx must `proxy_set_header X-Forwarded-Proto $scheme;` (overwrite) and `X-Forwarded-For $proxy_add_x_forwarded_for;`.
  - Add the word **overwrite** to questions 6.
- **X-Forwarded-Host.** vite passes the client's `X-Forwarded-Host` unchanged (`config.js:21446`), and with `trustProxy` Fastify's
  `req.host`/`hostname` trust it. Nothing in `apps/api/src` reads host, origin or `req.protocol` today (I grepped). Keep it that way, or
  have nginx set the header.
- **Loopback trust.** `trustProxy=loopback` makes **every local process** a trusted proxy. A local unprivileged process can forge
  X-Forwarded-For and X-Forwarded-Proto to rotate rate-limit and lockout keys. The precondition is a local attacker, so the risk is low.
  - **Suggestion for P10.** Have nginx reach the API from a dedicated address, for example 127.0.0.2, and set `VRX_TRUST_PROXY` to that
    address only.

### L4: a logout that races a refresh of the same cookie can end nothing
`apps/api/src/auth/tokens.service.ts:308-317` versus `:330-341`. `consumeRefresh` runs `GETDEL rt:<h>` and later `SET rtused:<h>`. A
logout whose `GETDEL` and `GET rtused` fall between the two sees neither key and returns "unknown token". The refresh then continues the
chain. The window is narrow, and it needs the same browser to refresh and log out together.

**Fix (later).** One Lua script for GETDEL plus SET rtused.

### L5: `rotate-jwt-key` is not audited, although the wrapper and USAGE say every action is
`deploy/sbin/vrx-authctl:9` and `break-glass-cli.ts:98-114`: the rotation returns before any database or event is touched. Unlock writes
an audit row and a system_event.

**Fix (cheap; do it with M1).** Have the API record a `JWT_KEY_RING_ROTATED` system_event when `refreshRing` loads a changed signing key
(`tokens.service.ts:154-171`). That also covers manual edits of the file. Alternatively, correct the wording.

### L6: the docs cookie carries a full-power access token
`apps/api/src/auth/cookies.ts:36-42`. The flags are right:
- HttpOnly, so the Swagger page's scripts cannot read it;
- SameSite=Strict, so there is no cross-site use, and it is accepted for GET/HEAD only;
- Path=/api/docs, so browsers do not send it to `/api/docs-json` or `/api/v1`;
- Secure by default (`VRX_COOKIE_SECURE=1`);
- its lifetime is at most the token's and the session's.

The docs hook accepts it only under `/api/docs`, and the route-guard test proves that API routes ignore it. However, the value is a
valid `Bearer` for every API route if someone lifts it from the browser's cookie store.

**Defence in depth (tech debt).** Mint a docs-only token (`typ: 'docs'`) that `verifyAccess` refuses outside the docs hook.

### L7: `checkKeyFile` checks, then reads by path
`key-file.ts:20` runs lstat, and `tokens.service.ts:148` then runs `readFileSync(path)`. The directory's mode is not checked. A
directory that another user can write allows a symlink swap between the two calls.

**Nit.** Open with `O_NOFOLLOW`, `fstat` the descriptor and read from it.

### L8: the secret-store key file has no owner check (the hand-off in questions 5; my rating: L)
TD-10a's `secrets.service.ts` already runs `chmodSync(file, 0o600)` before reading. That fails with EPERM, and the API stops, unless the
API user owns the file (or is root). So in practice the ownership already holds. What is missing is refusing a symlink (chmod and read
follow it) and a clear message. Once TD-10a is on main this is a one-line `checkKeyFile(file)`. It belongs in a follow-up row (TD-22),
not in a merge squash (D-134).

### L9: audit aggregation hides magnitude, and past 200 keys a minute it hides sources
`audit/audit.service.ts:92-113`.
- **Counts.** Rows are written at 1, 10, 100 …, so 99 attempts read as "≥ 10". The exact count per minute is never written.
- **Sources.** An IPv6 sprayer can fill the 200 keys early in a minute. Later 401s from its real address then land in the
  source-less overflow row.
- **Impact.** Every such request failed authentication, and failed logins are still audited one row each (bounded by the rate
  limit). So this is visibility only.

**Suggestion.** A closing row per minute with the exact count and the top-N sources.

### L10: smaller notes
- **`writeFailures` is not exported.** The API has no metrics endpoint, so the counter is visible only as `failuresTotal` in
  AUDIT_WRITE_FAILED events (`audit.service.ts:57`). Wire it up when P10 or F-dashboard adds metrics.
- **Fail-closed scope.** Config-path user changes (commit, confirm, rollback or import changing hashes, roles or deletions) stay
  fail-open for the audit row. The revision row written in the same promote transaction, with author and diff, is their durable
  record. State that in the D-line (below).
- **Last-admin predicate.** Any enabled admin that is not locked counts as "another admin who can log in", even one with no usable
  password (`lockout.ts:60-72`), so the real last admin can be locked at an address. The effect is per address only.
- **Multi-process.** Per-sid revocations are reloaded from Valkey only at boot. A second API process would accept a logged-out
  session's access token for up to 15 minutes. This is the same known limit as D-111 (one API process).
- **Nit.** `datastore/pg-repo.ts:250` defines its own `rank`, while `ROLE_RANK` already exists in `common/principal.ts`.
- **Secrets routes lack the new 503 in OpenAPI.** `POST /api/v1/secrets` and `DELETE /api/v1/secrets/:kind/:name` can now answer 503
  `audit-unavailable`. Their controller belongs to TD-10a, so the 503 is not in their OpenAPI yet. This is a contract follow-up after
  both branches merge.

## Answers to the review dimensions

1. **2.3a lockout.** The `FAIL_SCRIPT` does INCR, the other-admin check and SET in one script, so it is atomic. Two parallel bursts lock
   at most one admin: the e2e gives `["lock","throttle"]`, and the logic holds for N admins, where N−1 are locked and the last is
   throttled.
   - Rotating addresses **cannot lock anyone out** anywhere except at the attacker's own addresses. What rotation buys is guessing
     volume (M2). The shared-address case (NAT, or a proxy without X-Forwarded-For) degrades to the old account-wide behaviour, and the
     last admin is still only throttled.
   - The throttle cannot be bypassed by generation (the key includes it), by address form (IPv4-mapped addresses are folded, IPv6 is
     keyed per /64) or in parallel (L2, probe).
   - On the account-wide path, the last admin is never locked, except in the concurrent case of L1.
   - **Break-glass.** It is root-only twice: `$EUID` in the wrapper and `getuid()` in `main()`.
     - It is safe against injection. The username goes into a parametrised drizzle `eq`. The SCAN patterns use the numeric id from
       the database, never user input. Arguments reach node as an argv array after the script path, so they cannot become node options.
       `--env-file` takes only `VRX_*` keys.
     - Unlock is audited with an audit row and a system_event. It never prints a setting.
     - Rotate is not audited (L5), and it is broken on the product layout (M1).
2. **2.3b trusted proxy.** Fastify's `trustProxy` uses VRX_TRUST_PROXY. `/0`, `true` and `*` are refused, and `none` becomes `false`.
   Fastify builds `req.ips` from X-Forwarded-For with proxy-addr, walking back through the trusted hops, and stops at the first
   untrusted one. `requestProtocol` takes the X-Forwarded-Proto entry of the outermost trusted hop, so address and protocol come from
   the same hop.
   - A peer that is not trusted cannot spoof either header. Its headers are ignored, and forged entries sit to the left of the address
     vite appends. Unit tests (a real Fastify instance) and the e2e confirm this.
   - The rate limit (`auth.service.ts:127`), the lockout (`:140-148`), the audit (`audit.interceptor.ts:84`, `auth.guard.ts:36`, the
     login/refresh/logout rows) and the TLS check (`transport.ts:25`) all use `sourceIp(req)`, or `clientKey` of it.
   - A trusted hop with no X-Forwarded-Proto counts as http, provided the client sent none either (L3).
3. **2.3c sessions.** Logout ends the refresh chain and revokes the sid in memory and in `atrevsid` (the restart e2e passes). It works
   with the cookie and with a Bearer token. A forged `<family>.<junk>` or a forged Bearer ends nothing, because only a token the store
   knows, or a verified JWT, acts. Reuse detection now revokes the sid too.
   - **Demotion and deletion hunk** (`pg-repo.ts:215-320`, `repo.ts:90-94`). It only ever **ends** sessions. It bumps the generation
     exactly as D-100 (3) does for a disable, grants nothing, changes no other data, and reversing it means removing about 25 lines and
     2 e2e cases. On substance it is safe.
   - Decision policy §4, however, makes the auth/session model always PENDING, and the PENDING file parks this hunk explicitly. So it
     **must wait for the product owner's answer** (C3). **Recommendation:** put PENDING-session-revocation and
     PENDING-tools-app-transport to the product owner in the next Persian report; both already have Persian summaries.
     - If option 1 comes back, merge as is.
     - If the answer is no, the merger removes the hunk mechanically (D-134 allows a mechanical removal).
     - If there is no answer and throughput matters, split the hunk onto `provisional/session-revocation` and merge the rest.
4. **2.3d max age.** It is absolute from `t0` in `rtfam`, the Lua script refuses and deletes the family after it, and it caps the
   refresh cookie, the docs cookie and the access token's `exp`. The e2e measured 5/5/5/5. Legacy chains start their clock at their
   first refresh.
5. **2.3e audit.**
   - **Logouts and refresh failures** carry a reason (`auth.service.ts` `refuse`, `logout`).
   - **401 aggregation** is bounded (L9 covers visibility).
   - **Write failures** produce AUDIT_WRITE_FAILED at most once a minute, with the running count and the SQLSTATE; the log shows the
     driver's cause, never the row.
   - **Fail-closed routes.** `begin` runs in the interceptor before `next.handle()`, and `catchError` sits before the handler's
     `mergeMap`, so only the write-ahead failure maps to 503. Nothing on those routes changes state before the write:
     - guards run first and only read; the one exception is the API key's `lastUsed`, which is metadata;
     - the transport check, `pwset:`, `registerFailure`, the commit lock, the transaction and the key or secret writes all run in the
       handler, after `begin`;
     - none of the six routes is `@NoAudit`, and a test pins that all six exist.

     For config-path user changes see L10.
6. **2.3g /api/docs.** The flags are correct and the design is sound: GET/HEAD only, the hook applies to `/api/docs*` only, and API
   routes read only the `Authorization` header. The Swagger page's scripts cannot read the HttpOnly cookie. There is no CSRF surface:
   SameSite=Strict, GET only, nothing changes state. Defence in depth: L6.
7. **P06 tech debt.** The key ring is sound. It uses `kid` = a truncated hash, which reveals nothing that an HS256 token does not
   already allow testing offline. Kid-less legacy tokens are tried against every key, an unknown kid gets nothing, a bad file keeps the
   old ring, and the file is refused at boot with a message that names only the path. Rotation is broken on the product layout (M1)
   and not audited (L5). The secret-store owner check is a hand-off, rated L (L8).
8. **Addendum (commit-busy).** `withinCommitLock` is correct:
   - if the deadline passes while the section is still waiting, the section is abandoned and does nothing when its turn comes;
   - if the section started before the deadline, the call waits for its result;
   - the e2e measured 409 after 1151 ms.

   The planned swap is right. TD-10a's `CommitService.userExclusive(fn)` (task/TD-10a `commit.service.ts:489`) wraps
   `lock.tryRun(fn, budget.lockWaitMs)`, which abandons the section by itself, and returns the same `commit-busy` body with
   `retryAfterSec: 2`.
   - **The swap:** replace `withinCommitLock((f) => this.commits.exclusive(f), async () => {…})` in `users.service.ts` with
     `this.commits.userExclusive(async () => {…})`. Delete `withinCommitLock`/`commitBusy` and `users/commit-busy.test.ts`, keep
     `CommitBusyDoc`.
   - **Acceptance:** the `td10b-audit` "manager addendum" e2e stays green.
   - **Without the swap** the behaviour is still correct: `withinCommitLock` over TD-10a's `CommitLock.run` also bounds the wait for
     the cross-process lock. The swap is therefore a clean-up the merger may do mechanically, not a correctness fix.
   - **Merge check:** I simulated the rebase of TD-10b onto main and merged TD-10a into the result. The only conflict is
     `docs/tech-debt.md`, and the merged `configResets` keeps TD-4's per-reason rows, which settles the worry in questions 3.
9. **Contract.** `62d53507` is additive only: +409/+503 on `Users_setPassword` and `Auth_password`, +503 on
   `Auth_createApiKey`/`Auth_deleteApiKey`, and the `Auth_logout` 204 description. No operation, field or schema changes shape. The
   worker's CI shows the generated files clean, and CLI (`operations_gen.go`, reference.md) and SDK outputs unchanged. The commit
   subject starts with `contract(api-client):` as the guard needs. The secrets routes' 503 is a follow-up (L10).
10. **The merge blocker.** Confirmed by reading vite's proxy code and by the e2e over the proxy (`plain HTTP 403`). The branch can
    merge safely **with C1** (`VRX_TRUST_PROXY=none` in `tools/app`, status quo, H1). Leaving `VRX_TRUST_PROXY` unset in `tools/app` is
    **not** enough, because unset means the default, `loopback`. Without C1 it must wait for the PENDING-tools-app-transport answer.
11. **Tests.** Unit 131/131, typecheck and lint green. The e2e run covered td10b ×4, td4 and auth: 41/41 on slot 5 as `w5b`, cleaned up.
    The pre-fix failures are credible (see Evidence). I did not re-run the full api e2e suite; the worker's run shows 93 passed and
    3 skipped.
12. **Merge fit.** `git merge-tree --write-tree main task/TD-10b` gives 9 conflicts, the expected ones after TD-4's squash:
    - content: `auth.controller.ts`, `auth.service.ts`, `datastore/pg-repo.ts`, `datastore/repo.ts`, `users/users.service.ts`,
      `packages/api-client/src/generated/schema.d.ts`;
    - add/add: `auth/transport.ts`, `auth/transport.test.ts`, `test/e2e/td4-auth-hardening.e2e.test.ts`.

    A simulated rebase with base `c05183a6` (`git merge-tree --write-tree --merge-base c05183a6 main task/TD-10b`) is **clean**, tree
    `66de2deb`. Merge procedure (D-112), in the worktree, after the fix round:
    ```
    git -C /root/ngfw-wt/TD-10b update-ref refs/archive/TD-10b HEAD
    git -C /root/ngfw-wt/TD-10b reset --soft c05183a6
    git -C /root/ngfw-wt/TD-10b commit -m "contract(api-client): TD-10b auth, session and audit hardening (REVIEW-2026-09-24 §2.3, D-125)"
    git -C /root/ngfw-wt/TD-10b rebase --onto main c05183a6
    pnpm --filter @ngfw/api-client gen && git -C /root/ngfw-wt/TD-10b diff --exit-code   # schema.d.ts was merged textually
    TMPDIR=/tmp/g-w5 tools/ci.sh --base main
    ```
    Then `git merge --no-ff task/TD-10b` under the main lock (D-130). If TD-10a merges first, apply the item 8 swap during that rebase.

## Required before merge
- **C1 (H1):** in `tools/app`, add `VRX_TRUST_PROXY=none` on the API line and correct the banner (status quo, until
  PENDING-tools-app-transport). Otherwise, hold the merge for the answer.
- **C2 (M1):** make the rotation keep the file's existing owner when run as root, with a unit test on a chowned file that fails first.
  Doing L5 in the same hunk is recommended.
- **C3:** get the answer to PENDING-session-revocation, or split the hunk out (item 3).

## Proposed LOG entries (numbers for the manager to assign; next free: D-136)

```
| 2026-09-25 | D-136 | TD-10b decisions (review <this commit>): D-TD10b-1 the LOGIN lockout is keyed by (user, credential generation, client address) in Valkey — IPv4 per address, IPv6 per /64, IPv4-mapped as IPv4; password checks made inside a session (API-key step-up, own-password change) keep TD-4's account-wide lock; accepted residual: distributed guessing gets VRX_LOGIN_MAX_FAILURES per address per lockout window (tech-debt: account-wide soft backstop + IPv6 /48 key, owner SEC-auth); D-TD10b-2 the last enabled admin who could still log in from that address is throttled 1-2-4…≤60 s, never locked, decided in one Valkey script with the other admins' keys; the account-wide lock never hits the last admin either; break-glass `vrx-authctl` (root only: unlock, locks, rotate-jwt-key) packaged by P10; D-TD10b-3 logout ends THAT login session (refresh chain + access tokens by sid, memory + Valkey `atrevsid`, reloaded at boot), cookie or Bearer; only a token the store knows acts; refresh-token reuse also revokes the sid; D-TD10b-4 VRX_SESSION_MAX_SEC = 12 h absolute from login (min 5 s, no off switch), capping the refresh cookie, the docs cookie and the access token; D-TD10b-5 audit write-ahead, fail closed (503 audit-unavailable, nothing changed) on password set, API-key create/delete, secret create/delete; all other mutations fail open with a counter and at most one AUDIT_WRITE_FAILED system_event a minute; config-path user changes are recorded by the revision row of the promote transaction; D-TD10b-6 /api/docs in a browser via the `vrx_docs` cookie (the session's access token; HttpOnly, SameSite=Strict, Path=/api/docs, GET/HEAD only); D-TD10b-7 401 mutations and unknown refresh/logout tokens are audited aggregated per (client, route, reason, minute) at the 1st/10th/100th… occurrence, ≤ 200 keys a minute + one overflow row per action; D-TD10b-8 successful refreshes are not audited (the login row starts the session); D-TD10b-9 a trusted hop that sends no X-Forwarded-Proto counts as plain HTTP; P10's nginx must SET (overwrite) `X-Forwarded-Proto $scheme` and `X-Forwarded-For $proxy_add_x_forwarded_for` | 1 (a) every check per address (b) login per address, session-held checks account-wide (c) per account, last admin throttled; 3 (a) refresh family only (b) per sid (c) generation bump; 4 8 h / 12 h / 24 h / off; 5 (a) fail open (b) breaker (c) write-ahead on privileged routes; 6 (a) cookie (b) loopback only; 7 row per 401 / aggregate / none | 1(c) keeps the anonymous DoS review 2.3a found; 1(a) rewrites TD-2/TD-4 semantics with no gain against a session holder; 3(c) logs every device out; 5(c) is the only option under which no credential change happens without its row; 6(b) leaves the docs unusable from a desktop; 7 bounded rows without losing the trace | low | TD-10b, P10, SEC-auth, F-aaa, TD-10a |
| 2026-09-25 | D-137 | tools/app keeps main's current lab behaviour until PENDING-tools-app-transport is answered: its API runs with VRX_TRUST_PROXY=none (vite's loopback peer is the client, as before TD-10b; cleartext lab logins stay the recorded exception); the listener hunk of the answer removes the line | (a) hold TD-10b's merge for the answer (b) status-quo line in tools/app (c) merge as is | (a) parks the auth hardening on a lab question; (c) breaks remote lab logins (403 tls-required) at the next tools/app restart | trivial | TD-10b, tools/app |
| 2026-09-25 | D-138 | TD-10b hand-offs → tech-debt / rows: key-file owner check for VRX_SECRET_KEY_FILE after TD-10a merges (TD-22); `TokensService.hit` EXEC error read as 0 (TD-22); `config.user-demoted`/`config.user-deleted` audit rows in configResets and memory-repo parity (owner of commit/**, after DEC session-revocation); 503 audit-unavailable in the secrets routes' OpenAPI (contract follow-up after TD-10a+TD-10b); last-admin SQL serialisation (review L1), docs-only token (L6), atomic consume (L4), per-minute closing audit row (L9) → SEC-auth; root-side password reset (questions 7) → F-aaa, raised as a PENDING there (auth model); userExclusive swap (questions 2) is mechanical and pre-reviewed, done by whichever of TD-10a/TD-10b merges second | per item | none of them is a merge blocker for TD-10b; each has an owner | trivial | TD-22, SEC-auth, F-aaa, TD-10a |
```
If the product owner picks option 1 of PENDING-session-revocation: `DEC-<n>-session-revocation.md`, with `decision: option 1: demotion and
deletion end the user's sessions like a disable (built in TD-10b, pg-repo syncUsers)`, logged as D-139.
