# DF-2 — Descriptors: ip_neighbor, arp (proxy-ARP), ip6_nd (RA, proxy-ND, DAD), urpf, adl, abf, classify, ip_session_redirect

Branch `task/DF-2` · worktree `/root/ngfw-wt/DF-2` · slot 3 (`w3`, tables 3000–3999, `10.3.0.0/16`, `2001:db8:3::/48`).
Continued after a worker stall from the manager's salvage commit 800b936.

## Object types (20)

| Plugin | Descriptor names | Retrieve |
|---|---|---|
| ip_neighbor | `ip-neighbor.neighbor`, `ip-neighbor.config` | full |
| arp | `arp.proxy-range`, `arp.proxy-interface` | full |
| ip6_nd | `ip6-nd.ra-config`, `ip6-nd.ra-prefix`, `ip6-nd.dad` | full |
| ip6_nd | `ip6-nd.proxy` | full, but **opt-in** (`RegisterProxyNd`, D-064): unverified on the host |
| urpf | `urpf.interface` | full |
| adl | `adl.interface` | presence via `feature_is_enabled` (review H2) |
| adl | `adl.allowlist` | **write-only** (D-063), `RegisterWriteOnly` only |
| abf | `abf.policy`, `abf.attach` | full |
| classify | `classify.table`, `classify.session`, `classify.input-acl` | full |
| classify | `classify.output-acl` | presence via `feature_is_enabled` + recorded tables (review H2) |
| classify | `classify.interface-ip-table`, `classify.interface-l2-tables` | **write-only** (D-063), `RegisterWriteOnly` only |
| ip_session_redirect | `ip-session-redirect.redirect` | full |

Every type has KeyOf/Dependencies/Create/Update (or `ErrRecreate`)/Delete/Retrieve, fake-client unit tests and a
host integration test. Object ↔ message tables: `docs/agent/descriptors/{ip_neighbor,arp,ip6_nd,urpf,adl,abf,classify,ip_session_redirect}.md`.
Entry points (wired by P05/P08): `Register(r scheduler.Registry, c vpp.Client, owner string[, …])` per package — arp and abf take
a `*df2.IDRange` (owned table / policy ids), classify and ip_session_redirect a `classify.Store`.
Shared helpers: `internal/descriptors/df2` (keys, address/MAC/FIB-path codecs, interface snapshot, typed errors, `df2test` fixtures).

## Done in this continuation
- ACL dependency key aligned with DF-4 `docs/agent/descriptors/acl.md`: `acl.acl/<name>` (was `acl/<name>`).
- `classify.session` Retrieve no longer reports `ip_session_redirect` sessions (VPP lists them in `classify_session_dump`, so the
  reconciler would have deleted every redirect) — exclusion comes from `ip_session_redirect_dump`, so it is restart-safe; unit-tested.
- Retrieve tolerates an interface disappearing between `sw_interface_dump` and a per-interface query (`df2.InterfaceVanished`).
- New host test `df2/idempotency`: applies a 13-object desired state, recomputes the plan exactly as the scheduler contract defines
  it (absent→Create, !proto.Equal→Update, owned-undesired→Delete) → empty.
- `docs/agent/descriptors/*.md` (8), DF-2-questions.md, DAD comments corrected (ip6_dad API is core in 26.06), lint fix.

## Verification (real output)

