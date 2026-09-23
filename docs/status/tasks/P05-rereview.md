# P05 re-review — after the fix round for the BLOCK verdict

Reviewer: independent re-review agent (I did not write this code) · branch `task/P05` @ `e83cbc6` · diff reviewed
`b7d8533..HEAD` (fix commits `1b3c874`, `c2f54e1`, `636981c`, `7a607c3`, `fd60c9d`, `ecdf16e`, plus docs) · slot 7 (`w7`,
tables 7000–7999) · run directly on the host · 2026-09-24.
VPP `NRestarts=2` before and after every host run. I did not restart VPP.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (my run, log `/root/ngfw-wt/logs/ci/P05-20260924-013433-1432473`) | **CI GATE PASSED**, wall time 1m09s. The only warning is main's `review(P05): findings` subject. Matches the gate pasted in P05.md |
| `go test -race -count=1 ./internal/...` (apps/agent) | all packages ok, no races |
| `VRX_INTEGRATION=1 go test -run OnHost ./internal/agent/ ./internal/descriptors/core/` under `flock -s` on the lab lock | PASS: `TestAgentOnHost` 5.00s (converged 225 ms after the loss), `TestAgentProcessOnHost` 6.98s (kill -9 by PID, converged 1.02 s), `TestCoreOnHost`, `TestClaimRulesOnHost` (log lines identical to P05.md) |
| trial merge `git merge-tree main HEAD` | clean. `go vet` and `go test ./internal/...` on the merged tree (DF-2 + RF-1 now on main) pass. The only failures were contracttest fixtures, which were missing because I extracted `apps/agent` alone |
| contract guard, `binapi/`, `tools/binapi-gen.sh` | untouched. The fix commits touch only `apps/agent/internal/**` and P05 status docs; the board/LOG hunks in `b7d8533..HEAD` come from the main merge `d8cdc0b` |
| my throwaway tests (fake + host) for concurrency, the revert wedge and the claim rule | written in the worktree, run, then deleted. Not committed (`git status` clean) |

### Restart simulation with the real binary (once, as required)
Built `vrx-agent`/`vrx-agentctl` into my scratch dir. Owner `w7`, own state dir, socket `/run/vrx-test/w7/rr.sock`,
metrics off. Doc: VRF `w7-rr`=7031, `loop731` (in 7031, v4+v6), `loop732` (default), recursive route in 7031, blackhole in table 0.
```
NRestarts=2
apply doc.json -txn rr-1                         → APPLIED created 9
kill -9 1417829 ; vpp_papi: delete_loopback loop731 5 / loop732 16 ; ip_table_flush + ip_table_add_del del 7031 ip4/ip6
restart → "reconcile done" mode=resync APPLIED summary="created:8 unchanged:1"   (table-0 route survived the loss)
vppctl show ip fib table 7031 10.7.131.0/24     → w7:w7-rr … locks:[interface:1, API:1, recursive-resolution:1, ]
kill -9 1422783 ; restart                        → resync APPLIED "unchanged:9" reapplied:1
apply pend.json -txn rr-p -confirm 3 (adds loop733, drops the table-0 route) → APPLIED created 2 deleted 1
kill -9 1424989 ; restart 4 s later              → WARN "confirm timeout: reverting…" rr-p ; mode=revert APPLIED "created:1 deleted:2 unchanged:8"
vppctl: loop733 gone, 10.7.132.0/24 back          ; vrx-agentctl confirm rr-p → FailedPrecondition "not pending confirmation"
cleanup apply {} -subsystems interfaces,vrfs,routing → deleted 9 ; show ip table: no 7031 (so the resync Reapply did not stack an API lock)
NRestarts=2
```

## Original findings

