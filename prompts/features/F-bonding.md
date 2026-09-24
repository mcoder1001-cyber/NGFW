# Task: F-bonding — LACP / XOR / round-robin / active-backup bonds   (prepend 00-CONTEXT.md)

> Checked 2026-09-24 against main + P08 (wave-A prep). The bond descriptors already exist (DF-1). P08 already provides the builder,
> the registry, the projection hook, the live-state RPC pattern and the topology-test pattern. This task builds the bond model, its
> projection, the bond state RPC, the API module, the screen, the tests and the docs. Shared files follow
> `docs/status/wave-A-hotspots.md`: anchors, allocated numbers, and logic only in your own files.

## Goal
Implement link aggregation end to end in FAST MODE: bond interfaces (modes LACP, XOR, round-robin, active-backup, broadcast) with
their member NICs, load-balance hash, LACP rate, member weight and live LACP/member state.
Reference: TNSR "Bond interfaces / LACP"; VPP `bond` + `lacp` plugins (WBS D1.5 in `plan/wbs.csv`).

## Inputs to read first
- `apps/agent/internal/descriptors/bond/` + `docs/agent/descriptors/bond.md` (DF-1, merged). **Reuse** these:
  - `bond.bond`: `bond.bond/<name>`, `bond_create2`, Update = ErrRecreate. VPP forces lb for rr/ab/broadcast, so the model must carry the
    forced value. Since TD-3, its Create runs the V19 sanitizer.
  - `bond.member`: `bond.member/<bond>/<member>`, `bond_add_member` with `is_passive`/`is_long_timeout`, `bond_detach_member`.

  DF-1 has **no weight object**. The weight can be read back (`sw_member_interface_dump.weight`), so the new weight descriptor gets a real
  Retrieve and is not write-only.
- **P08 (vertical slice, merged)**: the patterns you extend. Read `docs/status/vertical-slice.md` and `docs/status/wave-A-hotspots.md` first.
  - Builder package `apps/agent/internal/desired/`: `Sink`, `interface/<name>` alias references, `Assemble`. Today `KindOf` treats a bond
    name as `KindExisting` (an alias without a creator, so the interface must already exist). In `interfaces.go` you add **only** the bond
    kind: one const, one `KindOf` case, and one creator case (alias creator `bond.bond/<name>`).
  - Your builder and assembler go in `desired/bond*.go`. Call them once from `project()` and once from `assemble()` in
    `apps/agent/internal/agent/projection.go`; the `assemble()` call comes after `desired.Assemble` and adds `bond` to the assembled
    interfaces.
  - Registry `apps/agent/internal/subsystems/subsystems.go`: `Domains["interfaces"]` gets your descriptor names, one per line, and
    `bond.Register` goes at the end of `Register()`. Any new `Wiring` method goes in your own `subsystems/bonding.go`.
  - Live state is a read-only agent RPC merged in the API (the `InterfaceState` pattern). The method goes in your own
    `internal/agent/rpc_bonding.go`; the server embeds `UnimplementedDataplaneServer`, so `server.go` needs no edit.
  - Topology test pattern `test/topology/interfaces/`: own Go module, real agent + API + DB per slot, rig hand-over, V19 guard, D-101
    veth quiesce before any af_packet delete, `NRestarts` checked before and after.
- `apps/agent/binapi/bond/` (`sw_interface_set_bond_weight`, `sw_bond_interface_dump`, `sw_member_interface_dump`) and
  `apps/agent/binapi/lacp/` (`sw_interface_lacp_dump`): the only source of message names.
- `docs/agent/descriptors/interface.md`: alias `interface/<name>` (D-065), logical names (D-069), physical-NIC ClaimStore (D-075).
- `packages/schema/src/domains/interfaces.ts`: **no bond model exists**. `interfaces` is keyed by parent name (`BondEthernet0` is a
  legal key) but has no mode or members fields.
- `docs/lab/shared-host-rules.md` (slot prefix; take bond ids from your table range); VPP 26.06 docs → "Bonding".

