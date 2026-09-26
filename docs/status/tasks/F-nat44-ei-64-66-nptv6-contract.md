# F-nat44-ei-64-66-nptv6 — contract changes

Committed on `task/F-nat44-ei-64-66-nptv6` itself (envelope: no own contract branch, P08 pattern). Additive only; no
existing field renamed, renumbered or reshaped; no new RPC, no `ActionRequest` member, no `NatConfig` number.

## `contract(proto): nat session variants`

`packages/proto/vrx/v1/dataplane.proto`:

| where | what | number |
|---|---|---|
| `NatSessionsRequest` (F-nat44-ed-sessions, max 4) | `optional NatSessionVariant variant` | **5** |
| `NatSessionsResponse` (max 7) | `optional NatSessionVariant variant` (the table the page comes from) | **8** |
| `NatSessionKillAction` (max 6) | `optional NatSessionVariant variant` | **7** |
| `// ----- F-nat44-ei-64-66-nptv6 -----` section | `enum NatSessionVariant { UNSPECIFIED = 0, ED = 1, EI = 2, NAT64 = 3 }` | own |

The three fields are `optional` so the generated TS types keep them optional (`variant?:`): F-nat44-ed-sessions' API
code and fake build unchanged, and UNSPECIFIED / unset means NAT44-ED exactly as before.

Semantics: `docs/contracts/proto.md` §11 "F-nat44-ei-64-66-nptv6: NAT session variants" (EI paging and kill by the
inside endpoint; NAT64 read-only paging with the NatSession field mapping and `protocol`-only filter).

Regenerated (never hand-edited): `apps/agent/gen/vrx/v1/*`, `packages/proto/gen/ts/vrx/v1/dataplane.ts`
(`packages/proto/gen.sh`). Config contract: none (every `nat.nat64` / `nat.nat66` / `nat.nptv6` leaf and every NAT44
leaf used by `mode: "ei"` is already mirrored in `NatConfig`).