| # | verdict | evidence |
|---|---|---|
| **H1** foreign VRF table deleted by our rollback | **FIXED** | `vrf.go:85-111` `ensure` dumps the id and returns `ErrTableConflict` **before any message** when a family carries another name. Create cleanup removes only the families this call created (`:145-151`). `remove` (`:115-137`) re-dumps and deletes only families still named `<owner>:<vrf>`; Delete, Update and Reapply all go through it. Host `TestClaimRulesOnHost`: `w7ra` on `w7rb`'s 7050 → ROLLED_BACK, `w7rb`'s plan is still empty afterwards. Fake `TestVRFTableIDOfAnotherOwner` asserts that no `ip_table_add_del` is sent and that a renamed family survives Delete |
| **H2** foreign route overwritten, then deleted | **FIXED for foreign API routes, but the fix over-rejects (see N1)** | `route.go:197-223`: a prefix not in the owner table is claimed only when `ip_route_dump` has no entry for it. Delete (`:236-262`) acts only on claimed keys and re-checks that the entry exists. Host: the table-0 collision → `ErrRouteConflict`, nothing sent, nothing recorded, and B's route is intact. Residual (documented in P05.md): VPP's API source is shared, so a claim can go stale (N3) |
| **H3** failed revert lost | **FIXED as specified, but it introduces a liveness regression (N2)** | `service.go:422-461`: `desired := confirm`, `Reverting=true`, persisted **before** the attempt. `Resync` (`:467-476`) goes straight to the revert when the revert is owed or the deadline has passed, and never re-applies the unconfirmed doc. `TestRevertOwedWhenVPPDownAtDeadline` covers VPP disconnected at the deadline → still pending across a restart → reconnect → baseline → the second resync is a no-op. I re-ran it and read it; it asserts on the fake VPP snapshot, not only on the agent map |
| **M1** confirm accepted after the deadline | **FIXED** | `service.go:251-255` rejects the confirm when `Reverting` or `now ≥ deadline`, under the txn lock. `TestLateConfirmRejectedAfterRestart` confirms before the first resync after a restart. Real binary (above): the confirm after the revert is rejected |
| **M2** map write under RLock | **FIXED** | `reconciler.go:404` `plan()` no longer writes `s.woDescriptors`. `ApplyWith:634-636` records it under the write lock. `TestConcurrentPlansWithWriteOnly` passes under `-race`. My own `-race` test with 8× concurrent DryRun + Retrieve + Apply + Health on the service (`-count=3`) was clean |
| **M3** three files not atomic together | **FIXED** | `state.go` keeps one authoritative `agent-state.json` (meta + both documents as protojson, one atomic rename). `desired.pb` is a mirror that is never read, except to migrate the old layout. `TestStateCrashInjection` injects a crash before the state write → old consistent state; one before the mirror → pending txn still unconfirmed. `TestStateMigratesOldLayout` covers the old layout. Order check: `Apply(confirm)` programs VPP, then saves. A crash between the two restarts on the old baseline, and resync converges back, which is the safe direction |
| **M4** `vrf` unset for default-table interfaces | **FIXED** (one low edge case, L-a) | `projection.go:426-432`. `TestDefaultVRFNoDrift`: Retrieve == canonical, and re-applying the retrieved doc makes no changes. An unknown table id is still reported numerically, not as `default` |
| **M5** addresses on non-loopback interfaces | **HANDED OFF** (as the review asked) | `descriptors/core/README.md` "Handoff to P08" |
| **L1** stale Meta index in undo/delete | **FIXED** | loopback, address and table-binding deletes re-resolve by tag right before acting (`loopback.go:84-96`, `ifaddr.go:86-94`, `:198-206`) |
| **L2** owner table lost/corrupt | **FIXED (documented)** | README "Owner table recovery (L2)" |
| **L3** unimplemented domains silent | **FIXED** | `projection.go:253-257` warns `agent.unimplemented-domain` for non-empty unimplemented domains only (so the 13-domain prefaulted `{}` doc stays quiet). `TestUnimplementedDomainWarning` |
| **L4** `vrx-agentctl` scope creep | accepted, unchanged | — |

