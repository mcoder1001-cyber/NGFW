# Task: F-loopback-bvi-gso-lldp-span — loopback/BVI, GSO & offload flags, LLDP, SPAN/ERSPAN, nsim   (prepend 00-CONTEXT.md)

> Checked 2026-09-24 against main + P08 (wave-A prep). Several parts already exist:
> - **DF-7 is merged**: the LLDP and SPAN descriptors are on main, including an LLDP neighbour reader.
> - **P08** already creates and deletes loopbacks end to end.
> - **Jumbo MTU** (≤ 9216) is already in the schema, the `interface.mtu` descriptor and the drawer form.
> - **F-bridge-l2** is a board dependency, merged before you start, and provides BVI membership.
>
> This task builds GSO, mirroring, the nsim test tool, the LLDP/SPAN wiring, the LLDP neighbour state, the pages, the tests and the docs.
> Shared files follow `docs/status/wave-A-hotspots.md`: anchors, allocated numbers, and logic only in your own files.

## Goal
Finish the remaining interface features end to end in FAST MODE:
- loopback interfaces and their use as bridge BVI (verify + document; creation exists);
- per-interface GSO (jumbo MTU exists);
- LLDP (global + per interface, neighbour table);
- port mirroring (SPAN, ERSPAN via a GRE destination);
- the network delay simulator (test tool).

Reference: TNSR "Loopback, LLDP, SPAN"; VPP `gso`, `lldp`, `span`, `nsim` plugins (WBS D1.11, D1.8, D1.7, D1.10, D1.9 in `plan/wbs.csv`;
D1.9 is T3 "test tool, not a product feature").

## Inputs to read first
- **Loopbacks** exist end to end since P08: the Add dialog accepts `loop<N>` → `desired.Interfaces` (`KindLoopback`) →
  `interface.loopback/loop<N>` in `apps/agent/internal/descriptors/core/loopback.go`. Since TD-3, it is sanitized on create.
  Reuse it; do not edit core (manager-owned); do not build a second loopback UI.
- **BVI**: membership is F-bridge-l2's model and `l2.bridge-domain-member` port_type bvi. Consume both; add no descriptor.
- **LLDP + SPAN** (DF-7, merged): `apps/agent/internal/descriptors/{lldp,span}` + `docs/agent/descriptors/{lldp,span}.md`.
  - `lldp.global`: global; registered only by `lldp.RegisterGlobals`, so only on the globals owner (D-071); write-only.
  - `lldp.interface/<if>`: write-only; Update = disable + enable; Create verifies with `lldp_dump` (V20, `ErrIndexMismatch`).
  - **`lldp.Neighbours(ctx, client)`** returns the neighbour table (`lldp_dump`: chassis id, port id, TTL, last heard).
  - `span.mirror/<src>/<dst>/<device|l2>`: `sw_interface_span_enable_disable`; Retrieve `sw_interface_span_dump`.

  LLDP host tests need an index-aligned loopback (`df7test.AlignedLoopback`, V20) or skip with the reason.
- `apps/agent/binapi/gso/` (`feature_gso_enable_disable`, no dump) and `apps/agent/binapi/nsim/` (`nsim_configure2`,
  `nsim_cross_connect_enable_disable`, `nsim_output_feature_enable_disable`, **no dump or getter at all**): the new descriptors you write.
- **P08 (vertical slice, merged)**: the patterns you extend. Read `docs/status/vertical-slice.md` and `docs/status/wave-A-hotspots.md` first.
  - Builder package `apps/agent/internal/desired/` (`Sink`, `interface/<name>` alias, `Ptr`, `Assemble`). Your builders iterate
    `ds.Interfaces` / `ds.Services` from your own `desired/{gso,mirror,lldp,nsim}*.go`. Call them once from `project()` and once from
    `assemble()` in `apps/agent/internal/agent/projection.go` (after `desired.Assemble`). **Do not edit `desired/interfaces.go`.**
  - Registry `apps/agent/internal/subsystems/subsystems.go` (`Domains`, `Env.GlobalsOwner`); any new `Wiring` method goes in your own
    `subsystems/loopback_bvi_gso_lldp_span.go`.
  - Live state is a read-only agent RPC; the method goes in your own `internal/agent/rpc_loopback_bvi_gso_lldp_span.go`, and `server.go`
    needs no edit.
  - Topology test pattern `test/topology/interfaces/`.
- `packages/schema/src/domains/services.ts`: `services.lldp{enabled, systemName, txHold, txIntervalSec, interfaces[]}` **exists**.
  `interfaces.ts` has `mtu` (68–9216) but no gso/mirror fields.
