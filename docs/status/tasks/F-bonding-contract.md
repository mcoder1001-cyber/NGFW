# F-bonding — contract changes (additive, for the manager's review)

Two commits on `task/F-bonding` (P08 pattern, no own branch): `contract(schema): interfaces bond …` and
`contract(proto): F-bonding …`. Nothing existing is renamed or reshaped.

## Schema (`packages/schema`)

| file | change |
|---|---|
| `src/domains/ext/bonding.ts` (new, owned) | `BondSchema` = `{ mode: lacp\|xor\|round-robin\|active-backup\|broadcast, loadBalance?: l2\|l23\|l34, members: record<memberName, {passive=false, longTimeout=false, weight?: 1–255}> = {}, numaOnly = false, id?: 0–4294967294 }`, `interfaceBondField`, `bondIdOf()`, `BOND_INTERFACE_RE`; `x-vrx-ui` group `bonding` |
| `src/domains/interfaces.ts` (C1) | one import line (top of file, `// wave-A: F-bonding`) + `bond: interfaceBondField,` under the anchor |
| `src/index.ts` (C3) | `export * from './domains/ext/bonding.js';` |
| `src/semantic/bonding.ts` (new, owned) + test | `bondingValidators`: `interfaces.bonding-name`, `-member-exists`, `-member-kind`, `-member-unique`, `-member-l3`, `-load-balance`, `-lacp-options`, `-weight` |
| `src/semantic/index.ts` (C2) | one import + one spread under the anchors |

Naming: a bond interface is keyed `BondEthernet<id>` (VPP's and TNSR's name), so key = logical name = VPP name (D-069);
`bond.id` is optional and must equal the name's number. The member record key uses the parent-interface alphabet (no
sub-interfaces). See decision F-B1 in `F-bonding-questions.md`.

## Proto (`packages/proto/vrx/v1/dataplane.proto`, numbers from wave-A-hotspots §2)

- `Interface.bond = 13` (`Bond`, message presence) — the only field number taken.
- `// ----- F-bonding -----` section: `Bond`, `BondMember` (mirrors, every scalar `optional`, D-039), `BondStateRequest`,
  `BondStateResponse`, `BondStatus`, `BondMemberStatus`, `BondLacpState`, `BondLacpPort` (all prefixed `Bond`).
- `rpc BondState(BondStateRequest) returns (BondStateResponse)` under the service anchor (framed by blank lines).
- `apps/api/src/testing/fake-agent.ts` (P5): `bondState` UNIMPLEMENTED stub under the anchor.
- `docs/contracts/proto.md` §11: `### F-bonding: BondState (+ Interface.bond 13)`.
- `packages/proto/test/fixtures/bonding-lacp-ab.json`: LACP + active-backup document (round-trip corpus).
- Regenerated: `apps/agent/gen`, `packages/proto/gen/ts`, `packages/api-client/src/generated/schema.d.ts` (`pnpm gen`);
  `make -C apps/cli gen docs` produced no change at the contract commit (no route yet).

## Evidence (at the contract commits)

```
packages/schema  vitest: Test Files 38 passed (38) · Tests 1226 passed (1226); tsc --noEmit clean; eslint clean
packages/proto   buf lint clean; buf format -d clean; vitest: Test Files 2 passed (2) · Tests 70 passed (70)
apps/agent       go test ./internal/contracttest/  → ok (TestSchemaProtoDrift: bond ↔ Interface.bond 13 mirrored)
apps/api         tsc -p tsconfig.json → clean (fake-agent DataplaneServer is exhaustive again)
```
