# P02b — questions / notes for the manager (none blocking; work continued)

1. **RESOLVED (D-036, fix on main, merged into this branch at 757edaa).** ~~`packages/schema/src/index.test.ts:26` (P02a-owned) fails on every group's branch.~~ It asserts
   `RootConfig.parse({})[key]` deep-equals `{}` for all 13 keys — true only for the P02s `looseObject({})`
   placeholders. D-017 chose `.prefault({})` precisely so nested field defaults are filled, and P02a's `system`
   (`ntp.enabled`, `servers`, `vrf`), `dataplane` (`pciWhitelist`) and P02c's `vpn` use `.default(...)` just like
   `nat`/`objects`/`acl` do (checked read-only via `git show task/P02a:…`, `task/P02c:…`). So the assertion must be
   relaxed by its owner (P02a), e.g. `expect(r[key]).toEqual(DOMAINS[key].parse({}))` or
   `expect(DOMAINS[key].safeParse(r[key]).success).toBe(true)`. Until then `tools/ci.sh` fails at `test` on
   `task/P02b` with exactly this one assertion (296/297 pass); everything else in the gate is green (see P02b.md).
   Proposed manager action at merge: merge P02a first (with the fixed test) or apply the one-line fix on main.
2. **Export-name collisions across groups** (index.ts re-exports every module with `export *`; two modules exporting
   the same name is TS2308 and breaks `pnpm typecheck` after merge). Found and avoided on my side: P02a exports
   `ipv4Address`, `ipv6Address` (mine are now module-private in `nat.ts`); P02c exports `l4Port` (mine is renamed
   `l4PortNumber`). Still colliding **between P02a and P02c** (not my files): both export `secretRef`
   (`primitives.ts` vs `domains/vpn.ts`). Please flag to P02a/P02c before merging both.
3. **Local primitives that belong in `primitives.ts` (P02a) once contracts-v1 is tagged** — additive follow-up:
   `ipPrefix` (= P02a's `ipCidr`), `l4PortNumber` (= P02a's `portNumber`), `l4PortRange`, `timeOfDay`, `hexColor`,
   `ipv4AddressRange` + `splitIpv4Range`, `ruleSequence`, `linuxInterfaceName`. Also the cross-domain lookup helpers
   in `semantic/objects.ts` (`interfaceExists`, `interfaceVrf`, `vrfExists`, `prefixToRange`, `rangesOverlap`,
   `ipv6ToBigInt` …) are generic and could move to a shared `semantic/refs.ts` owned by P02a — P02c's validators
   (IPsec/tunnel interfaces, VRFs) will need the same lookups.
4. **`default` VRF is implied** in `vrfExists()` (`name === 'default' || name in vrfs`). P02a's TODO says "the
   `default` VRF present" will be a `vrfs.*` rule — that is compatible (their rule complains once, mine never
   duplicates the complaint). If P02a instead requires *every* reference to resolve to a key, drop the shortcut (one
   line in `semantic/objects.ts`, tests in `objects.test.ts` "vrfExists implies default").
5. **Sub-interface resolution** assumes the docs/04 / two-interfaces.json shape (`interfaces[parent].subinterfaces[N]`
   → VPP name `parent.N`) and also accepts the full VPP name as its own key. If P02a keys sub-interfaces differently,
   only `lookupInterface()` in `semantic/objects.ts` changes.
6. **F-nat44 `external{ip|pool}`**: modelled as `external { ip? | pool? | interface? , port? }` with an exactly-one
   semantic rule; `pool` = name of an entry in `nat.pools`, renderer uses the pool's first address (VPP static
   mappings take one address; `nat44_add_del_static_mapping_v2` also has `external_sw_if_index`, hence `interface`).
   If the F-nat44 worker prefers "pool = any address of the pool" semantics, that is a renderer decision, not a
   schema change.
7. **Coverage**: `@vitest/coverage-v8` is only on P02a's branch (D-024), so the "100 % branch coverage" acceptance
   could not be measured here. Every branch of the validators has a dedicated test (see the `describe` blocks per
   rule); please run `pnpm test:coverage` on main after P02a merges and open a follow-up if anything in
   `semantic/{nat,objects,acl}.ts` is below 100 %.
