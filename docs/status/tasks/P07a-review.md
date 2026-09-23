# P07a review — UI shell (theme/RTL, i18n, SchemaForm, ServerDataGrid, WS hook, frame skeleton)

Reviewer: independent review agent · 2026-09-24 · branch `task/P07a` @ `22febb3` · merge-base `main@2de6c2f`
(`git merge-tree` against current `main` is clean; main has moved 65 commits and none of them touch `apps/web` or `packages/ui-kit`).

## Checklist

| # | Check | Result |
|---|---|---|
| 1 | Contract compliance | OK. No hits under `packages/schema`, `packages/proto`, `apps/agent/gen`, `packages/api-client`. Only file changed outside the owned paths: `pnpm-lock.yaml` (needed for the new deps; acceptable). |
| 2 | Real verification (VPP) | N/A. This is a UI-only task with no server calls. The unit tests are real: the WS client is tested against a real `ws` server on loopback, and the form, grid and shell are tested in jsdom. |
| 3 | Restart safety | N/A (no VPP objects). |
| 4 | VPP API provenance | N/A. `binapi/` and `tools/binapi-gen.sh` are untouched. |
| 5 | Shared-host rules | OK. Vite dev and preview bind `127.0.0.1` on `VRX_WEB_PORT` with `strictPort`, and the proxy goes to `VRX_HTTP_PORT`. The status file says the preview server was killed by PID. |
| 6 | Security | OK. No `exec`, `child_process`, `dangerouslySetInnerHTML`, `eval` or `new Function`. localStorage holds only UI settings. The test PSK uses the `VRX_TEST_PSK_<id>` convention. |
| 7 | Transaction semantics | N/A. |
| 8 | UI honesty | **Partial.** Unbuilt screens show "Not yet available" and no data. The Dashboard shows the real `/api/v1/health` card. No TODO, mock or stub markers. **But:** there is no screenshot in `P07a.md` (the browser check is described in text only), and the synthetic-data demo pages ship to every user (M1, M2). |
| 9 | Scope creep | **Some** (see M1). `/dev/data-grid` (20 000 synthetic interface-like rows), `/dev/stream` and a "Developer" nav group are shipped in production builds. The prompt asked only for "Storybook or a `/dev/schema-form` route". The extra widgets (slider, chips, json, radio, textarea, password), the `contrast()` helper and the bundle-budget script are small and justified. |
| 10 | i18n / RTL | **Mostly OK.** `check-logical-css` passes, and my own grep found no physical `left/right`, `ml/mr` or `margin-left`. `en`/`fa` key parity holds for all three web namespaces (checked by hand) and for ui-kit (tested). The i18n lint rule really fires, but it has blind spots (L2). Technical inputs are not forced to LTR in RTL (L7). |
| 11 | Tests actually run | **OK.** I ran `tools/ci.sh --base main` myself in `/root/ngfw-wt/P07a` with the slot-8 env: `CI GATE PASSED`, `EXIT=0`, the same stages as the pasted output. Because the CI log hides turbo output, I also ran the packages directly: ui-kit `Test Files 10 passed, Tests 33 passed` (56 s), web `Test Files 2 passed, Tests 8 passed`, web lint `check-logical-css: OK (71 files …)`. This matches the pasted output (33/33, 8/8). |

## Findings (ranked)

### Medium

**M1 — Developer demo routes, including 20 000 synthetic "interface" rows, ship to every user in production**
`apps/web/src/nav/nav.ts:74-78`, `apps/web/src/router.tsx:29-31`, `apps/web/src/pages/dev/DataGridDemoPage.tsx:20-33`
- Failure scenario: an operator on a production appliance opens the "Developer" nav group. They see a table of `demo-00001…` rows with up/down/degraded `StatusChip`s, MTUs and bit rates. It looks exactly like device data, and a warning banner is the only signal that it is not. The prompt asked only for `/dev/schema-form` (or Storybook). The data-grid and stream demos and the nav group are extra. D-P07a-7 decided this unilaterally, and it conflicts with "never ship a UI screen whose backend is stubbed".
- Fix: gate the `dev` nav group and `/dev/*` routes behind `import.meta.env.DEV` or an explicit `VITE_VRX_DEV_ROUTES=1` build flag, so production builds have neither the routes nor the chunks. Otherwise, drop `/dev/data-grid` and `/dev/stream` and keep only `/dev/schema-form`. Mark D-P07a-7 for the manager's decision instead of treating it as taken.

