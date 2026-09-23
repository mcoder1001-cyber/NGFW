# DF-4 — Descriptors: acl (incl. macip), acl stats

Branch `task/DF-4` · worktree `/root/ngfw-wt/DF-4` · slot 10 (`w10`) · prompt `prompts/factories/DF-4.md` · WBS D5.2
Base: main (P05a interfaces, P04 binapi). Files: `apps/agent/internal/descriptors/acl/**`, `docs/agent/descriptors/acl.md`,
`docs/status/tasks/DF-4*.md`; plus `apps/agent/go.{mod,sum}` (govpp transitive modules for the socket/stats adapters, `// indirect`).

## What was built

Package `apps/agent/internal/descriptors/acl` (`Register(registry, client, owner, opts...)` wires all of it):

| # | Object type | Descriptor name / key | VPP messages (binapi/acl) | Meta |
|---|---|---|---|---|
| 1 | ACL (ordered L3/L4 rule list; deny / permit / permit+reflect) | `acl.acl` / `acl.acl/<name>` (`KeyACL`) | `acl_add_replace` (`~0` create, index update), `acl_del`, `acl_dump` | `Meta{ACLIndex}` |
| 2 | interface ACL lists (in/out, `n_input` split; empty = unbind) | `acl.interface-binding` / `…/<ifname>` | `acl_interface_set_acl_list`, `acl_interface_list_dump` (+ `sw_interface_dump` for names) | `BindingMeta{SwIfIndex}` |
| 3 | ethertype whitelist (in/out) | `acl.etype-whitelist` / `…/<ifname>` | `acl_interface_set_etype_whitelist`, `acl_interface_etype_whitelist_dump` | `BindingMeta` |
| 4 | MACIP ACL (L2: src MAC/mask + src prefix, permit/deny) | `acl.macip-acl` / `…/<name>` (`KeyMacipACL`) | `macip_acl_add_replace`, `macip_acl_del`, `macip_acl_dump` | `MacipMeta{ACLIndex}` |
| 5 | MACIP interface binding (one per interface, inbound) | `acl.macip-interface-binding` / `…/<ifname>` | `macip_acl_interface_add_del`, `macip_acl_interface_list_dump` | `MacipBindingMeta{SwIfIndex, ACLIndex}` |
| 6 | hit-counter switch (global singleton) | `acl.stats-enable` / `acl.stats-enable/global` | `acl_stats_intf_counters_enable` (raw stream, see below) | – |
| 7 | hit counters (Retrieve-only typed reader, not a descriptor) | `acl.StatsReader` (`ReadOwned`, `ReadIndex`, `ListPaths`) | stats segment `/acl/<acl_index>/matches` via `adapter.StatsAPI` + `acl_dump` | – |
| 8 | health/limits helper | `acl.GetPluginInfo` | `acl_plugin_get_version`, `acl_plugin_get_conn_table_max_entries` | – |

- `spec.go`: typed specs (`ACL`, `Rule`, `MacipACL`, `InterfaceBinding`, `EtypeWhitelist`, `MacipBinding`, `StatsEnable`) ⇄
  `*structpb.Struct` (`.Proto()` / `*FromProto`), `Validate()` with canonical-form rules. `rules.go`: the one rule codec
  shared by acl and macip (prefix/MAC/action/ports/tcp-flags, byte-identical round trip).
- Dependencies: acl, macip-acl, stats-enable → none · interface-binding → interface (Optional, key via `WithInterfaceKey`,
  default `interface/<name>`) + every listed `acl.acl` (mandatory) · etype-whitelist → interface (Optional) ·
  macip-interface-binding → interface (Optional) + `acl.macip-acl` (mandatory).
- Ownership: ACL/MACIP tag `<owner>:<name>` (`vpp.OwnerTag`); bindings owned through their ACLs; whitelists through the
  interface tag. Retrieve never returns another owner's objects (unit-tested with `w3:` and untagged objects present).
- Docs: `docs/agent/descriptors/acl.md` — key contract (published in the first commit for DF-2), value layout, object ↔
  message table, stats paths, caveats.

## How it was verified (real output)

### Unit tests (fake VPP client, `internal/vpp/fake`) + lint

