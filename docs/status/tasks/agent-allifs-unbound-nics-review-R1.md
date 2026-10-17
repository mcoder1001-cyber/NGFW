# Review R1 (correctness and tests): agent-allifs-unbound-nics

- **Reviewer:** panel R1, correctness and tests (Opus 5.5, read-only agent). This run was read-only: no files in the repository or worktree were modified, and no stash, checkout, commit or push was made. The developer saves this text as `docs/status/tasks/agent-allifs-unbound-nics-review-R1.md`.
- **Head reviewed:** `552b9bcce` (product commit `9a63c09ce`; `8d34740d4` and `552b9bcce` change docs/status only).
- **Base:** `origin/main` `99a774734`.
- **PR:** #241, branch `claude/agent-allifs-unbound-nics`.
- **Branch moved during the review:** it is now at `7afee7aef` ("docs(status): quick gate PASSED"). I checked that `git diff --stat 9a63c09ce 7afee7aef -- apps packages tools deploy test` is empty, so the product tree is the same and this verdict also covers `7afee7aef`.

## Verdict: APPROVE — 0 BLOCKER, 0 MAJOR, 2 MINOR, 3 NIT (the MINORs and NITs are optional)

The fix does what the task asks. `globalBlocking` now builds the implicit all-interfaces target set from `SplitUnboundPhysical`'s kept rows. That covers user `allInterfaces` lists and the auto-block overlay, because the overlay always uses `AllInterfaces: true` (`autoblock/runtime.go:123`). Explicit interface lists are untouched: `ifs := l.GetInterfaces()` is unchanged.

- **Nil lookup:** `ACL` follows the same rules as the interfaces projection. Engine-owned physical rows are kept and released rows are dropped (`hostnics.go:22-29`), and a test asserts this.
- **Ordering:** the target set comes from `sortedKeys(kept)`, so it is deterministic.
- **Memoisation:** `sortedKeys` returns a non-nil empty slice (`interfaces.go:511`), so the `allIfs == nil` check works even when the kept map is empty.
- **Laziness:** the split runs only for an enabled list with entries and `allInterfaces`. Disabled lists, lists with no entries and lists with an over-long name `continue` before `allInterfaces()` is called, so ordinary configurations pay no netlink cost.
- **Shared slice:** the slice shared between lists is only read, never appended to.
- **Callers:** the only production caller of `desired.ACL` is `projection.go:411`, which receives the same `s.netdevKind` as the interfaces split (`service.go:1406`). Production always has a lookup: `subsystems.go:295-296` defaults it to `LinuxNetdevKind`. Nothing under `test/` or `apps/cli` calls `desired.ACL`.
- **The 15 test callers:** they now pass `nil`. They only use loopback or non-physical rows, so `nil` gives the same result as before and hides nothing.
- **Retrieve:** `recordGlobalBlocking` now stores a fingerprint over bindings for kept rows only, which is what VPP actually holds. `Retrieve` keeps returning the configured lists instead of a reconstructed copy.

### Acceptance items mapped to tests (each fails without the fix)

| Item | Test | Red reasoning (from the diff; I did not modify files) |
|---|---|---|
| allInterfaces list skips unbound and released rows; bound row still bound; no issue | `TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs` (`global_blocking_test.go:238-278`) | Old `sortedKeys(ds.GetInterfaces())` binds `ens192` → `t.Fatal` at `:262`. This matches the pasted red output, and the `ens161` assertion would fail too. |
| Auto-block overlay plus one unbound and one released row: no binding, no error, notes kept | `TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs` (`physical_nics_acl_test.go:38-59`) | `ens192` is iterated first → `:54`, matching the pasted output. |
| No plan issue end to end; AutoBlockSet enforces; interfaces commit accepted; bound NIC and a new interface bound | `TestAutoBlockWithUnboundAndReleasedNICsOnFake` (`:65-122`) | The first Apply has no entries, so it passes. AutoBlockSet then fails at `:86` with `acl.interface-binding/ens161: mandatory dependency interface/ens161`; `ens161` sorts before `ens192`. Matches the pasted output. |
| Explicit lists unchanged | Same desired test, `:264-267` (`ens161` is bound by the explicit list `pick` only) | If the split were applied to explicit lists, `binding(t, s, "ens161")` would `t.Fatal`, so this is a real regression guard. |

`DryRun` really plans (`service.go:1177-1188`), so "no ERROR issue" covers the scheduler's dependency check. The final Apply binding `loop712` proves the acl domain was part of the interfaces-only transaction (`service.go:1385-1389`, live entries).

