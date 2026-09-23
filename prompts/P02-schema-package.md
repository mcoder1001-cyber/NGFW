# Task P02 — Configuration schema (contract)   (prepend 00-CONTEXT.md)

## Goal
Define the **root configuration document** in `packages/schema` with Zod. This is a frozen
contract after human approval: model it carefully, keep it minimal, make it extensible.

## Read first
`docs/04-api-datamodel.md` (config document shape), `docs/00-MASTER-PROMPT.md` §4.

## Build exactly this
1. `RootConfig` with top-level keys `system, dataplane, interfaces, vrfs, routing, nat,
   objects, acl, vpn, tunnels, services, ha, management`. Every key optional-with-default so
   an empty document is valid.
2. Fully model **only** these (others are `z.object({}).passthrough()` placeholders with a
   `// TODO(<domain>)` marker):
   - `system`: hostname (RFC 1123), timezone (IANA), banner
   - `dataplane`: workers, rxQueues, hugepagesGb, pciWhitelist[], mainCore, corelist
   - `interfaces`: record keyed by VPP interface name → enabled, description, mtu (68–9216),
     mac (optional), ipv4[] CIDR, ipv6[] CIDR, vrf, rxMode (polling|interrupt|adaptive),
     subinterfaces record (vlanId, innerVlanId?, same address fields), unnumbered?
   - `vrfs`: record name → { id: 0..2^32-1, description }
   - `routing.static`: array of { prefix, nextHops: [{address, interface?, weight}], vrf }
   - `management.users`: array of { username, role: admin|operator|readonly, passwordHash }
3. Semantic validators as **named functions** in `packages/schema/src/semantic/*.ts`
   returning `{ pointer, message }[]`: interface references an existing VRF; no overlapping
   IPv4 on the same VRF; sub-interface VLAN ids unique per parent; static route next-hop
   interface exists; at least one admin user.
4. `gen`: JSON Schema (draft 2020-12) per top-level key + root, and OpenAPI 3.1 components,
   into `dist/`. Include `x-vrx-ui` hints (widget, group, order, help) via a small helper
   so the UI form renderer can consume them.
5. Reusable primitives in `src/primitives.ts`: `ipv4Cidr`, `ipv6Cidr`, `ipAddress`, `macAddress`,
   `vppInterfaceName`, `hostname` — with tests including nasty inputs.
6. `merge-patch` (RFC 7386) helper and a structured `diff(a, b)` returning JSON-pointer
   changes `{op, pointer, from, to}` — both pure, both tested.
7. `docs/contracts/schema.md`: one table per modelled key, plus the rule "changing this
   package requires a PR labelled `contract`".

## Acceptance
- [ ] 100% branch coverage on primitives and semantic validators
- [ ] `pnpm gen` output deterministic (run twice, `git diff` empty)
- [ ] Example documents in `packages/schema/examples/{minimal,two-interfaces,invalid-*}.json` used by tests

## Out of scope
NAT, ACL, VPN, BGP, services models (placeholders only). API endpoints. UI.
