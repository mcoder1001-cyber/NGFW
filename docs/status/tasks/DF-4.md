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
| D-DF4-2 | Descriptor names `acl.acl`, `acl.macip-acl`, … → keys `acl.acl/<name>`; helpers `KeyACL`/`KeyMacipACL` | (a) `acl/<name>` literal from DF-4.md (b) README `<plugin>.<object>` | frozen contract needs the descriptor name as first key segment; README names `acl.acl` explicitly |
| D-DF4-3 | Tag `<owner>:<name>` via `vpp.OwnerTag` | (a) `w<N>-<name>` (DF-4.md) (b) shared helper | one ownership mechanism for interfaces and ACLs (D-030) |
| D-DF4-4 | Interface dependency Optional with pluggable key (`WithInterfaceKey`, default `interface/<name>`); ACL deps mandatory | (a) hard-code `interface.loopback/<name>` mandatory (b) optional + pluggable | DF-1 not merged; a wrong mandatory key would fail every transaction with bindings |
| D-DF4-5 | Binding ownership = all listed ACLs are ours; whitelist ownership = interface tag; mixed/foreign never touched | (a) interface tag for all (b) ACL ownership for ACL bindings | physical ports may be untagged; ACLs always carry the owner |
| D-DF4-6 | Codec is faithful (no masking/normalising); `Validate` requires canonical prefixes/MACs, sorted ethertypes, no duplicate ACL per direction | (a) canonicalise silently in the encoder (b) reject non-canonical | (a) makes desired ≠ retrieved → perpetual Update; VPP stores rules as sent |
| D-DF4-7 | `acl.stats-enable`: raw-stream send accepting `acl_del_reply` (VPP 26.06 handler bug); Retrieve = last value applied by this process; never disables; Delete no-op | (a) generated Invoke (fails) (b) raw stream (c) skip stats-enable | (b) is configuration-only and works; no getter exists in the API |
| D-DF4-8 | Integration test leaves the counters flag on unless `VRX_ACL_STATS_RESTORE_DISABLED=1` (operator read it first) | (a) always disable in Cleanup (b) never (c) opt-in restore | (a) can break a concurrent owner; (c) honours "restore" when the operator knows the prior state |
| D-DF4-9 | `MacipBinding.Update` to another ACL = one `macip_acl_interface_add_del` add (VPP unapplies the old one) | (a) ErrRecreate (b) in place | acl.c does the swap; fewer operations, same result |
| D-DF4-10 | Stats reader takes the `adapter.StatsAPI` subset (`StatsSource`) — no change to `vpp.Client` | (a) request a contract change adding stats to `vpp.Client` (b) separate interface | P05 can hand `*statsclient.StatsClient` straight in; no contract churn |

## Out of scope / not done

API, UI, schema, F-* wiring, ABF/classifier (DF-2), NAT, policers, packet-level tests, conn-table sizing, hash-lookup
tuning (`acl_plugin_use_hash_lookup_set` left at default), `acl_interface_add_del` (superseded by set-list), `macip_acl_add`
(superseded by add_replace). Restart-safety proof with a running agent is P05's (the descriptors' Retrieve provides it).

## Open questions

See `docs/status/tasks/DF-4-questions.md` (7 items, none blocking): VPP reply-id bug → `docs/vpp-code-track.md` row; who adds
the ACL proto; interface key scheme wiring; key/tag spelling vs DF-4.md; counters-flag restore policy; go.mod additions.
