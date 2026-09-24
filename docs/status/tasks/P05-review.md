# P05 review — agent core (govpp, reconciler, core descriptors, ownership, gRPC, resync, confirm timer)

Reviewer: independent review agent · branch `task/P05` @ `5062933` · slot 7 (`w7`, tables 7000–7999) · 2026-09-24
VPP `NRestarts=2` before and after every host run (no VPP restart, D-012/D-064).

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (my run, log `/root/ngfw-wt/logs/ci/P05-20260924-010303-1118937`) | **CI GATE PASSED**, wall time 1m03s. Matches the pasted gate in P05.md |
| `VRX_INTEGRATION=1 go test -run OnHost ./internal/agent/ ./internal/descriptors/core/` (slot 7, `flock -s` lab lock) | PASS: `TestAgentOnHost` 4.80s (restart after loss converged in 209ms), `TestAgentProcessOnHost` 6.43s (kill -9 by PID, converged in 1.0s), `TestCoreOnHost` 0.13s |
| my own restart simulation with the real binary (owner `w7`, own state dir, own socket `/run/vrx-test/w7/rv.sock`) | apply 10 objects → re-apply `unchanged:10` → `kill -9 <pid>` → loopbacks + tables deleted via the binary API (vpp_papi) → restart → `created:7 updated:2 unchanged:1` and `vppctl show interface address` / `show ip fib table 7001` back, `locks:[interface:2, API:1, …]` → `kill -9` of the converged agent → restart → `unchanged:10`, same sw_if_index |
| confirm timer | `-confirm 4`, never confirmed → reverted at the deadline (`deleted:3`). Across restart: `-confirm 4`, `kill -9` right away, restart 6 s later → one resync, then **one** revert (no double revert, no lost revert) |
| contract guard (`packages/schema`, `packages/proto`, `apps/agent/gen`, api-client) | no changes |
| P05a frozen files (`scheduler/descriptor.go`, `vpp/client.go`, `vpp/fake`, `renderers/renderer.go`, the READMEs) | not modified; only the new `descriptors/core/README.md` was added |
| binapi provenance | `binapi/` and `tools/binapi-gen.sh` not touched; P05 imports only generated packages (`interface`, `interface_types`, `ip`, `ip_types`, `fib_types`, `memclnt`, `vpe`, `vlib`); no hand-typed `vl_api_*` / `*_reply` names |
| security | no `exec.Command` in P05 production code; gitleaks clean; no secrets in logs, evidence or status files; `desired.pb` carries no secret fields (D-040); socket 0660, no `groupadd` |

I used a throwaway test file on the fake VPP (`coretest`) to reproduce H2, H3, M1 and M2. I deleted it afterwards and did not commit it. H1 is also reproduced on the host VPP (transcript below).

## Findings (ranked)

### H1 — A rolled-back transaction deletes another owner's VRF table and its routes (violates 3b "must never delete each other's objects", D-071)
`apps/agent/internal/descriptors/core/vrf.go:69-80` (Create), `:102-105` (Delete), `:48-66` (addDel).
`ip_table_add_del(is_add=1)` on a table id that already exists does nothing in VPP: the table keeps its old name. So
Create "succeeds" for a table another owner already created. Retrieve then filters by name (`<owner>:`), does not see
the table, and verify fails with `vrf/<id> missing`. The rollback runs `undo(OpCreate)` → `Delete` →
`ip_table_add_del(is_add=0)` with no identity check. VPP drops the API lock and flushes the API routes, and the foreign
table disappears. The same unconditional delete sits in Create's own cleanup path (`vrf.go:76`) and in a plain
`Delete`, which runs by id and never re-checks the name.
Reproduced on the host VPP (slot 7):
```
owner w7rb applies {"vrfs":{"blue":{"id":7050}}, route 10.7.250.0/24 blackhole in blue}   → APPLIED, created 2
  w7rb:blue, fib_index:2 … locks:[API:1, ]   10.7.250.0/24
owner w7ra applies {"vrfs":{"red":{"id":7050}}}                                          → ROLLED_BACK
  "verify: actual state differs from desired: vrf/7050 missing"
table 7050 after w7ra's transaction: (nothing — `show ip fib table 7050` and `show ip table` empty)
w7rb retrieve: {"desiredState":{"routing":{}}, …}          ← its VRF and route are gone
NRestarts=2 → 2
```
On the shared host this needs a table-id collision, which slot ranges prevent only by convention. In production any
table the agent did not create with that id (DF-* / lcp / operator) is at risk. The two-owner tests
(`agent_integration_test.go`, the core tests) use disjoint ids and therefore miss it.
**Fix:** Before creating, `ip_table_dump` the id. If the table exists under a name that is not `<owner>:*`, fail
Create with a clear conflict error and send no message. If it exists under our name, it is ours (repair). Create's
cleanup deletes only the families this call actually created. `Delete` and `undo` re-dump and delete only when the name
is still `<owner>:<vrf>` (D-071 "deletes by index must re-verify identity"). Add a fake test and a host test for an id
collision between two owners.

