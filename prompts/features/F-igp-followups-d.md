# Task: F-igp-followups-d — VRRP live state: VrrpState RPC, EVENT_KIND_VRRP_STATE_CHANGED (17), GET /api/v1/state/ha/vrrp, vrrp.events   (prepend 00-CONTEXT.md)

> Split 4 of 6 of board row F-igp-followups (D-173). Needs F-igp-followups-a merged (EventKind 17, `rpc VrrpState`). Parallel with -b/-c.

## Goal
Report what each configured VR is doing right now (Init/Backup/Master/Fault, master address, transitions) for both engines and push every
role change as an event, so -e can show live role chips. Reference: TNSR "VRRP status"; VPP `vrrp` plugin `vrrp_vr_dump` + `want_vrrp_vr_events`;
keepalived JSON dump / notify; WBS D9.1. Read-only: no configuration behaviour changes.

## Inputs to read first
- `apps/agent/internal/descriptors/vrrp/{vrrp.go,events.go}` (`WatchEvents` :60, `DecodeEvent`, `StateName`: init/backup/master/interface-down) and
  `apps/agent/binapi/vrrp/` — the only source of message names
- `apps/agent/internal/renderers/keepalived/state.go` (`ParseDump`/`DumpInstance`, SIGJSON dump) — read-only for you (S-vrrp-product-fixes owns the stage)
- `apps/agent/internal/subsystems/{vrrp.go,keepalived.go}` (engine gates `NGFW_VRRP_VPP` / `NGFW_KEEPALIVED`, `VrrpEnv()`; S-rva-agent-gates),
  `apps/agent/internal/desired/vrrp.go` (VR naming, `vrrp.meta`), `descriptors/core/coretest/vrrp.go` (fake plugin)
- An existing state RPC + events family as the pattern: `apps/agent/internal/agent/rpc_tunnels.go` (owner-checked unary RPC) and
  `apps/agent/internal/subsystems/frr.go` publish path (`Env.Publish`)
- API: `apps/api/src/features/mpls-ldp/mpls-ldp.controller.ts` (state route pattern: `agentError`, 501 → empty), anchors `// wave-BC: F-vrrp-config-sync` in
  `apps/api/src/{app.module.ts,agent/agent.client.ts,testing/fake-agent.ts,infra/bus.ts,telemetry/relay.service.ts}`
- `docs/status/tasks/F-vrrp-config-sync.md` ("Not built"), `docs/status/tasks/F-vrrp-config-sync-host.md` (window/locks of `test/topology/vrrp/host.sh`)

## Contract changes
None expected (from -a). A proven gap → `contract(proto): …` commit first + `-contract.md` + questions file.

## Scope — build exactly this
1. **Agent RPC** `VrrpState` (new file `agent/rpc_vrrp_state.go`): owner-checked, read-only, bounded by ctx; VPP engine from one `vrrp_vr_dump` filtered to
   this owner's VRs (the claim/tag rule the descriptors use — never another slot's VR); keepalived engine from the stage's JSON dump (no dump while the
   engine gate is off → the VR is reported with state "" and a reason, never an error for the whole call); names from `vrrp.meta` / the desired document.
2. **Events** (new file `subsystems/vrrp_events.go` + exactly one anchored line `// wave-BC: F-igp-followups-d` in `subsystems.go` register()): start
   `vrrp.WatchEvents` when the VPP engine gate is on, map to `EVENT_KIND_VRRP_STATE_CHANGED` with attributes `engine, vr, interface, vr_id, family, old, new`;
   keepalived role changes by polling the dump (≥ 2 s, only when the keepalived gate is on — no notify-script change to the renderer). Stop with the agent; no goroutine leak (test).
3. **API**: `apps/api/src/features/vrrp-config-sync/**` (`VrrpConfigSyncController`): `GET /api/v1/state/ha/vrrp` (all VRs; `agentError` on 501/unavailable),
   `agent.client.ts` method, fake agent answering from the applied document (Backup for every enabled VR), `vrrp.events` topic + relay case, OpenAPI, regenerate
   `packages/api-client`. Read role ≥ viewer, no secrets in the payload.
