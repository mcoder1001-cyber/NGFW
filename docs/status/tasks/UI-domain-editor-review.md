# UI-domain-editor — review (6.2, D-125)

Branch `task/UI-domain-editor` @ `50d986b6`, base `task/WEB-1@c2e3eabb` (speculative, D-114). Diff reviewed:
`git -C /root/ngfw-wt/UI-domain-editor diff c2e3eabb HEAD` (16 files, +1523/-7). CI already green
(`tools/ci.sh --base main`, pasted in `UI-domain-editor.md`) — not rerun here; unit tests/lint/typecheck rerun
independently (below).

## Verdict: **APPROVE WITH CHANGES**

Nothing here blocks the speculative merge once WEB-1 lands. One follow-up (a small `packages/api-client` helper
for multi-segment addressing, D-UDE-1) should be opened as its own task before this pattern is copied into a third
or fourth domain screen; it does not need to happen before this branch merges. No product-code changes requested.

---

## 1. Architecture — PASS

- Form + tree are schema-derived only. `apps/web/src/schema/registry.ts` builds `ROOT_KEYS`/`domainSchemas`/`domains`
  from `@ngfw/schema`'s `RootConfig` (registry.ts:1,14-17,37); `schemaPath.ts` walks that JSON Schema with
  `@ngfw/ui-kit/schema-form`'s own `resolveRef`/`mergeAllOf`/`isRecordSchema`/`recordValueSchema` — no hand-written
  API types, no hand-written domain list. Confirmed: `parseConfigPath` never falls back to a literal list
  (schemaPath.ts:23-28), and the "unknown domain" page lists `domains` (AdvancedEditorPage.tsx:108-114).
- Writes are merge patches through the generic candidate routes: `usePatchDomain` → `PATCH /api/v1/config/{path}`
  (queries.ts:26-32); every mutation in `AdvancedEditorPage.tsx` (`save`, `removeAt`, lines 152-163) goes through it.
  `useDiff`/`useConfirmState` (pre-existing, untouched) still drive the pending-change bar; `patch.onSettled`
  invalidates both the shared config cache and this domain's own cache (queries.ts:30).
- `notApplied` and `agent.unsupported-field` warnings for the subtree: `subtree.ts`'s `notAppliedForDomain` /
  `inSubtree`, wired at AdvancedEditorPage.tsx:139-142, rendered at :198-217. Verified against
  `subtree.test.ts` (ancestor/descendant/exact pointer matching, 5 tests, all pass).
- Secrets: this task adds no secret-specific code; it inherits the existing pipeline (API redacts running/candidate
  documents before they leave the server — `apps/api/src/config/config.controller.ts:167` "document that leaves
  here is redacted"; `SchemaForm` already renders `writeOnly`/`x-vrx-ui.secret` fields as write-only,
  `packages/ui-kit/src/schema-form/summary.ts:27`). `AdvancedEditorPage` never dumps a raw node value anywhere
  except into `<SchemaForm value>` and the client-side diff computation — both already redacted at the source.
  No echo path found.
- `dropPhantomOptionals`: zero occurrences in the diff (grepped `apps/web/src/domains/advanced` +
  `DomainPlaceholderPage.tsx`).
- Timers: no `refetchInterval`/`setInterval`/`setTimeout` anywhere in the new files; `useDomainCandidate` is
  explicit about it (queries.ts:14, citing D-132) and the page uses a manual Refresh button
  (AdvancedEditorPage.tsx:192-194). One paperwork note, not a code issue: **D-132 is cited in the envelope and in
  this task's own docs but there is no `D-132` row in `docs/decisions/LOG.md`** (grepped the whole log). Not this
  worker's problem to fix and not blocking, but the manager should either add the missing LOG row or correct the
  citation — a decision number that resolves to nothing is a paper cut for the next task that cites it.

## 2. D-UDE-1 — generic pointer route, one URL segment only (the main risk)

**Confirmed exactly as described, with one correction that lowers the severity.**

- Reproduced the client-side limit directly: `apps/web/node_modules/.vite/deps/openapi-fetch.js`'s
  `defaultPathSerializer` calls plain `encodeURIComponent(value)` on a path-param value with no `allowReserved`
  escape hatch (there is no per-call override surfaced by the generated `paths['/api/v1/config/{path}']` type
  either). So the **generated TS client** can only ever address one raw URL segment.