### H2 — Route ownership is claimed without an existence check: another owner's table-0 route is overwritten, then deleted (3b, D-071 claim rule)
`apps/agent/internal/descriptors/core/route.go:148-165` (Create adds the key to the owner table, then
`ip_route_add_del` with `is_multipath=0`, which replaces the API-source path set, per the comment at `:140`) and
`:176-185` (Delete removes every API path of the prefix).
If owner B already has `10.99.0.0/24` in table 0 and owner A applies the same prefix, A records it as owned and
replaces B's paths. A later authoritative-empty `routing` apply from A deletes the route: B's route is gone.
Reproduced on the fake VPP (`TestReviewForeignRouteTakenOver`: after A's `subsystems=routing` empty apply,
`HasRoute(0,"10.99.0.0/24") == false`). P05.md mentions only the milder half, "another owner … would replace our path
set". This half deletes a foreign object. Under D-072 (the agent is the single static-route programmer) the same
applies to any other API client that programs a route in a table we share.
**Fix:** Implement the D-071 claim rule. In Create, when the key is not already in the owner table, dump the prefix
first. If an entry with API-source paths exists, fail with a conflict error and record nothing. Document that table 0
is shared and that ownership is per prefix.

### H3 — Confirm-timeout revert is lost when the revert fails (e.g. VPP disconnected): the unconfirmed state becomes permanent (contract §4.3/§4.6)
`apps/agent/internal/agent/service.go:391-410` (revertLocked) together with `:310-314` (modeRevert updates
`st.desired` only on APPLIED).
`revertLocked` clears `PendingTxnID`/`ConfirmDeadline`, then calls `applyLocked(modeRevert, st.confirm)`. When that
fails (FAILED because VPP is down, ROLLED_BACK, or DEGRADED), `st.desired` still holds the pending document and the
state is saved with no pending transaction. The next resync (VPP reconnect or agent restart) re-applies the pending
document as if it had been confirmed. `CONFIRM_REVERTED` was emitted anyway, and new Applies are allowed again.
A confirmed commit exists for exactly this case: a bad commit that disrupts the box, including VPP itself.
Reproduced on the fake VPP (`TestReviewLostRevert`):
```
confirm timeout: reverting …  → revert status=APPLY_STATUS_FAILED "retrieve vrf: ip_table_dump: vpp: not connected"
health: pending="" degraded=true
VPP reconnect → resync status=APPLIED summary=unchanged:10   ← loop701 of the reverted txn p1 is still there
```
**Fix:** On timeout, set `st.desired = st.confirm` and `Managed = union(...)`, clear pending, and persist all of it
*before* applying the revert. Every later resync then converges to the confirmed baseline even if this attempt fails.
Alternatively, keep the transaction marked as "revert owed" until a revert succeeds. Add a test with VPP disconnected
at the deadline.

### M1 — A confirm is accepted after its deadline (restart window, or while the revert waits for the transaction lock)
`service.go:224-233`: the confirm path compares only `PendingTxnID`, never `ConfirmDeadline`. `agent.go:165-176`
starts serving gRPC before the first resync, and the timer is armed only in `Resync` (`service.go:430-435`), which
waits for VPP. After a restart past the deadline, or while VPP is not yet connected, `Apply{confirm_txn_id}` is
therefore accepted and the expired transaction becomes the confirmed baseline. That contradicts §4.5: "kill -9 of the
agent therefore never leaves an unconfirmed state confirmed by accident". Reproduced on the fake VPP
(`TestReviewLateConfirmAfterRestart`: CONFIRMED 200 ms after the deadline).
**Fix:** Reject a confirm with `FAILED_PRECONDITION` when `now >= ConfirmDeadline`. Optionally, have NewService arm the
timer and let the revert wait for VPP.

### M2 — Data race: `Plan` (DryRun) writes `s.woDescriptors` under the read lock
`apps/agent/internal/scheduler/reconciler.go:350-354` takes `RLock`, then `plan()` writes `s.woDescriptors[name] = true`
at `:405-407`. DryRun does not take the service's transaction lock (`service.go:479-502`), so two concurrent DryRuns,
or a DryRun racing Retrieve, perform concurrent map writes. That is a fatal runtime error that kills the agent as soon
as a write-only descriptor is registered (DF-2, D-063, wired in P08). The race is latent today only because core has
no write-only descriptor.
**Fix:** Have `plan()` return the write-only set and record it into `s.woDescriptors` only under the write lock (in
`ApplyWith`), or guard it with its own mutex. Add a `-race` test with a write-only fake descriptor and parallel `Plan`
calls.

### M3 — Crash window between the three state files can confirm a pending transaction or lose the pending marker
`apps/agent/internal/agent/state.go:114-129`: `desired.pb`, `confirmed.pb` and `agent-state.json` are each atomic, but
not atomic together. A `kill -9` or power loss after `desired.pb` (the pending document) and before `agent-state.json`
(which still says no pending transaction) restarts into a resync of the pending document with no timer. That is the
same outcome as H3, within a window of a few ms.
**Fix:** Write one file that holds the metadata and both documents (for example a small wrapper message, or JSON with
base64 pb), or write the json with a generation number that must match the pb files and roll back to `confirmed.pb`
on a mismatch.

