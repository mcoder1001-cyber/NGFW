# WEB-1 fix round 1: focused verification

Reviewer: the same agent that wrote `WEB-1-review.md` (595781c). 2026-09-25. Branch `task/WEB-1` @ ca594d9 (code 98fd042, main merged
at 0250955). I only read the code and ran tests; the one file I wrote is this one. My probe test files were temporary and I deleted them (`git status` clean).

**Verdict: APPROVE.** All the findings I asked to be fixed are fixed and tested. The cleared-means-absent change is correct, and it is an improvement: see §2.
Before merging, the manager should know about one coordination item. Removing `dropPhantomOptionals` from P08's `model.ts` breaks 8 in-flight branches
that import it, and WEB-2 has its own copy that would bring H1 back (§3, C1). There are also two small follow-ups (§4).

## Test runs (worktree, after building the schema, api-client and ui-kit dist files)
```
pnpm --filter @ngfw/ui-kit test   Test Files 14 passed (14)   Tests 81 passed (81)
pnpm --filter @ngfw/web test      Test Files 14 passed (14)   Tests 99 passed (99)
  ✓ src/domains/interfaces/InterfacesPage.test.tsx (8 tests)   ← incl. the new H1 drawer test
```
These match the numbers in `WEB-1.md`. I did not re-run the CI gate; the task did not ask for it.

## 1. Findings from my review

| # | status | evidence |
|---|---|---|
| H1 | **fixed** | `dropPhantomOptionals` and both of its calls are removed (`InterfaceDrawer.tsx` save and sub-interface save; `model.ts`). New drawer test: turning the switch on with only defaults saves `{'host-w1l0':{dhcpClient:{setBroadcastFlag:false}}}`, and turning it off saves `{'host-w1l0':{dhcpClient:null}}` (the merge patch deletes it). P08's "exactly the MTU" test still passes. |
| M1 | **fixed** | `DateTimeInput` remembers the value it emitted last (`emitted` ref). While the value is its own, it stays structured even when incomplete, so `unparsable` only applies to values it did not produce. Test: clear the offset, then type `+04:` — the input keeps focus and the date is unchanged. Save shows "Does not match the required format" and does not submit. Typing `30` then submits `…T18:00:00+04:30`. |
| L3 | **fixed** | `defaultOffsetFor(local)` uses `new Date('YYYY-MM-DDTHH:MM')`, which ES parses as local time, so the offset follows DST at the chosen date. `frac` is carried through edits of the offset and dropped only when the date/time changes. Unit tests with TZ=Europe/Berlin cover Dec and Jul. |
| M2 | **fixed as stated** | `prose()` = `<span dir={locale}>`. P08's fa MTU and rxMode help are now `dir="rtl"` with no `<bdi>` ancestor (test). Identifier chips, identifier table columns (`isIdentifierSchema`) and all-identifier item keys use `<bdi dir="ltr">`. The accepted trade-off is in §4-F2. |
| M3 | **fixed** | `useRowExpansion(ids, forced)` pins every row the list forced open (errors; or, for itemKey lists, an empty key). Only the user closes a row. The 3 cases I reproduced are now tests: sequence 0 → Save → type `7` keeps the same input and focus; a keyless stored row stays open while `bo` is typed; a server-pointer row stays open after clear/type/Tab. |
| L2 | **fixed, but adds a small regression** | Pasting `8000-8080` / `10.0.0.10-10.0.0.20` into either end fills both (test). See §4-F1 for the dash case. |

## 2. The extra behaviour change: "cleared" is stored as `null` and submitted as absent

**The bug it fixes is real, and it was worse than the worker described.** I probed the *pre-fix* ui-kit (P07 behaviour, run on a scratch copy of main):
```
PROBE pre-fix {"afterClear":"eth0","afterType":"eth0wan","mtuShown":"1500","submitted":{"name":"eth0wan"}}
```
When a pre-filled field was cleared, react-hook-form's `useController` showed the mount-time default again, and typing appended to it. For MTU,
the input still **showed 1500 while the form value was `undefined`**, and Save sent the interface without `mtu`. So in P08, "clear MTU" looked like a
no-op but deleted the MTU. The fix makes what is shown and what is submitted the same.

**Is "absent" correct for every consumer?**
- **P08 interfaces** (merge patch against the value the form opened with). If the user clears MTU, it is absent, and `createMergePatch` turns that into `mtu: null`, so the
  driver default applies, as the help text says. Untouched fields are identical to the base, so they produce no patch entry. Secrets are never in the base, so they are never sent unless typed.
- **P07b Users** (PATCH of `management.users` with the edited item replaced). A cleared optional field (`fullName`) is removed. An empty `passwordHash` stays absent,
  and the server keeps the password, as the help text says. Required fields that are cleared fail validation (`validation.required`) and are never dropped silently.
