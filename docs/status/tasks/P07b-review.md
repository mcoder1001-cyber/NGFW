# P07b review — UI flows (login, pending-change bar, commit dialog + confirm countdown, revisions + rollback, users)

Reviewer: independent review agent · 2026-09-24 · branch `task/P07b` @ `c4513b2` · base `main@78539ec`. `git merge-tree` against
current `main` (26 commits ahead) is clean.

## Checklist

| # | Check | Result |
|---|---|---|
| 1 | Contract compliance | OK. `git diff --name-only main...task/P07b -- packages/schema packages/proto apps/agent/gen packages/proto/gen packages/api-client/src/generated` is empty. The changes stay inside `apps/web/**`, `packages/ui-kit/src/ws/**` and `docs/status/tasks/P07b*`. |
| 2 | Real verification | OK for a UI task. The browser E2E drives headless Chrome against the real P06 API and the real P05 agent (owner `w1`) on the host VPP. I re-ran it (below). The users-only changes create no VPP objects. |
| 3 | Restart safety | N/A (no new VPP object type). |
| 4 | VPP API provenance | N/A. `binapi/` is untouched. |
| 5 | Shared-host rules | OK. Slot-1 ports, `vrx_w1`, Valkey db 1 / `vrx:w1:`. Vite binds 127.0.0.1 with `strictPort`. Prefixed test user `w1ro`. The author's leftovers `/run/vrx-test/w1/{admin.pw,jwt.key,agent-state/}` are still there (questions #7), so the manager has to remove them. |
| 6 | Security | **Findings H1, M4, M5 below.** No `dangerouslySetInnerHTML`, `innerHTML`, `eval` or `new Function` in `apps/web` or `packages/ui-kit`, and no `child_process`. Token storage is correct (see "Verified" below). |
| 7 | Transaction semantics (UI) | **Findings M1, M2, M3.** The countdown math is correct and ends early rather than late. The outcome is read back rather than assumed. |
| 8 | UI honesty | Every screen calls real P06 endpoints. The only stand-in is `src/test-api.ts`, used by unit tests only (verified not in `dist/`). No TODO, mock or stub markers. There are 23 screenshots (en/fa). **But** M1: the apply result of a confirmed commit is thrown away. |
| 9 | Scope creep | Minor and justified: a self-service password change in the user menu, the language switch on the login page, and the WS `protocols` getter in ui-kit (needed for D-P06-9). |
| 10 | i18n / RTL | en/fa key parity holds (auth 28/28, config 100/100, revisions 31/31, users 49/49, checked by script). The added lines have no physical left/right CSS, and `check-logical-css` passes. Technical values are `dir="ltr"` and server text is `dir="auto"`. Low findings on digits and the DataGrid footer (L6). |
| 11 | Tests actually run | OK. `tools/ci.sh --base main` in this worktree with the slot-1 env gives **CI GATE PASSED, EXIT=0** (wall 3m37s; turbo 30/30 successful, 24 cached). Because turbo hid most runs behind its cache, I also ran them directly: web `vitest` **56/56** (9 files), ui-kit `src/ws` **13/13** (5 files), and `pnpm build` gives initial **332.5 kB** gz of 600 kB with `check-no-dev-routes: OK`. All of these match the pasted output. |

### E2E re-run (real stack, slot 1, by PID; lab lock held shared only for the agent + test run, D-094)
Fresh `vrx_w1` (`pg-test.sh create w1`), API `apps/api/dist/main.js` on 3100, vite on 5100, `vrx-agent` owner `w1` with a fresh
state dir. The run was `flock -s /run/lock/vrx-lab.lock run-e2e.sh`, and the agent was stopped as soon as the run ended.
```
$ node apps/web/test/e2e/flow.e2e.mjs --langs en,fa --keyboard --revert --shots <scratch>
ok   [en] protected route redirects to /login?next=%2Fsystem%2Fusers
…  (same 50 checks as the status file: commit w/o revert, confirm ×2, rollback+confirm, readonly, fa pass, keyboard pass)
ok   [en] after the deadline the UI reports "Commit reverted It was not confirmed in time, … still in the candidate."
ok   [en] no revision was written for the unconfirmed commit (6 → 6)
E2E PASSED (50 checks)
agent stopped; E2E_RC=0
agent log: confirm timer armed ×6 · transaction confirmed ×5 · confirm timeout: reverting ×1
```
Teardown: vite, API and agent stopped by PID. `pg-test.sh drop w1` → `ok nothing named vrx_w1 / vrx_w1 remains`. 24 Valkey
keys `vrx:w1:*` in db 1 deleted (0 left). Ports 3100/5100/9111 are free. No `agent.sock` is left.