## Contract changes
Additive only. Commit them first as separate `contract(schema): interfaces bond` / `contract(proto): …` commits (as P08 did), with
`docs/status/tasks/F-bonding-contract.md`. Tell the manager in `F-bonding-questions.md` and continue without waiting.
- **Schema.** The sub-schema goes in your own `packages/schema/src/domains/ext/bonding.ts`, plus one key line in `InterfaceSchema` and one
  export line in `packages/schema/src/index.ts`:
  `interfaces.<name>.bond?{ mode: lacp|xor|round-robin|active-backup|broadcast, loadBalance?: l2|l23|l34, members: { <ifName>: { passive?,
  longTimeout?, weight? } }, numaOnly?, id? }`.
- **Proto** (numbers from your envelope, never "next free"):
  - `Interface.bond = 13`, with explicit presence (D-039);
  - a read-only unary RPC `BondState` that returns the bonds: mode, lb, member and active counts, and per member the weight + LACP
    actor/partner state (`sw_interface_lacp_dump`). Keep it bounded (bonds are few). Put the RPC under the service anchor and the messages
    in a `// ----- F-bonding -----` section;
  - in the same commit, a stub `UNIMPLEMENTED` handler in `apps/api/src/testing/fake-agent.ts` (its `DataplaneServer` is exhaustively
    typed);
  - a `### F-bonding: BondState` section in `docs/contracts/proto.md`.

Never reshape existing fields.

## Scope — build exactly this
1. **Schema** (contract commit). Rules:
   - members exist, are physical/non-bond, belong to at most one bond, and have no addresses/VRF/sub-interfaces of their own;
   - `loadBalance` only for xor/lacp; `passive`/`longTimeout` only for lacp; `weight` only for active-backup.

   Put them in your own `packages/schema/src/semantic/bonding.ts` (exporting `bondingValidators`, rule names `interfaces.bonding-…`), with one
   spread line in `semantic/index.ts`. Examples go in `packages/schema/examples/bonding-*.json`.
2. **Agent**:
   - In `desired/bond*.go`, project `interfaces.<bond>.bond` → `bond.bond` + `bond.member`, plus a small `bond.member-weight` descriptor
     inside `descriptors/bond/` for `sw_interface_set_bond_weight`.
   - A bond then behaves like any interface: admin/MTU/addresses/VRF/sub-interfaces through the existing DF-1/core descriptors keyed
     `interface/<bond name>`. Test that.
   - Retrieve covers bonds, members and weights. Unit tests with the fake client; an agent-level fake extension, if you need one, goes in
     `descriptors/core/coretest/bonding.go`.
   - ONE integration check on host VPP (`VRX_INTEGRATION=1`, lab lock shared): Retrieve == desired, `vppctl show bond details` shows it,
     rollback leaves nothing, the agent-restart simulation recreates it.
   - Members: through the config the agent creates only `loop<N>` and `host-<netdev>` interfaces. So the topology-test members are either
     af_packet host-interfaces on slot-prefixed veths (bring the veth down before any af_packet delete, D-101) or slot-prefixed taps made
     by the test fixture (untagged, so claimed through the ClaimStore). Never real NICs.
3. **API**:
   - config via the pointer routes;
   - `GET /api/v1/state/interfaces/bonds` (mode, lb, members with LACP actor/partner state, active member count), served from `BondState`
     by `BondingController` in your own `apps/api/src/features/bonding/`;
   - `index.ts` exports `{controllers, providers}`; `app.module.ts` gets one import line and one spread per array, under the anchors;
   - the fake-agent behaviour goes in `features/bonding/fake.ts`;
   - an e2e test with the fake agent.