```
$ cd apps/agent && go vet ./internal/descriptors/... && go test -race -count=1 ./internal/descriptors/...
ok  	ngfw/agent/internal/descriptors/acl	1.234s
$ golangci-lint run ./internal/descriptors/...
0 issues.
$ grep -rn "vppctl\|exec.Command" apps/agent/internal/descriptors/acl ; echo "grep exit=$?"
grep exit=1
```
Coverage: rule codec round trips (prefix lengths 0–32 / 0–128, ports 0–65535, ICMP type/code ranges, TCP flags,
wildcards, 50 generated rules), canonical-form rejection (prefix, MAC), spec validation, structpb tolerance; per
descriptor: create request contents, idempotent re-apply (`diffPlan` empty), update in place keeps the index, reorder =
update not recreate, `ErrRecreate` on identity change, delete, Retrieve decoding with other owners filtered, dependency
lists, VPP retval / `ErrDisconnected` / wrong-meta errors, in-use deletes; stats-enable with both reply types VPP can
send; stats reader (worker summation, spare-slot truncation, zero-fill, missing vector); `Register` order and routing.

### Integration on the host VPP (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w10`, shared lock, tagged loopbacks loop1040/1041)

```
$ VRX_INTEGRATION=1 VRX_TEST_PREFIX=w10 VRX_ACL_STATS_RESTORE_DISABLED=1 VRX_ACL_EVIDENCE_HOLD=1200ms \
    go test -race -count=1 -run TestACLPluginOnHost -v ./internal/descriptors/acl/
=== RUN   TestACLPluginOnHost
    integration_test.go:171: host: acl plugin v1.4, conn table max 500000 entries
    integration_test.go:191: created loop1040 sw_if_index 1 tag "w10:loop1040"
    integration_test.go:192: created loop1041 sw_if_index 2 tag "w10:loop1041"
=== RUN   TestACLPluginOnHost/acl
    integration_test.go:204: created acl.acl/t-lan-in → {ACLIndex:1}
    integration_test.go:205: acl.acl: Retrieve == desired for 1 object(s); re-apply plan:
          (empty plan)
    integration_test.go:217: update kept acl_index 1
=== RUN   TestACLPluginOnHost/acl-50-rules
    integration_test.go:228: acl.acl: Retrieve == desired for 2 object(s); re-apply plan:
          (empty plan)
    integration_test.go:231: 50-rule ACL {ACLIndex:0}: 50 rules retrieved byte-identical
=== RUN   TestACLPluginOnHost/interface-binding
    integration_test.go:244: bound {loop1040 [t-lan-in t-big50] [t-lan-in]} → {SwIfIndex:1}
    integration_test.go:245: acl.interface-binding: Retrieve == desired for 1 object(s); re-apply plan:
          (empty plan)
    integration_test.go:254: acl.interface-binding: Retrieve == desired for 1 object(s); re-apply plan:   ← after reorder → Update
          (empty plan)
    integration_test.go:259: acl_del while bound refused as expected: acl_del 1: VPPApiError: Inbound ACL in use (-142)
=== RUN   TestACLPluginOnHost/etype-whitelist
    integration_test.go:269/274/278: acl.etype-whitelist: Retrieve == desired for 1 / 1 / 0 object(s); re-apply plan: (empty plan)
=== RUN   TestACLPluginOnHost/macip
    integration_test.go:291: acl.macip-acl: Retrieve == desired for 2 object(s); re-apply plan: (empty plan)
    integration_test.go:296: macip acls {ACLIndex:1} {ACLIndex:0}; binding {SwIfIndex:2 ACLIndex:1}
    integration_test.go:297/302: acl.macip-interface-binding: Retrieve == desired for 1 object(s) (create, then switch ACL in place)
=== RUN   TestACLPluginOnHost/stats
    integration_test.go:309: acl.stats-enable: Retrieve == desired for 1 object(s); re-apply plan: (empty plan)
    integration_test.go:324: stats segment ACL vectors (all owners): [/acl/0/matches /acl/1/matches]
    integration_test.go:329: raw vector /acl/0/matches has 51 slots (rules 50 + VPP's spare)
    integration_test.go:350: counters t-big50 (acl 0): 50 rules, all zero
    integration_test.go:350: counters t-lan-in (acl 1): 6 rules, all zero
=== RUN   TestACLPluginOnHost/delete
    integration_test.go:357..383: acl.interface-binding / macip-interface-binding / acl / macip-acl / etype-whitelist:
          Retrieve == desired for 0 object(s); re-apply plan: (empty plan)
    integration_test.go:384: after delete: Retrieve shows nothing of ours for acl, macip-acl, interface-binding, macip-interface-binding, etype-whitelist
=== RUN   TestACLPluginOnHost/macip-del-unbinds
    integration_test.go:400/401: acl.macip-interface-binding / acl.macip-acl: Retrieve == desired for 0 object(s)
    integration_test.go:187: restored acl stats counters flag to disabled (VRX_ACL_STATS_RESTORE_DISABLED=1)
--- PASS: TestACLPluginOnHost (2.79s)
    --- PASS: TestACLPluginOnHost/acl (0.02s)
    --- PASS: TestACLPluginOnHost/acl-50-rules (0.04s)
    --- PASS: TestACLPluginOnHost/interface-binding (0.04s)
    --- PASS: TestACLPluginOnHost/etype-whitelist (0.01s)
    --- PASS: TestACLPluginOnHost/macip (1.22s)
    --- PASS: TestACLPluginOnHost/stats (0.17s)
    --- PASS: TestACLPluginOnHost/delete (0.02s)
    --- PASS: TestACLPluginOnHost/macip-del-unbinds (0.01s)
PASS
ok  	ngfw/agent/internal/descriptors/acl	3.884s
== counters flag BEFORE: Stats counters enabled for interface ACLs: 0
== counters flag AFTER:  Stats counters enabled for interface ACLs: 0
== after test: show acl-plugin acl → 0 lines; w10 tags: 0; macip acl: 0 lines; loop104x: 0
```

