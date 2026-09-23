# P02a — questions / notes for the manager (none blocking; work continued)

1. **New files not in the envelope's list** (all inside `packages/schema`, nothing another group would create):
   `packages/schema/vitest.config.ts` (coverage thresholds), `src/validate.ts` + `validate.test.ts` (tier a+b entry point for P06),
   `src/domains/group-a.test.ts`, `src/semantic/{system,dataplane,interfaces,vrfs,routing,management}.test.ts`. The envelope
   also lists `semantic/{system,interfaces,routing}*.ts` but the P02 prompt assigns all six group-(a) validator files to P02a —
   I followed the prompt (dataplane/vrfs/management validators are in this branch).
2. **Empty document is no longer semantically clean.** `semantic/index.test.ts` now asserts that `RootConfig.parse({})` yields
   exactly one issue (`management.admin-exists`). P02b/P02c validators must stay silent on `{}` (they should anyway — nothing
   to reference) or that test breaks at merge. Please pass this on when merging.
3. **Strict objects.** Every modelled object in group (a) is `z.strictObject` (D-a1 below). Recommend the same for groups (b)/(c);
   written into `packages/schema/README.md` and `docs/contracts/schema.md` as the convention.
4. **Coverage threshold glob** `src/semantic/**/*.ts` = 100 % applies to P02b/P02c validators too. `pnpm test:coverage` is not part
   of `tools/ci.sh`, so it cannot break the gate, but it is the P02 acceptance criterion — say so if a per-group threshold is preferred.
5. **First admin bootstrap (P06).** Because `management.admin-exists` always fires, the API's initial revision must contain one admin
   (or P06 seeds it from the install-time credentials before the first commit). `examples/minimal.json` shows the shape;
   `passwordHash` accepts `$vrx-test$VRX_TEST_HASH_<id>` placeholders in fixtures (underscore added to the pattern).
6. **Contract guard on a `task/` branch** — same as P02s #3: `contract(schema): …` commit subjects on `task/P02a`; rename to
   `contract/P02a` if wanted.
7. **BGP per-VRF instances** are not modelled (single `routing.bgp` with a `vrf` field, matching docs/04). FRR's
   `router bgp X vrf Y` would become an additive `routing.bgp.instances` (or `routing.bgpInstances`) later — flagging for P05/D3.x.
8. **`ip.test.ts` expectation changed**: `1:2:3:4:5:6:7::` is accepted (RFC 4291 allows `::` for one zero group; `z.ipv6()` and
   `inet_pton` accept it; RFC 5952 only discourages it in canonical output). The predecessor's test expected rejection.
9. **Prettier** was run on `packages/schema` (`tools/ci.sh` does not check formatting; P02s did the same). `.prettierignore` excludes `*.md`.

## Review-fix round (2026-09-24)

10. **Edits outside the envelope, forced by the merge of all three groups** (each minimal, commented "P02a merge"):
    P02b tests `semantic/{nat,acl,objects}.test.ts` (VRFs need `id`, sub-interfaces need `vlanId`, one helper test uses raw
    unparsed data, one VRF-mismatch expectation gained the now-defaulted `vrf: 'default'`), `semantic/index.test.ts` (empty
    document is clean, D-048), `domains/acl.ts` (MACIP `sourceMac`/`sourceMacMask` → `macPattern`: masks and the
    `00:00:00:00:00:00` wildcard are not unicast MACs). P03 contract tests lost their `ntp` field (D-061 allowed proto edits).
11. **P12 deviation (please confirm with P12):** `routing.policy.prefixLists.<name>` / `routeMaps.<name>` are objects
    `{ description?, family, rules[] }` / `{ description?, entries[] }`, not P12 §2's bare arrays — proto3 map values
    cannot be `repeated`, and the document is projected 1:1 (D-042/D-061). Same data, one extra key.
12. **OSPF area ids are strings** (`"0"`, `"0.0.0.51"`) in keys *and* in `interfaces.<n>.area` — a number|string union has
    no protobuf projection.
13. **P06:** use `redactSecrets()` on every outbound document and the round-trip rule in `docs/contracts/schema.md`
    (absent write-only = keep, `null` = clear; match users by `username`). `MergePatchError.pointer` → problem+json 400.
