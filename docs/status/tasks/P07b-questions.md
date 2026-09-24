# P07b — questions for the manager (none blocked the work; each has a default that is implemented)

1. **packages/api-client ships no types.** `dist/index.d.ts` imports `./generated/schema.js`, but `tsc` never copies the
   generated `src/generated/schema.d.ts` into `dist`, so every consumer sees `paths = any` (all API calls untyped — P07a's
   HealthCard compiled only because of that). Default: `apps/web/tsconfig.json` maps `@ngfw/api-client` to its
   *source* for type-checking only (runtime still uses dist). Fix owner: api-client (copy the d.ts in `build`, or point
   `types` at `src/index.ts`). Options: (a) copy step in api-client build (recommended) (b) keep the web mapping.
2. **`GET /api/v1/health` has no response schema** in the OpenAPI document (`content?: never`). The HealthCard reads the
   body defensively. Owner: apps/api (add `@ApiOkResponse` schema).
3. **Setting a user's password from the UI.** `management.users[].passwordHash` needs an argon2id PHC string for web/API
   sign-in; the browser cannot compute it (no argon2 in the stack, no new deps). Implemented: the Users form takes a hash
   (write-only, empty = keep) with an explanatory note; self-service password change is in the user menu. Proposal
   (P06-questions #2 option b): an admin action `POST /api/v1/auth/users/{name}/password {password}` that hashes
   server-side and stages the hash in the candidate. Options: (a) keep hash entry (b) API action (recommended) (c) argon2
   WASM in the browser (new dependency).
4. **Validation warnings list every unimplemented domain** (`/management …, /nat …, /system …`) on every validate/commit,
   also for a users-only change. Shown verbatim (UI honesty). Should DryRun warnings be limited to the changed domains
   (API side)?
5. **Lighthouse a11y ≥ 90 not measured**: neither Lighthouse nor axe-core exists on the host and packages may not be
   installed. Covered instead: keyboard-only login → commit → confirm in the real browser E2E, labelled controls,
   `aria-live` regions, skip link, focus ring (P07a). Please run Lighthouse where it is available or allow axe-core as a
   devDependency.
6. **Playwright is not a workspace dependency**: `apps/web/test/e2e/flow.e2e.mjs` loads `playwright-core` and a
   Chrome binary from env paths (as P07a's screenshots did). Add `@playwright/test` + a pinned browser to the lab image
   if E2E should run in `tools/ci.sh full`.
7. **Slot leftovers I could not delete** (the permission prompt denied `rm` under /run): `/run/vrx-test/w1/{admin.pw,
   jwt.key,agent-state/}` (random dev secrets, tmpfs, 0600). Database `vrx_w1` is dropped and the Valkey keys
   `vrx:w1:*` are deleted. Please remove `/run/vrx-test/w1` or allow it.
8. Redacted-only edits are invisible in the diff: changing only a user's `passwordHash` leaves the redacted diff empty,
   so the pending-change bar does not appear although the candidate differs. Should `/config/diff` report
   `{op:'replace', pointer, redacted:true}` for write-only members?
