# TD-11b: claim safety (persistent stores + create-then-claim)

Branch `task/TD-11b`, base `task/P08@998e391`. P08 has since merged on main as c2ca3ed; the merger rebases this branch. Slot 1 (w1). Review REVIEW-2026-09-24 §3: items 3.2, 3.3, 3.3b (generic half), and R2-stores.

## خلاصهٔ فارسی
- **ادعای مالکیت پیش از نوشتن در VPP:** شش توصیف‌گر ویژگی DF-1، `Descriptor[T]` عمومیِ natcommon و توصیف‌گرهای keyed در df6 حالا پیش از فراخوانی VPP ادعای مالکیت را ثبت می‌کنند.
  - اگر ثبت ادعا ممکن نباشد، Create با خطا تمام می‌شود و چیزی در VPP نوشته نمی‌شود.
  - اگر نوشتن در VPP شکست بخورد، ادعایی که همین Create ثبت کرده آزاد می‌شود.
- **حفاظ پایداری:** اگر توصیف‌گری با مخزن درون‌حافظه‌ای (claim یا boot) ثبت شود، `subsystems.Register` خطا می‌دهد و عامل بالا نمی‌آید. این حفاظ از طریق `dfkit/persist` کار می‌کند و برای هر خانواده آزمون واحد دارد.
- **Create نیمه‌کاره:** زمان‌بند فقط Create‌ای را در ژورنال می‌گذارد که Meta را همراه `scheduler.PartialCreate(err)` برگرداند. rollback همین شیء نیمه‌کاره را حذف می‌کند.
  - انحراف آگاهانه از متن پاکت: ژورنال کردن هر Meta غیر nil باعث می‌شد rollback نشانی‌ای را که هرگز اضافه نشده حذف کند و عامل DEGRADED شود. مورد مستند و آزموده است (پرسش Q2).
- **R2-stores:** تازه‌سازی نمایهٔ ادعا (sw_if_index) حالا با context فراخواننده محدود می‌شود، نه با ۵ ثانیهٔ خودِ مخزن.
- **یافتهٔ تازه:** ادعاهای keyed در df6 از راه `IfaceClaims` همیشه شکست می‌خوردند. `PairClaims` برای همین اضافه شد.
- **اثبات روی میزبان:** فرایند تازهٔ عامل، با همان state dir، ادعاهای رابط بدون برچسب را بازیابی کرد و resync آن هیچ شیئی نساخت. NRestarts=1 → 1.
- **CI:** گیت CI سبز شد.

## Items verified on the base (998e391) before changing them
| item | on the base | done here |
|---|---|---|
| 3.2 in-memory default stores, no guard | open. natcommon `New`/`BuildConfig`, `iface.Claims`, `df2.BuildOptions`, `df6.BuildOptions` and dfkit all fall back to memory; only ipsec has `Persistent()` | guard: `dfkit/persist` + `CheckPersistent` per family; `subsystems.Register` refuses to start |
| 3.3 VPP written first, claim recorded after; `executor.create` drops Meta on error | open. natcommon `descriptor.go:104-115`; `attributes.go:137/260/415/546/705/853`; `df6/keyed.go:165-171`; `reconciler.go:928-933` | claim first in natcommon, the six DF-1 attributes and df6 keyed; `dfkit.Target.ClaimFirst` and `df2.ClaimFirst` for the other call sites; the scheduler journals partial Creates |
| 3.3b wireguard meta+error | open (wireguard is not wired) | generic half: `scheduler.PartialCreate`; F-wireguard wraps its error (questions Q2) |
| R2-stores: claim refresh has its own 5 s | open (`subsystems.go:118-119`); reproduced below: Create fails after 10.4 s with a 30 s caller deadline | the refresh runs within the caller's ctx (`IfaceClaims.ClaimContext/ClaimedContext`, `IndexCache.Resolve(ctx, …)`) |
| DF-2 N5 / DF-6 N7 claim hygiene (only in these files) | open | df2/df6 `FileClaimStore`: memory changes only when the write succeeds; df6 keyed claim-first; df6 store split caught by the guard; the rest is in questions Q4 |

## What changed (24 files under apps/agent, 2 329 insertions / 81 deletions; the file-by-file list is in `git diff --stat 998e391`)
- **`scheduler/reconciler.go`** (the `executor.create` hunk plus `isNilMeta` below it) and **`scheduler/descriptor.go`**:
  - `ErrPartialCreate` / `PartialCreate(err)` and the documented `Create` contract;
  - a Create that returns Meta with a PartialCreate error is journaled and made live, so the rollback Deletes it.
