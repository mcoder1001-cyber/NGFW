# TD-11b verify — fix round 1 (reviewer, 2026-09-25)

Branch `task/TD-11b` @ 010027c4. The fix-round commits are c74a0319 (M2), 4c19106d (M1) and 38591aee (L1/L5).
- **Scope:** a focused verify of my own findings from `TD-11b-review.md` (be3a57a5), plus D-133 compatibility.
- **How:** read-only. No product code was changed, and there were no host runs. The only commit is this file.

**Verdict: APPROVE** (last line). M2, M1, L1 and L5 are fixed as asked. D-133 is compatible with this branch's claim-first order. The merge
obligations below are for the manager, not for this branch.

## What I ran
```
$ cd apps/agent && go test -race -count=1 ./internal/scheduler/... ./internal/descriptors/{natcommon,interface,dfkit,df6,df2,core,af_packet,dhcp}/... ./internal/subsystems/...
ok  scheduler 1.542s · natcommon 1.138s · interface 1.245s · dfkit 1.114s · dfkit/persist 1.067s · dfkit/restarttest 1.116s
ok  df6 1.139s · df2 1.223s · df2/idempotency 1.265s · core 1.492s · af_packet 9.167s · dhcp 1.259s · subsystems 6.775s
EXIT 0   (the *test helper packages have no test files)
```
Merge simulations: `git merge-tree`, extracted to the scratchpad, nothing checked out.

| tree | result |
|---|---|
| TD-11b onto current `main` (8a633616, with TD-8), `--merge-base 998e391` | clean. It builds, and the subsystems, scheduler, agent, core and dhcp tests pass |
| TD-11b with `task/TD-11c` (50798421, D-133 journal) | **1 conflict**, in the `stores.go` import block (`"bytes"` vs `"context"`, keep both). It then builds, and the subsystems, scheduler and agent tests pass. `core` **fails** `TestInterfaceObjectsRecordNoStore`: the M1 tripwire fires as designed (see O1) |
| the same merge plus the O1 switch (scratch only) | core, subsystems and agent tests pass |

## M2 — fixed
- **One predicate:** `scheduler.IsPartialCreate(err)` (`scheduler/descriptor.go:142`) is used by both `executor.create` (`reconciler.go:935`) and
  natcommon's Create (`natcommon/descriptor.go:127`). Both copies of `isNilMeta` are gone; `grep` finds no other user.
- **Nil Meta is journaled:** a failed Create marked `PartialCreate` is journaled and made live on the marker alone, nil Meta included. The rollback calls `Delete(obj, nil)`.
- **natcommon keeps the claim on every partial** (`!had && !IsPartialCreate(err)`), so the rollback Delete proves ownership and releases it.
- **Tests:**
  - `TestPartialCreateWithNilMetaIsRolledBack`: a key-addressed object is deleted by the rollback;
  - `TestPartialCreateNilMetaDeleteNeedsMetaDegrades`: it fails loudly (DEGRADED), never a silent ROLLED_BACK;
  - `TestGenericPartialCreateNilMetaKeepsClaim`;
  - `TestIsPartialCreate` (wrapped and unwrapped errors, nil).
  - The status file pastes the pre-fix failures.
- **Nit:** the package doc at `scheduler/descriptor.go:40` still reads "(Meta with PartialCreate(err))". The `Create` contract text below it is correct.

## M1 — fixed
- **The check:**
  - `persist.Declared` (`dfkit/persist/persist.go`) finds the first value through the wrappers that declares `CheckPersistent` or `RecordsNoOwnership`.
    It returns `ErrUndeclared` when there is none, and `ErrConflictingDeclaration` when one value declares both.
  - `RequirePersistent` (`subsystems/stores.go`) reports `ErrUndeclaredDescriptors` and `ErrVolatileStores` together. `persist.Check` still walks every
    wrapped level, so a declaring wrapper cannot hide a volatile inner store.
- **Tests:**
  - `TestRegisterGuardsEveryDescriptor` asserts `Declared` for *every* descriptor that `register()` registered. This replaces the old `checked >= 7`.
    So a wave row that forgets a declaration fails its own CI.
  - `TestRequirePersistentRefusesUndeclared` covers a plain descriptor, one behind `defaultTolerant`, and the case with both findings.
  - `TestDeclared` covers wrappers, both-declared and nil.
- **The new declaration files:**
  - `af_packet/ownership.go`: `RecordsNoOwnership`. Correct: the tag is the ownership, and an untagged interface is removed again.
  - `dhcp/ownership.go`: `CheckPersistent` = `dfkit.CheckClaims`. Correct: the client claims untagged interfaces in the DF-1 store.
  - `core/ownership.go`:
    - VRF (name) and loopback (tag): `RecordsNoOwnership`. Correct.
    - route: `CheckPersistent` over the owner table (`ownertable.File`, which product passes from `ownertable.Open`). Correct: `route.go:232-290` is the only
      core user of `Owned`.
