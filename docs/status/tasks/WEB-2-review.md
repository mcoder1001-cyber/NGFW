# WEB-2 review: config screen kit, data widgets, Secrets page

Reviewer: independent review agent, 2026-09-24. Branch `task/WEB-2` @ `4bca30a`, worktree `/root/ngfw-wt/WEB-2`.

**Diff base.** `task/P08` has moved since WEB-2 started. It is now the squashed commit `998e391` on top of main `7edac8c`,
so `git diff task/P08...task/WEB-2` also shows all of P08. The review covers WEB-2's own range, `abb6950..4bca30a`:
29 files, all in the envelope's owned list. The `config.json` diffs add `kit.*` keys only. No contract file is
touched (`packages/schema`, `packages/proto`, `*/gen`, `api-client/src/generated`: empty diff).

## Verdict: **APPROVE WITH CHANGES**

The Secrets page keeps the value out of React state, the query cache, the console, URLs and error toasts. The kit
derives its data from packages/schema, and nothing is routed. One High finding blocks the merge until it is fixed
(H1, about 3 lines, in WEB-2's own files). The kit's central claim, "same query keys as P08 and Users", causes a
reproducible crash of System › Users. The Medium findings are two misleading Secrets texts, a browser password-manager
risk, and secret leaves in the generic kit. M1 and M2 belong in this fix round. M3 must be done before any kit
screen with a `writeOnly` member ships, or before the Users migration (Q4).

## What I ran (worktree, no host runs, no `tools/ci.sh`, as instructed)

```
$ pnpm --filter @ngfw/web test
 Test Files  18 passed (18)
      Tests  139 passed (139)          # matches WEB-2.md §4.1
real 1m13.316s

$ cd apps/web && npx tsc -p tsconfig.json --noEmit && pnpm lint
TSC-OK
check-logical-css: OK (135 files, no physical left/right CSS)
LINT-OK
```

I also ran five throwaway jsdom probes in `apps/web/src/zz-review-web2.test.tsx`. The file was deleted afterwards and
never committed, and `git status` is clean.

```
P1 keyboard only, Secrets grid: tab trail [BUTTON "Add secret", DIV columnheader "Reference"] → ArrowDown →
   BUTTON "Open psk/branch-1" focused → Enter → drawer "Secret psk/branch-1"                     PASS (A11Y-1 holds in a real grid)
P2 400 {pointer:/value} then Cancel: value field aria-invalid; value not in query/mutation cache, console or DOM;
   the input has no name attribute                                                                PASS
P3 routing.static (compound key vrf+prefix), drawer heading: ["default","10.9.0.0/16"]           → L1
P4 LocalDataGrid remount (app staleTime 5 s), rows [gamma]: shows "alpha (STALE)"                 → L2
P5 kit screen over management → navigate to /users within 5 s:
   "Unexpected Application Error! running.map is not a function"                                 → H1
```

Merge mechanics:
- A plain merge onto the squashed P08 conflicts (`merge-tree 998e391 task/WEB-2`: `apps/cli/internal/api/operations_gen.go`,
  `docs/tech-debt.md`, both P08's).
- Replaying only WEB-2's commits is clean: `merge-tree --merge-base=abb6950 998e391 task/WEB-2` gives tree `55549fe`,
  with no conflicts.
- Use `git rebase --onto 998e391 abb6950 task/WEB-2` (or onto main once P08 is in), then re-run the web tests.

## Findings, ranked

### H1: the kit's query keys collide with Users and the pending-change bar, and cache a different shape (Users crashes)
- **Where.** `apps/web/src/config/collection/queries.ts:13-16` (`nodeKeys`) and `:23` (`fetchDomainNode` caches the whole
  domain node).
- **The collision.** The same keys hold a different shape elsewhere:
  - `pages/UsersPage.tsx:122` caches `['config','candidate'|'running','management']` as the users array (`.users`).
  - `config/effective.ts:112` does the same inside the shell's `PendingChangeBar`, which is mounted on every page and
    enabled after a password edit.
- **Failure.** Any kit screen over `management` (the tests already use `management.users`; RADIUS servers and the Q4
  Users migration are the stated next users) caches `{users,…}` under the key where Users expects `ConfigUser[]`.
  - Navigating to System › Users within the app's 5 s staleTime reuses that entry without a refetch. Probe P5 shows
    `running.map is not a function`, and the router error boundary replaces the page.
  - The reverse (Users → kit) gives the kit an array: `valueAt(array, path)` → an empty list until the next poll.
  - The pending bar's `effectiveChanges` gets an object where it expects `{username}[]`.
  - Writes are safe only because `useFreshCandidate` always refetches with the kit's own `queryFn`.
- **Fix (WEB-2's files).** Give the kit its own key space below the same prefix, for example
  `candidate: (d) => ['config','candidate',d,'node']` and `running: (d) => ['config','running',d,'node']`. The shell's
  `['config']` invalidation and any `qk.candidate(d)` prefix invalidation still reach it.
  - Correct the claim in WEB-2.md §1 (the `queries.ts` row) and in the `queries.ts` doc comment.
  - Add a test: a kit screen over `management`, then `UsersPage`, with a `QueryClient` using the app defaults
    (`staleTime: 5_000`).
  - Long term (owner of Users/effective): one node query per API path, with `select` for `.users`.

### M1: Secrets texts claim that rotation is live, but renderers resolve references only at render time
- **Where.** `locales/{en,fa}/config.json:186` (`kit.secrets.intro`: "changes here take effect immediately (no commit)")
  and `:216` (`kit.secrets.rotateNote`: "A new version replaces the current value at once").
- **Why the text is wrong.** The store row changes at once (`secrets.service.ts` `put`). Nothing triggers a re-render,
  though: `SECRET_REPLACED` is only a system event. The agent resolves `<kind>/<name>` "at Render time"
  (`apps/agent/internal/renderers/rfkit/secrets.go:13-17`).
- **Failure.** Running strongSwan/FRR/SNMP keep the old value until the next commit re-renders them. A later unrelated
  commit then applies the new value silently. An operator who rotates a compromised PSK is told it is already replaced.
- **Fix.** Reword in en and fa: "Stored at once, outside the candidate/commit flow. Services that use it switch to the
  new value the next time their configuration is applied (next commit)." Keep the rollback sentence.

### M2: the browser's password manager can capture a PSK
- **Where.** `pages/SecretsPage.tsx:71` (`type=password`), `:74` (`autoComplete: 'new-password'`) and `:287`
  (a `<form>` with a text "Name" field before the password field).
- **Risk.** The form is submitted, and it disappears after a successful fetch. Chromium and Firefox treat that as a
  sign-up/password-change submission and offer "Save password?" with username = secret name and password = the PSK.
  One click stores the secret in the browser vault, and possibly its cloud sync. That is outside rule 10's "encrypted
  at rest on the device".
- **Evidence.** This cannot be shown in jsdom. It is a design-level risk, and the E2E step (Q2) should confirm it in
  Chromium.
- **Fix.** Do not use `type=password` for store values.
  - Use a text input (and the existing textarea for PEM) masked with `WebkitTextSecurity: 'disc'`, plus
    `autoComplete="off"`, `data-1p-ignore`, `data-lpignore="true"`, `data-bwignore`, `data-form-type="other"`.
  - Drop the `<form>` element (submit from the button's `onClick`, Enter handled on the input), so the password
    heuristics see no form submission.
  - Update the test that asserts `type=password` / `new-password`.

### M3: the generic kit carries secret leaves through React state and the mutation cache
- **Where.** `config/collection/queries.ts:51`: the PATCH body is the mutation variable, kept in the `MutationCache` for
  gcTime (5 min). `useCollection.ts:145` keeps `cleaned` in `opened` React state, and `:148` hands `cleaned` to `onSaved`.
- **Failure.** For a collection with a `writeOnly` member (`management.users[].passwordHash`, the secret leaf per
  `packages/schema/src/secrets.ts:9`), the typed value stays in the query cache and React state. The envelope says
  "secrets never enter form state, query cache or logs".
  - This is no regression: `UsersPage.tsx:154-158` does the same.
  - The kit is schema-driven, though, so it should handle `writeOnly` generically, not copy the gap.
- **Fix.** Build the PATCH body inside `mutationFn` from a ref (variables = nothing), set `gcTime: 0` and `reset()` on
  settle, strip `writeOnly` members from `opened`, and pass `onSaved` the names of edited secret members, not the value.
  - If deferred: state in the kit's doc comment that `writeOnly` members must be `omit`ted, and make this a
    precondition of Q4.

### L1: a compound list key shows as raw JSON in the drawer heading
- **Where.** `config/collection/CollectionView.tsx:163`, `title={open?.id ?? …}`.
- **Failure.** `routing.static` shows `["default","10.9.0.0/16"]` (probe P3). The aria label already uses the row label.
- **Fix.** `title={open?.id != null ? (rowOf(collection, open.id)?.label ?? keyLabel(collection.model, open.id)) : t('kit.newTitle')}`.

### L2: LocalDataGrid can show a previous mount's cached page
- **Where.** `config/widgets/LocalDataGrid.tsx:18-33`. `useVersion` restarts at 0 on every mount, and the key
  `[...gridKey, version, request]` matches pages cached by an earlier mount. With the app's `staleTime: 5_000` there is
  no refetch on mount.
- **Failure.** Probe P4: a remount with rows `[gamma]` shows `alpha`. Real case: Secrets → another page → back within
  5 s shows the pre-load (empty) v0 page. It stays that way until the rows change or the window regains focus, because
  structural sharing keeps the same `rows` reference.
- **Fix.** Add a per-instance id to the key (`const inst = useId(); queryKey={[...gridKey, inst, version]}`), or let
  local grids pass `staleTime: 0` / `gcTime: 0`.

### L3 (Q3): 9 preview strings reach the production bundle
- **Where.** `locales/{en,fa}/config.json:227-237` (`kit.preview.*`). `config` is always imported by `i18n.ts`. `dev`
  is the namespace that review P07a M1 reserves for dev-only strings, loaded only with `DEV_ROUTES`.
- **Impact.** About 1 KB of harmless text, no security effect. `check-no-dev-routes.mjs` does not look for strings, so
  a deferred move will be forgotten.
- **Verdict.** Move them now. The manager grants WEB-2 `locales/{en,fa}/dev.json` (`kitPreview.*` keys only) for the
  fix round, and `KitPreview` uses `useTranslation('dev')`. Do not wait for the wiring.
- **Also.** When previews are wired (Q2), add `dev/config-kit`, `dev/secrets` and `KitPreview` to the script's
  `NEEDLES` (owned by the script's owner).

### L4: "4 deliberate differences" (WEB-2.md §3) is incomplete
The claim is otherwise verified: equivalence tests pass, and `NAME_RE` does accept `.`, which
`parentInterfaceName` (`packages/schema/src/domains/interfaces.ts:41`) rejects. Differences not listed:
- **(a) Save closes vs stays.** Users closes its dialog on success; the kit's drawer stays open with "Saved".
- **(b) Failed fresh read.** Users falls back to the cached list (up to 5 s old) and writes it
  (`UsersPage.tsx:170-175`); the kit refuses (`useCollection.ts:104-109`).
- **(c) Key clash.** The kit refuses a list add/rename onto an existing key locally; Users sends it.
- **(d) Sub-interface save.** P08's diffs against the fresh candidate (`InterfaceDrawer.tsx:140-144`), not the opened
  value, so it overwrites concurrent edits. The kit's nested collection applies N4 and warns.
- **(e) Rejected fresh read in P08.** A rejected `fresh()` in `saveInterface` is unhandled (`InterfaceDrawer.tsx:118`);
  the kit shows the failure.
- **(f) Changed elsewhere.** Only the kit warns "changed elsewhere" for lists.
- **Shared by all three.** Saving an item deleted elsewhere recreates it: lists append the whole item silently, maps
  send a partial merge patch.

All of (a)–(f) are improvements or neutral. List them in §3 so that Q4 migrates knowingly. Consider refusing a save
on an item deleted elsewhere, with a "deleted elsewhere" message.

### L5: Secrets row actions do not follow the grid's actions pattern
- **Where.** `pages/SecretsPage.tsx:126-145`: a custom `renderCell` with two `IconButton`s and no `hasFocus` handoff.
- **Failure.** Arrowing into the cell focuses the cell, and Enter does nothing. The buttons can be reached only with Tab.
  P08 uses `GridActionsCell`.
- **Fix.** `type: 'actions'` with `getActions` → `GridActionsCellItem`, keeping the disabled reason.

### L6: nits
- **LiveChip typing.** `widgets/cells.tsx:51` types `LiveChip.status` as `string`, and `KitPreview.tsx:32` lists the 5
  WS states by hand. `WsStatus` is exported by `packages/ui-kit/src/ws/client.ts:19`: type the prop with it, and use
  `satisfies readonly WsStatus[]` in the preview.
- **`createMergePatch` prototype check.** `collection/model.ts:155` still uses `k in to`, so a nested record member
  named `constructor`/`toString` is never nulled on removal. f9b6138 fixed only the key checks. Use `Object.hasOwn`.
  P08's copy has the same bug.
- **Test gaps.** Probes P1/P2 show the behaviour is right, but it is not pinned:
  - Add the in-grid keyboard path (Tab → header → ArrowDown → Enter); the tests call `.focus()` directly.
  - Add the Secrets 400 `/value` path with a console spy.
  - `document.body.innerHTML` cannot see `input.value` (a property, not an attribute). "Absent from the DOM" is
    really proven only by `input.value === ''`.

## Checked and fine
- **Secret value path.**
  - The value is read from the DOM only inside `mutationFn` (`SecretsPage.tsx:265`).
  - Mutation variables are `{kind,name,replace}` and the result is `{ref,created,version}`.
  - WEB-2's files contain no `console.*`.
  - The input has no `name`, so a native submit could not put the value in a URL; the query string carries only
    `replace`.
  - API problems carry pointer + message only (`ZodPipe`, `secrets.controller.ts` texts), so `ProblemAlert` cannot
    echo the value.
  - The 401-retry `request.clone()` is transient.
  - The input is emptied on success and unmounted on close.
- **Rule 5.** Schemas come from `schema/registry` (`domainSchemas`), keys from `propertyNames` / `x-vrx-ui.itemKey`,
  refs from `secretRef` / `SECRET_KINDS`, and API types from the generated `paths`. No hand-duplicated domain type
  (L6 aside).
- **No routed stub.** `SecretsPage` and `DEV_PREVIEWS` are imported nowhere, and `DEV_PREVIEWS` is `[]` in production.
  Main still has no `// web: WEB-2` anchor (checked on `main` and `998e391`).
- **A11Y-1.**
  - `KeyButton` is a `<button type="button">`.
  - Label-in-name holds in en and fa ("Open {{key}}", "باز کردن {{key}}").
  - Enter/Space do not leak to the grid, and the real grid keyboard path works (P1).
- **i18n / CSS.** en and fa key sets are identical (script check plus `locales.test`), the fa texts read naturally, and
  JSX has no hardcoded strings. Only logical properties are used; the drawer's `anchor="right"` is flipped by MUI in RTL.
- **Q1.** Fixed on main by `7edac8c` (D-127); WEB-2 inherits the fix on rebase.
- **Q2.** The wiring lines are right. The E2E create → rotate → delete flow and a `docs/user/` page must land with the
  route (DoD); verify M2 in Chromium there.
- **Q4.** Agreed as a follow-up, after H1 and M3.
- **Q5.** Agreed: an additive `version` in `SecretOut`.

## Required before merge
H1. In this fix round: M1, M2, L1, L2, L3 (after the manager grants `dev.json`). Before any kit screen with a secret
leaf or the Users migration: M3. The rest when convenient. Rebase with `--onto 998e391 abb6950`.
