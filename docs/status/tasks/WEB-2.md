# WEB-2 — config screen kit, data widgets, Secrets page (web-ahead track, D-123)

Branch `task/WEB-2`, worktree `/root/ngfw-wt/WEB-2`, base `task/P08@abb6950` (speculative start, D-114), slot 1.
Commits are on the branch. `main` and other worktrees were not touched.

## 1. What was built

### Config screen kit, `apps/web/src/config/collection/`
A *collection* is a node of the candidate document that holds many items. The kit handles any collection from the
schema alone (00-CONTEXT rule 5); no domain needs its own code:

| shape | examples | key from the schema |
|---|---|---|
| `map` | `interfaces.<name>`, `vrfs.<name>`, `objects.addresses.<name>`, nested `interfaces.<name>.subinterfaces.<id>` | `propertyNames` (pattern, length) |
| `list` | `management.users[]` (`username`), `routing.static[]` (`vrf`+`prefix`), `management.aaa.radius.servers[]` (`address`+`authPort`) | the array's `x-vrx-ui.itemKey` |

| file | what |
|---|---|
| `model.ts` | Pure functions. `collectionModel(spec)` gives shape, item and form schema (with `omit`) from `schema/registry`. Also: key ids and labels; `mapKeyIssue` (validation against `propertyNames`); `valueAt`/`nest`; `createMergePatch`, `deepEqual`, `dropPhantomOptionals`; `nextList` (N4 for arrays); `collectionRows` (candidate vs running → new/changed/removed); `itemPointer` / `problemAt` (RFC 6901, boundary-safe); `localizeSchema` (title/help/group/enum labels from the domain namespace) |
| `queries.ts` | `useDomainNode(domain, 'candidate'\|'running')`, `useFreshCandidate`, `usePatchDomain`. **Fix round 1 (review H1):** the kit caches the WHOLE node under its own key, one level below P08/Users' key for the same domain — `['config','candidate',d,'node']`, not `['config','candidate',d]` — because the exact key collided with Users and the pending-change bar (`config/effective.ts`), which cache one member of the same node (`.users`) under it and crashed on the kit's object shape. The shell's `['config']` invalidation, and any `qk.candidate(d)` / `['config','running',d]` prefix invalidation, still reach the kit's key |
| `useCollection.ts` | `useCollection(spec)` (read side: model, candidate, running, rows). `useItemEditor(collection, id)` is the write side for one item and carries P08's reviewed semantics: the form edits the value it opened with (N4); Save sends only what changed; "changed elsewhere → Reload"; a phantom optional object is dropped (Q2); a new key is checked against the fresh candidate (N5); server pointers are mapped onto the fields. Lists rewrite the array with only the edited members applied to the item as it is now. `onSaved` hook (Users: remember a password edit, never its value) |
| `CollectionDrawer.tsx` | The drawer shell from P08: `anchor="right"` (RTL flip), region label, key title LTR monospace, live-status slot, close |
| `ItemEditor.tsx` | SchemaForm over the item. Server problem on top, saved / changed-elsewhere alerts, key field for a new map item, Remove with confirmation |
| `CollectionView.tsx` | The composed screen body: toolbar (Add, gated on `perms.editConfig`), grid with the key cell as a button, caller columns (a bare `field` shows that item member), a **live-status slot** column, a pending-state column, rows removed in the candidate struck through, and the drawer with slots `drawerStatus` / `drawerLive` / `drawerExtra` |