### Q#8 probe against the real API (same stack, before teardown)
```
PATCH /config/management {users with only w1ro.passwordHash changed}  → 200
GET /config/diff                                                       → 200, changes: 0
GET /config/lock                                                       → locked: True, owner: admin
POST /config/discard                                                   → 200 (lock released)
```

## Findings (ranked)

### High

**H1 — A password-hash (write-only) edit cannot be reviewed or committed from the UI, holds the lock invisibly, and rides along unseen in the next commit** (answers Q#8: it is a security problem *and* a UX bug)
`apps/web/src/config/PendingChangeBar.tsx:71` (bar hidden when `changes.length === 0`), `config/CommitDialog.tsx:184` (Commit disabled
with 0 changes), `pages/RevisionsPage.tsx:165` (`dirty` from visible changes only), `pages/UsersPage.tsx:107` (row state compares redacted users)
- Failure scenario: an admin resets `alice`'s password in Users by entering a new hash, the only way to set another user's password (Q#3). The PATCH succeeds and the candidate now holds the lock (probe above). No pending-change bar appears, the row shows no "changed" chip, and there is no Commit or Discard. From the UI, the change **cannot be committed at all**, so the password is never reset. Meanwhile:
  - Other operators get `candidate-locked` with no visible cause.
  - Rollback stays enabled and fails with `candidate-dirty`.
  - The admin's next unrelated commit, for example an MTU, silently commits the credential change too. The commit dialog, the revision diff (revisions are redacted, D-P06-3) and the Users page never show it, so the review step the product is built around is bypassed for exactly the most sensitive field.
- Fix (UI, in P07b's files, now):
  - Treat "the lock is held by me, or `/config/diff` has changes" as dirty. When the lock is held with 0 visible changes, show the bar with "Changes to write-only fields (not shown)", and allow Commit/Discard.
  - In Users, remember the usernames whose hash was set in this candidate and show a "password changed" chip.
  - Use the same dirty rule for the Revisions rollback guard.

  Fix (API, follow-up `contract/` branch, P06 owner): `/config/diff` should emit `{op:'replace', pointer, redacted:true}` without values for secret leaves (additive). The UI then renders it as "password hash changed".

### Medium

**M1 — Under the default path (auto-revert ON), the real apply result is discarded; the user only ever sees "Commit confirmed"**
`config/CommitDialog.tsx:109-112`, `pages/RevisionsPage.tsx:88-90`, `config/confirm-store.ts:9-24` (TrackedCommit and outcome carry no result), `config/ConfirmBanner.tsx:173`
- Failure scenario: P06 returns the `pending` result with `notApplied`, `warnings`, `results` and `summary` (commit.service.ts:589-629). The confirm answer is only `{status:'confirmed', warnings:[], results:[]}`. The dialog closes on `pending` and the banner/outcome show only "saved as revision N". A NAT or VPN change that the agent does not implement is stored in running but not enforced, and the operator is told only "confirmed". Only the pre-commit *validate* prediction mentions it, and that can differ from the apply. The P07b brief requires applied / partially-applied / not-applied plus `notApplied` to be shown honestly. Without auto-revert the UI does show them (E2E "Committed and applied … /nat — … not applied").
- Fix: store `notApplied`, `warnings`, the non-OK `results` and `sync` of the pending result in `TrackedCommit`. Render them in the countdown banner ("will be stored but not enforced: NAT, VPN") and again in the confirmed outcome (derive "partially-applied/not-applied" from `notApplied` as P06 does, or ask P06 for an additive `applyStatus` on the pending result).

**M2 — Server loss plus a reload (or any non-401 refresh failure) drops the operator on /login, where no countdown exists**
`auth/session.ts:106-110` (`restore()` → anonymous when the refresh fails for network reasons), `auth/session.ts:156-158` (every non-OK refresh ends the session), `auth/AuthProvider.tsx:24-26`, `router.tsx` (ConfirmBanner only inside `RequireAuth`/AppShell)
- Failure scenarios:
  - (a) The operator commits a change that cuts their own access, the page looks stuck, and they press F5. `restore()` fails at the network level and the state becomes `anonymous`, so they are redirected to `/login` and sign-in fails "unreachable". The countdown that `confirm-store` persisted in sessionStorage "so a reload during the outage keeps it" is never rendered.
  - (b) A reverse proxy answers 502/504 while the API is down, or the API answers 503 (Valkey down) or 429, while the 80 % refresh timer fires. `refresh()` then treats it as expiry: the query cache is cleared, AppShell unmounts and the banner disappears mid-countdown with "session expired".
- Fix:
  - Only 401 (and 400/403 from `/auth/refresh`) end the session. Treat 5xx, 429 and network errors like the existing network-error path (keep the state and retry in 15 s).
  - In `restore()`, keep a distinct `offline` status instead of `anonymous` on a network error.
  - Render `<ConfirmBanner>` (local mode) on the login page, or in RequireAuth's waiting state, whenever `confirmStore.tracked` exists.
  - Add a unit test for each.

**M3 — No request timeouts: a black-holed device (the realistic "cut my own access") shows "waiting for confirmation" instead of "Reconnecting…", and refreshes hang across tabs**
`config/queries.ts:294-306` (and the other polls), `auth/session.ts:146-150`, `api.ts:24-27`
- Failure scenario: after a commit that drops the route to the operator, packets are silently lost; there is no RST. `fetch` does not fail for minutes. TanStack keeps the old successful `pending` data (`isError` false), so `unreachable` stays false. The banner says "Your commit is waiting for confirmation" with Confirm enabled, and a click hangs in "Confirming…". The local countdown itself continues, which is correct. `refresh()` inside the Web Lock hangs as well, which blocks every tab's refresh (single flight plus the cross-tab lock). The E2E and the outage demo only stopped the API process (connection refused, which fails fast), so this path is untested.
- Fix: combine the query `signal` with `AbortSignal.timeout(4000)` for polls and 10 s for mutations and refresh. `call()` already maps a thrown fetch to `unreachable`. Mark the banner unreachable when the last successful poll is older than 2 × `POLL_MS`. Test with a local TCP server that accepts and never answers.

**M4 — No cross-tab session coherence: sign-out in one tab leaves the others signed in, and a login as another user silently switches the identity of open tabs**
`auth/session.ts:133-140`, `auth/session.ts:185-191`, `auth/AuthProvider.tsx:22-34`
- Failure scenarios:
  - (a) Admin signs out in tab A on a shared console. Tab B still has a valid access token (P06 logout revokes only the refresh family; access JWTs live 15 min) and an open WebSocket. For up to about 12 minutes it can still edit, commit and confirm, until its scheduled refresh fails.
  - (b) Tab A signs in as `bob` while tab B was `alice`. Tab B's next scheduled refresh uses the shared cookie, and `accept()` replaces `alice` with `bob` without clearing the query cache or the confirm store. Tab B keeps showing `alice`'s cached data and "your commit" banners as `bob`.
- Fix: a `BroadcastChannel('vrx-auth')` (fallback: a `storage` event on a nonce key) that posts `logout` and `login:{userId}`. Receivers call `setAnonymous('signedOut')` or clear the cache. In `accept()`, if `user.id` changed, clear the query cache and `confirmStore` and close/reconnect the WS.

**M5 — "Break lock" is a single click that discards another user's uncommitted work**
`config/PendingChangeBar.tsx:136-140`
- Failure scenario: an admin misclicks next to the lock chip, and `DELETE /config/lock` discards the other user's candidate (D-P06-2) with no confirmation, no summary and no undo. Discard has a confirmation dialog; the more destructive action has none.
- Fix: add a confirmation dialog that names the owner, shows `lockedAt`/`lastActivity` and says "their uncommitted changes will be discarded".

### Low

- **L1 — Rollback is disabled when the running → target payload diff is empty.** `pages/RevisionsPage.tsx:140`. That blocks legitimate rollbacks that only re-activate pinned secret versions (D-P06-13), and re-applying a revision to recover `sync: degraded`. Fix: allow it with the note "no visible configuration change (secrets / re-apply)".
- **L2 — RBAC mirror gaps (UX only; the API stays the authority, and a 403 is shown verbatim).**
  - An operator sees Commit and Rollback enabled for candidates or targets that touch `/management/users|aaa` or secret refs, which the API refuses (D-P06-4).
  - `RevisionsPage.tsx:166` ignores lock expiry, while `PendingChangeBar.tsx:79` honours it, so the two disagree about a stale lock.
  - Fix: check the diff pointers for operator, and share one `lockedByOther()` helper.
- **L3 — `confirmStore` is reset only on an explicit sign-out.** `shell/UserMenu.tsx:88`. Session expiry and a password change (`UserMenu.tsx:122-126`) keep it, so the next user in the same tab sees the previous user's tracked commit as "your commit". Fix: reset in `AuthProvider` on `anonymous`, or key the store by user id.
- **L4 — Users writes the whole list from a snapshot up to 5 s old.** `pages/UsersPage.tsx:162-167`, `:334`.
  - Two tabs of the same admin can lose each other's edit.
  - Renaming a user silently drops its hash, because P06 keeps hashes by username, so the renamed user can no longer sign in.
  - Fix: `qc.fetchQuery` the candidate before building `next`, and warn on a username change without a new hash.
- **L5 — `safeNext()` accepts backslash paths.** `pages/LoginPage.tsx:152-153`. For `/\evil.example`, WHATWG URL parsing turns `\` into `/`. With the current router the cross-origin `pushState` throws, so the result is an error page rather than a redirect. Hardening: reject `\` and control characters, or resolve with `new URL(next, location.origin)` and require the same origin.
- **L6 — i18n digits and bidi.**
  - Numbers are interpolated raw, so they stay Latin when "Persian digits" is on: `RevisionsPage.tsx:60,97,103,116,142` (`{{id}}`, `{{from}}`, `{{to}}`), `DiffView.tsx:94` (`diff.count.*`), `CommitDialog.tsx:56` (`fa` renders "۱ تا 60", mixed digits) and `UserMenu.tsx:51`.
  - The ServerDataGrid footer is untranslated and bidi-garbled in fa ("of 4 4–1", screenshot 15). That is ui-kit/P07a, but visible here.
  - Free-text server strings in the revisions grid (comment, author) are not bidi-isolated; the API-side control-character tech-debt item (P13 H1) is still open.
  - Fix: pass `fmt.integer()` values, set the MUI X `localeText` for fa, and use `<bdi>` for free text.
- **L7 — `refresh()` does not guard `res.json()`.** `auth/session.ts:160`. A 200 with a non-JSON body (a captive portal or proxy page) rejects `restore()`, and the app spins forever in `unknown`. Fix: try/catch and treat it as unreachable.
- **L8 — The E2E runs only against the vite dev server.** The screenshots show the "Developer" nav group. Production-build gating and the served bundle are not exercised end to end. Fix: add a `vite preview` pass of the production build.
- **L9 — The initial bundle grew 237 → 332.5 kB gz (budget OK).** The commit and rollback dialogs could be lazy, since they are not needed for first paint.

### Info
- **Q#1, the api-client type stopgap: acceptable as a stopgap.** I confirmed that `packages/api-client/dist/index.d.ts` imports `./generated/schema.js` and that `dist/generated/` does not exist, so every consumer gets `paths = any`. The web `tsconfig` `paths` mapping type-checks against the generated source, while the runtime uses `dist` from the same turbo run, so there is no drift. The manager should file a named follow-up for api-client (copy the `.d.ts` in `build`, or point `types` at the source) and remove the mapping when it lands.
- For a users-only change the UI shows "Committed and applied" next to the agent warning "/management — … not applied". Both are server text shown verbatim. P06 applies users itself (`app_user`), and the agent warning is generic (Q#4, API side).
- The commit API takes no expected-candidate version, so what the dialog shows can lag the committed candidate by up to one 5 s poll (same user, another tab). API follow-up: `?expect=<candidate hash>` → 409 `candidate-stale`.
- Q#5: the Lighthouse a11y ≥ 90 acceptance item is still unmeasured, because of the environment. The keyboard-only path is proven. The manager should record this as an open acceptance item.

### Verified (no finding)
- **Token storage.** The access token lives only in memory (`Session.token`). Grep of `src` and the built `dist/`: localStorage holds only UI settings and MUI's colour-scheme keys; sessionStorage holds only `vrx.confirm` (txnId and deadlines, no credentials). There is no IndexedDB or service worker. The refresh token stays in P06's httpOnly `SameSite=Strict` cookie scoped to `/api/v1/auth`. The WS credential is sent as a subprotocol, never in the URL.
- **Refresh.** Refreshes are single-flight within a tab and serialised by a Web Lock across tabs, so there is no refresh-token replay that would revoke the family. The 401 → refresh → retry path uses a clone of the body. Logout clears the query and mutation caches and closes the WS in the current tab.
- **CSRF.** Every non-auth route needs the bearer header. The cookie routes (refresh, logout) are covered by `SameSite=Strict`.
- **XSS.** Comments, usernames, fullName, problem `detail`, agent messages and diff values are all rendered as React text. i18next `escapeValue:false` is correct with React, and `skipOnVariables` is the default. There is no HTML sink.
- **Password hash.** The widget is `type=password` with `autocomplete=new-password`. The API never returns the hash and the UI never displays or logs it.
- **Countdown.**
  - `serverOffsetMs` = Date + 999 − sentAt is the maximum plausible offset, so the local deadline is ≤ the server deadline. It is `min`-ed with sentAt + window, and `adjustDeadline` only shrinks it, so the countdown ends early, never late.
  - The API passes through the agent's own `confirmDeadline`.
  - The in-flight-poll race is prevented by invalidating (and so cancelling) the old poll before `trackPending`.
  - The outcome is read back from revisions (txnId), and the `--revert` E2E proves the reverted path.

**APPROVE WITH CHANGES**. Required before merge: H1 (the UI part), M1, M2, M3. M4 and M5 should be fixed in this branch or split out as a named follow-up before P08. The L items can be follow-ups.
