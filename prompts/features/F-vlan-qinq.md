# Task: F-vlan-qinq — 802.1q sub-interfaces and QinQ   (prepend 00-CONTEXT.md)

> Regenerated 2026-09-24 (D-104) against the merged DF-1 descriptors and the P08 vertical slice. Most of the stack already
> exists: single-tag VLAN sub-interfaces work end to end since P08. **This task closes the QinQ / 802.1ad gap and proves it.**
> It does not build a sub-interface descriptor, a sub-interface projection or a new interfaces screen.

## Goal
Make stacked VLANs (802.1ad outer + 802.1q inner, and 802.1q-in-802.1q) a verified, documented, first-class part of the interfaces
feature in FAST MODE: prove QinQ through candidate → commit → Retrieve → `vppctl` → rollback → restart simulation, show the tag stack
in the UI, and fix whatever gap that proof uncovers. Reference: TNSR "VLAN 802.1q / QinQ 802.1ad"; VPP `create_subif` (WBS D1.4).

## Dependencies (must be merged before you start)
- **DF-1** (merged): `apps/agent/internal/descriptors/interface/subinterface.go`: `interface.subinterface/<parent id>.<sub_id>`,
  `create_subif`/`delete_subif`, Retrieve from `sw_interface_dump` (`sub_number_of_tags`, `sub_outer/inner_vlan_id`, `sub_if_flags`),
  dot1q / dot1ad / QinQ / exact-match via `sub_if_flags`, Update = `ErrRecreate`, alias `interface/<parent>.<id>`. Docs:
  `docs/agent/descriptors/interface.md`.
- **P08** (vertical slice): `apps/agent/internal/desired/interfaces.go` already projects `interfaces.<if>.subinterfaces.<id>` →
  `interface.subinterface` (**it passes `OuterVlan`, `InnerVlan`, `Dot1Ad`, `ExactMatch: true`**) + alias + admin/MTU/VRF/addresses/DHCP, and
  assembles them back. The `InterfaceState` RPC reports `parent`, `vlan_id`, `inner_vlan_id`. `GET /api/v1/state/interfaces` lists sub-interfaces
  as their own rows with `parent`. The drawer in `apps/web/src/domains/interfaces/InterfaceDrawer.tsx` has a sub-interface table with
  add/edit/remove through the generated `SubinterfaceSchema` form. Read `docs/status/vertical-slice.md` and `docs/status/tasks/P08.md` first.
  If P08 is not merged yet, read it with `git show task/P08:<path>` and do not start coding.

## Inputs to read first
- `packages/schema/src/domains/interfaces.ts`: `SubinterfaceSchema{vlanId, innerVlanId?, dot1ad (default false), …commonFields}` **exists**.
  `packages/schema/src/semantic/interfaces.ts`: `interfaces.vlan-unique` (tag stack unique per parent, pointer to the second entry) and
  `interfaces.subinterface-mtu` (sub MTU ≤ parent MTU) **exist**. In `primitives.ts`, `vlanId` = 1–4094.
- `packages/proto/vrx/v1/dataplane.proto`: `Subinterface{vlan_id, inner_vlan_id, dot1ad, …}` exists. `InterfaceState` has no dot1ad flag, so
  take the tag type from the row's `actual` / `config` (no contract needed).
- `docs/agent/descriptors/interface.md` (sub-interface row, alias + logical-name rules D-065/D-069/D-073a)
- `test/topology/interfaces/` (P08): the topology/restart-safety test pattern you copy (own Go module, rig hand-over, V19 guard, D-101 quiesce)
- `docs/lab/shared-host-rules.md`: slot prefix, rig names `host-<prefix>l0|w0`, addresses `10.<N>.0.0/16`

## Contract changes
None expected: schema, semantic rules and proto already carry QinQ. If you find a real gap, change it additively only, on `contract/F-vlan-qinq`
(`contract(schema|proto): …`, `docs/status/tasks/F-vlan-qinq-contract.md`), tell the manager in `F-vlan-qinq-questions.md`, and continue.

## Scope — build exactly this
1. **Schema (tests only unless a rule is missing)**: unit tests in `packages/schema` for QinQ:
   - dot1ad + inner is accepted;
   - the same outer tag once as dot1q and once as dot1ad on one parent is accepted (distinct stacks);
   - a duplicate `(dot1ad, vlanId, innerVlanId)` is rejected with the pointer to the second entry;
   - `innerVlanId` without `vlanId` is rejected;
   - sub MTU > parent MTU is rejected.

   Add a rule only if one of these fails.
