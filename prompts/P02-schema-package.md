# Task P02 — Configuration schema (contract)   (prepend 00-CONTEXT.md)

This prompt is executed as **four board tasks**; your TASK ENVELOPE says which one you are:
- **P02s (skeleton, 2 h, first):** `packages/schema/src/index.ts` imports `./domains/<key>.js` for all 13 root keys and exports
  `RootConfig`; one file per domain under `src/domains/` exporting `<Key>Schema` (passthrough placeholder + `// TODO(P02x)`);
  `src/primitives.ts`, `src/semantic/index.ts` (registry of validators), `src/diff.ts`, `src/merge-patch.ts` stubs with tests
  scaffolding; `gen.ts` emitting per-domain JSON Schema + OpenAPI components. Commit fast — P02a/b/c branch from it.
- **P02a — group (a):** `domains/{system,dataplane,interfaces,vrfs,routing,management}.ts` fully modelled (routing: static + the
  policy skeleton: prefix-lists/route-maps records; bgp/ospf/isis/rip/bfd as typed-but-optional objects per docs/04) + primitives +
  merge-patch + diff + semantic validators for these domains. Owns `index.ts`, `primitives.ts`, `diff.ts`, `merge-patch.ts`, `gen.ts`.
- **P02b — group (b):** `domains/{nat,objects,acl}.ts` fully modelled per docs/04 and the WBS rows D4.*, D5.* + their semantic validators.
- **P02c — group (c):** `domains/{vpn,tunnels,services,ha}.ts` fully modelled per docs/04 and WBS D6.*, D7.*, D9.* + validators.
Each task owns only its files; never edit another group's domain file — if you need a shared primitive, add it in your own
domain file and write a questions file. The manager tags `contracts-v1` when all four are merged (the human reviews afterwards).

## Goal
Define the **root configuration document** in `packages/schema` with Zod. It becomes the frozen `contracts-v1`
when the manager tags it: model it carefully, keep it minimal, make it extensible.

## Read first
`docs/04-api-datamodel.md` (config document shape), `docs/00-MASTER-PROMPT.md` §4.

## Build exactly this
1. `RootConfig` with top-level keys `system, dataplane, interfaces, vrfs, routing, nat,
   objects, acl, vpn, tunnels, services, ha, management`. Every key optional-with-default so
   an empty document is valid.
2. Model every domain in your group following `docs/04-api-datamodel.md` shapes and the WBS rows in `plan/wbs.csv`.
   The list below is group (a)'s minimum; groups (b)/(c) apply the same depth to their domains:
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
7. `docs/contracts/schema.md`: one table per modelled key (each group appends its own section), plus the rule "changing this
   package requires a `contract/<id>` branch with a `contract(schema): …` commit".

## Acceptance
- [ ] 100% branch coverage on primitives and semantic validators
- [ ] `pnpm gen` output deterministic (run twice, `git diff` empty)
- [ ] Example documents in `packages/schema/examples/{minimal,two-interfaces,invalid-*}.json` used by tests

## Out of scope
API endpoints. UI. Other groups' domain files. Multi-tenancy (deferred, D-003 — but follow the guardrails in `docs/decisions/vdom.md`: VRF first-class, names unique per domain, `scope` field on role assignments).
