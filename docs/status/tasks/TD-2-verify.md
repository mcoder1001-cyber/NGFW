# TD-2 — verify review of fix round 1

Reviewer: independent review agent (did not write this code). Branch `task/TD-2` @ `a2748ce`, base `main`. Checked against
the previous review `cc70629` (TD-2-review.md), the fix-round section in TD-2.md, TD-2-contract.md and D-097.
Ran on the host, slot 7 (`w7`, DB `vrx_w7`, Valkey db 7). The probes were a throw-away e2e file in the worktree. I deleted it
after the runs and did not commit it. Cleanup: `ok nothing named vrx_w7 / vrx_w7 remains` after every run.
`valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0`, and `dbsize` → `0`. No processes were left running.

## What was run

| run | result |
|---|---|
| `tools/ci.sh --base main` (HEAD a2748ce) | **CI GATE PASSED** in 3m32s. Log: `/root/ngfw-wt/logs/ci/TD-2-20260924-073543-3229529`. The contract guard is ok: only `packages/api-client/src/generated`, with contract commits 097e014 and d7c83e2. The generated-output gate is clean, gitleaks is clean, turbo is 30/30 and agent `make` is ok. The only warnings are the non-conventional merge and review subjects. This matches the pasted run (HEAD 2d0bccb; the later commits are docs only). |
| `eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration` | **53 passed, 3 skipped (56)**. td2-review has 11 tests, all green, including H2 with 60 runs. This matches the pasted run. Afterwards no `ugen:*`, `rtfam:*` or `atrev:*` keys were left without a prefix in db 7, so the Lua `KEYS` are prefixed by iovalkey. |
| Probe P1: the target mints API keys while an admin reset runs (40 runs) | **Every run leaves at least one working key** (details under V1) |
| Probe P2: an admin resets a password through the config API (staged hash + commit) | the old sessions, the refresh chain and the API key all stay alive (details under V2) |

## Status of the previous findings

| finding | status | evidence |
|---|---|---|
| H1: reconcile or promote restores an old hash | **fixed** | I traced every `promote()` caller. `applyDocument` promotes `stored = stagedHashesOnly(v.config, doc)` (`commit.service.ts:534,654`). The lost-answer inflight copy is `stored` (`:558`), and so is the promote-failure copy (`:698`, same `config`). `confirm()` promotes the pending row, which is `stored` (`:624`, `:456`), and `setPassword` rewrites it. `reconcile()` promotes `f.config` (`:253`), which `replaceInflightHash` rewrites (`:353`). The re-apply branch does not promote (`:266-289`). Confirm-revert only drops the pending row (`:823`). Rollback payloads are redacted, because `redactSecrets` **deletes** the leaf, so no placeholder can be staged. Import goes through `withoutPasswordHashes`. The candidate never holds hydrated hashes: `edit()` bases on `c.payload` or the redacted running document. The reviewer's exact sequence is in the e2e, and I re-ran it. One small residual is V4. |
| H2: refresh races the reset | **fixed** | `ISSUE_SCRIPT` checks `rtfam == uid:gen` and writes it in one script, and it never re-creates a deleted family. `REVOKE_SCRIPT` is one `INCR`. The hash is committed before the `INCR`, and login reads `gen` before it reads the hash. So a login with the old password either fails its `expectGen` check or lands in `rtuser` before `SMEMBERS`, and its access token is below the new `gen`. The 60-run e2e reported 0 survivors, and I re-ran it green. The residual is V3 (Valkey failing after the DB commit). |
| M1: API keys survive a reset | **partly fixed** | Keys that already exist are deleted in the reset transaction. But keys **minted by the target's in-flight requests** survive every time (V1). The config-API reset path does not revoke keys at all (V2). |
| M2: guessing `current` | **fixed** | A wrong `current` goes through `registerFailure`, the same atomic statement as login. A locked account is refused (403). `pwset:target:<id>` counts next to the per-caller key. An e2e covers two admins alternating → 429 within 35 attempts. See the Info note on the success path. |
| L1: docstring and nginx | **fixed** (docstring) + question #1 to the manager | — |
| L2: U+2028/U+2029; pointer+value | **fixed** | `text.ts:14-20`, `newUnsafeTextIssues` `:87-92`, unit tests |
| L3: revocations lived in process memory | **fixed** | `atrev:<uid>` with TTL = access TTL + 5 s, reloaded at boot (`main.ts`, harness). The e2e starts a second app. |
| L4: a deleted key kept the lock | **fixed** | `releaseKeyLocks` runs in the same transaction as the delete, in both `deleteApiKey` and the reset |
| L5: "self" decided by name | **fixed** | `users.service.ts:102` uses `u.id === caller.id` |

