# UI-domain-editor — fix round 1 focused verify

Branch `task/UI-domain-editor` @ `ed1e28ce` (on top of review `3ac4825e`). Focused verify per the manager's request,
scope limited to D-UDE-1's fix, the screenshots, and regressions. Time-boxed 30 min.

## Verdict: **APPROVE**

## D-UDE-1 — real pointer depth

`packages/api-client/src/config-path.ts` (`configPointerPath`, `configUrl`) is hand-written (not in
`src/generated/`), exported from `packages/api-client/src/index.ts` with a comment marking it as such. It
percent-encodes each RFC 6901 pointer segment on its own and rejoins with a literal `/`, instead of
`openapi-fetch`'s `defaultPathSerializer`, which percent-encodes the whole path-param value in one piece (confirmed
in review 3ac4825e by reading `openapi-fetch`'s source directly). `AdvancedEditorPage`/`queries.ts` now call
`useConfigNode(pointer)` / `usePatchConfigNode()`, which GET/PATCH `configUrl(prefix, pointer)` directly — the whole
"fetch/patch the domain root, walk client-side" path (`useDomainCandidate`, `usePatchDomain`, `wrapAtPath`) is gone.

**Server-side round-trip, independently verified against the real code (not just read):**
`apps/api/src/config/config.controller.ts`'s generic routes are NestJS wildcards (`@Get('*')`/`@Patch('*')`/etc.,
already true before this fix) and `apps/api/src/config/path.ts`'s `pointerFromUrl` splits the raw URL on real `/`
characters. I ran the built `pointerFromUrl` (`apps/api/dist/config/path.js`) against `configPointerPath`'s output
(`packages/api-client/dist/index.js`, freshly built) for 7 pointers — multi-segment, an interface name with an
escaped `/` (`~1`), a name with `~0`, a literal `%`, non-ASCII (Persian) text, the domain root, and the empty
pointer — every one round-tripped back to byte-identical to the original pointer. Script and output:

```
{"pointer":"/interfaces/TenGigabitEthernet0~10~10/mtu", ..., "match":"MATCH"}
{"pointer":"/interfaces/weird~0name/value", ..., "match":"MATCH"}
{"pointer":"/interfaces/100%/value", ..., "match":"MATCH"}
{"pointer":"/interfaces/دامنه/value", ..., "match":"MATCH"}
{"pointer":"/interfaces/eth0/subinterfaces/100", ..., "match":"MATCH"}
{"pointer":"/interfaces", ..., "match":"MATCH"}
{"pointer":"", ..., "match":"MATCH"}
```

**The 9 tests** (`apps/web/src/domains/advanced/configPath.test.ts`, run in `apps/web`'s own vitest since
`packages/api-client` has no vitest of its own — correctly noted in the test file's own comment, matches
`package.json`'s `test` script, which only type-checks a built consumer): 7 for `configPointerPath` (keeps real
`/` between segments; single-segment round-trips unchanged; empty/root pointer has no suffix; `~0`/`~1` survive
untouched; a literal `%` and non-ASCII are per-segment percent-encoded; a plain alnum segment needs no encoding) + 2
for `configUrl` (candidate vs write prefix; root pointer resolves to the prefix alone). All pass; content matches
what I independently re-derived above.

