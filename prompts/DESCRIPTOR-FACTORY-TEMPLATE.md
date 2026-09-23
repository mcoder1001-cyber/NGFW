# Task: DF-<n> — Descriptors for VPP plugins: <plugin list>   (prepend 00-CONTEXT.md)

## Goal
Write the reconciler **descriptors** (Create/Update/Delete/Retrieve/Dependencies) for the object
types of these VPP plugins, against the scheduler interface published by P05a and the generated
bindings in `apps/agent/binapi/`. No API, no UI — pure agent-side building blocks that the F-*
feature tasks will wire up later.

## Inputs to read first
- `apps/agent/internal/scheduler/descriptor.go` (P05a — the interface; do not change it, request changes via a question file)
- `apps/agent/internal/descriptors/README.md` and one merged example (e.g. `interface/`)
- `apps/agent/binapi/<plugin>/` for each plugin — **the only source of message names and fields**
- VPP 26.06 docs for the plugin(s): https://s3-docs.fd.io/vpp/26.06/
- `docs/lab/host-vrx-a.md` — which plugins are loaded on the host (some, like linux_cp/npt66, are not — write the code, mark the integration test `skip-unless-plugin-loaded`)

## Scope — build exactly this
For each object type in <plugin list> (enumerate them in `docs/status/tasks/DF-<n>.md` first):
1. Descriptor in `apps/agent/internal/descriptors/<plugin>/<object>.go`: `KeyOf`, `Dependencies`, `Create`, `Update` (or `ErrRecreate`), `Delete`, `Retrieve` (full dump, decoded into the same proto type used for desired state, including metadata such as sw_if_index).
2. Registration in the plugin's `Register(scheduler)` function; add to the descriptor registry list.
3. Unit tests with the fake VPP client (table-driven: create, idempotent re-apply, update, delete, dependency ordering, Retrieve decoding).
4. Integration test against the host VPP (`/run/vpp/api.sock`): create → Retrieve shows it → delete → Retrieve shows nothing. Use loopbacks/tables/dummy objects; never touch `local0` or interfaces you did not create; clean up in `t.Cleanup`.
5. `docs/agent/descriptors/<plugin>.md`: table object type ↔ VPP messages ↔ notes/limitations.

## Rules
- Message names come from binapi; if a message you need is missing from `apps/agent/binapi/`, add the plugin to `tools/binapi-gen.sh`, regenerate, commit the generated code in the same branch.
- Retrieve must decode *everything* the diff needs; a descriptor without Retrieve is not done.
- No shelling out to `vppctl`. No C. No changes to `startup.conf`.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/descriptors/<plugin>/...` green (unit + integration on the host)
- [ ] `grep -rn "vppctl\|exec.Command" internal/descriptors/<plugin>` is empty
- [ ] Applying the same desired state twice yields an empty plan (log excerpt)
- [ ] Object ↔ message table committed

## Out of scope (do not build)
API endpoints, UI screens, schema changes, F-* feature wiring, performance, startup.conf changes.
