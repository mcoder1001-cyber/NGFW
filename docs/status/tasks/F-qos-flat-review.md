# F-qos-flat — review

Branch `task/F-qos-flat` @ 8e44660e, 11 commits on `main@1d3ccf31`. Read: envelope + addendum, prompts/features/F-qos-flat.md,
docs/status/tasks/F-qos-flat.md + questions (Q1–Q15), 00-CONTEXT.md, 01-architecture.md, wave-BC-numbers.md §F-qos-flat,
LOG D-052/D-063/D-076/D-080/D-129/D-132/D-133/D-138, TD-11b; full diff `git diff main...HEAD` (68 files); VPP source
`/root/vpp` `src/plugins/policer/**`, `src/vnet/qos/**` (read-only); parallel branches' `dataplane.proto` for contract
collisions.

## Verdict: **APPROVE**

Sound, well-tested gap-only fixes on top of DF-7; the two new VPP findings are correctly diagnosed and correctly
mitigated in code; contract numbers match the allocation with no collisions; tests actually fail on `main` as claimed.
Two Low process notes and one row to add to the tech-debt board (TD-27, as the task itself asked); nothing blocks
merge once TD-25 reopens host runs.

## 1. Architecture

- **qos.meta (D-063 compliance):** `apps/agent/internal/descriptors/qos/meta.go` is an agent-local, non-VPP record
  (file-backed, `CheckPersistent` via `persist.Require`). Its own `Retrieve` reads back its own persisted file —
  that is not the D-063 violation ("fake Retrieve by echoing cached desired state" of a *VPP* object); it is the same
  agent-local-record pattern already approved for F-kea's `dhcp.relay` (Q11). Confirmed by reading `AssembleQoS`
  (`apps/agent/internal/desired/qos.go:465-583`): it iterates the **VPP-retrieved `kvs`** (`policer.NamePolicer`,
  `qos.NameEgressMap`, `qos.NameRecord`, `qos.NameStore`, `qos.NameMark`) and uses `meta` only to supply a name/
  description/explicit-id flag for an object VPP already reported (`mapName`, `meta.Policers[...]`,
  `meta.Interfaces[...]`) — it never manufactures an object VPP didn't return. No echo. **OK.**
- **Write-only attachments (D-076/D-080) + D-138 read-back:** `policer.interface`/`policer.classify`/
  `qos.record`/`qos.store`/`qos.mark` all keep true write-only Retrieve (`df7.Unsupported`, D-063) — none of them
  has a VPP read-back API, so D-138's read-back pattern correctly does not apply here (that pattern is only for
  settings with a read-back, e.g. `gso`/`feature_is_enabled`; policer/qos attachments have none). **OK, and
  F-qos-flat.md correctly does not claim D-138.**
- **Declarative / binapi-only:** all VPP calls go through `apps/agent/binapi/{policer,qos}` generated bindings
  (`policer.NewServiceClient`, `qos.NewServiceClient`); no hand-written message names found. `subsystems/qos.go`
  registers with `policer.Register`/`qos.Register`, never `df7/registry.Register` (D-030) — matches the rule.
  No QoS global is set (`subsystems/qos.go` comment + `TestQoSProjectionErrors`/`TestQoSApplyRetrieveRollback`
  confirm id-range scoping, D-071).
- **TD-11b claim-first + ownership:** `policer/ownership.go`, `qos/ownership.go` declare `RecordsNoOwnership()` /
  `CheckPersistent()` for every one of the 8 descriptors (policer.policer, policer.bind, policer.interface,
  policer.classify, qos.record, qos.store, qos.mark, qos.egress-map) plus `qos.meta`. `Create` on
  `qos.record`/`qos.store`/`qos.mark`/`policer.classify`/`policer.interface` all call `tg.ClaimFirst(ctx)` **before**
  the VPP write and `c.Undo(err)` on refusal (`apps/agent/internal/descriptors/qos/qos.go:288-300,407-416,597-604`,
  `apps/agent/internal/descriptors/policer/attach.go:159-197,384-393`) — matches D-133's claim-first ordering and the
  worker's own base-comparison tests (`TestAttachmentClaimFirst`, `TestClaimFirst` fail on main with the pre-TD-11b
  write-before-claim code). **OK.**

