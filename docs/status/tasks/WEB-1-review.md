# WEB-1 review: ui-kit SchemaForm gaps

Reviewer: independent review agent, 2026-09-24. Branch `task/WEB-1` @ 7d6c547, base `16b622a`. Diff: `git -C /root/ngfw-wt/WEB-1 diff 16b622a...task/WEB-1`.
No host runs (no VPP, no dev server). The only browser used was headless Chrome, to render one static bidi test page from scratch.

**Verdict: APPROVE WITH CHANGES.** The design is right. Every widget is driven only by `x-vrx-ui.widget`/`itemKey`, JSON Schema
`format`/`pattern`/`required`/`default`. The only domain names in `packages/ui-kit/src/schema-form` are in comments. There is no
contract change, and only owned files are touched. `withDefaults` without options behaves as before, and the P07a/P07b `SchemaForm.test.tsx`
is unchanged and green. The P08 suite also passes against this ui-kit. Before merge, fix H1 (a merge-time condition: the merger removes P08 code) and
M1–M3. The rest can be a follow-up.

## What I ran (real output, summarised)

| check | result |
|---|---|
| `pnpm --filter @ngfw/ui-kit test` (worktree) | `Test Files 14 passed (14) · Tests 70 passed (70)`: matches the status file |
| `pnpm --filter @ngfw/web test` (after building the schema, api-client and ui-kit dist files) | `Test Files 11 passed (11) · Tests 75 passed (75)`: matches |
| ui-kit + web `lint`, `typecheck`; `check-logical-css` (covers `packages/ui-kit/src` too) | clean; `check-logical-css: OK (111 files, no physical left/right CSS)` |
| contract guard: `diff --name-only … -- packages/schema packages/proto packages/api-client apps/agent` | 0 files; every changed file is in the envelope's owned list; no TODO/mock/stub |
| **merge simulation**: `git archive main` (0ae3559, includes P08 + W-seed) + this branch's `packages/ui-kit/src` (`diff -rq` identical) + the 2 demo files, `pnpm install --offline --frozen-lockfile`, dist built | web `tsc` OK; `vitest run src/domains src/App.test.tsx`: `InterfacesPage 7/7, App 12/12, DomainTabsPage 4/4, model 5/5 · Tests 28 passed (28)` |
| real root schema scan (built `@ngfw/schema` + ui-kit dist) | 52 optional objects get a presence switch (matches the claim). All 44 `pattern`s are `^…$`-anchored. No property is named `title/help/enum/variant/group/placeholder/itemTitle/keyTitle` (the I18N-1 suffixes cannot collide today) |
| probes (a temporary test file in the worktree that I deleted afterwards (`git status` clean), and one in the scratch merge copy) | results in H1, M1, M3, L1, L2; the switch is `input[type=checkbox][role=switch]`, `tabIndex 0`, labelled "Configure DHCP client" (OK) |

## Findings (ranked)

### H1 — After merging with main, turning on "Configure DHCP client" with only defaults saves nothing (P08 `dropPhantomOptionals`)
- Where: main `apps/web/src/domains/interfaces/model.ts:113-123` (`dropPhantomOptionals`) and `InterfaceDrawer.tsx:120`, used together with
  this branch's `fields/SchemaField.tsx:135-141` (the presence toggle) and `schema-utils.ts` `FORM_DEFAULTS`.
- Scenario (reproduced in the merge simulation, driving the real drawer with the `installFakeApi` stand-in): an interface without `dhcpClient` →
  switch "Configure DHCP client" on → Save. The PATCH body is `{"host-w1w0":{}}`. The form submits `dhcpClient: {setBroadcastFlag:false}`,
  which is exactly `withDefaults(ps, undefined, schema)`, so P08 deletes it as a "phantom". The most common case, "just use DHCP",
  is lost without any message. The status file's Q2 ("harmless until then", `WEB-1.md:144`) is wrong.
- Fix, in the merge commit (P08's files are not WEB-1's to edit, so the merger does it): delete `dropPhantomOptionals`, its call in the drawer
  and its unit test. The presence switch now makes the "phantom" impossible. Add a drawer test: switch on + Save →
  `{ 'host-w1w0': { dhcpClient: { setBroadcastFlag: false } } }`. The existing "exactly the MTU" test stays green (verified). Correct Q2 in `WEB-1.md`.