- **`descriptors/interface/attributes.go`:**
  - `claimFirst`, which uses the store's ctx-bounded claim when it has one (`iface.ContextClaimStore`);
  - the six Creates now go resolve → claim → write → undo on failure;
  - `base.CheckPersistent`.
- **`descriptors/natcommon/descriptor.go`:**
  - Create claims first and releases on failure, unless the failure is partial (the claim then stays for the rollback's Delete);
  - `CheckPersistent`, which Global singletons skip.
- **`descriptors/df6`:**
  - keyed claim-first;
  - `CheckPersistent` on keyed and bypass, with the new `ErrClaimStoreKind`: an interface-bound store is refused for id claims;
  - `FileClaimStore` write hygiene, plus `Persistent()`;
  - bypass returns a PartialCreate when one family was enabled.
- **`descriptors/df2/claims.go`:** `Options.CheckPersistent`, `ClaimFirst`, `FileClaimStore` write hygiene, `Persistent()`.
- **`descriptors/dfkit`:**
  - `persist/`: the guard protocol (`Persistent()`, `CheckPersistent()`, `Check` through wrappers, `Require`);
  - `claimfirst.go`: `CheckClaims`, `CheckBoot`, `FileBootStore.Persistent`, `Target.ClaimFirst` → `Claim.Adopt` / `Claim.Undo`.
- **`subsystems/stores.go`:**
  - the exported `Register` runs the guard over every descriptor registered through it;
  - `RequirePersistent` / `ErrVolatileStores`;
  - the ctx-bounded refresh (context-less `Claim`/`Claimed` keep a 5 s cap);
  - `PairClaims` and `Wiring.PairClaims`;
  - `Persistent()` / `BindsInterfaceIndex()`.
- **`subsystems/subsystems.go`:** `register` (rename), the refresh closure takes ctx, and the `IfaceClaims` doc (questions Q1).
- **`agent/claims_restart_integration_test.go`:** the host proof.

## Every behaviour change: the new test FAILS on the base first
These are the new tests (the subset that compiles against the base API, with PartialCreate unwrapped) run on an export of 998e391:
```
$ go test -count=1 -run 'Partial|FailedCreateWithoutMeta' ./internal/scheduler/
--- FAIL: TestPartialCreateIsRolledBack (0.00s)
    partial_create_test.go:63: ops = create a/base,create p/x,delete a/base
--- FAIL: TestPartialCreateRollbackFailureDegrades (0.00s)
    partial_create_test.go:88: outcome ROLLED_BACK err create p/x: event subscription failed
--- FAIL: TestPartialCreateInRecreateRestoresOld (0.00s)
    partial_create_test.go:122: outcome DEGRADED err create p/x: claim store: no identity
FAIL
FAIL	ngfw/agent/internal/scheduler	0.021s
FAIL
$ go test -count=1 -run 'TestAttribute' ./internal/descriptors/interface/
--- FAIL: TestAttributeClaimsBeforeWrite (0.00s)
        claimfirst_test.go:129: sw_interface_set_flags sent 1 time(s) although the claim failed (VPP written, claim missing)
        claimfirst_test.go:129: sw_interface_set_mtu sent 1 time(s) although the claim failed (VPP written, claim missing)
        claimfirst_test.go:129: sw_interface_set_mac_address sent 1 time(s) although the claim failed (VPP written, claim missing)
        claimfirst_test.go:129: sw_interface_set_promisc sent 1 time(s) although the claim failed (VPP written, claim missing)
        claimfirst_test.go:129: sw_interface_set_rx_mode sent 1 time(s) although the claim failed (VPP written, claim missing)
        claimfirst_test.go:129: sw_interface_set_rx_placement sent 1 time(s) although the claim failed (VPP written, claim missing)
--- FAIL: TestAttributeClaimUsesCallerContext (0.00s)
        claimfirst_test.go:176: ctx claims 0, claimed true
        claimfirst_test.go:176: ctx claims 0, claimed true
        claimfirst_test.go:176: ctx claims 0, claimed true
        claimfirst_test.go:176: ctx claims 0, claimed true
        claimfirst_test.go:176: ctx claims 0, claimed true
        claimfirst_test.go:176: ctx claims 0, claimed true
$ go test -count=1 -run 'TestGeneric|TestGlobalCreateDoesNotClaim' ./internal/descriptors/natcommon/
--- FAIL: TestGenericCreateClaimsBeforeVPP (0.00s)
    claimfirst_test.go:107: VPP written although the claim failed: writes=1 objs=map[a:true] (invisible, unjournaled object)
--- FAIL: TestGenericPartialCreateKeepsClaimForRollback (0.00s)
    claimfirst_test.go:149: the partial object's claim was dropped: Retrieve cannot see it, the rollback cannot prove it is ours
FAIL
FAIL	ngfw/agent/internal/descriptors/natcommon	0.020s
FAIL
$ go test -count=1 -run 'TestKeyedClaimsBeforeAdd|TestFileClaimStoreWriteFailure|TestBypassPartialCreateReturnsMeta' ./internal/descriptors/df6/
--- FAIL: TestKeyedClaimsBeforeAdd (0.00s)
    claims_td11b_test.go:114: added although the claim failed: adds=1 ids=map[sid-1:true] (unclaimed object: ErrNotOurs forever)
--- FAIL: TestFileClaimStoreWriteFailure (0.00s)
    claims_td11b_test.go:154: claim kept in memory although it was never written
--- FAIL: TestBypassPartialCreateReturnsMeta (0.00s)
    claims_td11b_test.go:187: ip4 bypass is enabled (1) but Create returned <nil>, vxlan.bypass: sw_interface_set_vxlan_bypass: VPPApiError: Unimplemented (-9): the rollback cannot undo it
FAIL
FAIL	ngfw/agent/internal/descriptors/df6	0.026s
FAIL
$ go test -count=1 -run 'TestFileClaimStoreWriteFailure' ./internal/descriptors/df2/
--- FAIL: TestFileClaimStoreWriteFailure (0.00s)
    claims_td11b_test.go:38: claim kept in memory although it was never written
FAIL
FAIL	ngfw/agent/internal/descriptors/df2	0.020s
FAIL
$ go test -count=1 -run 'TestClaimRefresh' ./internal/subsystems/
--- FAIL: TestClaimRefreshBoundedByCallerContext (10.41s)
    claims_td11b_test.go:89: Create failed after 10.412s although the caller's deadline is 30 s: claim store: interface has no sw_if_index in VPP: "ens224" (claim ens224|interface.admin-state not recorded)
--- FAIL: TestClaimRefreshCancelledWithCaller (0.30s)
    claims_td11b_test.go:105: Create = sw_interface_dump: context deadline exceeded, want the claim to end with the caller's deadline
FAIL
FAIL	ngfw/agent/internal/subsystems	10.750s
FAIL
```
The guard tests (`TestCheckPersistent` per family, `TestRequirePersistentPerFamily`, `TestRegisterGuardsEveryDescriptor`), `TestClaimFirst` and `TestPairClaims` use API that does not exist on the base (`persist`, `CheckPersistent`, `ClaimFirst`, `PairClaims`), so they do not compile there. That is the "fail" for those items. On the base, nothing refused an in-memory store (3.2 above).

## Same tests on the branch
```
$ go test -count=1 -v -run 'Partial|FailedCreate|IsNilMeta' ./internal/scheduler
--- PASS: TestPartialCreateIsRolledBack (0.00s)
--- PASS: TestPartialCreateRollbackFailureDegrades (0.00s)
--- PASS: TestFailedCreateWithMetaButNotPartialIsNotJournaled (0.00s)
--- PASS: TestFailedCreateWithoutMetaIsNotJournaled (0.00s)
--- PASS: TestPartialCreateInRecreateRestoresOld (0.00s)
--- PASS: TestIsNilMeta (0.00s)
ok  	ngfw/agent/internal/scheduler	0.016s
$ go test -count=1 -v -run 'TestAttribute|TestCheckPersistent' ./internal/descriptors/interface
--- PASS: TestAttributeClaimsBeforeWrite (0.01s)
--- PASS: TestAttributeWriteFailureReleasesNewClaim (0.00s)
--- PASS: TestAttributeClaimUsesCallerContext (0.00s)
--- PASS: TestCheckPersistent (0.00s)
--- SKIP: TestAttributesOnHost (0.00s)
ok  	ngfw/agent/internal/descriptors/interface	0.059s
$ go test -count=1 -v -run 'TestGeneric|TestGlobalCreateDoesNotClaim|TestCheckPersistent' ./internal/descriptors/natcommon
--- PASS: TestGenericCreateClaimsBeforeVPP (0.00s)
--- PASS: TestGenericCreateFailureReleasesNewClaim (0.00s)
--- PASS: TestGenericPartialCreateKeepsClaimForRollback (0.00s)
--- PASS: TestGlobalCreateDoesNotClaim (0.00s)
--- PASS: TestCheckPersistent (0.00s)
--- PASS: TestGenericDescriptor (0.00s)
ok  	ngfw/agent/internal/descriptors/natcommon	0.052s
$ go test -count=1 -v -run 'TestKeyedClaimsBeforeAdd|TestFileClaimStoreWriteFailure|TestBypassPartialCreateReturnsMeta|TestCheckPersistent' ./internal/descriptors/df6
--- PASS: TestKeyedClaimsBeforeAdd (0.01s)
--- PASS: TestFileClaimStoreWriteFailure (0.00s)
--- PASS: TestCheckPersistent (0.00s)
--- PASS: TestBypassPartialCreateReturnsMeta (0.00s)
ok  	ngfw/agent/internal/descriptors/df6	0.071s
$ go test -count=1 -v -run 'TestFileClaimStoreWriteFailure|TestClaimFirst|TestCheckPersistent' ./internal/descriptors/df2
--- PASS: TestFileClaimStoreWriteFailure (0.00s)
--- PASS: TestClaimFirst (0.00s)
--- PASS: TestCheckPersistent (0.00s)
ok  	ngfw/agent/internal/descriptors/df2	0.034s
$ go test -count=1 -v -run 'TestClaimFirst|TestCheckPersistent|TestIsAndRequire|TestCheckSeesThroughWrappers' ./internal/descriptors/dfkit/...
--- PASS: TestClaimFirst (0.00s)
--- PASS: TestCheckPersistent (0.00s)
ok  	ngfw/agent/internal/descriptors/dfkit	0.040s
--- PASS: TestIsAndRequire (0.00s)
--- PASS: TestCheckSeesThroughWrappers (0.00s)
ok  	ngfw/agent/internal/descriptors/dfkit/persist	0.022s
ok  	ngfw/agent/internal/descriptors/dfkit/restarttest	0.018s [no tests to run]
$ go test -count=1 -v -run 'TestClaimRefresh|TestRegisterGuards|TestRequirePersistentPerFamily|TestPairClaims' ./internal/subsystems
--- PASS: TestClaimRefreshBoundedByCallerContext (5.21s)
--- PASS: TestClaimRefreshCancelledWithCaller (0.30s)
--- PASS: TestRegisterGuardsEveryDescriptor (0.00s)
--- PASS: TestRequirePersistentPerFamily (0.00s)
--- PASS: TestPairClaims (0.00s)
ok  	ngfw/agent/internal/subsystems	5.599s
```

## Host proof (slot w1, real vrx-agent binary, real VPP; no trace commands, D-128)
Run as `eval "$(tools/lab env 1)"; VRX_INTEGRATION=1 go test -count=1 -v -run TestUntaggedClaimsSurviveAgentRestartOnHost ./internal/agent/`, under the shared lab lock.

What the test does:
- It creates an untagged `tap173` directly through the binary API (the stand-in for a DPDK NIC, as in the DF-1 alias host test).
- Agent process 1 applies `enabled` + `mtu 1400`, which records 2 claims bound to the tap's sw_if_index.
- Process 1 gets SIGTERM (by PID). Process 2 starts on the same state dir, Retrieves the same state, and its first resync creates nothing.
- The test then removes both objects through the agent (claims released) and deletes the tap in Cleanup.

```
before: NRestarts=1
=== RUN   TestUntaggedClaimsSurviveAgentRestartOnHost
    claims_restart_integration_test.go:61: untagged tap173 sw_if_index 2 (no tag: ours only through a claim)
    claims_restart_integration_test.go:92: agent process 1: pid 181072
    claims_restart_integration_test.go:152: process 1: applied; Retrieve tap173 = {"enabled":true, "mtu":1400, "vrf":"default", "promiscuous":false}; 2 claims bound to sw_if_index 2: [map[boot:a93c0e7a-40b7-4755-9b0b-07eefa24137d/2006833/3203894 key:tap173|interface.admin-state sw_if_index:2] map[boot:a93c0e7a-40b7-4755-9b0b-07eefa24137d/2006833/3203894 key:tap173|interface.mtu sw_if_index:2]]
    claims_restart_integration_test.go:92: agent process 2: pid 181763
    claims_restart_integration_test.go:167: process 2: {"time":"2026-09-24T23:14:04.130275462+03:30","level":"INFO","msg":"reconcile done","owner":"w1","txn_id":"","mode":"resync","domains":["interfaces"],"status":"APPLY_STATUS_APPLIED","summary":"unchanged:3","reapplied":0,"duration":18313244,"err":""}
    claims_restart_integration_test.go:167: process 2: {"time":"2026-09-24T23:14:04.130337669+03:30","level":"INFO","msg":"resync finished","owner":"w1","status":"APPLY_STATUS_APPLIED","summary":"unchanged:3"}
    claims_restart_integration_test.go:175: process 2: Retrieve == process 1's, first resync created nothing
    claims_restart_integration_test.go:186: removed through the agent: summary deleted:2, claims file empty
--- PASS: TestUntaggedClaimsSurviveAgentRestartOnHost (9.29s)
PASS
ok  	ngfw/agent/internal/agent	9.360s
after: NRestarts=1
```
Also run on w1, one package at a time: the DF-1 host tests (`TestAliasOnHost`, `TestAttributesOnHost`, `TestRestartSimulationOnHost`) and the agent host tests (`TestAgentOnHost`, `TestAgentProcessOnHost`).
```
before: NRestarts=1
ok  	ngfw/agent/internal/descriptors/interface	3.515s
after interface: NRestarts=1
--- PASS: TestAgentOnHost (5.27s)
--- PASS: TestAgentProcessOnHost (7.30s)
    claims_restart_integration_test.go:61: untagged tap173 sw_if_index 2 (no tag: ours only through a claim)
--- PASS: TestUntaggedClaimsSurviveAgentRestartOnHost (6.89s)
PASS
ok  	ngfw/agent/internal/agent	19.510s
after agent: NRestarts=1
```
After the runs, `vppctl show interface` has no `tap173` and there is no `w1-tap73` netdev left.

## CI
The gate ran as `TMPDIR=/tmp/g-w1 tools/ci.sh --base main` at c0372fd.
- The first run, at cf2a04e, failed on one staticcheck QF1001 finding in natcommon; c0372fd fixes it.
- After that run, the only change is a doc comment in `subsystems.go` (the `IfaceClaims` note). `go vet` and golangci-lint pass on that package: 0 issues.
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   2m28s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   1m58s
  apps/agent: make lint test build                   1m30s
  apps/cli: make lint test build                     0m21s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m07s
  deploy/vpp: shellcheck + apply-startup fake-host harness   5m06s
  mode quick · wall time 11m38s · logs /root/ngfw-wt/logs/ci/TD-11b-20260924-232148-302976

CI GATE PASSED
```

## Out of scope / left for other rows
Questions file: `docs/status/tasks/TD-11b-questions.md`.
- **Call sites in files I do not own:**
  - dhcp.client (live since P08) still claims after the add;
  - the DF-7/DF-8 call sites that claim after the add;
  - the df2 consumers (urpf/adl/abf/arp/ip6_nd/classify).
  - All of them get helpers here (`Target.ClaimFirst`, `df2.ClaimFirst`, `CheckPersistent` helpers) and are listed with their owning rows in Q3.
- **wireguard 3.3b:** F-wireguard wraps the error with `scheduler.PartialCreate`.
- **Not claim-store hygiene:** DF-2 N3, DF-6 N4/N6, and the rest of N7 (Q4). These stay tech-debt.
- **Owned by other rows:** the TD-11c flush-per-txn / KeyedClaims batching, TD-9's ApplyWith / rollback ctx / DefaultReplyTimeout.

## Fix round 1 (review be3a57a5: APPROVE WITH CHANGES; the manager asked for M2, M1, L1 and L5; M3 waits for the manager's decision and is untouched)
Unit tests only. There were no host runs and no trace commands (D-128).

| finding | fix | commit |
|---|---|---|
| **M2**: a `PartialCreate` with nil Meta was dropped (not journaled; natcommon also released the claim) | One shared predicate, `scheduler.IsPartialCreate(err)`, used by both `executor.create` and natcommon's Create. The marker alone decides, and a nil Meta is journaled too. The rollback calls `Delete(obj, nil)`: a key-addressed descriptor deletes correctly, and one that needs the Meta fails loudly (DEGRADED). Both copies of `isNilMeta` are removed. | c74a0319 |
| **M1**: the guard checked only descriptors that implement `CheckPersistent` (it failed open) | Completeness is now enforced. `persist.Declared`: every descriptor registered through `Register` must declare exactly one of `CheckPersistent` / `RecordsNoOwnership` (the marker is `persist.NoOwnership`), looked for through wrappers. If none does, or both do, the result is `ErrUndeclaredDescriptors` and the agent refuses to start. Declarations were added for the product set, in new files only (see the list below). | 4c19106d |
| **L1**: a refresh without a caller deadline was unbounded while it held the cache mutex | `IndexCache.Resolve` caps a ctx without a deadline at `legacyBound` (5 s). A caller's own deadline still wins (R2). | 38591aee |
| **L5**: a crash between claim and write leaves a leftover claim | Documented in the claim-store doc (`subsystems/stores.go`, "Claim-first leftovers") and in `claimFirst` (`interface/attributes.go`), including the admin-state case. See below. | 38591aee |

The M1 declarations for the product set:
- `core/ownership.go`:
  - VRF, loopback, interface-ip.table and interface-ip declare `RecordsNoOwnership`;
  - ip.route has `CheckPersistent` over the owner table (`ownertable.File`, or a store reporting `Persistent()`);
  - `core/ownership_test.go` is a **merge tripwire**: it fails as soon as `core.Env` gains a field, e.g. TD-11c's `Claims`. The merger must then switch interface-ip / interface-ip.table to `CheckPersistent` over it (review Nit).
- `af_packet/ownership.go`: `RecordsNoOwnership` (tagged).
- `dhcp/ownership.go`: `CheckPersistent` = `dfkit.CheckClaims` (the Q3 claim-first change is still owed).

L5, the claim-first leftovers:
- An agent crash between the claim and the VPP write, or a failed release, leaves a claim on nothing.
- It never blocks a later Create: the claim is reused and the absent object is written.
- Within one VPP instance, the only cleanup is `Prune` on a VPP boot-identity change (`Connected`).
- The visible case is `interface.admin-state`. If someone else brings that untagged NIC admin-up, Retrieve reports it as ours, and a resync whose desired state does not name it sets the NIC admin **down**.
- This is the accepted cost of claim-first, which replaces write-then-claim's unowned object in VPP.
- Proposed D-entry, together with Q2. Tech-debt: after the first successful resync, drop claims whose key is neither desired nor present in VPP.

Not in this round (Low, not on the manager's list; left as the review describes them):
- L2: release only on a definite VPP rejection;
- L3: join the release errors in attributes, df6 keyed and df2;
- L4: bypass marks partial only when a family was changed.

### The fix-round tests FAIL on the pre-fix tree
The tree is be3a57a5 with the new tests, in a form that compiles there. For M1 and L1, the pre-fix variant asserts only "refused" / "has a deadline", because the new names do not exist on that tree.
```
pre-fix tree = be3a57a5 + fix-round-1 tests (API-compatible form)
$ go test -count=1 -run 'TestPartialCreateWithNilMetaIsRolledBack|TestPartialCreateNilMetaDeleteNeedsMetaDegrades' ./internal/scheduler/
--- FAIL: TestPartialCreateWithNilMetaIsRolledBack (0.00s)
    partial_create_test.go:177: ops = create k/x, want the partial object deleted by the rollback
--- FAIL: TestPartialCreateNilMetaDeleteNeedsMetaDegrades (0.00s)
    partial_create_test.go:195: outcome ROLLED_BACK results [{Key:p/x Op:create Code:FAILED Err:subscription failed}]
FAIL
FAIL	ngfw/agent/internal/scheduler	0.023s
FAIL
$ go test -count=1 -run 'TestGenericPartialCreateNilMetaKeepsClaim' ./internal/descriptors/natcommon/
--- FAIL: TestGenericPartialCreateNilMetaKeepsClaim (0.00s)
    claimfirst_test.go:228: claim released although VPP was written (partial with nil Meta)
FAIL
FAIL	ngfw/agent/internal/descriptors/natcommon	0.020s
FAIL
$ go test -count=1 -run 'TestRequirePersistentRefusesUndeclared|TestIndexRefreshCappedWithoutDeadline' ./internal/subsystems/
--- FAIL: TestRequirePersistentRefusesUndeclared (0.00s)
    fr1_prefix_test.go:20: undeclared descriptor accepted: the guard only checks descriptors that implement CheckPersistent
--- FAIL: TestIndexRefreshCappedWithoutDeadline (0.00s)
    fr1_prefix_test.go:28: refresh ran without a deadline (a stalled VPP holds the cache mutex forever)
FAIL
FAIL	ngfw/agent/internal/subsystems	0.034s
FAIL
```

### The same tests on the branch (38591aee)
```
$ go test -count=1 -v -run 'Partial|FailedCreate|IsPartialCreate' ./internal/scheduler/
--- PASS: TestPartialCreateIsRolledBack (0.00s)
--- PASS: TestPartialCreateRollbackFailureDegrades (0.00s)
--- PASS: TestFailedCreateWithMetaButNotPartialIsNotJournaled (0.00s)
--- PASS: TestFailedCreateWithoutMetaIsNotJournaled (0.00s)
--- PASS: TestPartialCreateInRecreateRestoresOld (0.00s)
--- PASS: TestPartialCreateWithNilMetaIsRolledBack (0.00s)
--- PASS: TestPartialCreateNilMetaDeleteNeedsMetaDegrades (0.00s)
--- PASS: TestIsPartialCreate (0.00s)
ok  	ngfw/agent/internal/scheduler	0.021s
$ go test -count=1 -v -run 'TestGeneric|TestCheckPersistent' ./internal/descriptors/natcommon/
--- PASS: TestGenericCreateClaimsBeforeVPP (0.00s)
--- PASS: TestGenericCreateFailureReleasesNewClaim (0.00s)
--- PASS: TestGenericPartialCreateKeepsClaimForRollback (0.00s)
--- PASS: TestCheckPersistent (0.00s)
--- PASS: TestGenericPartialCreateNilMetaKeepsClaim (0.00s)
--- PASS: TestGenericDescriptor (0.00s)
ok  	ngfw/agent/internal/descriptors/natcommon	0.033s
$ go test -count=1 -v -run '.' ./internal/descriptors/dfkit/persist/
--- PASS: TestIsAndRequire (0.00s)
--- PASS: TestCheckSeesThroughWrappers (0.00s)
--- PASS: TestDeclared (0.00s)
ok  	ngfw/agent/internal/descriptors/dfkit/persist	0.014s
$ go test -count=1 -v -run 'TestOwnershipDeclared|TestInterfaceObjectsRecordNoStore' ./internal/descriptors/core/
--- PASS: TestOwnershipDeclared (0.00s)
--- PASS: TestInterfaceObjectsRecordNoStore (0.00s)
ok  	ngfw/agent/internal/descriptors/core	0.032s
$ go test -count=1 -v -run 'TestRegisterGuards|TestRequirePersistent|TestIndexRefreshCapped|TestClaimRefresh|TestPairClaims' ./internal/subsystems/
--- PASS: TestClaimRefreshBoundedByCallerContext (5.21s)
--- PASS: TestClaimRefreshCancelledWithCaller (0.30s)
--- PASS: TestRegisterGuardsEveryDescriptor (0.00s)
--- PASS: TestRequirePersistentRefusesUndeclared (0.00s)
--- PASS: TestRequirePersistentPerFamily (0.00s)
--- PASS: TestPairClaims (0.00s)
--- PASS: TestIndexRefreshCappedWithoutDeadline (0.00s)
ok  	ngfw/agent/internal/subsystems	5.569s
```

### CI
The gate ran as `TMPDIR=/tmp/g-w1b tools/ci.sh --base main` on `task/TD-11b` @ 38591aee. Before that, `go test ./internal/...` and golangci-lint over the whole agent module passed with 0 issues.
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m38s
  forbidden patterns (+ gitleaks)                    0m05s
  lint · typecheck · unit tests · build (turbo)   1m37s
  apps/agent: make lint test build                   1m06s
  apps/cli: make lint test build                     0m11s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces)   0m09s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m11s
  mode quick · wall time 5m01s · logs /root/ngfw-wt/logs/ci/TD-11b-20260925-000206-1568349

CI GATE PASSED
```
