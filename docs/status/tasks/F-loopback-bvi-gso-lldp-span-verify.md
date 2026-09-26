# F-loopback-bvi-gso-lldp-span — focused verify of fix round 1

Reviewer session 2026-09-25. Branch `task/F-loopback-bvi-gso-lldp-span` @ 1ca7f9c3; the review was 777f629f.
Scope: the review's M1, M2, M3, M5 and M6, the new audit commit 1ca7f9c3, L1 and L3, and a regression check. M4 and L2
stay open until after TD-23 by the manager's decision.

No host runs, no vppctl, no trace. VPP facts come from reading `/root/vpp` (26.06).

## Verdict: APPROVE

Every finding in scope is fixed. Each fix has a test that fails on the old code, and nothing regressed. A few items
remain for the rebase round; they are listed at the end.

## What I ran (real output, HEAD 1ca7f9c3)

Go, with the race detector. `VRX_INTEGRATION`, `VRX_NSIM_HOST`, `VRX_NSIM` and `VRX_NSIM_POLL_MAIN_THREAD` were unset.
```
$ cd apps/agent && go test -race -count=1 ./internal/descriptors/{gso,nsim,span,lldp,core}/... ./internal/desired/... ./internal/subsystems/... ./internal/agent/...
ok  	ngfw/agent/internal/descriptors/gso	1.130s
ok  	ngfw/agent/internal/descriptors/nsim	1.172s
ok  	ngfw/agent/internal/descriptors/span	1.212s
ok  	ngfw/agent/internal/descriptors/lldp	1.170s
ok  	ngfw/agent/internal/descriptors/core	1.171s
?   	ngfw/agent/internal/descriptors/core/coretest	[no test files]
ok  	ngfw/agent/internal/desired	1.263s
ok  	ngfw/agent/internal/subsystems	1.307s
ok  	ngfw/agent/internal/agent	11.857s
```
Web. I first built the four workspace packages with `tsc` only, then removed their `dist/` again.
```
$ pnpm --filter @ngfw/web test
 ✓ src/domains/interfaces/loopback-bvi-gso-lldp-span/pages.test.tsx (7 tests) 31161ms
 Test Files  16 passed (16)
      Tests  107 passed (107)
```
API unit tests (`vitest run`):
```
 ✓ src/features/loopback-bvi-gso-lldp-span/nsim-gate.test.ts (3 tests) 54ms
 Test Files  9 passed (9)
      Tests  53 passed (53)
```
I did not rerun the CI gate or the e2e test, which needs PostgreSQL and was not in the allowed list. The worker's CI
passed at 4718b431. For 1ca7f9c3, which came after it, the status file pastes a green e2e run plus clean `tsc`,
eslint and prettier.

## Per finding

| id | status | evidence |
|---|---|---|
| **M1** wheel bound | **fixed** | See "M1 details" below. |
| **M2** nsim hazards | **fixed** | See "M2 details" below. |
| **M3** claim first | **fixed** | See "M3 details" below. |
| **M5** LLDP index mismatch | **fixed** | See "M5 details" below. |
| **M6** form submits | **fixed** | See "M6 details" below. |
| **Audit** (1ca7f9c3) | **correct** | See "Audit details" below. |
| **L1** | **fixed** | <ul><li>The schema help now reads "Empty keeps VPP's current system name (VPP starts without one)". It is in its own `contract(schema):` commit 9278c416 and is text only.</li><li>The `desired/lldp.go` comment is fixed.</li></ul> |
| **L3** | **fixed** | The guide now quotes the evidence: "back 0.61 s after the agent started (reconcile 0.428 s)". |
| **M4, L2** | **open, deferred** | By the manager's decision, until after TD-23; see "For the rebase round". |

### M1 details

- The agent now enforces the bound itself: `Config.Validate` refuses `WheelSlots() > WheelSlotsMax` (2^20) (`nsim.go:52-69, 97-98`).
- The agent formula is the same as the schema's `nsimWheelSlots` / `NSIM_WHEEL_SLOTS_MAX`. The only difference is rounding (ms against µs): a model exactly at the bound could pass the API and still be refused by the agent. That refusal is loud, and the model never reaches VPP.
- `desired.Nsim` runs `Validate` before `s.Add`, and `Create` runs it again.
- Tested by `TestConfigWheelBound`: the schema maxima give about 2·10⁹ slots and are refused; a model at the bound is accepted.

### M2 details

**(a) Worker threads.**
- `nsim.config` Create asks `show_threads` (binapi `vlib`). The VPP handler (`vlibmemory/vlib_api.c:182-209`) counts `vlib_worker_threads`, which includes the main thread at index 0, so `len-1` is the number of workers.
- It returns `ErrWorkerThreads` before any `nsim_configure2`, unless `VRX_NSIM_POLL_MAIN_THREAD=1`.
- Tested by `TestConfigRefusesWorkerThreads`: nothing is sent to VPP.

**(b) Lab gate.**
- Registration and projection happen only when the agent is the globals owner **and** `VRX_NSIM=lab` (`subsystems/loopback_bvi_gso_lldp_span.go:80-98`, `desired/nsim.go:49-56`). Otherwise the agent reports `agent.unsupported-field`.
- The API interceptor answers 409 problem+json with pointer `/services/nsim` for a commit or rollback to a document carrying `services.nsim` (`nsim-gate.ts`).

