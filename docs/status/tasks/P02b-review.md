# Review — task/P02b (schema group (b): `nat`, `objects`, `acl`) — 2026-09-23

Reviewer: review agent (did not write the code). Branch `task/P02b` at `757edaa` (merge-base with main `34d5d07`; main
has since moved to `a9a264e` = P09, no schema changes). Files changed: exactly the P02b allowlist — `packages/schema/src/
domains/{nat,objects,acl}.ts` (+tests), `packages/schema/src/semantic/{nat,objects,acl}.ts` (+tests, `nat-objects-acl-examples.test.ts`),
`packages/schema/examples/{nat,acl,objects}-*.json` + `invalid-{nat,acl,objects}-*.json`, `docs/contracts/schema-nat-objects-acl.md`,
`docs/status/tasks/P02b*.md`. **No scope creep.** Contract commit present: `41689e8 contract(schema): nat, objects, acl …`.
No touch of `index.ts`, `primitives.ts`, `ui.ts`, `semantic/index.ts`, proto, binapi.

## Gate — re-run by the reviewer (worktree `/root/ngfw-wt/P02b`, HEAD `757edaa`)

```
$ tools/ci.sh --base main        (log /root/ngfw-wt/logs/P02b-review-ci.log, 1m16s)
== agent ==
CI GATE PASSED
$ pnpm --filter @ngfw/schema test | tail -4
      Tests  297 passed (297)
```

This is *better* than the evidence pasted in `docs/status/tasks/P02b.md:36-64`, which is from HEAD `41689e8` and says
"CI GATE NOT PASSED" (the pre-D-036 `index.test.ts:26` failure). The branch has since merged main with the D-036 fix
(`757edaa`), so the pasted block is stale — see M5.

## Spot-check of validators (executed against `dist/` with the example documents + reviewer probes)

| rule | input | result |
|---|---|---|
| `nat.static-mappings` | `nat-semantic-invalid-port-without-protocol.json` | `/nat/staticMappings/0/protocol` "protocol is required when ports are set" ✓ |
| `nat.static-mappings` | `external: {}` / `{ip,pool}` / unknown pool / one-sided port | `/…/0/external`, `/…/0/external` + `/…/0/external/pool`, `/…/0/external/pool`, `/…/0/external/port` ✓ |
| `acl.attachments` | `acl-semantic-invalid-vrf-mismatch.json` | `/acl/attachments/0/vrf` "interface 'Gig0/0/2' is in VRF 'customer-a', not 'default'" ✓ |
| `acl.attachments` | unknown list+zone; dup sequence; zone member in other VRF | `/acl/attachments/0/list` + `/acl/attachments/0/target/zone`; `/acl/attachments/1/sequence`; `/acl/attachments/0/vrf` ✓ |
| `objects.address-group-members` | `objects-semantic-invalid-group-cycle.json`; `constructor`/`toString` members; self-cycle; 3-cycle | `/objects/addressGroups/b/members/0` "a → b → a"; both reported as unknown (prototype-safe) ✓; `g → g`; `a → b → c → a` ✓ |
| whole document | `{}`; `nat-basic.json`; `acl-basic.json`; overlapping pools; inside-outside | `[]`, `[]`, `[]`, `/nat/pools/1/range`, `/nat/outside/0` + `/nat/outside/1` ✓ |

Pointers are RFC 6901 via `jsonPointer()`; record keys are `objectName` (no `/` or `~` possible), interface names only
appear as array indices — escaping is not an issue. Tests: `domains/*.test.ts` carry 194 negative (`bad(`) and 71
positive assertions over 33 cases incl. `24:00`, `0x50`, `10.0.0.01`, `'a'.repeat(64)`, `constructor` as a key,
`65536`, `-1`, `80-90-100`, datetime without offset; `semantic/*.test.ts` has 53 exact `toEqual([{pointer,…}])`
assertions; `nat-objects-acl-examples.test.ts` asserts the exact pointer list of every `*-semantic-invalid-*` fixture.
Hostile-input tests are real and assert.

---

## Findings (ranked)

### HIGH

**H1 — `x-vrx-ui` widget/help hints are silently dropped on most leaf fields of all three domains.**
`packages/schema/src/ui.ts:31-41` (`withUi`) always writes `[X_VRX_UI]: hints`; Zod 4's registry merges a clone's
meta with its parent's *shallowly*, so a second `withUi()` over an already-hinted schema replaces the whole
`x-vrx-ui` object. P02b re-wraps hinted primitives everywhere — `nat.ts:587-597` (`inside`/`outside`/`outputFeature`
over `interfaceList`), `nat.ts:141-142` (`local.ip`/`local.port`), `nat.ts:148-160` (`external.*`), `nat.ts:366-367`
(det44 prefixes), `acl.ts:135-139` (`sourceMac`, `sourceMacMask`), `acl.ts:214-217` (`sequence`), `objects.ts:112,122-123`
(`address`, `start`, `end`), … Verified in `dist/json-schema/*.json` after the gate's `gen`:

