# TD-2 — independent review (API follow-ups)

Reviewer: independent review agent (did not write this code). Branch `task/TD-2` @ `7f4578f`, base `main`.
Ran directly on the host, slot 7 (`w7`, DB `vrx_w7`, Valkey db 7). Probes were throw-away e2e files in the worktree. They were
deleted after the runs and are not committed. Cleanup: `ok nothing named vrx_w7 / vrx_w7 remains`, `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0`.
No processes were left running. The files in `/run/vrx-test/w7` are from 00:50–01:47 and are not mine.

## What was run

| run | result |
|---|---|
| `tools/ci.sh --base main` (HEAD 7f4578f) | **CI GATE PASSED**. Contract guard ok: only `packages/api-client/src/generated`, with `contract(api-client)` commits d7c83e2 and 097e014 plus `TD-2-contract.md`. The generated-output gate is clean, forbidden patterns and gitleaks are clean, turbo is 30/30 and agent `make` is ok. Log: `/root/ngfw-wt/logs/ci/TD-2-20260924-051524-2966541`. This matches the pasted run. |
| `tools/lab lock shared vitest -c vitest.e2e.config.ts` (auth, config, stream, td2, integration) | **41 passed, 3 skipped (44)**, the same as the pasted run. The agent test skips without `VRX_INTEGRATION=1`, which is acceptable because TD-2 changes nothing agent-facing. |
| Review probes 1 + 2 (same harness: host PG + Valkey + fake agent) | the results are quoted with each finding below |

## Contract / scope
- The api-client diff is **additive only**: 308 lines added and 2 removed. The removed lines are the old `/auth/password` summary and `content?: never` of `/health`. The contract commits and the contract note are present. **OK.**
- Scope: `GET /config/revisions/{rev}/diff` and the `/auth/password` refactor are inside items 6 and 1. No scope creep.
- P06 security fixes did not regress. The atomic lockout (P06 H1) e2e passes with 60 parallel attempts. The operator commit-time check (M1, `assertMayApply`) is intact. The route-guard enumeration includes the new route.

## Findings (by severity)

### H1 — The old password comes back: a reconcile promotes the in-flight document, which carries every user's hash from before the set
`apps/api/src/commit/commit.service.ts:544` (inflight `config: v.config`), `:684`, `:251` (`promote(f.config, …)`); `apps/api/src/commit/validation.service.ts:82` (the validated document is **hydrated with all app_user hashes**); `apps/api/src/datastore/pg-repo.ts:214` (`syncUsers` writes every hash the document carries); `apps/api/src/users/users.service.ts:92-116` (replaces the hash in the candidate and pending rows only, not in `CommitService.inflight`).

TD-2.md claims "a later commit cannot write the old hash back". The claim fails on the M3 lost-track path, and that path is TD-2.md question #3.
**Reproduced** with the fake agent and `VRX_AGENT_TIMEOUT_MS=800`:
1. `applyDelayMs=2500`, then commit → `504 running-unknown`.
2. Admin `POST /users/victim/password` → `204`. The new password logs in (`200`).
3. The agent comes back and `reconcile()` → `in-sync: "transaction … was applied; revision saved"`.
4. **Old password → 200, new password → 401.**

Because `v.config` is hydrated, this is not an edge case limited to one user. Every password set or changed between the lost answer and the reconcile is reverted: admin resets, self-service `/auth/password` and `/users/{name}/password`. The same happens after a failed `promote` (`:684`). The reconcile may retry for minutes while the agent is unreachable (`RECONCILE_RETRY_MS`), so the window is realistic. A typical case is an admin resetting a compromised account during an agent outage.

**Fix (pick one, then test with the fake agent):**
- (preferred) Keep hashes out of what `promote()` writes. Make `syncUsers` write only the hashes that the **unhydrated** candidate or pending document explicitly carries (staged by an admin). Keep the hydrated copy for validation and DesiredState only.
- Or make `setPassword` also rewrite `this.inflight.config`, under the same mutex.
- Or refuse a password set with 409 while `sync ≠ in-sync` or an inflight transaction exists.

Add an e2e for exactly the sequence above.

### H2 — The target's refresh chain can survive the password set (race between `consumeRefresh` and `revokeUser`)
`apps/api/src/auth/tokens.service.ts:163` (`exists rtfam` check), `:130-141` (`issueRefresh` re-`SET`s `rtfam:<fam>` and re-`SADD`s `rtuser`), `:178-182` (`revokeUser`); `apps/api/src/users/users.service.ts:118-119`.

The race works like this:
1. A refresh that passed the `exists rtfam` check is still in flight when `revokeUser` deletes the family.
2. `issueRefresh` then recreates the family.
3. `revokeAccess` has already run, so the new access token (`ims` > `at`) is accepted.

