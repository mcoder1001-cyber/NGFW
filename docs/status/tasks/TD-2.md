# TD-2 — P06 API follow-ups (password set, api-client types, /health schema, safe text, per-key candidates, redacted secret changes)

Branch `task/TD-2` (worktree `/root/ngfw-wt/TD-2`, slot 7: `w7`, DB `vrx_w7`, Valkey db 7). Base `main@b36b91c`.
Sources: P07b questions #1–#3, P13 review H1, D-093; item 6 added by the manager mid-task (P07b review H1).
Ran directly on the host; no servers were started (all API tests run in-process), and the lab lock was held shared only during test runs (D-094).

## What was built

| # | item | where | notes |
|---|---|---|---|
| 1 | `POST /api/v1/users/{name}/password` `{password, current?}` → 204 | `src/users/users.{controller,service}.ts`, `auth/tokens.service.ts`, `infra/bus.ts`, `telemetry/relay.service.ts`, `commit/commit.service.ts` (`exclusive`) | admin: any user; everyone (readonly too): their own, with `current` (400 pointer `/current` when missing, 403 when wrong). A non-admin who names another user gets 403 **before** the user is looked up, so there is no enumeration. argon2id is computed server-side and written to **app_user only** (D-P06-3/D-091). Revisions stay redacted and no revision is written. A hash that the stored candidate or pending commit already carries for that user is replaced in the same DB transaction, serialised with commit/confirm/rollback, so a later commit cannot write the old hash back. **TLS only**: the request must come over a TLS socket or from a loopback peer (nginx terminates TLS locally); otherwise 403 `tls-required`. **Rate limit**: `VRX_PASSWORD_RATE_PER_MIN` (default 5) per caller; wrong `current` attempts count toward it. **Audit**: `resource user/<name>`, `after {passwordSet, self}`, never the value. **Other sessions end**: refresh families revoked, access tokens issued before the change are refused, WebSockets closed. The caller's own session survives when they change their own password. `POST /auth/password` now uses the same code. |
| 2 | api-client ships types | `packages/api-client/{package.json,scripts/copy-types.mjs,test/,turbo.json}` | `build` = `tsc` + copy `src/generated/schema.d.ts` → `dist/generated/`. `types`/`exports` already point at `dist/index.d.ts`, which now resolves. `test` type-checks `test/consumer.ts` against the **built** package through its `exports` (self-reference). The test asserts `paths` is not `any`, typed `/health` fields, a `@ts-expect-error` on an unknown route and on a wrong body, plus the new TD-2 fields. A package-level `turbo.json` makes `test` depend on the package's own `build`. |
| 3 | `/health` response schema | `src/health/health.controller.ts` | `HealthOut` (Zod) is both the handler type and the OpenAPI 200 schema. The unit test checks the answer against the documented schema. |
| 4 | control characters / bidi overrides → 400 problem+json with pointer | `src/common/text.ts`, `common/zod.ts` (`SafeParamPipe`), `config/path.ts`, `datastore/datastore.service.ts`, DTOs | Rejected: C0 except TAB, DEL, C1, U+202A–202E, U+2066–2069. Checked on the commit/rollback `comment`, API key `name`, login `username`, `vrf` query, path params (`users/{name}`, `secrets/{kind}/{name}`, `api-keys/{id}`, `actions/{action}`) and config URL path segments. Checked as a whole-document net on the candidate write path (every string value and every member name that the edit adds; LF is allowed there for banners), **before** the schema so no problem text quotes such a key raw. The rule is emitted as `pattern` in OpenAPI. Passwords and secret values are exempt. |
| 5 | candidate + lock owner per API key | `datastore/lock.ts`, `repo.ts`, `pg-repo.ts`, `db/schema.ts`, migration `0002_td2_key_lock_secret_changes.sql` | `config_candidate.owner_key_id` (uuid). The owner identity is (user, key): an API-key session owns the candidate by itself, while interactive (JWT) sessions of one user still share it, as before. A 409 names the key: `locked by 'admin' (API key 'pipeline-a')`, with `lock.ownerKeyId/ownerKey`. The single-candidate design (D-P06-2) is kept. |
| 6 | secret leaves in diffs as redacted entries | `datastore/documents.ts` (`secretChanges`, `markSecretChanges`), `datastore.service.ts`, `commit.service.ts` (`promote`), `config.controller.ts`, `config_revision.secret_changes` | `GET /config/diff` adds `{op, pointer, redacted: true}` (no from/to) per changed secret leaf. It compares running and candidate hydrated from app_user and matches users by username, so a reorder is not a change. Every revision stores `secretChanges[]` (returned in the revision meta/list/commit result). New `GET /config/revisions/{rev}/diff` returns the redacted diff vs the parent plus those entries. Edit responses carry `secretChanges`, and audit rows mark the leaf `"<redacted>"` → `"<redacted:changed>"`. Secret *references* (`*Ref`) were already visible as ordinary changes. |
| — | OpenAPI + api-client regenerated (`pnpm gen`) | `packages/api-client/src/generated/schema.d.ts` | additive only; see `TD-2-contract.md`; commits `d7c83e2`, `097e014` (`contract(api-client): …`) |

## How it was verified (real output)

### CI gate: `tools/ci.sh --base main` (HEAD 097e014)
```
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  packages/api-client/src/generated/schema.d.ts
ok — contract commit(s) on the branch:
  097e014 contract(api-client): regenerate — /auth/password documents 429 (TD-2)
  d7c83e2 contract(api-client): regenerate — users password, /health schema, safe-text patterns, lock ownerKey, redacted secret changes (TD-2)

== tools (golangci-lint, gitleaks) ==
golangci-lint 2.13.2
gitleaks 8.30.1

== install (pnpm --frozen-lockfile --prefer-offline) ==
Lockfile is up to date, resolution step is skipped Done in 206ms using pnpm v12.5.1 

== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated

== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~107715 bytes (107.71 KB) in 927ms no leaks found 

== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    12 cached, 30 total Time:    2m7.599s  

== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.523s; ok  	ngfw/agent/internal/agent	9.303s; ok  	ngfw/agent/internal/contracttest	2.177s; …

== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.016s; 

== summary (quick) ==
  mode quick · wall time 4m43s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-045854-2848750

CI GATE PASSED
```