**api-client (D-078): additive.** Against `main`, only two lines are removed: the old `/auth/password` summary and `content?: never` of `/health`. The fix-round changes are:
- `keepApiKeys?` and `ignoredSecrets?` are optional additions;
- the 204 → 200 `{self, apiKeysRevoked}` change is on `POST /users/{name}/password`, which exists only on this branch;
- `/auth/password` is still 204, and the web `UserMenu` uses it.

The contract note lists every change. Process nit (Info): the fix-round regeneration landed in `8dde3e4 wip(TD-2): salvage …`, not in a `contract(` commit. The guard passes only because the earlier contract commits exist. The manager may want to note this when merging.

## Findings (by severity)

### V1 — High: an admin reset does not reliably revoke the target's API keys. Keys minted by in-flight requests survive every run.
`apps/api/src/users/users.service.ts:155-169` (keys deleted inside the transaction; sessions revoked only after it commits),
`apps/api/src/auth/tokens.service.ts:206-222` (`this.revoked.set` runs after four Valkey round trips),
`apps/api/src/auth/auth.service.ts:240-260` (`createApiKey` is a bare `INSERT` with no re-check of the caller).

**Failure scenario.** An attacker holds a stolen session: an access token, a refresh cookie or a key. They script `POST /api/v1/auth/api-keys` in a loop, which is cheap, and can delete the previous key each time. The admin resets the password, the D-097 response to a compromise.

Any mint request that authenticated before the revocation took effect, and whose `INSERT` runs after the reset's `DELETE … WHERE user_id` statement, stays alive. The two cases are:
- a JWT request authenticated before `this.revoked.set`;
- an ApiKey request authenticated before the reset transaction committed.

READ COMMITTED means the DELETE never sees such a row. No lock conflict orders them either: the FK insert takes `KEY SHARE` on the app_user row, and the reset's `UPDATE` takes `NO KEY UPDATE`, which does not conflict. The attacker keeps a key that can be valid for up to 3650 days.

**Reproduced** (probe with the same harness and host PG + Valkey, 40 runs each, one reset per run):
```
4 loops (2 JWT, 2 ApiKey): keys minted during the reset that still work afterwards: via JWT 144, via ApiKey 65;
                           api_key rows left right after the reset: 209; runs with >=1 surviving key: 40/40;
                           survivors whose request STARTED after the reset answered: 0
1 sequential JWT loop:     via JWT 66; runs with >=1 surviving key: 40/40
```
None of the surviving requests started after the reset answered, so this is purely the in-flight window, and it is deterministic. The M1 e2e creates keys only before the reset, so it cannot see this. This is the same class of race as the original H2, and here it wins every time.

**Fix.** Make key creation conflict with the reset in PostgreSQL:
1. Add a credential generation (or `password_changed_at`) column to `app_user`. Bump it in the reset's `UPDATE`, in the same transaction.
2. `createApiKey` runs in its own transaction:
   - `SELECT credential_gen FROM app_user WHERE id=$uid FOR SHARE`. This conflicts with the reset's `NO KEY UPDATE`, so it waits for the reset to commit.
   - Refuse (401) when the caller's JWT `gen` is below it, or, for an ApiKey caller, when `SELECT 1 FROM api_key WHERE id=$keyId` is gone.
   - Only then `INSERT`.
3. Mirror the generation into Valkey as today, or have `auth.refresh` compare the chain's `gen` with the PostgreSQL column; it already reads the app_user row. That also closes V3.
4. Set the in-memory revocation before any Valkey call.
5. Turn P1 into an e2e: continuous minting during a reset → 0 working keys afterwards, and 0 `api_key` rows except keys created with the new password.

### V2 — Medium: D-097 is not applied when an admin resets a password through the config API (staged `passwordHash` + commit), which is how the shipped Users UI does it
`apps/api/src/commit/commit.service.ts:743-763` (`promote()` computes `changedSecrets`, then `syncUsers` writes the new hash, with no revocation), `:785` (`usersChanged` only re-checks WebSocket role and disabled state), `apps/web/src/pages/UsersPage.tsx:189` (the edit dialog stages a hash for an existing user).

**Reproduced:**
```
P2: after a staged-hash commit: old password login 401, new password login 200;
    target's old access token /auth/me 200, old refresh cookie 200, API key 200
```
D-097 says "an admin password reset revokes all of the target's sessions, refresh chains AND API keys by default". Today an admin who resets a compromised account from the Users page changes the hash and nothing else. The thief keeps the refresh chain indefinitely and keeps the keys. This behaviour predates TD-2, but it is now a D-097 violation on the main UI path, and TD-2 owns the password lifecycle.

