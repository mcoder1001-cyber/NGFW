# TD-11b review — claim safety (reviewer, 2026-09-24)

Branch `task/TD-11b` @ 3b1931d, base task/P08@998e391, diff `git diff 998e391...task/TD-11b` (24 agent files, +2 329/−81).
Architecture review as the manager asked: seven focus items, Q2 and Q3. No product code was changed, and there were no host runs. The only
commit is this file.

**Verdict: APPROVE WITH CHANGES** (last line).

The core of the branch is sound:
- **Claim before the write, with an undo:** DF-1, natcommon and df6 keyed.
- **Partial Creates are an explicit opt-in:** Q2's deviation is right.
- **`PairClaims` for df6:** a real latent bug is found and fixed. Every df6 keyed claim would have failed through `IfaceClaims`, and failed *after* the add.
- **The guard is wired at the one registration point.**

Two small in-branch changes are needed before merge (M1, M2). One decision is for the manager (M3). The rest are Low items for this
branch or for tech-debt.

## What I ran
```
$ cd apps/agent && go test -race -count=1 ./internal/scheduler/... ./internal/descriptors/{natcommon,interface,dfkit,df6,df2}/... ./internal/subsystems/...
ok   ngfw/agent/internal/scheduler                         1.519s
ok   ngfw/agent/internal/descriptors/natcommon             1.217s
ok   ngfw/agent/internal/descriptors/interface             1.251s
ok   ngfw/agent/internal/descriptors/dfkit                 1.148s
ok   ngfw/agent/internal/descriptors/dfkit/persist         1.071s
ok   ngfw/agent/internal/descriptors/dfkit/restarttest     1.154s
ok   ngfw/agent/internal/descriptors/df6                   1.166s
ok   ngfw/agent/internal/descriptors/df2                   1.143s
ok   ngfw/agent/internal/descriptors/df2/idempotency       1.109s
ok   ngfw/agent/internal/subsystems                        6.732s      (EXIT 0; the *test helper packages have no test files)
$ go vet ./... && go test -count=1 ./...        → VET 0, TEST 0 (whole agent module, unit mode)
```
- **Base failure reproduced:** I put the new DF-1 tests on an export of 998e391, trimmed to the part that compiles there.
  - `TestAttributeClaimsBeforeWrite` fails for all six attributes ("sw_interface_set_* sent 1 time(s) although the claim failed").
  - `TestAttributeClaimUsesCallerContext` fails too.
  - Both match the paste in TD-11b.md.
- **CI:** the gate evidence is at c0372fd. Since then only a doc comment in subsystems.go and the docs have changed (`git diff c0372fd 3b1931d`). I did not re-run
  `tools/ci.sh`; the manager asked for the targeted run above.
- **Merge simulations** (`git merge-tree --write-tree --merge-base 998e391`, nothing checked out):
  - TD-11b onto `main`: clean. That tree builds, and subsystems/scheduler/agent unit tests pass.
  - TD-11b with `task/TD-11c`: clean (tree 2f939f1). It builds, and subsystems/core/scheduler/agent unit tests pass.
- **Contract, provenance, security:**
  - No `packages/`, `gen/`, `binapi/` or `tools/` changes.
  - `exec.Command` appears only in the host test, with a fixed argv.
  - The host test uses the slot prefix, stops agents by PID and deletes its tap in `t.Cleanup`.

## Findings (ranked)

### M1 — the persistence guard is fail-open: a descriptor that records claims but has no `CheckPersistent` is never checked
`subsystems/stores.go:430-470` (`Register` → `RequirePersistent` → `persist.Check`) checks only descriptors that implement `persist.Checker`.

**What is covered today:**
- In product only DF-1, core, af_packet and dhcp.client are registered.
- dhcp.client shares the DF-1 store, so it is covered indirectly.