### M1 — datetime: the UTC-offset input cannot be edited key by key
- Where: `fields/widgets.tsx:168` (`unparsable`), `:189` (`{!unparsable && …}`), `:197`.
- Scenario (probe): stored `2026-09-24T18:00:00+03:30`. The user deletes one character of the offset (`+03:3`), types `+03:`, or clears it.
  `onChange` sends the partial string, `parseDateTime` fails, and the component switches to a raw text input and **unmounts the offset
  input**. Focus is lost, and the only way back is to type a full RFC 3339 string. The worker's test only edits the date part.
- Fix: keep the draft offset in local state. Call `onChange(joinDateTime(local, offset))` only when the offset matches
  `^(Z|[+-]\d{2}:\d{2})$`, and otherwise mark the offset input invalid. Or decide `unparsable` only for values the widget did not produce,
  so the structured mode never switches while the user types in it. Add a test that types `+04:` and then `+04:30` and submits `…+04:30`.

### M2 — `isolate()` puts Persian help that begins with a Latin word in the wrong order (RTL-1 regression for P08's fa texts)
- Where: `fields/bidi.tsx:7-9` (`<bdi>` without `dir` is `dir=auto`, which takes the direction from the first strong character). The same pattern is at `composites.tsx:485, 577`.
- Scenario: P08's `localizeSchema` puts translated help into `x-vrx-ui.help`, for example fa `interfaces:field.mtu.help` = "MTU لایه‌ی ۳ به بایت …" and
  `rxMode.help` = "polling (پیش‌فرض DPDK)، interrupt یا adaptive". I rendered both in headless Chrome (fa page, `dir=rtl`). Without
  `<bdi>`, main shows them correctly. With `<bdi>`, the line is laid out LTR: "MTU" becomes the *last* word read, and the rxMode
  sentence reads "adaptive یا interrupt …". Many network help texts start with an acronym (MTU, VLAN, BGP, IPv4).
- Fix: pick the direction from the content. Text that contains any RTL-script character (`/[֐-ࣿיִ-﷿ﹰ-﻿]/`) is rendered as is
  (it follows the page). Other text is `<bdi dir="ltr">`, which still fixes the "443 or 8000-8080" case. Use the same helper for the table cells and row summaries,
  and add a fa test with a help text that begins with "MTU".

### M3 — Rows opened because of errors or an empty key close while the user is editing them, and focus falls to `<body>`
- Where: `composites.tsx:463` (`open = !keyed || summary === '' || hasError || isOpen`) and `:570` (`open = hasError || isOpen`).
- Scenarios (probes, all reproduced):
  1. rule-editor: an existing rule with `sequence: 0`. Save opens the row, and typing a valid value closes it (`rowFormStillOpen: false, activeElement: BODY`).
  2. itemKey list: a stored row with an empty key starts open. The first typed character creates a summary and the row closes.
  3. A server-error pointer into a collapsed row opens it. After the user fixes the field and leaves it, the row closes.
- Fix: once a row is shown open for any reason (errors, empty summary, new), add its id to `expanded` in `useRowExpansion` (for example with
  an effect over the ids that are open now, or `onFocusCapture` on the row), so that only the user closes it. Add tests for scenarios 1 and 2.

### L1 — time: an invalid stored value is hidden
`widgets.tsx:103`: `<input type="time">` clears a value that is not a valid time (probe: stored `8:30` is shown as `''`). The user then sees an empty field
with a pattern error. Fix: if the value is not empty and does not match `^\d{2}:\d{2}(:\d{2})?$`, render `type="text"`, as `DateTimeInput` already does.

### L2 — ranges: pasting a whole range into "From" breaks it
`widgets.tsx:75`: pasting `8000-8080` gives `80008080` (the probe confirms this), and an IP range gives `10.0.0.1010.0.0.20`. Fix: if the text typed in From contains `-`, split it with `splitRange` and clean each part.

