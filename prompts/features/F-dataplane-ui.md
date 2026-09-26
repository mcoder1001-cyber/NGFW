# Task: F-dataplane-ui — Dataplane screen (VPP startup settings) with preview and gated apply   (prepend 00-CONTEXT.md)

## Goal
Give the `dataplane` schema domain a screen (WBS D0.6, D-152). The backend exists: F-startup-gen renders startup.conf from the
document (`apps/agent/internal/renderers/vppstartup`) and F-startup-apply ships `deploy/vpp/apply-startup.sh`. Missing: the page,
a read-only preview route and a clear "needs a VPP restart" flow. The Dataplane nav entry shows "soon" today.

## Inputs to read first
- `packages/schema/src/domains/dataplane.ts` — plugin switches, per-NIC `name`/queues/descriptors, `workers`, `corelist`, `mainCore`,
  default queue counts, hugepages.
- `apps/agent/internal/renderers/vppstartup/**`, `deploy/vpp/apply-startup.sh`, `docs/decisions/PENDING-handover.md`,
  `docs/decisions/PENDING-vpp-host-hardening.md`, TD-17 (appliance approval gate) — **no VPP restart from this task**.
- WEB-2 config kit `apps/web/src/config/**`; `apps/web/src/domains/services/ServicesPage.tsx` for the page pattern.

## Contract changes
None expected (additive `contract(schema|proto)` commits only if a field is missing).

## Scope — build exactly this
1. **API** `apps/api/src/features/dataplane/`: `GET /api/v1/state/dataplane` (running VPP workers/cores, loaded plugins, NIC queues, hugepages
   in use — read via the agent state stream, no shell) and `POST /api/v1/actions/dataplane/preview` → the rendered startup.conf for the
   candidate + a diff against the running file (agent RPC to the existing renderer; read-only).
2. **UI** `apps/web/src/domains/system/dataplane/`: SchemaForm with groups (CPU, DPDK/NICs, plugins, memory), a live "running vs candidate"
   column, and a preview dialog showing the rendered file and diff. A banner states that these settings take effect only after a VPP restart.
   The "apply" button stays **disabled with an explanation** until TD-17 merges and PENDING-handover allows restarts; add `dataplane` to
   `BUILT_DOMAINS` under a `// F-dataplane-ui` anchor; en + fa strings.
3. **Validation surfaced in the UI**: corelist length = workers, main core not a worker core, descriptors power of two — reuse the schema's
   semantic rules; nothing duplicated in the UI.
4. **Docs**: `docs/user/system/dataplane.md` — what each setting does, why a restart is needed, how to roll back.

## Acceptance (paste the evidence)
- [ ] Preview for a candidate that changes workers shows the expected startup.conf diff (pasted)
- [ ] Invalid corelist → 400 problem+json with a `pointer`, shown on the field
- [ ] No code path restarts VPP (grep evidence) and the apply button is disabled with its reason
- [ ] Screenshot of the Dataplane screen against the real endpoint; nav entry no longer shows "soon"
- [ ] `tools/ci.sh --base main` green

## Out of scope (do not build)
Running apply-startup.sh from the UI (after TD-17, own row), P10 packaging, per-slot VPP (LAB-vpp-per-slot).

## Open questions to surface, not to decide silently
- Who may press "apply" once enabled (admin only? confirmed-commit style dead-man?) — belongs to TD-17's approval gate.
