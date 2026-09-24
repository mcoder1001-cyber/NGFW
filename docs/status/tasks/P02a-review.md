# Review — task/P02a (schema group (a): system, dataplane, interfaces, vrfs, routing, management + primitives/diff/merge-patch/gen)

Reviewer: review agent (did not write this code). Branch `task/P02a` @ `df554dc`, worktree `/root/ngfw-wt/P02a`, base `main@2de6c2f`.
Reviewed 2026-09-23 against prompts/P02-schema-package.md (group (a)), docs/04-api-datamodel.md, docs/decisions/vdom.md,
LOG D-017…D-021/D-024/D-036/D-037, WBS D0.6/D0.8/D0.9/D1.2–D1.5/D2.1–D2.3/D3.3–D3.10, prompts/P12-frr-linuxcp.md §2.

## What I verified myself (host `ngfw`)

| check | result |
|---|---|
| `tools/ci.sh --base main` in the worktree (log `/root/ngfw-wt/logs/P02a-review-ci.log`) | `CI GATE PASSED` — matches P02a.md |
| `pnpm --filter @ngfw/schema test` | 20 files / 664 tests passed — matches |
| `pnpm --filter @ngfw/schema test:coverage` | 100 % stmts/branches/funcs/lines on primitives, ip, semantic/**, pointer, json, diff, merge-patch, ui, validate — matches |
| `pnpm gen` twice with `dist/` removed in between, sha256 of every output | 15 files identical |
| ownership (`git diff --name-only main...task/P02a`) | every path inside the allowed set; `semantic/{dataplane,vrfs,management}*.ts` beyond the envelope list are assigned to P02a by the P02 prompt (questions #1) — accepted; no scope creep |
| contract commits | `contract(schema): …` subjects present (`0f27fb8`, `b5a05a6`); no `P02a-contract.md` (questions #6 — manager's call, the P02 prompt defines this task as the contract) |
| validators → pointers (ran `validateConfig` on the five `invalid-semantic-*.json`) | `/interfaces/TenGigabitEthernet0~10~10/vrf`, `/interfaces/TenGigabitEthernet0~10~10/subinterfaces/101/vlanId`, `/routing/static/0/nextHops/0/interface`, `/interfaces/TenGigabitEthernet0~10~11/ipv4/0`, `/management/users` — all correct, `/` escaped as `~1` |
| diff / merge-patch on tricky inputs | RFC 7386 Appendix A: all 15 cases in `merge-patch.test.ts:5-25`; nested `null` (remove vs replace), `{}`→`[]`, object→`null`, arrays as one `replace`, `undefined` vs `null` all behave as documented |
| `x-vrx-ui` in `dist/` | present on fields (`interfaces` `propertyNames`, `mtu`, …); `passwordHash` carries `writeOnly: true` + `x-vrx-ui.secret`; arrays carry `itemKey`; root has no `required` |
| secrets | no inline secret material; RADIUS/TACACS/TLS/BGP-MD5 are `secretRef`s; fixtures use `$vrx-test$VRX_TEST_HASH_<id>`; write-only semantics documented (schema.md:38-39, management.ts:19-20) |
| vdom guardrails | VRF first-class on (sub-)interfaces, static routes, every protocol instance, NTP/DNS/AAA/syslog; names unique per record; `users[].scope` literal `'*'` (D-a6); domain `x-vrx-ui.order` drives navigation (#4) |
| `semantic/index.test.ts:73-80` "exactly one issue on `{}`" vs P02b/P02c validators | holds: both siblings' own CI asserts `[]` on `{}` (P02b/P02c `semantic/index.test.ts:74`), so their validators are silent on the empty document; P02a's file replaces theirs without a textual conflict |

The work is solid: strict objects with explicit defaults, portable patterns, pointer discipline, exhaustive nasty-input tests,
deterministic gen. The findings below are about what happens **when this branch meets its siblings and contracts-v1 is tagged**.

## Findings (ranked)

### HIGH

**H1 — Merging P02a turns `pnpm test` red on main: `management.admin-exists` (D-a7 option b) fires on every sibling fixture, and P02a's `examples.test.ts` claims every file in `examples/`.**
- `semantic/management.ts:12-29` fires unless an enabled admin with a password/SSH key exists — so `RootConfig.parse({})` is never semantically clean.
- `examples.test.ts:40-59` iterates *all* `examples/*.json`; every file not starting with `invalid-` must be "accepted, semantically clean". P02b ships `nat-basic.json`, `nat-cgnat.json`, `objects-basic.json`, `acl-basic.json` and `{nat,objects,acl}-semantic-invalid-*.json`; P02c ships `ha-vrrp.json`, `services-*.json`, `tunnels-*.json`, `vpn-*.json` and `*-semantic-*.json`. Only one of the 35 non-shared sibling fixtures (`vpn-remote-access.json`) contains `management.users`, and none of the `*-semantic-invalid-*` files start with `invalid-`.
- I ran P02a's built package over both siblings' `examples/`: **all 35 fixtures report `/management/users`** (`/root/ngfw-wt/logs/P02a-review-xfix.mjs`). Under P02a's `examples.test.ts` the 20 `*-basic` / `*-semantic-*` files fail ("must be clean"); P02b's `semantic/nat-objects-acl-examples.test.ts:42-52` asserts exact pointer lists / `[]` for 11 fixtures — each gains an extra `/management/users` and fails. Roughly 30 red tests on the merged tree, regardless of merge order.
- Root cause is a convention conflict: `docs/contracts/schema.md:27-28` promises "Empty is valid … `RootConfig.parse({})` succeeds", both siblings built on "`{}` is clean" (their `semantic/index.test.ts:74`), while D-a7(b) makes every document without an admin uncommittable — including every fixture and every test document P06/P07 will ever build.
- Fix (both parts, P02a-owned files):
  1. `examples.test.ts`: scope the loop to group-(a) fixtures (P02b's approach: filter by own prefixes) **or** treat any `*semantic*` file as a semantic fixture owned by another group and only assert `RootConfig.safeParse` success for files it does not own.
  2. Decide D-a7 explicitly for the merge. Recommended: option (a)-variant — fire only when `management.users` is non-empty and no usable admin remains (protects against disabling/demoting the last admin); bootstrap ("first revision must contain an admin") is enforced by P06 at install time, where it belongs (docs/04 keeps `app_user` as the auth store anyway, see M4). If (b) is kept, the manager must add an admin to ~20 sibling fixtures and relax P02b's exact-pointer expectations at merge, and schema.md:27-28 must be reworded. Log the outcome in `docs/decisions/LOG.md`.

**H2 — `export *` name clash with P02c: `DnsSchema`, `NtpSchema`, `NtpServerSchema` are exported by both `domains/system.ts:51,37,27` (P02a) and `domains/services.ts:526,841,817` (P02c, branch `task/P02c`).**
- `index.ts:57-69` star-exports all 13 domain files; two star-exports of the same name is TS2308 ("has already exported a member named …") → `pnpm typecheck` and `build` fail on the merged tree. Same class as D-037 (`secretRef`, which P02c already renamed to `secretReference`), but this pair is not logged anywhere. Scan (`LC_ALL=C comm` over owned files): P02a∩P02b = ∅, P02a∩P02c = {DnsSchema, NtpSchema, NtpServerSchema}, P02b∩P02c = ∅.
- Fix: rename in `domains/system.ts:27,37,51` to `SystemNtpServerSchema` / `SystemNtpSchema` / `SystemDnsSchema` (TS identifiers only — JSON shape, JSON Schema and OpenAPI output unchanged), or make `index.ts` export explicitly. Run `pnpm typecheck` on a merged tree before tagging.

**H3 — Textual merge conflict on `packages/schema/src/index.test.ts` against main (D-036).**
- `git merge-tree --write-tree main task/P02a` → `CONFLICT (content): packages/schema/src/index.test.ts`. The manager's D-036 one-liner (`toBeTypeOf('object')`, `toMatchObject`) and P02a's rewrite (`index.test.ts:23-45`, asserts concrete nested defaults) touch the same hunks. P02a's version is a superset — take it.
- Consequence: the `CI GATE PASSED` pasted in P02a.md was produced on a tree without main's D-036 commit; re-run `tools/ci.sh --base main` on the merged tree (this also catches H2).

**H4 — Decide before tagging: BGP/policy shapes differ from what P12 §2 says its worker expects; one of them is a reshape after the tag.**
- `routing.ts:369-373` `bgp.neighbors` is an **array** of `{ address, … }` (itemKey `address`); P12 §2 expects a **record** `neighbors{ ip → … }`. Everything else keyed by name in this package is a record (`peerGroups`, `prefixLists`, `routeMaps`, `interfaces`, `vrfs`); the array also has no uniqueness rule (M2) and forces index-based PATCH pointers and whole-array diffs (D-a8) for the one list that P12 will edit most.
- Also divergent (all additive/renamable *now*, PENDING after the tag): `routing.prefixLists`/`routing.routeMaps` (`routing.ts:586-595`) vs P12 `routing.policy.{prefixLists,routeMaps}`; `passwordRef` vs `password(secretRef)`; flat `ipv4Unicast`/`ipv6Unicast` vs `afi{…}` with `softReconfig`; `redistribute[]` of `{protocol, metric, routeMap}` vs `redistribute{connected,static}`; `set.localPreference`/`set.metric` vs `localPref`/`med`.
- Fix: either convert `neighbors` to `z.record(ipAddress, BgpNeighborSchema)` now (free uniqueness, stable pointers `/routing/bgp/neighbors/10.0.0.1`, consistent with the rest), or log a decision that P12 follows P02a's shapes (adding `softReconfig` later is additive). Not deciding is the only wrong option.

### MEDIUM

**M1 — `routeMaps.*.entries[].match.asPath` accepts control characters → FRR config-line injection.** `routing.ts:216-220` is `z.string().max(255)`; probe `"^65000_" + LF + "no router bgp" + CR + "!"` → ACCEPTED. P12 renders this verbatim into `frr.conf` (`bgp as-path access-list … permit <regex>`), so an operator-role API user can append arbitrary FRR statements. D-a10 applied the no-control-chars rule to descriptions for exactly this reason but not here. Fix: reuse `NO_CONTROL_CHARS` and restrict to the FRR regex alphabet (`^[0-9_.*+?^$()|\[\]\- ]+$`). Same hardening for `system.banner.*` (`system.ts:12`: multi-line is intended, but BEL/ESC/C1 are accepted — allow printable + LF/TAB only).

**M2 — Arrays carry `itemKey` but nothing enforces the key is unique.** Probe: two `bgp.neighbors` with the same `address`, two `ospf.interfaces` for `loop0`, two identical `bfd.sessions`, two `redistribute: static`, two identical `syslog` entries — all ACCEPTED. Consequences: FRR gets duplicate `neighbor` stanzas (last wins silently), VPP `bfd_udp_add` fails on the second session at apply time (tier c) instead of tier b, UI pairing by `itemKey` is ambiguous. `routing.static-unique` and `management.username-unique` show the pattern; generalise it: one `uniqueBy(items, itemKey)` helper applied to `bgp.neighbors[address]`, `{ospf,isis,rip}.interfaces[name]`, `bfd.sessions[interface,peerAddress]`, `*.redistribute[protocol]`, `syslog[address,port,protocol]`, `aaa.{radius,tacacs}.servers[address,port]`, `ospf.areas[id]` (already refined). Or convert to records where the key is a name/address (H4).

**M3 — `routing.static-unique` compares prefix *text*.** `semantic/routing.ts:85` keys on `vrf + prefix.toLowerCase()`; probe `2001:db8:0::/32` vs `2001:DB8::/32` → ACCEPTED as two routes. VPP sees one prefix, so the declarative reconciler ends up with two desired entries for one FIB key (D-a11 bypassed). Fix: key on `parseCidr()` → `(family, networkAddress, length)` as `interfaces.address-no-overlap` already does.

**M4 — `passwordHash` inside the config document leaks through every non-GET path.** Probe: changing only `fullName` on a user yields `diff()` → `replace /management/users from [{…"passwordHash":"$…"}]` (array-as-leaf sends *every* user's hash). The same bytes flow into `GET /config/diff`, revisions, export/import and `audit_log.before/after` unless every consumer strips `x-vrx-ui.secret` fields — the package gives them no helper. Round-trip hazard: a client that GETs (hash stripped per `writeOnly`) and PUTs back deletes hashes (probe: for the admin the semantic rule catches it; for other users the hash silently vanishes). docs/04 already has `app_user.password_hash` as the auth store, so `management.users[].passwordHash` is a second source of truth. Fix (additive, P02a-owned): export `secretPointers(schema)` / `redactSecrets(doc)` driven by the `x-vrx-ui.secret` meta (it is in `z.globalRegistry`), and write the round-trip rule into `docs/contracts/schema.md` ("absent write-only field on PATCH/PUT keeps the stored value; `null` clears it"). Flag to P06: `app_user` vs `management.users` ownership.

**M5 — Coverage threshold `src/semantic/**/*.ts` = 100 % (`vitest.config.ts:19`) covers P02b/P02c validators that were never measured.** P02b.md:135: "the number was not measured (P02b-questions #7)". `pnpm test:coverage` is not in `tools/ci.sh`, so the gate does not break, but the P02 acceptance check the manager runs on the merged tree probably will. Fix: run `pnpm --filter @ngfw/schema test:coverage` on the merged tree; either siblings top up tests or thresholds are listed per file (P02a can list its six validator files explicitly; the manager extends).

**M6 — `mergePatch` / `mergePatchAt` honour `__proto__` keys.** `merge-patch.ts:17-22,49-51` assign `result[key] = …` on a plain object; probe `mergePatch({}, {"__proto__":{"pwn":1}})` → an object whose *prototype* is attacker-controlled (`polluted.pwn === 1`, `Object.keys` empty, `JSON.stringify` = `{}`); `mergePatchAt({}, '/__proto__/pwn2', true)` likewise. No global pollution (verified `({}).pwn` undefined) and `RootConfig` strict parsing only reads own keys, so validation is safe — but the candidate object between PATCH and validate carries inherited attacker properties; any `in` check or property read on it before parse is unsafe. Fix: reject `__proto__`, `constructor`, `prototype` segments in `parsePointer()` / `mergePatch()` with a clear error (→ problem+json 400), or build results via `Object.create(null)` / `Object.defineProperty`.

**M7 — WBS rows in the envelope not covered (all additive later; list so they are planned, not discovered).** D1.5 bonding (no `bond {mode, loadBalance, members[]}`; `BondEthernet0` exists only as a name); D1.2 per-interface rx/tx queue counts and worker placement; L2 bridge domains (none); D2.3 neighbour DB (static ARP/ND, RA, DAD); D2.2 blackhole/reject routes are *impossible* (`routing.ts:128` `nextHops.min(1)`; probe `nextHops: []` rejected) and no next-hop table (route leaking); D3.9 PBR; D3.3 AS-path / community lists, per-VRF BGP instances (questions #7), `remoteAs: external|internal`, unnumbered BGP by interface; D0.6 NUMA/isolcpus/RSS. Blackhole is the one worth adding now (`type: 'blackhole'|'reject'` with `nextHops.min(0)` conditional) because relaxing `min(1)` later is a loosening of a tagged constraint.

### LOW

- **L1** `validate.ts:16-23` `pointerIssues` for `unrecognized_keys` points at the object (`/interfaces/Ten0~10~10`, message `Unrecognized key: "bogus"`) — append `issue.keys[0]` so the UI can highlight `/interfaces/Ten0~10~10/bogus`.
- **L2** `diff.ts:28` `diff(a, b, base = '')` exposes the recursion accumulator as public API; a future `options` for keyed array diff (D-a8's "additive option later") cannot be added without a signature change. Wrap: `export function diff(a, b)` → internal `walk(a, b, base)`.
- **L3** `ui.ts:24-28` says `itemKey` is "used by `diff()` to match items" — `diff()` does not (schema.md:43-44 is correct). Fix the comment.
- **L4** `ip.ts` is not exported from `index.ts` (`package.json` `exports` has only `.`), so P02b re-implemented `ipFamily`/`prefixToRange`/`vrfExists` in `semantic/objects.ts` (P02b-questions #3). Add `export * from './ip.js'` — no name clashes (scanned).
- **L5** `routing.ts:158-177` prefix-list rules: FRR requires `len < ge <= le` and `le <= 32|128` by family; probe `10.0.0.0/24 ge 8` ACCEPTED → rejected only by `vtysh -C` (tier c). Add a family-aware refine.
- **L6** `interfaces.ts:47-53` `subInterfaceId` accepts `0` (probe) — confirm with P04 whether VPP accepts sub-id 0; if not, `^[1-9][0-9]{0,9}$`.
- **L7** `semantic/interfaces.ts:61-94` flags `loop0 10.0.0.5/32` vs `loop1 10.0.0.1/24` as overlap (probe). Confirm against VPP 26.06 `ip4_add_del_interface_address` conflict rules; if VPP permits /32,/128 host addresses inside another interface's subnet (common loopback/anycast pattern), exempt host-length prefixes.
- **L8** Sub-interface `mtu` may exceed the parent's (probe accepted); VPP rejects at apply time — cheap validator.
- **L9** `primitives.ts:274-285` `timezone` existence depends on the runtime's ICU data; the JSON Schema only carries the pattern, so the UI/other consumers cannot reproduce the check. Document, or ship the tzdata name list.
- **L10** `username` accepts `root`, `vpp`, `daemon` (probe) — reserve system accounts once P06 fixes the PAM/SSH mapping.
- **L11** `routing.ts:80-88` `ospf.areas[].id` union (`0` vs `'0.0.0.0'`) with `itemKey: ['id']`: the refine treats them as one area, `itemKey` pairing treats them as two. Normalise on input (`transform` to one form) or restrict to one form.
- **L12** `docs/status/tasks/P02a-wip.md` says 22 validators, P02a.md says 24 (24 is correct) — cosmetic.

## Decisions D-a1…D-a11 — will any force a breaking change soon after the tag?

| decision | verdict |
|---|---|
| D-a1 strict objects | good; P02b/P02c follow the same convention |
| D-a2 `default` VRF implicit, declared only with id 0 | good; consistent with P02b's `vrfExists`; `vrfs.default-is-table-zero` also rejects a non-default VRF with id 0 (probe) |
| D-a3 `enabled` defaults `false` | fine — matches VPP/TNSR admin-down; no schema change expected |
| D-a4 `corelist` as `number[]` | fine; renderer should emit `corelist-workers` only (VPP refuses `workers` + `corelist-workers` together) — validator already requires equality |
| D-a5 optional protocol objects | good; `null` in a merge-patch disables cleanly |
| D-a6 `scope` literal `'*'` | good; widening to an enum is additive |
| D-a7 admin always required | **change or fixture-fix before merge (H1)**; product-wise defensible, process-wise it contradicts schema.md:27-28 and both siblings |
| D-a8 arrays as diff leaves + `itemKey` | acceptable for v1, but `itemKey` without uniqueness (M2) is a half-contract, and `diff()`'s signature (L2) blocks the promised additive option |
| D-a9 parent-only keys, nested sub-interfaces | good; P02b `lookupInterface` and P02c `tunnels-common.ts:199` already resolve `<parent>.<id>` |
| D-a10 portable patterns, no control chars in descriptions | good but incomplete (M1: `asPath`, banners) |
| D-a11 duplicate static routes are errors | right call, but text-keyed (M3) |
| (not logged) `bgp.neighbors` as array | **decide before tag (H4)** |

## Required before merge / tag

1. H1: scope `examples.test.ts` to group-(a) fixtures and settle D-a7 (recommended: fire only when `users` is non-empty); update schema.md:27-28 accordingly.
2. H2: rename `DnsSchema`/`NtpSchema`/`NtpServerSchema` in `domains/system.ts` (or explicit exports in `index.ts`).
3. H3: resolve the `index.test.ts` conflict (take P02a's version) and re-run `tools/ci.sh --base main` + `pnpm --filter @ngfw/schema test:coverage` on the merged tree.
4. H4: decide array vs record for `bgp.neighbors` and log the P12 shape decision.
5. M1–M3: control-char rule on `asPath`/banners, `uniqueBy` validators for `itemKey` arrays, parsed-prefix key in `static-unique` — all small, P02a-owned, additive.

Everything else can follow as ordinary `contract/<id>` branches after the tag.

**APPROVE WITH CHANGES**