### Unit tests: `pnpm --filter @ngfw/api test` (no DB)
```
 ✓ src/config.test.ts (3 tests)
 ✓ src/auth/tokens.service.test.ts (2 tests)
 ✓ src/common/json.test.ts (5 tests)
 ✓ src/common/text.test.ts (16 tests)
 ✓ src/datastore/documents.test.ts (6 tests)
 ✓ src/config/path.test.ts (8 tests)
 ✓ src/datastore/datastore.service.test.ts (21 tests)
 ✓ src/auth/route-guard.test.ts (6 tests)
 ✓ src/health/health.controller.test.ts (2 tests)
 ✓ src/commit/commit.service.test.ts (19 tests)
 Test Files  10 passed (10)
      Tests  88 passed (88)
```
The new tests are: `text.test.ts` (12 characters rejected, Persian/TAB/LRM accepted, the OpenAPI `pattern`, document pointers); `tokens.service.test.ts` (access tokens refused after a set, kept session survives); datastore: **two keys of one user → 409 naming the key, no edit/discard by the other key or by the interactive session**, interactive sessions still share the candidate, stale key lock takeover, ESC/RLO/C1 in values and member names → 400 with pointer, LF/TAB allowed, **hash-only edit → one redacted change**, reorder is not a secret change, new user → redacted add; documents: `replaceUserHash`, `secretChanges`, `markSecretChanges`; path: ESC/CR/CSI/RLO/FSI in URL segments; health: answer = documented schema; route guard: the new route (readonly may call it).

### api-client consumer test (through turbo, forced)
```
@ngfw/api-client:build: $ tsc -p tsconfig.json && node scripts/copy-types.mjs
@ngfw/api-client:build: api-client: copied src/generated/schema.d.ts → dist/generated/schema.d.ts
@ngfw/api-client:test: $ node scripts/copy-types.mjs --check && tsc -p test/tsconfig.json
@ngfw/api-client:test: api-client: dist/generated/schema.d.ts present
 Tasks:    9 successful, 9 total
```
Without the copy step (plain `tsc`, as on main) the same test fails:
```
api-client: /root/ngfw-wt/TD-2/packages/api-client/dist/generated/schema.d.ts is missing — run the build (it copies the generated types)
test/consumer.ts(19,3): error TS2578: Unused '@ts-expect-error' directive.
test/consumer.ts(34,3): error TS2578: Unused '@ts-expect-error' directive.
```
`pnpm --filter @ngfw/api-client lint` → `openapi.json: validated … Woohoo! Your API description is valid.`

### e2e on the host PostgreSQL 18.6 + Valkey (fake agent): `eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`
```
create role vrx_w7
create database vrx_w7 (owner vrx_w7)
check  vrx_w7 as vrx_w7 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
 ✓ test/e2e/config.e2e.test.ts (15 tests)
 ✓ test/e2e/stream.e2e.test.ts (3 tests)
 ✓ test/e2e/td2.e2e.test.ts (13 tests)
 ✓ test/e2e/auth.e2e.test.ts (10 tests)
agent integration test skipped: VRX_INTEGRATION is not 1
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
 Test Files  4 passed | 1 skipped (5)
      Tests  41 passed | 3 skipped (44)
e2e teardown: deleted 77 Valkey keys vrx:w7:e2e:* in db 7
drop   database vrx_w7
drop   role vrx_w7
ok     nothing named vrx_w7 / vrx_w7 remains
```
The TD-2 file, verbose (`vitest run -c vitest.e2e.config.ts --reporter=verbose test/e2e/td2.e2e.test.ts`):
```
 ✓ … #1 … > admin sets another user’s password: old one dead, new one works, every session of the target ends
 ✓ … #1 … > a user sets their own password with the current one; their session survives, the others end
 ✓ … #1 … > RBAC: non-admins cannot set other users’ passwords (403, no user enumeration); unknown user 404 for admins
 ✓ … #1 … > plain HTTP from a non-loopback peer is refused (TLS only); nothing changes
 ✓ … #1 … > a hash staged in the candidate is replaced, so the next commit does not bring the old password back
 ✓ … #1 … > rate-limited per caller (429)
 ✓ … #3 GET /api/v1/health answers what the OpenAPI response schema says
 ✓ … #4 … > commit / rollback comments
 ✓ … #4 … > API key names, login usernames, path parameters, query parameters
 ✓ … #4 … > configuration documents: values and member names the schema leaves open, and URL path segments
 ✓ … #5 candidate + lock owner per API key (D-093) > two keys of ONE user edit in parallel without interfering
 ✓ … #5 candidate + lock owner per API key (D-093) > interactive sessions of one user still share one candidate (unchanged)
 ✓ … #6 secret leaves are visible as redacted changes (P07b review H1) > hash-only edit → one redacted diff entry → commit → revision diff shows it redacted; the value never leaves
 Test Files  1 passed (1)
      Tests  13 passed (13)
e2e teardown: deleted 34 Valkey keys vrx:w7:e2e:* in db 7
ok     nothing named vrx_w7 / vrx_w7 remains
```
What the key e2e tests assert:
- **#1**: no plaintext in `audit_log`/`config_revision`/`system_event`; no `argon2id` in revisions; the revision count is unchanged; the audit row is exactly `{resource: user/op1, after: {passwordSet: true, self: false}}`.
- **#5**: both keys PUT at the same moment → exactly one 200 and one 409 whose `lock.ownerKeyId` is the winner. The loser's discard/commit/PATCH and the user's own JWT discard all get 409. The winner's diff holds only its change, and it commits. The loser then gets a clean candidate and commits. Running has both interfaces, and the revisions are `loser, winner`.
- **#6**: the diff is exactly `[{op:'replace', pointer:'/management/users/<i>/passwordHash', redacted:true}]`. Commit → `revision.secretChanges` and `/revisions/{rev}/diff` show the same entry. The audit before/after carry `<redacted>`/`<redacted:changed>`. The hash does not appear in audit, revisions, or any of 7 GET endpoints, and app_user follows the committed hash.