```
nat.properties.inside                         x-vrx-ui {"group":"General","order":3}     ← widget interface-picker + help lost
nat.…staticMappings.items.external.interface  x-vrx-ui {"help":"Use this interface’s address"}  ← widget lost
nat.…staticMappings.items.local.ip            x-vrx-ui {}                                  ← widget ip lost
nat.properties.forwarding                     x-vrx-ui {"group":"General"}                 ← help lost
nat.…det44.mappings.items.inside              x-vrx-ui {}                                  ← widget cidr lost
acl.…macip.rules.items.sourceMac              x-vrx-ui {}                                  ← widget mac lost
objects.…addresses(host).address              x-vrx-ui {}                                  ← widget ip lost
```
Failure scenario: the SchemaForm renderer (D-019 is the *only* plumbing for hints) shows plain text boxes for
interface/IP/MAC/CIDR fields on the NAT, ACL and Objects screens; `docs/contracts/schema-nat-objects-acl.md:14`
("Every field goes through withUi() → title + x-vrx-ui {widget, group, order, help}") is false. The existing test
(`domains.test.ts` "carries a title and x-vrx-ui hints") only checks the domain root, so this slipped through.
Fix (choose one, both cheap): (a) root cause — make `withUi` merge: `const parent = schema.meta()?.[X_VRX_UI] ?? {};
[X_VRX_UI]: { ...parent, ...hints }` in `ui.ts` (P02a/P02s-owned → questions item; manager one-liner like D-036 is
appropriate since it affects every group); or (b) in own files pass the widget explicitly at every re-wrap.
Either way add a leaf-level assertion in `domains/nat.test.ts` / `acl.test.ts` (e.g. `nat.properties.inside['x-vrx-ui'].widget
=== 'interface-picker'`, `…sourceMac…widget === 'mac'`) so it cannot regress. Metadata only — not a shape change, so it
does not need to block the merge, but it must land before the `contracts-v1` tag.

### MEDIUM — shape gaps that become PENDING reshapes (decision-policy #1) once `contracts-v1` is tagged; fix now

**M1 — `nat.pools[]` cannot express "use the interface's address as the NAT address".** `nat.ts:116-125`
`NatPoolSchema.range` is required in a `strictObject`; VPP has `nat44_add_del_interface_addr {sw_if_index, flags}`
(and `nat64_add_del_interface_addr`) — the DHCP-assigned-WAN case, TNSR's default outbound NAT. Probe
`{pools:[{name:"wan",interface:"Gig0/0/0"}]}` → schema-rejected. Adding it later means relaxing `range` to optional
(= reshaping a required field → PENDING). Fix now: `range?` + `interface?` (+ exactly-one rule in `nat.pools-valid`,
same pattern as `external`), or a `source: {kind:'range',range}|{kind:'interface',interface}` union. The F-nat44
shape `pools:[{name, range, vrf?}]` stays valid either way.

**M2 — MAP cannot be bound to an interface, and `mode` is not tied to what VPP actually uses.** `nat.ts:492-498`
`MapSchema` has `domains[]` + `parameters` only; VPP 26.06 enables MAP per interface with
`map_if_enable_disable {sw_if_index, is_enable, is_translation}` (binapi confirmed) — without it no packet is ever
processed, so D4.5 MAP-E/MAP-T/lw4o6 is not deployable from this schema. Probe `{map:{interfaces:[…]}}` →
schema-rejected. Also `map_add_domain` has **no mode field** (`ip6_prefix, ip4_prefix, ip6_src, ea_bits_len, psid_offset,
psid_length, mtu, tag`): the renderer must derive encapsulation vs translation from `ipv6Source` length (/128 → MAP-E/lw4o6,
shorter → MAP-T; verify in map.c) yet `nat.map-valid` (`semantic/nat.ts:504-553`) never relates `mode` to `ipv6Source`
— `mode:"map-t"` with `ipv6Source: …/128` passes (probe E1). Fix: add `map.interfaces[] { interface, translation }`
(or per-domain) + `nat.interfaces-exist` coverage; add to `nat.map-valid`: `map-e|lw4o6 ⇒ ipv6Source /128`,
`map-t ⇒ length < 128`.

**M3 — `cnat.snat.interfaces[].side: inside|outside` does not map to VPP.** `nat.ts:547-558`. The message is
`cnat_snat_policy_add_del_if {sw_if_index, is_add, table}` with `table ∈ {CNAT_POLICY_INCLUDE_V4, INCLUDE_V6, POD, HOST}`
(binapi confirmed); "inside/outside" has no counterpart, so the F-cnat renderer would have to invent a mapping. Also
`cnat.snat.addresses` (`nat.ts:538-546`) lacks the `sw_if_index` form of `cnat_set_snat_addresses`, and
`CnatEndpoint {addr | sw_if_index + if_af, port}` interface-addressed VIPs/backends are not expressible (low). Fix now:
`table: include-v4|include-v6|pod|host` (enum change later = reshape), optional `addresses.interface`.

