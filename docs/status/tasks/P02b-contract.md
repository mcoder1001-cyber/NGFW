
## contract(proto) — schema→proto sync for group (b), applied by the manager (D-061), 2026-09-24
Reason: P03's strict drift test (`apps/agent/internal/contracttest TestStrictDecodeOfEveryDocument`) failed on `nat-basic.json` /
`nat-cgnat.json` because `vrx/v1/dataplane.proto` mirrored the first draft of this branch (`ec0ccda`).
Changes (`packages/proto/vrx/v1/dataplane.proto`, regenerated Go + TS stubs via `packages/proto/gen.sh`):
- additive: `nat.pools[].interface`, `nat.staticMappings[].external.pool`, `nat.map.interfaces[]`, `nat.cnat.snat.addresses.interface`
- rename (pre-`contracts-v1`, allowed): `nat.cnat.snat.interfaces[].side` → `table` (the only `buf breaking` hit); P03 fixture `all-domains.json` follows
- `nat.enabled` stays `optional bool` (explicit presence, D-039); consumers must call `isNat44Enabled()` (D-P02b-10)