2. **Agent (gap-fixing only)**: add QinQ cases to the P08 builder tests (`desired` package, fake client):
   - dot1ad outer 200 + inner 100;
   - dot1q-in-dot1q;
   - a tag change → recreate;
   - an assemble round-trip (Retrieve → document equals desired, with `dot1ad` default false omitted or false canonically).

   Fix `desired/interfaces.go` or `descriptors/interface/subinterface.go` **only** if a test shows a defect, and say which in the PR.
3. **API**: nothing new. `GET /api/v1/state/interfaces` must return QinQ rows with `vlanId` + `innerVlanId` + `parent`. Add an API e2e case
   (fake agent) if the existing one does not cover `innerVlanId`.
4. **UI**:
   - Extract the drawer's sub-interface table into `apps/web/src/domains/interfaces/subinterfaces/SubinterfaceTable.tsx` (one-hunk
     replacement in `InterfaceDrawer.tsx`).
   - Show the tag stack in these columns: **Encapsulation** (`dot1q 100` / `dot1ad 200 · dot1q 100`), **Inner VLAN**, admin/link status
     from live state, and addresses.
   - The edit dialog keeps using the generated `SubinterfaceSchema` form (its encapsulation group already has vlanId / innerVlanId / dot1ad).
   - Add en + fa strings: new keys go in `vlan-qinq.json`; the existing `sub.*` keys in `interfaces.json` stay.
   - Add a Vitest for the tag-stack formatter.
   - Take a screenshot (en + fa/RTL) against the real endpoint.
5. **Docs**: write `docs/user/interfaces/vlan-qinq.md` (dot1q, dot1ad+dot1q QinQ, exact-match note, REST + CLI equivalent). In
   `docs/user/interfaces/basics.md`, change the one line "QinQ is not supported by this release" to a link to the new page.

**Files you own:**
- `apps/web/src/domains/interfaces/subinterfaces/**`
- `apps/web/src/locales/{en,fa}/vlan-qinq.json`
- `docs/user/interfaces/vlan-qinq.md`
- `test/topology/vlan-qinq/**`
- `docs/status/tasks/F-vlan-qinq*.md`
- new test files `apps/agent/internal/desired/interfaces_qinq_test.go` and `packages/schema/src/semantic/interfaces-qinq.test.ts`

**Shared, minimal hunks only (state each in the PR):**
- `apps/web/src/domains/interfaces/InterfaceDrawer.tsx` (swap the table for the component)
- `apps/web/src/i18n.ts` (namespace)
- `docs/user/interfaces/basics.md` (one line)
- only for a proven defect: `apps/agent/internal/desired/interfaces.go`, `apps/agent/internal/descriptors/interface/subinterface.go`,
  `packages/schema/src/semantic/interfaces.ts`

## Acceptance (paste the evidence)
- [ ] Through the API (candidate → commit) on the slot rig parent `host-<prefix>w0`: `.100` = dot1q 100 and `.200` = dot1ad 200 + dot1q 100,
      each with an address. `Retrieve()` == desired, and `vppctl show interface <parent>.200` / `show interface address` show both tag stacks and
      addresses (pasted)
- [ ] Agent-restart simulation (stop your agent, delete the two sub-interfaces via binapi, start it) → both are back with addresses within 30 s (log excerpt)
- [ ] Rollback deletes both sub-interfaces (Retrieve shows none; `vppctl show interface` has no `<parent>.200`)
- [ ] A duplicate `(dot1ad, vlanId, innerVlanId)` on one parent → 400 problem+json with `pointer` to the second entry
- [ ] UI screenshot (en + fa) of the drawer with the QinQ row in `docs/status/tasks/F-vlan-qinq.md`
- [ ] `tools/ci.sh --base main` green in your worktree

(A packet-level test is not required. Optional: tagged frames with scapy from `ns-<prefix>-wan`; say if you did it.)

## Out of scope (do not build)
- **Use, do not rebuild:**
  - DF-1 `interface.subinterface` / alias / admin-state / MTU descriptors;
  - P08's `desired.Interfaces` projection and assembler, `subsystems` registry, `InterfaceState` RPC and `GET /state/interfaces`;
  - the interfaces list page, drawer and SchemaForm.

  No new descriptor package, no second projection, no new state route, no new screen.
- Bonding (F-bonding).
- Bridge domains / L2 xconnect / VLAN tag rewrite on sub-interfaces (F-bridge-l2).
- LLDP (F-loopback-bvi-gso-lldp-span).
- `unnumbered` (no descriptor; stays `agent.unsupported-field`).
- Non-exact-match / default / untagged / outer-any sub-interfaces in the product model.
- DPDK-path tests (handover).
- linux-cp mirrors of sub-interfaces (P12).

## Open questions to surface, not to decide silently
- P08 hard-codes `exact-match` for every sub-interface (routed L3). TNSR also offers non-exact-match for L2 use. Record whether the model
  should expose it (default: no; F-bridge-l2 decides for L2 sub-interfaces).
