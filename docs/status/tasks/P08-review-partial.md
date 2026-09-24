# P08 review — PARTIAL (stopped on coordinator order: manager handover, 2026-09-24 13:40)

Reviewer: reviewing agent, worktree `/root/ngfw-wt/P08`, branch `task/P08` @ ffecf13. No code changed. No verdict yet.
The provisional verdict so far is **APPROVE WITH CHANGES**. It is not final because items 2, 3 and 11 still need the integration result.

## Checklist status
| # | item | status |
|---|---|---|
| 1 | Contract compliance | **done.** Proto change is additive: no `-` lines in `dataplane.proto`. Contract commits 51b7c42 and c02aa32 and `P08-contract.md` are present. See F1: `/state/interfaces` changed the meaning of an existing field. |
| 2 | Real verification | code read: the topology test runs against the host VPP and the af_packet rig. It asserts through Retrieve, `vppctl show int/address/trace`, and WS counters vs vppctl within 5 %. My own run of it has not finished (see 11). |
| 3 | Restart safety | code read: stop agent → binapi delete (addresses first, then af_packet) → start → recreated and ping works within 30 s, timed from agent log timestamps. It adds no new descriptor type; all objects come from DF-1/core/af_packet/dhcp, which have Retrieve. Pasted evidence in P08.md looks consistent. I have not re-run it myself. |
| 4 | VPP API provenance | done. Every message comes from `ngfw/agent/binapi/*`. The branch changes nothing under `binapi/` or `tools/binapi-gen.sh`. |
| 5 | Shared-host rules / D-101 | done (code read). **Every af_packet delete path quiesces the veth first**: hand-over from the rig (`interfaces_test.go` `r.peers(t,false)` before `deleteBehindBack`), simulated loss (same order), and the agent's delete commit in `cleanup-through-api` (`r.peers(t,false)` first). On failure, cleanup goes through `tools/lab rig down`, which quiesces as of 21c81a4 (merged into the branch, `tools/lab:402`). The test also takes `flock -s`, stops only the PIDs it started, uses the slot prefix/DB/ports, and compares NRestarts. |
| 6 | Security | partly done. No `child_process` in `apps/api` or `apps/web`. `exec.Command` appears only in test code, with fixed arguments. The new route uses `@Protected`. See F4. |
| 7 | Transaction semantics | partly done. Rollback of MTU and address is proven through Retrieve in the test. The `defaultTolerant.Update` edge case (F5) was not fully analysed. |
| 8 | UI honesty | done. The screen calls the real `/state/interfaces` and the generic config routes, with no TODO, mock or stub. Screenshots are present. The Playwright video is missing (F6). |
| 9 | Scope creep | done. DF-5 `IPsecOptions/IKEv2Options`, `KeyedClaims` and `ClassifyStore` are wiring the envelope asked for (obligations table), so they are acceptable. |
| 10 | i18n | done. en and fa `interfaces.json` have identical key sets. No `margin-left/right`. The drawer anchor follows the theme direction. |
| 11 | Own `tools/ci.sh` run | **NOT finished.** Command: `VRX_CI_SLOT=1 tools/ci.sh full --base main`, started 13:19. The quick part **passed** every step: contract guard ok, gen gate, forbidden patterns + gitleaks, turbo 30/30, agent `make lint test build`, test/ modules. The integration step (slot 1, rig w1, lock converted to shared) was still running at 13:40, with NRestarts 0 → 0 so far. Log: `/root/ngfw-wt/logs/ci/P08-20260924-131914-10894`. I left it running so that ci.sh does its own rig-down/cleanup. |

### Item 11 — the CI run finished after the stop order (14:01), result recorded without new checks
- **Result:** `EXIT 1`, `CI GATE FAILED — Go integration tests failed in apps/agent`. NRestarts stayed 0 before and after. The rig is down (no `w1l0/w1w0` veths remain).
- **What failed:**
  - `renderers/frr`: `TestReviewH2NotConvergedLive`, `TestReviewM2SecretLive`, `TestReviewM3ManyRoutesLive`.
  - `renderers/frr/frrtest`: `TestHarnessSlotLockSerialises`. mgmtd could not bind `/var/run/frr/w1/mgmtd_fe.sock` (Permission denied) and could not set its log file.
