# ui-nav-collapse — collapsible left navigation (product-owner request; review D-117)

Branch `task/ui-nav-collapse` (dd05bad feature, 27b4fdf review fixes, 06066bf + e2e `nav()` wait: verify round). Web only: `apps/web/src/shell/AppShell.tsx`,
`apps/web/src/nav/nav.ts` (`isCollapsible`), `apps/web/src/App.test.tsx`, `apps/web/test/e2e/flow.e2e.mjs`.

## What
- Groups with more than one entry (Routing, Firewall / NAT, VPN, System, Developer in dev builds) render as a header button
  (`aria-expanded`, `aria-controls`, chevron) over a `Collapse`; all start collapsed, a click toggles.
- Single-entry groups (Dashboard, Interfaces, Services, Tools) stay plain top-level links.
- D-117: the group holding the current page opens on load and on every navigation (state adjusted during render when
  `pathname` changes — no effect); the user can still close it. Its header is bold/primary while it holds the current page.

## Review findings (D-117) → fix
| # | finding | fix |
|---|---|---|
| 1 | e2e `nav()` cannot reach Users/Revisions | `flow.e2e.mjs` `nav()` opens the owning group (`NAV_GROUP_OF`) when `aria-expanded="false"` (recorded as an `ok … nav: opened` line), clicks the link and waits until it is `aria-current="page"`; the readonly half starts at `/` and walks the menu, so every default run takes the collapsed-group path (E2E run below, en + fa) |
| 2 | deep link hides the current entry | `useNavGroups`: current group open on load + navigation; tests "opens the group of a deep-linked page…" (load) and "…reached by in-app navigation, and reopens it after the user closed it" (navigation; a query-only change keeps the user's choice) |
| 3 | duplicate ids with the phone drawer open | `useId()` per `NavList` → `${baseId}-${group.id}`; test "phone drawer: unique ids…" |
| 4 | phone drawer forgets expanded groups | open-set lifted into `AppShell` (`useNavGroups`), passed to both `NavList`s; same test asserts both lists agree |
| 5 | literal group names in App.test | expectations derived from `buildNav(domains, { devRoutes: DEV_ROUTES })` + `isCollapsible` |
| 6 | evidence | this file |

## How verified

### Web gate (worktree, `TMPDIR=/tmp/vt`)
```
$ pnpm lint
check-logical-css: OK (105 files, no physical left/right CSS)
$ pnpm typecheck
$ tsc -p tsconfig.json --noEmit
$ pnpm test   # full run at load average ~1 (after fix commit 27b4fdf)
 ✓ src/auth/session.test.ts (7 tests) 136ms
 ✓ src/locales/locales.test.ts (8 tests) 128ms
 ✓ src/auth/session-resilience.test.ts (13 tests) 215ms
 ✓ src/config/confirm-store.test.ts (3 tests) 20ms
 ✓ src/api.test.ts (2 tests) 82ms
 ✓ src/config/refine.test.ts (3 tests) 12ms
 ✓ src/nav/nav.test.ts (5 tests) 35ms
 ✓ src/schema/registry.test.ts (14 tests) 415ms
 ✓ src/review-fixes.test.tsx (6 tests) 7402ms
 ✓ src/flows.test.tsx (6 tests) 12481ms
 ✓ src/App.test.tsx (11 tests) 36248ms
 Test Files  11 passed (11)
      Tests  78 passed (78)
   Duration  44.62s (transform 4.96s, setup 5.94s, collect 25.05s, tests 57.18s, environment 15.38s, prepare 3.90s)
$ pnpm build
$ tsc -p tsconfig.json --noEmit && vite build && node scripts/bundle-budget.mjs && node scripts/check-no-dev-routes.mjs
✓ built in 1m 48s
bundle-budget: initial   336.1 kB gzipped of   600.0 kB budget; 5 lazy chunk(s)
bundle-budget: OK
check-no-dev-routes: OK (7 files, no /dev routes, dev nav or demo chunks)
```
An earlier full run at load average ~100 (other sessions' workers) had 7 tests time out at 34–50 s against the 30 s limit
(App.test nav tests, flows, review-fixes); the same files re-run alone passed (12/12, 11/11) and the full run above is 78/78.

### Browser E2E against a real stack (slot 11, torn down afterwards)
vrx-agent from this tree (owner `w11`, socket `/run/vrx-test/w11/agent.sock`, host VPP), API from this tree on 127.0.0.1:4100
(db `vrx_w11`, Valkey db 11 prefix `vrx:w11:`), `vite preview` of this tree's build on 127.0.0.1:6100. Headless
Chrome-for-Testing 153.0.8010.12 (`chrome-headless-shell`, npmmirror, libs unpacked into scratch — nothing installed) and
playwright-core 1.63 from the npx cache. Ran under `flock -s /run/lock/vrx-lab.lock` (only for the run). No af_packet /
host-interface created; VPP NRestarts 0 before and after. Final run after the verify round, on a fresh database: 43 checks
(the 41 of the P07b flow + the two new collapsed-group lines).
```
$ node apps/web/test/e2e/flow.e2e.mjs --langs en,fa --shots <scratch>     # fresh db vrx_w11; long "confirmed" lines cut at 230 chars […]
ok   [en] protected route redirects to /login?next=%2Fsystem%2Fusers
shot 01-login-en.png  (ltr/en)  /login?next=%2Fsystem%2Fusers
ok   [en] signed in as admin, returned to /system/users
ok   [en] users: added the bootstrap admin to the configuration (its hash is kept by the API)
ok   [en] users: added w11ro in the candidate (PATCH /config/management)
ok   [en] commit dialog: server validation passed (schema → semantic → agent DryRun)
ok   [en] auto-revert checkbox is ON by default
ok   [en] commit without auto-revert → "Committed and applied. Revision 157048c86-3762-4296-ac0c-c57cba535d44 /management — management is not implemented by this agent build (Health.subsystems) and is not applied /nat — nat is not […]
ok   [en] users: edited w11ro in the candidate (PATCH /config/management)
ok   [en] pending-change bar: "2 uncommitted changes", "Locked by: you"
shot 02-pending-bar-users-admin-en.png  (ltr/en)  /system/users
ok   [en] commit dialog: server validation passed (schema → semantic → agent DryRun)
ok   [en] auto-revert checkbox is ON by default
shot 03-commit-dialog-en.png  (ltr/en)  /system/users
ok   [en] countdown banner is ticking ("Your changes will auto-revert in 2:00 — this is expected if you cut your own access." → "Your changes will auto-revert in 1:58 — this is expected if you cut your own access.")
shot 04-confirm-countdown-en.png  (ltr/en)  /system/users
ok   [en] confirmed: "Commit confirmed The changes are kept and saved as revision 2. What the device applied Committed and applied. Revision 2e6ea6171-7964-45c5-9761-55e7098237d1 /management — management is not implemented by this […]
ok   [en] revisions: newest is #2 "e2e en: rename w11ro" by admin
ok   [en] revisions: diff #1 → #2 shows /management/users changes
shot 05-revisions-diff-en.png  (ltr/en)  /system/revisions
ok   [en] rollback dialog: auto-revert ON by default, shows what changes
shot 06-rollback-dialog-en.png  (ltr/en)  /system/revisions
ok   [en] countdown banner is ticking ("Your changes will auto-revert in 2:00 — this is expected if you cut your own access." → "Your changes will auto-revert in 1:58 — this is expected if you cut your own access.")
ok   [en] confirmed: "Commit confirmed The changes are kept and saved as revision 3. What the device applied Committed and applied. Revision 331760899-3c6b-4812-9941-fdbde6fd3a70 /management — management is not implemented by this […]
ok   [en] rollback confirmed → revision #3 kind=rollback "rollback to revision 1"
shot 07-revisions-after-rollback-en.png  (ltr/en)  /system/revisions
shot 08-users-admin-en.png  (ltr/en)  /system/users
ok   [en] signed in as readonly w11ro (argon2id hash set through the Users form)
ok   [en] nav: opened the collapsed "System" group to reach users
ok   [en] readonly: "Add user" disabled
ok   [en] readonly: edit disabled
shot 09-users-readonly-en.png  (ltr/en)  /system/users
shot 10-revisions-readonly-en.png  (ltr/en)  /system/revisions
ok   [en] readonly: all 3 rollback buttons disabled
ok   [fa] protected route redirects to /login?next=%2Fsystem%2Fusers
shot 11-login-fa.png  (rtl/fa)  /login?next=%2Fsystem%2Fusers
ok   [fa] signed in as admin, returned to /system/users
ok   [fa] users: edited w11ro in the candidate (PATCH /config/management)
ok   [fa] pending-change bar: "2 تغییر ثبت‌نشده", "قفل‌شده توسط: شما"
shot 12-pending-bar-users-admin-fa.png  (rtl/fa)  /system/users
ok   [fa] commit dialog: server validation passed (schema → semantic → agent DryRun)
ok   [fa] auto-revert checkbox is ON by default
shot 13-commit-dialog-fa.png  (rtl/fa)  /system/users
ok   [fa] countdown banner is ticking ("تغییرات شما تا 2:00 دیگر به‌طور خودکار بازگردانده می‌شود — اگر دسترسی خودتان را قطع کرده باشید، این رفتار مورد انتظار است." → "تغییرات شما تا 1:58 دیگر به‌طور خودکار بازگردانده می‌شود — اگر  […]
shot 14-confirm-countdown-fa.png  (rtl/fa)  /system/users
ok   [fa] confirmed: "ثبت تأیید شد تغییرات حفظ و به‌عنوان نسخه 4 ذخیره شد. آنچه دستگاه اعمال کرد ثبت و اعمال شد. نسخه 4a5679f3b-9acc-4e50-8257-c28d0db6ac96 /management — management is not implemented by this agent build (Health.su […]
ok   [fa] revisions: newest is #4 "e2e fa: rename w11ro" by admin
ok   [fa] revisions: diff #3 → #4 shows /management/users changes
shot 15-revisions-diff-fa.png  (rtl/fa)  /system/revisions
ok   [fa] rollback dialog: auto-revert ON by default, shows what changes
shot 16-rollback-dialog-fa.png  (rtl/fa)  /system/revisions
ok   [fa] countdown banner is ticking ("تغییرات شما تا 2:00 دیگر به‌طور خودکار بازگردانده می‌شود — اگر دسترسی خودتان را قطع کرده باشید، این رفتار مورد انتظار است." → "تغییرات شما تا 1:58 دیگر به‌طور خودکار بازگردانده می‌شود — اگر  […]
ok   [fa] confirmed: "ثبت تأیید شد تغییرات حفظ و به‌عنوان نسخه 5 ذخیره شد. آنچه دستگاه اعمال کرد ثبت و اعمال شد. نسخه 5f52a5236-c5e3-438f-88a2-2033bed6b89c /management — management is not implemented by this agent build (Health.su […]
ok   [fa] rollback confirmed → revision #5 kind=rollback "rollback to revision 3"
shot 17-revisions-after-rollback-fa.png  (rtl/fa)  /system/revisions
shot 18-users-admin-fa.png  (rtl/fa)  /system/users
ok   [fa] signed in as readonly w11ro (argon2id hash set through the Users form)
ok   [fa] nav: opened the collapsed "سیستم" group to reach users
ok   [fa] readonly: "Add user" disabled
ok   [fa] readonly: edit disabled
shot 19-users-readonly-fa.png  (rtl/fa)  /system/users
shot 20-revisions-readonly-fa.png  (rtl/fa)  /system/revisions
ok   [fa] readonly: all 5 rollback buttons disabled
E2E PASSED (43 checks)
EXIT:0
```
Teardown: agent/API/web stopped; `pg-test.sh drop w11` → "nothing named vrx_w11 / vrx_w11 remains"; Valkey `vrx:w11:*` keys left: 0;
`/run/vrx-test/w11` removed; ports 4100/6100/9211 closed.

### Screenshots (`ui-nav-collapse-screens/`, 1366×860, light, same stack, signed in as admin)
```
shot nav-1-collapsed-en.png (ltr/en) / headers: Routing=false Firewall / NAT=false VPN=false System=false current: ["Dashboard"]
shot nav-2-expanded-en.png (ltr/en) / headers: Routing=true Firewall / NAT=true VPN=false System=true current: ["Dashboard"]
shot nav-3-deeplink-tunnels-en.png (ltr/en) /vpn/tunnels headers: Routing=false Firewall / NAT=false VPN=true System=false current: ["Tunnels\nsoon"]
[en] pageErrors=0
shot nav-1-collapsed-fa.png (rtl/fa) / headers: مسیریابی=false فایروال / NAT=false VPN=false سیستم=false current: ["داشبورد"]
shot nav-2-expanded-fa.png (rtl/fa) / headers: مسیریابی=true فایروال / NAT=true VPN=false سیستم=true current: ["داشبورد"]
shot nav-3-deeplink-tunnels-fa.png (rtl/fa) /vpn/tunnels headers: مسیریابی=false فایروال / NAT=false VPN=true سیستم=false current: ["تونل‌ها\nبه‌زودی"]
[fa] pageErrors=0
```
- `nav-1-collapsed-{en,fa}.png` — `/`: every group collapsed, single-entry groups as links.
- `nav-2-expanded-{en,fa}.png` — Routing, Firewall / NAT, System opened by click (fa: RTL, chevrons and indentation mirrored).
- `nav-3-deeplink-tunnels-{en,fa}.png` — direct load of `/vpn/tunnels`: VPN opened automatically, Tunnels current.

## Verify round (adversarial: 8 lenses + an independent refute pass per claim)
One skeptic per review finding plus a correctness lens and an a11y/i18n lens; every claimed defect was re-checked by an
independent refuter. Confirmed (all low) and fixed:
- a11y: with a group closed over the current page, the link's `aria-current` is hidden with it → the header now carries
  `aria-current="true"` (not `"page"`: exactly one page element, review L4). The deep-link test asserts it after closing.
- test gap: the navigation half of D-117 (open on pathname change) had no test — a mutant without it passed every nav test →
  new in-app navigation test. Mutation check: removing the navigation half fails it; removing the header `aria-current` fails
  the deep-link test.
- evidence gap: the first pasted E2E never took `nav()`'s click branch (D-117 had System open on every call) → the readonly
  half starts at `/`; `nav()` records the click and waits for `aria-current="page"` (without the wait it returned before the
  router navigated, and the next step timed out once).
Refuted (not defects of this branch): App.test's per-domain link loop has no `defaultValue` (same on main); the "all headers
start collapsed at `/`" loop needs changing only if the dashboard group ever gets a second entry.
After the round: App.test 12/12; web lint + typecheck clean; web build OK (bundle-budget OK, no dev routes).

## Out of scope
Persisting the expanded set across reloads; accordion (one-open-at-a-time) behaviour; nav icons.

## Open questions
None.
