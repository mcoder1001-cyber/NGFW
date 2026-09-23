# P06 — independent review (API core)

Reviewer: independent review agent (did not write this code). Branch `task/P06` @ `20e2a78`, base `main@76b017c`.
Ran on the dev host directly, slot 1 (`w1`, port 3100, DB `vrx_w1`, Valkey db 1). All probes cleaned up (see end).

## What was run

| run | result |
|---|---|
| `tools/ci.sh --base main` in `/root/ngfw-wt/P06` (HEAD 20e2a78) | **CI GATE PASSED** (contract guard ok — only `packages/api-client/src/generated` with `contract(api-client)` commit 69302a6 + P06-contract.md; generated-output gate clean; forbidden patterns + gitleaks clean; turbo 30/30; agent make ok). Matches the pasted run. |
| Throw-away review worktree `task/P06` + `git merge main` (P05 merged) | one conflict: `pnpm-lock.yaml`, 4 hunks, pure union (`@fastify/websocket`/`util-deprecate` vs `@floating-ui/utils`/`use-sync-external-store`); resolved as union, `pnpm install --frozen-lockfile` ok. Nothing else conflicts. |
| Real P05 agent built from the merged tree (`make build`), `VRX_INTEGRATION=1 tools/lab lock shared pnpm --filter @ngfw/api test:integration` (slot 1, `VRX_OWNER=w1`, socket `/run/vrx-test/w1/agent.sock`, state dir under `/run/vrx-test/w1`) | **25 passed, 1 failed** (see H2). The agent test is no longer skipped. |
| Ad-hoc probes against the built `dist/main.js` on :3100 + the real agent | lockout race, stale-lock takeover, secret overwrite, `/api/docs`, confirm-timer across an API restart, agent-down commit (below) |

### Real-agent end-to-end (the question asked)
All against VPP on this host, objects `loop101` / `10.1.101.0/24`, agent log excerpts:
```
created  interface-ip/loop101/10.1.101.1/24                 ← commit 1
deleted  …/10.1.101.1/24 · created …/10.1.101.2/24           ← commit 2 (Retrieve via /state/interfaces = [10.1.101.2/24], vppctl show int addr contains it)
deleted  …/10.1.101.2/24 · created …/10.1.101.1/24           ← POST /config/rollback/{rev1}  (vppctl: .1 present, .2 absent)
confirm timer armed … in 4.99s → "confirm timeout: reverting to the last confirmed state" → deleted .3, created .1   ← ?confirm=5, no confirm
deleted interface-ip/loop101/10.1.101.1/24 · deleted interface.loopback/loop101   ← afterAll cleanup
```
- commit → VPP (loopback IP) → Retrieve + `vppctl show int addr`: **works**.
- rollback → reverted on VPP: **works**.
- `confirm=5` without confirm → agent reverts, API clears `config_pending`: **works** (test green, 5.2 s).
- overlap → 400 with `/interfaces/loop102/ipv4/0`; readonly PATCH → 403: **works**.
- Confirmed commit **across an API restart** (probe): `commit?confirm=8` → VPP had `10.1.101.3/24` → API killed by PID → after the
  deadline VPP showed `10.1.101.1/24` → API restarted → `resumePending()` + Health path dropped the pending commit, system event
  `CONFIRM_REVERTED via=health`, running = `.1`. **Works.**
- Agent stopped → commit = 503 `Agent unavailable`, running untouched. **Works.**
- **Fails:** the last assertion of test 1, `GET /state/drift` must be `[]` — see H2.

## Findings (by severity)

### H1 — Lockout bypass: failed-login counter is a read-modify-write race — `apps/api/src/auth/auth.service.ts:78-95`
`login()` reads `u.failedLogins`, runs argon2 (~50 ms), then writes `failedLogins = u.failedLogins + 1`. Parallel attempts all
read the same value and all write the same `+1`. **Probe:** 60 parallel wrong passwords for `victim` (rate limit raised to isolate
the race) → 60 `auth.login failure` audit rows, `failed_logins` never reached 10, the correct password then logged in with **200**
(no lockout). With the default per-IP limit (20/min) one IP gets ~20 guesses per increment; with several source IPs the lockout
(P06 §6 "lockout after 10 failures") is effectively gone.
**Fix:** one atomic statement — `UPDATE app_user SET failed_logins = failed_logins + 1, locked_until = CASE WHEN failed_logins + 1
>= $max THEN now() + $lock ELSE locked_until END WHERE id = $id RETURNING …` (or a Valkey `INCR` per user), and check the lock
from that result; add an e2e that fires ≥ 20 parallel failures and expects the account locked.