- **Not P08's code:** P08 changes nothing under `apps/agent/internal/renderers`. This looks like an environment or slot-1 FRR problem. That is my reading; I did not verify it.
- **What it means for P08:** the gate stops at the `apps/agent` suite, so the **P08 topology and restart-safety test (`test/topology/interfaces`) did not run in my gate.**
- **Still open:** re-run on a clean slot, or run with the FRR issue triaged. Item 11 stays open until then.

## Findings so far (ranked)
**F1 — medium. `/state/interfaces` changed the meaning of `items[].config`, and the CLI still reads the old meaning.**
- **What changed:** `apps/api/src/state/state.controller.ts` (`interfaces()`, `config: runIfs.get(name)?.value ?? null`). Before P08, `config` held the Retrieve (data-plane) view and was never null. Now it holds the running configuration, may be null, and the old meaning has moved to `actual`. The list also now includes every live VPP interface.
- **Who breaks:** `apps/cli/internal/cli/cmd_op.go:184-221` (`showInterfaces`) still renders `it.Config` as "retrieved". Unconfigured live interfaces now print as empty rows, and the command no longer shows data-plane state. P08's own user doc (`docs/user/interfaces/basics.md:78-81`) names `vrx show interfaces` as the CLI equivalent.
- **Contract note:** `P08-contract.md` calls this change "additive". It is not.
- **Fix:** update the CLI to use `state`/`actual` and correct `P08-contract.md`, or keep the old meaning of `config` and add `running`.

**F2 — medium. The CLI operations table is stale after the new route.**
- `apps/cli/internal/api/operations_gen.go` lacks `State_counters`.
- `cd apps/cli && go test ./internal/api/` → `FAIL TestOperationsTableMatchesOpenAPI: internal/api/operations_gen.go is stale`. I regenerated to scratch and diffed: the only difference is the P08 route.
- `tools/ci.sh` does not run the `apps/cli` tests, so the gate did not catch this (a gap for P09/the manager).
- **Fix:** `make -C apps/cli gen` and commit.

**F3 — low. Two descriptors named in the task were not wired and are not listed as out of scope.**
- The task names "neighbor" and "description tag".
- No `ip_neighbor` descriptor is wired, and there is no schema leaf for it. The description is kept in agent state (D-073b) rather than as a VPP tag.
- **Fix:** list both explicitly in P08.md, out of scope or deferred.

**F4 — low (lab path). Any Linux netdev can be attached to VPP from the config.**
- `apps/agent/internal/desired/interfaces.go` `hostRe` (`^host-([A-Za-z0-9_-]{1,15})$`) lets any config writer make the agent attach af_packet to *any* Linux netdev, including the management NIC `ens192`.
- **Fix:** restrict it with a semantic rule or an allow-list, or gate af_packet to lab builds.

**F5 — low. `defaultTolerant.Update` can leave a partial change.**
- `apps/agent/internal/subsystems/tolerant.go` (Update). It calls `Delete(old)` before checking `inEffect(new)`. If that check then fails, the old MTU or rx-mode is already reset while an error is returned.
- Whether the scheduler's revert covers this was not verified.

**F6 — low. The Playwright video and screenshot script are missing.**
- The acceptance item "Playwright run video" is not met; P08.md says so.
- The node script that `TestInterfacesScreenshots` needs is not committed, so nobody else can reproduce the screenshots.

**Known, already tracked (not held against P08):**
- TD-5: the agent's own af_packet Delete does not quiesce the veth. The UI and API can therefore still delete a `host-*` interface while its veth is up (V24).
- TD-3: `ifsanitize.Release` is not wired.

## Not yet checked
- The result of the integration run (items 2, 3 and 11 from my own run).
- A deeper review of `apps/agent/internal/agent/interfaces_test.go` and `subsystems/stores*.go`.
- A line-level read of `InterfaceDrawer.tsx` and `InterfacesPage.tsx`.
- The fake-agent fidelity of `apps/api/src/testing/fake-agent.ts`.