### Data widgets, `apps/web/src/config/widgets/`
- `LocalDataGrid`: a **ServerDataGrid wrapper** over rows that are already in the browser. It keeps the same paging protocol, overlays and RTL/locale; the rows' identity is its version.
- `paging.ts`: `pageRows` / `matchesFilter`. P08's `pageOf` made generic, with every grid operator and `and`/`or` logic.
- `KeyButton`: **the key cell as a real `<button>` (A11Y-1)**. It takes focus when the grid moves to its cell (`hasFocus`, as GridActionsCell does), Enter/Space open the item, and it stops propagation so the grid never sees them as row selection or cell editing.
- `cells.tsx`: `IdText` (LTR monospace `<bdi>`), `StatusCell` (StatusChip or "not in the data plane"), `CounterText`, `RateText`, `RowStateChip`, `LiveChip` (WS state), `Why` (tooltip on disabled actions).
- `format.ts`: `formatCounter` is exact for 64-bit decimal strings (D-039) beyond 2^53, with grouping and Persian digits; `sumCounters` is the BigInt sum (P08's errors + drops).
- `Sparkline` (P08's, shared).
- `KitPreview.tsx`: dev-only preview of every widget with synthetic values.

### System › Secrets, `apps/web/src/pages/SecretsPage.tsx`
List, create, rotate and delete **by reference** on the P06 store (`/api/v1/secrets`, D-051). Only generated OpenAPI types are used.
- The list shows reference, kind, name and created time. The reference cell is a `KeyButton` that opens a details drawer (`CollectionDrawer`) with usage help and Rotate/Delete.
- Create: kind comes from `SECRET_KINDS`, and the name is validated with `secretRef` of packages/schema (the one schema). Rotate sends `POST ?replace=true` with kind and name fixed; revisions keep the old version for rollback. Delete confirms first; a 409 `secret-in-use` is shown with its pointers (where the reference is used).
- **The value never enters form state, the query cache or logs.** It is typed into an *uncontrolled* input (`type=password`, or a multi-line field for PEM `key`/`cert`, `autocomplete=new-password`). The page reads it from the DOM only inside the request function. Mutation variables hold `{kind, name, replace}`, and React state holds only a boolean "has a value". The input is emptied after a successful store and dropped on close. Nothing is logged. A 409 `secret-exists` is mapped to the name field.
- Admin only (the API's `@MinRole('admin')`). Other roles see the list with every action disabled and a reason tooltip.
- **Not routed and not in the navigation.** Main has no `// web: WEB-2` anchor (envelope), so there is no routed stub. Q2 gives the wiring lines.

### Previews, `apps/web/src/pages/dev/previews.ts`
`DEV_PREVIEWS` (dev builds only; `[]` in production) holds `/dev/config-kit` (KitPreview) and `/dev/secrets` (the Secrets page on its real API). Each entry carries its route and its nav item for the "Developer" group. Nothing is wired: the anchors are missing (Q2).

### Strings
`config:kit.*` in `locales/{en,fa}/config.json`: generic kit strings, `kit.secrets.*` and `kit.preview.*`. en and fa have identical keys and variables (the locales test checks this). Every UI string goes through `t()`, and CSS uses logical properties only.

## 2. What was extracted from P08 and System › Users

WEB-2 does not own `domains/interfaces/*` or `pages/UsersPage.tsx`, so both screens are **unchanged** (behaviour kept
by construction). Their generic parts now also live in the kit. `config/collection/equivalence.test.ts` runs each
screen's own helper and the kit's on the same inputs:

| P08 / Users | kit | equivalence test |
|---|---|---|
| `interfaceItemSchema`, `interfaceFormSchema`, `subinterfaceSchema` | `collectionModel({domain:'interfaces', omit:['subinterfaces']})`, nested path `[name,'subinterfaces']` | deep-equal schemas |
| `userItemSchema` | `collectionModel({domain:'management', path:['users']})` | deep-equal |
| `createMergePatch`, `dropPhantomOptionals` | same names in `model.ts` | same outputs on a corpus |
| `localizeSchema` (P08), `localizeUserSchema` (Users) | `localizeSchema` (both: title, help, group, enum labels) | Users: deep-equal; P08: equal titles/help/groups, enum labels = the value (what SchemaForm shows by default) |
| `problemFor`, `problemForItem` | `problemAt` + `itemPointer` | Users: deep-equal; P08: equal except the boundary bug below |
| `pageOf` | `pageRows` | the same pages for 8 requests (paging, sorts, quick filter, column filter) |
| `rowStates` | `collectionRows` | the same `[key, index, state]` |
| drawer shell, N4 opened value, changed-elsewhere, N5 key check, `Why`, pending chip, Sparkline, live chip | `CollectionDrawer`, `useItemEditor`, `ItemEditor`, `widgets/*` | component tests (§4) |

## 3. Deliberate differences, kit vs the per-screen copies
1. `problemAt` maps only pointers **at or below** the item. P08's `startsWith` turned `/interfaces/host-w1l0x/mtu`, an error of another interface, into field `x/mtu` of `host-w1l0`. The kit leaves it absolute, so it is listed on top of the form; asserted in the equivalence test.
2. A list save is N4 for arrays: `nextList` applies only the changed members to the item **as it is now** in the candidate. Users writes back the whole form item, which can overwrite a member another session changed. The component test covers this: role edit + concurrent `fullName` change → both kept.
3. A new map key is validated with the schema's `propertyNames`, not P08's hand-written `NAME_RE`. `NAME_RE` accepts `.`, which the schema's interface-name pattern rejects, and the server then answered 400.
4. `deepEqual` for row state ignores member order. Users compared `JSON.stringify` results.

## 4. How it was verified

### 4.1 Unit tests (jsdom, scripted stand-in of the API as in P08's screen tests)
```
$ cd apps/web && npx vitest run src/config src/pages/SecretsPage.test.tsx src/locales
 ✓ src/config/confirm-store.test.ts (3 tests) 16ms
 ✓ src/locales/locales.test.ts (9 tests) 152ms
 ✓ src/config/refine.test.ts (3 tests) 25ms
 ✓ src/config/collection/model.test.ts (18 tests) 40ms
 ✓ src/config/collection/equivalence.test.ts (10 tests) 49ms
 ✓ src/config/widgets/widgets.test.tsx (9 tests) 13526ms
 ✓ src/pages/SecretsPage.test.tsx (8 tests) 42840ms
 ✓ src/config/collection/collection.test.tsx (6 tests) 44959ms
 Test Files  8 passed (8)
      Tests  66 passed (66)

$ cd apps/web && pnpm test            # whole web suite, P08 and Users screens included, unchanged
 ✓ src/config/collection/collection.test.tsx (6 tests) 39487ms
 ✓ src/pages/SecretsPage.test.tsx (8 tests) 44384ms
 ✓ src/domains/interfaces/InterfacesPage.test.tsx (7 tests) 64124ms
 ✓ src/flows.test.tsx (6 tests) 14358ms
 ✓ src/App.test.tsx (8 tests) 23928ms
 …
 Test Files  18 passed (18)
      Tests  139 passed (139)

$ cd apps/web && npx tsc -p tsconfig.json --noEmit && pnpm lint
$ eslint src && node scripts/check-logical-css.mjs
check-logical-css: OK (135 files, no physical left/right CSS)
```
What these tests pin down:
- **A11Y-1.** The key cell is a `<button type="button">` with an accessible name. Tab reaches it, and Enter and Space open the item (user-event). It takes focus when its grid cell gets it (`hasFocus`), and a click does not also fire the row click. In the collection and Secrets screens, focusing the key button and pressing Enter opens the drawer.
- **Secret value.** A create or rotate sends the value exactly once in the POST body (`?replace=true` only for rotate). Afterwards the value is absent from the whole query cache (every query and mutation state, variables and errors included), from every `console.*` call, and from the DOM. The input is emptied after the store and has `type=password` and `autocomplete=new-password`. For key/cert kinds the value field is a multi-line textarea.
- **P08 semantics generalised.** Save sends only the changed members (`{red: {id: 11}}`) while another session changed the description. The "changed elsewhere" warning appears, and Reload shows the combined value (N4). A new key is checked against the schema (`-bad`: invalid) and against the fresh candidate (`red`: taken, nothing sent) (N5). A server pointer `/vrfs/red/id` lands on the Table ID field. Remove asks for confirmation and sends `{red: null}`. A read-only role cannot add.
- **Lists.** The Users array is rewritten with the role edit applied to the item as it is now, so a concurrent `fullName` change survives. A rename onto an existing username is refused on the field and nothing is sent.
- **Equivalence with the per-screen helpers** (§2), property-tested merge-patch round trip (300 generated documents through `mergePatch` of packages/schema), exact 64-bit counters (`18446744073709551615` → `18,446,744,073,709,551,615`, Persian digits), grid operators, and previews that render in fa/RTL.

### 4.2 CI gate: `TMPDIR=/tmp/g-WEB-2 tools/ci.sh --base main`
```
== VRX CI gate: quick ==
branch    task/WEB-2 @ 3b53c2a   (base: main)
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:            # P08's (the base), not WEB-2's — see below
  apps/agent/gen/vrx/v1/dataplane.pb.go · apps/agent/gen/vrx/v1/dataplane_grpc.pb.go · packages/api-client/src/generated/schema.d.ts
  packages/proto/gen/ts/vrx/v1/dataplane.ts · packages/proto/vrx/v1/dataplane.proto
ok — contract commit(s) on the branch:
  6ce08c2 contract(api-client): … (P08)   f6fbdf3 contract(api-client): … (P08)   c02aa32 contract(api-client): … (P08)   51b7c42 contract(proto): … (P08)
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~697515 bytes (697.52 KB) in 1.04s no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    20 cached, 30 total Time:    3m27.735s
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m49s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   3m29s
  apps/agent: make lint test build                   0m39s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m05s
  mode quick · wall time 6m19s · logs /root/ngfw-wt/logs/ci/WEB-2-20260924-191858-2498934

CI GATE PASSED
```
- The run above is attempt 2. Attempt 1 stopped in the contract guard's SIGPIPE flake (Q1), which fails about half of all
  runs on any P08-based branch. Only guard failures were retried, never another step. An earlier full run on
  `2065464` also passed (`logs/ci/WEB-2-20260924-190225-2210989`, wall time 9m34s). The only commit after `3b53c2a`
  is this document.
- WEB-2 itself changes no contract file: `git diff --stat abb6950 HEAD -- packages/schema packages/proto apps/agent/gen
  packages/api-client/src/generated` is empty. The files listed come from the P08 base.
- WEB-2 adds no route and no import to the app, so the production bundle and `check-no-dev-routes` (inside the build step)
  are unaffected.

### 4.3 The Secrets page's requests against the REAL API (slot 1, no mocks)
The page is not routed (Q2), so there is no browser E2E. Instead, the exact request sequence the page sends ran against
this worktree's P06 API on slot 1: port 3100, database `vrx_w1` from `deploy/dev/pg-test.sh`, Valkey db 1 with the
prefix `w1web2:`, and a bootstrap password and JWT key read from 0600 files. The final DELETE goes through the generated
openapi-fetch client, as `api.DELETE` does in the page. Script: `docs/status/tasks/WEB-2-secrets-real-api.mjs`.
```
$ PWFILE=… API_CLIENT=packages/api-client/dist/index.js node docs/status/tasks/WEB-2-secrets-real-api.mjs
PASS  login admin → 200
PASS  create → 200 {"ref":"psk/w1-web2","created":true,"version":1}
PASS  create answer does not contain the value
PASS  create again → 409 https://vrx.dev/problems/secret-exists (page: name-field error)
PASS  409 problem does not echo the value
PASS  rotate (?replace=true) → 200 {"ref":"psk/w1-web2","created":false,"version":2}
PASS  oversized value → 400 pointers ["/value"] (page: value-field error)
PASS  list → 200 {"ref":"psk/w1-web2","kind":"psk","version":2,"createdAt":"2026-09-24T15:47:25.544Z","name":"w1-web2"} (no value)
PASS  candidate references psk/w1-web2 → 200
PASS  delete while referenced → 409 https://vrx.dev/problems/secret-in-use [{"pointer":"/management/aaa/radius/servers/0/secretRef","message":"references psk/w1-web2"}]
PASS  discard candidate → 200
PASS  delete via the generated client (the page's api.DELETE) → 204
PASS  delete again → 404

$ grep -c VRX_TEST_PSK_WEB2 api.log                 # API at VRX_LOG_LEVEL=debug, 54 lines
0
$ psql vrx_w1 …
audit rows for secret/psk/w1-web2: 8
audit rows containing the value: 0
system_event rows (secrets): 6, containing the value: 0
secret_version rows with plaintext: 0
```
The first run of the script had a bug of its own: it sent `content-type: application/json` on body-less requests, which
Fastify answers with 400. After the fix above, it was re-run from a clean state.

Clean-up: the API was stopped by its PID (`api 2482953 stopped`, port 3100 free), `pg-test.sh drop w1` reported
"nothing named vrx_w1 / vrx_w1 remains", the 6 Valkey keys `w1web2:*` in db 1 were deleted (0 left), and
`/run/vrx-test/w1/web2` was removed. No process of this task is left running, and no daemon or VPP object was touched.

## 5. Out of scope
- Moving P08 interfaces and Users onto the kit (their files are not WEB-2's; see Q4).
- Router/nav registration of the Secrets page and previews (no `// web: WEB-2` anchor; see Q2), and with it an E2E browser flow and a `docs/user/` page for Secrets (Q2).
- WEB-1 widgets: WEB-1 is not merged (soft dependency). `localizeSchema` and `dropPhantomOptionals` step aside when WEB-1's per-path translation and presence toggle land.
- Any API or contract change. The `version` in the secrets list is a candidate (Q5).

## 6. Questions
See `WEB-2-questions.md`: Q1 CI contract-guard flake (`pipefail` + `grep -q`), Q2 wiring lines, Q3 preview strings
in the production bundle, Q4 migration follow-up, Q5 `version` missing from `GET /api/v1/secrets`.

## 7. Fix round 1 (review `edd2ea3`, APPROVE WITH CHANGES)

**Rebase.** Step 0 of the fix envelope (`rebase --onto 998e391 abb6950 task/WEB-2`) was denied by the permission
system in this round. P08 has since merged to main as `c2ca3ed` (D-112: squash + rebase). All fixes below touch only
files WEB-2 owns and apply cleanly on the current (unrebased) base; the rebase onto main is left to the merger, who
already rebases and squashes every branch at merge time (D-112).

### H1 (required before merge): fixed
`config/collection/queries.ts` `nodeKeys` no longer reuses `qk.candidate(domain)` / `['config','running',domain]`
verbatim: both now carry a trailing `'node'` segment — `[...qk.candidate(domain), 'node']` and
`['config','running',domain,'node']` — so the kit's cached whole-node object is never stored under the exact key
`pages/UsersPage.tsx` and the pending-change bar (`config/effective.ts`) use for `.users`. The shell's `['config']`
invalidation, and any `qk.candidate(domain)` / `['config','running',domain]` prefix invalidation, still reach the new
key (TanStack's default prefix matching). The doc comment above `nodeKeys` and the `queries.ts` row of §1 above are
corrected to say so.

Regression test: `config/collection/collection.test.tsx` › "does not collide with the key Users and the
pending-change bar use for the same domain (review H1)" — mounts the kit's `UsersScreen` over `management` with the
app's real `staleTime: 5_000`, waits for it to have fetched and cached both nodes, navigates (via `router.navigate`,
no remount) to a second route rendering the real `UsersPage`, and asserts it renders instead of crashing. Verified by
hand that this test reproduces probe P5 exactly against the pre-fix `nodeKeys` (`TypeError: running.map is not a
function` at `UsersPage.tsx:105`, the same line and message the review found) and passes again with the fix; the
revert was never committed.

### M1: fixed
`locales/{en,fa}/config.json`, `kit.secrets.intro` and `kit.secrets.rotateNote`: reworded in en and fa to say the
value is stored at once, outside the candidate/commit flow, and that a service using it switches to the new value
only the next time its configuration is applied (next commit) — not "take effect immediately" / "replaces … at
once". The rollback sentence of `rotateNote` is unchanged. Neither string has interpolation variables, so
`locales.test.ts` needed no change.

### M2: fixed
`pages/SecretsPage.tsx`: the value field is no longer `type=password`. A single-line value is now `type=text` masked
with `-webkit-text-security: disc` (`MASK_STYLE`) — Chromium/WebKit only; Firefox and other engines show it in clear
text, since there is no cross-browser masked plain-text input — plus `autoComplete="off"` and the
1Password/LastPass-Bitwarden/generic "ignore this field" data attributes. The dialog's `<Box component="form"
onSubmit>` is now a plain `<Box>`: the submit button calls `put.mutate` from `onClick`, and Enter on the name field or
a single-line value field calls the same `submit()` (a multi-line PEM field keeps Enter as a newline, as it would
inside a real form too) — so there is no submitted form left for a password manager's save heuristic to key off.
`SecretsPage.test.tsx` now asserts `type="text"`, `autocomplete="off"`, `data-lpignore="true"` and
`input.closest('form') === null` in place of the old `type=password` / `autocomplete=new-password` assertions. The
masking itself is a CSS paint and cannot be shown in jsdom (noted in a comment above `MASK_STYLE`); Chromium
verification of the "no save prompt" behaviour stays with the Secrets E2E step (Q2), as the review says.

### L3: fixed
The kit's 9 preview strings (`kit.preview.*`) moved out of `config.json` (always in the production bundle) into
`locales/{en,fa}/dev.json` as `kitPreview.*` (the `dev` namespace, loaded only with `DEV_ROUTES`; granted for this fix
round). `KitPreview.tsx` now reads `useTranslation(['config', 'dev'])` and prefixes the moved keys `dev:kitPreview.*`;
`pages/dev/previews.ts`'s `labelKey` for the config-kit preview (`'config:kit.preview.title'` → `'dev:kitPreview.title'`)
follows the same rename. Text is unchanged, so `widgets.test.tsx`'s preview assertions (en heading, fa RTL text)
needed no changes.

### L1: fixed
`CollectionView.tsx`'s drawer `title` no longer shows the raw `id` for a compound list key (`["default","10.9.0.0/16"]`
for `routing.static`, probe P3): it now reads `rowOf(collection, open.id)?.label`, falling back to
`keyLabel(collection.model, open.id)` when the row isn't found — the review's suggested fix, verbatim.

### L2: fixed
`LocalDataGrid.tsx` gives each mount its own `useId()`, included in the grid's `queryKey`
(`[...gridKey, inst, version]`). Before, two mounts of the same grid within the app's 5 s staleTime (Secrets →
another page → back) could share a `[...gridKey, version]` cache entry and show the other mount's page.

### L5: not done — an ownership boundary, not a 15-minute fix
Turning `SecretsPage.tsx`'s row actions into a `type: 'actions'` / `GridActionsCellItem` column needs
`packages/ui-kit/src/data-grid/index.ts` to re-export `GridActionsCellItem` (today it re-exports only
`ServerDataGrid`, `GridColDef`, `GridRenderCellParams`, `GridValidRowModel`, `GridSortModel`); no apps/web screen
imports `@mui/x-data-grid` directly, and WEB-2 owns `apps/web/src/config/{collection,widgets}/**` and the Secrets
page, not `packages/ui-kit/**` (envelope: "never: … edit files you do not own"). Left as-is: the two `IconButton`s
keep their `Why`-wrapped disabled-reason tooltip and are reachable by Tab; only the in-grid keyboard path (arrow to
the cell, Enter) is still missing.

### M3: recorded as tech debt, not fixed now (per the review and the fix envelope)
`config/collection/queries.ts` (`usePatchDomain`'s PATCH body as a mutation variable, kept in the `MutationCache` for
`gcTime`) and `useCollection.ts` (`opened`/`cleaned` kept in React state, `cleaned` handed to `onSaved`) carry a
`writeOnly` member (e.g. `management.users[].passwordHash`, per `packages/schema/src/secrets.ts`) through the query
cache and component state for any kit collection that has one — same gap as `UsersPage.tsx` already has, just no
longer hand-written. **Tech debt, before either lands:** a kit screen over a collection with a `writeOnly` member, or
the Q4 Users migration. Fix then: build the PATCH body inside `mutationFn` from a ref (mutation variables become
`{}`), set `gcTime: 0` and call `reset()` on settle, strip `writeOnly` members from `opened`, and pass `onSaved` only
the names of edited secret members, never the value.

### Not touched this round
L4 (list the kit's other deliberate differences in §3) and L6 (the three nits) are outside this fix round's scope
(H1, M1, M2, L1-L3/L5 if quick, M3 as a note).

### Verification
```
$ cd apps/web && npx vitest run src/config src/pages/SecretsPage.test.tsx src/locales
 Test Files  8 passed (8)
      Tests  67 passed (67)          # 66 before this round + the new H1 regression test

$ cd apps/web && pnpm test          # whole web suite
 Test Files  18 passed (18)
      Tests  140 passed (140)        # 139 before this round + 1

$ cd apps/web && npx tsc -p tsconfig.json --noEmit && pnpm lint
TSC-OK
check-logical-css: OK (135 files, no physical left/right CSS)
LINT-OK
```
`TMPDIR=/tmp/g-WEB-2 tools/ci.sh --base main` was run and failed at the contract guard, as expected given the rebase
above was not done:
```
== contract guard: HEAD vs main ==
contract files changed in HEAD since main:
  apps/agent/gen/vrx/v1/dataplane.pb.go · apps/agent/gen/vrx/v1/dataplane_grpc.pb.go ·
  packages/api-client/src/generated/schema.d.ts · packages/proto/gen/ts/vrx/v1/dataplane.ts ·
  packages/proto/vrx/v1/dataplane.proto
CI GATE FAILED — CONTRACT FILES CHANGED WITHOUT A CONTRACT COMMIT.
```
This is base drift, not a regression from this fix round: `task/WEB-2` is still built on the old `main` plus P08's own
(unsquashed) commits, which carried these same contract files with their own `contract(…):` commits (§4.2 of this
document shows the gate passing against that base). `main` now has P08 squashed into one merge commit (`c2ca3ed`,
D-112), so `git diff HEAD main -- <contract paths>` sees a different commit graph for identical content and the guard
can no longer find a `contract(…):` subject on `HEAD`'s own log. Rebasing onto `main` (left to the merger, see above)
resolves it — `task/WEB-2`'s own commits change no contract file (`git diff --stat abb6950 HEAD -- packages/schema
packages/proto apps/agent/gen packages/api-client/src/generated` is still empty). Lint, typecheck and every unit test
— which is what this fix round's changes touch — already passed cleanly above.

## 8. Follow-up: WEB-1 H1 (kit's own `dropPhantomOptionals` removed)

WEB-1 removes P08's `dropPhantomOptionals` behaviour: with the new presence toggle, it silently deleted an optional
object the user just switched on if it still held only its defaults (e.g. a DHCP client left at its defaults).
The kit (`config/collection/model.ts`) had its own copy of the same function, called from `useCollection.ts`'s
`save()` (the drawer's write side) — every kit screen would have brought the bug back.

**Fix.** Removed `dropPhantomOptionals` and the now-unused `withDefaults` import from `model.ts`; `useCollection.ts`'s
`save()` now sends the form value as-is (`value`, not a `cleaned` copy stripped of phantom optionals) into
`createMergePatch`/`nest`/`nextList`/`onSaved`. Removed the function's two unit tests (`model.test.ts`) and its
equivalence-with-P08 test (`equivalence.test.ts`, which compared the kit's copy against `p08.dropPhantomOptionals` —
gone along with the kit's copy; P08's own copy and its removal are WEB-1's file, not WEB-2's). `useItemEditor`'s
doc comment is updated to say the value goes through untouched; the presence toggle (WEB-1, in SchemaForm) now owns
whether an optional object is "on" with defaults or "off", not a guess from the value.

**Verified:** `pnpm --filter @ngfw/web test` — 18 files, 138 tests passed (140 before this change, minus the 2 removed
dead tests). `tsc --noEmit` and `pnpm lint` clean.
