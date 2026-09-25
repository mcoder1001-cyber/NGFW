# WEB-3 — committed browser harness for E2E and screenshots (web-ahead track, D-123)

Branch `task/WEB-3`, worktree `/root/ngfw-wt/WEB-3`, base `main@f13b744`, slot 11 (`w11`, ports 4100/6100).
Files touched: `apps/web/test/e2e/{lib/**,shots.mjs,README.md,screens/**,flow.e2e.mjs}`, `docs/status/tasks/WEB-3*`.

## What was built

A small library (`apps/web/test/e2e/lib/`) over the already-used headless Chrome + `playwright-core` (no new
dependency — the same loading pattern P07a/P07b/F-vlan-qinq/F-nat44-ed-sessions already used, confirmed by
`git -C /root/ngfw grep -n "playwright" -- package.json '*/package.json'` finding nothing — Playwright is
deliberately not a workspace package):

- `lib/locales.mjs` — `createTranslator(webRoot, langs)` → `tr(lang, "ns:dotted.path", vars)` over the app's own
  locale JSON, so a script's selectors always match what the UI renders (never a hand-typed string).
- `lib/browser.mjs` — `launchBrowser(opts)`, `newPage(browser, { lang, mode, persianDigits, dense })`: seeds
  `localStorage['vrx.ui.settings']` before first load and collects every `pageerror` into an array plus
  `assertNoPageErrors()`, which throws (failing the run) if any fired.
- `lib/auth.mjs` — `login()`, `signOut()` through the real form/menu.
- `lib/nav.mjs` — `nav(page, tr, lang, key)`: clicks a left-nav entry, opening its collapsed group first if needed
  (ui-nav-collapse, D-117). No per-screen "which group" map: the target link's own enclosing `<ul id>` names its
  header via `aria-controls`, so it generalizes to every group without a lookup table to keep in sync with
  `apps/web/src/nav/nav.ts`. Also resolves the two label-key shapes `buildNav` actually uses (`nav:<key>` for the
  fixed screens, `nav:domains.<key>` for schema-domain screens) and exports `waitForCurrentPage()` standalone.