**Fix (pick one; either is small):**
- After a promote, call the same revocation as `setPassword` for every existing user whose hash changed:
  - `changedSecrets` already lists `/management/users/username=<u>/passwordHash`;
  - revoke that user's sessions and chains and delete their keys, reusing the V1-safe path;
  - put `apiKeysRevoked` into the commit's audit row.
- Or refuse `passwordHash` for **existing** users on the config API (400 with a pointer: "use POST /users/{name}/password"). Staging would still be allowed for new users and for console crypt hashes. Record this in the LOG, and give P07b a follow-up row to switch the dialog to the password route. The P07b switch is already listed as out of scope in TD-2.md.

### V3 — Low: if Valkey fails after the DB commit, the password is changed but no session is revoked, and the admin gets a 500
`apps/api/src/users/users.service.ts:130-170`, `apps/api/src/auth/tokens.service.ts:207-223`.

The hash update and the key deletion commit first. If `REVOKE_SCRIPT` then throws (Valkey restart or timeout), nothing below it runs: no `INCR`, no `revoked.set`, no `bus.sessions`. The refresh chains keep generation g and continue once Valkey is back. The old access tokens stay valid until their TTL. The audit row records a failure, although the password did change. The window is narrow, because `hit()` at the top already needs Valkey. The effect, though, is a reset that silently did not end the sessions, and an admin who thinks the reset itself failed.

**Fix:** keep the generation in PostgreSQL (V1, point 1) so the refresh check is transactional, and call `revoked.set` before any Valkey call. At minimum, catch the error and return a problem that says "password changed, sessions NOT revoked; retry", and record that in the audit row.

### V4 — Low: `replaceInflightHash` runs inside the transaction before it commits
`apps/api/src/users/users.service.ts:154`.

If the transaction then aborts, the in-memory inflight copy already carries the new hash, while app_user keeps the old one. The request answers 500. A later reconcile promotes the "failed" password, and nothing revokes the sessions. An abort can come from a `40P01` deadlock:
- a reset holds the candidate row (`FOR UPDATE`) and waits on the target's `api_key` rows;
- a concurrent `deleteApiKey` of a lock-holding key holds its `api_key` row and waits on the candidate.

**Fix:** call `replaceInflightHash` after `db.transaction()` resolves, still inside `exclusive`.

### V5 — Low: `keepApiKeys: true` is not audited explicitly
`apps/api/src/users/users.controller.ts:65-70`.

D-097 says the opt-out is "audited", but the row is `{passwordSet, self, apiKeysRevoked: []}`. That row is the same for "the admin chose to keep keys" and "the user had none". **Fix:** add `keepApiKeys: true` and the number of keys kept. Also add `discardedCandidate: true` when `releaseKeyLocks` discarded a key-owned candidate. Its return value is ignored today, so the admin never learns that staged edits were dropped.

### Info
- **WebSocket race** (`telemetry/stream.route.ts:42-56`). A handshake that authenticated before `revoked.set`, but attached after `bus.sessions`, stays open until the token's `exp`, at most one access TTL, because `relay.service.ts:259` closes it then. This is bounded. It is fixed properly if `attach` re-checks the revocation once it is registered.
- **M2 success path.** A correct `current` does not re-check the lock atomically, unlike login (`auth.service.ts:99-109`). A right guess sent in parallel with the wrong guess that locks the account still succeeds. The per-target limit caps this at 5 per minute, so there is no real gain for an attacker. Using the same conditional `UPDATE … WHERE locked_until IS NULL OR <= now()` would keep the two paths identical.
- **`/auth/password`** passes `req.principal.username` (`auth.controller.ts:186-188`). Suppose an admin whose account was renamed calls it while the old name belongs to someone else. It then becomes an admin reset of that other account (no `current` needed). This is not an escalation, because admins can reset anyone, but passing the id would be cleaner.
- **Whole-document edits** (`PATCH /config`, `PUT /config/management/users`) can still stage hashes from an old snapshot. D-097 lists import, and import is covered. The other paths are explicit admin staging and collapse into V2's decision.
- **Upgrade.** Refresh families from before this change (`rtfam = "<uid>"`) fail once, and the users log in again. TD-2.md states this, and it is acceptable.
- **No scope creep** in the fix round. Tests are real: they run on the host PostgreSQL 18 and Valkey, with the fake agent only for Apply.

## Required before merge
- **V1**, with an e2e: continuous key minting during a reset leaves 0 working keys.
- **V2**: implement it, or record a LOG decision that refuses hash staging for existing users, plus a P07b follow-up row. The Users page must not remain a reset path that bypasses D-097.
- V3, V4 and V5 are small and should go in the same pass. Otherwise they go to tech-debt.

**APPROVE WITH CHANGES**