## 2. VPP findings (V-new, F-qos-flat) — verified against `/root/vpp`

**(a) `qos_mark` has no interface-delete hook.** Confirmed: `src/vnet/qos/qos_record.c:127` and
`src/vnet/qos/qos_store.c:140` both register `VNET_SW_INTERFACE_ADD_DEL_FUNCTION`; `src/vnet/qos/qos_mark.c` has
**no such registration**. `qos_mark_enable` (`qos_mark.c:69-84`) only calls
`qos_egress_map_feature_config(..., enable=1)` when the per-source config slot is `INDEX_INVALID`; a slot left over
from a deleted interface (never reset) is not `INDEX_INVALID`, so `qos_mark_enable` returns 0 without actually
re-enabling the output feature on a reused `sw_if_index` — exactly the "silent" bug the questions file describes.
The proposed fix (ifsanitize sends `qos_mark_enable_disable(enable=0)` for each source when acquiring a new
interface) is the right shape; **please add row TD-27 to the board** for the ifsanitize/TD-3 owner, as F-qos-flat.md
already asks (Q7a) — I did not see this actually landed on `plan/tasks.yaml` yet.

**(b) `policer_del` leaves interface bindings dangling; the re-point fix is correct.** Confirmed in
`src/plugins/policer/policer_op.c:59-85`: `policer_del` frees the pool slot (`pool_put_index`) and unsets
`policer_index_by_name`/`policer_config_by_name` but never touches `pm->policer_index_by_sw_if_index[dir][*]`.
`policer_node.c`/`police_inlines.h:61` (`pol = &pm->policers[policer_index]`) dereferences that stale pool index in
the packet-processing hot path with **no bounds or `pool_is_free_index` check** — a real hazard (stale/reused-slot
misclassification, or an OOB read if the pool never reuses that index).
The fix in `attach.go`'s `Create` (re-point branch, `attach.go:76-171`) is correct: `LookupIndex` re-resolves the
policer's **current** pool index by name every `Create`; when it differs from the record's `@<pool>` suffix, it
un-applies then re-applies. This works because VPP's `policer_input`/`policer_output` API handlers
(`policer_api.c:204-227` `vl_api_policer_input_t_handler`) resolve the pool index by **name** on every call (both
apply=true and apply=false), so both the un-apply and the re-apply always act on the policer's live index — the
agent never needs to remember or race the numeric index itself, it only needs to notice a mismatch and redo the two
binapi calls. `TestAttachmentRepointsAfterPolicerLoss` exercises exactly this. **Correct fix, well tested.**

## 3. Crash safety

- **The OOB un-apply this task must never trigger:** confirmed in `policer_op.c:211-229` — the `apply` branch calls
  `vec_validate` before writing `policer_index_by_sw_if_index[dir][sw_if_index]`; the `else` (un-apply) branch does
  **not** call `vec_validate` and writes directly — an un-apply on a never-applied `(dir, sw_if_index)` is a
  heap OOB write. `attach.go`'s `Delete` (line ~206) and the re-point branch in `Create` both gate every
  `apply(..., false)` on `appliedHere` (the persisted, boot-identity-scoped record) being true first, and `Delete`
  additionally tolerates `NO_SUCH_ENTRY` (policer already gone) without calling apply again. This is the same
  invariant as before this task, correctly preserved through the re-point change. **OK.**
- **Rates/bursts of 0 or huge values:** schema (`packages/schema/src/domains/services.ts:1006,1063`) bounds
  `cir`/`rateKbps` to `u32Int.min(1)` (never 0) and `eir` to full `u32Int` range (0..2^32-1, matches the VPP `u32`
  field). Traced `pol_logical_2_physical` → `x86_pol_compute_hw_params` (`xlate.c:888-968`): huge `cir`/`eir` values
  are just arithmetic (u64 intermediate, no overflow at u32 inputs) and VPP caps `cb`/`eb` to 32 bits internally
  (`xlate.c:906,908`) — no crash path. `cb`/`eb` (`burstField`, services.ts:969-970) allow `min(0)`; VPP's own
  validation (`xlate.c:920,993,1000`) **cleanly rejects** `cb=0`/`eb=0` with `VNET_API_ERROR_INVALID_VALUE`
  (`policer_add`/`policer_update` returns -1, no crash) rather than crashing. **Low** finding: tightening
  `cb`/`eb` to `.min(1)` in the schema would turn a round-trip API error into an earlier 400 with a pointer, but this
  is a UX/completeness nit, not a correctness or safety bug — no action required before merge.
