# WEB-2 verify: fix round 1 (`3b6c06c`) against review `edd2ea3`

This is a focused check of the review's own findings only. I did not re-review the whole branch.
- **Scope:** read-only, except this file.
- **Not run:** host runs and `tools/ci.sh`.
- **Rebase:** the branch is not rebased yet; the merger does it.

## Verdict: **APPROVE**

- **Fixed and verified:** H1, M1, M2, L1, L2 and L3.
- **Deferred:** L5 is a ui-kit ownership item, and I agree it can wait. M3 is recorded as tech debt.
- **Blocking:** nothing.
- **Leftovers:** only Low items, listed below.

## What I ran (`/root/ngfw-wt/WEB-2` @ `3b6c06c`)

```
$ pnpm --filter @ngfw/web test
 ✓ CollectionView — list collection … > does not collide with the key Users and the pending-change bar use for the same domain (review H1)  8515ms
 Test Files  18 passed (18)
      Tests  140 passed (140)          # 139 + the new H1 regression test; matches WEB-2.md §7
real 1m42.5s

$ cd apps/web && npx tsc -p tsconfig.json --noEmit && pnpm lint      # on the clean tree
TSC-OK
check-logical-css: OK (135 files, no physical left/right CSS)          lint-exit=0
```

I also ran two throwaway probe files in `apps/web/src/zz-verify-{a,b}.test.tsx`. Both were deleted afterwards and never
committed, and `git status` is clean.
- One lint run happened while probe b still existed and flagged that file. The clean re-run above is the result that counts.
- **Probe b** replays the committed H1 scenario with the pre-fix keys. It swaps in the old `nodeKeys`, `useDomainNode`
  and `useFreshCandidate` through `vi.mock`, and does not edit `queries.ts`.

```
V5 (pre-fix keys)  kit cached under OLD key: {"users":[…]} → navigate /users →
                   "Unexpected Application Error! TypeError: running.map is not a function"   (crash reproduced;
                   the probe only "failed" on its own findByText matching the message several times)
V1 (fixed, reverse direction, staleTime 5 s)  Users first → kit shows "Open alice";
                   ['config','candidate','management'] is an array (Users), ['config','candidate','management','node'] an object (kit);
                   invalidateQueries(['config']) refetched the kit node; back to /users renders          PASS
V2 (L1)  routing.static drawer heading: "default 10.9.0.0/16"                                           PASS
V3 (L2)  LocalDataGrid unmount + remount within staleTime shows the new rows, not the old page          PASS
V4 (M2)  value input: type=text, no <form> anywhere in the document, style "-webkit-text-security: disc;",
         data-1p-ignore / data-lpignore / data-bwignore / data-form-type=other, autocomplete=off, no name;
         Enter in the value field → 1 POST; Enter in the name field → 1 POST; Enter in a PEM textarea → no POST;
         value absent from query/mutation cache and console                                             PASS
```

## Findings, one by one

| # | Status | Evidence |
|---|---|---|
| H1 | **Fixed** | `queries.ts:17-20` uses `[...qk.candidate(d),'node']` and `['config','running',d,'node']`. The regression test fails on the old keys (V5) and passes on the new ones (140/140). The reverse direction and prefix invalidation work (V1). No code reads data by key prefix (`getQueriesData`/`setQueriesData`: none), so the extra key segment is safe |
| M1 | **Fixed** | `kit.secrets.intro` and `rotateNote`, en and fa: "stored at once, outside the candidate/commit flow … services switch at the next apply (next commit)". This is accurate: renderers resolve refs at render time. The fa text reads naturally. Nit: the code comment at `SecretsPage.tsx:103` still says "Changes are immediate" |
| M2 | **Fixed** (assessment below) | V4. The `<form>` is gone, the value field is `type=text` with a CSS mask, the vendor ignore attributes are present, and Enter is wired by hand |
| L1 | **Fixed** | V2 |
| L2 | **Fixed** | `useId()` is in the grid key (V3) |
| L3 | **Fixed** | `config.json` has no `kit.preview.*` left. `dev.json` has the 9 `kitPreview.*` keys with en/fa parity. `KitPreview` and `previews.ts` use `dev:`, and the `dev` namespace is still gated by `DEV_ROUTES` on main's new `i18n.ts` |
| L5 | **Deferred, agreed** | apps/web has no direct `@mui/x-data-grid` dependency. `GridActionsCellItem` needs a one-line re-export in `packages/ui-kit/src/data-grid/index.ts`, which WEB-2 does not own. Correction to my review: P08 does not use `GridActionsCell` either. The buttons can still be reached with Tab. Backlog: the ui-kit owner adds the re-export, then Secrets switches to `type:'actions'` |
| M3 | **Tech debt, agreed** | Recorded in WEB-2.md §7 with the gate "before a kit screen with a `writeOnly` member or the Q4 Users migration". The manager should add it as a board/tech-debt row that gates Q4, because WEB-2 does not own `docs/tech-debt.md` |

### M2: judging the `-webkit-text-security` approach
**Sound for the risk it targets.** Browser password managers key their save/offer heuristics on `type=password`, and
form-history capture keys on form submission. With neither present, the PSK has no path into the browser vault or
autofill history. The decision is still E2E-confirmable in Chromium at the Q2 step.

Residual Low items. None of them block, since none puts the value anywhere that persists:
1. **Clear text in some browsers.** The mask is paint-only. Per the implementer's note, engines without
   `-webkit-text-security` show the value in clear text. That is exposure to someone watching the screen, not
   persistence, and it is an acceptable trade against vault capture. The PEM textarea was already clear text.
2. **Visual mask only.** Unlike a password field, a text field can be copied from, and screen readers read it out.
   Mobile keyboards may learn the words typed.
   - Cheap hardening: add `autoCorrect: 'off'` and `autoCapitalize: 'off'` to `VALUE_INPUT`.
3. **The mask is not pinned by a test.** "Cannot be unit-tested" is only half true: jsdom keeps the style attribute
   (V4). Add `expect(input.getAttribute('style')).toContain('-webkit-text-security: disc')`, and add the Enter-submit
   path (name, value, PEM no-submit) to `SecretsPage.test.tsx`.
4. **Nit.** `submitOnEnter` should skip `e.nativeEvent.isComposing`, so that Enter which confirms an IME composition
   does not submit.

## Merge note
A replay onto the current main (`0ae3559`: P08 as `c2ca3ed` plus W-seed) is conflict-free:
`merge-tree --merge-base=abb6950 main task/WEB-2` gives tree `48f8e33` with no conflict list.
- **Rebase command:** `git rebase --onto main abb6950 task/WEB-2`.
- **Expected CI result:** the ci.sh contract-guard failure before the rebase is base drift, as expected. WEB-2's own
  commits touch no contract path.
- **Anchors:** main still has no `// web: WEB-2` anchor, so Secrets and the previews stay unrouted, which is correct.
- **After the rebase:** re-run `pnpm --filter @ngfw/web test`, because main's `i18n.ts` changed (+114 lines).