- `docs/vpp-code-track.md`:
  - **V20**: lldp uses the sw_if_index as a hw index.
  - **V19/V21**: per-interface feature state survives interface deletion, so clear GSO/SPAN/nsim before an interface is deleted. If you
    find inherited state, append `### V-new (F-loopback-bvi-gso-lldp-span)` and ask the manager to extend
    `apps/agent/internal/vpp/ifsanitize` (TD-3, manager-owned; do not edit it).

## Contract changes
Additive only. Commit them first as separate `contract(schema): …` / `contract(proto): …` commits (as P08 did), with
`docs/status/tasks/F-loopback-bvi-gso-lldp-span-contract.md`; tell the manager in the questions file. Numbers come from your envelope,
never "next free".
- **Schema**, in your own `packages/schema/src/domains/ext/loopback-bvi-gso-lldp-span.ts` (one key line per field in the domain objects,
  one export line in `src/index.ts`):
  - `interfaces.<if>.gso?: boolean`;
  - `interfaces.<if>.mirror?[]{destination, direction: rx|tx|both, level: device|l2}`;
  - `services.nsim?{delayMs, bandwidthMbps, packetSize, dropFraction, crossConnect?{a,b}, outputInterfaces[]}`.

  LLDP config needs none.
- **State**: a read-only unary RPC `LldpNeighbors` → neighbours from `lldp.Neighbours` (bounded). Put the RPC under the service anchor
  and the messages in a `// ----- F-loopback-bvi-gso-lldp-span -----` section. In the same commit, add a stub `UNIMPLEMENTED` handler in
  `apps/api/src/testing/fake-agent.ts` and a section in `docs/contracts/proto.md`.

## Scope — build exactly this
1. **Schema**. New rules go in your own `packages/schema/src/semantic/loopback-bvi-gso-lldp-span.ts`, with one spread line in
   `semantic/index.ts` and rule names `<domain>.loopback-bvi-gso-lldp-span-…`. Examples in `packages/schema/examples/`.
   - mirror destination ≠ source, exists, and is not itself mirrored (no loops);
   - GSO only on interface types that support it (not sub-interfaces);
   - nsim values within VPP's ranges;
   - LLDP interface exists (the rule exists — test it).
2. **Agent**:
   - New `gso.interface/<if>`: write-only (D-063/D-076). Idempotent Create, or a boot-identity claim record per D-080 keyed on sw_if_index
     **and** logical name.
   - `nsim.config` is **global**: only the globals owner sets it (D-071); non-owners skip it with a warning. Plus `nsim.cross-connect` and
     `nsim.output`. All nsim objects are write-only.
   - Wire DF-7 `lldp`/`span` into the projection. `services.lldp` globals are applied only on the globals owner. Slot agents
     (`VRX_GLOBALS_OWNER=0`) project `lldp.interface` only and warn for the globals.
   - **`services` domain:** it becomes "implemented" as soon as one feature puts it in `subsystems.Domains`. From then on, every
     non-empty `services.*` leaf that no descriptor handles must produce `agent.unsupported-field` (dhcp, dns, snmp, ipfix, ntp, qos, …).
     Otherwise it is silently dropped and shows up as drift, because the API's drift view skips only reported pointers.
     - If `Domains["services"]` is not on main yet, create it and that warning list.
     - F-rpf-adl-pbr (`services.autoSdl`) may have created them first. Then append your names under your anchor, and take `lldp`/`nsim`
       off the unsupported list.
   - Unit tests with the fake client (model duplicate-add behaviour); an agent-level fake extension goes in
     `descriptors/core/coretest/loopback_bvi_gso_lldp_span.go`.
   - ONE host integration check (prefixed loopbacks/taps): Retrieve == desired for readable types; `vppctl show span` / `show lldp` /
     `show interface` contain it; rollback clears it; restart simulation. The simulation deletes dependents (mirror, GSO) before
     interfaces (D-095c).
   - ERSPAN evidence: the fixture creates a slot-prefixed ERSPAN GRE tunnel with DF-6's `gre` descriptor directly (the `tunnels` domain
     belongs to F-tunnels). The config mirrors to it by name.
   - nsim host evidence: `nsim_configure2` has no getter, so a test cannot restore the previous value (shared-host-rules §7). Keep the nsim
     host test opt-in (`VRX_NSIM_HOST=1`, globals lock exclusive), to run only in a manager VPP window. The default gate evidence for nsim
     is the fake client; say so.
3. **API**:
   - config via the pointer routes;
   - `GET /api/v1/state/lldp/neighbors` from `LldpNeighbors`, served by `LoopbackBviGsoLldpSpanController` in your own
     `apps/api/src/features/loopback-bvi-gso-lldp-span/` (`index.ts` exports `{controllers, providers}`; fake behaviour in `fake.ts`).
   - Mirror sessions and GSO show up in `/state/interfaces` items through `config`/`actual` once your assembler returns them. Do not edit
     `apps/api/src/state/**`.
