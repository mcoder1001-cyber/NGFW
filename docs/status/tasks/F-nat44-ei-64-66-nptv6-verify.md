# F-nat44-ei-64-66-nptv6 — focused verify of fix round 1

**Verdict: APPROVE.** Merge once F-nat44-ed-sessions and TD-23 have merged, following D-112 and D-134; the merge notes are at the end.

**Scope.** This checks R1–R4 and L1–L6 and L8 from the review `57d18bbc`, on `task/F-nat44-ei-64-66-nptv6@0d861e3b`: fix `9203615d`, CI notes `0d861e3b`, second ED merge `3df4b9f1` (ED@`4421baec`). Nothing was run on the host, and no product code was edited. The mutations below ran in scratch copies taken with `git archive HEAD`.

## R1–R4

| item | verified | how |
|---|---|---|
| **R1** (H1) | yes | `desired/nat64.go`: `nat.nat64-tenant-vrf` is a Warn (not an error) at `/nat/nat64/prefixes/<i>/vrf` and `/nat/nat64/staticBibs/<i>/vrf` for a non-default VRF. It does not fire for pools, which lock and unlock correctly in `nat64.c:389/404`. The API passes non-error DryRun issues through as `warnings` (`commit/validation.service.ts:104`), so the commit is not blocked. `TestNat64TenantVRFWarningAndOnePrefixPerVRF` pins the exact pointers. The user page (a paragraph of its own) and V-new (c) now describe the real failure: the whole commit is rolled back (DEGRADED only when a revert fails), a rollback to a revision without the VRF fails the same way, and a confirmed-commit revert cannot complete. They also point to the `nat.nat64-tenant-vrf` warning (both) and the opt-in (V-new c). The core VRF descriptor is untouched (TD-26). |
| **R2** (H2) | yes | `VRX_NAT64_TENANT_VRF_HOST=1` (`nat_test.go` `tenantVRFHost`) gates the slot VRF in rev 1, `nat64`, `restart-nat64` and the NAT64 screenshot. Without it, both phases `t.Skip` with the reason and no configuration names the slot VRF. The other phases (EI, NAT66, NPTv6, restart-ei, rollback, cleanup) do not depend on it. Q10 carries the slot-4 quarantine note for the manager. `go vet` and `go test` of the topology module pass (they skip without `VRX_INTEGRATION`). |
| **R3** (M1) | yes | ED's stub and the `nat44_ei` import are gone from `coretest/nat44ed.go`. **Mutation:** with the 3-line stub restored, `TestExtensionsModelDisjointMessages` FAILs with `"nat44_ei_show_running_config" is modelled by extensions #0 and #1`. **Rebase simulation, repeated:** TD-23's `coretest/fakevpp.go` (and its `extensions_test.go`, both unchanged since `39e07a0f`) was dropped into a copy of HEAD, and the four `init()` bodies became `RegisterExtension("nat44ed"/"nat44ei"/"nat64"/"npt66", …)`. There was **no registry panic**, and `go test -race` passed for `coretest` (TD-23's own tests included), `npt66`, `agent`, `desired`, `actions/...` and `subsystems`. One mechanical rebase edit remains (see the merge notes): the new guard test ranges over `extensions`, which TD-23 replaces with `extAll`. |
| **R4** (M2) | yes | `natSessionsVariant` takes `s.natWalk(ctx)` (ED's per-agent, context-aware slot) after `natReady` and before any VPP call, for both EI and NAT64. The package mutex is gone. **Mutation:** with those 5 lines removed, `TestNatVariantWalksShareTheEDWalkSlot` FAILs (`NAT_SESSION_VARIANT_EI walk while the ED walk runs: <nil> (want DeadlineExceeded)`). ED's NatSessions/NatSummary and these variants now share one slot, which satisfies D-132. |

## L items

- **L1.** The inside port now always comes from the BIB. The fields are swapped only when `il_port` differs from the BIB's `in_port`, and a row with no BIB entry is left untouched. The test covers the 26.06 row, a fixed-VPP row, an ICMP row with remote port 0, and a row with no BIB entry. The remaining edge case (remote port equal to inside port shows 0 on 26.06) is documented in V-new (b).
- **L2.** `nat.ei-port-forward-pool` now also applies to EI identity mappings with a port, at `/identityMappings/<i>/ip`. It is skipped for static-mapping-only mode, interface pools and mappings without a port, which matches `nat44_ei.c:2487-2497`. Tested.
- **L3.** An unknown variant is INVALID_ARGUMENT for both sessions and kill. Tested.
- **L4.** New scoped locale keys `fieldIn.<subtree>.<name>`. en and fa have the same 99 keys, and the NAT64 inside/outside and NPTv6 external strings are now translated. The ui-kit strings are not this task's.
- **L5.** The stale-binding hazard is now documented on `npt66.md` and in V-new (a), with the correct attribution: the agent's own order deletes the binding first.
- **L6.** The status reads "configured (not readable)" / "پیکربندی‌شده (خواندنی نیست)", in the UI and on the user page.
- **L8.** The builder allows one NAT64 prefix per VRF, with the error at the second prefix's `/vrf`. The rejected prefix is not projected. Tested.

Nothing regressed:
- `go vet ./internal/...` is clean.
- `go test -race -count=1` passed on all 12 touched packages: `actions/{nat44-ed-sessions,nat44-ei-64-66-nptv6}`, `agent`, `core/coretest`, `nat44ed`, `nat44ei`, `nat64`, `nat66`, `natcommon`, `npt66`, `desired`, `subsystems`.
- The topology module passes `vet` and `test`.
- Web `src/domains/firewall`: 15/15. API feature unit tests: 8/8. There is no API change since the review.

L9: I accept the worker's `-race -count=20 ./internal/agent/` 20/20 run together with my own 15 clean runs. It stays on D-121's watch list only if it recurs.

## CI judgement

This is acceptable for the merge, with one correction to Q9's wording.
- The branch's `tools/ci.sh` stops at the contract guard. That is the known D-127 SIGPIPE bug; main's copy fixes it, and the branch really does carry `contract(proto)` commits.
- Main's copy passes the guard, the generated-output gate and gitleaks. It then stops on the D-128 trace ban in `test/topology/interfaces/interfaces_test.go`, P08's pre-merge copy inherited through ED ← W-seed ← P08. `task/F-nat44-ed-sessions` has the same 3 trace lines and main has 0, so this is not this task's code.
- The branch's own `quick` run passed the whole gate apart from the guard: turbo 30/30, agent lint with 0 issues and 94 packages ok, cli, and the topology module. I checked this in the step logs of `…/ci/F-nat44-ei-64-66-nptv6-20260925-094625-571965`.
- **Correction to Q9.** The rebase does not replace that file silently. `git merge-tree main task/F-nat44-ei-64-66-nptv6` (merge base `63178d29`, before P08's squash) conflicts in `test/topology/interfaces/{interfaces,helpers,vpp}_test.go`. It also conflicts in ~30 hotspot and generated files, which are mostly inherited and shared with ED. The merger resolves `test/topology/interfaces/*` by taking **main's** side, and the pre-merge-commit hook then runs main's gate, trace ban included, on the squashed tree. That hook run is the real CI for this branch.

## Merge notes (for the merger)

1. **Order.** ED first, then this branch, rebased onto main. Most hotspot conflicts then reduce to ED's already-resolved unions.
2. **TD-23 (D-134).** Replace the three `init()` bodies with `RegisterExtension` lines at `coretest/nat44ei.go:98-99`, `coretest/nat64.go:105-106` (nat64 + nat66) and `coretest/npt66.go:105-106`. ED does the same for `nat44ed.go`. In `coretest/nat44ei_test.go:40`, change `for i, ext := range extensions {` to range over `extAll` and use `e.install`. Alternatively, drop the test, because TD-23's `VPP.On` guard now enforces the same rule. Both variants were exercised in the simulation (the adapted loop passed).
3. **fake.ts.** Keep the spread order in `fake-agent.ts` so EI/NAT64 wins over ED (L10, Q10).
4. **Conflicts.** In `test/topology/interfaces/*`, take main's version.
5. **Slot-4 quarantine.** Record Q10's note for the next slot-4 envelope.

Housekeeping: to run the web and API tests I rebuilt the gitignored `dist/` of `packages/{schema,proto,api-client,ui-kit}` and `apps/api`. This session was not permitted to delete them, so the worker or the merger should remove them.