**Docs and UI checked against VPP:**
- "keeps the main thread polling until VPP restarts" is **true**. `nsim-wheel` is registered DISABLED (`nsim_input.c:125`), and the only state transition anywhere in the plugin is to POLLING (`nsim.c:203-209`). A polling input node stops the main thread from sleeping (`vlib/file.c:139`).
- "no main-thread wheel without poll-main-thread; a main-thread frame crashes" is **true** (`nsim.c:196-198`; `node.c:201-207` behind an `ASSERT` only).

### M3 details

**Every Create claims before its VPP write:**
- gso (`gso.go:171-205`), span (`span.go:144-151`), lldp (`lldp.go:228-252`), and nsim cross-connect and output (`nsim.go:324-346, 465-481`).

**The local `claimFirst` behaves like TD-11b's `ClaimFirst` → `Undo`:**
- A claim that existed before this Create is kept on failure.
- On a tagged interface, claiming is a no-op.

**On a failed write:**
- A failed VPP write releases the claim.
- A failed boot-record write disables the enable again, so no invisible, stackable enable is left (gso and both nsim enables).
- On the lldp index mismatch, the claim is released.

**Tests:** one `claimfirst_test.go` per family, covering both a failed claim (no VPP write) and a failed write or record (claim released, enable undone).

### M5 details

**The text is correct in:**
- the user guide, which now says LLDP may be enabled on **another** hardware interface, the commit fails with `ErrIndexMismatch`, the agent cannot undo it until VPP restarts, and LLDP should be used only on interfaces created at start-up;
- a new warning alert on the LLDP page (`lldp.indexNote`, en and fa);
- `lldp.md` and V-new.

**The worker's claim holds:** no side-effect-free binapi call gives an interface's hw_if_index.
- In gtpu, vxlan offload and flow, `hw_if_index` is an input field.
- `sflow_interface_dump` returns it only for interfaces that already have sflow enabled (`plugins/sflow/sflow.c:1330-1339`). Using it would mean enabling sflow first, a VPP-wide side effect.
- The only other route is parsing `cli_inband` "show hardware-interfaces" text, which is kept test-only (`df7test.AlignedLoopback`).
- The guide's phrase "The binary API exposes no hardware index" is slightly too strong because of the sflow case (L-a below). It is not blocking.

### M6 details

Three new submit tests assert the PATCH bodies:
- **LLDP:** `{lldp: {systemName}}` only. Empty management fields are not sent.
- **nsim without a cross-connect:** `{nsim: {delayMs: 35}}`.
- **Switching an existing cross-connect off:** `{nsim: {crossConnect: null}}`. This was the risky case: the hidden field must turn into a merge-patch null. It does, through `createMergePatch`.

The cross-connect is now its own switch (`NsimPage.tsx`, `withoutProps`), so SchemaForm no longer materialises empty required A and B fields.

### Audit details

**How the gate audits:**
- The gate writes one row itself (`failure`, 409, `after.reason = 'nsim-disabled'`, action `POST /api/v1/config/commit` or `POST /api/v1/config/rollback/:rev`) and only then throws.
- This is the AuthGuard 403 pattern.

**Why the order matters:** the gate is the outer interceptor. Its providers sit at `app.module.ts:124`, before `AuditInterceptor` at `:139`, and the first `APP_INTERCEPTOR` registered is the outermost. So the audit interceptor never sees a refusal.

**The test pins it:** the e2e test compares the exact list of `audit_log` rows written after the request (one row). If a merge reorders the providers, the test catches both a missing row and a doubled one.

**Guards still run first:** a readonly user gets 403 before the gate, as expected.

## Regression check

- **No contract change:** proto, `apps/agent/gen`, `packages/api-client` and the CLI's generated files are unchanged since 777f629f. The only schema change is the help text in L1.
- **Test-file edits are for the new gate only:**
  - `rpc_…_test.go` adds `t.Setenv("VRX_NSIM", "lab")` to the globals-owner test, which is not parallel.
  - The coretest fake answers `show_threads` with a main-only VPP.
  - `mirror_test.go` updates the `Nsim(…)` signature and adds the owner-without-gate case.
- **Behaviour kept:**
  - D-132 (30 s polls, Refresh, one LLDP walk at a time) is unchanged.
  - The GSO read-back and boot record (Q4) are unchanged, apart from the claim-first reordering.
  - The span stale-destination Delete is unchanged.
  - The TD-11b declarations are unchanged.
- **Tests:** everything that ran above is green; the Go packages ran with `-race`.

## For the rebase round (not blocking)

- **M4 / L2**, as the manager decided:
  - two TD-23 registrations: `RegisterExtension` and `RegisterFeatureIsEnabled("gso-ip4")`;
  - delete `lldp_services_seam.go` and the `reportUnsupportedServices` hook;
  - swap the local `requirePersistent` for `dfkit.CheckClaims`/`CheckBoot`, and the four copies of `claimFirst` for `dfkit.Target.ClaimFirst`.
- **L-b:** `lldp.go` returns `Meta` together with an error when `lldp_dump` fails after the enable. On main that must become `scheduler.PartialCreate(err)` (TD-11b Q2); otherwise the scheduler drops the Meta. The kept claim makes it safe on this base.
- **L-a:** change the guide to "no side-effect-free API gives the hardware index (sflow's dump lists only sflow-enabled interfaces)".
- **Keep the gate outermost:** at the merge, `...loopbackBviGsoLldpSpanFeature.providers` must stay before `AuditInterceptor` in `app.module.ts`. The e2e test enforces it.
- **Cleanup:** `apps/agent/bin` exists in the worktree again. It is gitignored, and it is a leftover of the worker's fix-round CI `make build`, not of this verify. Remove it before the worktree is released.