- **Correction to the worker's writeup:** the *server* is not actually limited to one segment. `ConfigController`
  registers the generic routes as NestJS wildcards — `@Get('*') / @Patch('*') / @Put('*') / @Delete('*')`
  (`apps/api/src/config/config.controller.ts:442,452,463,475`) — and `pointerFromUrl`
  (`apps/api/src/config/path.ts:11-38`) manually splits the **raw URL** on `/` into as many segments as are
  present, decoding and pointer-escaping each one; its own doc comment describes exactly the two ways to address a
  name containing a literal `/`. A real request to `PATCH /api/v1/config/interfaces/eth0/mtu` (three literal URL
  segments) already works today, end to end, with no server change. So:
  - **Option "a path-array route in the API contract" (a `contract(...)` row) is not needed.** The contract
    already supports arbitrary depth; only the generated client's path templating does not expose it.
  - The real fix is entirely client-side: either (1) a small hand-written helper in `packages/api-client` (not
    generated) that builds the raw URL string itself and calls the underlying `fetch` used by `openapi-fetch`,
    bypassing its path templating for this one route family, or (2) a `pathSerializer` override scoped to this
    operation if `openapi-fetch`'s config surface allows one per-path. Either is small, additive, and does not
    touch `packages/schema` or the OpenAPI document — no `contract` branch required.
  - The worker's own two other options (query-parameter JSON pointer; client-side subtree PATCH at the domain
    root) are valid too, but the client-side-subtree choice actually taken is the right one for *this* task given
    the fix above is out of scope here.
- **Verified what actually works today through this branch's code**, by tracing the primitives rather than
  standing up the stack (screenshots are blocked separately per D-UDE-4 and are explicitly out of scope for this
  review):
  - `/config/interfaces` — domain root, record: covered by `AdvancedEditorPage.test.tsx` "lists the candidate
    keys, deletes one as a merge patch…" — PATCH body observed as `{ eth0: null }`. Works.
  - `/config/interfaces/eth0` — one segment deep, record item: covered by the same file's "saves a field as a
    merge patch scoped to the domain…" — PATCH body `{ eth0: { mtu: 1400 } }`, then `{ eth0: null }` for remove.
    Works.
  - `/config/system/banner` and deeper (`schemaPath.test.ts`'s `resolveNode` case for
    `['eth0','subinterfaces','100']`, three segments) — not screen-tested at that exact depth, but
    `resolveNode`/`valueAt`/`wrapAtPath`/`withoutChildValues` are all depth-generic (they recurse on `segments`
    with no special-casing below depth 1) and are unit-tested at depth 2-3 directly (schemaPath.test.ts:61-81,
    122-137). By construction (`AdvancedEditorPage` never branches on `segments.length` except for the
    breadcrumb/back-button), a save at any depth produces one `PATCH` of the domain root with the edit nested at
    the right place. I'm satisfied this generalizes; the only gap is a screen-level test that exercises it, which
    would be a cheap addition (see Tests section).
- **Severity: Medium, not blocking.** The generic editor **does** functionally address any depth end-to-end (the
  scope is met), but every write to a deep node fetches/patches the *whole domain*, which is a real concern for a
  domain that grows large (a routing table, a big ACL) — exactly the case the worker flagged for a follow-up. Not
  a correctness bug (each subtree's own merge patch is computed and wrapped narrowly, so concurrent edits to
  different subtrees of the same domain do not clobber each other — verified by reading `createMergePatch`'s
  scoping in `save()`, AdvancedEditorPage.tsx:152-158), just a scalability/efficiency debt. Recommend a follow-up
  task (not a `contract` branch) for the `packages/api-client` helper described above, opened before this
  generic editor is pointed at a domain expected to be large.

## 3. D-UDE-2 — no wave-A anchor, hunks added directly

Confirmed no `// wave-A: UI-domain-editor` anchor existed before this task (checked `git log -p` on `router.tsx`/
`i18n.ts` from W-seed's squash forward; W-seed only seeded anchors for the wave-A hotspot list, and this task is
not on it). The two hunks are minimal and isolated:
- `router.tsx`: one new route object (+5 lines) inserted right after the per-domain `DomainPlaceholderPage` map,
  before any `// wave-A: F-*` marker — it does not touch or reorder any existing anchor.