- `lib/theme.mjs` — `setTheme()` / `setLanguage()`: runtime en/fa and light/dark/system toggles through the app's
  own Settings popover — no reload, so a script can shoot one screen in every combination inside one signed-in
  session (matters once a screen's setup is expensive).
- `lib/shot.mjs` — `shot(page, outDir, name, { base })`: the `<slug>-<lang>-<theme>-<step>.png` naming convention,
  one settle wait, one console log line (`<html dir/lang>` + relative URL) per capture.
- `lib/checklist.mjs` — `createChecklist()` → `{ ok(msg), check(cond, msg), results }`, `flow.e2e.mjs`'s own
  ok/check pattern, shared so `shots.mjs` reports the same way.

`apps/web/test/e2e/shots.mjs` — the CLI entry point:
```
node apps/web/test/e2e/shots.mjs --base http://127.0.0.1:<port> --screens <slug>[,<slug>...] --out <dir> \
  [--langs en,fa] [--themes light,dark] [--admin-user admin] [--admin-password-file <file>]
```
Logs in once per language, calls each `screens/<slug>.mjs`'s default export once per (language × theme) with a
ready-made `ctx` (`page`, `lang`, `theme`, `t()`, `nav()`, `shot()`, `ok()`, `check()`). Exits non-zero on any failed
check **or any browser `pageerror`**.

`apps/web/test/e2e/screens/_example.mjs` — the pattern every feature copies (one file per screen, one default
export, JSDoc on every `ctx` field). `screens/interfaces.mjs` and `screens/users.mjs` are the two proof screens.

`apps/web/test/e2e/README.md` — full usage: adding a screen, running against a worker's own slot stack
(`tools/lab env`, `pg-test.sh`, `apps/cli/test/devstack.sh`, `vite preview`), the naming convention, where
Playwright/Chrome come from, and the shared-host rules this harness follows (own prefix only, no `pkill`, no `show
trace`, full teardown).

`apps/web/test/e2e/flow.e2e.mjs` (the P07b/ui-nav-collapse 43-check flow) migrated onto the library where it removed
real duplication: `loadLocales`/`tr`, `newPage`, `login`/`signOut`, `shot`, and `nav` (the static `NAV_GROUP_OF`
map is gone — the library derives the group from the DOM) are now thin call-throughs to `lib/*`. Net diff:
**+30/−66 lines**. Also picked up the library's pageerror-fails-the-run behaviour (`assertNoPageErrors()` before each
pass's `ctx.close()`) — previously a `pageerror` was only logged, never failed the script. The 43 checks themselves
(their messages, order and count) are byte-for-byte unchanged.

### A real bug the migration exposed and fixed

`nav()`'s first cut used `menu.getByRole('link', { name })` to probe whether the target entry was already reachable.
That is wrong: Playwright's `getByRole` matches against the accessibility tree, and MUI's `Collapse` in its collapsed
state removes its children from that tree entirely (not "present but hidden" — **zero matches**) even though the
DOM still has them. So a role-based probe can never see a collapsed-group link at all, and the "open the group"
branch never ran. Confirmed with a standalone repro (`page.getByRole('link',{name:'Users'})` → 0 matches while
`nav.innerHTML()` plainly showed the `<a href="/system/users">Users</a>`). Fixed by probing with a plain,
non-accessibility-filtered locator (`menu.locator('a').filter({ hasText: /^Users$/ })`) to find the link and its
enclosing collapsed `<ul id>` regardless of visibility, then switching to the role-based locator (now genuinely
accessible) only to click and to wait for `aria-current`. `lib/nav.mjs` documents why in a comment so nobody
"simplifies" it back to `getByRole` for the initial probe.

## How verified

### Unit gate (worktree)
```
$ npx turbo run test --filter=@ngfw/web
 Test Files  14 passed (14)
      Tests  98 passed (98)
   Duration  134.03s
```
(includes the App.test.tsx D-117 collapsed-group tests, unaffected by this task — nothing in `apps/web/src` changed.)

### Real-stack proof — slot 11, `172.30.126.195`
vrx-agent from this tree (owner `w11`, `/run/vrx-test/w11/agent.sock`, host VPP), vrx-api on `127.0.0.1:4100`
(db `vrx_w11`, Valkey db 11), `vite preview` of this tree's build on `127.0.0.1:6100` — the same procedure
`ui-nav-collapse` used. Headless Chrome-for-Testing 154.0.8037.57 (`chrome-headless-shell`, already unpacked in the
session scratch dir, missing shared libs already extracted from `apt-get download` .debs, `LD_LIBRARY_PATH` only —
nothing installed) and `playwright-core` 1.63.0 from the existing npx cache. `flock -s /run/lock/vrx-lab.lock` held
for each run only. No af_packet / host-interface created; no `show trace`/`trace add` used anywhere in this task.

```
$ eval "$(tools/lab env 11)"
$ deploy/dev/pg-test.sh create w11
create role vrx_w11
create database vrx_w11 (owner vrx_w11)
check  vrx_w11 as vrx_w11 · PostgreSQL 18.6 (Ubuntu 18.6-0ubuntu0.26.04.1) on x86_64-pc-linux-gnu
ok     env /run/vrx-test/w11/pg.env (0600) · DSN postgres://vrx_w11:<redacted>@127.0.0.1:5432/vrx_w11
$ make -C apps/agent build
$ flock -s /run/lock/vrx-lab.lock apps/cli/test/devstack.sh start
agent  pid 1706784 owner w11 socket /run/vrx-test/w11/agent.sock
api    pid 1706846 http://127.0.0.1:4100 (admin password in /run/vrx-test/w11/admin.pw)
$ npx turbo run build --filter=@ngfw/web
✓ built in 30.85s
bundle-budget: initial   339.5 kB gzipped of   600.0 kB budget; 12 lazy chunk(s)
bundle-budget: OK
check-no-dev-routes: OK (14 files, no /dev routes, dev nav or demo chunks)
$ pnpm --filter @ngfw/web preview &        # 127.0.0.1:6100, proxies /api to 127.0.0.1:4100
```

#### `shots.mjs` — Interfaces + System › Users, en + fa
```
$ node apps/web/test/e2e/shots.mjs --base http://127.0.0.1:6100 --screens interfaces,users \
    --out <dir> --langs en,fa --admin-password-file /run/vrx-test/w11/admin.pw
ok   [interfaces/en] signed in as admin
ok   [interfaces/en/light] [en/light] interfaces grid is visible
shot interfaces-en-light-1-grid.png  (ltr/en)  /interfaces
ok   [interfaces/fa] signed in as admin
ok   [interfaces/fa/light] [fa/light] interfaces grid is visible
shot interfaces-fa-light-1-grid.png  (rtl/fa)  /interfaces
ok   [users/en] signed in as admin
ok   [users/en] [en] nav: opened the collapsed "System" group to reach users
ok   [users/en/light] [en/light] users heading is visible
shot users-en-light-1-list.png  (ltr/en)  /system/users
ok   [users/fa] signed in as admin
ok   [users/fa] [fa] nav: opened the collapsed "سیستم" group to reach users
ok   [users/fa/light] [fa/light] users heading is visible
shot users-fa-light-1-list.png  (rtl/fa)  /system/users

SHOTS OK (10 checks, 2 screen(s) x 2 lang(s) x 1 theme(s))
EXIT: 0
```
**pageErrors: 0** (a nonzero count would have thrown inside `assertNoPageErrors()` before `SHOTS OK` could print).

Screenshot list (`<slug>-<lang>-<theme>-<step>.png`, 1366×860, light):
- `interfaces-en-light-1-grid.png`, `interfaces-fa-light-1-grid.png` — Interfaces grid.
- `users-en-light-1-list.png`, `users-fa-light-1-list.png` — System › Users list.

#### `flow.e2e.mjs` — 43/43, migrated onto the library
```
$ node apps/web/test/e2e/flow.e2e.mjs --langs en,fa --shots <dir>     # fresh db vrx_w11
ok   [en] protected route redirects to /login?next=%2Fsystem%2Fusers
shot 01-login-en.png  (ltr/en)  /login?next=%2Fsystem%2Fusers
ok   [en] signed in as admin, returned to /system/users
ok   [en] users: added the bootstrap admin to the configuration (its hash is kept by the API)
ok   [en] users: added w11ro in the candidate (PATCH /config/management)
ok   [en] commit dialog: server validation passed (schema → semantic → agent DryRun)
ok   [en] auto-revert checkbox is ON by default
ok   [en] commit without auto-revert → "Committed and applied. Revision 191974931-… […]"
ok   [en] users: edited w11ro in the candidate (PATCH /config/management)
ok   [en] pending-change bar: "2 uncommitted changes", "Locked by: you"
shot 02-pending-bar-users-admin-en.png  (ltr/en)  /system/users
ok   [en] commit dialog: server validation passed (schema → semantic → agent DryRun)
ok   [en] auto-revert checkbox is ON by default
shot 03-commit-dialog-en.png  (ltr/en)  /system/users
ok   [en] countdown banner is ticking ("…2:00…" → "…1:57…")
shot 04-confirm-countdown-en.png  (ltr/en)  /system/users
ok   [en] confirmed: "Commit confirmed The changes are kept and saved as revision 2. […]"
ok   [en] revisions: newest is #2 "e2e en: rename w11ro" by admin
ok   [en] revisions: diff #1 → #2 shows /management/users changes
shot 05-revisions-diff-en.png  (ltr/en)  /system/revisions
ok   [en] rollback dialog: auto-revert ON by default, shows what changes
shot 06-rollback-dialog-en.png  (ltr/en)  /system/revisions
ok   [en] countdown banner is ticking ("…1:59…" → "…1:57…")
ok   [en] confirmed: "Commit confirmed The changes are kept and saved as revision 3. […]"
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
ok   [fa] countdown banner is ticking (…)
shot 14-confirm-countdown-fa.png  (rtl/fa)  /system/users
ok   [fa] confirmed: "ثبت تأیید شد تغییرات حفظ و به‌عنوان نسخه 4 ذخیره شد. […]"
ok   [fa] revisions: newest is #4 "e2e fa: rename w11ro" by admin
ok   [fa] revisions: diff #3 → #4 shows /management/users changes
shot 15-revisions-diff-fa.png  (rtl/fa)  /system/revisions
ok   [fa] rollback dialog: auto-revert ON by default, shows what changes
shot 16-rollback-dialog-fa.png  (rtl/fa)  /system/revisions
ok   [fa] countdown banner is ticking (…)
ok   [fa] confirmed: "ثبت تأیید شد تغییرات حفظ و به‌عنوان نسخه 5 ذخیره شد. […]"
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
EXIT: 0
```
**43/43, byte-identical to the pre-migration set** (same messages/order as `docs/status/tasks/ui-nav-collapse.md`'s
own pasted run). **pageErrors: 0** in every pass (`grep -c pageerror` on the full run log = 0; a nonzero count would
have thrown inside `assertNoPageErrors()`). A second run against the now-non-empty database correctly took the
"baseline already exists" branch and reported 38 checks — expected, pre-existing behaviour of the flow itself, not a
regression (the baseline block is `if (revisions.total === 0)`).

### Teardown
```
$ kill <vite preview pid + its child>          # port 6100 closed
$ apps/cli/test/devstack.sh stop
api stopped (pid 1706846)
agent stopped (pid 1706784)
removed slot secrets and logs from /run/vrx-test/w11 (use stop --keep to keep them)
$ deploy/dev/pg-test.sh drop w11
drop   database vrx_w11
drop   role vrx_w11
ok     nothing named vrx_w11 / vrx_w11 remains
$ valkey-cli -h 127.0.0.1 -n 11 flushdb
OK
```
Verified after teardown: `ss -ltnp` shows nothing on 4100/6100/9211; `/run/vrx-test/w11` no longer exists;
`vrx_w11` database/role gone; Valkey db 11 `dbsize` = 0; no `w11`-owned process left running.

One wrinkle worth recording: `pnpm --filter @ngfw/web preview` forks vite's actual server as a **child** process —
killing the PID captured from `$!` only stops the pnpm wrapper, leaving the real listener on the port orphaned.
Caught by re-checking `ss -ltnp` after the first kill and stopping the child PID too. `README.md`'s teardown snippet
kills the wrapper PID as the simple case; a worker should still verify the port is actually closed afterward.

## Out of scope
- Dark theme was not exercised in the real-stack proof (only `light`, per the proof requirement); `lib/theme.mjs`'s
  `setTheme`/`setLanguage` are implemented and used by `screens/_example.mjs`'s documented pattern, but not run
  against the live stack here — a feature worker exercising `--themes light,dark` on their own screen is the next
  real use.
- Migrating F-vlan-qinq's and F-nat44-ed-sessions' private `shots.mjs` onto this library (envelope: "they migrate
  later", not this task).
- No product code touched; `tools/app` was never used (slot 11 stack only, per the envelope).

## Questions
None.
