# F-security-page — WIP status

Branch: `claude/F-security-page` (worktree `/root/NGFW/.claude/worktrees/wizardly-turing-057676`).
Base: `origin/main` db7b3b536 (rebased 2026-10-18 from 99a774734 on the manager's request; no conflict — main's
`auto-block.md` addition about enforcement interfaces sits in another hunk and is kept).
Pre-rebase reviewed head 80bf2477e is archived as `refs/heads/archive/F-security-page-80bf2477e` on origin.
Checkpoints after the rebase (local = remote, pushed to `origin/claude/F-security-page`):
- b0c6937eb feat(web): build the Security domain page with the auto-block policy tab (was 51e10036a).
- ca91ce0af docs(security): point the auto-block guide at the Security page; F-security-page status (was feb6ef0da).
- 24a96fab5 docs(status): full web test, build and check results (was 3d1c64d03).
- e442d2bd2 docs(status): PR #239 opened (was 80bf2477e) — pushed with --force-with-lease, reported to the manager.
Owned files: see `F-security-page-envelope.md`.

## Completed code
- `apps/web/src/domains/security/SecurityPage.tsx`, `tabs.ts`: `DomainTabsPage` for the `security` domain with the
  tabs `policy` (new `policy/PolicyTab.tsx` + `policy/queries.ts`) and `blocked` (the existing `AutoBlockPage`).
- `policy/PolicyTab.tsx`: `SchemaForm` on the `security.autoBlock` sub-schema (`domainSchemas.security`), value from
  `GET /api/v1/config/candidate/security`, saved with `PATCH /api/v1/config/security` `{ autoBlock: … }` (merge
  patch; a dropped description is sent as `null`), `invalidateConfig` afterwards; API problems through `ProblemAlert`
  with pointers under `/security/autoBlock` mapped onto the fields; read-only roles get a disabled form and a chip;
  a link to the advanced editor at `/config/security/autoBlock`.
- `AutoBlockPage.tsx`: no longer renders its own h2 (it is a tab now); otherwise unchanged.
- `nav/nav.ts`: `'security'` in `BUILT_DOMAINS`; the Auto-block entry is the tab link `/firewall/security?tab=blocked`.
- `router.tsx`: lazy route for `domainPath('security')`; `firewall/auto-block` → `Navigate` to the Blocked sources tab.
- `i18n.ts` + `locales/{en,fa}/security.json` (title, tabs, tab labels, policy texts, per-field titles/help/enum);
  `locales/{en,fa}/auto-block.json` intro/allowlistHint now point at the Auto-block policy tab.
- Tests: `domains/security/SecurityPage.test.tsx` (nav availability, two tabs, load + save merge patch, validation
  problem on the field, read-only role, Persian labels, `/firewall/auto-block` redirect + current nav entry, pointer
  mapping), `domains/security/locale.test.ts` (en/fa key parity, fa translated); `App.test.tsx` "not yet available"
  case replaced by "every root key is built; /firewall/security renders the Security page"; `nav/nav.test.ts` lists
  `security` as available and checks the tab-link resolution; `AutoBlockPage.test.tsx` expects the Security heading
  and the selected Blocked sources tab.
- Docs: `docs/user/security/auto-block.md` points at Firewall › Security › Auto-block policy / Blocked sources (the
  advanced editor stays mentioned as an alternative). No `docs/user/security/README.md`: the generated
  `docs/user/README.md` index excludes README files by name, so none is expected.

## Actual test results
- `pnpm exec vitest run src/domains/security src/nav/nav.test.ts` (apps/web): 5 files, 22 tests passed.
- `pnpm run typecheck` (apps/web): exit 0. `pnpm run lint` (apps/web, eslint + check-logical-css): exit 0.
- Full `vitest run` (apps/web, 2026-10-17 09:08): `Test Files 117 passed (117)`, `Tests 692 passed (692)`, 414 s.
- `pnpm run build` (repo root, turbo `build` for all 14 tasks incl. `@ngfw/web` vite build, bundle-budget OK,
  check-no-dev-routes OK): exit 0, 2m13s.
- `tools/ci.sh check --base origin/main`: `check PASSED` (forbidden patterns, gitleaks, contract guard).
- On the rebased tree e442d2bd2 (2026-10-18): `pnpm --filter @ngfw/web lint` exit 0 (check-logical-css OK, 480
  files); `pnpm --filter @ngfw/web typecheck` exit 0; `pnpm --filter @ngfw/web test` `Test Files 117 passed (117)`,
  `Tests 692 passed (692)`; `tools/ci.sh quick` → `CI GATE PASSED` (turbo 35/35 tasks, agent + cli make lint test
  build, 27 Go test modules in unit mode, deploy/vpp fake-host harness; wall 18m43s, logs
  `/root/ngfw-wt/logs/ci/wizardly-turing-057676-20261018-061121-2342193`).
- Not run here: the hosted gate on the PR (manager) and `tools/ci.sh --base main` (contract guard ran via `check`).

## PR
- https://github.com/mcoder1001-cyber/NGFW/pull/239 (`claude/F-security-page` → `main`), opened 2026-10-17 with the
  verification output above in its description.

## Remaining
- Independent panel (R1 correctness/tests, R2 security, R6 UX/i18n/docs) is reviewing; the manager forwards the
  findings. Any MAJOR is fixed in one more commit, the verdict table is recorded here and in the PR body, then the
  branch is frozen. Merge by the manager after the hosted gate is green on the final head. Live appliance/browser
  acceptance of the new screen is not claimed here.

## Current failure
- none in this task's scope. `python3 tools/docs/generate-reference.py --check` reports drift in
  `docs/user/README.md` / `source-reference.md`, but it is pre-existing on `origin/main` 99a774734 (missing index
  lines for `ux-structure-stage1.md`, `interfaces/pppoe-delegation.md`, `system/backup-restore.md`, `vpn/ra-vpn.md`
  from other tasks); this task changed no page title and does not own those files — left for a docs follow-up.

## Exact next command
```
# after the panel findings arrive: fix, then
cd apps/web && pnpm exec vitest run src/domains/security src/App.test.tsx src/nav/nav.test.ts && cd ../.. && tools/ci.sh check --base origin/main
```