### L3 — datetime defaults and precision
- `widgets.tsx:163`: a new value gets today's offset, not the offset of the chosen date. In a zone with daylight-saving time, a December window picked in September stores `+02:00` for
  Europe/Berlin. Fix: until the user edits the offset, use `localOffset(new Date(local))`.
- `widgets.tsx:123/136`: the fractional part (`m[3]`) is parsed but thrown away, so `…:30.25Z` loses `.25` on any edit. `z.iso.datetime` accepts
  fractions. Fix: carry the fraction through `parseDateTime`/`joinDateTime`.

### L4 — I18N-1 group keys do not match P08's layout
`text.ts:63` looks up `<prefix>.group.<name>` (`interfaces:field.group.*`), but P08's `interfaces.json` has `group.*` at the namespace root. The `title/help/enum`
layout matches `users.json` and `interfaces.json` (`field.<name>.*`). Fix: add `<ns>:group.<name>` (the namespace from the prefix) as the last fallback, or record
the move in docs/05 when that file is owned.

### L5 — Rule 5 drift in ui-kit `UiHints`
`types.ts:76-78` extends the hand-written copy of `packages/schema`'s `UiHints`. `itemKey` duplicates the schema field, and `enumLabels` exists only in the UI.
The copy predates this branch, but it is growing, and the widget names (`IDENTIFIER_WIDGETS` plus the `switch` in `inputs.tsx`) are a second hand-kept list.
Fix, either:
- use `import type { UiHints } from '@ngfw/schema'` (type-only, no runtime dependency) and extend it; or
- add an apps/web test that walks `rootSchema` and asserts that every `x-vrx-ui.widget` is known to ui-kit (export a `KNOWN_WIDGETS` set).

### L6 — Accessibility details
- `composites.tsx:473/588`: the toggles change their accessible name ("Open row 1" ↔ "Close row 1") *and* set `aria-expanded`. Use a stable
  name that includes the row summary, keep `aria-expanded`, and add `aria-controls`.
- `:301-312`: "Move up/Move down/Remove" in table rows do not say which row. Use `{index}`/summary-aware keys.
- `widgets.tsx:65`: the two range inputs have no `required`/`aria-required` (only the legend shows the asterisk).
- `:556`: the `#` header is a bare literal.
- `:63`: the sticky actions cell keeps the `background.paper` colour on a selected row (see screenshot 06).
- The presence switch is correct: `role=switch`, reachable with Tab, has a label, and is disabled when read-only.

### L7 — Every keystroke recomputes all rows
`composites.tsx:436/529`: the whole array is watched, so every keystroke recomputes `fromFormValue` and the summaries for every row and column. This is fine at demo sizes.
Note it for large ACLs (week 4, no performance work now).

### Nits
- `schema-utils.ts:321`: `isAsciiOnlyPattern` assumes the pattern is anchored (JSON Schema `pattern` searches). Every current pattern is anchored. To stay safe, require `^…$` and no top-level `|`.
- fa `expandRow`: 'بازکردن' should be 'باز کردن'.
- Turning presence off throws away what the user typed, and turning it back on gives the defaults. Keeping the last value until Save would be kinder.
- The timezone and interface suggestion lists are not LTR-isolated (the input is).

## Scope
The additions outside the literal scope are generic, small and justified. I accept them:
- sibling-title disambiguation (`propertyTitles`)
- long identifier inputs are no longer textareas
- server errors inside an absent container are listed
- `tag-picker` chips

`propertyTitles` changes the default label wherever sibling titles collide. None of the P08/P07 tests are affected.

## Checklist (REVIEW-PROMPT)
| item | result |
|---|---|
| 1 contract | none |
| 2–5, 7 | not applicable (UI-only, no VPP objects) |
| 6 security | no shell/exec; secrets are left out of summaries and table columns; the fixture uses `VRX_TEST_PSK_web1` |
| 8 UI honesty | the demo route only; screenshots present |
| 9 scope | see above |
| 10 i18n | 13 new keys, en/fa identical; logical CSS only |
| 11 tests | re-run and matching the status file; I did not re-run the CI gate (`tools/ci.sh`): the task excluded host runs |