The P06 e2e expectations changed on purpose (these are the only changes to existing tests):
- The audit check now requires that every `passwordHash` in `audit_log` is a redaction marker (#6). Before, it required the key to be absent.
- The self-service test now uses a second login as "the other session", because the caller's own session survives (#1).

### Cleanup
`vrx_w7` dropped (`ok nothing named vrx_w7 / vrx_w7 remains`). `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0`. The API keys were created only inside the e2e databases, and the tests delete them (dropped with the DB anyway). No processes were started: the API runs in-process in vitest, the agent is the in-process fake, and nothing listens on 3700/5700/9171. `/run/vrx-test/w7` still holds files from an earlier slot-7 user, timestamped 00:50–01:47, before this task. Its `agent.pid` points to no live process. I did not delete them because they are not mine.

## Decisions (for the LOG)
| id | decision | options | why |
|---|---|---|---|
| D-TD2-1 | A password set writes **app_user immediately** and never the running document. It replaces a hash that the candidate or pending commit already carries for that user, in the same transaction and under the commit mutex. `POST /auth/password` uses the same code. | (a) stage the hash in the candidate (P07b proposal) (b) app_user now | "ends the other sessions" and "never in revisions" need an immediate, out-of-document effect. Hashes already live only in app_user (D-P06-3), so the config document keeps only the hash, as today. |
| D-TD2-2 | "TLS only" means a TLS socket **or a loopback peer** (the local nginx TLS terminator). `X-Forwarded-Proto` is not trusted. | (a) trust the XFP header (b) socket/loopback | there is no proxy-trust configuration (review L3); a remote plain-HTTP peer is refused with 403 `tls-required` |
| D-TD2-3 | Ending sessions revokes refresh families (except the caller's own on a self change), refuses access tokens issued earlier (an in-process marker per user for one access TTL, using a new `ims` ms claim), and closes WebSockets (`exceptSid`). The target's **API keys are not revoked**. | — | JWTs were not revocable before; the appliance runs one API process |
| D-TD2-4 | Safe text: single-line inputs reject C0 (except TAB), DEL, C1, U+202A–202E and U+2066–2069. The document net allows LF/TAB and checks only strings and keys **added** by an edit, so pre-existing ones do not block. Passwords and secret values are exempt. | — | D-049 and P13 H1 without a packages/schema change |
| D-TD2-5 | Per-key candidates are implemented as **per-key lock ownership of the single candidate**: the owner is (user, key), and interactive sessions share the user's lock. `owner_key_id` has no FK, so a deleted key's lock goes stale (TTL) or an admin breaks it; it never becomes the user's interactive lock. | (a) one candidate per key (parallel candidates) (b) key-owned lock | D-P06-2 (single writer, single candidate) stays; the other pipeline gets a 409 naming the key and never touches the first pipeline's edits (F-sdk review M3) |
| D-TD2-6 | Secret changes are value-free `{op, pointer, redacted:true}` entries in `/config/diff`, in edit responses and in `config_revision.secret_changes` (with a new `/revisions/{rev}/diff`), plus `<redacted>`/`<redacted:changed>` markers in audit rows | (a) diff redacted docs only (b) marker entries | P07b review H1: a hash-only change must be reviewable and committable |
| D-TD2-7 | The api-client build copies the generated `.d.ts` into dist. A package-level `turbo.json` (`extends: ["//"]`) makes the package's `test` depend on its own `build`. | (a) point `types` at src (b) copy | consumers must type-check against what ships |

## Out of scope / left undone
- The real-agent integration suite was not re-run: TD-2 changes nothing agent-facing, and it skips without `VRX_INTEGRATION=1`.
- Schema-level patterns for the pattern-less descriptions and map keys (packages/schema, contract). The API-side net covers every write; the schema owner may still want them for the UI/JSON Schema.
- P07b UI adoption: switch the Users form from hash entry to `POST /users/{name}/password`, render `redacted: true` diff entries, drop the apps/web tsconfig mapping to the api-client sources.

## Open questions
1. Access-token revocation is kept in process memory. After an API restart with a persistent `VRX_JWT_SECRET`, tokens issued before a password set work again for up to 15 minutes. Should it move to Valkey? That would cost a per-request lookup, and JWT auth would then fail closed when Valkey is down.
2. Should an admin password reset also revoke the target's API keys (compromised-account case)? Today it does not (D-TD2-3).
3. Edge case: while `sync = unknown`, a reconcile may promote an in-memory, already-hydrated config that was validated before a password set, which writes back the older hash. Should that path re-hydrate?
4. A rollback never restores password hashes (app_user is authoritative, D-P06-3), so its revision diff shows no secret changes. Confirm this is intended.

---

## Fix round 1 (review cc70629 APPROVE WITH CHANGES, D-097)

The first fix-round worker died at about 05:45. Its uncommitted edits were salvaged as `8dde3e4`. I reviewed that diff and kept it.
On top of it I added unit tests for the new document helpers (`1940efa`) and updated the contract note (`2d0bccb`). I also checked that `pnpm gen` reproduces the committed `schema.d.ts`: no diff.

| finding | fix | verified by |
|---|---|---|
| **H1** reconcile/promote restores old hashes | `stagedHashesOnly(v.config, rawDoc)` in `commit.service.ts`. The pending row, the in-flight (lost-answer) copy and every promote keep only the hashes that the **unhydrated** candidate staged. Hydrated copies are used only for validation and DesiredState, so `syncUsers` never rewrites an app_user hash it did not get from an admin. `setPassword` also replaces a staged hash in `CommitService.inflight`, under the commit mutex. Import drops every `passwordHash` and lists them in `ignoredSecrets`. Rollback applies redacted revisions, so no hash is present. | e2e `H1 …` ×4: the reviewer's exact sequence (504 running-unknown, then admin resets `victim` and `staged`, then reconcile → `in-sync … revision saved`, then **old → 401, new → 200** for the hydrated and the staged user); confirm + confirm-timeout revert; rollback; import. Unit `stagedHashesOnly / withoutPasswordHashes` |
| **H2** refresh races reset | Per-user **credential generation** `ugen:<uid>` in Valkey. `rtfam` stores `uid:gen`. Continuing a chain is one Lua script (`ISSUE_SCRIPT`) that refuses when the family is missing or belongs to an older generation, and it never re-creates a deleted family. A reset is one Lua `INCR` (`REVOKE_SCRIPT`) that moves only the caller's kept family to the new generation. Access tokens carry `gen`. Login reads the generation **before** it checks the password, so a login that races a reset fails. | e2e `H2 — 60 runs`: 2 hammered chains per run plus a racing old-password login → **survivors 0** |
| **M1** keys survive reset | An admin reset deletes the target's API keys in the same transaction and releases any candidate lock they hold, unless `keepApiKeys: true`. The response and the audit row carry `apiKeysRevoked: [{id,name}]`. A self-service change keeps keys. | e2e `M1 default: keys revoked, listed and audited; the key's candidate lock is released`, `keepApiKeys: true keeps them; a self-service change keeps them too` |
| **M2** `current` guessing | A wrong `current` goes through `AuthService.registerFailure` (the same atomic statement as login). A locked account takes no guesses (403 `locked`). A per-target limit `pwset:target:<uid>` sits next to the per-caller one. | e2e `M2 …` (3 wrong guesses → locked, right current → 403, login → 401, an admin reset unlocks); `rate-limited per target too: two admins alternating → 429 within 35`; `td2.e2e` per-caller test now asserts the lock |
| L1 | Docstring corrected (the body is parsed before the check). The nginx requirement is in the questions file for P10. | — |
| L2 | U+2028/U+2029 rejected in both classes and in the OpenAPI pattern. The document net compares `pointer + value` (`newUnsafeTextIssues`). | unit `text.test.ts` (LS/PS; `review L2: legacy unsafe data …`) |
| L3 | Revocations persisted as `atrev:<uid>` (TTL = access TTL + 5 s) and reloaded at boot (`loadRevocations`). | e2e `L3 — access-token revocations survive an API restart` |
| L4 | `deleteApiKey` releases the key's candidate lock in the same transaction and discards the candidate. | e2e `L4: deleting a key releases the candidate lock it holds immediately` |
| L5 | `self` = `u.id === caller.id` after the lookup. Non-admins are still refused before the lookup. | e2e RBAC tests (unchanged, green) |

Side effect: refresh families issued before this change store `uid`, not `uid:gen`, so they fail once after the upgrade and the user logs in again. This is intended.

### Unit (`pnpm --filter @ngfw/api test`)
```
 Test Files  10 passed (10)
      Tests  90 passed (90)
```
plus `src/datastore/documents.test.ts`: `Tests  8 passed (8)` (2 new).

### e2e: `eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`
```
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 25377ms
   ✓ … H1 … > reconcile after a lost Apply answer: the set password stays (hydrated AND staged hashes)  3728ms
   ✓ … H1 … > confirmed commit: confirm, and the confirm-timeout revert, keep the password set meanwhile  3140ms
   ✓ … H1 … > rollback to a revision from before the set keeps the current password  330ms
   ✓ … H1 … > import never brings hashes: they are listed as ignored, the current password stays  380ms
   ✓ … H2 — 60 runs: parallel refresh chains and a racing login during a reset → 0 survivors  13191ms
   ✓ … M1 … > keepApiKeys: true keeps them (service users); a self-service change keeps them too  396ms
   ✓ … M2 — wrong `current` counts toward the lockout; a locked account takes no more guesses  488ms
   ✓ … L3 — access-token revocations survive an API restart (Valkey, TTL = token lifetime)  608ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests)
 ✓ test/e2e/config.e2e.test.ts (15 tests) 10859ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 6646ms
agent integration test skipped: VRX_INTEGRATION is not 1
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
 Test Files  5 passed | 1 skipped (6)
      Tests  53 passed | 3 skipped (56)
e2e teardown: deleted 1228 Valkey keys vrx:w7:e2e:* in db 7
drop   database vrx_w7
drop   role vrx_w7
ok     nothing named vrx_w7 / vrx_w7 remains
```
(The verbose reporter lists only tests slower than 300 ms. All 11 td2-review tests passed, including M1 default, L4 and the WebSocket-4403 test.)

### CI: `tools/ci.sh --base main` (HEAD 2d0bccb)
```
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  packages/api-client/src/generated/schema.d.ts
ok — contract commit(s) on the branch: 097e014, d7c83e2
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: gitleaks — … no leaks found
Tasks:    30 successful, 30 total
  mode quick · wall time 4m38s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-072528-3158417
CI GATE PASSED
```
Warnings: the untracked manager file `TD-2.continue-envelope.md` (not mine, not committed) and non-conventional subjects of the manager's merge and review commits. The salvage commit `8dde3e4` also changed `schema.d.ts` (the 200 body and `ignoredSecrets`). That change is additive to the new route, and the contract note lists it.

### Cleanup
`ok nothing named vrx_w7 / vrx_w7 remains`. `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0`. No processes were left running.

### Left open (see TD-2-questions.md)
- The account-disable path does not bump the credential generation. Disabled users are refused on refresh by the DB check, so they keep only the current access token, until its TTL. Bumping it would need a hook in `promote()`.
- Step-up (current password) for API-key creation from a JWT session was suggested by the review but is not in D-097, so it was not implemented. It needs a LOG decision.

---

## Fix round 2 (verify ae52906 APPROVE WITH CHANGES, D-102) — the last round

Everything fix round 1 fixed is kept. This round changes only what the verify listed. The design change behind V1 and V3: the
credential generation now lives in **PostgreSQL** (`app_user.credential_gen`, migration `0003_td2_credential_gen.sql`, additive:
`ALTER TABLE "app_user" ADD COLUMN "credential_gen" integer DEFAULT 0 NOT NULL;`). Every password write bumps it in the same
transaction. Valkey no longer holds its own counter (`ugen:*` is gone). Refresh chains (`rtfam = <uid>:<gen>`) and access tokens (`gen`)
still carry the generation they were issued under, but the checks that matter read the column.

| finding | fix | verified by |
|---|---|---|
| **V1** (High) keys minted by in-flight requests survive a reset | The reset's `UPDATE app_user` bumps `credential_gen` (`users.service.ts`). `createApiKey` is now one transaction (`auth.service.ts`): `SELECT credential_gen … FOR SHARE` on the caller's row, which conflicts with the reset's `NO KEY UPDATE`, so it waits for a reset in progress and then reads the committed row. A JWT caller must carry that generation, or be the session that a self-service change kept (`TokensService.sessionCurrent`). An ApiKey caller's key row must still exist. Only then is the key inserted. So either the reset's `DELETE … WHERE user_id` sees the new key (the insert committed first), or the mint sees the new generation or the deleted key and gets 401. Login reads the hash and the generation from **one** row and succeeds only if `credential_gen` is unchanged (`UPDATE … WHERE credential_gen = $read`). Refresh continues a chain only when `rtfam` equals `<uid>:<credential_gen now>` (the column is read after the token is consumed). | e2e `V1 — 40 runs`: 2 JWT loops plus 2 ApiKey loops **chaining through the newest key** during each reset → **0 working keys, 0 rows** (563–573 keys minted during the resets, across three runs). **Negative control**: the same test against the round-1 bare `INSERT` → `working afterwards 314, api_key rows left 314` (output below). e2e `V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can`. H2 (60 runs) is still green on the new checks: `survivors 0`. |
| **V2** (Medium) config-API hash staging bypasses D-097 | D-102 exactly. `PgConfigTx.syncUsers` notes the hashes before the promote. For every **existing** user whose hash the promoted snapshot changes, it applies admin-reset semantics in the **same** promote transaction: generation bumped, failed logins and lock cleared, **all** API keys deleted (no `keepApiKeys` on this path), and any candidate lock those keys held released. It returns the resets. Once the promote committed, `CommitService.configResets` ends the sessions (`revokeUser`, no kept session) and writes one audit row per user: `action: config.password-reset`, `resource: user/<name>`, `after: {passwordSet, self, via: 'config', revision, txnId, apiKeysRevoked[, discardedCandidate][, revocationPersisted:false]}`. Every promote goes through this: commit, confirm, and the reconcile of a lost answer. A new user with a hash is not a reset, and neither is the same hash staged again. A confirmed commit resets at **confirm**, when the hash reaches app_user, not while it is pending. Staging stays allowed, so the Users page keeps working. | e2e `V2 commit`: `{"me":401,"refresh":401,"apiKey":401,"keyRows":0,"oldLogin":401,"newLogin":200}` plus the exact audit row. e2e `V2 confirmed commit` (alive while pending, dead after confirm; `newbie` is not a reset). e2e `V2 10 runs` with keys minted during the config-path commit → **0 working, 0 rows**. Unit `D-102: …` ×2 (memory repo: generation, `revokeUser(id, 1)`, audit row, no hash in the audit). |
| **V3** (Low) Valkey fails after the DB commit | This is now harmless by construction. `revokeUser` first sets the in-process revocation and closes the WebSockets (`bus.sessions`), neither of which can fail. The Valkey part follows (kept family moved, other families deleted, `atrev` persisted) and runs in `try/catch`. If Valkey fails, refresh is still refused (the PostgreSQL generation moved), and so is key creation. The answer is 200. The audit row carries `revocationPersisted: false`, and the error is logged. `atrev` writes never downgrade: the Lua script keeps a newer generation. | e2e `V3`: `eval` of the revocation script made to fail → 200; the old `rtfam` **is still in Valkey**, yet the old access token → 401 and the old refresh cookie → 401; new login 200; audit `{…, revocationPersisted: false}`. Unit `Valkey failing after the commit: the in-process revocation and the WebSocket close still happen`. |
| **V4** (Low) `replaceInflightHash` before commit | It is called after `db.transaction()` resolves, still inside `exclusive`. The reset now also takes locks in the order `app_user → api_key → candidate → pending`, the same order as `deleteApiKey` (`api_key → candidate`), so the deadlock that the verify described cannot happen between them. | e2e `V4`: a forced deadlock (a second connection holds the candidate row and then wants the target's app_user row) aborts the reset → 500, `replaceInflightHash` **not called**, old password and old session still work. A successful reset calls it only when the new hash is visible from another connection. **Negative control** (round-1 position): `expected "replaceInflightHash" to not be called at all, but actually been called 1 times`. |
| **V5** (Low) `keepApiKeys` not audited | The audit row carries `keepApiKeys: true, apiKeysKept: <n>` on an opt-out, and `discardedCandidate: true` when a revoked key's candidate lock was released. The 200 body gains `discardedCandidate: boolean`, so the admin learns about it. This is a contract change, additive, in its own commit `7ab9c83 contract(api-client): …`, and `TD-2-contract.md` lists it. | e2e `V5`: audit `{passwordSet, self:false, apiKeysRevoked:[], keepApiKeys:true, apiKeysKept:2}`; response `{self:false, apiKeysRevoked:[{…'lock-holder'}], discardedCandidate:true}`, audit `discardedCandidate: true`, lock released |

**Behaviour to know (D-102 as written):** an admin who changes **their own** hash through the config API ends **all** of their own sessions and keys too. This path has no kept session and no `keepApiKeys`. `POST /users/{name}/password` is the path that keeps the caller's session. **Upgrade:** refresh chains and access tokens issued before the migration carry a generation that no longer matches (tokens from `main` have no `gen` claim). Users log in once again, as in fix round 1, and a pre-upgrade JWT cannot mint API keys.

### Unit (`pnpm --filter @ngfw/api test`)
```
 ✓ src/config.test.ts (3 tests)
 ✓ src/auth/tokens.service.test.ts (3 tests)
 ✓ src/common/json.test.ts (5 tests)
 ✓ src/config/path.test.ts (8 tests)
 ✓ src/common/text.test.ts (19 tests)
 ✓ src/datastore/documents.test.ts (8 tests)
 ✓ src/datastore/datastore.service.test.ts (21 tests)
 ✓ src/auth/route-guard.test.ts (6 tests)
 ✓ src/health/health.controller.test.ts (2 tests)
 ✓ src/commit/commit.service.test.ts (21 tests)
 Test Files  10 passed (10)
      Tests  96 passed (96)
```
New tests:
- `commit.service.test.ts`: `D-102: a hash changed through the config API ends the user’s sessions after the promote, audited via: config`, and `D-102: a confirmed commit resets at CONFIRM …`.
- `tokens.service.test.ts`: `Valkey failing after the commit: the in-process revocation and the WebSocket close still happen`, and `sessionCurrent: current generation, or the session a self-service change kept — nothing else`.

### e2e, the new file, verbose (`eval "$(tools/lab env 7)"; tools/lab lock shared pnpm exec vitest run -c vitest.e2e.config.ts --reporter=verbose test/e2e/td2-verify.e2e.test.ts`)
```
create role vrx_w7
create database vrx_w7 (owner vrx_w7)
check  vrx_w7 as vrx_w7 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w7/pg.env (0600) · DSN postgres://vrx_w7:<redacted>@127.0.0.1:5432/vrx_w7
V1: 40 admin resets, 4 minting loops each: minted 573 keys (573 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
 ✓ … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows 18828ms
 ✓ … > V1 — a session from before the reset cannot mint (401); the session a self-service change kept still can 309ms
V2 commit: {"me":401,"refresh":401,"apiKey":401,"keyRows":0,"oldLogin":401,"newLogin":200}
 ✓ … > V2 — D-102: a hash staged through the config API is an admin reset > commit: old sessions, refresh chain and API keys end; old password 401, new 200; audited via: config 508ms
 ✓ … > V2 — D-102: a hash staged through the config API is an admin reset > confirmed commit: sessions live while pending, end at confirm; a NEW user with a hash is no reset 659ms
V2 race: 10 config-path resets: minted 182 (182 in flight during the commit) → working afterwards 0, rows left 0
 ✓ … > V2 — D-102: a hash staged through the config API is an admin reset > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows 6579ms
 ✓ … > V3 — Valkey fails after the commit: 200, old access token and refresh chain still refused, audited 266ms
   (logged by the API during V3: "[Tokens] user 7: sessions revoked in this process and by the database generation 1, but Valkey could not be updated (simulated Valkey outage); old access tokens would be accepted again after an API restart within 905s")
V4: reset answered 500 after a deadlock
 ✓ … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone 1380ms
 ✓ … > V5 — keepApiKeys and a discarded key-owned candidate are audited (and the discard is answered) 428ms
 Test Files  1 passed (1)
      Tests  8 passed (8)
e2e teardown: deleted 150 Valkey keys vrx:w7:e2e:* in db 7
drop   database vrx_w7
drop   role vrx_w7
ok     nothing named vrx_w7 / vrx_w7 remains
```

### Negative control (not committed): V1 and V4 against the round-1 code
The round-1 bare `INSERT` in `createApiKey` and the round-1 position of `replaceInflightHash` (inside the transaction) were patched back in temporarily. Then `-t "V1 — 40 runs|V4"` ran, and the files were restored with `git checkout`:
```
V1: 40 admin resets, 4 minting loops each: minted 960 keys (909 by requests in flight during the reset) → working afterwards 314, api_key rows left 314
V4: reset answered 500 after a deadlock
   × TD-2 verify fixes e2e (round 2, D-102) > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows 22002ms
     → expected 314 to be +0 // Object.is equality
   × TD-2 verify fixes e2e (round 2, D-102) > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone 1198ms
     → expected "replaceInflightHash" to not be called at all, but actually been called 1 times
      Tests  2 failed | 6 skipped (8)
ok     nothing named vrx_w7 / vrx_w7 remains
Updated 2 paths from the index
```

### Full e2e / integration (`eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`)
```
create role vrx_w7
create database vrx_w7 (owner vrx_w7)
check  vrx_w7 as vrx_w7 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
V1: 40 admin resets, 4 minting loops each: minted 563 keys (563 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
V2 commit: {"me":401,"refresh":401,"apiKey":401,"keyRows":0,"oldLogin":401,"newLogin":200}
V2 race: 10 config-path resets: minted 204 (204 in flight during the commit) → working afterwards 0, rows left 0
V4: reset answered 500 after a deadlock
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 29699ms
H2: 60 runs, 120 hammered chains + 47 racing logins that got in → survivors 0
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 27824ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11857ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 11170ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 9775ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 6980ms
 Test Files  6 passed | 1 skipped (7)
      Tests  61 passed | 3 skipped (64)
e2e teardown: deleted 1342 Valkey keys vrx:w7:e2e:* in db 7
drop   database vrx_w7
drop   role vrx_w7
ok     nothing named vrx_w7 / vrx_w7 remains
```
The skipped file is `test/integration/agent.int.test.ts`, the real-agent test. It needs `VRX_INTEGRATION=1`, and TD-2 changes nothing on the agent side. Two existing expectations changed on purpose, because the 200 body now also carries `discardedCandidate: false`: `td2.e2e` #1 and the `td2-review` M1 self-service test.

### CI (`tools/ci.sh --base main`, HEAD 783a9e9; the later commits are docs only)
```
== VRX CI gate: quick ==
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  packages/api-client/src/generated/schema.d.ts
ok — contract commit(s) on the branch:
  7ab9c83 contract(api-client): regenerate — users password 200 body gains discardedCandidate (TD-2 verify V5)
  097e014 contract(api-client): regenerate — /auth/password documents 429 (TD-2)
  d7c83e2 contract(api-client): regenerate — users password, /health schema, safe-text patterns, lock ownerKey, redacted secret changes (TD-2)
WARN commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
== tools (golangci-lint, gitleaks) ==
== install (pnpm --frozen-lockfile --prefer-offline) ==
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~279129 bytes (279.13 KB) in 1.21s no leaks found 
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    12 cached, 30 total Time:    2m27.621s  
== apps/agent: make lint test build ==
ok  	ngfw/agent/cmd/vrx-startupgen	1.436s; ok  	ngfw/agent/internal/agent	9.367s; ok  	ngfw/agent/internal/contracttest	2.002s; ok  	ngfw/agent/internal/descriptors/abf	1.141s; ok  	ngfw/agent/internal/descriptors/acl	1.225s; ok  	ngfw/agen …
== test/ Go modules, unit mode (test/integration/smoke) ==
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.020s; 
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m38s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   2m29s
  apps/agent: make lint test build                   0m25s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
  mode quick · wall time 4m42s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-081118-3387037
CI GATE PASSED
```

### Cleanup
After every e2e run the teardown printed `ok nothing named vrx_w7 / vrx_w7 remains`, so the slot database is dropped. `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0` and `valkey-cli -n 7 dbsize` → `0`. No process was left running: the API runs in-process in vitest, the agent is the in-process fake, and the CI shell exited. The older files in `/run/vrx-test/w7` belong to an earlier slot-7 user and are untouched, as noted in fix round 1.

### Left over (not in the verify's required list, or moved elsewhere)
- Info items of the verify that were not changed:
  - the WebSocket attach race is bounded by the token `exp`;
  - the M2 success path does not re-check the lock atomically;
  - `/auth/password` passes the username, not the id.
  They are listed here for tech-debt. The WebSocket race is now narrower, because the in-process revocation is set before any Valkey round trip.
- D-100 moved these to **TD-4**, and they were not done here: the transport check on login, step-up for API-key creation, and "disable bumps the generation". With the generation in `app_user`, the last one is small: `syncUsers` can bump `credential_gen` when `disabled` flips to true and return it like a D-102 reset.
- A kept session can lose its refresh chain if its own refresh races its own password change in the microseconds between the PostgreSQL commit and the Valkey move of the kept family. This fails closed: the user logs in again.

## T1 fix (verify round 2 finding T1, D-111): bounded e2e timing, test code only

`TD-2-verify2.md` T1: `td2-verify.e2e.test.ts` overran the 30 s default on the shared host. One overrun cascaded into 3–5 failures and a
hung `afterAll`. This round changes **no product code**. `git diff --name-only a74adec..HEAD` (everything after the merge of main) lists only
`apps/api/test/e2e/{td2-verify,td2-review,td2}.e2e.test.ts`, `apps/api/test/support/bounded.ts`, `apps/cli/internal/api/operations_gen.go` (generated) and the two
status docs. `git diff --name-only 9bdc466..HEAD -- apps/api/src` → nothing. The regenerated CLI table was needed by the merge of main (see below).

| commit | what |
|---|---|
| `a74adec` | `git merge main` (clean, no conflicts; no `apps/api` change came from main) |
| `5b36365` `test(TD-2): T1 bounded e2e timing` | td2-verify: explicit budgets. `mintDuring` ends its loops in `finally`. Long tests run through `guarded`: a per-test `AbortSignal`, and `afterEach` aborts the body and **awaits** it, so nothing a test started outlives it. V4 now waits on real lock state and every wait is bounded: the helper transaction runs `SET LOCAL lock_timeout = '10s'`, then polls `pg_stat_activity` with `pg_blocking_pids(pid)` until the reset really waits on the helper (bounded at 15 s, replacing `sleep(300)`), then waits for the reset's answer with `Promise.race` (15 s) |
| `4391a44` | Shared helper `test/support/bounded.ts` (`within`, `guardLongTests`, `countWhere`). V1's survivor checks run 8 at a time; the sequential checks were most of V1's time under load (1624 keys → 155 s in the first full run). **td2-review H2** (the verify's optional item) gets the same treatment: `guarded`, loops ended in `finally`, 180 s budget. It was the only failure of full run 1 after `5b36365` |
| `47ebe60` | **td2.e2e** `rate-limited per caller`: pre-existing flake, failed once here (`expected 429 to be 200` at the admin unlock). The per-target neighbour had the same fixed-60 s-window dependency. Cause and fix: TD-2-questions **#9** |
| `ff9811f` | `make -C apps/cli gen`: generated file only. The merge brought main's CLI operations table, which did not list TD-2's routes, so the gate's `TestOperationsTableMatchesOpenAPI` failed. The regen adds `Users_setPassword` and `Config_revisionDiff` |

Budgets: V1 40 runs 300 s, V2 race 120 s, V4 60 s, H2 180 s, per-target rate test 90 s (it may wait up to 30 s for a fresh rate window).
Scope note: td2-review and td2.e2e are outside the envelope's `td2-verify*.ts`. Why they were touched: TD-2-questions **#8/#9**.

**Abort path checked.** With V1's budget cut to 5 s for one local run (not committed), V1 timed out. `afterEach` stopped its body: no late `V1:`
line, 8911 ms in total including the drain. The other 7 tests passed and `afterAll` returned:
```
 × … > V1 — 40 runs: … 8911ms
   → Test timed out in 5000ms.
 ✓ … > V1 — a session from before the reset cannot mint (401); … 2303ms
 ✓ … > V2 … > 10 runs: … 10872ms
 ✓ … > V4 — … 1335ms
 ✓ … > V5 — … 348ms
      Tests  1 failed | 7 passed (8)
ok     nothing named vrx_w7 / vrx_w7 remains
```

### Full api e2e on slot 7, three runs in a row on `47ebe60` (`eval "$(tools/lab env 7)"; tools/lab lock shared pnpm --filter @ngfw/api test:integration`)
The host was busy: two other slots' `tools/ci.sh` ran during run 1, and the 1-min load was 174 a few minutes before it started.

Run 1:
```
START 2026-09-24T15:37:51+03:30 HEAD 47ebe60 load: 15.92 67.69 59.47
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 97177ms
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 32902ms
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 161827ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 12008ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11398ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 7445ms
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
   ✓ … > rate-limited per caller (429)  7904ms
   ✓ … > rate-limited per target too (review M2): two admins alternating on one user → 429  40612ms
   ✓ … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows  19021ms
   ✓ … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows  5828ms
   ✓ … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone  1326ms
   ✓ … > H2 — 60 runs: parallel refresh chains and a racing login during a reset → 0 survivors  120711ms
V1: 40 admin resets, 4 minting loops each: minted 867 keys (867 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
V2 race: 10 config-path resets: minted 187 (187 in flight during the commit) → working afterwards 0, rows left 0
V4: reset answered 500 after a deadlock
H2: 60 runs, 120 hammered chains + 40 racing logins that got in → survivors 0
 Test Files  6 passed | 1 skipped (7)
      Tests  61 passed | 3 skipped (64)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 0 2026-09-24T15:44:49+03:30 load: 16.32 34.04 47.16
```
Run 2:
```
START 2026-09-24T15:44:49+03:30 HEAD 47ebe60 load: 16.32 34.04 47.16
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 134627ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 38616ms
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 45113ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 11432ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11667ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 6554ms
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
   ✓ … > H2 — 60 runs: parallel refresh chains and a racing login during a reset → 0 survivors  88162ms
   ✓ … > rate-limited per caller (429)  1333ms
   ✓ … > rate-limited per target too (review M2): two admins alternating on one user → 429  31488ms
   ✓ … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows  27434ms
   ✓ … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows  6432ms
   ✓ … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone  1408ms
H2: 60 runs, 120 hammered chains + 47 racing logins that got in → survivors 0
V1: 40 admin resets, 4 minting loops each: minted 725 keys (725 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
V2 race: 10 config-path resets: minted 190 (190 in flight during the commit) → working afterwards 0, rows left 0
V4: reset answered 500 after a deadlock
 Test Files  6 passed | 1 skipped (7)
      Tests  61 passed | 3 skipped (64)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 0 2026-09-24T15:49:49+03:30 load: 9.70 20.74 37.87
```
Run 3:
```
START 2026-09-24T15:49:49+03:30 HEAD 47ebe60 load: 9.70 20.74 37.87
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 31530ms
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 27547ms
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 9242ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 10884ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 12031ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 6197ms
 ↓ test/integration/agent.int.test.ts (3 tests | 3 skipped)
   ✓ … > H2 — 60 runs: parallel refresh chains and a racing login during a reset → 0 survivors  17205ms
   ✓ … > V1 — 40 runs: keys minted (JWT + chained API keys) while an admin reset runs → 0 working keys, 0 rows  16290ms
   ✓ … > 10 runs: keys minted while a config-path reset commits → 0 working keys, 0 rows  5596ms
   ✓ … > V4 — a reset whose transaction aborts (deadlock) leaves the in-flight copy, the hash and the sessions alone  1339ms
   ✓ … > rate-limited per caller (429)  1216ms
   ✓ … > rate-limited per target too (review M2): two admins alternating on one user → 429  2262ms
H2: 60 runs, 120 hammered chains + 48 racing logins that got in → survivors 0
V1: 40 admin resets, 4 minting loops each: minted 589 keys (589 by requests in flight during the reset) → working afterwards 0, api_key rows left 0
V2 race: 10 config-path resets: minted 201 (201 in flight during the commit) → working afterwards 0, rows left 0
V4: reset answered 500 after a deadlock
 Test Files  6 passed | 1 skipped (7)
      Tests  61 passed | 3 skipped (64)
ok     nothing named vrx_w7 / vrx_w7 remains
EXIT 0 2026-09-24T15:52:23+03:30 load: 8.58 18.25 34.48
```
Earlier full runs, for the record (not counted): after `5b36365`, td2-verify was green but td2-review H2 hit the 30 s default (`1 failed | 60 passed`).
After `4391a44`, `td2.e2e … rate-limited per caller` failed (`1 failed | 60 passed`) → `47ebe60`. I stopped two of my batches early by PID when a
fix invalidated them. Two orphaned vitest workers from those batches (slot 7, this worktree) were found and killed by PID before the counted runs.
H2 took 88–121 s in runs 1–2 at load 20–60, against 13–20 s on a quiet host; its 180 s budget covers that. If the host gets busier, the budget
fails on its own, without a cascade.

### CI — `TMPDIR=/tmp/g-td2 tools/ci.sh --base main` (HEAD `ff9811f`)
The first gate run on `47ebe60` failed only in `apps/cli` (`TestOperationsTableMatchesOpenAPI: internal/api/operations_gen.go is stale`) → `ff9811f`.
```
START 2026-09-24T15:59:07+03:30 HEAD ff9811f TMPDIR=/tmp/g-td2
branch    task/TD-2 @ ff9811f   (base: main)
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
ok: gitleaks — scanned ~356884 bytes (356.88 KB) in 896ms no leaks found
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    1m24.473s
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m32s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m26s
  apps/agent: make lint test build                   0m26s
  apps/cli: make lint test build                     0m13s
  test/ Go modules, unit mode (test/integration/smoke)   0m01s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-2): verify round 2 — partial (stopped at manager handover)
      review(TD-2): verify fix round 1
      merge main into task/TD-2
      review(TD-2): findings
  mode quick · wall time 3m45s · logs /root/ngfw-wt/logs/ci/TD-2-20260924-155907-897264

CI GATE PASSED
EXIT 0 2026-09-24T16:02:52+03:30
```

### Cleanup
After the runs, `valkey-cli -n 7 --scan --pattern 'vrx:w7:*' | wc -l` → `0` and `dbsize` → `0`. Nothing listens on 3700/5700/9171, and no
process from this worktree is running. The build output (`dist/`, `apps/agent/bin`) was deleted after the gate.

### Left for the manager
- `docs/user/cli/reference.md` (from `make -C apps/cli docs`) does not list the two TD-2 routes yet. The gate does not check it, and it is not in this scope.
- TD-2-questions #8/#9: the scope extension to td2-review H2 and to the td2.e2e rate tests.