## Findings

1. **MINOR — the interfaces projection and the ACL projection look up netdevs separately.**
   - **Where:** `apps/agent/internal/desired/global_blocking.go:107-114` and `apps/agent/internal/agent/projection.go:242`.
   - **What:** the interfaces split at `projection.go:242` and the new re-split at `global_blocking.go:110` each call the lookup. In production that is `LinuxNetdevKind`, a full `RTM_GETLINK` dump per call (`subsystems/netdev.go`), and the two calls can give different answers. `SplitUnboundPhysical` keeps a row when the lookup fails (`hostnics.go:16-17,30`).
   - **Failure scenario:** the first call says `ens192` is unbound, so its alias is dropped. The second call returns a transient netlink error (ENOBUFS or EMFILE), or the netdev disappears between the two dumps. The all-interfaces list then binds `ens192` again, and that transaction fails with the same `CodeDependencyMissing` this PR removes, with a misleading message.
   - **Impact:** narrow and transient. The resync and the auto-block loop retry it, hence MINOR. It also doubles the per-physical-row netlink dumps for every projection with an active `allInterfaces` list.
   - **Fix:** cache lookup results for the duration of one projection inside `projectOwned`: wrap `netdev` in a map-backed cache before line 242 and pass the cached function to both `Interfaces` and `ACL`. Alternatively, pass the kept names (the alternative D-250 rejected). Either way "the rows the projection keeps" holds by construction rather than by recomputation, and the current tests stay valid.

2. **MINOR — no test varies the netdev answer within one service lifetime.**
   - **Where:** `apps/agent/internal/agent/physical_nics_acl_test.go:65-122`; the claim is at `docs/status/tasks/agent-allifs-unbound-nics.md:87`.
   - **What:** the all-interfaces set now depends on host state, but `newACLSvc` hard-codes `fakeNetdevs` (`rpc_acl_test.go:77`). Three things are therefore untested:
     - a NIC bound after the fact gets its binding at the next projection (the status file claims this at `:87`; the schema help and user doc promise "includes interfaces added later");
     - idempotency: re-applying or resyncing the same state produces no change (only visible today in the log line `unchanged:8`);
     - releasing a bound NIC that carries the all-interfaces binding while entries are live: the commit should be APPLIED and the binding removed.
   - **Fix:** one fake-VPP test with a switchable lookup (a closure over a map):
     - `ens192` unbound plus a live entry → `AutoBlockSet` OK;
     - `v.AddInterface("ens192","dpdk","")`, flip the lookup, `s.Resync(ctx)` → `ens192` carries `_gb.auto-block.i00`;
     - a second `Resync` reports no change;
     - release `lan` (`owner: host`) in an interfaces commit → APPLIED, `lan` binding gone.

3. **NIT — the `ACL` doc comment is inaccurate for a nil lookup.**
   - **Where:** `apps/agent/internal/desired/acl.go:91-93`.
   - **What:** "netdev (nil: no check) … never an unbound or released physical NIC" is wrong when the lookup is nil. Unbound NICs cannot be told apart and are kept; released rows are still dropped. The new test asserts exactly this (`global_blocking_test.go:269-277`).
   - **Fix:** reword it to "nil: no netdev check — an unbound NIC cannot be recognised and is kept; a released row is still dropped".

4. **NIT — the cache check depends on a helper detail.**
   - **Where:** `apps/agent/internal/desired/global_blocking.go:107-109`.
   - **What:** `allIfs == nil` relies on `sortedKeys` (`interfaces.go:511`) returning a non-nil empty slice. That is correct today, but if the helper ever returns nil for an empty map, a box whose rows are all unbound or released re-runs the split (one netlink dump per physical row) for every `allInterfaces` list.
   - **Fix:** use an explicit `computed bool`.

5. **NIT — the "selects no interface" case for the overlay is untested.**
   - **Where:** `apps/agent/internal/desired/global_blocking.go:147-149`.
   - **What:** when every row is unbound or released (a fresh appliance before any data NIC is bound), the overlay now takes the "block list %q selects no interface: it is not enforced in the data plane" path. The warning points at `/acl/globalBlocking/lists/auto-block/interfaces`, a reserved runtime list that is not in the user's document. `AutoBlockSet` reports success with `ActiveEntries = n`, while only the host nftables rule enforces. The behaviour is correct (there is nothing in VPP to bind), but nothing tests it.
   - **Fix:** add a one-line test case or a comment that pins it.