### `vppctl show …` while the objects existed (captured by an operator-side watcher during the run; full log `/root/ngfw-wt/logs/DF-4-vppctl-evidence.log`)

```
--- show acl-plugin acl ---
acl-index 0 count 50 tag {w10:t-big50}
          0: ipv4 permit src 10.0.0.0/16 dst 172.16.0.0/24 proto 6 sport 0-60000 dport 0-65535
          1: ipv4 deny src 10.1.0.0/16 dst 172.16.1.0/24 proto 17 sport 1-60001 dport 0-65535 tcpflags 1 mask 1
          2: ipv4 permit+reflect src 10.2.0.0/16 dst 172.16.2.0/24 proto 6 sport 2-60002 dport 0-65535 tcpflags 2 mask 2
          3: ipv6 permit src 2001:db8:3::/48 dst ::/0 proto 6 sport 0-65535 dport 1003
          … (46 more, byte-identical to desired)
  applied inbound on sw_if_index: 1
  used in lookup context index: 0
acl-index 1 count 6 tag {w10:t-lan-in}
          0: ipv6 permit src 2001:db8::/32 dst ::/0 proto 58 sport 128-129 dport 0-255
          1: ipv4 permit src 10.0.0.0/8 dst 0.0.0.0/0 proto 6 sport 0-65535 dport 80
          2: ipv4 permit src 0.0.0.0/0 dst 0.0.0.0/0 proto 6 sport 0-65535 dport 0-65535 tcpflags 2 mask 18
          3: ipv4 deny src 0.0.0.0/0 dst 0.0.0.0/0 proto 0 sport 0 dport 0
          4: ipv6 deny src ::/0 dst ::/0 proto 0 sport 0 dport 0
          5: ipv6 permit src fe80::1/128 dst ::/0 proto 1 sport 0-255 dport 0-255
  applied inbound on sw_if_index: 1
  applied outbound on sw_if_index: 1
  used in lookup context index: 1, 0
--- show acl-plugin interface ---
sw_if_index 1:
  input acl(s): 0, 1
  output acl(s): 1
--- show acl-plugin macip acl ---
MACIP acl_index: 0, count: 1 (true len 1) tag {w10:t-l2-alt} is free pool slot: 0
    rule 0: ipv4 action 1 ip 10.10.1.0/24 mac 02:00:00:00:00:01 mask ff:ff:ff:ff:ff:ff
  applied on sw_if_index(s): 2
MACIP acl_index: 1, count: 2 (true len 2) tag {w10:t-l2-guard} is free pool slot: 0
    rule 0: ipv4 action 1 ip 10.10.1.0/24 mac 02:00:00:00:00:01 mask ff:ff:ff:ff:ff:ff
    rule 1: ipv4 action 0 ip 0.0.0.0/0 mac 00:00:00:00:00:00 mask 00:00:00:00:00:00
  applied on sw_if_index(s):
--- show acl-plugin macip interface ---
  sw_if_index 2: 0
```
(The first snapshot, before the update, showed `acl-index 0 count 7 tag {w10:t-lan-in}` with the original rule order — the
in-place update kept the index and changed the order.) After the test: `show acl-plugin acl` empty, `show acl-plugin macip
acl` empty, no `loop104x` interfaces, counters flag back to 0.

