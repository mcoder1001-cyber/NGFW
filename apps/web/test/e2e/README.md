# Browser E2E / screenshot harness (WEB-3)

One committed library for getting a real browser against a real stack, so a feature worker's UI evidence is
reproducible and cheap instead of a hand-rolled `shots.mjs` per feature (P08's script was lost; F-vlan-qinq and
F-nat44-ed-sessions each wrote a private one — review F6).

**`tools/app` is NOT this harness's target.** Everything here talks to your own slot's stack only (docs/lab/shared-host-rules.md):
your own `vite` dev/preview port, your own API port, your own agent. Never point `--base`/`VRX_E2E_BASE` at 3000/8080/9101.

## Layout

```
apps/web/test/e2e/
  lib/            the library — import from here, do not duplicate it in a feature's own script
    locales.mjs     tr(lang, "ns:dotted.path", vars) over the app's OWN locale JSON (never a hand-typed string)
    browser.mjs     launchBrowser(), newPage(browser, { lang, mode, ... }) — pageerror collection built in
    auth.mjs        login(page, tr, lang, user, pw), signOut(page, tr, lang)
    nav.mjs         nav(page, tr, lang, key) — opens a collapsed left-nav group first if needed (ui-nav-collapse, D-117)
    theme.mjs       setTheme(page, tr, lang, mode), setLanguage(page, tr, lang, newLang) — runtime toggles, no reload
    shot.mjs        shot(page, outDir, name, { base }) — screenshot + one console log line naming what was captured
    checklist.mjs   createChecklist() → { ok(msg), check(cond, msg), results } — check() throws (fails the run) on false
  shots.mjs       CLI entry point (below)
  screens/
    _example.mjs    the pattern to copy for a new screen
    interfaces.mjs  WEB-3 proof screen (Interfaces grid)
    users.mjs       WEB-3 proof screen (System > Users)
  flow.e2e.mjs    the P07b/ui-nav-collapse commit/rollback/auth flow (43 checks) — migrated onto the same lib
```

## Adding your feature's screenshots

1. Copy `screens/_example.mjs` to `screens/<your-slug>.mjs`. Keep the single default export — `shots.mjs` calls it
   once per `(--langs x --themes)` combination with a ready-made `ctx`: already signed in, `ctx.nav(key)` to reach
   your screen (handles a collapsed group automatically), `ctx.shot(step)` to capture, `ctx.t(key, vars)` for any
   string you need to match against, `ctx.ok`/`ctx.check` to record checks.
2. Run it against your own slot stack (below).
3. Commit `screens/<your-slug>.mjs` under your own feature branch/task — this harness's owner (WEB-3) does not touch
   feature screens; you own yours.

## Running against your slot stack

```bash
eval "$(tools/lab env <slot>)"                       # VRX_SLOT, VRX_TEST_PREFIX, VRX_HTTP_PORT, VRX_WEB_PORT, ...
deploy/dev/pg-test.sh create "$VRX_TEST_PREFIX"       # once per fresh run; idempotent
apps/cli/test/devstack.sh start                       # vrx-agent (owner = your prefix) + vrx-api on VRX_HTTP_PORT
                                                       # prints nothing secret; admin password -> /run/vrx-test/<prefix>/admin.pw
pnpm --filter @ngfw/web build                         # or `pnpm --filter @ngfw/web dev` for a dev server instead of preview
( cd apps/web && VRX_WEB_PORT=$VRX_WEB_PORT VRX_HTTP_PORT=$VRX_HTTP_PORT pnpm preview ) &
WEB_PID=$!
# wait for it, e.g.: until curl -fsS "http://127.0.0.1:$VRX_WEB_PORT" >/dev/null 2>&1; do sleep 0.2; done

export VRX_PLAYWRIGHT_CORE=<path to an installed playwright-core>   # e.g. the npx cache; nothing is installed for you
export VRX_CHROME=<path to a chrome-headless-shell binary>          # + LD_LIBRARY_PATH for its shared libs if needed

node apps/web/test/e2e/shots.mjs \
  --base "http://127.0.0.1:$VRX_WEB_PORT" \
  --screens interfaces,users \
  --out /tmp/shots-<slot> \
  --langs en,fa \
  --admin-password-file "/run/vrx-test/$VRX_TEST_PREFIX/admin.pw"

# the P07b/ui-nav-collapse flow (unrelated to --screens; runs its own login/commit/rollback checks):
VRX_E2E_BASE="http://127.0.0.1:$VRX_WEB_PORT" \
VRX_E2E_ADMIN_PASSWORD_FILE="/run/vrx-test/$VRX_TEST_PREFIX/admin.pw" \
VRX_TEST_PREFIX="$VRX_TEST_PREFIX" \
  node apps/web/test/e2e/flow.e2e.mjs --langs en,fa --shots /tmp/shots-<slot>

# teardown — stop everything you started, by PID, and leave no state behind:
kill "$WEB_PID"
apps/cli/test/devstack.sh stop
deploy/dev/pg-test.sh drop "$VRX_TEST_PREFIX"
valkey-cli -h 127.0.0.1 -n "$VRX_VALKEY_DB" flushdb
rm -rf "/run/vrx-test/$VRX_TEST_PREFIX"
```

Both `shots.mjs` and `flow.e2e.mjs` exit non-zero on any failed check **or any browser `pageerror`** — a run that
took screenshots while the page silently threw is not evidence of anything working.

## Screenshot naming

`shots.mjs` (and `lib/shot.mjs`) name every file `<slug>-<lang>-<theme>-<step>.png` — `theme` defaults to `light`
(`--themes light,dark` to also capture dark; `lib/theme.mjs` toggles it live through the Settings popover, no
reload/re-login needed between themes). `flow.e2e.mjs` keeps its own numbered step names (`01-login-en.png`, ...) —
they are not screen evidence for a feature, just the flow's own record.

## Environment / what is NOT a dependency

Playwright is deliberately **not** a workspace package (00-CONTEXT: `pnpm test` is unit-only and packages may not be
installed on the shared host). Every script here loads `playwright-core` from `VRX_PLAYWRIGHT_CORE` (point it at an
existing install — e.g. the npx cache — never `pnpm add` it) and drives a Chrome-for-Testing `chrome-headless-shell`
binary at `VRX_CHROME` (its shared libraries via `LD_LIBRARY_PATH` if it needs any that are not already on the
host — `apt-get download` + `dpkg-deb -x` into a scratch dir, nothing installed system-wide, as P07a/P07b did). No new
dependency is added by this harness.

## Rules this harness follows (docs/lab/shared-host-rules.md)

- Every object your run creates (test users, revisions, ...) is prefixed with your slot's `VRX_TEST_PREFIX`.
- Never `pkill`/`killall` — stop exactly the PIDs you started (`devstack.sh stop`, `kill $WEB_PID`).
- Never `show trace`/`trace add` (banned on the shared VPP, D-128) — this harness never touches VPP directly; it only
  drives the browser against the API/web stack.
- Tear down everything before you finish: `devstack.sh stop`, drop your database, flush your Valkey db, remove
  `/run/vrx-test/<prefix>`.