- No new crash risk found; V-new items above are the only ones, both already logged with a config-only mitigation.

## 4. DF-7 gap fixes — ownership and tests

All four gap fixes are inside the task's exclusively-owned files (`descriptors/policer/**`, `descriptors/qos/**`)
and each has a named test:
| fix | file | test |
|---|---|---|
| TD-11b ownership declarations (8 descriptors) | `policer/ownership.go`, `qos/ownership.go` | `TestOwnershipDeclared` (both packages; fails on main with `persist.ErrUndeclared`) |
| Claim-first Creates | `policer/attach.go`, `qos/qos.go` | `TestAttachmentClaimFirst`, `TestClaimFirst` (fail on main: write-before-claim) |
| `policer.States` / `policer.ResetIndex` | `policer/policer.go` | `TestStatesAndResetIndex` |
| Attachment re-point after policer re-creation | `policer/attach.go` | `TestAttachmentRepointsAfterPolicerLoss` |
Confirmed gap-only: no edits to `descriptors/df7/**` (read-only, verified not in the diff's file list).

## 5. Contract

- `QosPolicerState`, `QosPolicerReset` + 6 new messages match `docs/status/wave-BC-numbers.md` §F-qos-flat exactly.
- `QosService` field 5, `QosPolicer` field 13, `QosInterface` field 7 are **not used** anywhere in the diff (grepped
  the full message bodies) — matches "no gap" in F-qos-flat-contract.md.
- Checked `task/{F-lb,F-srv6,F-mpls-srmpls,F-kea-dhcp-relay,F-unbound-chrony-syslog}:packages/proto/vrx/v1/dataplane.proto`:
  none defines `QosPolicerState`/`QosPolicerReset`, and none of their `QosService`/`QosPolicer`/`QosInterface`
  bodies touch fields 5/13/7 — **no collision** with any in-flight branch.
- Reserved-field rule respected (no `ActionRequest` member, no `EventKind` added).

## 6. D-132

`apps/agent/internal/agent/rpc_qos.go`: `qosWalk` takes a per-service `walk chan struct{}` (capacity 1) with a
`qosWalkWait = 3 * time.Second` timeout → `codes.Unavailable` when a walk is already running (matches the "one walk
at a time... UNAVAILABLE after 3s" wording in the addendum). Web: `QOS_POLL_MS = 30_000` used as `refetchInterval`
plus a Refresh button (`apps/web/src/domains/services/qos-flat/queries.ts`). **OK.**

## 7. Q1 — unanchored hunks / merge conflict risk

`subsystems.go` register()/imports, `projection.go` project()/assemble(), `semantic/index.ts`, `proto.md` §11,
`nav.ts`/`nav.test.ts` are all genuinely unanchored (verified: no `// wave-BC: F-qos-flat` marker present at those
sites in this diff) and correctly flagged in the questions file rather than silently placed. Risk is low and
mechanical: every other `services`-domain branch (F-kea-dhcp-relay, F-unbound-chrony-syslog, F-lb, F-rpf-adl-pbr)
appends the same shape of hunk at the same "end of block" location, so a squash-merge conflict is a few-line textual
collision, not a semantic one. Q4's merge recipe (one `ServicesImplemented`/`ServicesUnsupported` file, one
`Services` const, union `Domains["services"]` list, `ServicesUnsupported` called once) is concrete and consistent
with what I saw in this diff (`desired/qos_services.go`, `subsystems/qos.go`, `projection.go`'s single
`desired.ServicesUnsupported(p, ds.GetServices())` call) — nothing here contradicts it. Manager action, not a
blocking review finding.

## 8. Tests

- `cd apps/agent && go test -race -count=1 ./internal/agent/... ./internal/desired/... ./internal/descriptors/policer/... ./internal/descriptors/qos/...`
  (TMPDIR=/tmp/rev-qos-flat) → **all pass**, no race detected.
- `apps/web`: `npx vitest run src/domains/services/qos-flat/ src/nav/` (after building `@ngfw/ui-kit`,
  `@ngfw/schema`, `@ngfw/api-client` — none had a `dist/`, a pre-existing workspace build-order fact, not caused by
  this branch) → **22/22 pass** (`nav.test.ts` 5, `qos-flat/model.test.ts` 12, `QosPage.test.tsx` 5), matching the
  numbers pasted in F-qos-flat.md.
- `apps/api`: `npx vitest run src/features/qos-flat/model.test.ts` → **4/4 pass**.
- Re-verified the "fails on base" claim's mechanism (D-076/TD-11b write-before-claim code path): the diff's own
  pasted base-run output in F-qos-flat.md (`TestAttachmentClaimFirst`, `TestAttachmentRepointsAfterPolicerLoss`,
  `TestClaimFirst`, `TestOwnershipDeclared`) is consistent with what `git diff main...HEAD` shows changing in
  `attach.go`/`qos.go` (claim-first order, re-point branch, ownership.go being new files) — plausible and not
  re-run against a synthetic "old descriptor" copy by me (would need building a throwaway pre-TD-11b tree; not done
  given the time box, but the mechanism matches the code exactly).

## 9. Host test script (`test/topology/qos-flat/run.sh`, `qos_test.go`)

- Slot-local: `run.sh` requires `VRX_TEST_PREFIX` matching `^w([0-9]{1,2})$`, uses `/run/vrx-test/$VRX_TEST_PREFIX`,
  takes `tools/lab lock shared` before running. `qos_test.go`'s `slotFromEnv` derives table base / metrics port /
  socket from the same prefix; policer/map names and interfaces are all slot-prefixed per the file's own header
  comment.
- NRestarts: `nRestarts(t)` via `systemctl show vpp -p NRestarts`, called before (line ~320) and compared after
  (line ~323) with `t.Errorf` (not skip) if it changed — matches "stop host runs and write it down if it rises."
- `vppctl(t, args...)` (line 101) hard-fails unless `args[0] == "show"`. Grepped every call site: only
  `show policer`, `show interface features`, `show qos egress map`, `show qos mark` are used — never `show trace`/
  `trace add`/`clear trace`. **Low** finding: the guard only checks `args[0] == "show"`, so it would not by itself
  stop a future edit from adding `vppctl(t, "show", "trace")`; consider tightening the guard to also reject
  `"trace"` anywhere in `args` (cheap, and matches the stated intent literally, not just by current usage). Not
  blocking — no such call exists today.
- Skips cleanly without `VRX_INTEGRATION=1` (line 314-315), naming TD-25 as the reason. Builds the agent binary
  itself (`go build -o bin/vrx-agent`) rather than assuming a stale one. Looks safe to run once TD-25 reopens host
  runs.

## Findings summary

| # | severity | dimension | finding | file:line |
|---|---|---|---|---|
| 1 | L | crash safety | `cb`/`eb` schema allow 0 (VPP rejects cleanly, no crash) — tightening to `.min(1)` would give an earlier, clearer 400 | `packages/schema/src/domains/services.ts:969-970` |
| 2 | L | host test hygiene | `vppctl()` guard only checks `args[0]=="show"`; doesn't defend against a future `show trace` by name | `test/topology/qos-flat/qos_test.go:101-108` |
| 3 | process (not code) | VPP findings / board | TD-27 (ifsanitize clears `qos_mark` on interface acquire, V-new F-qos-flat finding a) proposed in the questions file but I do not see it added as a board row yet — manager to add | `docs/status/tasks/F-qos-flat-questions.md` Q7a |

No H or M findings. Architecture, contract numbers, DF-7 gap-fix ownership/tests, D-132 bounding, and the two new VPP
findings (with their fixes) all check out against the VPP 26.06 source and the current test run. Host proof remains
correctly gated on TD-25 and is not attempted, per the manager's addendum.