### H2 — Real-agent e2e is red on merged main: `/state/drift` reports fields the agent does not implement — `apps/api/src/state/state.controller.ts:222-228`, `apps/api/test/integration/agent.int.test.ts:129`
```
AssertionError: expected [ { op: 'remove', …(2) }, …(2) ] to deeply equal []
 + { op: remove, pointer: /interfaces/loop101/enabled, from: true }
 + { op: remove, pointer: /interfaces/loop101/promiscuous, from: false }
 + { op: remove, pointer: /routing/policy, from: {} }
```
`drift()` diffs the whole running projection of each implemented subsystem against `Retrieve`, but the P05 agent only projects part
of `interfaces`/`routing` (it answers `agent.unsupported-field` warnings for `enabled`, `promiscuous`, …, visible in every commit
response). Drift is therefore never empty on a real box, which makes the endpoint useless as a health signal and fails the P06 §10
test. **Fix (API side, no per-domain special-casing):** drop from the drift the pointers the agent's last DryRun/Apply reported
as `agent.unsupported-field`/`agent.unimplemented-domain` (or ask the agent for them via DryRun of running), or compare only keys
present in the Retrieve result for defaulted schema values; alternatively a LOG decision that Retrieve echoes unsupported fields
from its state dir (D-073(b) precedent). Keep the assertion; it caught a real problem.

### M1 — Operator commits admin-only (users/AAA) changes after a stale-lock takeover — `apps/api/src/commit/commit.service.ts:129-148`, `apps/api/src/datastore/datastore.service.ts:122-135`, `apps/api/src/datastore/lock.ts:371`
The operator check compares the *candidate before* vs *after* an edit, never *running vs candidate* at commit. When an admin's
candidate with user changes goes stale (`VRX_LOCK_TTL_SEC`), any operator takes the lock with a harmless edit and commits it.
**Probe (TTL 5 s):** admin stages a new `evil` admin user → operator PATCH while locked = 409 → after TTL operator PATCH
`/system` = 200 → operator `POST /commit` = **200 applied** → running users include `evil:admin`, `evil` logs in (200); the
revision's author is the operator. This violates "operator: no user/AAA changes" (P06 §6, questions #4 "checked on the whole
document").
**Fix:** in `commit()` (and `validate` for a clear 403) for non-admins: `adminOnlyChanges(hydrate(running), hydrate(candidate))`
→ 403 with pointers; add the scenario to the e2e.

