# F-security-page — WIP status

Branch: `claude/F-security-page` (worktree `/root/NGFW/.claude/worktrees/wizardly-turing-057676`).
Base: `origin/main` 99a774734.
Checkpoints (local = remote, pushed to `origin/claude/F-security-page`):
- 51e10036a feat(web): build the Security domain page with the auto-block policy tab — pushed.
- feb6ef0da docs(security): point the auto-block guide at the Security page; F-security-page status — pushed.
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
- Not run here: the hosted quick gate (`tools/ci.sh --base main`) in full — the manager runs it on the PR; the Go
  agent/API trees are untouched by this task.

## PR
- https://github.com/mcoder1001-cyber/NGFW/pull/239 (`claude/F-security-page` → `main`), opened 2026-10-17 with the
  verification output above in its description.

## Remaining
- PR review and merge (manager). Live appliance/browser acceptance of the new screen is not claimed here.

## Current failure
- none in this task's scope. `python3 tools/docs/generate-reference.py --check` reports drift in
  `docs/user/README.md` / `source-reference.md`, but it is pre-existing on `origin/main` 99a774734 (missing index
  lines for `ux-structure-stage1.md`, `interfaces/pppoe-delegation.md`, `system/backup-restore.md`, `vpn/ra-vpn.md`
  from other tasks); this task changed no page title and does not own those files — left for a docs follow-up.

## Exact next command
```
gh pr view --repo mcoder1001-cyber/NGFW claude/F-security-page --json url,statusCheckRollup
```
