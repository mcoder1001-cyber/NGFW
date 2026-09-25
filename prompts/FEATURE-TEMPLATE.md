# Task: F-<slug> — <Feature name>   (prepend 00-CONTEXT.md)

## Goal
Implement **<feature>** end to end in FAST MODE: schema → agent descriptor/renderer wiring → API → UI → verification → docs.
Reference behaviour: TNSR "<TNSR feature name>" and VPP feature/plugin "<vpp feature>" (WBS <ids> in `plan/wbs.csv`).

## Inputs to read first
- `packages/schema/src/domains/<domain>.ts` — extend additively (contract rule below), do not fork
- `packages/proto/vrx/v1/dataplane.proto` — the `<Subsystem>` message you fill
- `apps/agent/internal/descriptors/<plugin>/` (built by DF-<n>) — reuse; if something is missing, add it there (you own it for this task)
- `apps/agent/binapi/<plugin>/` — **the only source of VPP API names** (manager-owned; missing plugin → questions file)
- `docs/lab/shared-host-rules.md` — your slot prefix, ports, table range; daemon ownership
- VPP 26.06 docs: https://s3-docs.fd.io/vpp/26.06/

## Contract changes
If the schema/proto lacks fields you need: commit them **first** on branch `contract/<id>` with subject `contract(<pkg>): …` and
`docs/status/tasks/<id>-contract.md`, tell the manager via `docs/status/tasks/<id>-questions.md`, then continue on your task
branch against those changes — do not wait. Renaming/reshaping existing fields is not allowed (PENDING).

## Scope — build exactly this
1. **Schema**: Zod model for `<config path>` with semantic rules: <list>.
2. **Agent**: wire descriptors for `<objects>` into the `<subsystem>` apply path; `Retrieve` covers every object; unit tests with the fake
   client; ONE integration check on the host VPP (`VRX_INTEGRATION=1`, shared lock, prefixed objects): after Apply `Retrieve()` == desired and
   `vppctl show <x>` contains it; after rollback nothing remains; agent-restart simulation recreates it.
   An id-allocating family takes its range only from `w.IDRange()`, never `nil` or a missing option (df7: `df7.WithIDs(ids.DF7())`);
   its test asserts that `NoIDs()` owns nothing (`docs/lab/shared-host-rules.md` §12).
   **A daemon descriptor implements `scheduler.Validator` and declares `StageDaemon`** (TD-13, D-125; recipe and contract in
   `docs/agent/scheduler-validators.md`): `Validate` = render + the renderer's staged checker, read-only and bounded by ctx, so a
   configuration the daemon refuses fails DryRun and Apply before any VPP write. It masks every plaintext it resolved
   (`rfkit.Redactor`). Test: a failing checker leaves the fake VPP untouched, and `Validate` makes exactly one checker call on a
   staged path and nothing else.
3. **API**: config via the generic pointer routes; state under `/api/v1/state/<path>` (server-side paging if a list can exceed 1k rows);
   OpenAPI; regenerate `packages/api-client`.
4. **UI**: list page + schema-driven edit form + live status column; en + fa strings; pending-change bar shows the diff; **screenshot of the
   screen against the real endpoint** pasted in `docs/status/tasks/<id>.md` (no per-feature Playwright).
5. **Docs**: `docs/user/<domain>/<feature>.md` with an example and the CLI equivalent.

## Acceptance (paste the evidence)
- [ ] `vppctl show <thing>` reflects the committed config (pasted)
- [ ] Agent-restart simulation → config back within 30 s (log excerpt)
- [ ] Rollback removes the objects (Retrieve output, not assumption)
- [ ] Validation failure for <bad input> → 400 problem+json with a `pointer`
- [ ] `tools/ci.sh --base main` green in your worktree
<packet-level test line ONLY if this feature is one of: vertical slice, NAT, IPsec, BGP→FIB, VRRP — then: path recorded (`af_packet` rig)>

## Out of scope (do not build)
<explicit list of adjacent features the agent must not touch>

## Open questions to surface, not to decide silently
<list, or "none">
