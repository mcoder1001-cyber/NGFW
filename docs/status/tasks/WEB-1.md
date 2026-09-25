# WEB-1 — ui-kit SchemaForm gaps (web-ahead track, D-123) — done 2026-09-24

Branch `task/WEB-1`, worktree `/root/ngfw-wt/WEB-1`, base `main@16b622a`, slot 11 (web dev server on 6100 only for the
screenshots, stopped afterwards). Files touched are only the owned ones: `packages/ui-kit/src/schema-form/**`,
`packages/ui-kit/src/i18n/locales/{en,fa}.ts`, `apps/web/src/pages/dev/{SchemaFormDemoPage.tsx,demoSchema.ts}` and
`docs/status/tasks/WEB-1*`. No dependency added. No contract change: every widget reads existing `packages/schema`
metadata (`x-vrx-ui.widget`, `itemKey`, `group`, `help`, JSON Schema `required`/`default`/`format`/`pattern`). Main has
not touched any of these files since the base, so the D-112 rebase at merge has no overlap here.

## What was built

| scope item | where | notes |
|---|---|---|
| Presence toggle for optional objects (P08-questions Q2) | `schema-utils.ts` (`isOptionalObject`, `DefaultsOptions`, `FORM_DEFAULTS`), `form-value.ts` (`withDefaults(…, options)`), `fields/SchemaField.tsx` (`ObjectSection`), `SchemaForm.tsx` | An **optional object** is a member that is not `required`, is a plain object (not a record, not a union) with at least one property, and has no `default`. It stays **absent** until the user turns on its switch ("Configure DHCP client"). The switch fills it with its defaults, and turning it off removes it again. While it is absent it is neither rendered, validated nor submitted. This covers 52 members of the root schema (`dhcpClient`, `routing.ospf`, `redistribute.*`, `vpn.pki.hsm`, …). `withDefaults(schema, value, root)` **without options behaves as before**. (My earlier claim here, that P08's `dropPhantomOptionals` "still works and simply has nothing to remove", was **wrong**: it also deleted a DHCP client the user had just switched on with only its defaults — review H1, fixed in fix round 1 by removing it.) `<SchemaForm>` passes `{ presence: true }`, and so do every fresh value the form builds (Add row, Add entry, variant switch, switch on). |
| Widgets `port-range`, `ip-range`, `time`, `datetime`, `timezone-picker` (alias `timezone`), `color` | `fields/widgets.tsx`, dispatch in `fields/inputs.tsx` | Each widget edits **the same string** the schema validates, and the schema's `pattern` stays the only rule. **Ranges** have *From*/*To* inputs that compose `443`, `8000-8080` or `10.0.0.10-10.0.0.20`; characters are filtered (digits for ports; hex, `.` and `:` for addresses). **time** is a native `<input type=time>` (`HH:MM`). **datetime** is a `datetime-local` input plus a separate **UTC offset** input, so a stored `…+03:30` keeps its offset exactly; seconds are added because `z.iso.datetime` requires them, and a value that is not RFC 3339 falls back to plain text. **timezone-picker** is free text that suggests IANA zones (the ICU list, with `UTC` first because ICU omits it). **color** is the hex text with a native colour swatch. |
| LTR for identifier widgets inside RTL (RTL-1) | `schema-utils.ts` (`IDENTIFIER_WIDGETS`, `isLtrString`, `isAsciiOnlyPattern`), `fields/inputs.tsx`, `fields/widgets.tsx` | A value renders `dir="ltr"` when one of these holds: its widget is an identifier widget (ip/cidr/mac/interface/vrf/object/host-interface/tag pickers, secret-ref, ranges, time, datetime, timezone, color, record keys); its `format` is an identifier format (ipv4/6, cidr, mac, hostname, email, uri, uuid, date-time …); or its `pattern` can only match ASCII (object names, user names, host names — a conservative scanner). Such values cannot hold Persian text anyway. Free text still follows the page direction, which the existing test for `Name` checks. Identifier chips (`cidr` items, `tag-picker`) render each chip in `<bdi dir="ltr">` so `2001:db8::/64` is never reordered. Table cells and row summaries use `<bdi>`. Pickers that have no data source yet (vrf/object/host-interface/secret-ref) render as LTR monospace text, and an app can still override any widget with `<SchemaForm widgets>`. |
| Per-path title/help/group/enum translation (I18N-1) | `text.ts` (`createSchemaText`), `fields/hooks.ts` (`useSchemaText`, `useEnumHints`), used everywhere a label is shown | New prop `<SchemaForm i18nPrefix="users:field">`. Keys: `<prefix>.<propPath>.title`, `.help`, `.placeholder`, `.enum.<value>`, `.variant.<discriminator>`, `.group.<name>` (and a prefix-wide `<prefix>.group.<name>`), `.itemTitle`, `.keyTitle`. `propPath` is the dotted property path without indexes or record keys. Maps are read with `returnObjects`, so enum values such as `802.1q` work. A missing key falls back to the schema's English, and the old behaviours are kept (`x-vrx-ui.help`/group names that are themselves i18n keys, literal `enumLabels`, the `translateLabel` hook, which now receives the translated fallback). The `users` namespace already follows this layout, so `UsersPage`'s `localizeUserSchema` could be replaced by `i18nPrefix="users:field"` (P07b's file — not changed here). |
| `itemKey` row summaries | `summary.ts` (`summarizeValue`), `fields/composites.tsx` (`ObjectListField`) | Arrays of objects with `x-vrx-ui.itemKey` show each row's key in its heading ("User alice"; "Item 1 192.0.2.254" when the item schema has no title). Rows can be collapsed. Rows that already have a key start collapsed. A new row, a row without a key yet and a row with errors are open (a row with errors cannot be closed). Summaries translate enum and variant labels, show booleans as yes/no (inside an object: the member's title when true), shorten lists to three entries (`+N`), and never show secrets. Lists without `itemKey` keep their old layout. |
| Table view for `rule-editor` lists | `summary.ts` (`tableColumns`), `fields/composites.tsx` (`RuleEditorField`) | `widget: 'rule-editor'` on an array of objects (ACL / MACIP / host-ACL rules) renders a table: `#`, one column per column-worthy member (`itemKey` members first; textarea, JSON, secrets, records and lists of objects stay in the row form), and Actions (edit / move up / move down / remove). The full row form opens beneath its row. New rows open. Rows with errors are marked (error icon, "This row has errors") and forced open after a failed submit. Cells wrap at spaces, padding is tight and the buttons are compact, so the 11-column ACL table fits a 930 px form in en and fa (screenshots 05/06). A narrower screen scrolls the table sideways inside its fieldset (never the page); the actions column then stays visible (sticky), and the open row's form keeps the visible width (`container-type` + `100cqi`), not the table's. |
| Found in the browser run, fixed generically | `schema-utils.ts` (`propertyTitles`), `fields/bidi.tsx`, `fields/inputs.tsx` | (1) **Sibling labels that collide** (a shared sub-schema without its own title: ACL `source`/`destination` were both "Address match", IPsec `localId`/`remoteId` both "IKE identity" — the only 3 cases in the root schema) fall back to the humanized key ("Source", "Destination") in the form and the table; per-path i18n still wins. (2) **Help and error texts are `<bdi>`-isolated**: untranslated English help such as `443 or 8000-8080` was rendered `or 8000-8080 443` in RTL. (3) A **long identifier** (`hostname`, `maxLength` 253) had been inferred as a textarea; it is now a one-line LTR input (long prose still gets a textarea). |
| Server errors inside an absent container | `SchemaForm.tsx` | A problem `pointer` into an optional object that is switched off, or into a missing list item, used to map onto a field that is not rendered, so the error was invisible. It is now listed in the alert with the unmapped pointers. |
| Demo coverage | `apps/web/src/pages/dev/demoSchema.ts`, `SchemaFormDemoPage.tsx` | The demo schema (built from the real `packages/schema` primitives) adds: `DhcpClientSchema` as an optional object, `vrfName` (vrf-picker), a `tag-picker`, `itemKey` on the static neighbours, `l4PortRange`, `ipv4AddressRange`, `timeOfDay`, an offset `z.iso.datetime` (widget datetime), `timezone`, `hexColor`, and `AclRuleSchema` rows as a `rule-editor`. A new choice renders the generated `management.users` item with `i18nPrefix="users:field"`, using the users screen's existing en/fa keys (label from existing keys: "Users (generated)"). |
| en + fa strings | `packages/ui-kit/src/i18n/locales/{en,fa}.ts` | 13 new `form.*` keys with identical key sets and interpolation variables (the `locales.test.ts` check passes). |

### Decisions taken (worker-level, for the manager's log)
- **D-WEB1-1 presence rule:** a member is optional-with-presence exactly when it is not required, is a plain object with members, and has no `default`. A `default` means the server fills it in anyway, so no switch is shown. Unions keep their variant picker; records and arrays stay as they are (absent ≡ empty).
- **D-WEB1-2 RTL-1 detection** is driven by metadata only: widget hint, JSON Schema `format`, or an ASCII-only `pattern`. No per-domain code and no new hint. A new `x-vrx-ui` hint would have been a contract change.
- **D-WEB1-3 i18n key layout** follows the one `users.json` already uses (`field.<name>.title|help|enum.<v>`), extended to nested paths. The namespace is part of the prefix.
- **D-WEB1-4 itemKey rows start collapsed** when they have a key summary. The alternative (always open) makes long lists (users, BGP neighbours) unusable, and the summary is what identifies a row. New rows and rows with errors are always open.

## How it was verified (real output)

### CI gate — `TMPDIR=/tmp/g-web1 tools/ci.sh --base main` (the branch's own ci.sh; no SIGPIPE in the contract guard)
```
branch    task/WEB-1 @ a52e8bc   (base: main)
logs      /root/ngfw-wt/logs/ci/WEB-1-20260924-230024-3896392
== contract guard: HEAD vs main ==
no contract files changed in the 5 commit(s) of HEAD since main (16b622a)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   2m39s
  forbidden patterns (+ gitleaks)                    0m06s
  lint · typecheck · unit tests · build (turbo)   2m17s
  apps/agent: make lint test build                   0m40s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 5m57s · logs /root/ngfw-wt/logs/ci/WEB-1-20260924-230024-3896392

CI GATE PASSED
```
(The same gate passed on 1b6b6d1 at 19:17 and on 4213f30 at 19:52, both in 5m57s–6m10s at load 20–55. a52e8bc is the manager's
salvage commit of this file's draft, with no code change.)

### ui-kit — `npx vitest run` (all files, HEAD 4213f30)
```
 ✓ src/schema-form/form-value.test.ts (11 tests) 119ms
 ✓ src/schema-form/web1.test.ts (14 tests) 87ms
 ✓ src/schema-form/SchemaForm.web1.test.tsx (8 tests) 26363ms
 ✓ src/schema-form/SchemaForm.test.tsx (12 tests) 105162ms
 ✓ src/i18n/locales.test.ts (1 test) 49ms
 … (ws, theme, data-grid, formatters, index: all ✓)
 Test Files  14 passed (14)
      Tests  70 passed (70)
```
All 12 P07a/P07b `SchemaForm.test.tsx` tests are unchanged and green (every widget, parsed submit, translated
validation, RFC 9457 pointers with escaped record keys, dependsOn gating/pruning, fail-closed compile, oneOf switch,
record/list add-remove, Persian RTL with LTR technical inputs, L8 value identity, read-only). New tests:
- `web1.test.ts` (14 unit tests): `isOptionalObject`; `withDefaults` **unchanged without options** (P08's use) and with
  `{presence:true}` (absent stays absent, including inside record values and fresh documents); ASCII-only pattern scanner
  (objectName/username/hostname yes; printable-text prose, `.`, `\S`, `\p{L}`, non-ASCII no); `isLtrString`; long
  identifier ≠ textarea; range/date-time/offset helpers (round-trips); time zones (UTC first); summaries (enum labels,
  variants, `+N`, booleans, records, secrets hidden); table columns (itemKey first, textarea/secret/record/list-of-object
  out); distinct sibling titles; `createSchemaText` with a real i18next instance (prefix keys, fallbacks, dotted enum keys,
  per-object and prefix-wide groups, variants, itemTitle/keyTitle, no-prefix behaviour).
- `SchemaForm.web1.test.tsx` (8 render tests): presence off → submit has no `dhcpClient`; on → `{hostname, setBroadcastFlag:false}`;
  off again → gone; stored value opens switched on · a server pointer into an absent object is listed in the alert ·
  all six widgets edit and submit exact strings (`443-8080`, `10.0.0.10`, `09:15`, `2026-10-01T07:05:00+03:30` keeping the
  stored offset, `Asia/Tehran`, `#ff0000` from the swatch) · time zone suggestions incl. `UTC` · RTL-1 in `fa`
  (vrf-picker, ASCII-pattern, hostname, time, range inputs `dir=ltr`; textarea and free text not; identifier chips in
  `<bdi dir=ltr>`; English help isolated in `<bdi>`) · I18N-1 via `i18nPrefix` in `fa` (group legend, label, help,
  enum options, variant options, table header and cell; untranslated keys fall back) · itemKey rows collapsed, open/close,
  a new row stays open while its key is typed, submit payload · rule-editor table (headers without the textarea, cell
  summaries, edit beneath the row, live cell update, move down, a new invalid row is marked and blocks submit, remove, payload).

### web — `apps/web: npx vitest run` (HEAD 4213f30)
```
 ✓ src/schema/registry.test.ts (14 tests) 761ms
 ✓ src/flows.test.tsx (6 tests) 15129ms
 ✓ src/App.test.tsx (8 tests) 33754ms
   ✓ App frame > code-splits the developer routes and renders the SchemaForm demo  17141ms
 Test Files  11 passed (11)
      Tests  75 passed (75)
```
`apps/web` lint: `eslint src` clean, `check-logical-css: OK (111 files, no physical left/right CSS)`; web typecheck clean.

### P08 compatibility (merge simulation — P08 does not touch `packages/ui-kit`)
`git archive task/P08` (2f2f289) into scratch, `packages/ui-kit/src` replaced by this branch's (`diff -rq` identical), dist built:
```
P08 web typecheck OK
 ✓ src/domains/interfaces/InterfacesPage.test.tsx (7 tests) 59729ms
   ✓ interfaces screen > lists live interfaces with status chips and pending marks; the drawer edits through a merge patch of /interfaces  8691ms
   ✓ interfaces screen > saves only the edits against the value the form opened with; a new sub-interface cannot reuse an id (review N4/N5)  13038ms
   ✓ interfaces screen > a server error for a new sub-interface lands on its field, mapped with the typed id (review N5)  22213ms
   ✓ interfaces screen > a refetch of the candidate keeps what the user typed in the open form (review N4)  10081ms
   ✓ interfaces screen > renders in Persian  5559ms
 Test Files  13 passed (13)
      Tests  88 passed (88)
```
Among them is P08's "exactly the MTU: no phantom dhcpClient from the form's defaults" check. With the presence toggle, the form no
longer produces the phantom that `dropPhantomOptionals` was written to remove. (That check alone missed H1: see fix round 1.)

### Browser (real Chrome) — `/dev/schema-form`, en + fa
Headless Chrome-for-Testing 153.0.8010.12 (unpacked in scratch) + playwright-core 1.63 (npx cache); nothing installed.
`vite` from this worktree on slot-11 port 6100, stopped by PID afterwards (6100 closed). Only the session endpoints are
answered in the browser (`page.route`): the demo page renders synthetic schema data and calls no config API. No page errors,
no console errors. Measured on the page:
```
[en] facts {"vrfDir":"ltr","dnsServerDir":"ltr","descriptionDir":null,"tzValue":"Asia/Tehran","tableHeaders":["#","Sequence","Enabled","Action","IP version","Source","Destination","Service match","Schedule","Log","Actions"],"firstRow":"110Yespermitanyprefix 192.0.2.0/24anyobject httpsNo"}
[fa] dir=rtl
[fa] facts {"vrfDir":"ltr","dnsServerDir":"ltr","descriptionDir":null,"tzValue":"Asia/Tehran","tableHeaders":["#","Sequence","Enabled","Action","IP version","Source","Destination","Service match","Schedule","Log","عملیات"],"firstRow":"110بلهpermitanyprefix 192.0.2.0/24anyobject httpsخیر"}
```
Screenshots (`docs/status/tasks/WEB-1-screens/`, `-en`/`-fa` each): `01-presence-off`, `02-presence-on`, `03-ranges-time`,
`04-itemkey-summary`, `05-rule-editor`, `06-rule-editor-row-open`, `07-chips`, `08-users-item-i18n` (the generated users item
with `i18nPrefix="users:field"`: Persian labels, help and role names from the existing `users` keys). The first round of
screenshots found the three issues in the "Found in the browser run" row (duplicate "Address match" headers, reordered
English help in RTL, an overflowing table); they are fixed, and the screenshots are from the fixed build.

## Out of scope
- Pickers backed by live data (vrf/object/host-interface/secret-ref/tag lists from the candidate): they render as LTR text here and belong to WEB-2 / the domain screens, which plug in through `<SchemaForm widgets>`.
- Jalali calendar and Persian digits inside the native `time`/`datetime-local` inputs (the browser controls those inputs).
- Virtualisation of very long rule tables (polish, week 4+).
- `UsersPage` switching from `localizeUserSchema` to `i18nPrefix` (P07b's file), and fa translations for the demo schema's own labels (`apps/web/src/locales/*/dev.json` is not owned).
- `docs/05-ui-spec.md` "Schema-driven forms" does not yet list the new props and hints (not owned); the key layout is documented in `text.ts` and the widget list in `types.ts`.

## Questions / notes for the manager
- **Q1 (flake, pre-existing):** `apps/web/src/App.test.tsx` "code-splits the developer routes and renders the SchemaForm demo" waits
  8 s (`asyncUtilTimeout`) for the lazy `/dev/schema-form` route. At load ~30–55 it sometimes sees an empty `<body>` because the
  route module has not loaded yet: 1 of 3 isolated runs failed that way at 19:0x, and in one full-file run at 18:5x both /dev tests
  hit the 30 s test timeout. Timing at the same load: original demo 10.2 / 10.0 / 6.3 s, extended demo 8.5 / 9.5 / 8.9 s, so the
  bigger demo is not slower within the noise. It passed in the final suite (17.1 s) and in both CI gates. It belongs on the
  TD-12 flake list (D-121).
- **Q2 (corrected in fix round 1):** I wrote that P08's `dropPhantomOptionals` was "harmless until then". **That was wrong** (review H1): with the
  presence switch, an optional object switched on with nothing but its defaults (`dhcpClient: {setBroadcastFlag:false}`) looks exactly
  like the phantom it removes, so "just use DHCP" saved `{"host-w1w0":{}}` with no message. Fix round 1 removes it (with the
  manager's permission for this one edit in P08's files) and adds a drawer regression test.

## Fix round 1 (review `WEB-1-review.md` @ 595781c: APPROVE WITH CHANGES)

Step 0: `git -C /root/ngfw-wt/WEB-1 merge main` (P08 is on main now): clean merge, 0250955. No host runs in this round.

| finding | fix | where | test |
|---|---|---|---|
| **H1** DHCP client switched on with only defaults was deleted by P08's `dropPhantomOptionals` → `{"host-w1w0":{}}` | Removed `dropPhantomOptionals` (model.ts) and its two calls (interface + sub-interface save). Nothing else is needed: the presence switch makes a phantom impossible. P08 files edited **with the manager's permission for this fix only**. The false "harmless" claims above are corrected. | `apps/web/src/domains/interfaces/{InterfaceDrawer.tsx,model.ts}` | new drawer test: switch "Configure DHCP client" on + Save → `{'host-w1l0':{dhcpClient:{setBroadcastFlag:false}}}`; off + Save → `{'host-w1l0':{dhcpClient:null}}`; P08's "exactly the MTU" test still green |
| **M1** a partial/cleared UTC offset flipped the widget to raw text and unmounted the offset input | The widget remembers the value it last emitted: its own value stays in the structured inputs even when incomplete (`…+04:`, or no offset). The schema reports it on blur/submit. Raw text is used only for a value the widget did not produce and cannot read. | `fields/widgets.tsx` `DateTimeInput` | clear → `''`, type `+04:` (focus stays, date unchanged), Save → "Does not match the required format" and no submit; `30` → submits `…T18:00:00+04:30` |
| **L3** (DST, fractions) | A new value takes `localOffset` **at the chosen date** (`defaultOffsetFor`) until the user types an offset; fractional seconds (`.25`) are carried through | same | unit: TZ=Europe/Berlin → Dec `+01:00`, Jul `+02:00`; `…00.5+03:30` → offset edit keeps `.5` |
| **M2** `<bdi>` (auto direction) laid out Persian help that starts with a Latin word ("MTU لایه‌ی ۳ …") LTR | Help and error text are prose in the UI language: `<span dir={locale direction}>`, set from the active locale, never guessed. Explicit LTR `<bdi dir="ltr">` is used only for identifier chips, identifier table columns and identifier item keys (`isIdentifierSchema`). Words (enum labels, variant names, yes/no) follow the page. Consequence: *untranslated* English help in fa is laid out RTL again (as before WEB-1); the fix for that is its fa key. | `fields/bidi.tsx` (`prose`, `identifier`), `summary.ts`, `fields/composites.tsx` | fa render with P08's exact fa MTU and rxMode help: `dir="rtl"`, no `<bdi>` ancestor; the same English help is `dir="ltr"` in en and `dir="rtl"` in fa; unit test for `isIdentifierSchema` |
| **M3** rows opened by an error or an empty key closed as soon as typing fixed it; focus fell to `<body>` | `useRowExpansion(ids, forced)` pins every row the list had to open (errors, no key summary yet). Only the user closes a row. | `fields/composites.tsx` | the 3 reproduced cases: rule with `sequence: 0` → Save opens it → typing `7` keeps the row, the same input and the focus, and the Close button is enabled; a stored keyless user row stays open while `bo` is typed; a row opened by a server pointer stays open after clear/type/Tab |
| found while testing M3 (pre-existing since P07) | Clearing a pre-filled input made it **snap back** to the stored value, and typing then appended to it (`alicealice2`). Cause: react-hook-form's `useController` falls back to the mount-time default when a value becomes `undefined`. This also broke P08's "MTU empty = driver default". Fix: the bound field stores "cleared" as `null`, and `fromFormValue` maps it to "absent" (or keeps `null` where the schema allows null: `acceptsNull`). | `fields/SchemaField.tsx` `PrimitiveField`, `form-value.ts`, `schema-utils.ts` | render: clear "eth0" → `''`, type → `wan` (not appended), clear MTU → submitted `{name:'wan'}`; unit: `null` → absent / kept for nullable |
| **L2** pasting `8000-8080` into From gave `80008080` | A whole range pasted into either end fills both ends | `fields/widgets.tsx` `RangeInput` | port range into From, IP range into To → `8000-8080`, `10.0.0.10-10.0.0.20` |

### Left for later (not done in this round, as instructed)
L1 (time widget hides an invalid stored value), L4 (I18N-1 group keys: add `<ns>:group.<name>` as a fallback for P08's layout), L5
(ui-kit `UiHints` copy / known-widgets test against `rootSchema`), L6 (a11y: stable row-toggle names with `aria-controls`,
row-aware Move/Remove labels, `aria-required` on range inputs, `#` header through `t()`, sticky cell colour on selected rows),
L7 (whole-array watch per keystroke; week 4), nits (require `^…$`/no top-level `|` in `isAsciiOnlyPattern`; fa `expandRow`
'بازکردن' → 'باز کردن'; keep typed values when presence is switched off and on again; LTR suggestion lists).

### Verification (fix round 1, real output)
CI gate `TMPDIR=/tmp/g-web1 tools/ci.sh --base main` on the fix-round code (98fd042; this doc is the only later change):
```
branch    task/WEB-1 @ 98fd042c   (base: main)
no contract files changed in the 9 commit(s) of HEAD since main (869c5808)
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m54s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   3m44s
  apps/agent: make lint test build                   0m58s
  apps/cli: make lint test build                     0m16s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m06s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m07s
  mode quick · wall time 12m14s · logs /root/ngfw-wt/logs/ci/WEB-1-20260924-234930-1249366

CI GATE PASSED
```
Suites run directly (after the merge with main, so apps/web now includes P08):
```
packages/ui-kit  ✓ src/schema-form/web1.test.ts (17 tests)
                 ✓ src/schema-form/SchemaForm.web1.test.tsx (16 tests)
                 ✓ src/schema-form/SchemaForm.test.tsx (12 tests)        ← P07a/P07b file, unchanged
                 Test Files  14 passed (14)   Tests  81 passed (81)
apps/web         ✓ src/domains/interfaces/InterfacesPage.test.tsx (8 tests)   ← incl. the new H1 test
                 ✓ src/App.test.tsx (12 tests)
                 Test Files  14 passed (14)   Tests  99 passed (99)
```
ui-kit and web `lint`/`typecheck` are clean, and `check-logical-css: OK (125 files)`.

## Verify follow-up (`WEB-1-verify.md` @ c0eb8d28: APPROVE + C1)
- **C1:** `dropPhantomOptionals` is back in `apps/web/src/domains/interfaces/model.ts` as a `@deprecated` **identity** (returns `after`
  unchanged). The 7 in-flight F-* branches that import it still compile at rebase, and none of them can bring H1 back. Test:
  `model.test.ts` "dropPhantomOptionals is a deprecated identity". The calls in those branches, and this function, should go in a later TD.
  WEB-2's own copy (`config/collection/model.ts`) must be dropped or neutralised before WEB-2 merges (not this branch's file).
- **F1:** a lone `-` typed at either end of a range is dropped and no longer clears the other end. A pasted range counts only when both
  parts are present. Test: stored `8000-8080`, `-` typed in From and in To → `8000` / `8080`.
- F2 (untranslated English help in fa) stays open, as the verify allows.
- `pnpm --filter @ngfw/ui-kit test` → `Test Files 14 passed (14) · Tests 82 passed (82)`;
  `pnpm --filter @ngfw/web test` → `Test Files 14 passed (14) · Tests 100 passed (100)`; ui-kit and web lint/typecheck are clean.
