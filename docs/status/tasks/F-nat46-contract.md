# F-nat46 — contract changes (additive only)

`docs/status/wave-BC-numbers.md` has no F-nat46 section. Numbers verified on this branch's `dataplane.proto` (tip of
`task/F-det44-b`): `NatConfig` uses 1–24 and 27 (`pnat`); 25–26 stay reserved for F-nat44-ed-sessions → **28 `nat46`**.
No `ActionRequest`, `EventKind` or RPC (stateless: nothing to page or act on). No `wave-BC: F-nat46` anchors were seeded,
so every shared-file line sits directly below the F-det44-map-dslite-cnat line and says "unanchored".

## Schema (`contract(schema)`)
- `nat.nat46` (optional; absent by default) — `packages/schema/src/domains/ext/nat46.ts`:
  `{clientPrefix (IPv6 CIDR, default 64:ff9b::/96), interfaces[], mappings[]{name, ipv4, ipv6, mtu?}}`.
- Refinements mirroring `descriptors/nat46.Validate`: clientPrefix /96 with host bits zero; interfaces required with
  mappings, unique; names unique, ≤ 54 chars (`nat46-` + name fits the tag); IPv4/IPv6 unicast and unique; server not
  inside the client prefix; MTU 1280..65535.
- Semantic rules (`semantic/nat46.ts`): `nat.nat46-interfaces` (exist), `nat.nat46-map-overlap` (service /32 inside a
  `nat.map` domain ipv4Prefix; a `nat.map` domain named `nat46-<mapping>` — the reverse of the det44 name guard),
  `nat.nat46-map-interface` (a NAT46 interface bound map-e in `nat.map`).

## Proto (`contract(proto)`)
- `NatConfig.nat46 = 28` → `Nat46Config{client_prefix, interfaces, mappings}`, `Nat46Mapping{name, ipv4, ipv6, mtu}`.
- Regenerated Go, TS (`timestamp.ts` unchanged), `vrx-nat.yang`, api-client `schema.d.ts`; `vrx-opgen` output unchanged.
