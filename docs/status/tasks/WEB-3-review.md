# WEB-3 review — committed browser harness for E2E and screenshots

Branch `task/WEB-3` @ `cb47f86a`, base `main@f13b744`. Reviewed in `/root/ngfw-wt/WEB-3`.

## Verdict: APPROVE WITH CHANGES

The harness is well-built, in-scope, adds no dependency, preserves `flow.e2e.mjs`'s 43 checks byte-for-byte,
correctly fails on `pageerror`, and its `nav()` fix for MUI `Collapse` vs. the accessibility tree is real and
well-reasoned. CI gate, unit tests and the worker's pasted real-stack proof all pass. Two gaps (README teardown
instructions, untested theme/lang runtime toggle) should be fixed — a small, fast, non-architectural change —
before or immediately after merge; nothing here blocks merging the harness itself.

## 1. Scope — PASS

`git -C /root/ngfw-wt/WEB-3 diff main...HEAD --name-only` touches exactly 15 files, all within the owned set:
`apps/web/test/e2e/{lib/**,shots.mjs,README.md,screens/{_example,interfaces,users}.mjs,flow.e2e.mjs}` and
`docs/status/tasks/WEB-3.md`, `WEB-3.envelope.md`. No product code (`apps/web/src`, `apps/api`, `apps/agent`)
touched. `package.json` and `pnpm-lock.yaml` are byte-identical to `main` (empty diff) — no new dependency.
`screens/interfaces.mjs` / `screens/users.mjs` are the two envelope-required proof screens and fall under the
review brief's `screens/**` scope.

## 2. flow.e2e.mjs — 43 checks preserved — PASS

Extracted every `check(`/`ok(` call site from `main`'s `flow.e2e.mjs` (38 sites, excluding the `ok`/`check`
function definitions themselves) and from `HEAD`'s version: identical, in the same order, with byte-identical
message templates. The only check that moved is `nav()`'s "opened the collapsed group" message, now built in
`lib/nav.mjs:56` from `header.innerText()` instead of the old static `tr(lang, 'nav:groups.<group>')` lookup —
equivalent at runtime, confirmed by the pasted proof run showing `"System"`/`"سیستم"` unchanged. The pasted
real-stack run in `docs/status/tasks/WEB-3.md` contains exactly 43 `ok   [...]` lines (grep-counted), matching
`E2E PASSED (43 checks)`.

## 3. Harness correctness

- **pageErrors fail the run — PASS.** `lib/browser.mjs:34-46` collects every `pageerror` and exposes
  `assertNoPageErrors()`, which throws. Called in `shots.mjs:81` after each screen/lang pass and in every
  `flow.e2e.mjs` pass (`adminPass`/`revertPass`/`keyboardPass`/`secretPass`/`blackholePass`, each right before
  `ctx.close()`) — this is new behavior vs. `main`, where a `pageerror` was only logged.
- **nav() opens collapsed groups without flaky sleeps — PASS.** `lib/nav.mjs` has zero `waitForTimeout`/sleep
  calls (grep-verified across the whole `lib/`); it uses `waitFor({state:'attached'})` + `aria-expanded` +
  role-based `waitForCurrentPage`. The documented root cause (MUI `Collapse` removes a collapsed group's links
  from the accessibility tree entirely, so `getByRole` finds zero matches — not "present but hidden") is a
  real, verifiable Playwright/MUI interaction, and the fix (plain CSS-locator probe, then role-based click) is
  sound and well-commented against being "simplified" back.
- **Screenshot naming `<slug>-<lang>-<theme>-<step>.png` — PASS**, implemented in `lib/shot.mjs:5-7,13-21` and
  used correctly by `shots.mjs:75`. **[L]** `lib/shot.mjs:5` exports a `shotName()` helper for exactly this
  convention, but neither `shots.mjs` nor `flow.e2e.mjs` calls it — both re-build the same template string
  inline (currently identical, but now two sources of truth for the one naming convention the whole task
  exists to standardize).
