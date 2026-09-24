# F-bonding — questions and decisions for the manager

Written while working; nothing here blocks the task (00-CONTEXT: decide, log here, continue).

## CONTRACT — please review (additive)
`contract(schema): interfaces bond …` and `contract(proto): F-bonding …` are on `task/F-bonding`; details and evidence in
`F-bonding-contract.md`. Numbers: only `Interface.bond = 13` (§2). RPC `BondState`, messages `Bond*`.

## Decisions taken (with options)

**F-B1 — bond interface names.** (a) the key is `BondEthernet<id>` (VPP's name, as TNSR), `bond.id` optional and equal to
the number, `KindOf` recognises the name; a `BondEthernet<id>` without a `bond` leaf keeps today's meaning (a pre-existing
bond, alias without creator) · (b) any key + `bond` leaf, `id` mandatory. **Chose (a)**: `KindOf(name)` only sees the name
(the envelope allows one const, one case, one creator case in interfaces.go), key = logical = VPP name (D-069) removes a
mapping, and it is TNSR's model. (b) would need `KindOf` to read the document (a signature change in a P08 hotspot).

**F-B2 — schema examples.** The prompt puts examples in `packages/schema/examples/bonding-*.json`, but
`packages/schema/src/examples.test.ts` (not mine) fails for any file whose prefix is not in its `SIBLING` regex
(`nat|objects|acl|vpn|tunnels|services|ha`). (a) add `bonding` to that regex (a non-owned file) · (b) put the valid
document in `packages/proto/test/fixtures/bonding-lacp-ab.json` (owned; directory-scanned by both proto corpora and read by
my semantic test) and keep invalid cases inline in `semantic/bonding.test.ts`. **Chose (b).** If you want schema examples,
extend `SIBLING` with `bonding` and I (or the merger) move the file.

**F-B3 — `weight` range 1–255** (VPP takes a u32; 0 is its "never set" and would be invisible to Retrieve, like an MTU equal
to the default). Relaxing later is additive.

**F-B4 — `bond.bond` provides `interface/<name>` (scheduler `KeyProvider`)** — the D-125 alternative to waiting for TD-11c:
without it a delete-only plan (rollback) has no edge from the bond's address / members (which depend on the alias) to
`bond.bond` and orders them by registration rank, i.e. deletes the bond before its address. Implemented in my own
`descriptors/bond/bond.go` (`TestBondProvidesAlias`; `TestBondingOnFake` asserts the call order; on the host the rollback
deleted weight → members → admin state → address → bond). The envelope predates D-125, so it does not name this
obligation; please confirm it satisfies "a KeyProvider obligation" for the merge order (see Q4 for what it does not cover).

**F-B5 — member NICs on the host are fixture taps**, not af_packet host-interfaces: the envelope allows both; taps avoid
af_packet deletes entirely (V24/D-101) and need no rig. They are untagged (claimed through the persisted ClaimStore like a
DPDK NIC), VPP name `tap6000…6002` (ids from the slot range), host side `w6bm0…2` kept down with IPv6 off, so the kernel
sends nothing into VPP. No packet is sent by the test (LACP evidence = config + `show lacp`).

**F-B6 — `bond.member-weight` is registered by `subsystems/bonding.go`, not inside `bond.Register`**: DF-1's
`descriptors/interface/registry_test.go` (not mine) pins `bond.Register` to exactly `bond.bond` + `bond.member`.

**F-B7 — P08's drawer excludes `bond`** (the one named line in `apps/web/src/domains/interfaces/model.ts` the prompt allows):
with `bond` in `InterfaceSchema` the drawer's SchemaForm fills the absent optional object with defaults (P08-questions Q2)
and its required `mode` then blocks every save — two P08 drawer tests failed (no PATCH sent). Bonds are edited on the
Bonds page.

**F-B8 — one line in `descriptors/core/coretest/fakevpp.go`** (`v.installBonding()` in `New()`, right after `v.install()`,
one line away from F-bridge-l2's `installBridgeL2()`): A6 says existing coretest files are read-only, but every agent test
on `coretest.New()` retrieves `bond.bond` now (it is in the interfaces domain), so the model must answer the bond dumps;
the model itself is in my own `coretest/bonding.go`. Same approach as F-bridge-l2.

**F-B9 — no `react-hook-form` in apps/web** (not a declared dependency), so the member picker is not a SchemaForm custom
widget: like P08's sub-interface table, the Bonds drawer has a SchemaForm for the bond fields and a members table whose
rows are edited in a SchemaForm dialog on the member schema; the picker lists only eligible NICs.

## Open questions (surfaced, not decided silently)

**Q1 — broadcast mode.** TNSR omits it; the prompt's schema lists it. It is in the enum (contract as specified) and the UI
offers it. Remove before v1 tag? (removing an enum value later is a reshape → PENDING).

**Q2 — LACP on the rig.** Members are slot-prefixed fixture taps (no partner), so LACP never reaches
collecting-distributing; the evidence is configuration + `show lacp` (actor state, partner all-zero), as the envelope
expects.

**Q3 — D-126 and `bond.bond` Delete.** DF-1's `bond.bond` Delete calls TD-3's `ifsanitize.BeforeDelete`, which probes/unbinds
classify tables on the bond's own sw_if_index right before `bond_delete`. D-126 forbids sweeping bindings by index in
*tests*; I assume the agent-side sanitizer (manager-owned, on our own tagged interface) is not what D-126 targets. My
topology test sends no packets and does no binding sweep of its own.

**Q4 — sub-interface removal ordering (TD-11c 3.1c, general, not bond-specific).** A delete plan has no edge from the
attributes of a sub-interface (`interface.admin-state/<p>.<id>`, `interface-ip/<p>.<id>/…`) to `interface.subinterface`:
they depend on the alias `interface/<p>.<id>`, which is observe-only (never planned) and has no planned provider. On the fake
VPP, removing a bond that has an enabled, addressed VLAN sub-interface in one commit deletes the sub-interface first and then
fails on its admin state (`sw_interface_set_flags: Invalid sw_if_index`), rolled back. The same shape exists for a
sub-interface of any parent (P08). `TestBondingOnFake` removes the sub-interface's attributes in an earlier commit; the user
doc says so. Fix belongs to TD-11c (topo through the alias to its creator) or a `KeyProvider` on DF-1's
`interface.subinterface` (manager-owned `descriptors/interface`).

**Q5 — CLI `show bonds`.** apps/cli is not in my files; the operation table has `Bonding_bonds`
(`GET /api/v1/state/interfaces/bonds`) after `make -C apps/cli gen`, but no `show` command maps to it. The user doc gives
the REST call. A follow-up for the CLI owner?

**Q6 — base.** I started from task/W-seed@df67a8e (D-114) and did not merge main (P08 is merged there now); the merger
rebases (D-112). Hotspot hunks are only under my anchors, so the rebase should be mechanical; generated files need
`pnpm gen && make -C apps/cli gen docs` after it (rule 3).