8. **Gate note**: `tools/ci.sh --base main` requires a `contract…` commit subject on the branch; this branch carries
   `contract(schema): …` on `task/P02b` (same approach P02s used, see P02s-questions #3).

9. **H1 root cause is still in `ui.ts` (P02a-owned).** This branch no longer depends on it (private merging
   `withUi` in the three domain files), but other groups' re-wrapped leaves keep losing hints until `ui.ts` merges
   `x-vrx-ui` itself (D-043: manager fix after P02a merges). Suggested one-liner: in `withUi`,
   `[X_VRX_UI]: { ...(schema.meta()?.[X_VRX_UI] ?? {}), ...hints }`. After that my private helper can become a plain
   re-export (not required — it is idempotent with the fixed one).
10. **P03b proto sync**: `nat.enabled` is now optional without default (D-P02b-10) — `optional bool enabled`
    (explicit presence, D-039) already matches; the renderer must call `isNat44Enabled()` rather than read the field.

## Decisions taken (to be copied to docs/decisions/LOG.md by the manager)

| date | id | decision | options considered | why | reversal cost | tasks |
|---|---|---|---|---|---|---|
| 2026-09-23 | D-P02b-1 | `nat` top level **is** NAT44 (`mode, inside, outside, pools, staticMappings, identityMappings, loadBalancedMappings, timeouts, sessionLimit`); other translators are siblings `nat64, nat66, nptv6, det44, dslite, map, cnat, ipfix`; docs/04 `static`+`portForwards` are one `staticMappings` list (no ports = 1:1) | (a) `nat.nat44.{…}` nesting (b) NAT44 at top level + siblings | (b) matches docs/04 and the F-nat44 contract verbatim; one mapping list mirrors VPP's own static-mapping model | medium (rename = PENDING) | P02b, F-nat44, DF-* |
| 2026-09-23 | D-P02b-2 | `staticMappings[].external` = exactly one of `ip` / `pool` (name in `nat.pools`, first address) / `interface`, enforced semantically | (a) `ip` only (b) `ip \| pool` (c) `ip \| pool \| interface` | F-nat44 asks for `ip\|pool`; `interface` is what VPP's `external_sw_if_index` offers for DHCP WANs; optional fields keep JSON Schema simple | low | P02b, F-nat44 |
| 2026-09-23 | D-P02b-3 | Names are unique per record; additionally `addresses`∩`addressGroups` = ∅ and `services`∩`serviceGroups` = ∅ (`objects.names-disjoint`) because ACL rules reference either kind through one `name` | (a) separate `kind: address\|group` in the ACL match (b) shared namespace per family | (b) keeps the rule editor to one picker and stays within vdom.md #2 | low | P02b, D5.4 UI |
| 2026-09-23 | D-P02b-4 | `default` VRF is implied by `vrfExists()`; all other VRF names must be keys of `vrfs` | (a) require `default` in `vrfs` for every reference (b) imply it | (b) an empty document stays valid with `vrf: "default"`; P02a may add "default present" once in `vrfs.*` | trivial | P02b, P02a |
| 2026-09-23 | D-P02b-5 | Pool overlap is checked across all `nat.pools` regardless of `vrf` (VPP keeps one NAT44 address list) | (a) per-VRF (b) global | (b) matches `nat44_add_del_address_range` behaviour (address already present fails) | trivial | P02b, F-nat44 |
| 2026-09-23 | D-P02b-6 | Semantic validators live in three files with one shared helper set in `semantic/objects.ts` (`interfaceExists`, `vrfExists`, IP arithmetic); `acl` imports `duplicates()` from `semantic/nat.ts` | (a) a new `semantic/refs.ts` (not in my file allowlist) (b) helpers inside owned files | (b) respects the envelope; move to `refs.ts` is a mechanical follow-up (question 3) | trivial | P02b, P02a |
| 2026-09-23 | D-P02b-7 | ACL `attachments[].vrf` must equal the VRF declared by the target interface(s); a zone target checks every member | (a) informational only (b) hard rule | (b) is the vdom.md #1 guardrail: VRF is first-class, a mismatch is always a mistake | trivial | P02b, D5.2 |
| 2026-09-23 | D-P02b-8 | `objects.schedules.recurring` windows are same-day (`start < end`); overnight = two schedules | (a) allow wrap-around (b) two schedules | (b) renderers/UI stay unambiguous; wrap-around can be added later without breaking documents | trivial | P02b |
| 2026-09-24 | D-P02b-9 | Review H1 fixed inside the owned files: a private `withUi` in `domains/{nat,acl,objects}.ts` merges `x-vrx-ui` with the wrapped schema's hints; leaf tests + a walker (every format-carrying leaf with hints has a `widget`) | (a) wait for the `ui.ts` fix on main (b) explicit widget at every re-wrap (c) private merging helper | (c) one place per file, no dependence on merge order, idempotent with the later `ui.ts` fix | trivial | P02b, P02a |
| 2026-09-24 | D-P02b-10 | `nat.enabled` optional, no default; `isNat44Enabled()` = explicit value, else "any NAT44 object configured" (review L6) | (a) error rule `nat.enabled-consistency` (b) infer when absent (c) leave as is | (b) documents written to the F-nat44 contract work verbatim, and a disabled-but-staged config stays legal (an error rule would forbid it) | low | P02b, F-nat44, P03b |
| 2026-09-24 | D-P02b-11 | Address/service groups may be empty (`members` default `[]`); `acl.rule-references` rejects a rule that references a group with no members (transitively) (review L9) | (a) keep `min(1)` (b) allow empty everywhere (c) allow empty groups, forbid using them | (c) create-then-fill UI flows work, and a deny rule over an empty group can never silently vanish | trivial | P02b, D5.4 |
| 2026-09-24 | D-P02b-12 | Review L2: keep the shared `addresses`∩`addressGroups` / `services`∩`serviceGroups` namespace (D-P02b-3) — reviewer concurs; the D5.4 picker enforces it on create | (a) `kind` discriminator in `AddressMatch` (b) keep | changing later is a reshape; FortiGate/PAN convention; tier (b) rule already exists | low | P02b, D5.4 |