4. **Tests**: agent unit tests on coretest (dump filtered by owner, keepalived dump parse, gate-off reasons, event mapping, cancel); API unit/e2e with the fake agent.
   Host check (ONE): only inside a VRRP window the manager grants in your envelope (`NGFW_VRRP_VPP=on` on your slot agent, `flock -s /run/lock/ngfw-lab.lock` +
   `flock -x /run/lock/ngfw-globals.lock`, as `test/topology/vrrp/host.sh` does): two slot VRs → `GET /state/ha/vrrp` == `vppctl show vrrp vr` (paste), one
   `vppctl vrrp proto stop` on a slot VR → one event on the WS topic. Without a granted window: say so and leave the host check owed — never start VPP VRRP outside it.
5. **Docs**: `docs/user/system/vrrp-config-sync.md` "Live state" section (route, states, CLI equivalent), `docs/contracts/proto.md` VrrpState semantics.

## Acceptance (paste the evidence)
- [ ] agent + API own tests green (paste)
- [ ] `GET /api/v1/state/ha/vrrp` against your slot API with the real agent: gate-off reasons when the engines are off (paste)
- [ ] in a granted window: state == `vppctl show vrrp vr` for your two VRs, event received on `vrrp.events` (paste), NRestarts before/after
- [ ] `tools/ci-slot.sh --base main` green (tail pasted)

## Out of scope (do not build)
UI role chips (-e); config sync, cluster view, `syncExclude` behaviour (-f); any change to the VRRP/keepalived configuration paths, the keepalived
renderer or its notify scripts (S-vrrp-product-fixes owns `renderers/keepalived/**` stage); VRRPv2 auth; failover tests; starting keepalived or VPP VRRP outside a granted window.

## Open questions to surface, not to decide silently
Whether "Fault" (keepalived) and "interface-down" (VPP) map to one UI state; event rate limit for a flapping VR.

## Files you own
`apps/agent/internal/agent/rpc_vrrp_state*.go` (new), `apps/agent/internal/subsystems/vrrp_events*.go` (new) + one anchored line in `subsystems.go`,
`apps/api/src/features/vrrp-config-sync/**` (new), the `// wave-BC: F-vrrp-config-sync` lines in `apps/api/src/{app.module.ts,agent/agent.client.ts,testing/fake-agent.ts,infra/bus.ts,telemetry/relay.service.ts}`,
`packages/api-client/**` (generated), `docs/user/system/vrrp-config-sync.md` (live-state section), `docs/contracts/proto.md` (VrrpState section),
`docs/status/tasks/F-igp-followups-d*`. List every shared-file line under `## Shared hunks`.

## Rules
- Files you own: above. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/F-igp-followups-d-questions.md`.
- Shared VPP: slot prefix `w<N>` on every object, tables N000–N999 (`eval "$(tools/lab env <N>)"`); never restart or kill VPP; `timeout 10` on every vppctl;
  packet trace banned (D-128). VPP-engine VRRP only in a granted window (V22b, D-167). Daemons: keepalived only if your envelope names it (slot netns,
  /run/ngfw-test/w<N>/, left stopped); `ha` is licence-gated — use a slot-local test licence as F-vrrp-config-sync-host did (Q1), no keys in the repo.
- D-210a: write tests for your change and get them passing in your package (paste output); run them through `tools/heavy.sh` (D-224), e.g.
  `../../tools/heavy.sh go test ./internal/<pkg>/...` from apps/agent, `tools/heavy.sh pnpm --filter <pkg> …` from the repo root; no full suite, no lint, no other packages' tests.
- Contracts: additive only — `contract(schema|proto): …` commits first, numbers only from `docs/status/wave-BC-numbers.md`; renaming/reshaping = PENDING.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never `tools/ci.sh` directly), commit on your branch,
  `docs/status/tasks/F-igp-followups-d.md` with pasted real output; evidence `.txt` under `docs/status/tasks/F-igp-followups-d-evidence/` (D-175).