- **Is `RecordsNoOwnership` on core interface-ip / interface-ip.table correct *today*?** Yes.
  - On this branch `core.Env` has no claim store.
  - `ifaddr.go` resolves through `t.owned()`, which is tag only (`core.go:212`). Delete treats `ErrNotOwned` as gone. Neither descriptor touches `Owned`.
  - So they record nothing, and the declaration is true.
- **The tripwire:** `core/ownership_test.go:43` fails as soon as `core.Env` gains a field. I confirmed it fires on the TD-11b × TD-11c merge
  ("core.Env gained Claims").

## L1 — fixed
`IndexCache.Resolve` wraps a ctx that has no deadline with `legacyBound` (5 s) before `refresh`. A caller's deadline still wins (R2).
`TestIndexRefreshCappedWithoutDeadline` checks both cases: about 5 s without a caller deadline, about 30 s with one.

## L5 — documented
- **Where:** the "Claim-first leftovers" paragraph in the `subsystems/stores.go` header and the `claimFirst` doc (`interface/attributes.go`).
- **Content:** both state the admin-state case (a resync can set a NIC admin down that we never set), that the cost is accepted, and the tech-debt GC rule.
- **Tech-debt row:** there is none on `main` yet; the status file proposes one. The manager files it: TD-22, or a line in `docs/tech-debt.md`.

## D-133 (TD-11c's unsynced journal) is compatible with this branch's claim-first order
The key is that TD-11c's `fileClaims.setLocked` in a batch **appends the journal line with one write(2) before it returns** and only then changes memory.
A failed append returns an error and leaves memory unchanged. So every claim-first caller that goes through a batched store records the claim before its VPP call:
- natcommon → `KeyedClaims("nat")`;
- `df2.ClaimFirst` → `KeyedClaims("acl")`;
- df6 keyed → `PairClaims("df6")`. It shares `KeyedClaims("df6")`'s `fileClaims`, so it gets the journal from `OpenKeyedClaims` and the batch from `ClaimsTxn`.

On a failed claim these callers return before the VPP call. Their undo `Release` appends a delete line to the journal. A partial Create keeps its claim in the journal until the rollback's Delete releases it.

`IfaceClaims` (DF-1, dfkit, df6 per-interface, TD-11c core) is never batched: every claim is an immediate, atomic, fsync'd write.

This matches this branch's memory-after-write rule in the df2/df6 `FileClaimStore`s.

**Nit, for TD-11c:** the `KeyedClaims.Begin` doc in TD-11c still says "an agent process that dies inside a transaction loses that
transaction's keyed claims". Under D-133 that is no longer true.

## Merge obligations for the manager (not blockers for this branch)
- **O1 — TD-11b × TD-11c.**
  - Resolve the import conflict by keeping both imports.
  - The tripwire then requires `core/ownership.go` to switch interface-ip and interface-ip.table to
    `CheckPersistent() error { return persist.Require("core interface-ip[.table]: claims", d.Claims) }`, and to add `"Claims"` to the tripwire allowlist.
  - I verified this in the scratchpad: core, subsystems and agent tests pass.
  - TD-11c's core still claims through the context-less `Claim`. It is bounded by `legacyBound`, not the caller's ctx, which is acceptable; `ClaimContext` would be better.
- **O2 — TD-11b × F-kea-dhcp-relay.**
  - No conflict with `dhcp/ownership.go`. F-kea's claim-first fix, 28c98486, adds no `CheckPersistent`, so there is no duplicate method.
  - F-kea registers five undeclared descriptors: `kea.NewDescriptor` ×2, `dhcp.NewProxy`, `dhcp.NewProxyVSS` and `dhcp.NewRelay`. Whichever of the two merges second adds
    their declarations, or the guard test fails and the agent refuses to start.
  - Also drop the now-stale "(Its Create still claims after the add …)" remark in `dhcp/ownership.go:5-8`.
- **O3 — every wave row in flight.** Add an envelope addendum: "every descriptor you register through subsystems declares `CheckPersistent` (over the
  persisted Wiring store) or `RecordsNoOwnership` (TD-11b M1)". The guard test enforces it, but a row that knows it up front saves a CI round.
- **Still tech-debt** (the review's Lows, which the manager did not ask for): L2 (release only on a definite VPP rejection), L3 (join the release errors) and
  L4 (bypass marks a Create partial only after a family was changed).

**APPROVE**
