# Task: F-<slug> — <Feature name>   (prepend 00-CONTEXT.md)

## Goal
Implement **<feature>** end to end: agent renderer → schema → API → UI → tests → docs.
Reference behaviour: TNSR "<TNSR feature name>" and VPP plugin/feature "<vpp feature>".

## Inputs to read first
- `packages/schema/src/<domain>.ts` — extend, do not fork
- `packages/proto/vrx/dataplane.proto` — the `<Subsystem>` message you must fill
- `apps/agent/internal/descriptors/README.md` — how a descriptor is written
- Generated bindings: `apps/agent/binapi/<plugin>/` — **the only source of VPP API names**
- VPP docs for the feature: `https://s3-docs.fd.io/vpp/26.06/` (search the plugin)

## Scope — build exactly this
1. **Schema**: Zod model for `<config path>` with semantic validation rules: <list>.
2. **Agent**: descriptor `<name>` implementing Create/Update/Delete/Retrieve/Dependencies
   against binapi `<plugin>`; register in the KV scheduler; unit tests with a fake VPP client;
   integration test against the lab VM's VPP proving <packet-level behaviour>.
3. **API**: config CRUD under `/api/v1/config/<path>`, state under `/api/v1/state/<path>`
   (server-side paging if list can exceed 1k rows); OpenAPI; regenerate `packages/api-client`.
4. **UI**: list page + schema-driven edit form + live status column; en + fa strings;
   pending-change bar shows the diff; E2E: create → commit → verify → rollback.
5. **Docs**: `docs/user/<domain>/<feature>.md` with an example and the CLI equivalent.

## Acceptance (paste the evidence in the PR)
- [ ] `vppctl show <thing>` reflects the committed config; `vppctl trace` shows <packets doing X>
- [ ] `tools/lab restart-vpp vrx-a` → within 30 s the config is back (agent log shows reconcile)
- [ ] Rollback removes the objects from VPP (verified via Retrieve, not assumed)
- [ ] Validation failure for <bad input> returns 400 problem+json with a `pointer`
- [ ] `pnpm lint && pnpm typecheck && pnpm test` and `go test ./...` green; integration suite green

## Out of scope (do not build)
<explicit list of adjacent features the agent must not touch>

## Open questions to surface, not to decide silently
<list, or "none">
