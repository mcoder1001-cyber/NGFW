# TD-11c focused verify: fix round 1

Reviewer: the author of TD-11c-review.md (ac313a1c), 2026-09-25. Branch `task/TD-11c` @ 08a95957. The branch's own diff is
`git diff d3a7c266 HEAD`, where d3a7c266 is the merge-base with main.

Scope: only my findings F1–F5, O1 from TD-11b's verify, and regressions in the touched packages. Every fix must have a test that
fails on the old code. I did not re-review anything else, made no host runs and did not run ci.sh. CI evidence at 8106bd5c is
pasted in TD-11c.md.

**Verdict: APPROVE.** Every item is fixed, and a mutation that reverts each fix makes its test fail. Three nits can be done at
the squash or by the manager.

## What I ran
```
$ cd apps/agent && TMPDIR=/tmp/g-rv11c go test -race -count=1 ./internal/agent/... ./internal/subsystems/... ./internal/descriptors/core/... ./internal/scheduler/...
ok  	ngfw/agent/internal/agent	13.118s
ok  	ngfw/agent/internal/subsystems	17.325s
ok  	ngfw/agent/internal/descriptors/core	1.383s
?   	ngfw/agent/internal/descriptors/core/coretest	[no test files]
ok  	ngfw/agent/internal/scheduler	1.409s
$ go vet (same four packages)  → clean
```
Mutation checks: each one reverts a single fix in a scratch copy of apps/agent, then runs the new test.
```
M1 F1  appendLocked returns nil (no journal)          --- FAIL: TestKeyedClaimsSurviveAgentDeathMidTransaction
                                                         stores_journal_test.go:131: claims of the interrupted transaction lost: 0 records
                                                       --- FAIL: TestKeyedClaimsJournal
                                                         stores_journal_test.go:178: claim a not in the journal when Claim returned
M2 F2  claimsNotPersisted call removed               --- FAIL: TestClaimStoresBracketEveryTransaction
                                                         claimstxn_test.go:38: status APPLY_STATUS_APPLIED, want APPLY_STATUS_DEGRADED
M3 F4  claimsBatch cleanup disabled                  --- FAIL: TestClaimStoresBracketEveryTransaction
                                                         claimstxn_test.go:70: panicking transaction: begins 6 flushes 5, want 6 and 6
M4 F5  syncLocked without claimsBatch                --- FAIL: TestClaimStoresBracketSourceSync
                                                         claimstxn_test.go:92: sync: begins 0 flushes 0, want 1 and 1
M5 O1  ownership.go from main (RecordsNoOwnership)   --- FAIL: TestInterfaceObjectsRequirePersistedClaims
                                                         ownership_test.go:77: interface-ip.table with claims <nil>: <nil>
M6 F3  gre removed from knownAliasCreatorGaps        --- FAIL: TestEveryInterfaceCreatorNamesItsAlias (…:135 gre.tunnel neither maps … nor provides …)
M7 F3  "bond" device class unmapped in dump.go       --- FAIL: TestEveryInterfaceCreatorNamesItsAlias (…:136 bond.bond neither maps … nor provides …)
```

## Per finding

- **F1 / D-133: fixed.**
  - In a batch, `setLocked` (`subsystems/stores.go:250`) calls `appendLocked` (`:331`) before it changes memory.
    `appendLocked` does one `write(2)` with O_APPEND (`:356`) and no fsync; the only `Sync()` calls are inside `atomicWrite`
    (`:460`, `:476`).
  - A failed or short append is cut back (`:361`) and fails the Claim, so the descriptor never writes VPP.
  - The snapshot write compacts: `flushLocked` then `truncateJournalLocked` (`:325`), once per transaction.
  - `OpenKeyedClaims` replays the journal (`:542` → `replayJournal`, `:406`):
    - A torn last line is dropped (`:422`), and the open compacts the journal.
    - A bad line before the last one fails closed.
    - If the compaction at open fails, the torn tail is cut off, so later appends never glue onto it.
  - `TestKeyedClaimsSurviveAgentDeathMidTransaction` (`stores_journal_test.go:110`) uses the product wiring, a claim-first
    family and the real scheduler. The agent dies after the VPP writes and before the end of the transaction. After the
    restart:
    - the claims are known;
    - Retrieve sees both objects;
    - a re-commit gives an empty plan (`:137`);
    - removal converges.
  - Only KeyedClaims are ever batched (`w.keyed map[string]*KeyedClaims`), and every one of them is opened through
    `OpenKeyedClaims`, so no batched store is without a journal.
  - Measured cost: 2000 claims take about 50 ms.