### M2 — Operators can silently replace any secret, including the RADIUS/TACACS secrets of `management.aaa` — `apps/api/src/secrets/secrets.service.ts:186-205`, `secrets.controller.ts:277-292`
`POST /api/v1/secrets` with an existing `<kind>/<name>` overwrites the ciphertext (`created:false`) for any operator. That (a)
changes AAA by the back door (`management.aaa.*.servers[].secretRef` — questions #4 only considered VPN PSKs), and (b) changes live
behaviour of *committed* config with no candidate/commit/revision/rollback (a PSK swap is invisible to diff and rollback). Probe:
operator overwrote `psk/radius` twice → 200.
**Fix:** replacing a secret that running/candidate/pending references → admin only if referenced from `/management/**`, and in
general 409 unless `?replace=true`, with the change recorded as a system event; long-term, version secrets so a revision pins
the ciphertext it was committed with (manager decision → LOG).

### M3 — API ↔ VPP divergence when anything fails after the agent applied — `apps/api/src/commit/commit.service.ts:284-322, 361-367, 225-231`
- `agent.apply` has a 60 s deadline (`VRX_AGENT_TIMEOUT_MS`); on `DEADLINE_EXCEEDED`/connection loss after the request was sent the
  agent may have applied, the API returns 504/503 and running stays old.
- `promote()` (DB tx) failing after `APPLIED`, or after `CONFIRMED` in `confirm()`, leaves VPP on the new state and running on the old.
- `DEGRADED` is reported as 422 "running is unchanged" although the agent says the data plane is partially changed.
Nothing reconciles afterwards (drift is noisy, H2). Rule 4 "any failure rolls everything back" is not met on these paths.
**Fix:** after an Apply error/timeout or a failed promote, query `Health` (last applied txn id) and either promote the revision or
re-Apply the old running document; for DEGRADED say so in the problem and record an `error` system event (done) + mark running
"out of sync". Unit tests with the fake agent for each branch.

### M4 — `notApplied` domains are committed as "applied" — `apps/api/src/commit/validation.service.ts:85-88`, `commit.service.ts:381`
With the P05 agent every domain except `interfaces/vrfs/routing` is `notApplied` (probe: `["system","dataplane","nat","objects",
"acl","vpn","tunnels","services","ha","management"]`). A commit that only changes `acl`/`nat`/`vpn` returns `200 status:"applied"`
and a revision, and drift ignores those subsystems — for a firewall, "rule committed but not enforced" with only a side array as
signal. Also `management` is listed as not applied although the API itself applies `management.users` (confusing).
**Recommendation (questions #6):** refuse (409/422 `domain-not-implemented`, pointer = changed subtree) when the running↔candidate
diff touches a not-implemented domain unless `?allowNotApplied=1`; exclude API-applied parts (`/management/users`) from the list.
Manager decision → LOG.

### L1 — Route-guard enumeration test does not see routes registered in `main.ts`; `/api/docs` is unauthenticated — `apps/api/src/main.ts:17`, `apps/api/src/auth/route-guard.test.ts:141-149`
The test is not self-fulfilling for everything built by `createApp()` (it enumerates via `onRoute`, calls every route without
credentials, pins the public list — good), but `SwaggerModule.setup('api/docs', …)` runs only in `main.ts`, so `/api/docs` and
`/api/docs-json` never reach the test. Probe: both **200 without credentials** (full API surface disclosure). **Fix:** register
docs inside `createApp()` (so the test sees them) and either guard them or add them to the reviewed PUBLIC list deliberately.
The test also checks only 401-without-credentials; it does not check `@MinRole('admin')` routes against an operator token — add
a matrix row per route (readonly → non-GET 403, operator → admin routes 403).

### L2 — Secret delete ignores the pending confirmed commit — `apps/api/src/secrets/secrets.service.ts:208-219`
Checks running + candidate, not `config_pending.payload`; deleting a secret during a confirm window and then confirming leaves
running referencing a missing secret. **Fix:** include `config_pending`.

### L3 — Session hygiene
- Role/disable changes take up to 15 min for JWTs (questions #5, accepted), but **WebSocket connections never re-check**: a
  deleted/disabled/demoted user keeps `/api/v1/stream` open indefinitely (`relay.service.ts:208`). Close sockets at token `exp`
  or on a `users` change. No per-connection backpressure (`send` ignores `bufferedAmount`).
- Password change (`auth.service.ts:241-250`) does not revoke existing refresh families/API keys of that user.
- `VRX_COOKIE_SECURE` defaults to `0` (`config.ts:37-40`): production default should be `1` (dev overrides it).
- JWT secret: min 32 chars, HS256, iss/aud/typ checked — fine; no rotation path (no `kid`, no previous-key verify). Note for P07b/ops.
- No `trustProxy`: `X-Forwarded-For` spoofing is **not** possible (probe: audit shows `127.0.0.1`), but behind a reverse proxy all
  clients share one rate-limit bucket and audit IP. Document a `VRX_TRUST_PROXY` (exact proxy address) before a proxy is added.

### L4 — Secret store details — `apps/api/src/secrets/secrets.service.ts:144-171`
AES-256-GCM with a fresh random 96-bit IV per encryption (no nonce reuse), tag checked, key file created `wx` + 0600 and re-chmodded:
fine. Minor: no AAD (ciphertexts can be swapped between refs by a DB writer — bind `ref` as AAD); two concurrent first uses race on
`wx` (one 500); the file's owner is not checked.

### Info
- **Checked and fine:** guard is global (`APP_GUARD`) + WS authenticates in `preValidation` before the upgrade (no token in URL, no
  cookie auth on WS → no CSWSH); no CORS registered (same-origin only); refresh cookie httpOnly + SameSite=Strict + path
  `/api/v1/auth`; refresh rotation with family revocation on reuse (`tokens.service.ts:379-397`, `getdel` atomic); API keys
  `vrxk_` + 256-bit random, sha256 at rest, format-checked before the query, role-capped, live user role/disabled per request;
  argon2id m=19456,t=2,p=1 with a dummy verify for unknown users; operator restriction on PUT/import/merge-patch of the root and on
  rollback is document-level (verified in code + e2e); prototype pollution: `__proto__`/`constructor`/`prototype` rejected by
  `mergePatchAt`/`setAt` (400 with pointer, probed), Zod records drop `__proto__` keys; **no SQL built from pointers** (whole-document
  jsonb, Drizzle parameterised; only constant `sql\`\`` fragments); GET/candidate/diff/revisions/export/edit results/audit all go
  through `redactSecrets` (D-070), revisions store the redacted snapshot + hash, hashes only in `app_user`/candidate/pending rows;
  `grep child_process|execSync apps/api/src` empty; `.env.example` has no values for secrets.
- **Transaction semantics OK:** FAILED/ROLLED_BACK → 422 with results, running + candidate untouched (unit test with fake agent);
  commit/confirm/rollback serialised by a mutex + `SELECT … FOR UPDATE` on the singleton candidate; a pending confirmed commit blocks
  further commits (409); candidate edited during an Apply is kept and rebased.
- **History rewrite (D-P06-12):** task-branch commits recreated once to drop gitleaks-flagged literal test passwords; branch-only,
  same precedent as D-067 — note only. Current history (5 commits) is clean per gitleaks.
- **Merge with main:** only `pnpm-lock.yaml` conflicts (union). Resolve on the branch before merge.
- **Scope:** `/state/drift`, `/state/events`, `/audit`, `config_pending`, `mfa_secret` column are within P06's remit or justified
  by proto.md; no scope-creep objection.

## Required before merge
H1, H2 (test green on merged main), M1, and the lockfile merge. M2–M4 may land in this fix round or as tracked follow-ups with a
LOG decision (M2/M4 need one); L-items → tech-debt.

**APPROVE WITH CHANGES**