- `i18n.ts`: two-line imports + two one-line entries in `NAMESPACES`/`en`/`fa`, placed with the other
  already-built namespaces (`interfaces`/`services`/`vpn`), not inside the reserved per-feature anchor lists.

Checked against WEB-1 (the base, already includes these files unchanged at this point) and against the currently
running WEB-2 branch (`apps/web/src/config/**`, `SecretsPage.tsx`, `locales/*/config.json` — disjoint files) and
the queued wave-A `F-*` rows (all reserved anchor lines, none touched here). **Low conflict risk**; both hunks are
easy to re-target if the manager wants a different shape, as the worker already noted.

**Worth flagging (not a merge conflict, an architecture-overlap note):** WEB-2's title is "Config screen kit
(generic list+drawer+live-status over **any candidate path**) + data widgets + Secrets page" — conceptually
adjacent to this task's "generic editor for any domain path." They don't collide at merge (disjoint files,
disjoint routes — WEB-2's kit is unrouted so far), but the programme now has two independently-built "generic path"
mechanisms in `apps/web`. Recommend the manager have someone reconcile them at the next contract/kit pass (see next
section) rather than let a third one appear.

## 4. D-UDE-3 — createMergePatch triplication

Confirmed three copies, not two: `apps/web/src/domains/interfaces/model.ts:67-82` (existing),
`apps/web/src/domains/advanced/schemaPath.ts:141-156` (this task), and — checked via
`git log --oneline --all -S"function createMergePatch"` — `apps/web/src/config/collection/model.ts` on the
currently-running `task/WEB-2` branch (its own independent copy, with its own "gives the same patches as P08"
parity test). All three are byte-for-byte the same RFC 7386 diff algorithm.

**Recommendation: `packages/schema`, not `packages/ui-kit`.** `packages/schema` already ships the *apply* side of
the same RFC 7386 family (`mergePatch`/`mergePatchAt`, `packages/schema/src/merge-patch.ts`), is pure (no React/MUI
dependency), and is already a dependency of every place that needs this (`schemaPath.ts` already imports
`jsonPointer` from `@ngfw/schema`; `interfaces/model.ts` and WEB-2's kit both live in `apps/web` which already
depends on `@ngfw/schema`). `packages/ui-kit` is scoped to rendering (theme, `SchemaForm`, DataGrid) and has no
existing reason to own a pure diff algorithm. Putting compute (`createMergePatch`) next to apply
(`mergePatch`/`mergePatchAt`) in the same package also reads naturally — one file, one RFC, both directions.
Correctly deferred by the worker to a future `contract(schema)` task (00-CONTEXT rule 3); the manager should note
it now touches three branches (main, WEB-2, this one) so the follow-up's diff will need three call-site updates,
not two.

## 5. Tests — PASS, independently reproduced

Reran everything myself in the worktree (not just re-reading the pasted output):

```
$ TMPDIR=/tmp/g-ude-review pnpm --filter @ngfw/web test
 Test Files  17 passed (17)
      Tests  136 passed (136)
$ npx eslint src && node scripts/check-logical-css.mjs
check-logical-css: OK (132 files, no physical left/right CSS)
$ npx tsc -p tsconfig.json --noEmit
(clean)
```
`packages/ui-kit/dist` and `packages/api-client/dist` were already built (fresh, from the worker's own CI run this
morning, `packages/ui-kit/dist/index.js` mtime 09:37, `api-client/dist/index.js` mtime 09:39); reused them rather
than rebuilding, so no dist folders were created or removed by this review.

The 35 new tests (24 `schemaPath.test.ts` + 5 `subtree.test.ts` + 6 `AdvancedEditorPage.test.tsx`) do cover what the
proof line claims: path parsing (`parseConfigPath`/`configPathTo`), schema walk incl. `$ref`/record/array segments,
subtree PATCH-as-merge-patch and DELETE-as-null-merge-patch at both the domain root and one level down,
`agent.unsupported-field`/notApplied scoping (`inSubtree`, ancestor+descendant+exact), en/fa key parity (via the
existing `locales.test.ts`, which discovers namespaces by directory listing) and an RTL render test. One real gap:
no test exercises a save/delete at **two or more** segments deep at the screen level (only `resolveNode` is unit
tested that deep, not `AdvancedEditorPage`'s save/removeAt through a nested chip navigation) — cheap to add, not
blocking given the primitives are depth-generic and unit-tested at depth 2-3 (see §2).

The two bugs-caught-by-tests the worker described (unprojected value rejected as "Unknown field"; schema/value
identity churn resetting the form) check out by reading `withoutChildValues` (schemaPath.ts:104-109) and the
`useMemo`-wrapped `childKeys`/`formSchema` (AdvancedEditorPage.tsx:149-150) — real fixes, not incidental.

## 6. Self-reported rule slips — verified, no side effects

- `pkill -f "vitest run src/domains/advanced/..."`: confirmed no matching process running now
  (`ps aux | grep -i "vitest run src/domains/advanced"` — empty). Pattern was specific to the worker's own
  just-started command; disclosed, not repeated (later waits used `run_in_background`/exact-PID kills per its own
  account). One-time rule break, no evidence of collateral damage on the shared host.
- `git status` (no `-C`) run once in `/root/ngfw`: confirmed `/root/ngfw`'s working tree is clean now (`git -C
  /root/ngfw status --porcelain` → empty) and `git -C /root/ngfw log -3` shows only manager/other-session commits,
  nothing from this task. Read-only command, no write, no lasting effect — but still a rule break exactly as
  described; noting it here per the envelope, not asking for anything further.
- Screenshot-attempt teardown independently reverified: `ss -ltn` shows neither 4100 nor 6100 listening,
  `/run/vrx-test/w11` does not exist, no `vrx_w11` database present. Matches the claimed teardown.

## 7. Hands-off files — untouched, confirmed

`git diff c2e3eabb HEAD --name-only` touches only files under the task's ownership
(`apps/web/src/domains/advanced/**`, `DomainPlaceholderPage.tsx`, `locales/{en,fa}/advanced.json`, the two
router/i18n hunks, the new e2e script, and `docs/status/tasks/UI-domain-editor*`). No hits for `BUILT_DOMAINS` or
`NavItem.available` in the diff; `nav.test.ts` and `packages/ui-kit` do not appear in the changed-files list at
all.

---

## Findings summary

| # | Finding | Location | Severity |
|---|---|---|---|
| 1 | Generic pointer route only reachable at one URL segment via the generated client; server already supports full depth (`@Get('*')` etc.) so the real fix is a small `packages/api-client` helper, not a contract change | `apps/web/node_modules/.vite/deps/openapi-fetch.js` (`defaultPathSerializer`), `apps/api/src/config/config.controller.ts:442-478`, `schemaPath.ts` (whole file) | Medium — follow-up task, not blocking |
| 2 | `D-132` cited in envelope/status docs but has no row in `docs/decisions/LOG.md` | `docs/decisions/LOG.md` (missing), cited at `apps/web/src/domains/advanced/queries.ts:14` | Low — paperwork, manager to add or correct |
| 3 | `createMergePatch` now exists in 3 places (main, this branch, WEB-2); recommend hoisting to `packages/schema` alongside `mergePatch`/`mergePatchAt` | `apps/web/src/domains/interfaces/model.ts:67-82`, `apps/web/src/domains/advanced/schemaPath.ts:141-156`, `apps/web/src/config/collection/model.ts` (task/WEB-2) | Low/Medium — flagged for a `contract(schema)` follow-up touching 3 branches |
| 4 | WEB-2's "config screen kit ... over any candidate path" and this task's generic advanced editor are conceptually overlapping generic mechanisms built independently | `apps/web/src/config/collection/**` (task/WEB-2) vs `apps/web/src/domains/advanced/**` | Informational — no file/merge conflict, but worth reconciling before a third one appears |
| 5 | No screen-level test exercises a save/delete two or more segments deep (only unit-tested at the primitive level) | `apps/web/src/domains/advanced/AdvancedEditorPage.test.tsx` | Low — cheap to add, not blocking |

No blocking findings. Recommend **APPROVE WITH CHANGES**: merge as-is once WEB-1 lands (per D-114's speculative-start
rule); open the `packages/api-client` helper and the `createMergePatch`/`packages/schema` hoist as separate
follow-up tasks; the manager adds or corrects the `D-132` LOG row.