### CI gate

```
$ tools/ci.sh --base main            (log /root/ngfw-wt/logs/DF-4-ci.log)
== install (frozen lockfile) ==
== generated output must be clean ==
== contract guard vs main ==
== forbidden patterns ==
== typecheck == / == lint == / == test == / == build == / == agent ==
CI GATE PASSED
```

## Acceptance checklist (DF-4.md)

- [x] `go test ./internal/descriptors/acl/...` green — unit and integration on the host (above)
- [x] `grep -rn "vppctl\|exec.Command" internal/descriptors/acl` is empty
- [x] Same desired state twice → empty plan: logged for every descriptor, incl. the 50-rule ACL and the reordered binding
- [x] Object ↔ message table + `acl.acl/<name>` key contract in `docs/agent/descriptors/acl.md` (first commit 9948f7d)
- [x] `show acl-plugin acl` / `interface` / `macip acl` pasted for the tagged objects, then empty after delete

## Decisions (with options) — for `docs/decisions/LOG.md`

| # | Decision | Options | Why |
|---|---|---|---|
| D-DF4-1 | Desired-state values are `*structpb.Struct` built from typed Go specs (`spec.go`) | (a) private `.proto` + protoc in the package (b) structpb + typed specs, as the P05a example (c) wait for P03 | (c) stalls; (a) duplicates a contract P03 owns; (b) is one-file-swappable |
| D-DF4-2 | **Key spelling**: descriptor names `acl.acl`, `acl.macip-acl`, `acl.interface-binding`, `acl.etype-whitelist`, `acl.macip-interface-binding`, `acl.stats-enable` → keys `acl.acl/<name>`, `acl.macip-acl/<name>`, …, `acl.stats-enable/global`; helpers `acl.KeyACL`/`KeyMacipACL`/…; DF-2 uses `acl.KeyACL` + `acl.LookupIndex`, never a literal | (a) `acl/<name>` literal from DF-4.md (b) README `<plugin>.<object>` = `acl.acl/<name>` | (b): the frozen scheduler contract routes a key by its first segment = descriptor name, and the README names the descriptor `acl.acl`; (a) would need a descriptor called `acl` for ACLs only |
| D-DF4-3 | **Tag spelling**: VPP `tag` = `<owner>:<name>` via `vpp.OwnerTag` (owner = `VRX_OWNER`, tests `VRX_TEST_PREFIX`, e.g. `w10:t-lan-in`); duplicates reported as `<name>#<index>` (D-DF4-12) | (a) `w<N>-<name>` (DF-4.md / shared-host-rules wording) (b) the shared `vpp.OwnerTag` helper | (b): one ownership mechanism for interfaces and ACLs (D-030); `ParseOwnerTag` is unambiguous (`:` is not allowed in owners), `-` would be ambiguous with names containing `-` |
| D-DF4-4 | Interface dependency Optional with pluggable key (`WithInterfaceKey`, default `interface/<name>`); ACL deps mandatory | (a) hard-code `interface.loopback/<name>` mandatory (b) optional + pluggable | DF-1 not merged; a wrong mandatory key would fail every transaction with bindings |
| D-DF4-5 | *(revised in the review round)* Binding ownership = our entries in the interface list; other owners' entries are preserved (kept first per direction); MACIP binding refuses an interface with a foreign MACIP ACL; whitelist ownership = interface tagged ours **or** untagged + claimed in a `ClaimStore` (in-memory default, `WithEtypeClaims` for a persisted one); foreign-tagged interfaces refused | ACL list: (a) refuse when foreign entries exist (b) preserve them. Whitelist: (a) require the owner tag on the interface (Create errors on untagged ports; DF-1/P05 must tag physical ports) (b) owner registry (claim store) (c) claim every untagged interface | ACL list (b): two owners can share a port and nothing foreign is ever removed. Whitelist (b): works on untagged physical ports today without a DF-1 dependency, never claims a whitelist it did not apply; (c) would delete an operator's whitelist on a shared VPP |
| D-DF4-6 | Codec is faithful (no masking/normalising); `Validate` requires canonical prefixes/MACs, sorted ethertypes, no duplicate ACL per direction | (a) canonicalise silently in the encoder (b) reject non-canonical | (a) makes desired ≠ retrieved → perpetual Update; VPP stores rules as sent |
| D-DF4-7 | `acl.stats-enable`: raw-stream send accepting `acl_del_reply` (VPP 26.06 handler bug); Retrieve = last value applied by this process **to the current VPP** (identity = main-thread PID from `show_threads`, read before the request; a changed identity → Retrieve reports nothing → re-enable); `Reset()` for a reconnect hook; never disables; Delete no-op; `enabled:false` = "not managed" | identity: (a) stats `/sys/boottime` (b) `show_threads` main PID (c) `show_vpe_system_time` (d) only a `Reset()` hook for P05 | (c) is wall-clock (`unix_time_now`), useless; (a) needs the stats client in a descriptor that only has `vpp.Client`; (b) works through the binary API and changes on every restart; (d) kept as an extra, but alone depends on P05 wiring |
| D-DF4-8 | *(reversed in the review round)* Integration test restores the counters flag to **disabled** in Cleanup by default; `VRX_ACL_STATS_KEEP=1` keeps it on | (a) always disable in Cleanup (b) never (c) opt-in restore (d) restore by default, opt-out | (d): the flag is 0 on the host and nothing on main enables it; leaving it on changes global data-plane cost for every slot and `tools/ci.sh full` would never opt in |
| D-DF4-11 | Empty binding / whitelist (both lists empty) rejected by `Validate` ("omit the object to unbind"); Delete uses a bounds-only check | (a) accept and treat as "unbound" (b) reject | (a) never converges: Retrieve cannot report an empty binding, so every reconcile re-creates it and verify-after-apply fails |
| D-DF4-12 | Duplicate owner tags: lowest index is the object; others reported as `acl.acl/<name>#<index>` / `acl.macip-acl/<name>#<index>` (never desired → scheduler deletes them; `#` rejected in names); `LookupIndex`/bindings resolve the lowest | (a) delete extras inside Retrieve (b) report under distinct keys (c) keep last (old behaviour, random) | Retrieve must not mutate VPP; (b) lets the normal plan remove the orphan (bindings referencing it are updated first by the dependency order) |
| D-DF4-9 | `MacipBinding.Update` to another ACL = one `macip_acl_interface_add_del` add (VPP unapplies the old one) | (a) ErrRecreate (b) in place | acl.c does the swap; fewer operations, same result |
| D-DF4-10 | Stats reader takes the `adapter.StatsAPI` subset (`StatsSource`) — no change to `vpp.Client` | (a) request a contract change adding stats to `vpp.Client` (b) separate interface | P05 can hand `*statsclient.StatsClient` straight in; no contract churn |