- **en/fa and light/dark toggles — PARTIAL.**
  - en/fa: proven for real (`docs/status/tasks/WEB-3.md`'s pasted run: `interfaces-en-*`/`interfaces-fa-*`,
    logged as `ltr/en` and `rtl/fa`). **[L]** But no `check()`/`ok()` anywhere ever asserts
    `document.documentElement.dir` — `lib/shot.mjs:17-19` only *logs* it. A silent regression (`dir` staying
    `ltr` under `fa`) would not fail any run; it would only be caught by a human reading a log line or a
    screenshot.
  - light/dark (`lib/theme.mjs`'s `setTheme`/`setLanguage`, the *runtime*, no-reload toggle that is this
    module's actual reason to exist — see its own doc comment on cost savings for expensive screen setups):
    **[M]** never exercised against a live browser. `docs/status/tasks/WEB-3.md`'s own "Out of scope" section
    admits dark theme was not run against the live stack; the en/fa comparison that *was* proven used two
    separate per-language browser contexts (`newPage({lang})`'s `localStorage` seed before first load), not
    `setLanguage()`. There is also no unit test (Playwright is deliberately not a workspace dependency, so
    `vitest` cannot cover it either). The first future feature worker to pass `--themes light,dark` — which
    `screens/_example.mjs` and the README both document as the normal way to use this harness — will be the
    first real exercise of this code path.
- **Login/credential hygiene — PASS.** `lib/auth.mjs`'s `login()`/`signOut()` only ever `.fill()`/click; grepped
  every `console.log`/`ok(`/`check(` call in `auth.mjs`, `shots.mjs`, `flow.e2e.mjs` — no password value is ever
  interpolated into a message. The admin password is read from a file path (`--admin-password-file` /
  `VRX_E2E_ADMIN_PASSWORD_FILE`), never passed as a literal CLI argument. The generated readonly-user password
  (`RO_PW`, `flow.e2e.mjs`) stays in memory only.
- **Ports — PASS.** `shots.mjs:35` and `flow.e2e.mjs:55` both default `BASE` to `http://127.0.0.1:5100`
  (pre-existing, unchanged), never 3000/8080/9101. `README.md:8` and `:12` explicitly call out never pointing
  at 3000/5173/8080/9101 and that `tools/app` is not this harness's target.

## 4. Teardown / README

- **No pkill advice — PASS.** `README.md`'s "Rules this harness follows" section explicitly bans
  `pkill`/`killall` and matches `docs/lab/shared-host-rules.md`'s "stop by PID, never `pkill -f`" rule.
- **[M] vite-preview child-PID wrinkle is NOT documented in README.md, only in the status doc.**
  `docs/status/tasks/WEB-3.md`'s own "Teardown" section explicitly discovered and named this during the
  real-stack proof: *"`pnpm --filter @ngfw/web preview` forks vite's actual server as a **child** process —
  killing the PID captured from `$!` only stops the pnpm wrapper, leaving the real listener on the port
  orphaned... a worker should still verify the port is actually closed afterward."* That caveat never made it
  into `README.md`. `README.md:48-49` starts the preview exactly the way that produces the wrinkle
  (`( cd apps/web && ... pnpm preview ) & WEB_PID=$!`), and `README.md:68-73`'s teardown block is just
  `kill "$WEB_PID"` — no mention of the child process, no `ss -ltnp` re-check step, nothing. Every future
  worker (~40 expected) who follows the README's own procedure literally will orphan a vite listener on their
  slot's web port after "teardown". On a shared host this is a real, recurring hazard: the next run on that
  slot hits "port already in use" with no clue why, and the natural response for someone who doesn't know the
  rule is to reach for `pkill` — the exact thing this harness is supposed to prevent. This is a one-line-ish
  documentation fix (add the wrinkle + a `ss -ltnp`/kill-the-child step to the teardown snippet), not a design
  problem, so it does not block merging the harness, but it should be fixed promptly given how many workers will
  copy this exact procedure.

## 5. Usability for ~40 future screens — PASS, one nit

`screens/_example.mjs` is clear, fully JSDoc'd on every `ctx` field, and explicitly warns "never a fixed sleep
alone — wait for something that means the screen actually rendered." The two real proof screens
(`interfaces.mjs`, `users.mjs`) follow the pattern. CLI flags in `README.md` (`--base/--screens/--out/--langs/
--themes/--admin-user/--admin-password-file`) match `shots.mjs`'s `opt()` parsing exactly.
**[L nit]** `screens/interfaces.mjs:9` and `screens/users.mjs:7` each use a bare `page.waitForTimeout(800|500)`
("live counters ... settle") layered on top of a real `waitFor()`, which technically contradicts
`_example.mjs`'s own "never a fixed sleep alone" guidance (it's not the *sole* wait here, but it is an
unconditioned one). Low risk of being copied as an anti-pattern since it's commented and narrow, but worth a
second look if future screens start growing their own ad hoc timeouts.

## 6. CI / tests

- `TMPDIR=/tmp/g-rw3 tools/ci.sh --base main` (quick, `--base main`): **CI GATE PASSED**, wall time 13m38s.
  Contract guard: no contract files touched (correct — this branch never touches schema/proto/agent-gen/
  api-client-generated). All stages green: tools, install, gen+dirty-gate, forbidden-patterns+gitleaks (no
  leaks), turbo lint/typecheck/test/build (30/30 tasks), `apps/agent`/`apps/cli` make lint/test/build, Go
  modules unit mode, `deploy/vpp` shellcheck + apply-startup fake-host harness (4 shards, 128 checks).
- `pnpm --filter @ngfw/web test`: **14 test files, 98 tests, all passed** (134s), matching the worker's pasted
  unit gate exactly.
- `pnpm --filter @ngfw/web lint`: passes (`eslint src && check-logical-css`, 119 files, no physical left/right
  CSS). **[L, informational]** This script only lints `src/` — `apps/web/test/e2e/**` (old `flow.e2e.mjs`
  included) has never been covered by it or by `tools/ci.sh`'s forbidden-pattern grep
  (`CONTROL_PLANE_PATHS=(apps/api/src apps/web/src 'packages/*/src')`, `tools/ci.sh:53`). Pre-existing gap, not
  introduced by this task, but worth a manager note now that the e2e tree is about to grow to ~40 files.

## Other notes

- `docs/decisions/LOG.md` D-117/D-123/D-128 references in code comments and README are accurate (checked
  against the log directly).
- The optional real-stack rerun on slot 11 was not performed in this review: the harness's own sandbox denied
  starting the additional background stack (build + devstack + browser run) as "Interfere With Workloads",
  most likely because the host was already running several other sessions' concurrent CI jobs at review time.
  No slot-11 resources were touched by the attempt (verified: ports 4100/6100 empty, `/run/vrx-test/w11`
  absent, no stray processes). The review instead relies on: full static/diff review, `tools/ci.sh --base main`
  (PASSED), `pnpm --filter @ngfw/web test` (98/98) and lint (PASSED) run directly in the worktree, and the
  worker's own pasted real-stack evidence in `docs/status/tasks/WEB-3.md`, which is internally consistent
  (check counts, messages, and the post-teardown host-state claims all match what this review independently
  observed on the host afterward).

## Findings summary

| # | Severity | File:line | Finding |
|---|----------|-----------|---------|
| 1 | M | `apps/web/test/e2e/README.md:48-49,68-73` | Teardown snippet doesn't document the `pnpm preview` child-PID wrinkle; following it literally orphans a vite listener on the slot's web port |
| 2 | M | `apps/web/test/e2e/lib/theme.mjs:15-30` | `setTheme`/`setLanguage` (dark theme, runtime lang toggle) never exercised against a live browser or any test |
| 3 | L | `apps/web/test/e2e/lib/shot.mjs:17-19` | RTL `dir` switch is only logged, never asserted with `check()`/`ok()` |
| 4 | L | `apps/web/test/e2e/lib/shot.mjs:5-7` | `shotName()` helper exported but unused; naming convention duplicated inline in `shots.mjs`/`flow.e2e.mjs` |
| 5 | L | `screens/interfaces.mjs:9`, `screens/users.mjs:7` | Bare `waitForTimeout` layered on real waits; narrow contradiction of `_example.mjs`'s own "no fixed sleep alone" guidance |
| 6 | L (informational) | `apps/web/package.json:11` | `pnpm --filter @ngfw/web lint` and the CI forbidden-pattern grep don't cover `test/e2e/**`; pre-existing, not introduced here |
