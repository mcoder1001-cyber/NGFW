# WEB-3 fix round 1 — focused verify

Verifying `task/WEB-3 @ 9ccd8632` against `docs/status/tasks/WEB-3-review.md`'s own findings only (M1, M2, the
`shot.mjs`/`shotName()` L, and the "nothing else regressed" checks). Scope: `git -C /root/ngfw-wt/WEB-3 diff
0ea17400..9ccd8632`.

## Verdict: APPROVE

All items from the fix-round request are addressed correctly; nothing regressed.

## Scope of the fix (nothing regressed)

`git diff 0ea17400..9ccd8632 --stat`: 5 files — `README.md`, `lib/shot.mjs`, `screens/_example.mjs`, `shots.mjs`,
`docs/status/tasks/WEB-3.md`. All owned. `package.json`/`pnpm-lock.yaml`/`apps/web/package.json`: empty diff — no
new dependency. `flow.e2e.mjs`: empty diff — confirmed untouched, as claimed.

- `pnpm --filter @ngfw/web test`: **14 test files, 98 tests, all passed** (144.9s) — same as before the fix round,
  no regression.
- `pnpm --filter @ngfw/web lint`: passed (`eslint src` + `check-logical-css`, 119 files clean).

## M1 — README teardown finds the real listener, no pkill — CONFIRMED FIXED

`README.md`'s "Running against your slot stack" now:
- Warns at the point `pnpm preview` is started (right before the `( cd apps/web && ... ) & WEB_PID=$!` line) that
  the real vite listener is a **child** of `$WEB_PID`.
- Teardown block now does:
  ```bash
  WEB_LISTEN_PID=$(ss -ltnp "sport = :$VRX_WEB_PORT" | grep -oP 'pid=\K[0-9]+' | head -1)
  [ -n "$WEB_LISTEN_PID" ] && kill "$WEB_LISTEN_PID"
  kill "$WEB_PID" 2>/dev/null   # the pnpm wrapper, if it's still around
  ```
  finds the actual PID bound to the slot's web port and kills that specific PID — no `pkill`/`killall` anywhere,
  matching `docs/lab/shared-host-rules.md`.
- Adds a verify step right after teardown: `ss -ltn "sport = :$VRX_WEB_PORT" | grep -q LISTEN && echo WARNING ...
  || echo port closed`.
- "Rules this harness follows" updated to match (no longer says just `kill $WEB_PID`).

`docs/status/tasks/WEB-3.md`'s pasted fix-round teardown shows this working for real: `ss -ltnp` found the
listener at pid `3412046`, explicitly noted as different from the pnpm wrapper's PID captured at start, killed by
that PID, then `ss -ltnp` on 4100/6100/9211 confirmed empty. This is exactly the scenario M1 asked to see fixed.

## M2 — screens/_example.mjs asserts colorScheme and `<html dir>` — CONFIRMED FIXED, 2 real bugs plausible

`screens/_example.mjs` now, after its normal nav+shot steps:
- Reads `document.documentElement.style.colorScheme` before/after `setTheme(target)` and `check()`s it changed to
  the target and differs from before. `target` is picked as "whichever mode is not already active" rather than a
  hardcoded `'dark'` — correctly avoiding a no-op on the `--themes light,dark` pass that starts in `dark` already.
  Confirmed `document.documentElement.style.colorScheme` really is set by `VrxThemeProvider`
  (`packages/ui-kit/src/theme/VrxThemeProvider.tsx:31`, untouched, pre-existing product code) — a legitimate
  ground-truth signal, not an invented one.
- Reads `document.documentElement.dir` before/after `setLanguage(otherLang)` and `check()`s it flipped, then
  restores the original language.

**The two claimed bugs are real and the fixes are in the diff, not just asserted in prose:**
1. *Hardcoded `'dark'` no-op*: avoided by picking `target` from the pre-toggle `colorScheme` read
   (`screens/_example.mjs`) — visible directly in the diff, not something that could be faked by prose alone.
2. *Stale `lang` in the restore `setLanguage` call*: `shots.mjs` now threads a `currentLang` variable (starts at
   `lang`, updated inside `ctx.setLanguage` after each call) through both `ctx.setTheme`/`ctx.setLanguage`
   instead of the pass's fixed `lang` — `lib/theme.mjs`'s `setLanguage(page, tr, lang, newLang)` needs the
   *currently-displayed* language to find the Settings popover by its (translated) accessible label, so a second
   toggle back with a stale value would indeed hang looking for a label that's no longer on screen. The fix is a
   small, correctly-scoped change (only `shots.mjs`'s ctx wiring; `lib/theme.mjs` itself is untouched, correctly —
   the bug was in the caller's bookkeeping, not the library function).

Pasted real-stack evidence (slot 11, `_example,interfaces,users` × en/fa × light/dark) independently checks out
arithmetically: `_example` produces 5 checks per (lang×theme) combo + 1 "signed in" per lang = 22; `interfaces`
produces 2 checks per combo + 1 "signed in" per lang = 10; `users` produces 2 checks per combo + 1 "signed in" +
1 "opened the collapsed group" per lang = 12. **22+10+12 = 44**, matching `SHOTS OK (44 checks, ...)` exactly.
Shots: `_example` 2/combo×4 + `interfaces` 1×4 + `users` 1×4 = **16**, matching "16 screenshots captured" exactly.
This internal arithmetic consistency is strong evidence the pasted log is a real run transcript, not hand-edited.

## L — shot.mjs dir assertion opt-in, flow.e2e.mjs untouched, shotName() used — CONFIRMED

- `lib/shot.mjs`'s `shot()` gained optional `lang`/`check` params; the dir assertion only runs `if (lang && check)`.
  `flow.e2e.mjs`'s own `shot()` wrapper still calls `libShot(page, SHOTS, name, { base: BASE })` — no `lang`, no
  `check` — so its 43 fixed checks are untouched, confirmed both by the empty `git diff` on `flow.e2e.mjs` and by
  the pasted regression run (`E2E PASSED (43 checks)`, `grep -c "^ok"` = 43, 0 pageerrors).
- `shotName()` (previously unused/dead code) is now actually called from `shots.mjs`'s `ctx.shot`
  (`shotName({ slug, lang, theme, step })`) instead of the filename being rebuilt inline — one source of truth,
  as the original finding asked for.

## Live evidence — judged, no rerun performed (per instructions)

`docs/status/tasks/WEB-3.md`'s "Fix round 1" section pastes a full real-stack run on slot 11 (same procedure as
the original proof: agent+API owner `w11`, `vite preview` on `127.0.0.1:6100`, `flock -s /run/lock/vrx-lab.lock`,
no `show trace`/`trace add`, slot checked free before starting). Judged as credible: internally consistent check
math (44 = 22+10+12, 16 shots, see above), consistent with the code changes actually present in the diff (every
assertion in the log has a corresponding `check()` call in the diff that could produce that exact message string),
`pageErrors: 0` stated and consistent with `assertNoPageErrors()`'s throw-on-nonzero behavior, and the teardown
transcript's PID/port narrative matches the M1 code change exactly. `flow.e2e.mjs` regression run: 43/43, 0
pageerrors, matching the untouched file. No live rerun performed, per instructions.

## Findings

None outstanding against the fix round. No new issues introduced.