- **F-\* screens in flight.** I read their save paths. F-acl/ObjectsPage (`mergePatch(current, v)`), F-bonding, F-bridge-l2 and the P08 copies all diff against the
  stored item, or replace the whole item or list. For those, absent means "remove", which is what the user asked for by clearing. A screen that sent the raw form value as an RFC 7386
  patch *without* diffing would silently ignore a clear, but it would not delete anything. I found none that does.
- **Record values** (for example a record of strings). A cleared value becomes `{key: undefined}`, and Zod rejects it (an error on that field). The entry is not dropped.
- **Fields with a schema `default`.** A cleared field submits the default, because the parsed JSON reaches `onSubmit`. "Empty = default" is intended. Showing the default as a placeholder would make that visible (nit).

**Can "absent" delete a value the user did not touch?** Only an `onChange` from a widget can create `null`. I checked every widget reached through `PrimitiveField`:
- text, number, slider, boolean, select, radio, range, time, datetime, colour, suggest (MUI skips unchanged values; `autoSelect` on blur does nothing when the input is empty): these fire only on user input.
- `JsonInput` commits on blur, but it only emits `undefined` when its text is already empty, which means the value was already absent.
- `dependsOn` treats `null` like `undefined` (`dependencyMet`), and the presence and variant logic work on objects, not on primitives set to `null`.
- A stored `null` in a field that does not accept null would become absent. The server's schema never stores such a value, and against a merge-patch base it would only "delete" something that is already null.

So no untouched value is deleted. One edge case, with no current instance: a `json` widget on a schema that accepts null turns absent into `null` when the user tabs through it. No field in `packages/schema` is nullable, and none uses the `json` widget.

## 3. C1 — coordinate before merge: `dropPhantomOptionals` is used outside P08
In-flight branches import `dropPhantomOptionals` from `domains/interfaces/model.ts` for **new** screens:
- F-bonding (`BondDrawer`)
- F-bridge-l2 (`BridgeDomainDrawer`, `RecordDialog`)
- F-loopback-bvi-gso-lldp-span (`LldpPage`, `NsimPage`, bridge-l2 files)
- F-kea-dhcp-relay (`ConfigTabs`)
- F-nat44-ed-sessions (`ListSection`, `OutboundTab`)
- F-nat44-ei-64-66-nptv6 (`ListSection`, `OutboundTab`, `SubtreeForm`)
- F-neighbors-ra (`StaticNeighborsForm`)

**WEB-2 keeps its own copy** in `config/collection/model.ts`, and `useCollection.ts:102` calls it.

After WEB-1 merges:
- the 7 importing branches stop compiling at rebase;
- anyone who "fixes" that by copying the function back, and every WEB-2 collection screen, gets H1 again: an optional object switched on with only its defaults is deleted silently.

Recommended, one of these:
- (a) Keep `export function dropPhantomOptionals(_schema: JsonSchema, _before: unknown, after: unknown): unknown { return after; }` in `interfaces/model.ts`, marked
  `@deprecated` (WEB-1 H1: the presence switch makes phantoms impossible). This is correct behaviour for every caller and breaks no rebase. Remove it in a later TD.
- (b) Tell each branch above to delete its calls at rebase.

In both cases, **WEB-2 must drop or neutralise its copy** before it merges. A test like the new H1 drawer test (switch on, defaults only → the object is sent) would catch this on any screen.

## 4. Follow-ups (not blocking)
- **F1 — L2 regression: typing a dash wipes the other end.**
  - Where: `widgets.tsx` `RangeInput.typed()`.
  - What happens (probe, stored `8000-8080`): typing `-` at the end of From gives From `8000`, To `''`, so `8080` is lost. Typing `-` after a To of `9000` moves
    `9000` into From and clears To.
  - It is visible at once, so nothing is lost silently.
  - Fix: treat the input as a pasted range only when both parts from `splitRange` are non-empty; otherwise strip the `-` as before.
- **F2 — M2 trade-off: untranslated English help is misordered in fa.** `prose()` always uses the locale direction, so in fa an untranslated English help that starts with a number
  is laid out RTL again. "443 or 8000-8080" shows as "or 8000-8080 443" (screenshot 03's first-round fix is gone, as `WEB-1.md` says). Most
  `packages/schema` help has no fa key yet. Better rule: text that contains an RTL-script character follows the locale; text with none gets `<bdi dir="ltr">`. That keeps
  P08's Persian help correct and fixes the English-only texts.
- The review items listed as "left for later" in `WEB-1.md` (L1, L4–L7, nits) are still open. That is fine as follow-ups.
