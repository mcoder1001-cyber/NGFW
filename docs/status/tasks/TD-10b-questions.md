# TD-10b — questions and hand-offs for the manager

1. **PENDING-tools-app-transport — resolved for the merge by review C1 (fix round 1, `fe41a982`).** `tools/app` starts its API
   with `VRX_TRUST_PROXY=none`: vite's loopback peer is the client again, exactly main's behaviour (plain-HTTP lab logins from
   other machines stay accepted, the recorded exception). The banner says so. The answer's listener hunk (TLS or loopback bind)
   removes the setting. Without that line the default (`loopback`) + vite `xfwd` would have refused every remote lab login.

2. **Rebase onto TD-10a (`userExclusive`).** TD-10a's `CommitService.userExclusive()` (409 `commit-busy` after the 1 s lock
   wait, also across API processes) is not on my base. `users.service.ts` uses a local equivalent,
   `withinCommitLock((f) => this.commits.exclusive(f), async () => { … })` (`users/commit-busy.ts`, same 409 body and
   `retryAfterSec: 2`, in-process only). When both are on main, the merger swaps that call for
   `this.commits.userExclusive(async () => { … })` and deletes `withinCommitLock`/`commitBusy` from `users/commit-busy.ts`
   (keep `CommitBusyDoc`, the OpenAPI decorator). The tests stay valid: unit `users/commit-busy.test.ts` goes with the helper;
   the e2e `td10b-audit` "manager addendum" case tests the route and must stay green.

3. **`configResets` does not write rows for `demoted`/`deleted` (commit/**, not mine).** PENDING-session-revocation option 1
   is ONE commit since fix round 1 (`cb74843d`; drop it with `git revert cb74843d` if the product owner picks another option)
   and lives in the `syncUsers` hunk: the resets come back with `reasons: ['demoted']` / `['deleted']`, and `configResets`
   already calls `revokeUser` for every reset — so the sessions end. It writes `config.password-reset` /
   `config.user-disabled` rows only for those two reasons; the commit's own audit row and the revision diff record the change.
   Suggest `config.user-demoted` / `config.user-deleted` rows in `configResets` (TD-10a's file; ~10 lines). Note: TD-10a's
   current branch writes `config.password-reset` for EVERY reset (it predates TD-4's `reasons`); the TD-4 → TD-10a rebase must
   keep TD-4's per-reason rows, or demotions/deletions would be audited as password resets.

4. **`testing/memory-repo.ts` `syncUsers` not mirrored** (not in my files): the in-memory repo of the commit-engine unit tests
   does not return `demoted`/`deleted`. Nothing depends on it today; a one-hunk parity change for whoever owns it next.

5. **Key-file owner check for the secret store's master key.** `auth/key-file.ts` `checkKeyFile(path)` (regular file, not a
   symlink, owner = the API's uid or root, no group/other bits) guards `VRX_JWT_KEY_FILE`. `secrets.service.ts` (TD-10a,
   "must not touch" here) still only `chmod`s `VRX_SECRET_KEY_FILE`; the call is one line before its `readFileSync`.

6. **P10 packaging of the break-glass.** `deploy/sbin/vrx-authctl` (root check, then `node $VRX_API_DIST/auth/break-glass-cli.js`;
   default dist `/usr/lib/vrx/api/dist`, default settings file `/etc/vrx/api.env` when readable — both P10's choice) and a
   `docs/user/` page for operators (unlock, locks, rotate-jwt-key; the key ring is `vrx:vrx 0600` and `vrx-authctl` takes
   `VRX_API_USER`, default `vrx`). P10's nginx (review L3): **overwrite** `proxy_set_header X-Forwarded-Proto $scheme;` (a hop
   that passes the client's own X-Forwarded-Proto through would let a forged `https` count; without the header every login
   through nginx is refused) and `proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;`; set X-Forwarded-Host too, or
   keep the API from ever reading host/origin (it reads neither today); suggestion: nginx reaches the API from a dedicated
   address (e.g. 127.0.0.2) and `VRX_TRUST_PROXY=127.0.0.2`, so other local processes are not trusted proxies.

7. **Password reset from the box is not part of the break-glass** (unlock only). A root-side password reset would have to
   repeat `UsersService.setPassword` (candidate/pending/in-flight hashes, commit lock) outside the API process; with only one
   admin and a lost password there is no recovery path yet — suggest F-aaa or P10 (e.g. `vrx-authctl reset` that talks to a
   loopback-only API endpoint). Needs a decision.

8. **Slot 5 is assigned to both TD-10a and TD-10b** (same `vrx_w5` database; the e2e global setup drops it at the end). I ran
   every e2e as prefix `w5b` (database `vrx_w5b`, Valkey db 5 under `vrx:w5b:e2e:`, port 3550) under the shared lab lock, so
   the two workers could not drop each other's database. Nothing named `w5b` is left.

9. **Not fixed, noticed (TD-4 verify V-I3):** `TokensService.hit` still reads an EXEC-time reply error as count 0 (fail open for
   that one request). Out of TD-10b's scope; a two-line tech-debt fix.

10. **docs/tech-debt.md:21** (P06: JWT signing-key rotation, `VRX_TRUST_PROXY`, key-file owner check) is done by TD-10b except the
    secret-store call site (item 5) — for the manager to tick.