## Notes outside R1 (not graded)

- **[other: R7]** Evidence and docs:
  - **PR body placeholder:** the suggested tech-debt row text in "Follow-up on the #238 branch" still contains `PR #@@N@@`; it should be #241.
  - **User doc:** `docs/user/security/global-blocking.md:27` says "*All interfaces* also covers interfaces added later". It should add that an unbound or released physical NIC is covered only once it is bound to the data plane, and that the same applies to auto-block.
  - **#238 doc sentence:** before the #238 exception paragraph is dropped, the sentence "a released row … does not make commits fail" needs a qualifier: an explicit block list, ACL attachment or zone that names such a row still fails closed, by design.
  - **Gate header vs build SHA:** the status file quick-gate block (`:49,:52`) says `@ 8d34740d4`, but the agent build in the same run embedded `main.version=552b9bcce` (`/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-112350-1815599/09-agent.log`). `552b9bcce` was committed at 11:25:16Z while the gate, started at 11:23:50Z, was running in that worktree. The product tree is identical, so the PASS stands, but the status file should say so precisely, and nothing should be committed in a worktree while its gate runs.
  - **Abridged evidence:** the pasted agent-gate block is abridged ("… 143 packages ok" and the golangci-lint line are paraphrases). It is consistent with the log: 143 `ok`, 0 `FAIL`.
  - **Hosted gate:** the hosted "Mandatory quick gate" was IN_PROGRESS on `7afee7aef` when I checked; merge must wait for it.
- **[other: R4]** Pre-existing and outside this task's fence: some other per-row projections still read the full `ds.GetInterfaces()` instead of `boundIfs`.
  - **Checked:** `desired.RpfAdlPbr` (`rpf_adl_pbr.go:135`) emits uRPF objects with a mandatory interface dependency (`descriptors/urpf/urpf.go:62-65`). So uRPF configured on an unbound or released row still fails the plan. That contradicts the `agent.nic-not-bound` note "its configuration is applied once the NIC is handed to VPP".
  - **Not checked:** `BridgeL2`, `LoopbackBviGsoLldpSpan`, `Bonds` and the ADL half of `RpfAdlPbr` (`projection.go:391-404`) are worth a follow-up debt row.

## Evidence (all commands run in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`; Go commands from `apps/agent`)