**M2 — No screenshot in the status file (review check 8)**
`docs/status/tasks/P07a.md` ("Built app checked in a browser" section)
- Failure scenario: the RTL rendering, the drawer side, the focus ring and the "not yet available" pages are claimed in prose only. The reviewer and the manager cannot check the RTL/visual acceptance without re-running a browser.
- Fix: add at least four screenshots under `docs/status/tasks/P07a-img/` and link them from `P07a.md`: `/` in en light, `/` in fa dark (RTL), `/dev/schema-form` in fa, and a placeholder domain page.

**M3 — Fields hidden by `dependsOn` are still validated and submitted**
`packages/ui-kit/src/schema-form/fields/SchemaField.tsx:160-173` (`DependsOnGate` only unmounts the UI), `packages/ui-kit/src/schema-form/resolver.ts:194-224`
- Failure scenario: the user types an invalid VRF, then switches "Enabled" off, so the VRF field disappears. React-hook-form keeps the value (`shouldUnregister` defaults to false). Zod reports the error on a field that is no longer rendered, so Save does nothing and the user sees no message. In the valid case, a stale hidden value is still sent to the server. The `dependsOn` test (`SchemaForm.test.tsx:73-80`) checks only visibility.
- Fix: when a `dependsOn` condition is false, drop the gated sub-path in `fromFormValue` before validation and submission, or register the gated subtree with `shouldUnregister`. Add a test: hide an invalid field, then submit succeeds and the payload has no hidden key.

**M4 — `compile()` silently turns validation off when `z.fromJSONSchema` throws**
`packages/ui-kit/src/schema-form/form-value.ts:85-95` (`catch { zs = z.unknown(); }`)
- Failure scenario: once P02a/b/c land real domain schemas, any construct `fromJSONSchema` cannot handle makes that form accept anything, with no signal to the user or the developer. Only the server would catch the bad input. Today's domain schemas are `{}`, so nothing exercises this path yet.
- Fix: surface the fallback. `console.error` in dev, plus a form-level "client-side validation unavailable" alert through `ROOT_ERROR`. Add a web test that `rootSchema` and every `domainSchemas[key]` compile without falling back; it guards P02a's merge.

### Low

**L1 — Compile cache misses on `$ref`/`allOf` schemas; the SchemaForm suite is very slow**
`schema-utils.ts:29-41` (`resolveRef` returns a new object on each call), `schema-utils.ts:100-114` (`mergeAllOf` likewise), `fields/composites.tsx:419,424`, `form-value.ts:86-96` (WeakMap keyed by object identity)
- Failure scenario: in a `oneOf` with `$ref` variants, every keystroke recompiles every variant with `z.fromJSONSchema`, and each compile carries all of root `$defs` via `withDefs`. This gets slow on the full root schema. The suite already takes 50–63 s, with single tests taking 5–10 s against a raised 30 s `testTimeout` (`vite.config.ts`). That is a flake risk when 12 workers share the host.
- Fix: memoise `resolveRef` and `mergeAllOf` per input object in a WeakMap, so `compile` hits its cache. Profile the slowest tests.

**L2 — The i18n lint rule has blind spots** (I probed it on temporary files in `apps/web/src`, deleted afterwards)
`apps/web/eslint.config.js:12-21`, `packages/ui-kit/eslint.config.js:12-21`
- Caught: JSX text, `{"literal"}`, ternary literals, and `aria-label`, `placeholder` and `label` attributes.
- **Missed:** `<X tooltip="Click to save" />` and any attribute not on the allowlist (`subheader`, `description`, `message`, `aria-valuetext` and so on), and all-caps text such as `<p>MTU</p>` or `<p>OK</p>` (excluded by the `[A-Z_-]+` word rule). Strings outside JSX, such as `GridColDef.headerName` in column arrays and anything in `.ts` files, are also not checked. There is also no automated `en`/`fa` parity test for `apps/web/src/locales` (parity holds today; ui-kit has one).
- Fix: widen the attribute allowlist, or switch to an exclude-list of non-text attributes (`to`, `href`, `id`, `name`, `variant`, `color`, `size`, `component`, `data-*`). Accept all-caps words only through the key files. Port `locales.test.ts` to `apps/web`.

