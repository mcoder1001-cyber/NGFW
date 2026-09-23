# Task: F-bonding — LACP / XOR / round-robin / active-backup bonds   (prepend 00-CONTEXT.md)

## Goal
Implement link aggregation end to end in FAST MODE: bond interfaces (modes LACP, XOR, round-robin, active-backup, broadcast) with
their member NICs, load-balance hash, LACP rate, member weight and live LACP/member state.
Reference: TNSR "Bond interfaces / LACP"; VPP `bond` + `lacp` plugins (WBS D1.5 in `plan/wbs.csv`).

## Inputs to read first
- `apps/agent/internal/descriptors/bond/` + `docs/agent/descriptors/bond.md` (DF-1, merged) — **reuse** `bond.bond`
  (`bond.bond/<name>`, `bond_create2`, Update = ErrRecreate, VPP forces lb for rr/ab/broadcast — the model must carry the forced value)
  and `bond.member` (`bond.member/<bond>/<member>`, `bond_add_member` with `is_passive`/`is_long_timeout`, `bond_detach_member`)
- `apps/agent/binapi/bond/` (`sw_interface_set_bond_weight`, `sw_bond_interface_dump`, `sw_member_interface_dump`) and
  `apps/agent/binapi/lacp/` (`sw_interface_lacp_dump`) — the only source of message names
- `docs/agent/descriptors/interface.md` — alias `interface/<name>` (D-065), logical names (D-069), physical-NIC ClaimStore (D-075)
- `packages/schema/src/domains/interfaces.ts` — **no bond model exists**: `interfaces` is keyed by parent name (`BondEthernet0` is a
  legal key) but has no mode/members fields
- `docs/lab/shared-host-rules.md` (slot prefix, own bond ids from your table range); VPP 26.06 docs → "Bonding"

## Contract changes
Additive, on `contract/F-bonding` first (commit `contract(schema): interfaces bond`, `docs/status/tasks/F-bonding-contract.md`,
tell the manager in `F-bonding-questions.md`, continue on your branch): `interfaces.<name>.bond?{ mode: lacp|xor|round-robin|
active-backup|broadcast, loadBalance?: l2|l23|l34, members: { <ifName>: { passive?, longTimeout?, weight? } }, numaOnly?, id? }`
+ matching proto fields. Never reshape existing fields.

## Scope — build exactly this
1. **Schema** (on the contract branch): members exist, are physical/non-bond, not members of two bonds, carry no addresses/VRF/
   sub-interfaces of their own; `loadBalance` only for xor/lacp; `passive`/`longTimeout` only for lacp; `weight` only for active-backup.
2. **Agent**: projection `interfaces.<bond>.bond` → `bond.bond` + `bond.member` (+ a small `bond.member-weight` descriptor inside
   `descriptors/bond/` for `sw_interface_set_bond_weight` if DF-1 lacks it). Bond then behaves like any interface: admin/MTU/addresses/
   VRF/sub-interfaces via the existing DF-1/core descriptors keyed `interface/<bond name>` — test that. Retrieve covers bonds + members;
   unit tests with the fake client; ONE integration check on host VPP (`VRX_INTEGRATION=1`, lab lock shared, members = your slot's
   prefixed tap/af_packet interfaces — never real NICs): Retrieve == desired, `vppctl show bond details` shows it, rollback leaves nothing,
   agent-restart simulation recreates it.
3. **API**: config via pointer routes; `GET /api/v1/state/interfaces/bonds` (mode, lb, members with LACP actor/partner state from
   `sw_interface_lacp_dump`, active member).
4. **UI**: "Bonds" tab on the interfaces screen: list + SchemaForm (member picker filtered to eligible NICs) + live member/LACP state column; en+fa.
5. **Docs**: `docs/user/interfaces/bonding.md` (LACP with 2 members, active-backup; CLI equivalent).

Files you own: `apps/agent/internal/descriptors/bond/**`, `docs/agent/descriptors/bond.md`, `apps/agent/internal/agent/project_bonding*.go`,
`apps/api/src/features/bonding/**`, `apps/web/src/domains/interfaces/bonding/**`, `apps/web/src/locales/*/bonding.json`,
`docs/user/interfaces/bonding.md`, `test/topology/bonding/**`.
Shared files: one-line appends only (agent registry/projection hook, `app.module.ts`, web router/nav); `packages/api-client` regenerated.

## Acceptance (paste the evidence)
- [ ] `vppctl show bond details` shows the committed LACP bond with both members and lb l34; Retrieve == desired (pasted)
- [ ] Agent-restart simulation → bond + members + bond address back within 30 s (log excerpt)
- [ ] Rollback removes bond and memberships; members return to plain L3 interfaces (Retrieve, not assumption)
- [ ] Member already in another bond → 400 problem+json with `pointer` to the second membership
- [ ] UI screenshot against the real endpoint in `docs/status/tasks/F-bonding.md`
- [ ] `tools/ci.sh --base main` green in your worktree

## Out of scope (do not build)
VLAN sub-interfaces on bonds beyond "they work via F-vlan-qinq's descriptor"; bridge domains (F-bridge-l2); LLDP on members
(F-loopback-bvi-gso-lldp-span); linux-cp pairs for bonds and their V1 edge cases (P12); DPDK NIC binding / startup.conf (F-startup-gen);
GSO on bonds; bond MAC override beyond the existing `interface.mac-address`.

## Open questions to surface, not to decide silently
LACP on af_packet members may not negotiate on the rig (no partner) — evidence is configuration + `show lacp` state, say so.
Whether `broadcast` mode should be offered at all (TNSR omits it).
