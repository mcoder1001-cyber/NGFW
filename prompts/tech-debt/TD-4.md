# Task: TD-4 — Auth hardening follow-ups from TD-2 (D-100 (1)–(3))   (prepend 00-CONTEXT.md)

## Goal
Close three auth gaps that TD-2 left open and D-100 decided. The work goes on top of TD-2's merged code:
1. **Login transport check.** `POST /api/v1/auth/login` gets the same transport check as a password set.
2. **Step-up for API keys.** Creating an API key from a JWT session requires the current password.
3. **Disable ends sessions.** Disabling an account bumps the credential generation, so its access tokens die immediately, as they do after a reset.

This is small and additive (board est. 3 h). Do not refactor TD-2's code. Add the three checks, prove each one, and adapt the callers the step-up would otherwise break.

## Read first (everything is on `main` once TD-2 is merged)
- `docs/decisions/LOG.md` **D-100** (the three items, and why they are not part of TD-2), D-097 (password lifecycle), D-102 (config-path reset)
- `docs/status/tasks/TD-2-questions.md`, Q1–Q3 and **Q7** (hint for (3)), and `docs/status/tasks/TD-2.md`, the "Fix round 2" section (the `credential_gen` design)
- `apps/api/src/users/users.service.ts`: `secureTransport()` and `isLoopback()` (the TD-2 #1 rule: a TLS socket or a loopback peer, with `X-Forwarded-*` not trusted)
- `apps/api/src/auth/auth.service.ts`: `login()` (hash and `credential_gen` come from one row read), `createApiKey()` (V1: `FOR SHARE` on app_user plus `sessionCurrent`), `registerFailure()` (lockout)
- `apps/api/src/datastore/pg-repo.ts` `PgConfigTx.syncUsers`, `apps/api/src/testing/memory-repo.ts` `syncUsers`, `apps/api/src/commit/commit.service.ts` `configResets` (the D-102 hook you extend)
- `apps/api/test/e2e/td2*.e2e.test.ts`: the e2e patterns (`remoteAddress: '192.0.2.10'` for a plain-HTTP remote peer, negative controls)

## Scope — build exactly this
1. **Login transport (D-100 (1)).** Move `secureTransport`/`isLoopback` to `apps/api/src/auth/transport.ts` and import them in users. `POST /auth/login` from a
   non-loopback peer over plain HTTP → 403 problem+json `tls-required`. The check runs **first**: before the rate limiter, the user lookup and
   argon2. So such a request does not count a failed login and does not change the lockout. Audit it as `auth.login` with failure reason `tls-required`
   (username only). Loopback and TLS behave exactly as before. Document the 403 in OpenAPI.
2. **Step-up for key creation (D-100 (2)).** `ApiKeyBody` gains an optional `current` field (string, 1–1024, the same shape as `PasswordBody.current`).
   - A **JWT** caller must send it. Missing → 400 with pointer `/current`. Wrong → 403, and the attempt goes through `registerFailure`, so it counts toward
     the lockout. Locked account → 403 `locked`, and no key row is written. The body carries a password, so the same transport check as (1) applies.
   - Verify `current` against the hash that was read **together with** `credential_gen`. Inside the existing `FOR SHARE` transaction, require that
     `credential_gen` is unchanged, as `login()` does. Never run argon2 while holding the row lock. Keep TD-2's V1 ordering intact.
   - An **ApiKey** caller does not need `current` (automation).
   - The audit row records `via: 'jwt' | 'apikey'` and never the password. Neither do logs or problem details.
   - Keep the OpenAPI **summaries** of `Auth_login` and `Auth_createApiKey` unchanged, so the generated CLI and SDK operation tables do not move. Put the new
     rule in the field and response descriptions.
3. **Disable bumps the generation (D-100 (3), TD-2 Q7).** In `syncUsers` (pg and memory), when an **existing** user's `disabled` flips false → true in the
   promoted snapshot, do these in the same promote transaction:
   - bump `credential_gen`;
   - return the user with a reason, next to the D-102 resets.

   After the commit, `configResets` (or a sibling) calls `tokens.revokeUser(id, gen)`. That ends access tokens, refresh chains and WebSockets now, not at TTL.
   It also writes one audit row: `action: config.user-disabled`, `resource: user/<name>`, `after: {disabled: true, via: 'config', revision, txnId}`.
   - API keys are **not** deleted. They are already refused while the account is disabled (`auth.service.ts`, the `row.user.disabled` check), and they
     work again after a re-enable. Record this in TD-4.md.
   - A confirmed commit disables at **confirm**, not while the commit is pending (the same rule as D-102).
   - These cases do not bump: re-enable, a user created already disabled, and the same flag staged again.
   - A hash change plus a disable in one commit bumps once and gives two audit rows.
   - D-102's behaviour for hash changes must stay byte-identical: all TD-2 e2e tests stay green.
4. **Callers of `POST /auth/api-keys` that the step-up would break.** Adapt each one with a minimal hunk:
   - **CLI** `apiKeyCreate` (`apps/cli/internal/cli/cmd_op.go`): with a login/session credential, take the current password from `--password-file`
     when it is set. Otherwise prompt `Current password: ` without echo (`a.readSecret`). If there is no terminal, fail with a usage error that tells the user
     to use a terminal, `--password-file` or an API key. With an API-key credential there is no prompt.
     Update the REPL e2e (`apps/cli/test/e2e/repl_e2e_test.go`) to answer the prompt. Its "password never on the terminal" check must still hold.
   - `test/topology/sdk-terraform-ansible/live.sh`: add `"current"` to the key-creation body. The password file is already there.
   - `docs/user/system/sdk-terraform-ansible.md`: update the key-creation example.
   - The existing api e2e files that create keys through a JWT: add `current` to those bodies, and change nothing else.

**Files you own:** `apps/api/src/{auth,users}/**`, `apps/api/test/e2e/td4-*.e2e.test.ts`, `docs/status/tasks/TD-4*`.

**Minimal hunks (name each one in TD-4.md):**
- `apps/api/src/datastore/{repo,pg-repo}.ts` (only `syncUsers` and its reset type)
- `apps/api/src/testing/memory-repo.ts` (only `syncUsers`)
- `apps/api/src/commit/commit.service{,.test}.ts` (only the post-promote hook)
- `apps/api/test/e2e/{auth,td2,td2-review,td2-verify}.e2e.test.ts` (only the key-creation bodies)
- `apps/cli/internal/cli/cmd_op.go` (only `apiKeyCreate`) plus its unit test
- `apps/cli/test/e2e/repl_e2e_test.go`
- `test/topology/sdk-terraform-ansible/live.sh` (one line)
- `docs/user/system/sdk-terraform-ansible.md`

**Generated (never hand-edit):** `packages/api-client/src/generated/schema.d.ts` goes in its own `contract(api-client): …` commit, together with
`docs/status/tasks/TD-4-contract.md`, as TD-2 did. The change is additive: a 403 on login, and `current` plus a 403 on key creation. For JWT callers it
tightens behaviour on purpose (D-100). If `make -C apps/cli gen docs` or `sdk/gen.sh` change their outputs, commit those regenerated files as well.

## Acceptance (paste the evidence into docs/status/tasks/TD-4.md)
- [ ] **(1) Login transport.** e2e `td4-*`: login from `192.0.2.10` over plain HTTP → 403 `tls-required`. `failed_logins` and `locked_until` are unchanged. The audit row has
      reason `tls-required`, and no password appears in `audit_log`. Loopback login → 200 (unchanged).
- [ ] **(2) Step-up.** e2e covers each case:
  - JWT without `current` → 400 with pointer `/current`;
  - wrong `current` → 403, `failed_logins` +1, and after `VRX_LOGIN_MAX_FAILURES` → 403 `locked` with no new `api_key` row;
  - right `current` → 200, and the key authenticates;
  - ApiKey caller without `current` → 200, audited `via: 'apikey'`;
  - plain-HTTP remote JWT caller → 403 `tls-required`;
  - the password string does not appear in `audit_log`, `system_event` or the API log;
  - TD-2's V1 race e2e is still `0 working keys, 0 rows`.
- [ ] **(3) Disable.** e2e: user X holds an access token, a refresh cookie, an open WebSocket and an API key. The admin commits `disabled: true`. Then:
  - access token → 401 immediately;
  - refresh → 401;
  - the WebSocket is closed;
  - the API key → 401;
  - one audit row `config.user-disabled`.

  Re-enable → login 200, the old access token is still 401, the API key works again. With a confirmed commit, X stays alive while the commit is pending and is cut off at confirm.
  **Negative control:** the same e2e against TD-2's `syncUsers` (patched back temporarily, restored with `git checkout`) shows the old access token still
  200. Paste that failure.
- [ ] Unit: `pnpm --filter @ngfw/api test` (new cases for the memory repo, `configResets`/disable and the transport helper) — pasted
- [ ] e2e, whole API suite on your slot:
  ```
  eval "$(tools/lab env <slot>)"
  tools/lab lock shared pnpm --filter @ngfw/api test:integration
  ```
  Paste the output: TD-2's 61 tests plus yours pass, and the 3 agent-int tests skip. The teardown ends with `nothing named vrx_w<slot> … remains`.
- [ ] Generated outputs:
  - `pnpm gen`, then `git status --porcelain packages/api-client/src/generated` → a contract commit;
  - `make -C apps/cli gen docs lint test` → green, and the generated files are unchanged or committed;
  - `sdk/gen.sh --check --openapi packages/api-client/openapi.json` → clean.
- [ ] CLI REPL e2e on your slot, with the lock held only for the run:
  ```
  deploy/dev/pg-test.sh create w<slot>
  tools/lab lock shared bash -c 'apps/cli/test/devstack.sh start && make -C apps/cli e2e; apps/cli/test/devstack.sh stop'
  deploy/dev/pg-test.sh drop w<slot>
  ```
  Paste the output, and NRestarts before and after (the stack's agent creates `loop<slot>01`).
- [ ] `tools/ci.sh --base main` green (under the CI lock named in your envelope). Every process you started is stopped by PID. The slot DB and Valkey keys are gone (`valkey-cli -n <slot> --scan --pattern 'vrx:w<slot>:*' | wc -l` → 0).

## Out of scope (do not build)
- The product nginx rule "`/api` is never proxied from plain :80" (D-100 (1)) belongs to P10. Only mention it in TD-4.md.
- Moving the Users page to `POST /users/{name}/password`, or any apps/web change (F-aaa / P07b).
- Session revocation on **role demotion** or **user deletion**. These are not in D-100: raise them in the questions file if you see the gap, and do not build them.
- The committer's own session on the config path (TD-2 Q6, a manager decision). Keeping access-token revocation in Valkey on every request (TD-2 open question 1).
- Schema-level single-line patterns (D-100 (4) → the `packages/schema` contract branch, tech-debt).
- MFA/TOTP/WebAuthn, RADIUS/TACACS, and password policy (F-aaa).
- New DB columns or migrations (`credential_gen` already exists), new env vars, and any bypass switch for the transport check.
- Refactoring TD-2's token/revocation design. Touching `apps/api/src/state/**` (P08), `apps/api/src/features/**` or `app.module.ts` (wave-A features).

## Open questions to surface, not to decide silently
- Should re-enabling an account also require its API keys to be re-issued? The default here is no: keys are refused while the account is disabled and work again after re-enable.
- Is there any flow in `tools/`, `test/` or `deploy/dev/` that reaches the API over plain HTTP from a non-loopback address? If so, list it. It now gets `tls-required` at login.