**Reproduced:** an attacker loop refreshed back-to-back with the target's cookie while an admin set the password. **2 of 12 chains survived**: after the set, `/auth/refresh` → 200 and `/auth/me` → 200, indefinitely. A thief with a stolen refresh cookie who scripts continuous refreshes keeps the session through the reset about one time in six. The same race affects the account-disable path, because the mechanism predates TD-2. TD-2 advertises "every session of the target ends", and the test does not cover this race.

**Fix:** make continuation atomic.
- In `issueRefresh` for an existing family, use `SET rtfam:<fam> … XX` (only if the key still exists) inside the MULTI, or a Lua script. Treat a no-op as revoked, and never `SADD` for a continued family.
- Better, add a per-user credential generation stored in Valkey or app_user. Embed it in refresh records and access tokens (`gen`), and compare it on consume and verify. That also fixes L3.
- Add an e2e that hammers refresh during a set.

### M1 — API keys of the target survive an admin reset and can mint new keys (TD-2 question #2)
`apps/api/src/users/users.service.ts:117-121` (no API-key revocation), `apps/api/src/auth/auth.controller.ts:203` (key creation needs no re-authentication).

**Probe** (operator `o`):
1. A key was created from a session, then an admin reset `o`'s password.
2. `GET /auth/me` with the key → **200**.
3. `POST /auth/api-keys` with that key → **201**: a fresh key minted after the reset.

The usual reason for an admin reset is a compromised account. An attacker who held a session for a minute can create a key that lasts up to 3650 days (`expiresInDays`) and so survive the reset. Severity: medium. It needs a prior session, but it defeats the purpose of the reset.

**Fix:**
- An **admin** reset of another user revokes that user's API keys by default. An explicit `keepApiKeys: true` may opt out. Record the revocation in the audit row as `apiKeysRevoked: n`.
- Self-service changes may keep keys.
- Separately, require the current password (step-up) to create an API key from a JWT session. Record both as LOG decisions.

### M2 — Wrong `current` is a brute-force channel with no lockout; the rate limit is per caller only
`apps/api/src/users/users.service.ts:60` (`pwset:<caller.id>` only), `:86-88` (a wrong `current` → 403 but no `failed_logins` increment), `:96` (a success resets `failed_logins` and `locked_until`).

**Probe:** 7 wrong `current` guesses from the victim's own session gave `403×5, 429×2`. Afterwards `failed_logins = 0` and `locked_until = null`. The attacker needs only a stolen access token (15 min) or a refresh cookie. They get 5 guesses per minute indefinitely, about 7 200 per day, and the account never locks. The login path allows 10 failures and then a 15-minute lock, about 960 per day, and it is audited as `auth.login failure`. The task asked for a limit per caller **and** per target. The per-target key is missing. That matters for admin callers via several admin accounts or keys, and for a thief rotating through several stolen sessions of one user.

**Fix:**
- Count a wrong `current` into the same atomic failed-login counter and lock as login (P06 H1 statement).
- Add `pwset:target:<u.id>` next to the caller key.
- Consider a daily cap. Audit rows exist (`failure/403`), which is good.

### L1 — "TLS or loopback" is only as strong as a proxy that is not in the repo yet
`apps/api/src/users/users.service.ts:17-29`.

- The XFF/Forwarded spoof is not possible because there is no `trustProxy`. **Probe:** remote `192.0.2.10` with `X-Forwarded-For: 127.0.0.1`, `X-Forwarded-Proto: https` and `Forwarded: for=127.0.0.1;proto=https` → **403**. Good.
- However, every request relayed by any local process counts as "TLS": nginx on `:80`, the Vite dev proxy, an SSH `-L` tunnel. No nginx config exists in `deploy/` today.
- The docstring claims the request "is refused before anything is read from the body". That is not true: the Nest body pipe has already parsed it.
- `POST /auth/login` carries a plaintext password with no such check, so the rule is inconsistent.

**Fix:** write the requirement into the nginx/packaging task: `:80` only redirects and never proxies `/api`. Correct the comment. Either apply the same check to login or drop the claim.

### L2 — Safe-text gaps
`apps/api/src/common/text.ts:13-14`, `apps/api/src/datastore/datastore.service.ts:184-185`.

- U+2028/U+2029 (LINE/PARAGRAPH SEPARATOR) are accepted. **Probe:** commit `comment=a b` → 200, API-key `name 'k x'` → 201. JS log viewers, JSON-lines tooling and some terminals treat them as line breaks, so log-line forgery is possible. Add them to both classes. U+061C and U+200E/F can stay (they are marks, not overrides).
- The document net filters by **pointer** (`known`). A string that already carried an unsafe character (legacy data) can therefore be replaced by *another* unsafe value at the same pointer. Compare values (`pointer + value`), not pointers.
- LF is allowed in every document string, so single-line fields depend entirely on schema patterns. This is acknowledged as out of scope. Track it for the schema owner.

