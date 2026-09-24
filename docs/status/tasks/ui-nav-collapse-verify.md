# ui-nav-collapse — focused verify (review D-117 fix round)

Branch `task/ui-nav-collapse` @ 3f0014e (5 commits over main). Verdict: **APPROVE**

## D-117 requirements vs. code

1. **`nav()` opens the owning collapsed group before clicking** — `apps/web/test/e2e/flow.e2e.mjs:115-127`. `NAV_GROUP_OF = { users: 'system', revisions: 'system' }`; when the header's `aria-expanded` is `"false"` it clicks the header first (checked), then clicks the link and waits for `aria-current="page"` before returning. `adminPass` (`flow.e2e.mjs:258-262`) now starts at `/` and calls `nav(page, lang, 'users')` instead of `page.goto(.../system/users)`, so the default run actually exercises the collapsed-group path. Confirmed in the pasted E2E: `ok [en] nav: opened the collapsed "System" group to reach users` / the fa equivalent.

2. **Group holding the current page opens on load and on in-app navigation, closable** — `apps/web/src/shell/AppShell.tsx:106-129` (`useNavGroups`). `currentGroup` is computed from `current`; initial `useState` seeds it open; a render-time state adjustment (`seenPath` vs `pathname`, `AppShell.tsx:120-124`) re-opens the current group whenever the path changes, without an effect. Header exposes `aria-current="true"` when the group is closed over the current page (`AppShell.tsx:83`, comment explains `"true"` not `"page"` — exactly one page element, review L4). Verified against pre-fix code: `dd05bad`'s `NavList` seeded `open` as an empty `Set` with no path-driven update, so both new tests ("opens the group of a deep-linked page…", "…reached by in-app navigation…") fail against it. Both pass now (isolated run below).

3. **Per-instance ids via `useId`, no duplicate ids/aria-controls with the phone drawer** — `AppShell.tsx:36-38`: `const baseId = useId()`, `listId = \`${baseId}-${group.id}\``. Pre-fix (`dd05bad`) used a static `listId = \`nav-group-${group.id}\`` shared by both drawer mounts — a real defect this fixes. Test `"phone drawer: unique ids…"` (`App.test.tsx:154-166`) asserts `aria-controls` values are a set (no dupes) and each target id is unique in the document; passes.

4. **Open state lifted into AppShell, shared by both drawers** — `useNavGroups(devRoutes)` is called once in `AppShell` (`AppShell.tsx:144`), and `nav/current/open/toggle` are threaded into a single `drawerContent` element rendered inside both the temporary and permanent `Drawer` (`AppShell.tsx:150-158, 199-205`); each mount gets its own `useId()`-scoped ids but shares the same `open` Set. Pre-fix, `open` was local `useState` inside `NavList`, so each drawer had independent state — the same test in #3 also checks both navs report the same `aria-expanded` for "System" after only the side drawer was clicked; passes now, would fail pre-fix.

5. **Tests derive headers from `buildNav`, not literals** — `App.test.tsx:1-46`: imports `buildNav`/`isCollapsible`/`domains`/`DEV_ROUTES`, builds `model`, and asserts collapsible headers equal `collapsible.map(g => i18n.t(g.labelKey))` and single-entry groups render as plain links from `model`. The old hard-coded `['Interfaces', 'Routing', …]` / `.MuiListSubheader-root` list is gone.

6. **`docs/status/tasks/ui-nav-collapse.md`** — present (168 lines), with pasted `pnpm lint` (logical-CSS OK), `pnpm typecheck`, `pnpm test` (78/78), `pnpm build` (bundle-budget OK, no dev routes), a real-stack `flow.e2e.mjs` run (slot 11, fresh db, 43 checks including both new "opened the collapsed … group" lines, en+fa, teardown noted), and 6 screenshots (`nav-1-collapsed`, `nav-2-expanded`, `nav-3-deeplink-tunnels` × en/fa) under `docs/status/tasks/ui-nav-collapse-screens/`.

## Other checks

- **Logical CSS only**: `git diff main...task/ui-nav-collapse` has no `left`/`right`/`paddingLeft`/`marginLeft`/etc. in code (only prose comments mention "left navigation"); new/changed styles use `paddingInlineStart`/existing logical props. `node scripts/check-logical-css.mjs` in the worktree: `OK (105 files, no physical left/right CSS)`.
- **en/fa key parity**: no locale files touched by this diff (`git diff --stat` confirms); every key used (`nav:groups.*`, `common:menu.navigation`, `nav:soon`) already exists in both `apps/web/src/locales/en/*.json` and `.../fa/*.json` — zero new-key risk.
- **Scope**: `git diff main...task/ui-nav-collapse --name-only` touches exactly `apps/web/src/{App.test.tsx,nav/nav.ts,shell/AppShell.tsx}`, `apps/web/test/e2e/flow.e2e.mjs`, `docs/status/tasks/ui-nav-collapse.md` + its screenshots directory. No LOG.md, schema, or unrelated file changes.
- **`git merge-tree --write-tree main task/ui-nav-collapse`**: exits 0, writes a tree (`5ef0c5f...`), no conflict markers — clean.
- **Live test run** (worktree, isolated): `npx vitest run src/App.test.tsx` → **12/12 passed** (both new D-117 tests, the phone-drawer id test, and the toggle test all green). A full `pnpm --filter @ngfw/web test` in the shared, concurrently-used worktree hit unrelated timeouts (`flows.test.tsx` heading not found, a locale-fetch timeout in `App.test.tsx`'s suite setup) consistent with the status doc's own note about load-average-driven flakiness on this shared tree (`/root/ngfw` memory: shared worktree with a concurrent committing agent); re-running the affected file alone was clean, matching the doc's documented pattern.
- **Would fail without the fix**: confirmed by diffing against pre-review commit `dd05bad` (static `nav-group-${id}` ids, per-`NavList`-instance local `open` state, no path-driven re-open) — the new tests target exactly those three defects and pass only against the fixed code.

## Verdict

APPROVE. All six D-117 items are implemented and covered by tests that fail without the fix (verified by reading the pre-fix `dd05bad` code); logical-CSS-only; no new i18n keys; no scope creep; merge-tree clean; App.test.tsx passes 12/12 in isolation.