## Out of scope / not done

API, UI, schema, F-* wiring, ABF/classifier (DF-2), NAT, policers, packet-level tests, conn-table sizing, hash-lookup
tuning (`acl_plugin_use_hash_lookup_set` left at default), `acl_interface_add_del` (superseded by set-list), `macip_acl_add`
(superseded by add_replace). Restart-safety proof with a running agent is P05's (the descriptors' Retrieve provides it).

## Open questions

See `docs/status/tasks/DF-4-questions.md` (9 items, none blocking; Q6 resolved, Q8/Q9 added in the review round): VPP reply-id bug → `docs/vpp-code-track.md` row; who adds
the ACL proto; interface key scheme wiring; key/tag spelling vs DF-4.md; counters-flag restore policy; go.mod additions.

## Review fixes (review `d75fed1`, verdict APPROVE WITH CHANGES)

Main merged first (`4d2839d`, clean). Every finding fixed; none rejected.

| # | Finding | Fix | Commit | Test |
|---|---|---|---|---|
| M1 | `acl.stats-enable` Retrieve stale after a VPP restart | the applied value is stored with the VPP identity (main-thread PID via `show_threads`, read **before** the enable request); Retrieve reports nothing when the identity changed → scheduler re-enables; `Reset()` hook for P05; doc: `enabled:false` = "not managed", not "off" (D-DF4-7) | `f1d47cf`, `4b40b0d` (host log line) | unit `TestStatsEnableSurvivesVPPRestart` (fake restart → empty Retrieve → Create planned → enabled again; Reset; `show_threads` error surfaces); host: identity == `pgrep -x vpp` |
| M2 | etype whitelist on untagged interfaces invisible → perpetual Create, never deleted | `ClaimStore` (in-memory default, `WithEtypeClaims` for a persisted one): Create claims an untagged interface, Retrieve reports whitelists on interfaces tagged ours or untagged+claimed, Delete/failed Create release; interfaces tagged by another owner refused (`ErrForeignInterface`) (D-DF4-5) | `f1d47cf` | unit `TestEtypeWhitelistUntaggedInterface`; host subtest `etype-whitelist-untagged` on untagged `loop1043` (regression) |
| L3 | empty binding / whitelist passes Validate | `Validate` rejects both-lists-empty ("omit the object to unbind"); Delete uses bounds-only `validateLists`; doc line fixed (D-DF4-11) | `f1d47cf` | `TestSpecValidation` (+5 cases), `TestInterfaceBindingErrors` |
| L4 | duplicate owner tags → duplicate keys, orphan never deleted | lowest index = object; extras reported as `acl.acl/<name>#<index>` / `acl.macip-acl/<name>#<index>` (never desired → deleted); `#` rejected in names; `LookupIndex`, bindings and the stats reader use the same rule (D-DF4-12) | `f1d47cf` | `TestDuplicateTagACL` (incl. a binding on the extra → Update then Delete, empty plan after), `TestDuplicateTagMacipACL` |
| L5 | integration test leaves the global counters flag on | restore to disabled in Cleanup by default; `VRX_ACL_STATS_KEEP=1` opts out (D-DF4-8 reversed, Q6 resolved) | `cc9b01a` | host run log "restored acl stats counters flag to disabled"; `show acl-plugin tables` = 0 after |
| L6 | binding Create removes another owner's ACLs | `setList` dumps the interface's list first and keeps foreign entries (first in their direction, current order), our desired entries after; Retrieve reports only our entries; Delete leaves only the foreign ones. Same spirit for MACIP (one per interface): Create refuses an interface with a foreign MACIP ACL (`ErrForeignMacipBinding`; VPP's `~0` "removed" entries are ignored — found on the host) | `f1d47cf`, `cc9b01a` | unit `TestInterfaceBindingPreservesForeignACLs`, `TestMacipBindingRefusesForeign`; host subtest `foreign-acl-preserved` (owner `w10f` ACL on `loop1041`) |
| I7 | `acl.md:29` wording; key/tag decisions | "no *leaf* ACL messages yet (P03b, D-055)"; new "Ownership of untagged objects" section, duplicate-tag rule, stats identity; D-DF4-2 (key spelling) and D-DF4-3 (tag spelling) rewritten with options for the LOG | `d4173f3` | — |
| I7 (extra) | no host test of a bound-ACL update | `interface-binding` subtest updates the bound `t-lan-in` in place and re-checks ACL + binding Retrieve | `cc9b01a` | host log below |

INFO items not acted on: go.mod `// indirect` additions (reviewer: acceptable; merge before P05 touches go.mod);
`StatsReader.ReadOwned` single-`DumpStats` optimisation (performance is out of scope; noted for F-*).

### Unit tests (fake VPP) — after the fixes

```
$ go test -count=1 -v ./internal/descriptors/acl/ | grep -E '^(--- |ok|FAIL)'
--- PASS: TestACLDescriptor (0.01s)
--- PASS: TestACLDescriptorErrors (0.00s)
--- PASS: TestLookupIndex (0.00s)
--- PASS: TestInterfaceBindingDependencies (0.00s)
--- PASS: TestInterfaceBindingLifecycle (0.00s)
--- PASS: TestInterfaceBindingPreservesForeignACLs (0.00s)
--- PASS: TestInterfaceBindingErrors (0.00s)
--- PASS: TestEtypeWhitelistLifecycle (0.00s)
--- SKIP: TestACLPluginOnHost (0.00s)
--- PASS: TestMacipACLDescriptor (0.00s)
--- PASS: TestMacipBindingLifecycle (0.00s)
--- PASS: TestEtypeWhitelistUntaggedInterface (0.00s)
--- PASS: TestDuplicateTagACL (0.00s)
--- PASS: TestDuplicateTagMacipACL (0.00s)
--- PASS: TestMacipBindingRefusesForeign (0.00s)
--- PASS: TestRulesRoundTrip (0.00s)
--- PASS: TestMacipRulesRoundTrip (0.00s)
--- PASS: TestPrefixCanonical (0.00s)
--- PASS: TestMACCanonical (0.00s)
--- PASS: TestSpecValidation (0.00s)
--- PASS: TestProtoCodec (0.00s)
--- PASS: TestStatsEnableDescriptor (0.00s)
--- PASS: TestStatsEnableSurvivesVPPRestart (0.00s)
--- PASS: TestStatsReader (0.00s)
--- PASS: TestPluginInfo (0.00s)
--- PASS: TestRegister (0.00s)
ok  	ngfw/agent/internal/descriptors/acl	0.043s
```

### Host integration (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w10 go test -race -count=1 -run TestACLPluginOnHost -v ./internal/descriptors/acl/`)

The manager restarted VPP (D-060) during this round (sw_if_index values differ between my runs: 2 → 16 → 2); the final run below is against the VPP that was running afterwards, and all runs before and after the restart passed.

```
    created loop1043 sw_if_index 2 (untagged)
    bound ACL t-lan-in updated in place (7 rules), binding unchanged
=== RUN   TestACLPluginOnHost/etype-whitelist-untagged
    untagged loop1043: whitelist created, retrieved, deleted
    foreign ACL 2 (owner w10f) kept on loop1041 across our Create and Delete
    acl.stats-enable applied to VPP identity (main-thread PID from show_threads) 668679
    restored acl stats counters flag to disabled (set VRX_ACL_STATS_KEEP=1 to keep it on)
--- PASS: TestACLPluginOnHost (0.64s)
    --- PASS: TestACLPluginOnHost/acl (0.04s)
    --- PASS: TestACLPluginOnHost/acl-50-rules (0.09s)
    --- PASS: TestACLPluginOnHost/interface-binding (0.12s)
    --- PASS: TestACLPluginOnHost/etype-whitelist (0.01s)
    --- PASS: TestACLPluginOnHost/etype-whitelist-untagged (0.01s)
    --- PASS: TestACLPluginOnHost/foreign-acl-preserved (0.05s)
    --- PASS: TestACLPluginOnHost/macip (0.03s)
    --- PASS: TestACLPluginOnHost/stats (0.16s)
    --- PASS: TestACLPluginOnHost/delete (0.03s)
    --- PASS: TestACLPluginOnHost/macip-del-unbinds (0.02s)
PASS
ok  	ngfw/agent/internal/descriptors/acl	1.753s
```

Host state afterwards (operator-side check):

```
$ pgrep -x vpp                                         → 668679   (== the identity logged above)
$ vppctl show acl-plugin tables | grep 'Stats counters' → Stats counters enabled for interface ACLs: 0
$ vppctl show acl-plugin acl | grep -c w10             → 0
$ vppctl show interface | grep -c loop104              → 0
```

### CI gate after the fixes (code at `4b40b0d`)

```
== VRX CI gate: quick ==
worktree  /root/ngfw-wt/DF-4
branch    task/DF-4 @ 4b40b0d   (base: main)
...
ok  	ngfw/agent/internal/descriptors/acl	1.255s
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   0m14s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m14s
  apps/agent: make lint test build                   0m11s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      merge main into task/DF-4
      review(DF-4): findings
  mode quick · wall time 0m48s · logs /root/ngfw-wt/logs/ci/DF-4-20260924-003334-752508

CI GATE PASSED
```