```
$ git log --oneline -5 ; git rev-parse HEAD origin/main
552b9bcce docs(status): task report for agent-allifs-unbound-nics (PR #241)
8d34740d4 docs(status): envelope, WIP and decision D-250 for agent-allifs-unbound-nics
9a63c09ce fix(agent): all-interfaces block lists target only the rows the projection keeps
99a774734 fix(deploy): complete package upgrades and audit unfinished branches
...
552b9bcce6e33847cd61ba245b83556966cca8e4
99a774734e2cf455f71563ff0dba5afb58051685

$ git diff --stat 99a774734...552b9bcce
 apps/agent/internal/agent/physical_nics_acl_test.go | 122 +++  projection.go 5 +-  desired/acl.go 9 +-  acl_test.go 10 +-
 global_blocking.go 18 ++-  global_blocking_test.go 58 ++-  rule_expiry_test.go 6 +-  subsystems/acl_pbr_test.go 2 +-
 docs/decisions/LOG.md 2 +  + 3 status files        12 files changed, 297 insertions(+), 22 deletions(-)

$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake' -v ./internal/desired/ ./internal/agent/ | grep -E '^(=== RUN|--- |ok|FAIL|PASS)'
=== RUN   TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
PASS
ok  	ngfw/agent/internal/desired	0.139s
=== RUN   TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.01s)
=== RUN   TestAutoBlockWithUnboundAndReleasedNICsOnFake
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.05s)
PASS
ok  	ngfw/agent/internal/agent	0.203s
(log of the fake run: nics-config APPLIED "created:1 unchanged:1"; auto-block [acl] APPLIED "created:6";
 nics-update [interfaces] APPLIED "created:4 unchanged:8")

$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'TestGlobalBlocking|TestACL|TestParseGlobalBlocking|TestProjectPhysicalNic|TestProjectAutoBlock|TestAutoBlock|PhysicalNIC|PhysicalNic|TestPBRPolicyNamesFACLList' ./internal/desired/ ./internal/agent/ ./internal/subsystems/
ok  	ngfw/agent/internal/desired	26.268s
ok  	ngfw/agent/internal/agent	5.061s
ok  	ngfw/agent/internal/subsystems	1.630s
exit=0      (-v: 38 tests PASS, 0 FAIL, incl. the 6 TestProjectPhysicalNic*, 7 TestAutoBlock*, 8 TestGlobalBlocking*,
             TestACLBindingAndInterfaceRemovedInOneCommit, TestACLDryRunAndRollbackDoNotChangeTheAppliedView, TestPBRPolicyNamesFACLList)

$ env -u NGFW_INTEGRATION go test -count=1 -run 'Project|Global|ACL|Acl|AutoBlock|Nic|NIC|Split|Unbound' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	7.222s
ok  	ngfw/agent/internal/agent	7.856s
exit=0      (-v: 104 "--- PASS" lines)

$ go vet ./internal/desired/ ./internal/agent/        → vet exit=0
$ gofmt -l <the 5 changed Go files>                    → (no output; listing only, nothing written)

$ grep -rl --include='*_test.go' '^package ' . | sed 's#/[^/]*$##' | sort -u | wc -l     → 143   (matches "143 packages ok")
$ grep -c '^ok' .../09-agent.log → 143 ; grep -c FAIL .../09-agent.log → 0
  ok ngfw/agent/internal/agent 59.375s · ok internal/desired 29.442s · ok internal/subsystems 33.127s
  go build ... -X main.version=552b9bcce ...   (gate header says @ 8d34740d4; see [other: R7])
$ git log -4 --format='%h %cI %s' 7afee7aef
7afee7aef 2026-10-17T11:34:53Z docs(status): quick gate PASSED ...
552b9bcce 2026-10-17T11:25:16Z docs(status): task report ...
8d34740d4 2026-10-17T11:23:26Z docs(status): envelope, WIP and decision D-250 ...
9a63c09ce 2026-10-17T11:19:58Z fix(agent): ...
$ git diff --stat 9a63c09ce 7afee7aef -- apps packages tools deploy test      → (empty: product tree unchanged)

$ grep -rn "desired\.ACL(" --include=*.go test/ apps/cli/                     → (none)
$ grep -n "a released NIC is not an all-interfaces target\"" internal/desired/global_blocking_test.go → 262
$ grep -n "no binding for %s\|auto-block set with an unbound" internal/agent/physical_nics_acl_test.go → 54, 86
  (these match the line numbers in the developer's pasted red output)

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json headRefOid,statusCheckRollup,mergeable
{"headRefOid":"7afee7aef318514139fecfa6e0bb5a45f42f5098","mergeable":"MERGEABLE",
 "statusCheckRollup":[{"name":"Mandatory quick gate","status":"IN_PROGRESS",...}]}
```

Not run, as the brief required: `tools/ci.sh`, `make`, `pnpm` and whole-tree `go test ./...`. In their place I checked the developer's pasted gate output against the CI log directory and the package count, and ran the targeted tests above myself. I did not reproduce the red state; it is reasoned from the diff and the failure line numbers.