**The gap:**
- No df2 consumer (urpf/adl/abf/arp/ip6_nd/classify) implements `CheckPersistent` yet.
- No dfkit consumer does either (lldp, vrrp, qos, span, bfd, lcp, … and their boot stores). Neither does acl or core (TD-11c now gives core a claim store).
- A wave row that registers one of these under its W-seed anchor with the in-memory default will start the agent normally. That is exactly what
  the envelope says must be impossible ("no descriptor may be registered with an in-memory claim or boot store; the agent refuses to
  start otherwise").
- `TestRegisterGuardsEveryDescriptor` only asserts `checked >= 7` (`claims_td11b_test.go:141`).

**Fix (≈30 lines, in files this branch owns):** make the check complete at CI time.
- Every descriptor that `register()` registers must either implement `persist.Checker` (through wrappers) or declare that it records no
  ownership. Use a marker method in its own package, e.g. `RecordsNoOwnership()`; this avoids a shared allowlist that every row edits.
- `TestRegisterGuardsEveryDescriptor` fails on any descriptor that does neither.
- Today that means marking core (tags), af_packet (tags) and dhcp.client. dhcp.client's marker should carry a TODO pointing at Q3's `CheckPersistent`.
- Every wave row's registration runs through `register()` in the subsystems tests, so forgetting then fails the row's own CI.

### M2 — `PartialCreate` with a nil Meta is silently dropped (the one silent-orphan path left in the new mechanism)
`scheduler/reconciler.go:935` journals only when `errors.Is(err, ErrPartialCreate) && !isNilMeta(meta)`. `natcommon/descriptor.go:127` uses
the same condition. When it is false, natcommon **releases the claim**.

**Why this matters:**
- The marker alone already says "VPP was changed". The Meta test adds nothing but a silent drop.
- Some descriptors have a nil Meta by design: df6 keyed returns `nil, nil` on success, and many natcommon ops do `return nil, p.addDel…()`, e.g. nat64/nat66/det44.
- If such a descriptor returns `nil, PartialCreate(err)`, three things happen: the object stays in VPP, it is not journaled, and (in natcommon) it is unclaimed.
  That is the review 3.3 failure again, with nothing logged beyond "create failed".

**Fix:**
- Journal on the marker alone. The rollback then calls `Delete(obj, nil)`: a descriptor that needs Meta fails loudly (DEGRADED) instead of
  silently, and key-addressed ones (df6 keyed, natcommon) delete correctly.
- Export one predicate, e.g. `scheduler.IsPartialCreate(err) bool`, and use it in both places so the executor and natcommon cannot diverge.
  This also removes the duplicated `isNilMeta`.
- Add a test for the nil-Meta case.

### M3 — for the manager: TD-11c's per-transaction batching removes claim-first's crash guarantee for `KeyedClaims` and `PairClaims`
TD-11c (N3 in its questions file) batches every store in `Wiring.keyed` and flushes once at the end of the transaction. `PairClaims("df6")` is
opened through `KeyedClaims("df6")`, so it is batched too.

**What that means after both branches merge:** a claim made before the VPP write is only in memory until the end of the transaction. If
the agent *process* dies inside the transaction, the result is the same as the old write-then-claim order:
- an untagged NAT, ACL/df2 or df6 keyed object stays in VPP unclaimed until VPP restarts;
- the df6 keyed "unclaimed → ErrNotOurs forever" window that `keyed.go:167-170` says it closes comes back.

**What still holds:** IfaceClaims (DF-1, dfkit, df6 per-interface, TD-11c core) still write at once, so the guarantee holds there.

**The merge is clean in text and green in unit tests** (simulated above), so nothing will flag this.

**Needs a D-entry.** Options:
- (a) accept the window for keyed families and fix the TD-11b doc comments that promise more (`natcommon/descriptor.go:106-112`, `df6/keyed.go:167-170`);
- (b) a follow-up row for a write-ahead batch: at the start of the transaction, pre-claim the plan's Create keys in one flush, and release the unused ones in the end flush.

I recommend (a) now and (b) as tech-debt before the NAT/ACL scale tests.

### L1 — R2: a caller with no deadline now gets an unbounded refresh, with the index-cache mutex held
The problem is in `subsystems/stores.go:389-393`: `IndexCache.Resolve` holds `c.mu` across `refresh(ctx)`.

**The resync path:** the resync on every VPP connect calls `a.svc.Resync(ctx)` with the agent's run context (`agent.go:231`), and `service.go` adds no
deadline. govpp v0.13 has `core.DefaultReplyTimeout = 0` (disabled), so a VPP stall now makes the refresh wait forever. Before this branch it waited 5 s.

**The knock-on effect:** meanwhile, every context-less `Claimed` blocks on the mutex, past its own 5 s `legacyBound`. That includes every Retrieve through `Table.Owns`.

**Fix (one line):** in `Resolve`, when `ctx` has no deadline, wrap it with `legacyBound`. The caller's deadline still wins when one is set, which is the point of R2.
TD-9's planned `DefaultReplyTimeout` also closes this, but it should not depend on TD-9's merge order.

### L2 — an ambiguous write error releases the claim although the write may have landed
All the undo paths release on *any* error:
- the DF-1 `claimFirst` undo (`interface/attributes.go:99-104`);
- natcommon (`descriptor.go:128`);
- df6 keyed (`keyed.go:175-178`);
- `dfkit.Claim.Undo` (`claimfirst.go:90`);
- `df2.ClaimFirst` (`df2/claims.go:118-131`).

After a context deadline or transport error, VPP may have applied the change. Releasing then leaves it unowned for good. Keeping the claim
heals itself: Retrieve reports the value if it exists, and a resync deletes it if it is not desired.

**Fix:** release only on a definite VPP rejection (`errors.As(err, &api.VPPApiError{})`, or the retval path the family already classifies),
and keep the claim otherwise.

### L3 — release errors are discarded in three of the five undo paths
- `interface/attributes.go:102`: `_ = s.Release`;
- `df6/keyed.go:177`;
- `df2/claims.go:128`.

natcommon and `dfkit.Claim.Undo` join the release error into the returned error. Do the same in these three, so that a phantom claim left
by a failed release shows up in the error.

### L4 — df6 bypass reports a partial Create even when nothing was changed
`df6/bypass.go:233` wraps every `apply` error, including a `bootid.Current` failure and a failure of the first `ensure`. The rollback's Delete
is idempotent, so the result is correct. But on the same flaky connection that Delete can fail too, and then a transient VPP error ends as
DEGRADED instead of ROLLED_BACK.

**Fix:** return `PartialCreate` only when the first family was actually changed.

### L5 — a phantom claim from a crash between claim and write is never cleaned up within one VPP boot
**The failure scenario:**
- The crash leaves the claim; the next Create just re-uses it (had = true), so nothing is blocked forever. I checked natcommon, DF-1, df6 keyed and dfkit.
- But a claim whose key is no longer desired stays until the next boot-identity change. `Connected` → `Prune` is the only cleanup.
- For `interface.admin-state` this is visible:
  - a leftover claim on an untagged NIC that someone else set admin-up makes Retrieve report it as ours;
  - if the restarted agent's desired state does not include it, the resync Deletes it, i.e. sets the NIC **admin down**. We never wrote that state.

This is the accepted cost of review 3.3's claim-first order, and it is a smaller window than the old orphan.

**Fix:** record it (D-entry, with the Q2 LOG entry) and add a tech-debt row: after the first successful resync, GC claims whose key is neither desired nor present in VPP.

### Nits
- **TD-11c's core** (after the merge) claims through the context-less `Claim`, so it has no R2 bound, and it has no `CheckPersistent`. Whoever merges second should switch core to
  `ClaimContext`/`ClaimedContext` and add `persist.Require(…, e.Claims)` when `e.Claims != nil`. This is covered by M1 once the completeness check exists.
- **`natcommon.isNilMeta`** duplicates the scheduler's (see M2).

## The ten points the manager asked about
1. **Q2: opt-in `PartialCreate` instead of "any non-nil Meta".**
   - **Sound; I agree with the deviation.** Journaling every non-nil Meta would roll back objects that were never created.
     `core/ifaddr.go` returns `IfMeta{}` with a failed add, and `l2/fib_entry.go:92`, `l2/flags.go:157` and `bond/member.go:70` do the same with a pre-write error.
   - **Which Creates can leave something in VPP after an error:**
     - DF-1 attributes are single writes, and claim-first removes the post-write failure. L2 is the remaining edge.
     - natcommon passes the marker through. The in-tree ops I spot-checked (nat44ed interface, nat44ei range, mapnat domain) are single writes.
     - df6: bypass wraps (L4); keyed is a single add; `IfDescriptor` cleans up after itself with `TagOrRollback`/`ifsanitize.Acquire`.
     - core loopback and sub-interface also clean up after themselves.
   - **Silent-orphan paths left:**
     - (a) the nil-Meta hole (M2);
     - (b) `wireguard/peer.go:129` still returns Meta with an unmarked error, so under the new contract it is *not* journaled. It is not wired yet.
       The F-wireguard board note still says "delete it, or best-effort subscription". Neither the note nor `F-wireguard.envelope.md` mentions
       `scheduler.PartialCreate`, so amend both;
     - (c) dhcp.client (Q3 below). It is live, and `PartialCreate` cannot fix it.
2. **Claim → write → release on error:** correct in natcommon, the six DF-1 attributes and df6 keyed. Each is proven by a test that fails on
   the old order. A crash between claim and write never blocks a later Create: the claim is idempotent and the absent object is simply added.
   Cleanup at restart is only by boot-identity pruning (L5). TD-11c's batching changes this for keyed stores (M3).
3. **Persistent guard.**
   - **Product wiring:** the one registration path is `agent.go:138` → `subsystems.Register` → `register(guardRegistry)`, so core, DF-1 and every
     W-seed anchor are seen. The guard sees through `defaultTolerant`/`vethOnly` by reflecting on the embedded `scheduler.Descriptor`.
   - **Test wiring is not refused:** tests that use `Register` get persisted stores in a temp dir, and unit tests that build descriptors directly never pass through the guard.
   - **Gap:** it refuses only descriptors that opt in (M1).
4. **`Wiring.PairClaims("df6")`:** correct.
   - It uses the `id|holder` key, is bound to the boot identity, is pruned in `Connected` (it lives in `w.keyed`), and never binds to an index.
   - `checkIDClaims` refuses `IfaceClaims` via `BindsInterfaceIndex`, which is a good structural check.
   - Caveat: it is batched by TD-11c (M3).
5. **R2 refresh on the caller's ctx:** correct for callers with a deadline, and tested for both deadline and cancel. For callers without one it is now unbounded (L1).
6. **Hunks outside the file list:** all justified.
   - `subsystems.go`: `Register`→`register`, so the guard wraps every anchor without touching the W-seed lines; the refresh closure, which is where R2 lives; one doc line.
   - `stores_test.go` needs the resolver signature change. `dfkit/iface.go` changes doc comments only.
   - The host test is a new file in `agent/` with no hunks in TD-9's files.
   - The rebase onto main is clean (simulated).
7. **The TD-11c conflict:** there is no textual conflict.
   - TD-11c moved its hunks off TD-11b's lines (1c7ded2).
   - A 3-way merge from 998e391 is clean, and the merged tree builds and passes subsystems/core/scheduler/agent unit tests.
   - The real conflict is semantic: M3, plus core's missing ctx claim and `CheckPersistent` (Nits).
8. **Q3 — dhcp.client claims after the write. Recommendation: F-kea-dhcp-relay fixes it, as a manager addendum to its envelope.**
   - **Why F-kea-dhcp-relay:** it is active and owns `descriptors/dhcp/**` exclusively. A TD-11b amendment would edit another worker's file.
   - **Why it cannot wait for `PartialCreate`:** the rollback's `Delete` goes through `dfkit.ResolveForDelete`, which returns "not ours" for an
     unclaimed untagged interface. It is then a no-op, reported as success, and the client stays in VPP.
   - **Worse:** the next resync's `config` gets INVALID_VALUE → `tg.Adopt()` → `ErrNotOurs`. The Create is then blocked until VPP restarts.
     A realistic trigger is `bootid.Current` failing at connect, which gives `ErrNoIdentity`.
   - **Acceptance for the addendum (≈1 h):**
     - claim first (`tg.ClaimFirst(ctx)` once TD-11b is on main, or the same thing inline with `Claimed`/`Claim`/`Release`);
     - INVALID_VALUE → `c.Adopt()`; any other error → `c.Undo(err)`;
     - `CheckPersistent` via `dfkit.CheckClaims`;
     - a fake-VPP test that fails on the current order.
9. **Q4 and Q5:** agreed. Per-boot "applied" records (df6 bypass, cnat) must stay write-then-record (D-076). The TD-9 notes are accurate.
10. **Evidence and scope:**
    - The host proof runs as a new agent process on the same state dir, NRestarts 1→1. It is regression evidence; it would also pass on the base, as
      the envelope allows.
    - The base failures are pasted for every behaviour change, and I reproduced one of them.
    - No scope creep beyond PairClaims, which is justified because the guard needs it.

## Required before merge
- M1: add the completeness check for registered descriptors.
- M2: journal on the marker alone, with one shared predicate and a nil-Meta test.
- L1: the one-line `legacyBound` cap for callers with no deadline.

## Recommended in this branch, or as tech-debt
- L2, L3 and L4.

## For the manager
- M3: a D-entry on batching vs. claim-first.
- L5: a D-entry plus a tech-debt row for claim GC.
- The Q2 LOG entry.
- Amend F-wireguard (3.3b → `PartialCreate`).
- Add the F-kea-dhcp-relay addendum (Q3).

**APPROVE WITH CHANGES**