**L3 — The Users, Revisions and Tools placeholder titles do not follow a language switch**
`apps/web/src/router.tsx:12-14` (`NotAvailableByKey` calls `i18n.t` without subscribing)
- Failure scenario: open `/system/users` and switch to فارسی. The route element is created once, so the heading stays English until the user navigates away.
- Fix: use `useTranslation()` inside `NotAvailableByKey`.

**L4 — Nested domain paths select two nav items and set `aria-current` twice**
`apps/web/src/shell/AppShell.tsx:43-45`, `apps/web/src/nav/nav.ts:48-51`
- Failure scenario: on `/vpn/tunnels`, both "VPN" (`/vpn`) and "Tunnels" are `selected`, and NavLink sets `aria-current="page"` on both. The same happens for `/system` and `/system/users` or `/system/revisions`, and for `/routing` and `/routing/vrfs`. Screen readers announce two current pages.
- Fix: use exact matching (`end`) for domain items, or map the root-key domains that equal their group to `/<group>/settings`.

**L5 — ServerDataGrid live-table behaviour**
`packages/ui-kit/src/data-grid/ServerDataGrid.tsx:155`, `:178`
- With `refetchInterval` set, `loading` is true on every background refetch, so the loading overlay flashes every poll.
- A background refetch error while stale rows are shown gives no indication, because the error overlay is `noRowsOverlay`.
- Fix: `loading = isPending || (isFetching && isPlaceholderData)`, and show an inline error (toolbar or `Alert`) when `isError && data`.

**L6 — WS client edge cases**
`packages/ui-kit/src/ws/client.ts:114`, `:245-248`, `:123`; `packages/ui-kit/src/ws/useTopic.ts:317-324`
- (a) The per-topic buffer is unbounded. Background tabs throttle `setInterval` to about once a minute, so a burst of samples accumulates and is then delivered in one go. Cap it, for example keep the last N entries per topic.
- (b) A new `subscribe()` during backoff calls `connect()` right away and bypasses the delay. Guard it with `reconnectTimer`.
- (c) `closeWhenIdle` closes and reopens the socket when navigating between two pages that use topics. Add a short idle grace delay.
- (d) `useTopic` keeps the previous topic's `data` when `topic` changes. Reset the state in the effect.

**L7 — Technical inputs render RTL in `fa`**
`packages/ui-kit/src/schema-form/fields/inputs.tsx:46`, `:142`
- Failure scenario: in `fa`, IP, CIDR, MAC, interface-name and JSON inputs are right-aligned RTL text boxes. Mixed IPv6 or interface strings get bidi-reordered while typing.
- Fix: set `dir="ltr"` on the input element (`slotProps.htmlInput`) for the `MONO_WIDGETS` and the json widget.

**L8 — `<SchemaForm value>` resets on identity change**
`packages/ui-kit/src/schema-form/SchemaForm.tsx:83-93`
- Failure scenario: a caller that passes an inline object literal (`value={{…}}`) resets the form on every parent render, which wipes the user's edits. This will be likely in P08 screens.
- Fix: deep-compare (or JSON-key) `value` before `reset`, or document the requirement loudly in the JSDoc.

### Info
- `pnpm-lock.yaml` is outside the owned paths; the change is unavoidable when adding deps. OK.
- `createVrxTheme.ts:43` lists `Inter` and `Vazirmatn`, but neither is loaded, so `fa` falls back to system fonts. Bundle Vazirmatn or drop it from the stack.
- `apps/web/src/schema/registry.ts` duplicates `generateSchemas()` (question 8). Re-export it from `@ngfw/schema` via a `contract/` branch later.
- ui-kit strings live in TS resources (`packages/ui-kit/src/i18n/locales/*.ts`), not in `apps/web/src/locales/*.json`. 00-CONTEXT names the latter location. This is acceptable for a reusable package, but the manager should note it.
- The "Tests actually run" requirement is met, but `tools/ci.sh` hides turbo output, so its log alone cannot distinguish cache hits from real runs. That is a manager tooling item, not P07a's.

**APPROVE WITH CHANGES**. Required before merge: M1 and M2. M3 and M4 should be fixed or split out as a named follow-up before P08 relies on SchemaForm. The L items can go to P07b.