**Verdict line: R1 APPROVE — 0 BLOCKER, 0 MAJOR, 2 MINOR (#1, #2), 3 NIT (#3–#5).** The MINORs and NITs are optional; the developer may fix them or answer won't-fix in one line. The merge still needs the hosted Mandatory quick gate to finish green.

## Verify round (head `873f1d946`)

- **Reviewer:** panel R1, correctness and tests (Opus 5.5, read-only agent).
- **Read-only:** no repository or worktree file was created, modified or deleted. No stash, checkout, worktree, apply, commit or push. `git status --porcelain` was empty before and after, and HEAD is `873f1d946` throughout.
- **Scratch files:** all under `<scratchpad>/review-R1-verify/`: mutation copies, overlay JSONs, one scratch test and logs.
- **Mutation runs:** I used Go's `-overlay` flag. It swaps a file's contents for a scratch copy at build time only. This is the one addition to the allowed `go test -count=1 -run … ./internal/…` form; nothing in the worktree is touched.
- **Scope:** product delta `9a63c09ce..856bf743a` (`apps/agent/internal/{agent/projection.go, agent/physical_nics_acl_test.go, desired/acl.go, desired/global_blocking.go, desired/global_blocking_test.go}`). `856bf743a..873f1d946` is docs/status only: `git diff --stat 856bf743a 873f1d946 -- apps packages tools deploy test` is empty.

### Verdict: R1 APPROVE — 0 BLOCKER, 0 MAJOR, 1 MINOR (new V1), 3 NIT (new V2–V4). All five round-1 findings are fixed. Everything left is optional.

### Round-1 findings

1. **MINOR #1 (two separate netdev observations per projection): FIXED.**
   - **What changed:** `projection.go:242` runs the only `SplitUnboundPhysical(…, netdev)` in production. `projection.go:411` passes its `boundIfs` to `desired.ACL`.
   - **`globalBlocking`** (`global_blocking.go:109-131`) uses `x.kept` and falls back to `SplitUnboundPhysical(ds.GetInterfaces(), nil)` only when `kept == nil`.
   - **Only one production split:** `grep -rn "SplitUnboundPhysical(" --include=*.go . | grep -v _test` lists only `projection.go:242` and the nil fallback `global_blocking.go:115`. `.globalBlocking(` and `aclExpander{` have one caller each (`acl.go:108,118`).
   - **No production nil:** `SplitUnboundPhysical` always returns `make(map…)` (`hostnics.go:19`), so `boundIfs` is never nil. The only production caller of `desired.ACL` is `projection.go:411`, so nil reaches `ACL` only from tests.
   - **Same `ds` throughout:** `ds` is not reassigned in `projectOwned` between `:242` and `:411`. `boundIfs` is read-only (the only `ifs[...] =` in `acl.go:639,642` writes a local `map[string]bool`).
   - **The split was already unconditional at base** (`99a774734` `projection.go:241`), so acl-only projections gain no netlink cost.
   - **The guard test bites.** With mutation A (the acl domain re-runs the split with the lookup), `TestProjectAutoBlockOverlayAddsNoNetdevLookups` fails at `:143` with `map[ens192:2 lan:2] vs map[ens192:1 lan:1]`; the other five tests stay green. The check is one-directional (it only asserts that acl adds none), which is exactly the property claimed.

2. **MINOR #2 (no test varies the netdev answer within one service lifetime): FIXED.**
   - **Lookup that changes over time:** `nicLookup(kernel)` closes over a map the test mutates, and `nicsSvc` installs it as `s.netdevKind`.
   - **`TestAutoBlockWithUnboundAndReleasedNICsOnFake` (`:153-213`) covers all four requested steps:**
     - `AutoBlockSet` OK with ens192 unbound;
     - `AddInterface("ens192")`, the lookup flipped, `Resync`: ens192 bound to `_gb.auto-block.i00` and admin-up;
     - a second `Resync` is APPLIED with `len(Results)==0` (`:200-202`);
     - `lan` released in an interfaces commit: APPLIED, binding gone, admin-down; ens192 and loop712 keep the ACL (`:205-213`).
   - **`TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake` (`:219-248`)** covers the user-list half: `Retrieve` equals desired before and after the bind, re-apply summary 0, resync empty, a later bind picked up.
   - **`len(r.GetResults()) == 0` is the right idempotence signal.**
     - `fillResponse` (`service.go:1271-1281`) copies `res.Results`. The scheduler appends one per executed op (`reconciler.go:1030`) plus plan issues, skips and verify failures (`:781,821,833`), so no results means no create, update, delete or recreate.
     - Reapplier calls are counted separately (`TxnResult.Reapplied`, not in `Results`). That is resync's normal re-assertion, not a change.
     - The paired `GetStatus()==APPLIED` check rules out a nil response (lock error), where `Results` would also be empty.
     - My `-v` run log shows the second resync as `summary=unchanged:15 reapplied=9`.

3. **NIT #3 (inaccurate doc comment for nil): FIXED.** `acl.go:91-96` now reads "nil: the split without a netdev lookup, which keeps every engine-owned row (an unbound NIC cannot be recognised) and drops the released rows". This matches `hostnics.go:22-29` and the nil case of `TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs` (`:271-283`).

4. **NIT #4 (memo depended on `sortedKeys` returning non-nil): FIXED, but no test pins it** (see V2).
   - The memo now uses an explicit `computed` flag (`global_blocking.go:110,112,128`).
   - Mutation D (the flag never set) passes all six committed tests.

5. **NIT #5 ("selects no interface" untested for the overlay case): FIXED.** `TestGlobalBlockingAllInterfacesNothingBound` (`global_blocking_test.go:290-310`) checks three things:
   - exactly two warnings: the skipped NICs, then "selects no interface";
   - no `acl.interface-binding/*` key;
   - no `_gb.` ACL.

**The 15 nil callers** (`acl_test.go` ×5, `rule_expiry_test.go` ×3, `global_blocking_test.go` ×6 plus the deliberate nil case at `:275`, `subsystems/acl_pbr_test.go:74`):
- Nil means exactly what the old nil lookup meant: `SplitUnboundPhysical(ifs, nil)`.
- None of them has a physical row. `grep Physical` hits only the two new tests. The one other all-interfaces list (`:126`) uses loopbacks only, so no new warning appears and nothing is hidden.

**A row missing from `kept` that is not physical cannot occur.** `kept` comes from the same `ds` and `SplitUnboundPhysical` only drops KindExisting rows with `physical`. A row with `owner: host` that is not KindExisting is kept, and so is never classified.

### New findings

**V1 — MINOR: the "never silent" claim is not met on the auto-block runtime reconcile, the main acl-only path.**
- **Where:** `apps/agent/internal/desired/global_blocking.go:106-108` (the comment names "auto-block reconcile"), `docs/status/tasks/agent-allifs-unbound-nics.md:9` ("so an acl-only transaction never drops a binding silently"), `apps/agent/internal/agent/rpc_autoblock.go:112-115`, `apps/agent/internal/agent/service.go:660`.
- **What happens:** `reconcileAutoBlockLocked` runs `applyLocked(modeRuntime, …, []string{"acl"})` and checks only `resp.GetStatus()`. `resp.Validation`, which carries the new `acl.global-blocking` warning, is dropped. `AutoBlockSetResponse` has only `ActiveEntries`. The `reconcile done` log line has status, summary and err, but no issues. `lastResp` is read only by the revert path (`service.go:935`).
- **Scenario:** R2's name-clash case. VPP owns X and a kind-less kernel netdev named X appears. The next `AutoBlockSet` or expiry reconcile deletes `acl.interface-binding/X`. The only trace is `summary=deleted:N`; the warning naming X is computed and discarded.
- **Where it is not silent:** security, interfaces and acl commits do surface the warning (`service.go:563-566`).
- **Fix, either of:**
  - (a) In `reconcileAutoBlockLocked`, or `applyLocked` for `modeRuntime`, log WARNING-severity projection issues when their set changes. De-duplicate by message set, so the 1 s expiry ticker does not spam.
  - (b) Narrow the comment and status wording to "commit responses carry it", and add the auto-block path to the #238 name-clash debt row.
- **[other: R2]:** whether this satisfies R2 MINOR-1 option (a) is R2's call.

**V2 — NIT: the memo flag and two all-interfaces lists in one projection are untested.**
- **Where:** `global_blocking.go:110-131`, `global_blocking_test.go`.
- **Realistic case:** a user `allInterfaces` list plus the auto-block overlay in the same projection.
- **What breaks without the flag:** with mutation D, the second list's warning reads "not bound to the data plane yet: ens192, ens192; released to the host: ens161, ens161", and the committed suite stays green.
- **Evidence:** my scratch test `TestReviewR1TwoAllInterfacesLists` (overlay-added, never written to the worktree) checks two lists, one warning each, each NIC named once, loopbacks bound to `_gb.a.i00,_gb.b.i00,_gb.pass`. It passes at head and fails under mutation D.
- **Fix:** add that case.

**V3 — NIT: the overlay warning has the wrong severity and points at a path the user cannot have.**
- **Where:** `global_blocking.go:163-172`; the test pins the pointer at `physical_nics_acl_test.go:115`.
- **Severity:** a released NIC is a deliberate operator state. The interfaces domain reports it at INFO (`projection.go:250`, `agent.nic-released`), but the new list warning reports it at WARNING.
- **Pointer:** for the overlay list, the pointer is `/acl/globalBlocking/lists/auto-block/interfaces`, a reserved list that the user's document is forbidden to contain (`service.go:1413-1414`).
- **Effect:** after #238 a released management NIC is normal. So every interfaces or security commit made while auto-block entries are live (the overlay list is non-empty only then) carries a WARNING the operator cannot act on, pointing at nothing in their document. There is precedent for the pointer (the reserved-list error, the "selects no interface" warning), so this is not a regression of convention.
- **Fix:** emit the released-only part at INFO (keep WARNING for unbound), and for the overlay point at `/security/autoBlock`.
- **[other: R6]**

**V4 — NIT: the WIP red-run claim is wrong.**
- **Where:** `docs/status/tasks/agent-allifs-unbound-nics-wip.md:6` ("red with the pre-fix line put back — the six tests fail"), repeated in the manager brief.
- **What actually happens:** the pre-fix mutation (`allIfs = sortedKeys(ds.GetInterfaces())`) fails five tests. `TestProjectAutoBlockOverlayAddsNoNetdevLookups` passes under it; it is red only against an acl-side re-lookup (mutation A above).
- **The paste itself is honest:** `red.log` and the status-file paste both show five `--- FAIL` lines.
- **Fix:** say "five fail" and cite the re-lookup mutation for the sixth.
- **[other: R7]**

### Notes outside R1 (not graded)

- **[other: manager/R7] The PR cannot merge as it stands.**
  - `gh pr view 241` reports `mergeable: CONFLICTING`, `mergeStateStatus: DIRTY`, `statusCheckRollup: []` on `873f1d946`. No hosted quick gate result exists for this head.
  - The branch must be brought up to date with main and the hosted gate run.
  - If the conflict resolution touches `apps/agent`, the product verify must be repeated on the new tree.
- **Gate evidence matches the logs:**
  - `agent-gate-2.log`: `ok=143`, `FAIL=0`, `0 issues.`, `AGENT_GATE_EXIT=0`.
  - `ci-quick-2.log`: header `@ 0b57dc070`, `CI GATE PASSED`, `CI_QUICK_EXIT=0`, wall time 5m09s from 11:57:04Z.
  - `/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-115704-1958055/09-agent.log` embeds `main.version=0b57dc070`, with 143 `ok` and 0 FAIL.
  - `873f1d946` was committed at 12:03:06Z, after the gate ended (about 12:02:13Z), so the round-1 SHA-mismatch note is resolved.

### Commands run (worktree `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`; Go from `apps/agent`; `D=<scratchpad>/review-R1-verify`)

```
$ git rev-parse HEAD ; git status --porcelain
873f1d946a0c8e65f08603db4891025560bcaa66
(empty)
$ git diff --stat 99a774734...873f1d946 -- apps packages tools deploy test
 physical_nics_acl_test.go 249 + · projection.go 5 · acl.go 11 · acl_test.go 10 · global_blocking.go 45 · global_blocking_test.go 90 · rule_expiry_test.go 6 · subsystems/acl_pbr_test.go 2   (8 files, +396 -22)
$ git diff --stat 856bf743a 873f1d946 -- apps packages tools deploy test
(empty)

$ env -u NGFW_INTEGRATION go test -count=1 -v -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake' ./internal/desired/ ./internal/agent/
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.02s)
--- PASS: TestGlobalBlockingAllInterfacesNothingBound (0.00s)
ok  	ngfw/agent/internal/desired	0.183s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.02s)
--- PASS: TestProjectAutoBlockOverlayAddsNoNetdevLookups (0.00s)
  reconcile done txn_id=nics-config  status=APPLIED summary="created:1 unchanged:1"
  reconcile done mode=auto-block domains=[acl] status=APPLIED summary=created:6
  reconcile done txn_id=nics-update domains=[interfaces] status=APPLIED summary="created:4 unchanged:8"
  reconcile done mode=resync status=APPLIED summary="created:2 unchanged:13" reapplied=7
  reconcile done mode=resync status=APPLIED summary=unchanged:15 reapplied=9
  reconcile done txn_id=nics-release domains=[interfaces] status=APPLIED summary="deleted:2 unchanged:12"
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.08s)
  reconcile done txn_id=all1 status=APPLIED summary="created:10 unchanged:1"
  reconcile done txn_id=all2 status=APPLIED summary=unchanged:11
  reconcile done mode=resync status=APPLIED summary=unchanged:11 reapplied=6
  reconcile done mode=resync status=APPLIED summary="created:2 unchanged:12" reapplied=7
--- PASS: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.05s)
ok  	ngfw/agent/internal/agent	0.323s
exit=0          (log lines abridged to the fields shown)

$ env -u NGFW_INTEGRATION go test -race -count=1 -run 'TestGlobalBlocking|TestACL|TestParseGlobalBlocking|TestProjectPhysicalNic|TestProjectAutoBlock|TestAutoBlock|TestAllInterfaces|PhysicalNIC|PhysicalNic|TestPBRPolicyNamesFACLList|RuleExpiry|Expiry' ./internal/desired/ ./internal/agent/ ./internal/subsystems/
ok  	ngfw/agent/internal/desired	26.277s
ok  	ngfw/agent/internal/agent	5.914s
ok  	ngfw/agent/internal/subsystems	1.738s
exit=0

$ env -u NGFW_INTEGRATION go test -count=1 -v -run 'Project|Global|ACL|Acl|AutoBlock|AllInterfaces|Nic|NIC|Split|Unbound|Expiry' ./internal/desired/ ./internal/agent/ ./internal/subsystems/ > $D/broad.log
exit=0 ; ok desired 7.723s · ok agent 7.909s · ok subsystems 2.670s ; PASS=157 FAIL=0 SKIP=3
(the 3 SKIPs are TestHostNicsOnHost, TestSrv6GlobalsOnHost and TestUntaggedNICClaimsOnHost: "integration test: set NGFW_INTEGRATION=1" — true on this host)

$ go vet ./internal/desired/ ./internal/agent/          → vet_exit=0
$ gofmt -l <the 5 changed Go files>                      → (no output)

# Mutations via -overlay (scratch copies made with git show 873f1d946:<file> | sed; the worktree is untouched)
# A: projection.go:411 → desired.ACL(p, ds, in, func() map[string]*ngfwv1.Interface { k, _, _ := desired.SplitUnboundPhysical(ds.GetInterfaces(), netdev); return k }())
# B: projection.go:411 → desired.ACL(p, ds, in, nil)
# C: global_blocking.go:117 → allIfs = sortedKeys(ds.GetInterfaces())     (the developer's red)
# D: global_blocking.go:128 → `_ = computed` (the memo flag is never set)
$ env -u NGFW_INTEGRATION go test -overlay=$D/ov-<m>.json -count=1 -v -run '<the six tests>' ./internal/desired/ ./internal/agent/
A: exit=1  only FAIL TestProjectAutoBlockOverlayAddsNoNetdevLookups — physical_nics_acl_test.go:143: the acl projection must not look netdevs up again: map[ens192:2 lan:2] vs map[ens192:1 lan:1]
B: exit=1  FAIL TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (:108 no binding for ens192…), TestAutoBlockWithUnboundAndReleasedNICsOnFake (:165 …Unavailable… acl.interface-binding/ens192: mandatory dependency…), TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (:226 APPLY_STATUS_FAILED); desired pkg ok
C: exit=1  5 FAIL (global_blocking_test.go:266, :303; physical_nics_acl_test.go:108, :165, :226); PASS TestProjectAutoBlockOverlayAddsNoNetdevLookups
D: exit=0  all six PASS
$ env -u NGFW_INTEGRATION go test -overlay=$D/ov-T.json  -count=1 -v -run TestReviewR1TwoAllInterfacesLists ./internal/desired/   → --- PASS, ok
$ env -u NGFW_INTEGRATION go test -overlay=$D/ov-TD.json -count=1 -v -run TestReviewR1TwoAllInterfacesLists ./internal/desired/   → --- FAIL zz_review_r1_test.go:32: … list "b" … (not bound to the data plane yet: ens192, ens192; released to the host: ens161, ens161)
$ ls apps/agent/internal/desired/zz_review_r1_test.go → No such file or directory ; git status --porcelain → (empty)

$ grep -c '^ok' agent-gate-2.log → 143 ; grep -c -E '^(FAIL|--- FAIL)' agent-gate-2.log → 0 ; AGENT_GATE_EXIT=0
$ grep 'branch\|CI GATE\|CI_QUICK_EXIT\|wall time' ci-quick-2.log → branch … @ 0b57dc070 · wall time 5m09s · CI GATE PASSED · CI_QUICK_EXIT=0
$ grep -ho 'main.version=[0-9a-f]*' /root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-115704-1958055/09-agent.log → main.version=0b57dc070 (143 ok, 0 FAIL)
$ git log -4 --format='%h %cI %s' 873f1d946 → 873f1d946 12:03:06Z · 0b57dc070 11:52:50Z · 856bf743a 11:52:49Z · 20c3303fd 11:46:10Z
$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json headRefOid,mergeable,mergeStateStatus,statusCheckRollup
{"headRefOid":"873f1d946a0c8e65f08603db4891025560bcaa66","mergeStateStatus":"DIRTY","mergeable":"CONFLICTING","statusCheckRollup":[]}
```

Not run, as the brief required: `tools/ci.sh`, `make`, `pnpm`, `turbo` and whole-tree `go test ./...`. Instead I checked the gate outputs pasted for `0b57dc070` against the logs, as above.

**Verdict line: R1 APPROVE — 0 BLOCKER, 0 MAJOR, 1 MINOR (V1), 3 NIT (V2–V4).**
- Round-1 #1–#5 are all fixed.
- The developer may fix the new MINOR and NITs or answer won't-fix in one line each.
- Merge still needs the conflict with main resolved and a green hosted Mandatory quick gate on the resulting head.