14. **Not done this round (low, additive later):** L6 (sub-interface id 0 — needs P04 confirmation of VPP behaviour),
    L7 (/32 host addresses inside another interface's subnet — needs a VPP 26.06 check), L9 (tzdata list in JSON Schema),
    L10 (reserved usernames — after P06 fixes the PAM/SSH mapping). M7 remainder (bonding, per-interface queues, bridge
    domains, neighbour DB, PBR, AS-path/community lists, per-VRF BGP instances, NUMA/isolcpus) — additive feature tasks.

## Decisions taken (to be copied to docs/decisions/LOG.md by the manager)

| date | id | decision | options considered | why | reversal cost | tasks |
|---|---|---|---|---|---|---|
| 2026-09-23 | D-a1 | Every modelled object is `z.strictObject`; records validate keys with a primitive | (a) Zod default strip (b) strict | typos (`mtuu`) surface with a pointer instead of vanishing; root was already strict | trivial | P02a/b/c, P06, P07* |
| 2026-09-23 | D-a2 | `default` VRF is implicit (VPP table 0); may be declared under `/vrfs` only with `id: 0`; other VRFs need a unique non-zero id | (a) require explicit `vrfs.default` everywhere (b) implicit + optional declaration (c) parse transform injecting it | (b): empty/minimal documents stay valid, `vrf: 'default'` defaults resolve, no transform in the schema | low | P02a, P04/P05 agent, P06 |
| 2026-09-23 | D-a3 | `interfaces.*.enabled` (and sub-interfaces) default `false` | (a) `true` (b) `false` | VPP/TNSR semantics: admin-down until enabled; safest for a firewall | trivial | P02a, P07* |
| 2026-09-23 | D-a4 | `dataplane.corelist` is `number[]` of core ids; agent renders `corelist-workers 2-5,7` | (a) VPP range string (b) int array | (b) validates trivially (unique, main core not a worker, length = workers) | trivial | P02a, P04 |
| 2026-09-23 | D-a5 | `routing.{bgp,ospf,isis,rip,bfd}` are `.optional()` objects — absent = protocol disabled | (a) always-present object with `enabled` (b) optional object | (b) `{}` stays a complete routing section; renderers key on presence | low | P02a, P05 |
| 2026-09-23 | D-a6 | `management.users[].scope` is the literal `'*'` with default `'*'` (vdom.md #3) | (a) free string + semantic rule (b) literal | (b) JSON Schema `const`, no rule to maintain; widening to an enum later is additive | trivial | P02a, P06 |
| 2026-09-23 | D-a7 | **superseded by D-048** — `management.admin-exists` (an enabled admin with password or SSH key) always fires — the empty document is schema-valid but not committable | (a) skip the rule when `users` is empty (b) always enforce | (b) is the prompt's rule and prevents locking oneself out; bootstrap seeds one admin | low | P02a, P06 |
| 2026-09-23 | D-a8 | Arrays remain diff leaves for contracts-v1 (D-021 kept); arrays of objects declare `x-vrx-ui.itemKey` | (a) keyed element-wise diff now (b) leaves + itemKey hint | (b) no schema-aware diff needed yet; UI can pair items by key; adding an option later is additive | low | P02a, P06, P07* |
| 2026-09-23 | D-a9 | `interfaces` record keys are parent names only; sub-interfaces nest under `subinterfaces` keyed by decimal sub-id, referenced elsewhere as `<parent>.<id>` | (a) flat record incl. dotted names (b) nested per docs/04 | (b) matches docs/04; parent/child relation explicit; `interfaceNames()` gives the referenceable set | low | P02a/b/c, P04 |
| 2026-09-23 | D-a10 | `descriptionText` rejects C0/C1 control characters (TAB allowed); `hostname` pattern has no lookaround; `passwordHash` allows `_` | (a) keep permissive patterns (b) tighten/portable | (b) descriptions reach CLI/log output; patterns are reused by Go/Python consumers | trivial | P02a |
| 2026-09-23 | D-a11 | Duplicate `(vrf, prefix)` static routes are an error (`routing.static-unique`), not merged | (a) merge next hops (b) error | (b) explicit ECMP via `nextHops[]`; no hidden merging in the commit path | trivial | P02a, P05 |
| 2026-09-24 | D-a12 | Prefix lists / route maps are objects with `rules[]` / `entries[]` (not P12's bare arrays) | (a) bare arrays per P12 (b) objects | (a) has no 1:1 protobuf projection | low | P02a, P12 |
| 2026-09-24 | D-a13 | Duplicate record keys that differ only in spelling (`2001:DB8::1` vs `2001:db8::1`) are a semantic error, keys are not force-canonicalised | (a) require canonical keys (b) semantic uniqueness | (b) accepts what operators type, still one FIB/FRR object | trivial | P02a |
| 2026-09-24 | D-a14 | `withUi` merges with the wrapped schema's hints, including through `.optional()/.default()` wrappers | (a) `schema.meta()` only (b) walk wrappers | fields are usually `withUi(primitive.optional(), …)`; (a) would still lose the widget there | trivial | all groups |