- **F2: fixed.**
  - `claimsErr := flushClaims()` runs before the outcome switch (`agent/service.go:406`), so rollback releases are included.
  - `claimsNotPersisted` (`:876`) turns APPLIED into DEGRADED, so the desired state is not merged. `st.save` stores the
    unchanged document, and the API takes its DEGRADED path (lostTrack + re-apply).
  - A refused or rolled-back transaction keeps its status, and the agent is marked DEGRADED (`:439`).
  - With the journal, the records of a failed snapshot write remain durable.
- **F4: fixed.** `claimsBatch` (`:859`) returns end plus a cleanup that `applyLocked` defers (`:377`). A panic ends the batch.
  The test panics inside `beforeTxn`.
- **F5: fixed.**
  - `syncLocked` opens the batch at `dynsource.go:410` and flushes at `:424`. The flush runs before `applied` is computed, so a
    failed write means DEGRADED, out of sync, and a retry.
  - Lock order is unchanged: `ClaimsTxn` takes and releases `storesMu` and each store's `mu` before `mergeSources`/`Desired`
    run, so no lock is held across source code.
  - `applySources` (`dynsource.go:260-285`) runs inside `applyLocked`'s batch. No other `ApplyWith` call site exists in agent/.
- **F3: fixed.**
  - N4 is amended (questions N4) and recorded in D-133.
  - `TestEveryInterfaceCreatorNamesItsAlias` checks 19 creators (`iface.Kind` or `scheduler.KeyProvider`), and a source scan
    catches any interface-creating package missing from the table.
  - `knownAliasCreatorGaps` (`creators_guard_test.go:93`) is shrink-only for fixed creators (`:137`). A new gap fails (`:135`).
  - The owners match the board on main:
    - F-tunnels (`plan/tasks.yaml:1195`, "GRE, IPIP, VXLAN(-GPE), GTP-U, L2TPv3, PPPoE");
    - P12 (`:1243`, FRR + linux-cp);
    - F-mpls-srmpls (`:1409`).
  - None of the gap creators is registered in `subsystems.Register` today.
- **O1: fixed.**
  - `core/ownership.go:20,26` declare `CheckPersistent` = `persist.Require(…, d.Claims)`.
  - The tripwire allowlist includes `Claims` (`ownership_test.go:42`).
  - `TestInterfaceObjectsRequirePersistedClaims` (`:63`): nil and in-memory stores give ErrVolatile, and a persisted store
    passes.
  - The product wiring passes the persisted `IfaceClaims`, and the agent's ownership guard accepts it; the agent tests are
    green.
- **Regressions:** none in the four packages under `-race`.

## Nits (not blocking)
1. `subsystems/stores.go:581-586`: the `KeyedClaims.Begin` doc comment still describes the withdrawn N3 trade-off ("an agent
   process that dies inside a transaction loses that transaction's keyed claims … until VPP restarts"). Reword it to D-133 at
   the squash.
2. The gap list lets a gap creator be *wired* with no failing test. The obligation is also missing from the owners' prompts
   (`prompts/features/F-tunnels.md`, `F-mpls-srmpls.md` and `/root/ngfw-wt/P12.envelope.md` have 0 mentions of
   RegisterKind/KeyProvider).
   - Manager: add the line to those three.
   - TD-22 or the first wiring row: assert in the guard that no `knownAliasCreatorGaps` creator is in the registry built by
     `subsystems.Register`.
3. Merge note for TD-8b, which edits dynsource.go next: keep the bracket at `dynsource.go:410-428` around the source's
   `ApplyWith`. The batch opens after `beforeTxn` in `syncLocked` but before it in `applyLocked`. That difference is harmless;
   keep it consistent if TD-8b touches the spot.

F6 and F7 are deferred, as the status says (TD-22 / the TD-11b follow-up).

**APPROVE**