LOG checks:
- **D-063/D-065/D-069**: unchanged since the first review, still fine.
- **D-071**: the claim rule is applied to VRF, route, loopback, address and binding. The deletes re-verify identity.
- **D-073b** (descriptions from agent state): implemented; `TestDescriptionsRoundTrip` shows no description is invented for a missing object.
- **D-072**: there is **no hook**. The contract `StaticRoute` has no "FRR-programmed" flag yet (`packages/schema/src/domains/routing.ts:159-199`), so P05 programs every static route, which is D-072's default. Nothing in P05.md or the core README says where the skip goes (L-b).
- **D-076**: not applicable to core, which has no write-only descriptor. The only "re-apply on every resync" path is `VRFDescriptor.Reapply`. On the host it does not stack an API lock: after `reapplied:1`, a single `ip_table_add_del(del)` per family removed table 7031. The scheduler's write-only path leaves idempotence to the descriptor, as D-076 requires.

## NEW findings (ranked)

### N1 (High) — The route claim rule treats VPP-internal FIB entries as a foreign owner: legitimate configs fail, and so do re-adds and reverts
`apps/agent/internal/descriptors/core/route.go:152-178` (`lookup` returns any `ip_route_dump` entry for the prefix)
and `:204-211` (any entry that is not the default-drop /0 → `ErrRouteConflict`).
`ip_route_dump` returns every FIB entry, whatever its source. VPP itself creates entries that no API client owns:
- `recursive-resolution`: the /32 of every recursive next hop
- `adjacency`: ARP/ND neighbours
- `interface`: the connected /24 and local /32 of every address
- special entries: 224/4, 240/4, 255.255.255.255/32 and others

A static route whose prefix coincides with one of these is rejected as "owned by someone else".
Reproduced on the host VPP (slot 7, owner `w7rr`, table 7060, cleaned up, `NRestarts=2→2`):
```
{vrf, 10.7.160.0/24 via 10.7.161.1}                  → APPLIED
+ 10.7.161.1/32 (host route for the next hop)       → ROLLED_BACK create ip.route/7060/10.7.161.1/32: core: route prefix already present in the FIB and not owned by this agent
both in one fresh transaction                        → ROLLED_BACK (same; key order creates the /24 first)
baseline {vrf, 10.7.170.0/24 via 10.7.161.1, 10.7.161.1/32} → APPLIED   (the /32 sorts first)
remove the /32                                       → APPLIED
re-add the /32 (exactly what a confirm revert does)  → ROLLED_BACK (same error)
```
Pinning a default or aggregate route's next hop with a /32 is a common configuration. A static route over a connected
prefix or a neighbour /32 is less common, but VPP accepts it. Combined with N2, a valid baseline can therefore become
impossible to revert to without any foreign party involved.
**Fix:** decide the conflict on the entry's **source**, not on its existence:
- Use `ip_route_v2_dump`/`ip_route_v2_details` (`Src`, present in `binapi/ip`) or an equivalent source check.
- Only a client-programmed source (API; plus CLI/DHCP/lcp-rt if you want to be strict) blocks a claim.
- `recursive-resolution`, `adjacency`, `interface`, `default-route` and `special` never block. VPP stacks the API source next to them and picks by source priority, so no one else's object is touched.
- Delete keeps removing only the API source.

Make the fake model RR/connected entries and add a host regression for the three cases above.