**Screen-level proof of the real depth fix**, not just the helper: `AdvancedEditorPage.test.tsx` now mocks
`GET /api/v1/config/candidate/interfaces/eth0` and `GET /api/v1/config/candidate/system/banner` directly (previously
these tests mocked only the whole-domain GET and walked client-side) — real proof, since `installFakeApi` stubs
`globalThis.fetch` and matches on the literal `url.pathname`, so this exercises the actual `openapi-fetch` call +
`configUrl` string construction, not a shortcut. PATCH assertions also confirm the exact-pointer body: saving `mtu`
on `/config/interfaces/eth0` now asserts `patched === { mtu: 1400 }` (previously `{ eth0: { mtu: 1400 } }`), and
delete is still a merge patch of the **parent** (`{ eth0: null }` at `/interfaces`), which is correct RFC 7386 (you
remove a key by patching its parent, not the key's own pointer). This also closes the one screen-level test gap I'd
flagged in the prior review (no depth-≥2 screen test) — now covered.

**Severity resolved.** This was rated Medium/non-blocking in the prior review specifically because every write
fetched/patched the whole domain; that concern is now gone — reads and writes address the real pointer depth
end-to-end, with no `contract` branch needed (confirmed again: the server side needed no change).

## Screenshots

8 PNGs present at `docs/status/tasks/UI-domain-editor-screens/` (`{en,fa} x {light,dark} x {interfaces,system}`).
`docs/status/tasks/UI-domain-editor.md` §"Fix round 1" pastes the real `shots.mjs` run: `SHOTS OK (18 checks, 1
screen(s) x 2 lang(s) x 2 theme(s))`, each check asserting the heading is visible and `<html dir>` matches the
language (`ltr` for en, `rtl` for fa) — 18/18, 0 page errors (the harness's `assertNoPageErrors` would have failed
the run otherwise). Teardown (PIDs, `pg-test.sh drop w11`, Valkey flush, port check) is pasted and matches the
pattern from the original task and the prior review's independent re-check.

Viewed `advanced-fa-dark-2-config-system.png` and `advanced-en-light-1-config-interfaces.png` directly:
- **fa/dark**: nav drawer on the right, chevrons flipped to the RTL side, breadcrumb/labels/buttons correctly
  translated (بازخوانی، مقایسه با پیکربندی جاری، شامل، ذخیره در نامزد، بازنشانی), warning banner icon on the left
  with right-aligned Persian text — all chrome strings correctly Persian. Schema-derived content (the `hostname`/
  `timezone` field titles, the "DNS client"/"Banners" child-node chip labels) stays in English, because this is a
  generic editor with no per-domain field-title localization data — expected and consistent with the architecture
  (only WEB-1-built screens like `interfaces` have `localizeSchema`+translated field dictionaries; the generic
  editor correctly does not invent one). Not a defect.
- **en/light**: standard LTR layout, nav on the left, "No entries yet." / "New key" / "+ Add" all in English, dark
  mode correctly not applied. Matches the interfaces record-list screen from the earlier (non-screenshot) review.

No RTL defects found in either shot.

## Regressions

Reran independently (not just re-read the pasted output):
```
$ TMPDIR=/tmp/g-ude-review pnpm --filter @ngfw/web test
 Test Files  18 passed (18)
      Tests  142 passed (142)
$ npx eslint src && node scripts/check-logical-css.mjs   (apps/web)
check-logical-css: OK (133 files, no physical left/right CSS)
$ npx tsc -p tsconfig.json --noEmit   (apps/web)
(clean)
$ pnpm --filter @ngfw/api-client build && pnpm --filter @ngfw/api-client test && pnpm --filter @ngfw/api-client lint
api-client: dist/generated/schema.d.ts present
eslint src && redocly lint openapi.json — clean, "Your API description is valid."
```
142/142, matching the manager's number exactly (136 in the previous round + the 6 net new: +9 `configPath.test.ts`,
−6 `schemaPath.test.ts`'s removed `wrapAtPath` block, −1 net elsewhere — nets out to +6, confirmed by the file-level
counts in the diff). Lint/typecheck clean in both `apps/web` and `packages/api-client`.

`git diff c2e3eabb ed1e28ce --name-only` still touches only owned files (`apps/web/src/domains/advanced/**`,
`packages/api-client/src/config-path.ts` + `index.ts` — a new file the task now legitimately owns as part of the
D-UDE-1 fix, not a scope violation: it's a hand-written addition to fix the exact wiring problem this task's own
review raised, not a change to the generated schema/OpenAPI contract — plus the screenshots and status docs).
`BUILT_DOMAINS`, `NavItem.available`, `nav.test.ts`, `packages/ui-kit`: no hits. Working tree clean, nothing stray.

## Findings

None blocking. One small note for the manager's own tracking, not a code issue: `packages/api-client/test/consumer.ts`
(the package's own type-check-only "test") does not exercise `configPointerPath`/`configUrl` — coverage for them
lives entirely in `apps/web`'s vitest. That's an accurate, disclosed choice (the test file says so), not a gap
worth blocking on, since `@ngfw/web` is the only current consumer and the function is trivial and now
independently re-verified end to end above.

**APPROVE.**