### M4 — Retrieve leaves `interfaces.<if>.vrf` unset for interfaces in the default table: permanent drift (D-039 / contract §5)
`ifaddr.go:107` skips table 0, and `projection.go:394-397` sets `vrf` only from an `interface-ip.table` object. The
Zod schema defaults `interfaces.*.vrf` to `"default"` (`packages/schema/src/domains/interfaces.ts:114`), so every
parsed document carries `vrf: "default"`, while Retrieve omits it for every owned interface in table 0. The value is
known: every VPP interface is in exactly one table. §5 says a scalar is set exactly when the object carries it.
**Fix:** In `assemble`, set `vrf: "default"` for every owned interface without a table binding (for example from the
loopback KV). Add a Retrieve == canonical test with a loopback in the default VRF.

### M5 (handoff for P08, not a P05 defect) — Addresses on non-loopback interfaces cannot be applied
`core.go:69-74` (`DirectInterfaceRef` → `interface/<name>`, no descriptor registered) and `ifaddr.go:154-165`
(`t.owned()` → `ErrNotOwned`). A full document with, for example, `interfaces.lan.ipv4` fails validation with
`mandatory dependency interface/lan …`. Physical NICs are untagged, so even after DF-1's alias is wired,
`interface-ip` needs a D-071 ClaimStore or "physical/pre-existing interface" path to add addresses. P08 must plan for
this. I list it so it is not rediscovered at integration time.

### L1 — Rollback of an `OpCreate` uses the meta from creation time, which can be stale
`reconciler.go:1024` (`undo` → `Delete(newValue, newMeta)`). An address created on `loop7xx` (sw_if_index 5) and later
re-created by `around()` (loop recreated → sw_if_index 7) is deleted by index 5 during rollback. VPP reuses freed
sw_if_indexes (pool_get), so the index may belong to another interface by then. The practical harm is small (it
yields REVERT_FAILED/DEGRADED, not a foreign delete, unless the addresses coincide). Still, D-071 says to re-verify
identity. **Fix:** Undo re-resolves the interface by tag (pass nil meta), or `around()` updates earlier journal entries.

### L2 — Owner table lost or corrupt
`ownertable.go:107-113`: a corrupt file makes the agent refuse to start. Failing closed is correct, but no recovery
procedure is documented. A lost file turns owned table-0 routes invisible forever. They are never deleted, and
Retrieve under-reports them. Routes in owned VRF tables are flushed when the table is deleted. **Fix:** Document the
recovery in the core README. Optionally, reconstruct table-0 ownership from `desired.pb` on start when the file is
missing.

### L3 — Present but unimplemented domains are applied silently as APPLIED (D-P05-10)
`service.go:163-174` limits the authoritative domains to the implemented ones. A document carrying `nat`/`acl` returns
APPLIED and DryRun does not warn. That is contract-compatible (Health lists the subsystems), but the API must check
`Health.subsystems`, or DryRun should emit an `agent.unimplemented-domain` WARNING. Recommend the warning.

### L4 — Scope creep (minor, acceptable)
`cmd/vrx-agentctl` (D-P05-16) was not requested. It is a small dev client used for the evidence and is fine to keep;
P13 remains the product CLI. `interface-ip.table` (D-P05-12) is justified by the task text ("loopbacks with IPs in your
VRF range").

## Things that were checked and are fine
- Restart safety: owned leftovers are deleted on resync, foreign ones (other tag, untagged) are kept. Observe-only
  (D-065) objects are never deleted or verified. Write-only (D-063) objects are never deleted on absence and are
  re-applied on resync. The VRF API-lock re-assert (Reapplier) works on the host.
- Transactions: deletes run first in reverse topological order, then creates/updates in topological order. Live
  dependents are removed and re-created around any create/delete/recreate. Rollback undoes the journal in reverse with
  a cancel-free context. Verify re-Retrieves. The host rollback cases leave nothing behind.
- Concurrency: Apply, resync and revert are serialised by `txn`. Retrieve waits on the scheduler's RLock. The timer's
  revert re-checks `PendingTxnID` under the lock, so a late timer after a confirm is a no-op: no double revert (also
  seen on the host).
- gRPC: `Action` → UNIMPLEMENTED, an unimplemented subsystem → UNIMPLEMENTED, owner mismatch → INVALID_ARGUMENT,
  txn reuse → ABORTED, apply while pending → FAILED_PRECONDITION, VPP down → UNAVAILABLE. Presence: `Vrf.id`,
  `blackhole`, `weight` and `distance` (Zod default 1 → preference 1) round-trip.
- Mocks are not the only proof: the integration tests assert on host VPP state (Retrieve plus binapi dumps), and the
  pasted evidence matches my rerun.

## Required before merge
H1, H2, H3 (each with a regression test: an id or prefix collision between two owners on the host, and VPP
disconnected at the confirm deadline), plus M1 and M2 (small). M3 and M4 are strongly recommended in the same round.
M5 goes to P08's plan.

**BLOCK**
