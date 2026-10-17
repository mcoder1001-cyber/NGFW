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