4. **UI**:
   - No new loopback-create UI (P08's Add dialog does it).
   - GSO appears in the drawer's generated form via its `withUi` group hint; no drawer code.
   - LLDP page: global form + neighbour table.
   - Mirroring page: all sessions, edited through the generic merge-patch route.
   - nsim only under the Tools group (own route `tools/nsim`).
   - Routes and nav items go through the router/nav anchors; labels are in your own namespace. en + fa.
   - Only if `mirror` breaks the generated drawer form, exclude it with one named line in `apps/web/src/domains/interfaces/model.ts`.
5. **Docs**: `docs/user/interfaces/loopback-bvi-gso-lldp-span.md` (loopback as BVI, ERSPAN to a GRE tunnel, LLDP, nsim as a lab tool).
   Add one see-also line at the end of `docs/user/interfaces/basics.md`; do not edit its "Not in this release" line.

**Files you own:**
- `apps/agent/internal/descriptors/{lldp,span,gso,nsim}/**`, `docs/agent/descriptors/{lldp,span,gso,nsim}.md`
- `apps/agent/internal/desired/{gso,mirror,lldp,nsim}*.go`, `apps/agent/internal/agent/rpc_loopback_bvi_gso_lldp_span*.go`,
  `apps/agent/internal/subsystems/loopback_bvi_gso_lldp_span*.go`
- `apps/agent/internal/descriptors/core/coretest/loopback_bvi_gso_lldp_span*.go`
- `apps/api/src/features/loopback-bvi-gso-lldp-span/**`, `apps/api/test/e2e/loopback-bvi-gso-lldp-span*.ts`
- `apps/web/src/domains/interfaces/loopback-bvi-gso-lldp-span/**`, `apps/web/src/locales/*/loopback-bvi-gso-lldp-span.json`
- `packages/schema/src/domains/ext/loopback-bvi-gso-lldp-span*.ts`, `packages/schema/src/semantic/loopback-bvi-gso-lldp-span*.ts`
- `packages/schema/examples/loopback-bvi-gso-lldp-span-*.json`, `packages/proto/test/fixtures/loopback-bvi-gso-lldp-span-*.json`
- `docs/user/interfaces/loopback-bvi-gso-lldp-span.md`, `test/topology/loopback-bvi-gso-lldp-span/**`
- `docs/status/tasks/F-loopback-bvi-gso-lldp-span*.md`

**Shared hotspots: insert only under your `wave-A: F-loopback-bvi-gso-lldp-span` anchor, and name each hunk in the task's status file:**
- agent: `apps/agent/internal/subsystems/subsystems.go` (a new `Domains["services"]` entry), `apps/agent/internal/agent/projection.go`
- schema: `packages/schema/src/domains/{interfaces,services}.ts`, `packages/schema/src/index.ts`, `packages/schema/src/semantic/index.ts`
- proto: `packages/proto/vrx/v1/dataplane.proto`, `docs/contracts/proto.md`
- API: `apps/api/src/app.module.ts`, `apps/api/src/agent/agent.client.ts`, `apps/api/src/testing/fake-agent.ts`
- web: `router.tsx`, `nav/nav.ts`, `nav/nav.test.ts`, `i18n.ts`
- docs: `docs/user/interfaces/basics.md` (end), `docs/vpp-code-track.md` (append)

Generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs`.

## Acceptance (paste the evidence)
- [ ] `vppctl show span`, `show lldp`, `show interface <loopN>` and `show bridge-domain <id> detail` (BVI) reflect the commit (pasted)
- [ ] Agent-restart simulation → loopback, mirror, LLDP, GSO back within 30 s (log excerpt; write-only types re-applied exactly once)
- [ ] Rollback removes mirror/LLDP/GSO/nsim and the loopback (Retrieve for readable types, `vppctl` for write-only ones)
- [ ] Mirror destination equal to its source → 400 problem+json with `pointer`
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-loopback-bvi-gso-lldp-span.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
- Bridge domains and BVI membership descriptors (F-bridge-l2).
- Loopback creation (exists since P08).
- GRE tunnel creation for ERSPAN (F-tunnels; use a fixture tunnel).
- Bonds (F-bonding).
- Sub-interfaces (F-vlan-qinq).
- Checksum/TSO offload flags of DPDK devices (startup.conf, F-startup-gen).
- SNMP LLDP-MIB (F-snmp).
- Packet capture (F-capture-trace).
- The other `services.*` leaves (report them as unsupported).
- New lldp/span descriptors for objects DF-7 already has.

## Open questions to surface, not to decide silently
- LLDP on slot agents: `lldp.global` exists only on the globals owner, so on the shared host the system name and TX timers stay at VPP's
  current values. Record what the evidence shows.
- nsim is a test tool: confirm it belongs in the product UI at all (default: Tools group, marked "lab").