### Integration on the host VPP (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w3 VRX_SLOT=3 VRX_VPP_TABLE_BASE=3000 go test -p 1 -count=1 -v ./internal/descriptors/...`)
Unit tests run in the same invocation (all PASS; full log `/root/ngfw-wt/logs/DF-2-integration.log`). Excerpt:
```
    integration_test.go:54: abf policy Retrieve = policy_id:3001 acl:"abf-test" paths:{next_hop:"10.3.8.254" interface:"loop308" weight:1}
    integration_test.go:75: abf attach Retrieve = policy_id:3001 interface:"loop307" priority:10
--- PASS: TestABFOnHost (0.03s)
ok  	ngfw/agent/internal/descriptors/abf	0.052s
    integration_test.go:46: adl enabled on loop306 with allow-list table 3003 (write-only: no dump in the adl API)
--- PASS: TestADLOnHost (0.02s)
ok  	ngfw/agent/internal/descriptors/adl	0.037s
    integration_test.go:44: Retrieve: arp.proxy-range/3001/10.3.2.1-10.3.2.9
--- PASS: TestProxyRangeOnHost (0.01s)
    integration_test.go:74: Retrieve: arp.proxy-interface/loop301 (meta {SwIfIndex:3})
--- PASS: TestProxyInterfaceOnHost (0.01s)
ok  	ngfw/agent/internal/descriptors/arp	0.051s
    integration_test.go:67: table Retrieve = name:"w3-t1" nbuckets:8 memory_size:2097152 match_n_vectors:1 mask:"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\xff\xff\xff\xff" miss_next_index:4294967295 (meta {Index:0})
    integration_test.go:87: session Retrieve = table:"w3-t1" match:"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\n\x03\t\n" hit_next_index:65535 opaque_index:4294967295
    integration_test.go:108: input-acl Retrieve = interface:"loop309" ip4_table:"w3-t1"
    integration_test.go:138: bindings applied on loop309: input-acl, output-acl, ip-table, l2-tables (table index 0)
--- PASS: TestClassifyOnHost (0.03s)
ok  	ngfw/agent/internal/descriptors/classify	0.064s
ok  	ngfw/agent/internal/descriptors/df2	0.018s
    idempotency_test.go:142: apply #1 plan: create=13 update=0 delete=0
    idempotency_test.go:153:   created ip-neighbor.neighbor/loop350/10.3.50.10
    idempotency_test.go:153:   created ip-neighbor.neighbor/loop350/2001:db8:3:50::10
    idempotency_test.go:153:   created arp.proxy-range/3050/10.3.50.100-10.3.50.120
    idempotency_test.go:153:   created arp.proxy-interface/loop350
    idempotency_test.go:153:   created ip6-nd.ra-config/loop350
    idempotency_test.go:153:   created ip6-nd.ra-prefix/loop350/2001:db8:3:50::/64
    idempotency_test.go:153:   created urpf.interface/loop350/ipv4/rx
    idempotency_test.go:153:   created abf.policy/3050
    idempotency_test.go:153:   created abf.attach/3050/loop350/ipv4
    idempotency_test.go:153:   created classify.table/w3-idem
    idempotency_test.go:153:   created classify.session/w3-idem/0000000000000000000000000a03320a
    idempotency_test.go:153:   created classify.input-acl/loop351
    idempotency_test.go:153:   created ip-session-redirect.redirect/w3-idem/0000000000000000000000000a03320b
    idempotency_test.go:169: apply #2 (same desired state) plan: create=0 update=0 delete=0 empty=true
    idempotency_test.go:175: excluded (write-only, no VPP dump): classify.interface-ip-table
    idempotency_test.go:175: excluded (write-only, no VPP dump): classify.output-acl
--- PASS: TestApplyTwiceEmptyPlan (0.07s)
ok  	ngfw/agent/internal/descriptors/df2/idempotency	0.094s
    integration_test.go:46: ra-config Retrieve = interface:"loop303" managed:true other:true suppress_link_layer_option:true router_lifetime:1800 max_interval:300 min_interval:225 initial_count:2 initial_interval:16
    integration_test.go:69: ra-prefix Retrieve = interface:"loop303" prefix:"2001:db8:3:3::/64" valid_lifetime:7200 preferred_lifetime:3600 off_link:true
--- PASS: TestRaOnHost (0.03s)
    integration_test.go:109: skip: ip6nd_proxy_add_del crashed VPP 26.06 on vrx-a (2026-09-23 15:52:38); set VRX_DF2_PROXY_ND=1 to run
--- SKIP: TestProxyNdOnHost (0.00s)
    integration_test.go:150: dad before (global, restored in Cleanup): []
    integration_test.go:173: dad: enabled (transmits 2) → retrieved → disabled → absent; ip6_dad.api is a core API on this host
--- PASS: TestDadOnHost (0.01s)
ok  	ngfw/agent/internal/descriptors/ip6_nd	0.069s
    integration_test.go:72: Retrieve shows 2 neighbour(s) of owner w3: [ip-neighbor.neighbor/loop300/10.3.1.10 ip-neighbor.neighbor/loop300/2001:db8:3:1::10]
    integration_test.go:89: after Delete: 0 neighbour(s) of owner w3
--- PASS: TestNeighborOnHost (0.02s)
    integration_test.go:111: previous ipv4 config: max_number:50000
    integration_test.go:130: ipv4 config after set: max_number:50001
--- PASS: TestConfigOnHost (0.01s)
ok  	ngfw/agent/internal/descriptors/ip_neighbor	0.054s
    integration_test.go:61: redirect Retrieve = table:"w3-isr" match:"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\n\x03\n\n" opaque_index:4294967295 paths:{next_hop:"10.3.10.254" interface:"loop310" weight:1}
--- PASS: TestRedirectOnHost (0.02s)
ok  	ngfw/agent/internal/descriptors/ip_session_redirect	0.041s
    integration_test.go:58: Retrieve[urpf.interface/loop302/ipv4/rx] = interface:"loop302"  mode:STRICT
    integration_test.go:58: Retrieve[urpf.interface/loop302/ipv4/tx] = interface:"loop302"  direction:TX  mode:LOOSE  table_id:3002
--- PASS: TestURPFOnHost (0.02s)
ok  	ngfw/agent/internal/descriptors/urpf	0.046s
```
`TestDadOnHost` is `skip-unless-plugin-loaded` (skips on `df2.ErrPluginNotLoaded`); on vrx-a the ip6_dad messages are core API,
so it ran and passed. The skip path is unit-tested (`ip6nd_test.go`, UnknownMsgError → ErrPluginNotLoaded).
`TestProxyNdOnHost` is skipped on purpose (questions #1).

### Same desired state twice → empty plan
```
apply #1 plan: create=13 update=0 delete=0
apply #2 (same desired state) plan: create=0 update=0 delete=0 empty=true
excluded (write-only, no VPP dump): classify.interface-ip-table
excluded (write-only, no VPP dump): classify.output-acl
```

### vppctl while the idempotency test held its objects (`VRX_DF2_HOLD=20s`, evidence only — no vppctl in code)
```
vpp# show ip neighbors loop350
     Age                       IP                    Flags      Ethernet              Interface       
      1.0717            2001:db8:3:50::10             SN    02:00:00:03:50:11 loop350
      1.0724               10.3.50.10                  S    02:00:00:03:50:10 loop350
vpp# show ip6 interface loop350
loop350 is admin up
  Global unicast address(es):
    2001:db8:3:50::1/64
  Link-local address(es):
    fe80::dcad:ff:fe00:5e
  Joined group address(es):
    ff02::1
    ff02::2
    ff02::16
    ff02::1:ff00:5e
    ff02::1:ff00:1
  Neighbor Discovery: enabled
    ICMP redirects are disabled
    ICMP unreachables are not sent
    ND DAD is disabled
  Advertised Prefixes:
      prefix 2001:db8:3:50::, length 64
    MTU is 9000
    ICMP error messages are unlimited
    ICMP redirects are disabled
    ICMP unreachables are not sent
    ND DAD is disabled
    ND advertised reachable time is 0
    ND advertised retransmit interval is 0 (msec)
    ND router advertisements are sent every 400.0 seconds (min interval is 300.0)
    ND router advertisements live for 1200 seconds
    Hosts use stateless autoconfig for addresses
    ND router advertisements sent 1
    ND router solicitations received 0
    ND router solicitations dropped 0
  VRRP VRs monitoring this link:
vpp# show arp proxy
Proxy arps enabled for:
Fib_index 1   10.3.50.100 - 10.3.50.120 
vpp# show interface features loop350
  … (feature arcs without configuration elided)
arp:
  arp-reply
  arp-proxy
ip4-unicast:
  ip4-rx-urpf-strict
vpp# show abf policy
abf:[0]: policy:3050 acl:0
     path-list:[34] locks:2 flags:shared,no-uRPF, uRPF-list: None
      path:[44] pl-index:34 ip4 weight=1 pref=0 attached-nexthop:  oper-flags:resolved,
        10.3.51.254 loop351
      [@0]: arp-ipv4: via 10.3.51.254 loop351
vpp# show abf attach loop350
ipv4:
 abf-interface-attach: policy:3050 priority:5
  [@1]: arp-ipv4: via 10.3.51.254 loop351
vpp# show classify tables

  TableIdx  Sessions   NextTbl  NextNode
         0         2        -1        -1
  Heap: base 0x770fed741000, size 2m, locked, unmap-on-destroy, name 'classify'
          page stats: page-size 4K, total 512, populated 2, not-populated 510
            numa 1: 2 pages, 8k bytes
          total: 2.00M, used: 1.33K, free: 2.00M, trimmable: 2.00M
  nbuckets 8, skip 0 match 1 flag 0 offset 0
  mask 000000000000000000000000ffffffff
  linear-search buckets 0
vpp# show ip session redirect
[0] [acl] [ip4] table 0 key 0000000000000000000000000a03320b opaque_index 0xffffffff
 via:
    path-list:[34] locks:2 flags:shared,no-uRPF, uRPF-list: None
    path:[44] pl-index:34 ip4 weight=1 pref=0 attached-nexthop:  oper-flags:resolved,
      10.3.51.254 loop351
    [@0]: arp-ipv4: via 10.3.51.254 loop351
 forwarding
  [@2]: arp-ipv4: via 10.3.51.254 loop351
```

### vppctl after the test's cleanup (grep for the w3 objects: nothing left)
```
vpp# show ip neighbors loop350
unknown input `loop350'
vpp# show abf policy
vpp# show abf attach loop350
show abf attach: unknown input 'loop350'
vpp# show ip session redirect
vpp# show arp proxy
vpp# show interface | grep loop35

vpp# show ip neighbor-config
ip4:
  limit:50000, age:0, recycle:0
ip6:
  limit:50000, age:0, recycle:0
vpp# show classify tables
No classifier tables configured
```

### No shelling out
```
$ grep -rn "vppctl\|exec.Command" internal/descriptors/{ip_neighbor,arp,ip6_nd,urpf,adl,abf,classify,ip_session_redirect}
(no output, exit 1)
```

### CI gate (`tools/ci.sh --base main`)
```
== install (frozen lockfile) ==
== generated output must be clean ==
$ turbo run gen
• turbo 2.11.2
== contract guard vs main ==
== forbidden patterns ==
== typecheck ==
$ turbo run typecheck
• turbo 2.11.2
== lint ==
$ turbo run lint
• turbo 2.11.2
== test ==
$ turbo run test
• turbo 2.11.2
== build ==
$ turbo run build
• turbo 2.11.2
== agent ==
CI GATE PASSED
EXIT 0
```

## Out of scope / left undone
- Central descriptor registry list: none exists on the base (P05 in progress) → the eight `Register` calls are for P05/P08.
- Host test for `ip6-nd.proxy` disabled (VPP crash, questions #1). adl and three classify binding types are write-only (no dump).
- `no-adj-fib` neighbour flag and RA-prefix `no_onlink` are not modelled (not in binapi / not distinguishable in the dump).
- Values are package-local protos (D-055 stand-in); swapping to P03b domain messages is a follow-up.

## Decisions taken (for the LOG)
- D-DF2-1: ABF depends on `acl.acl/<name>` and resolves acl_index via acl_dump + owner tag (DF-4 contract) — options: own key / DF-4 key → DF-4 key.
- D-DF2-2: Untagged objects (proxy-ARP ranges, ABF policies) are owned by id range (`df2.IDRange`; nil in production) — options: owner table / id range → id range (stateless, restart-safe).
- D-DF2-3: Classify tables owned via a `classify.Store` (FileStore in the state dir) keyed by name, validated against `classify_table_ids` — the API has no tag.
- D-DF2-4: No-dump types return `df2.ErrRetrieveUnsupported` instead of echoing cached desired state (task rule); reconciler policy is questions #2.
- D-DF2-5: `classify.session` excludes ip_session_redirect sessions using `ip_session_redirect_dump` (VPP state, not the Store).

## Open questions
See `docs/status/tasks/DF-2-questions.md` (#1 proxy-ND crash, #2 write-only descriptors in P05, #3 normalisation contract,
#4 foreign key strings, #5 classify Store path, #6 VPP aborts in gtpu during runs, #7 registry wiring).

## Review fixes (review `docs/status/tasks/DF-2-review.md`, verdict BLOCK → fix round)

`git merge main` first (f70aff1): go.mod/go.sum conflict resolved by taking main's version (identical to main after
`go mod tidy`) — finding 7; DF-2 no longer changes go.mod/go.sum relative to main.

| Finding | Fix | Commit |
|---|---|---|
| H1 classify store claims reused indices | Store bound to the VPP instance (`control_ping_reply.vpe_pid`; other pid → reset), mask stored per record, record trusted only if `classify_table_ids` lists it **and** `classify_table_info` geometry (skip/match/mask) matches; unit tests for both | f3d1833 |
| H2 write-only types / output-acl A→B | `adl.interface` and `classify.output-acl` read presence via `feature_is_enabled` (device-input/adl-input, ip4-output/ip4-outacl, ip6-output/ip6-outacl); output-acl keeps bound indices in the Store, Create unbinds a recorded binding first, refuses when an unrecorded table is bound, Delete unbinds by recorded index; L2 output refused. `adl.allowlist`, `interface-ip-table`, `interface-l2-tables` moved to `RegisterWriteOnly` (D-063) | f3d1833, f5a5ab2 |
| H3 untagged interfaces invisible | `df2.WithClaims` (DF-4's `acl.ClaimStore`, claim key = object key) + `df2.FileClaimStore`; Create on an untagged interface claims, Delete releases, Retrieve reports untagged-interface objects only when claimed; interfaces tagged by another owner and `local0` refused (`df2.ErrForeignInterface`). Applied to neighbor, proxy-arp interface, RA config/prefix, proxy-ND, uRPF, adl, abf attach, input/output ACL | f3d1833, f5a5ab2 |
| H4 proxy-ND registered by default | out of `ip6nd.Register`, opt-in `RegisterProxyNd`; crash details (journal stack) in DF-2-questions.md #1 for vpp-code-track (D-064) | f3d1833, 88205a4 |
| M5 ABF vs DF-4 duplicate tags | Create uses `acl.LookupIndex` (lowest index canonical); Retrieve names non-canonical duplicates `name#idx` (D-066); dependency `acl.KeyACL` | f5a5ab2 |
| M6 Retrieve mutates the store | `LiveTables`/snapshot read-only (records read before the VPP dump); `Prune` only under the Store's transaction lock with a fresh snapshot; `TableDescriptor.Create` holds the same lock from its prune to its Put | f3d1833 |
| L8 | table Create deletes the VPP table when the record cannot be stored; `classify.session` refuses a redirect's match; global-singleton note in docs + questions #8 | f3d1833, 88205a4 |
| INFO | idempotency host test now has a restart pass (fresh descriptors, reopened classify FileStore + claim store → empty plan, Meta equal) and claimed objects on an untagged loopback (stand-in for a physical port); `arp.md` states the production nil-range semantics | f5a5ab2, 88205a4 |
| lint | OutputRecord field names | 5ca42e4 |

### Unit tests for the findings (`go test -v -run … ./internal/descriptors/{classify,abf,adl,ip6_nd}/`)
```
--- PASS: TestRegister (0.00s)
--- PASS: TestStoreDropsRecordsOfAnotherVPPInstance (0.00s)
--- PASS: TestStoreRejectsReusedIndexWithOtherGeometry (0.00s)
--- PASS: TestPruneWaitsForCreate (0.00s)
--- PASS: TestTableCreateRollsBackOnStoreError (0.00s)
--- PASS: TestOutputACLSwitchesTables (0.00s)
--- PASS: TestOutputACLUnknownBinding (0.00s)
--- PASS: TestBindingsOnUntaggedInterfaces (0.00s)
--- PASS: TestSessionRefusesRedirectMatch (0.00s)
ok  	ngfw/agent/internal/descriptors/classify	0.025s
--- PASS: TestRegister (0.00s)
--- PASS: TestDuplicateACLTagsFollowDF4 (0.00s)
ok  	ngfw/agent/internal/descriptors/abf	0.024s
--- PASS: TestInterface (0.00s)
--- PASS: TestRegister (0.00s)
ok  	ngfw/agent/internal/descriptors/adl	0.025s
--- PASS: TestRegister (0.00s)
ok  	ngfw/agent/internal/descriptors/ip6_nd	0.020s
```

### Host run (`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w3 VRX_SLOT=3 VRX_VPP_TABLE_BASE=3000 go test -p 1 -count=1 -v …`), NRestarts before/after
```
before: NRestarts=2
ok  	ngfw/agent/internal/descriptors/ip_neighbor	0.047s
ok  	ngfw/agent/internal/descriptors/arp	0.054s
--- SKIP: TestProxyNdOnHost (0.00s)
ok  	ngfw/agent/internal/descriptors/ip6_nd	0.048s
ok  	ngfw/agent/internal/descriptors/urpf	0.058s
    integration_test.go:44: adl.interface Retrieve = interface:"loop306" (meta {SwIfIndex:4})
ok  	ngfw/agent/internal/descriptors/adl	0.044s
ok  	ngfw/agent/internal/descriptors/abf	0.061s
    integration_test.go:135: output-acl Retrieve = interface:"loop309" ip4_table:"w3-t1"
    integration_test.go:156: output-acl after A→B = interface:"loop309" ip4_table:"w3-t2"
ok  	ngfw/agent/internal/descriptors/classify	0.078s
ok  	ngfw/agent/internal/descriptors/ip_session_redirect	0.046s
ok  	ngfw/agent/internal/descriptors/df2	0.020s
    idempotency_test.go:180: apply #1 plan: create=17 update=0 delete=0
    idempotency_test.go:207: apply #2 (same desired state) plan: create=0 update=0 delete=0 empty=true
    idempotency_test.go:219: apply #3 (fresh descriptors, reopened classify store + claim store) plan: create=0 update=0 delete=0 empty=true
    idempotency_test.go:237: excluded (write-only, no VPP dump): classify.interface-ip-table
    idempotency_test.go:237: excluded (write-only, no VPP dump): classify.interface-l2-tables
    idempotency_test.go:237: excluded (write-only, no VPP dump): adl.allowlist
ok  	ngfw/agent/internal/descriptors/df2/idempotency	0.125s
exit=0
after: NRestarts=2
```
(`loop352` is an **untagged** loopback in the slot range standing in for a physical port; its uRPF, neighbour and output ACL are
found by fresh descriptors only through the reopened claim store.) After the run: `vppctl show interface | grep -cE "loop3[0-9]{2}"` → `0`,
`vppctl show classify tables` → `No classifier tables configured`.

### CI gate (`tools/ci.sh --base main`, after the last code commit)
```
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m18s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   0m16s
  apps/agent: make lint test build                   0m16s
  test/ Go modules, unit mode (test/integration/smoke)   0m03s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      merge main into task/DF-2 (go.mod/go.sum: main's version; DF-4 acl, P02b, P02c)
      review(DF-2): findings
  mode quick · wall time 1m00s · logs /root/ngfw-wt/logs/ci/DF-2-20260924-005925-1086840
CI GATE PASSED
EXIT 0
```
The two warnings are commit subjects not written by this fix round's code commits (the merge commit and the manager's review commit).