4. **UI**:
   - A "Bonds" page in the Interfaces nav group, on its own route (e.g. `/interfaces/bonds`), added through the router/nav anchors. Do not
     restructure P08's `InterfacesPage.tsx`.
   - The nav label key lives in your `bonding` namespace.
   - List + SchemaForm (member picker filtered to eligible NICs) + a live member/LACP state column; en + fa.
   - P08's drawer form is generated from `InterfaceSchema`, so `bond` shows up there under its `x-vrx-ui` group. Only if that breaks the
     drawer, exclude it with one named line in `apps/web/src/domains/interfaces/model.ts`.
5. **Docs**: `docs/user/interfaces/bonding.md` (LACP with 2 members, active-backup; CLI equivalent). Add one see-also line at the end of
   `docs/user/interfaces/basics.md`. Do not edit its "Not in this release" line; the manager updates it after the wave.

**Files you own:**
- `apps/agent/internal/descriptors/bond/**`, `docs/agent/descriptors/bond.md`
- `apps/agent/internal/desired/bond*.go`, `apps/agent/internal/agent/rpc_bonding*.go`, `apps/agent/internal/subsystems/bonding*.go`
- `apps/agent/internal/descriptors/core/coretest/bonding*.go`
- `apps/api/src/features/bonding/**`, `apps/api/test/e2e/bonding*.ts`
- `apps/web/src/domains/interfaces/bonding/**`, `apps/web/src/locales/*/bonding.json`
- `packages/schema/src/domains/ext/bonding*.ts`, `packages/schema/src/semantic/bonding*.ts`
- `packages/schema/examples/bonding-*.json`, `packages/proto/test/fixtures/bonding-*.json`
- `docs/user/interfaces/bonding.md`, `test/topology/bonding/**`, `docs/status/tasks/F-bonding*.md`

**Shared hotspots: insert only under your `wave-A: F-bonding` anchor, and name each hunk in `F-bonding.md`:**
- agent: `apps/agent/internal/desired/interfaces.go` (the kind only), `apps/agent/internal/subsystems/subsystems.go`,
  `apps/agent/internal/agent/projection.go`
- schema: `packages/schema/src/domains/interfaces.ts`, `packages/schema/src/index.ts`, `packages/schema/src/semantic/index.ts`
- proto: `packages/proto/vrx/v1/dataplane.proto`, `docs/contracts/proto.md`
- API: `apps/api/src/app.module.ts`, `apps/api/src/agent/agent.client.ts`, `apps/api/src/testing/fake-agent.ts`
- web: `router.tsx`, `nav/nav.ts`, `nav/nav.test.ts`, `i18n.ts`
- docs: `docs/user/interfaces/basics.md` (end)

Generated files are never hand-merged: `pnpm gen && make -C apps/cli gen docs` regenerates `apps/agent/gen`, `packages/proto/gen`,
`packages/api-client`, `apps/cli/internal/api/operations_gen.go` and `docs/user/cli/reference.md`.

## Acceptance (paste the evidence)
- [ ] `vppctl show bond details` shows the committed LACP bond with both members and lb l34; Retrieve == desired (pasted)
- [ ] Agent-restart simulation → bond + members + bond address back within 30 s (log excerpt)
- [ ] Rollback removes bond and memberships; members return to plain L3 interfaces (Retrieve, not assumption)
- [ ] Member already in another bond → 400 problem+json with `pointer` to the second membership
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-bonding.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
- VLAN sub-interfaces on bonds, beyond showing that they work through DF-1's `interface.subinterface` and P08's projection (F-vlan-qinq verifies QinQ).
- Bridge domains (F-bridge-l2).
- LLDP on members (F-loopback-bvi-gso-lldp-span).
- linux-cp pairs for bonds and their V1 edge cases (P12).
- DPDK NIC binding / startup.conf (F-startup-gen).
- GSO on bonds.
- Bond MAC override beyond the existing `interface.mac-address`.
- A new descriptor for anything DF-1 already has.

## Open questions to surface, not to decide silently
- LACP on af_packet/tap members may not negotiate on the rig (no partner). The evidence is then configuration + `show lacp` state; say so.
- Whether `broadcast` mode should be offered at all (TNSR omits it).