### L3 — Access-token revocation lives in process memory (TD-2 question #1)
`apps/api/src/auth/tokens.service.ts:46`.

After an API restart, access tokens issued before the set are accepted again for up to `VRX_ACCESS_TTL_SEC`. The impact is low because the TTL is short and there is one process. The generation counter proposed in H2 covers this.

### L4 — A deleted API key keeps the candidate lock for the whole TTL
`apps/api/src/auth/auth.service.ts:249` (`deleteApiKey`), `apps/api/src/datastore/lock.ts:59-74`.

**Probe:** key A edits, then an admin deletes key A. `GET /config/lock` still shows `locked`, `ownerKeyId=<A>`, `ownerKey: null`, `expiresAt` 30 min later. The admin's own JWT PATCH → **409**. Only `DELETE /config/lock` (admin) frees it, so an operator is blocked for 30 min.

**Fix:** deleting a key releases the lock it holds, or `checkLock` treats a lock held by a no-longer-existing key as stale. A lock owned by a deleted key must still never become the user's interactive lock. That part is correct today.

### L5 — "self" is decided by username, not by id
`apps/api/src/users/users.service.ts:71`.

`name === caller.username` uses the JWT claim. Suppose a user is renamed and another account takes the old name within the access TTL. The old token's "self" then targets the other account. This is not exploitable, because that account's current password is required, but use `u.id === caller.id` after the lookup. For the pre-lookup authorisation, keep the username comparison.

### Info — checked and fine
- **AuthZ:**
  - A non-admin naming another user gets 403 before the lookup, so there is no enumeration or timing oracle.
  - Admin → any user. A 404 for an unknown user is shown to admins only.
  - Self requires `current` (400 with pointer `/current`, 403 when wrong).
  - A readonly user can call the route, and the route guard test pins this.
- **argon2id** m=19456, t=2, p=1 (OWASP minimum), computed server-side. Only app_user is written. No revision is created (the count is unchanged).
- **The password is never persisted anywhere else.** Probes found no password, `current` or `argon2` in `audit_log`, `config_revision`, `system_event`, GET candidate/running leaf (both 404), diff, export or the validate response. The audit row is `{passwordSet, self}`, and failures are audited as `failure/403` or `failure/429`.
- **Session end:**
  - Self change: the caller's own WebSocket stays open, and the other session's WebSocket is closed with 4403.
  - Admin reset: every WebSocket and access token of the target is refused (401).
  - API keys cannot open WebSockets (refused), so there is no WebSocket leak via keys.
- **Redacted diff (#6):**
  - Only `management.users[].passwordHash` is secret-marked. Every other secret is a `*Ref`. Staging a hash is admin-only, so the value-free `replace`/no-change signal gives only admins an equality bit about salted argon2 hashes, and admins can set passwords anyway. That signal is not an oracle.
  - Users are matched by username: a reorder is not a change, and a rename is remove plus add.
  - Entries have no from/to, so no length leaks.
  - A secret typed into a non-secret field (for example a description) cannot be detected, by design (D-051).
- **Per-key lock (#5):**
  - When two keys of one user edit at once, the second gets 409. That is acceptable, and D-P06-2 (single candidate) is kept.
  - Key B → commit, import and rollback all 409.
  - The user's own JWT → commit 409.
  - An admin key can `DELETE /config/lock`, which **discards** the candidate (the diff is empty afterwards). So key B can never commit key A's staged edits, except after the TTL takeover, which is the P06 semantics with its commit-time RBAC.
- **Rollback (TD-2 question #4):** consistent. Validation hydrates from the current app_user, so a rollback re-writes the *current* hashes and never restores old ones. This is intended per D-P06-3, and it should go into the LOG.
- **api-client dist (#2):**
  - `build` copies `schema.d.ts` to `dist/generated/`, and `dist/index.d.ts` imports `./generated/schema.js`, which resolves.
  - The consumer test checks through `exports` with a `paths`-is-not-`any` assertion and `@ts-expect-error` negatives.
  - Note: the package `test` depends on `gen` (uncached), so every test run rebuilds the OpenAPI document. It is slower but correct.
- **/health (#3):** one Zod definition feeds both the handler type and the OpenAPI response, and a unit test compares the two.
- **Test gaps (add alongside the fixes):** there is no e2e for WebSocket closure on a password set (only for the other paths), and none for the sync-unknown or reconcile interaction (H1).

## Required before merge
- H1 and H2, each with an e2e reproducing the scenario above.
- M1: implement it, or record a LOG decision that explicitly accepts it.
- M2: count wrong `current` attempts toward the lockout and add a per-target key.
- The L items may go to tech-debt.

**APPROVE WITH CHANGES**
