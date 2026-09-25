# dev-weak-passwords — VRX_DEV_WEAK_PASSWORDS (development only)

Product-owner request (2026-09-24): set the tools/app admin password to `admin`. The product requires ≥ 12 characters;
the owner chose a development switch in the product over a one-off bypass of this instance.

## What
- `VRX_DEV_WEAK_PASSWORDS` (apps/api/src/config.ts): `0|1|true|false`, **default off**; **refused at boot with
  `NODE_ENV=production`** (loadEnv error `VRX_DEV_WEAK_PASSWORDS: development only: refused with NODE_ENV=production`);
  a warning is logged at boot while it is on.
- Both password routes (`POST /api/v1/auth/password`, `POST /api/v1/users/{name}/password`) parse their body with
  `EnvZodPipe`: flag off → exactly main's schema (`min(PASSWORD_MIN = 12)`, every issue reported together as before);
  flag on → any non-empty password. Strictness, `current`, 1024 max, rate limits, lockout, TD-4 step-up and argon2 unchanged.
- `UsersService.setPassword`: a 2-line defence-in-depth `assertPasswordPolicy` at the top (same rule, same flag).
- The OpenAPI keeps documenting the product rule (`minLength: 12`): `openapi.json` regenerated with zero diff.
- `tools/app`: `VRX_APP_WEAK_PASSWORDS=1` passes the flag (validated `0|1|true|false` before anything runs);
  `status` warns, reading the flag from the running API process (`/proc/<pid>/environ`).
- docs/tech-debt.md: P10 row — the vrx-api unit sets `NODE_ENV=production`, nothing packaged sets the flag.

## Out of scope
The web "Change password" dialog still requires 12 characters (apps/web/src/shell/UserMenu.tsx, unchanged): a short
password is set through the API. Bootstrap password minimum (`VRX_BOOTSTRAP_ADMIN_PASSWORD` ≥ 8) unchanged.

## How verified
```
$ pnpm --filter @ngfw/api openapi && git status --short packages/     # → (empty)
$ apps/api: pnpm lint && pnpm typecheck                               # → clean
$ apps/api: pnpm test
 Test Files  12 passed (12)
      Tests  105 passed (105)
$ apps/api: pnpm test:e2e   # slot 11, host PostgreSQL + Valkey + in-process fake agent, 2026-09-25 03:54–03:58
 ✓ test/e2e/td2.e2e.test.ts (14 tests) 43562ms
 ✓ test/e2e/td2-verify.e2e.test.ts (8 tests) 25165ms
 ✓ test/e2e/config.e2e.test.ts (15 tests) 10714ms
 ✓ test/e2e/td2-review.e2e.test.ts (11 tests) 30321ms
 ✓ test/e2e/auth.e2e.test.ts (10 tests) 7502ms
 ✓ test/e2e/interfaces.e2e.test.ts (2 tests) 4319ms
 ✓ test/e2e/stream.e2e.test.ts (3 tests) 11536ms
 ✓ test/e2e/dev-weak-passwords.e2e.test.ts (2 tests) 2800ms
 Test Files  8 passed (8)
      Tests  65 passed (65)
```
Mutation check (each applied, the named tests run, reverted): users pipe always weak → FAIL; auth pipe always weak → FAIL;
users pipe never weak → FAIL; production refusal removed → FAIL; `'false'` turns it on → FAIL; boot warning removed → FAIL;
auth route publishes the weak schema → FAIL (route-guard contract check); service guard disabled → FAIL.

Review: adversarial, 5 lenses (security, correctness, contract, tests, ops) + an independent refuter per claim — 10
confirmed (low/medium), all fixed in 2546ad81 / 10d4cd06; 8 refuted (web dialog unchanged on this branch, pre-existing).
Slot 11 torn down: vrx_w11 dropped, Valkey db 11 = 0 keys, /run/vrx-test/w11 absent, ports 4100/6100/9211 closed.
