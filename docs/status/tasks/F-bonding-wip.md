# F-bonding — WIP log (slot 6, worktree /root/ngfw-wt/F-bonding, branch task/F-bonding)

Started 2026-09-24T19:10 from task/W-seed@df67a8e (speculative, D-114). Time box 15 h.

## Plan
1. contract(schema): `domains/ext/bonding.ts`, `semantic/bonding.ts` (+ tests, examples), one key / export / spread line
2. contract(proto): `Interface.bond = 13`, `Bond`/`BondMember`, rpc `BondState` (+ `BondState*`/`BondStatus*`), fake-agent stub,
   proto.md section, fixture, regenerated code; F-bonding-contract.md + questions
3. agent: `bond.member-weight` descriptor, bond.bond KeyProvider, `desired/bond.go`, kind in interfaces.go, registry, projection,
   `rpc_bonding.go`, coretest/bonding.go, unit tests
4. API: `features/bonding` (BondingController, fake.ts, index.ts), agent client method, app.module spreads, e2e
5. UI: Bonds page `/interfaces/bonds`, nav/router/i18n lines, en/fa `bonding.json`
6. host: `test/topology/bonding` (real agent + API + DB, fixture taps w6…, no packets), screenshot, restart simulation
7. docs: `docs/user/interfaces/bonding.md`, basics.md see-also, bond.md descriptor doc, F-bonding.md

## Log
- 19:10 read context, envelope, hotspots, DF-1 bond descriptors, P08 builder/registry/projection, sibling F-bridge-l2 branch patterns.
- 19:25 coordinator: VPP crashed 18:41:08 (NRestarts 0→1, baseline now 1). D-126: never sweep classify bindings by index; unbind
  only what you recorded; minimal packet phases; never 802.1ad on the rig. → this task sends NO packets; the topology test does not
  copy P08's V19-guard reset (it resets classify bindings by sw_if_index).