### N2 (High) — A revert that cannot succeed wedges the agent permanently: every Apply is refused, and nothing retries except a VPP reconnect or an agent restart
`service.go:261-264` (any Apply while `PendingTxnID != ""` → `FAILED_PRECONDITION`), `:422-461` (`Reverting` stays set
until a revert returns APPLIED), `agent.go:210` (the only `Resync` call site is the VPP (re)connect event; there is no periodic retry).
The H3 fix correctly refuses to lose the revert. But when the revert fails deterministically (a ROLLED_BACK plan, not VPP
down), the agent refuses every new configuration forever, and restarting does not help because `Reverting` is persisted.
The operator's only recovery is to edit `agent-state.json` by hand. A transient failure (for example a verify flap
while VPP stays connected) is also never retried until the next reconnect or restart. In that window every Apply is refused.
Reproduced on the fake VPP:
```
base: route 10.7.99.0/24 (blackhole)  →  p1 (-confirm 1) removes it  →  another owner (w7x) adds 10.7.99.0/24
after deadline: pending="p1" degraded=true
new Apply fix1 (routing {})  → FailedPrecondition "transaction \"p1\" is pending confirmation: confirm it … or let it revert"
Resync                        → ROLLED_BACK "create ip.route/0/10.7.99.0/24: … not owned by this agent"
new Apply fix2                → FailedPrecondition (same)
agent restart → Resync        → ROLLED_BACK, pending="p1"   (still wedged)
```
With N1, the same wedge happens on a single-owner box (baseline with a next-hop /32, pending txn drops it, deadline).
**Fix (all small):**
1. While `Reverting` is true, accept a new non-confirm Apply. It supersedes the owed revert: clear pending/`Reverting`, and on success the new doc becomes desired and confirmed. The API always sends the full running document, so this is safe.
2. Retry an owed revert on a backoff timer (for example 5 s → 60 s) while VPP is connected, not only on reconnect.
3. Surface `Reverting` in Health (for example the degraded reason already says "retried on the next resync"; make it accurate).

Add a fake test: revert ROLLED_BACK → a new Apply succeeds, and the timer retry converges once the obstacle is removed.

### N3 (Low) — A stale route claim can adopt a foreign route
`route.go:203` and `:213-221`: a key that is already in the owner table skips the FIB check. The claim goes stale in
two ways:
- A kill -9 between `Owned.Add` and `ip_route_add_del` leaves a claim with no route.
- A route lost with VPP while the owner table survives also leaves a claim with no route.

Once another API client adds that prefix, Retrieve (`:265-339`) reports it as ours, so Update replaces its paths and a
later removal deletes it. This is inherent to the shared API source and partly documented in P05.md "Out of scope".
Single-agent production is unaffected. **Fix (optional):** in Create, when `existed` is true but Retrieve did not
report the key, treat it as unclaimed and run the check. Or note the window in the README next to the L2 recovery text.

### L-a (Low) — `vrf: "default"` is hard-coded for unbound interfaces
`projection.go:430` writes the literal `"default"`. The projection allows a VRF with another name on id 0 (it only
warns `vrfs.default-table`). An interface configured with that VRF then Retrieves as `"default"`, which is permanent
drift. **Fix:** use the name of the desired VRF with id 0 when one exists, else `"default"`, or reject non-`default` names for id 0.

### L-b (Low) — D-072 hook not recorded
No flag exists in the contract yet, so P05 correctly programs every static route (the D-072 default). But no document
says who adds the flag (contract change) and the projection skip in `projection.go` `routing.static`. **Fix:** add one
line to the core README handoff: "a D-072 FRR-owned static route is skipped by the projection. Owner: P08/P12 +
contract". Also note that under the new claim rule a linux-nl route for the same prefix makes the VPP-programmed route
fail with `ErrRouteConflict` (the desired "never both" outcome, but it needs a clear message).

### L-c (Low) — Loopback Delete swallows the ownership error silently
`loopback.go:89-91` `if err != nil { return nil }`. The only error is `ErrNotOwned`, so this is harmless today. Make it
`errors.Is(err, ErrNotOwned)` like `ifaddr.go` so a future error type is not swallowed.

### Note (not a P05 defect)
The VPP 26.06 quirk the fixer found, where a table deleted with API drop routes leaks them into the next table reusing
the FIB index, is not yet in `docs/vpp-code-track.md`. The manager should add it and tell slot 10.

## Required before merge
N1 and N2 (with the regression tests named above). They are small and local (`route.go` claim check, `service.go`
Apply gate and retry timer). A follow-up re-review only needs to check N1/N2 plus a CI run. N3 and L-a…L-c can go in the
same round or to P08.

**BLOCK**