**M4 — Prefix fields that VPP treats as *networks* accept host bits and nothing rejects them.** `ipv4Cidr`/`ipv6Cidr`
are `z.cidrv4/6()` (host bits allowed by design for interface addresses). Used as network prefixes at `nat.ts:257`
(NAT64 `prefixes[].prefix`), `:337-338` (NPTv6 `internal`/`external`), `:366-370` (DET44 `inside`/`outside`),
`:418-420` (MAP `ipv4Prefix`/`ipv6Prefix`/`ipv6Source`), `:559` (CNAT `excludePrefixes`). Probes `det44 inside
"100.64.0.1/16"` and `nat64 prefix "64:ff9b::1/96"` pass both tiers; the renderer would hand host bits to
`det44_add_del_map` / `nat64_add_del_prefix` / `npt66_binding_add_del`, where the deterministic/prefix arithmetic
silently differs from what the operator typed. Fix (additive): one rule `nat.prefixes-are-networks` using the
existing `prefixToRange()` (`start` must equal the address), or switch to P02a's `ipv4Network`/`ipv6Network` after merge.

**M5 — Status evidence is stale (REVIEW-PROMPT #11).** `docs/status/tasks/P02b.md:36-64` pastes a failing gate from
`41689e8` and states "CI GATE NOT PASSED"; the branch HEAD `757edaa` includes the D-036 merge and the gate passes
(reviewer run above). Fix: re-run `tools/ci.sh --base main` at HEAD and replace the block; drop the "not passed"
paragraph and the now-resolved question #1.

### LOW

**L1 — D-P02b-2 `external.pool` = "pool's first address" is a renderer convention with no VPP counterpart.**
`nat44_add_del_static_mapping_v2` takes `external_ip_address | external_sw_if_index`; its `match_pool`/`pool_ip_address`
select the *twice-NAT* pool address, not the external side. Consequences: changing a pool's start silently moves the
mapping's external address; `externalKey()` (`semantic/nat.ts:130-136`) does not unify `pool:p` with the literal
first address (probe: `{ip:203.0.113.1,port:80}` + `{pool:p(203.0.113.1),port:80}` → `[]`), nor with `interface`.
Not a breaking risk (F-nat44 asked for `ip|pool`), but: resolve the pool in `externalKey`, and either restrict
`pool` references to single-address pools or say "start address" in the help. `twiceNat` mappings also cannot
choose a twice-NAT pool address (`pool_ip_address`) — additive later.

**L2 — D-P02b-3 shared `addresses`∩`addressGroups` / `services`∩`serviceGroups` namespace.** Conventional (FortiGate,
PAN do the same) and within vdom.md #2 (uniqueness stays inside the `objects` domain). Cost: a cross-record invariant
enforced only at tier (b) — `/config/objects/addresses/x` and `/config/objects/addressGroups/x` are both valid PATCH
targets until commit — and the alternative (`kind: address|addressGroup` in `AddressMatch`) would have needed no rule.
Changing later is a reshape, so decide consciously now; I would keep it and make the D5.4 picker enforce it.

**L3 — D-P02b-4 implied `default` VRF: consistent, no action.** P02a's `domains/vrfs.ts` has `DEFAULT_VRF = 'default'`,
`vrfExists(vrfs, name)` with the same implication and rule `vrfs.default-is-table-zero`; nothing conflicts, and
VRF 0 is *named* everywhere (guardrail #1 satisfied). Two `vrfExists` helpers with different signatures will
coexist (P02b's in `semantic/objects.ts:134` is not re-exported → no TS2308); dedupe in the follow-up (questions #3).

**L4 — `nat.inside-outside-disjoint` forbids one interface on both sides** (`semantic/nat.ts:182-210`); VPP allows
`NAT_IS_INSIDE|NAT_IS_OUTSIDE` on one interface (one-armed NAT). Done per F-nat44 §1, so acceptable; note it in
the contract doc and add an additive `both[]` later if needed.

**L5 — `nat.pools-valid` global overlap is slightly stricter than VPP.** VPP keeps `addresses` and
`twice_nat_addresses` as separate lists, so a twice-NAT pool may repeat a normal pool's address; the rule rejects it
(probe E5). Scope the overlap check by `twiceNat`.

**L6 — `enabled: false` with interfaces/pools/mappings configured passes silently** (probe E4). The F-nat44 contract
has no `enabled`; a document written verbatim to it renders nothing. Add `nat.enabled-consistency` (issue when
`enabled=false` and any NAT44 object is present) or infer `enabled`; settle before F-nat44 starts.

**L7 — `acl.attachments` duplicate checks key on the literal target** (`semantic/acl.ts:277-294`): the same list
attached via a zone and via one of its member interfaces, or two lists with the same `sequence` via zone+interface,
are not caught (probe E11) and surface as an apply-time VPP error/ordering ambiguity. Expand zones before the checks.

**L8 — Not modelled (all additive later; list for the F-nat44/DF workers):** `nat_set_mss_clamping`;
`nat44_ed_add_del_vrf_table` / `_vrf_route` (per-VRF NAT44-ED tables — relevant to the VRF guardrail);
`nat44_set_session_limit.vrf_id` (per-VRF limit; today `sessionLimit` maps to `nat44_ed_plugin_enable_disable.sessions`);
ACL `acl_stats_intf_counters_enable` (hit counters on/off) and `acl_interface_set_etype_whitelist`; NAT44-EI HA
(`nat_ha_*`) and address/port allocation algorithm; `dslite_set_aftr_addr` sets both ip4+ip6 while `aftr.ipv4` is optional
(renderer must default); VPP's plugin is `npt66` where the schema says `nptv6` (cosmetic).

**L9 — `AddressGroupSchema.members.min(1)` / `ServiceGroupSchema.members.min(1)`** (`objects.ts:142, 229`): a group
cannot exist empty, so "create group, then add members" UI flows must batch; consider `.min(0)`.

**L10 — FYI outside P02b's files (for the manager at merge, extends D-037):** besides `secretRef`, `DnsSchema`,
`NtpSchema` and `NtpServerSchema` are exported by both P02a `domains/system.ts` and P02c `domains/services.ts`
(`export *` in `index.ts` → TS2308). P02b introduces no collision (checked all exported names across the three branches).

---

## Checklist verdicts

| # | item | verdict |
|---|---|---|
| 1 | WBS D4.1–D4.7 / D5.1–D5.6 expressible | Mostly. NAT44-ED/EI (pools, 1:1, port-forward, identity, LB, twice-NAT, output-feature, timeouts, session limit), NAT64/66, NPTv6, DET44+IPFIX, DS-Lite, CNAT, objects (addresses/groups/FQDN/services/groups/schedules/zones/tags), ACL L3/L4 stateless+`reflect`, MACIP, per-interface/zone in/out attachments with `sequence`, host ACL — all present. Gaps: interface-address pools (M1), MAP interface binding + mode (M2), CNAT SNAT table (M3), host bits (M4), L8 list. Sessions/kill-session/hit counters are correctly left to `/state` and actions. |
| 2 | F-nat44 §Contract shape | Exact: `mode ed`, `inside[]`, `outside[]`, `pools[{name,range,vrf?}]`, `staticMappings[{name, local{ip,port?}, external{ip|pool,port?}, protocol?, vrf?, twiceNat}]`, `timeouts{udp,tcpEstablished,tcpTransitory,icmp}`, `sessionLimit ≥ 1024`; additive extras (`enabled`, `external.interface`, flags). Its four semantic rules exist. Caveat L6 (`enabled`). |
| 3 | vdom guardrails | ✓ `vrf` first-class on pools, static/identity/LB mappings, NAT64 prefixes/pools/BIBs, NAT66 mappings, DET44 sides, ACL + MACIP attachments (hard VRF-match, D-P02b-7 is the right call); names unique per record; API paths `/config/<domain>/<kind>/<name>`; `default` named, never numeric. |
| 4 | secrets | ✓ none in `nat`/`objects`/`acl`; fixtures clean. |
| 5 | validators `{pointer,message}` | ✓ 30 rules, correct pointers (table above); `domains[]` declared correctly for the commit engine's skip logic. |
| 6 | hostile-input tests | ✓ real and asserting (counts above). Branch coverage not measured (tooling on P02a; questions #7) — run after merge. |
| 7 | `x-vrx-ui` | Present at domain/group level and on unwrapped primitives; **lost on re-wrapped leaf fields** (H1). |
| 8 | D-P02b-1…8 | 1 ✓ (NAT44 at top level, siblings — matches docs/04 and F-nat44); 2 L1; 3 L2; 4 ✓ L3; 5 ✓ L5; 6 ✓; 7 ✓; 8 ✓ (wrap-around is additive later). None forces a *breaking* change; M1–M3 are the shapes that would. |

## Required changes

Before merge: **M5** (fresh gate paste at HEAD). Before the `contracts-v1` tag (short `contract/P02b-fix` or same
worker): **H1** (fix `withUi` merge or explicit widgets + leaf test), **M1**, **M2**, **M3**, **M4**. Lows → follow-up
task or the F-nat44 contract branch.

**APPROVE WITH CHANGES**
