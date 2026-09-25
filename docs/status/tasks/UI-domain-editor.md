# UI-domain-editor — generic advanced configuration editor (web-ahead track, review 6.2, D-125)

Branch `task/UI-domain-editor` @ `01fd548b`, worktree `/root/ngfw-wt/UI-domain-editor`, base `task/WEB-1@c2e3eabb`
(speculative, D-114; WEB-1 has not landed on `main` yet as of this writing, so no merge was done — the envelope's
`git merge main` step is conditional on WEB-1 landing first). Files touched are only the owned ones: `apps/web/src/domains/advanced/**`, `apps/web/src/pages/DomainPlaceholderPage.tsx`, `apps/web/src/locales/{en,fa}/advanced.json`,
one added splat route + one namespace registration in `apps/web/src/router.tsx` / `apps/web/src/i18n.ts` (see
`UI-domain-editor-questions.md` D-UDE-2 — no anchor was pre-seeded for this task), `apps/web/test/e2e/advanced.e2e.mjs`
(new, mirrors `flow.e2e.mjs`'s pattern) and `docs/status/tasks/UI-domain-editor*`. No dependency added.

## What was built

`/config/*` renders a generic, schema-driven page for **any** JSON-pointer subtree of **any** configuration domain,
through the existing generic candidate routes and the schema's own domain tree — never a hand-written path list.

| piece | file | notes |
|---|---|---|
| Path parsing | `domains/advanced/schemaPath.ts` (`parseConfigPath`, `configPathTo`) | Splits the route splat into a domain key (checked against `ROOT_KEYS`) and relative segments; builds the splat back for links. |
| Schema tree walk | `schemaPath.ts` (`resolveNode`, `childPropertyKeys`, `withoutChildProperties`, `withoutChildValues`) | Walks the generated JSON Schema of one root key (`apps/web/src/schema/registry.ts`) using `@ngfw/ui-kit/schema-form`'s own `resolveRef`/`mergeAllOf`/`isRecordSchema`/`recordValueSchema` — the same machinery `<SchemaForm>` itself uses, so there is no drift. A record's segment is any data key; a fixed object's segment must name a property; an array's segment must be an index. Nested object/record *properties* become their own tree node (a chip) instead of a field of the parent's form — the same convention `domains/interfaces/model.ts` already uses for `subinterfaces` — and, critically, `withoutChildValues` projects the **value** handed to `<SchemaForm>` the same way, or a strict-object schema (`additionalProperties: false`) rejects the untouched sibling keys as "Unknown field" (caught by a test, see below). |
| Data read/write | `schemaPath.ts` (`valueAt`, `wrapAtPath`, `createMergePatch`) + `domains/advanced/queries.ts` | The generic pointer route can only be called with a single, slash-free path segment (see D-UDE-1) — `useDomainCandidate`/`usePatchDomain` always call it with `path` = the top-level domain key, and the JSON-pointer subtree is read/patched **client-side**. A save computes a merge patch of the node's own fields (`createMergePatch`, skipping child-container keys) and a delete is the same wrap with a `null` body — one `PATCH` of the domain root either way (no separate DELETE call; this is what the proof line "subtree PATCH/DELETE as merge patches" means here). |
| Pointer-scope helpers | `domains/advanced/subtree.ts` | `pointerInSubtree`/`inSubtree` filter the *existing* pending-change bar's own diff (`useDiff`) and the session's last/pending commit summary (`useConfirmState`, `agent.unsupported-field` warnings, `notApplied` domains) down to the current subtree — no new endpoint, no new polling. |
| Screen | `domains/advanced/AdvancedEditorPage.tsx` | Breadcrumb (domain → segment → …), a record-entry browser (list + delete + "add a new key" for map nodes) or a `<SchemaForm>` + child-container chips (for fixed-shape nodes), a Refresh button (**no `refetchInterval`** on the domain fetch — D-132), and the running-vs-candidate diff / notApplied / unsupported-field panels sourced from the shared query state above. |
| Entry point | `pages/DomainPlaceholderPage.tsx` | "Open in the advanced editor" links every domain (built or not) to `/config/<domain>`. |
| i18n | `locales/{en,fa}/advanced.json` | Real Persian text (glossary matched to `config.json`: نامزد/جاری/پیکربندی/دامنه), logical CSS only, RTL-correct (breadcrumb/labels `dir="auto"`, identifiers `dir="ltr"`). |

## Decisions (see `UI-domain-editor-questions.md` for the full reasoning)
- **D-UDE-1**: the generic route is only ever called with a single top-level segment; the JSON-pointer subtree is
  walked/patched client-side (an `openapi-fetch` limitation, not new to this task).
- **D-UDE-2**: no `// wave-A: UI-domain-editor` anchor existed; `advanced` was added to `i18n.ts` the same way
  `interfaces`/`services`/`vpn` already are, and the route was added under a marker comment I added.
- **D-UDE-3**: `createMergePatch` is now a third copy of the same pure diff (after `domains/interfaces/model.ts`); a
  kit gap worth hoisting into `@ngfw/schema` in a future `contract` branch.
- **D-UDE-4**: real-stack screenshots are blocked by a sandbox permission denial on installing a headless browser's
  shared-library dependencies (full detail + what *did* get verified below).

## How it was verified

### `apps/web` unit/component tests — `TMPDIR=/tmp/g-ude npx vitest run` (full suite, HEAD 01fd548b)
```
 Test Files  17 passed (17)
      Tests  136 passed (136)
   Duration  110.53s (transform 5.73s, setup 8.60s, collect 43.80s, tests 232.03s, environment 24.00s, prepare 4.18s)
```
The three new files:
```
 ✓ src/domains/advanced/subtree.test.ts (5 tests) 21ms
 ✓ src/domains/advanced/schemaPath.test.ts (24 tests) 28ms
 ✓ src/domains/advanced/AdvancedEditorPage.test.tsx (6 tests) 20976ms
   ✓ advanced editor — record domains (a map of named entries) > lists the candidate keys, deletes one as a merge patch, and opens a new key not yet in the candidate
   ✓ advanced editor — a record item (JSON-pointer subtree two levels deep) > saves a field as a merge patch scoped to the domain, wrapped at the item's key
   ✓ advanced editor — a fixed-shape domain root (nested containers become their own tree node) > never nulls out a nested container it never touched when saving a sibling scalar field
   ✓ advanced editor — a fixed-shape domain root (nested containers become their own tree node) > opening a nested container navigates the breadcrumb one level down
   ✓ advanced editor — bad paths never invent a hand-written domain list > an unknown domain lists the real ones from the schema
   ✓ advanced editor — Persian/RTL > renders the chrome in Persian
```
`schemaPath.test.ts` covers path parsing, the schema walk (record/fixed/array segments, `$ref` following), child-key
projection of both the schema and the value, `wrapAtPath` (including the empty-path/domain-root case), and
`createMergePatch` (removed → null, unchanged omitted, arrays replaced wholesale per D-021). `subtree.test.ts` covers
the pointer-scope matching (exact/descendant/ancestor) used for both the diff panel and the `agent.unsupported-field`
warnings, and the `notApplied` domain check. `AdvancedEditorPage.test.tsx` is the screen-level proof for
PATCH/DELETE-as-merge-patch (against a scripted `fetch`, same harness as `InterfacesPage.test.tsx`) plus the fa/RTL
render.

**A real bug the tests caught** (worth recording): the first draft passed the full node value (including its
child-container members) straight to `<SchemaForm value>` while the *schema* had those members removed — a
`z.strictObject`-derived schema's `additionalProperties: false` then rejected the untouched sibling keys as "Unknown
field: banner, dns" and silently blocked every submit (no error surfaced to the test, `patch` mutation just never
fired). Fixed by projecting the value with `withoutChildValues` too, exactly mirroring the schema projection; a
second, related bug (schema/value identity churning on every render, which would silently reset the form via
`<SchemaForm>`'s own `useEffect(() => reset(initial), [initial])`) was pre-empted by memoising `childKeys`/`formSchema`.
Both are covered by dedicated tests (`schemaPath.test.ts` "withoutChildValues" describe block; the screen-level
"never nulls out a nested container" test).

### `apps/web` lint / typecheck
```
$ eslint src && node scripts/check-logical-css.mjs
check-logical-css: OK (132 files, no physical left/right CSS)
$ tsc -p tsconfig.json --noEmit
(clean)
```

### `en`/`fa` locale parity
Covered for free by the existing `apps/web/src/locales/locales.test.ts`, which discovers namespaces by directory
listing — adding `advanced.json` to both `en/` and `fa/` is automatically exercised (12/12 tests passed in the full
run above, including the new namespace).

### CI gate — `TMPDIR=/tmp/g-ude tools/ci.sh --base main`
```
branch    task/UI-domain-editor @ 01fd548b   (base: main)
logs      /root/ngfw-wt/logs/ci/UI-domain-editor-20260925-093522-409481

== contract guard: HEAD vs main ==
no contract files changed in the 2 commit(s) of HEAD since main (869c5808)

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m08s
  generate + generated-output gate                   1m52s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   3m53s
  apps/agent: make lint test build                   1m24s
  apps/cli: make lint test build                     0m24s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m07s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m27s
  mode quick · wall time 12m22s

CI GATE PASSED
```

### Screenshots — attempted on slot 11, blocked at the last step (full detail in `UI-domain-editor-questions.md` D-UDE-4)
Stood up for real: `deploy/dev/pg-test.sh create w11` (fresh `vrx_w11`), the real API built with `tsc` and run as
`node apps/api/dist/main.js` (port 4100 — **note:** `tsx`/esbuild cannot boot this NestJS app at this base commit,
see D-UDE-4; building with `tsc` fixes it and is not a code change), `vite` on port 6100 proxying `/api` to it,
**vrx-agent not started** (the manager's af_packet-free instruction; the advanced editor's reads/writes never call
the agent, confirmed by reading `ConfigController`/`DatastoreService`). Confirmed both processes boot cleanly and the
API mapped every `ConfigController` route including `GET /api/v1/config/candidate/*`.

The remaining step — driving a headless browser to take the 8 PNGs (`/config/interfaces`, `/config/system` × en/fa
× light/dark) — could not run: `chrome-headless-shell` needs shared libraries this host does not have installed
(`libatk`, `libgbm`, `libxkbcommon`, …; the exact gap P07a's screenshot round already documented and worked around
the same way), and the harness's auto-mode safety classifier denied the `dpkg-deb -x` extraction step needed to place
them ("Modify Shared Resources"). Per that denial's own instruction I did not try another tool or path to the same
end, and I am not treating this as a corner cut silently — it is a genuine gap in this proof, disclosed here for the
manager to decide (grant that permission for a follow-up run, accept unit-test + CI proof as sufficient for now, or
have a session with the right permissions take the screenshots).

**Teardown performed regardless** (nothing was left running or lying around from the attempt, aside from a scratch
download noted below): API and vite stopped by their exact PIDs (checked with `ps` first — an unrelated
`node apps/api/dist/main.js` already running on this host, started 2026-09-24 by someone else, was left alone);
`deploy/dev/pg-test.sh drop w11` → "nothing named vrx_w11 remains"; Valkey db 11 flushed (`dbsize` 0 before and
after — nothing had actually been written); `/run/vrx-test/w11` removed; ports 4100/6100 confirmed closed
(`ss -ltn`). The downloaded `chrome-headless-shell` archive (~261 MB, unpacked, its `.zip` already removed) sits
under `/tmp/g-ude/chrome` — a second permission denial on `rm -rf` of that directory (my own scratch dir, not shared)
means it is still there; nothing references it from any committed file.

## Out of scope
- The array-index case (`resolveNode`'s array-index handling exists and is unit-tested for robustness, e.g. a
  warning's pointer landing inside an array) is deliberately never exposed as a tree node — arrays stay leaves of
  their parent's `<SchemaForm>` (D-021 "arrays are leaves"), matching how the rest of the app already treats them
  (rule-editor tables, itemKey summaries).
- A literal `DELETE /api/v1/config/{path}` / `PUT /api/v1/config/{path}` call: both write paths go through the
  domain-root `PATCH` for the reasons in D-UDE-1 (and because `PUT`'s own multi-segment-path limitation is the same
  one).
- Real-browser screenshots (D-UDE-4).

## Open questions
See `docs/status/tasks/UI-domain-editor-questions.md`: D-UDE-1 (pointer-route limitation), D-UDE-2 (missing wave-A
anchor), D-UDE-3 (`createMergePatch` kit gap, worth a `contract(schema)` follow-up), D-UDE-4 (screenshot blocker +
the `tsx`/NestJS DI finding), and two self-reported rule slips (one `pkill`, one read-only `git status` run in
`/root/ngfw` instead of the worktree) — both disclosed, neither repeated, no lasting effect found.
