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
`descriptors/bond/` (see F-bonding.md for the unit test). The envelope predates D-125, so it does not name this obligation;
please confirm it satisfies "a KeyProvider obligation" for the merge order.

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
