# DF-3: NAT-family descriptors (nat44_ed, nat44_ei, nat64, nat66, det44, map, cnat, pnat)

Branch `task/DF-3` · worktree `/root/ngfw-wt/DF-3` · slot 9 (`w9`) · base main@030f404 · **not merged**.
The first worker built nat44-ed, nat44-ei, nat64 and nat66, and stalled with uncommitted det44 and map work (salvaged
by the manager in ee8d620). This worker checked the salvage, fixed det44 (it crashed VPP), finished map, built cnat
and pnat, and applied D-063/D-064/D-065.

## What was built: 46 descriptor types in 8 packages (`apps/agent/internal/descriptors/…`)

| Package | Descriptors (`Name()`) | Doc |
|---|---|---|
| `nat44ed` | nat44-ed.enable, .timeouts, .forwarding, .interface-feature, .output-feature, .interface-address, .address-pool, .static-mapping, .identity-mapping, .lb-static-mapping, .vrf-table (+ Users/UserSessions/DeleteSession helpers) | docs/agent/descriptors/nat44-ed.md |
| `nat44ei` | nat44-ei.enable, .timeouts, .forwarding, .ipfix *(write-only)*, .interface-feature, .output-feature, .interface-address, .address-pool, .static-mapping, .identity-mapping (+ session helpers) | nat44-ei.md |
| `nat64` | nat64.enable *(write-only)*, .timeouts, .prefix, .pool, .interface, .static-bib (+ Sessions) | nat64.md |
| `nat66` | nat66.enable *(write-only)*, .interface, .static-mapping | nat66.md |
| `det44` | det44.enable *(write-only, never disables)*, .timeouts, .interface, .map (+ Sessions, CloseSessionIn/Out) | det44.md |
| `mapnat` (MAP-E/T, LW4o6) | map.domain, map.rule, map.params, map.interface | map.md |
| `cnat` | cnat.translation, .snat-addresses, .snat-policy / .snat-interface / .snat-exclude-prefix *(write-only)*, .interface-feature (+ Sessions, PurgeSessions) | cnat.md |
| `pnat` | pnat.binding, pnat.attachment | pnat.md |
| `natcommon` (+`nattest`) | shared: structpb carrier (D-055), generic `Descriptor[T]`, owner `Scope` (slot `w<N>` → 10.N/16, fd00:N::/32, tables N000–N999, `w<N>:` tags), interface table, error classification, `ErrRetrieveUnsupported`; test harness (govpp connection, prefixed loopbacks/tables, `Apply`/`AssertPlan`/`AssertWriteOnly`/`CreateWriteOnly`, evidence pause hook) | — |

Every package has `Register(registry, client, owner)` (the entry point P05 wires), a unit test on
`internal/vpp/fake` (create, idempotent re-apply, update/recreate, delete, dependencies, Retrieve decoding, foreign
objects filtered, VPP errors) and a host integration test (`VRX_INTEGRATION=1`, shared lab lock, slot-prefixed
objects, cleanup in `t.Cleanup`). Dependencies use `interface/<name>` (D-065) and `vrf/<id>` (optional).

## How it was verified (real output)

### Unit (`go test ./internal/descriptors/...`)
```
ok  	ngfw/agent/internal/descriptors/cnat	0.033s
ok  	ngfw/agent/internal/descriptors/det44	0.030s
ok  	ngfw/agent/internal/descriptors/mapnat	0.041s
ok  	ngfw/agent/internal/descriptors/nat44ed	0.030s
ok  	ngfw/agent/internal/descriptors/nat44ei	0.030s
ok  	ngfw/agent/internal/descriptors/nat64	0.023s
ok  	ngfw/agent/internal/descriptors/nat66	0.018s
ok  	ngfw/agent/internal/descriptors/natcommon	0.021s
?   	ngfw/agent/internal/descriptors/natcommon/nattest	[no test files]
ok  	ngfw/agent/internal/descriptors/pnat	0.029s
```

### Host integration: all 8 packages, serial, on `/run/vpp/api.sock` (log `/root/ngfw-wt/logs/DF-3-integration.log`)
Command: `VRX_INTEGRATION=1 VRX_TEST_PREFIX=w9 VRX_SLOT=9 VRX_VPP_TABLE_BASE=9000 VRX_DF3_DET44=1 go test -p 1 -count=1 -v ./internal/descriptors/{nat44ed,nat44ei,nat64,nat66,det44,mapnat,cnat,pnat}/ -run OnHost`
(this ran at 403cd72; the only later commit is a lint annotation in the test harness). "plan … after re-apply: empty"
is the **same-desired-state-twice → empty plan** check: Retrieve is `proto.Equal` to desired for every object.
```
$ git -C /root/ngfw-wt/DF-3 log --oneline -1: 403cd72 fix(DF-3): D-063 write-only descriptors (nat64/nat66/det44 enable, nat44-ei ipfix, cnat snat-policy/interface/exclude-prefix) — no cached echo; cnat translation write-only fields not modelled; D-064 det44 host test opt-in; docs + questions
restarts before: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
    nat44ed_integration_test.go:61: plan for nat44-ed.enable after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:74: plan for nat44-ed.timeouts after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:82: plan for nat44-ed.forwarding after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:96: plan for nat44-ed.interface-feature after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:102: plan for nat44-ed.output-feature after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:106: plan for nat44-ed.interface-address after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:112: plan for nat44-ed.address-pool after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:122: plan for nat44-ed.static-mapping after re-apply: empty (4 objects converged)
    nat44ed_integration_test.go:126: plan for nat44-ed.identity-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:131: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:137: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:142: plan for nat44-ed.vrf-table after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:154: nat44-ed session users on host: 0 (asserted for shape only)
--- PASS: TestNat44EdOnHost (0.42s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ed	0.444s
    nat44ei_integration_test.go:57: plan for nat44-ei.enable after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:61: plan for nat44-ei.timeouts after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:64: plan for nat44-ei.forwarding after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:75: plan for nat44-ei.interface-feature after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:79: plan for nat44-ei.interface-address after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:83: plan for nat44-ei.address-pool after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:88: plan for nat44-ei.static-mapping after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:92: plan for nat44-ei.identity-mapping after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:98: plan for nat44-ei.output-feature after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:104: nat44-ei users: 0 (shape only)
--- PASS: TestNat44EiOnHost (0.28s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ei	0.299s
    nat64_integration_test.go:44: nat64.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat64_integration_test.go:48: plan for nat64.timeouts after re-apply: empty (1 objects converged)
    nat64_integration_test.go:57: plan for nat64.prefix after re-apply: empty (1 objects converged)
    nat64_integration_test.go:61: plan for nat64.pool after re-apply: empty (1 objects converged)
    nat64_integration_test.go:66: plan for nat64.interface after re-apply: empty (2 objects converged)
    nat64_integration_test.go:70: plan for nat64.static-bib after re-apply: empty (1 objects converged)
    nat64_integration_test.go:75: nat64 sessions: 0 (shape only)
--- PASS: TestNat64OnHost (0.35s)
PASS
ok  	ngfw/agent/internal/descriptors/nat64	0.376s
    nat66_integration_test.go:43: nat66.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat66_integration_test.go:50: plan for nat66.interface after re-apply: empty (2 objects converged)
    nat66_integration_test.go:54: plan for nat66.static-mapping after re-apply: empty (1 objects converged)
--- PASS: TestNat66OnHost (0.24s)
PASS
ok  	ngfw/agent/internal/descriptors/nat66	0.262s
    det44_integration_test.go:36: det44.enable is write-only: Retrieve → ErrRetrieveUnsupported
    det44_integration_test.go:50: plan for det44.timeouts after re-apply: empty (1 objects converged)
    det44_integration_test.go:57: plan for det44.interface after re-apply: empty (2 objects converged)
    det44_integration_test.go:62: plan for det44.map after re-apply: empty (1 objects converged)
    det44_integration_test.go:67: det44 sessions of 10.9.44.1: 0 (shape only)
--- PASS: TestDet44OnHost (0.33s)
PASS
ok  	ngfw/agent/internal/descriptors/det44	0.358s
    mapnat_integration_test.go:30: plan for map.domain after re-apply: empty (1 objects converged)
    mapnat_integration_test.go:35: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:41: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:49: plan for map.interface after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:69: plan for map.params after re-apply: empty (1 objects converged)
    mapnat_integration_test.go:75: map params restored to VPP defaults
--- PASS: TestMapOnHost (0.25s)
PASS
ok  	ngfw/agent/internal/descriptors/mapnat	0.277s
    cnat_integration_test.go:32: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:39: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:46: plan for cnat.interface-feature after re-apply: empty (1 objects converged)
    cnat_integration_test.go:51: cnat sessions: 0 (shape only)
    cnat_integration_test.go:61: plan for cnat.snat-addresses after re-apply: empty (1 objects converged)
    cnat_integration_test.go:72: cnat.snat-policy: create re-applied twice without error (idempotent)
    cnat_integration_test.go:73: cnat.snat-policy is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:72: cnat.snat-interface: create re-applied twice without error (idempotent)
    cnat_integration_test.go:73: cnat.snat-interface is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:72: cnat.snat-exclude-prefix: create re-applied twice without error (idempotent)
    cnat_integration_test.go:73: cnat.snat-exclude-prefix is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:89: cnat default SNAT entry removed again (global restored)
--- PASS: TestCnatOnHost (0.24s)
PASS
ok  	ngfw/agent/internal/descriptors/cnat	0.265s
    pnat_integration_test.go:27: plan for pnat.binding after re-apply: empty (2 objects converged)
    pnat_integration_test.go:36: plan for pnat.attachment after re-apply: empty (2 objects converged)
--- PASS: TestPnatOnHost (0.34s)
PASS
ok  	ngfw/agent/internal/descriptors/pnat	0.364s
exit=0
restarts after: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
```
`NRestarts` 2 → 2 and ActiveEnterTimestamp unchanged: this run did not restart VPP.

### `vppctl show` while the objects exist (captured at the test's evidence pause; read-only)
Output is unedited except where marked `[…]` (maglev hash maps and bihash internals dropped).
```
### [nat44ed] while the test's objects exist
$ vppctl show nat44 addresses
NAT44 pool addresses:
10.9.20.1
  tenant VRF independent
10.9.1.1
  tenant VRF: 0
10.9.1.2
  tenant VRF: 0
10.9.1.3
  tenant VRF: 0
10.9.1.4
  tenant VRF: 0
NAT44 twice-nat pool addresses:
10.9.2.1
  tenant VRF: 0
$ vppctl show nat44 static mappings
NAT44 static mappings:
 TCP local 10.9.10.50:80 external 10.9.1.1:8080 vrf 0  
 TCP local 10.9.10.51:443 external 10.9.1.2:8443 vrf 0 twice-nat 
 local 10.9.10.53 external 10.9.1.3 vrf 0  
 TCP local 10.9.10.52:22 external 10.9.20.1:2222 vrf 0  
 identity mapping UDP 10.9.1.4:500 vrf 0
 TCP external 10.9.1.4:80  
  local 10.9.10.60:8080 vrf 0 probability 50
  local 10.9.10.62:8080 vrf 0 probability 50
 TCP local 10.9.10.52:22 external loop902:2222 vrf 0
$ vppctl show nat44 interfaces
NAT44 interfaces:
 loop901 in
 loop902 out
 loop906 output-feature in out

### [nat44ei] while the test's objects exist
$ vppctl show nat44 ei addresses
NAT44 pool addresses:
10.9.40.1
  tenant VRF independent
  0 busy other ports
  0 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
10.9.3.1
  tenant VRF: 9002
  0 busy other ports
  0 busy udp ports
  1 busy tcp ports
  0 busy icmp ports
10.9.3.2
  tenant VRF: 9002
  0 busy other ports
  1 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
$ vppctl show nat44 ei static mappings
NAT44 static mappings:
 tcp local 10.9.30.50:80 external 10.9.3.1:8080 vrf 9002
 local 10.9.30.51 external 10.9.40.1 vrf 0
 identity mapping udp 10.9.3.2:500 vrf 0
 local 10.9.30.51 external loop904 vrf 0
$ vppctl show nat44 ei interfaces
NAT44 interfaces:
 loop903 in
 loop904 out
 loop905 output-feature in out

### [nat64] while the test's objects exist
$ vppctl show nat64 bib all
NAT64 BIB entries:
 fd00:9::64 80 10.9.64.1 8080 protocol tcp vrf 0 static 0 sessions
$ vppctl show nat64 pool
NAT64 pool:
 10.9.64.1 tenant VRF: 0
  0 busy other ports
  0 busy udp ports
  1 busy tcp ports
  0 busy icmp ports
 10.9.64.2 tenant VRF: 0
  0 busy other ports
  0 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
$ vppctl show nat64 prefix
NAT64 prefix:
 fd00:9:40::/96 tenant-vrf 9010
$ vppctl show nat64 interfaces
NAT64 interfaces:
 loop910 in
 loop911 out

### [nat66] while the test's objects exist
$ vppctl show nat66 interfaces
NAT66 interfaces:
 loop912 in
 loop913 out
$ vppctl show nat66 static mappings
NAT66 static mappings:
 local fd00:9::66 external fd00:9::6600 vrf 0
  total pkts 0, total bytes 0

### [det44] while the test's objects exist
$ vppctl show det44 mappings
NAT44 deterministic mappings:
 in 10.9.44.0/24 out 10.9.45.0/30
  outside address sharing ratio: 64
  number of ports per inside host: 1008
  sessions number: 0
$ vppctl show det44 interfaces
DET44 interfaces:
 loop921 out
 loop920 in
$ vppctl show det44 timeouts
udp timeout: 299sec
tcp established timeout: 7439sec
tcp transitory timeout: 239sec
icmp timeout: 59sec

### [map] while the test's objects exist
$ vppctl show map domain
[0] tag {w9:w9-lw} ip4-pfx 10.9.46.0/24 ip6-pfx ::/48 ip6-src fd00:9::4601/128 ea-bits-len 0 psid-offset 6 psid-len 4 mtu 1460 prefix
 rule psid: 3 ip6-dst fd00:9::4603
 rule psid: 9 ip6-dst fd00:9::4699
$ vppctl show map domain index 0 counters
[0] tag {w9:w9-lw} ip4-pfx 10.9.46.0/24 ip6-pfx ::/48 ip6-src fd00:9::4601/128 ea-bits-len 0 psid-offset 6 psid-len 4 mtu 1460 prefix  TX: 0/0  RX: 0/0
 rule psid: 3 ip6-dst fd00:9::4603
 rule psid: 9 ip6-dst fd00:9::4699

### [cnat] while the test's objects exist
$ vppctl show cnat translation
[0] 10.9.47.1;53 UDP lb:maglev fhc:0x9f(default)
0.0.0.0;0->10.9.48.3;5353
  fib-entry:26
  [@0]: dpo-load-balance: [proto:ip4 index:25 buckets:1 uRPF:0 to:[0:0]]
        [0] [@0]: dpo-drop ip4
0.0.0.0;0->10.9.48.4;5353
  fib-entry:24
  [@0]: dpo-load-balance: [proto:ip4 index:29 buckets:1 uRPF:0 to:[0:0]]
        [0] [@0]: dpo-drop ip4
 via:
  [@2]: dpo-load-balance: [proto:ip4 index:13 buckets:2 uRPF:-1 to:[0:0]]
    [0] [@15]: dpo-load-balance: [proto:ip4 index:25 buckets:1 uRPF:0 to:[0:0]]
          [0] [@0]: dpo-drop ip4
    [1] [@15]: dpo-load-balance: [proto:ip4 index:29 buckets:1 uRPF:0 to:[0:0]]
          [0] [@0]: dpo-drop ip4
maglev backends map
[1] 10.9.47.1;80 TCP lb:default fhc:0x9f(default)
0.0.0.0;0->10.9.48.1;8080
  fib-entry:20
  [@0]: dpo-load-balance: [proto:ip4 index:24 buckets:1 uRPF:0 to:[0:0]]
        [0] [@0]: dpo-drop ip4
0.0.0.0;0->10.9.48.2;8080
  fib-entry:22
  [@0]: dpo-load-balance: [proto:ip4 index:9 buckets:1 uRPF:0 to:[0:0]]
        [0] [@0]: dpo-drop ip4
 via:
  [@2]: dpo-load-balance: [proto:ip4 index:32 buckets:2 uRPF:-1 to:[0:0]]
    [0] [@15]: dpo-load-balance: [proto:ip4 index:24 buckets:1 uRPF:0 to:[0:0]]
          [0] [@0]: dpo-drop ip4
    [1] [@15]: dpo-load-balance: [proto:ip4 index:9 buckets:1 uRPF:0 to:[0:0]]
          [0] [@0]: dpo-drop ip4
$ vppctl show cnat snat-policy
Source NAT
  ip4: 10.9.49.1;0
  ip6: fd00:9::4901;0

Excluded prefixes:
  Hash table 'snat prefixes'
    […]  0: 10.9.50.0/24 dst […]


Included v4 interfaces:
  loop941

Included v6 interfaces:

k8s pod interfaces:

k8s host interfaces:

### [pnat] while the test's objects exist
$ vppctl show pnat translations
[0] match: {*:*,TCP,10.9.52.2:80} rewrite: {10.9.53.2:*,*:* clear byte@[3]}
[1] match: {10.9.51.1:*,UDP,10.9.52.1:53} rewrite: {*:*,10.9.53.1:5353}
$ vppctl show pnat interfaces
sw_if_index: 17 input mask: SA DA DP
sw_if_index: 12 output mask: DA DP

```
Note: `show map domain` prints `ip6-pfx ::/48` because `format_map_domain` blanks the IPv6 prefix of domains that
have rules (map.c). `map_domain_dump` returns `fd00:9:46::/48`, which is what AssertPlan compared.

### `vppctl show` after the tests (everything deleted, globals restored)
```
$ vppctl show nat44 addresses
NAT44 pool addresses:
NAT44 twice-nat pool addresses:
$ vppctl show nat44 static mappings
NAT44 static mappings:
$ vppctl show nat44 ei addresses
NAT44 pool addresses:
$ vppctl show nat64 bib all
NAT64 BIB entries:
$ vppctl show nat64 pool
NAT64 pool:
$ vppctl show nat66 static mappings
show nat66 static mappings: error plugin disabled
$ vppctl show det44 mappings
NAT44 deterministic mappings:
$ vppctl show det44 interfaces
DET44 interfaces:
$ vppctl show det44 timeouts
udp timeout: 300sec
tcp established timeout: 7440sec
tcp transitory timeout: 240sec
icmp timeout: 60sec
$ vppctl show map domain
$ vppctl show cnat translation
$ vppctl show cnat snat-policy
show cnat snat-policy: no default snat policy
$ vppctl show pnat translations
$ vppctl show pnat interfaces
```
det44 timeouts are back at the VPP defaults 300/7440/240/60, nat44 ED/EI and nat66 are disabled again (the tests
enabled them), map params were restored (logged "map params restored to VPP defaults"), and the cnat default SNAT
entry was removed ("no default snat policy"). det44 stays **enabled and idle**, by design (see the VPP bug below).

### Acceptance grep
```
$ grep -rn "vppctl\|exec.Command" internal/descriptors/{nat44ed,nat44ei,nat64,nat66,det44,mapnat,cnat,pnat,natcommon}
$ echo $?
1
```

### CI gate (`tools/ci.sh --base main`, log `/root/ngfw-wt/logs/DF-3-ci.log`)
```
== build ==
$ turbo run build
• turbo 2.11.2

== agent ==

CI GATE PASSED
exit=0
```

## VPP 26.06 bugs found (verified in /root/vpp source; details in DF-3-questions.md Q0/Q7/Q8, request V-items)
1. **det44 disable crashes VPP** (`det44_plugin_disable`: a pool is iterated as a vector, and an unformat function is
   used as a format). The DF-3 det44 test triggered it twice (2026-09-23 16:03, first worker; 2026-09-24 00:19, this
   worker's first run) and systemd restarted VPP each time. **Fix:** `det44.enable` never sends a disable, and the
   host test is opt-in (`VRX_DF3_DET44=1`, D-064). Also, a det44 interface delete re-enables the feature
   (`is_enable=1`).
2. **cnat:** `cnat_set_snat_policy` and `cnat_snat_policy_add_del_exclude_pfx` dereference NULL without a default
   SNAT entry, and `cnat_translation_update` with n_paths=0 underflows. Guarded, never triggered.
3. **pnat:** `pnat_flow_lookup` and `pnat_binding_detach` crash before the first attach (non-lazy bihash). Detach
   disables the interface's attachment point for all bindings. Details carry no binding index, which is recovered
   from the cursor semantics. Guarded, never triggered.
4. `nat44_ed_vrf_tables_v2_dump` answers with v1 details (Q4, worked around with a raw stream).

## Out of scope / not done
- **npt66 and dslite** are listed in the factory prompt but not in the envelope scope, so they were not built (Q5).
- No P05 wiring (the registry list lives in P05/main); every family exposes `Register`.
- Write-only (D-063, `ErrRetrieveUnsupported`): nat64/nat66/det44 enable, nat44-ei ipfix, cnat
  snat-policy/interface/exclude-prefix. cnat translation `flags`/`is_real_ip`/`flow_hash_config` are not modelled
  (not in the dump, Q6).
- No packet tests (F-nat44-ed-sessions owns them). Session dumps are asserted for shape only.
- `nat64_add_del_interface_addr` is not modelled (no dump; documented in nat64.md).

## Decisions (for the LOG)
- **D-DF3-1** det44: never disable the plugin; VRF change → `ErrVRFChangeUnsafe`. Options: (a) disable only when
  "safe" (impossible to observe), (b) never disable (chosen: a crash on a shared VPP outweighs an idle plugin),
  (c) drop det44.
- **D-DF3-2** crash guards in descriptors for cnat/pnat hazards (a guard call before the dangerous message) instead of
  dropping the object types.
- **D-DF3-3** pnat binding index recovered from `pnat_bindings_get` cursor semantics (O(1) extra call without pool
  holes, binary search otherwise) instead of keeping the index only in memory (not restart-safe).
- **D-DF3-4** map.interface key includes the mode (`map-e|map-t`), because VPP keeps two independent bitmaps.
- **D-DF3-5** cnat translation write-only fields are not modelled rather than cached (D-063).
- **D-DF3-6** scope follows the envelope (no npt66/dslite), per the precedence rule in 00-CONTEXT.
- **D-DF3-7** test-only evidence hook `nattest.Pause` (`VRX_EVIDENCE_DIR`), so descriptors/tests never exec `vppctl`.

## Open questions
See `docs/status/tasks/DF-3-questions.md`: Q0 (det44 crash, V-item), Q4 (vrf_tables_v2 quirk), Q5 (npt66/dslite),
Q6 (write-only objects), Q7/Q8 (cnat/pnat hazards, V-items), Q9 (`ErrRetrieveUnsupported` → alias to
`scheduler.ErrRetrieveUnsupported` when P05 merges), Q10 (D-064 NRestarts record).

---

## Review fixes (round 1): review `docs/status/tasks/DF-3-review.md` (8317a71), verdict BLOCK

Rules applied in this round: D-071 (globals owner, claim rule, re-verified deletes), D-069 (logical interface names via
DF-1's `iface.ResolveName`), D-076 (idempotent write-only re-application), D-064 (NRestarts around host runs;
`VRX_DF3_DET44` was never set). The semantics are written up once in `docs/agent/descriptors/nat-common.md`, which every
plugin doc links to.

| Finding | Fix | Commit(s) | Evidence |
|---|---|---|---|
| **H1** disable wipes other owners' objects; "enabled but empty" invisible | Plugin enable, timeouts, forwarding, IPFIX, MAP params and the cnat SNAT entry/policy are D-071 globals (`natcommon.Global`). Only `WithGlobalsOwner(true)` sets/resets them; every other owner only requires them (no set/reset/disable; Retrieve → `ErrRetrieveUnsupported`). The owner's disable runs only after `Plugin.Empty`, which counts **all owners and all object kinds** (ED: in/out and output interfaces, pool/twice-NAT addresses, interface-address pools, static/identity/LB mappings, VRF tables; EI, nat64 and nat66 likewise). Otherwise the disable is skipped. Owner Update (disable+enable) → `ErrNotEmpty` while objects exist. det44 is never disabled. ED↔EI → `ErrOtherVariant` | 9790a00 | unit `TestDisableNeedsCompleteEmptiness` (9 object kinds incl. the reviewer's foreign output interface), `TestEnableNonOwner`, `TestGlobalOwnership`, EI/nat64/nat66 equivalents; **host regression** in `TestNat44EdOnHost` (H1 lines below) |
| **H2** interface-bound static/identity mappings retrieved twice under one key | `dedupeByName` keeps the interface-bound (to-resolve) record and collapses multi-local identity details (ED+EI). The generic `Descriptor.Retrieve` fails with `ErrDuplicateKey`, and `nattest.Apply`/`AssertPlan` fail on duplicate keys too | 9790a00 | fakes now dump the twins like VPP; `TestMappings` asserts one `srv` key with `ExternalSwIfIndex 2`; host: `nat44-ed.static-mapping` 4 objects incl. `ifmap` and EI `eiif` converge under the strict check |
| **H3** pnat index recovery non-atomic; delete/attach by index unverified; duplicate tuples; cnat/map deletes by bare id | pnat: recovery accepted only when before/after `get(0)` snapshots are identical (retry, else `ErrUnstable`). `bindingAt` re-verifies live index + match + rewrite right before `pnat_binding_del` / `attach`, and `bindingIDAt` before detach. An attached binding is never deleted (`ErrAttached`). Duplicate tuples → `<id>#<index>` extras (`BindingSpec.Extra`). cnat translation Delete re-dumps and compares VIP/port/proto at the id. map domain/rule Delete re-checks the tag at the index; duplicate domain tags → `<name>#<index>` | 9790a00 | `TestRecoveryConcurrentDelete` (delete behind the recovery's back, stale-index delete not sent, duplicate extra deleted), `TestBindingAndAttachment` (attached binding refused), cnat `TestTranslation` (reused id not deleted), map `TestDeleteReverifies` |
| **M4** globals claimed by every slot; tests reset to defaults | Globals: see H1. Tests run as non-owners: plugins are fixtures (`nattest.EnsurePlugin`: enable if off; disable only if this test enabled it **and** the plugin is empty). Globals are checked as requirements, and the values before/after are asserted equal, so nothing is reset. The cnat SNAT entry is a fixture created only when absent | 9790a00, cd63b88 | host log: "globals required only … unchanged (D-071)", "fixture: … disabled again (previous state restored)" |
| **M5** production scope claims everything; Create accepts foreign interfaces | Claim rule: own tag → ours; any foreign tag → never; untagged (interfaces and untagged objects) → only via a `ClaimStore` record (`Item.NeedsClaim`; Create claims, Delete releases; slot ranges are a slot's standing claim). Create resolves interfaces with `ResolveOwned` = DF-1 `iface.ResolveName`, so foreign → `ErrForeignInterface`. Retrieve reports logical names (D-069) | 9790a00, 08d0af4 | `TestClaimRule` (foreign refused; `vrx` sees none of w9's objects; untagged NIC/pool only after claim), natcommon `TestScope`, `TestClaimsAndDuplicates` |
| **M6** write-only enables silently ignore a VRF change | nat66/det44 remember the VRFs this process enabled with. A differing Create → `nat66.ErrVRFChange` / `det44.ErrVRFChangeUnsafe`. Enabled before this process → unverifiable (documented) | 9790a00 | nat66/det44 unit tests ("vrf change via create") |
| **L7** pnat attachment Delete errors when nothing can be attached; interfaces_get single batch | Delete returns nil when no pnat interface exists (the crashing detach is still never sent). `pnatInterfaces` follows EAGAIN cursors | 9790a00 | `TestBindingAndAttachment` (fake fails if detach were sent) |
| **L8** cnat guards check-then-act across processes; policy Delete on a foreign entry | Host-wide flock `/run/lock/vrx-nat-cnat.lock`: shared around guard+policy/exclude, exclusive around entry create/delete. Policy/entry are globals (non-owner Delete = no-op) | 9790a00 | cnat `TestSnat` (non-owner never deletes the entry or resets the policy) |
| **L9** branch does not merge | `git merge main` twice; go.mod/go.sum are main's | abb2ece, 6e81620 | `git diff main -- apps/agent/go.mod apps/agent/go.sum` empty |
| D-069 / D-065 | interface refs stay `interface/<name>`; names are logical (owner-tag id / VPP name of untagged) | 08d0af4 | all unit + host tests |
| D-076 | only `cnat.snat-exclude-prefix` is non-idempotent in VPP (refcount per add) → claim record `<key>@vpp<main-thread PID>` skips re-adds on the same VPP process and re-adds once after a restart. The other write-only Creates are idempotent in VPP (see nat-common.md). The fake models the duplicate add | 08d0af4 | `TestExcludePrefixIdempotentAcrossResyncs` (3 resyncs → 1 instance; restart → re-added once) |

### Unit tests (`go test ./internal/descriptors/...`, DF-3 packages; log `/root/ngfw-wt/logs/DF-3-r1-unit.log`)
```
ok  	ngfw/agent/internal/descriptors/cnat	0.025s
ok  	ngfw/agent/internal/descriptors/det44	0.018s
ok  	ngfw/agent/internal/descriptors/mapnat	0.024s
ok  	ngfw/agent/internal/descriptors/nat44ed	0.040s
ok  	ngfw/agent/internal/descriptors/nat44ei	0.026s
ok  	ngfw/agent/internal/descriptors/nat64	0.016s
ok  	ngfw/agent/internal/descriptors/nat66	0.019s
ok  	ngfw/agent/internal/descriptors/natcommon	0.025s
ok  	ngfw/agent/internal/descriptors/pnat	0.041s

Finding tests (-v):
=== RUN   TestDisableNeedsCompleteEmptiness
=== RUN   TestDisableNeedsCompleteEmptiness/untagged_object_of_nobody
=== RUN   TestDisableNeedsCompleteEmptiness/foreign_in/out_interface
=== RUN   TestDisableNeedsCompleteEmptiness/foreign_output-feature_interface
=== RUN   TestDisableNeedsCompleteEmptiness/pool_address
=== RUN   TestDisableNeedsCompleteEmptiness/interface-address_pool
=== RUN   TestDisableNeedsCompleteEmptiness/static_mapping
=== RUN   TestDisableNeedsCompleteEmptiness/identity_mapping
=== RUN   TestDisableNeedsCompleteEmptiness/lb_mapping
=== RUN   TestDisableNeedsCompleteEmptiness/vrf_table
--- PASS: TestDisableNeedsCompleteEmptiness (0.01s)
=== RUN   TestEnableNonOwner
--- PASS: TestEnableNonOwner (0.00s)
=== RUN   TestMappings
--- PASS: TestMappings (0.01s)
=== RUN   TestClaimRule
--- PASS: TestClaimRule (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ed	0.050s
=== RUN   TestClaimsAndDuplicates
--- PASS: TestClaimsAndDuplicates (0.00s)
=== RUN   TestGlobalOwnership
--- PASS: TestGlobalOwnership (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/natcommon	0.020s
=== RUN   TestBindingAndAttachment
--- PASS: TestBindingAndAttachment (0.01s)
=== RUN   TestRecoveryConcurrentDelete
--- PASS: TestRecoveryConcurrentDelete (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/pnat	0.041s
=== RUN   TestTranslation
--- PASS: TestTranslation (0.00s)
=== RUN   TestExcludePrefixIdempotentAcrossResyncs
--- PASS: TestExcludePrefixIdempotentAcrossResyncs (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/cnat	0.030s
=== RUN   TestDeleteReverifies
--- PASS: TestDeleteReverifies (0.00s)
=== RUN   TestParamsNonOwner
--- PASS: TestParamsNonOwner (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/mapnat	0.030s
```

### Host integration, all packages, non-owner (log `/root/ngfw-wt/logs/DF-3-r1-integration.log`)
`VRX_INTEGRATION=1 VRX_TEST_PREFIX=w9 VRX_SLOT=9 VRX_VPP_TABLE_BASE=9000 go test -p 1 -count=1 -v …{nat44ed,nat44ei,nat64,nat66,det44,mapnat,cnat,pnat}/ -run OnHost`
(`VRX_DF3_DET44` unset, so det44 SKIP per D-064).
```
$ git log --oneline -1: 7529e46 docs(DF-3): nat-common.md (D-071 globals, claim rule, unique keys, identity re-verification, D-076, host lock, test model); plugin docs updated
restarts before: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
    nat44ed_integration_test.go:59: fixture: nat44-ed enabled for this test
    nat44ed_integration_test.go:73: nat44-ed.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat44ed_integration_test.go:92: globals required only: timeouts/forwarding/enable unchanged (D-071)
    nat44ed_integration_test.go:105: plan for nat44-ed.interface-feature after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:111: plan for nat44-ed.output-feature after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:115: plan for nat44-ed.interface-address after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:121: plan for nat44-ed.address-pool after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:131: plan for nat44-ed.static-mapping after re-apply: empty (4 objects converged)
    nat44ed_integration_test.go:135: plan for nat44-ed.identity-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:140: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:146: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:151: plan for nat44-ed.vrf-table after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:163: nat44-ed session users on host: 0 (asserted for shape only)
    nat44ed_integration_test.go:200: H1: w9b's output-feature interface loop907 present → globals-owner Delete skipped, nat44-ed still enabled
    fixture.go:62: fixture: nat44-ed disabled again (previous state restored)
--- PASS: TestNat44EdOnHost (0.42s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ed	0.449s
    nat44ei_integration_test.go:34: fixture: nat44-ei enabled for this test
    nat44ei_integration_test.go:68: nat44-ei.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat44ei_integration_test.go:87: plan for nat44-ei.interface-feature after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:91: plan for nat44-ei.interface-address after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:95: plan for nat44-ei.address-pool after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:100: plan for nat44-ei.static-mapping after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:104: plan for nat44-ei.identity-mapping after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:110: plan for nat44-ei.output-feature after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:116: nat44-ei users: 0 (shape only)
    nat44ei_integration_test.go:74: globals required only: timeouts/forwarding/ipfix unchanged (D-071)
    fixture.go:62: fixture: nat44-ei disabled again (previous state restored)
--- PASS: TestNat44EiOnHost (0.26s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ei	0.287s
    nat64_integration_test.go:23: fixture: nat64 enabled for this test
    nat64_integration_test.go:42: nat64.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat64_integration_test.go:65: plan for nat64.prefix after re-apply: empty (1 objects converged)
    nat64_integration_test.go:69: plan for nat64.pool after re-apply: empty (1 objects converged)
    nat64_integration_test.go:74: plan for nat64.interface after re-apply: empty (2 objects converged)
    nat64_integration_test.go:78: plan for nat64.static-bib after re-apply: empty (1 objects converged)
    nat64_integration_test.go:83: nat64 sessions: 0 (shape only)
    fixture.go:62: fixture: nat64 disabled again (previous state restored)
--- PASS: TestNat64OnHost (0.25s)
PASS
ok  	ngfw/agent/internal/descriptors/nat64	0.280s
    nat66_integration_test.go:22: fixture: nat66 enabled for this test
    nat66_integration_test.go:41: nat66.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat66_integration_test.go:48: plan for nat66.interface after re-apply: empty (2 objects converged)
    nat66_integration_test.go:52: plan for nat66.static-mapping after re-apply: empty (1 objects converged)
    fixture.go:62: fixture: nat66 disabled again (previous state restored)
--- PASS: TestNat66OnHost (0.14s)
PASS
ok  	ngfw/agent/internal/descriptors/nat66	0.163s
    det44_integration_test.go:22: det44 host test is opt-in (D-064): set VRX_DF3_DET44=1
--- SKIP: TestDet44OnHost (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/det44	0.018s
    mapnat_integration_test.go:29: plan for map.domain after re-apply: empty (1 objects converged)
    mapnat_integration_test.go:34: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:40: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:48: plan for map.interface after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:72: map params required only, unchanged (D-071)
--- PASS: TestMapOnHost (0.28s)
PASS
ok  	ngfw/agent/internal/descriptors/mapnat	0.312s
    cnat_integration_test.go:34: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:41: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:48: plan for cnat.interface-feature after re-apply: empty (1 objects converged)
    cnat_integration_test.go:53: cnat sessions: 0 (shape only)
    cnat_integration_test.go:103: cnat.snat-addresses is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:117: cnat.snat-interface: create re-applied twice without error (idempotent)
    cnat_integration_test.go:118: cnat.snat-interface is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:117: cnat.snat-exclude-prefix: create re-applied twice without error (idempotent)
    cnat_integration_test.go:118: cnat.snat-exclude-prefix is write-only: Retrieve → ErrRetrieveUnsupported
--- PASS: TestCnatOnHost (0.34s)
PASS
ok  	ngfw/agent/internal/descriptors/cnat	0.367s
    pnat_integration_test.go:27: plan for pnat.binding after re-apply: empty (2 objects converged)
    pnat_integration_test.go:36: plan for pnat.attachment after re-apply: empty (2 objects converged)
--- PASS: TestPnatOnHost (0.26s)
PASS
ok  	ngfw/agent/internal/descriptors/pnat	0.297s
exit=0
restarts after: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
```
`NRestarts` 2 → 2, ActiveEnterTimestamp unchanged. Per-package NRestarts checks were also taken around every first run of
the reworked tests during this round (all 2 → 2).

### `vppctl show` while the objects exist / after (read-only; logs `DF-3-r1-vppctl-during.txt`, `DF-3-r1-vppctl-after.txt`)
```
### [nat44ed] while the test's objects exist
$ vppctl show nat44 addresses
NAT44 pool addresses:
10.9.20.1
  tenant VRF independent
10.9.1.1
  tenant VRF: 0
10.9.1.2
  tenant VRF: 0
10.9.1.3
  tenant VRF: 0
10.9.1.4
  tenant VRF: 0
NAT44 twice-nat pool addresses:
10.9.2.1
  tenant VRF: 0
$ vppctl show nat44 static mappings
NAT44 static mappings:
 TCP local 10.9.10.50:80 external 10.9.1.1:8080 vrf 0  
 TCP local 10.9.10.51:443 external 10.9.1.2:8443 vrf 0 twice-nat 
 local 10.9.10.53 external 10.9.1.3 vrf 0  
 TCP local 10.9.10.52:22 external 10.9.20.1:2222 vrf 0  
 identity mapping UDP 10.9.1.4:500 vrf 0
 TCP external 10.9.1.4:80  
  local 10.9.10.60:8080 vrf 0 probability 50
  local 10.9.10.62:8080 vrf 0 probability 50
 TCP local 10.9.10.52:22 external loop902:2222 vrf 0
$ vppctl show nat44 interfaces
NAT44 interfaces:
 loop901 in
 loop902 out
 loop906 output-feature in out

### [nat44ei] while the test's objects exist
$ vppctl show nat44 ei addresses
NAT44 pool addresses:
10.9.40.1
  tenant VRF independent
  0 busy other ports
  0 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
10.9.3.1
  tenant VRF: 9002
  0 busy other ports
  0 busy udp ports
  1 busy tcp ports
  0 busy icmp ports
10.9.3.2
  tenant VRF: 9002
  0 busy other ports
  1 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
$ vppctl show nat44 ei static mappings
NAT44 static mappings:
 tcp local 10.9.30.50:80 external 10.9.3.1:8080 vrf 9002
 local 10.9.30.51 external 10.9.40.1 vrf 0
 identity mapping udp 10.9.3.2:500 vrf 0
 local 10.9.30.51 external loop904 vrf 0
$ vppctl show nat44 ei interfaces
NAT44 interfaces:
 loop903 in
 loop904 out
 loop905 output-feature in out

### [nat64] while the test's objects exist
$ vppctl show nat64 bib all
NAT64 BIB entries:
 fd00:9::64 80 10.9.64.1 8080 protocol tcp vrf 0 static 0 sessions
$ vppctl show nat64 pool
NAT64 pool:
 10.9.64.1 tenant VRF: 0
  0 busy other ports
  0 busy udp ports
  1 busy tcp ports
  0 busy icmp ports
 10.9.64.2 tenant VRF: 0
  0 busy other ports
  0 busy udp ports
  0 busy tcp ports
  0 busy icmp ports
$ vppctl show nat64 prefix
NAT64 prefix:
 fd00:9:40::/96 tenant-vrf 9010
$ vppctl show nat64 interfaces
NAT64 interfaces:
 loop910 in
 loop911 out

### [nat66] while the test's objects exist
$ vppctl show nat66 interfaces
NAT66 interfaces:
 loop912 in
 loop913 out
$ vppctl show nat66 static mappings
NAT66 static mappings:
 local fd00:9::66 external fd00:9::6600 vrf 0
  total pkts 0, total bytes 0

### [map] while the test's objects exist
$ vppctl show map domain
[0] tag {w9:w9-lw} ip4-pfx 10.9.46.0/24 ip6-pfx ::/48 ip6-src fd00:9::4601/128 ea-bits-len 0 psid-offset 6 psid-len 4 mtu 1460 prefix
 rule psid: 3 ip6-dst fd00:9::4603
 rule psid: 9 ip6-dst fd00:9::4699
$ vppctl show map domain index 0 counters
[0] tag {w9:w9-lw} ip4-pfx 10.9.46.0/24 ip6-pfx ::/48 ip6-src fd00:9::4601/128 ea-bits-len 0 psid-offset 6 psid-len 4 mtu 1460 prefix  TX: 0/0  RX: 0/0
 rule psid: 3 ip6-dst fd00:9::4603
 rule psid: 9 ip6-dst fd00:9::4699

### [cnat] while the test's objects exist
$ vppctl show cnat translation
[0] 10.9.47.1;53 UDP lb:maglev fhc:0x9f(default)
0.0.0.0;0->10.9.48.3;5353
0.0.0.0;0->10.9.48.4;5353
maglev backends map
[1] 10.9.47.1;80 TCP lb:default fhc:0x9f(default)
0.0.0.0;0->10.9.48.1;8080
0.0.0.0;0->10.9.48.2;8080
$ vppctl show cnat snat-policy
Source NAT
  ip4: 10.9.49.1;0
  ip6: fd00:9::4901;0

Excluded prefixes:
  Hash table 'snat prefixes'


Included v4 interfaces:
  loop941

Included v6 interfaces:

k8s pod interfaces:

k8s host interfaces:

### [pnat] while the test's objects exist
$ vppctl show pnat translations
[0] match: {*:*,TCP,10.9.52.2:80} rewrite: {10.9.53.2:*,*:* clear byte@[3]}
[1] match: {10.9.51.1:*,UDP,10.9.52.1:53} rewrite: {*:*,10.9.53.1:5353}
$ vppctl show pnat interfaces
sw_if_index: 16 output mask: DA DP
sw_if_index: 18 input mask: SA DA DP

----- after -----
$ vppctl show nat44 addresses
NAT44 pool addresses:
NAT44 twice-nat pool addresses:
$ vppctl show nat44 static mappings
NAT44 static mappings:
$ vppctl show nat44 interfaces
NAT44 interfaces:
$ vppctl show nat44 ei addresses
NAT44 pool addresses:
$ vppctl show nat64 bib all
NAT64 BIB entries:
$ vppctl show nat64 pool
NAT64 pool:
$ vppctl show nat66 static mappings
show nat66 static mappings: error plugin disabled
$ vppctl show map domain
$ vppctl show cnat translation
$ vppctl show cnat snat-policy
show cnat snat-policy: no default snat policy
$ vppctl show pnat translations
$ vppctl show pnat interfaces
```

### CI gate (`tools/ci.sh --base main`, log `/root/ngfw-wt/logs/DF-3-r1-ci.log`)
```
  mode quick · wall time 1m23s · logs /root/ngfw-wt/logs/ci/DF-3-20260924-012143-1290126

CI GATE PASSED
exit=0
```

### Decisions taken in this round (for the LOG)
- **D-DF3-8:** globals are implemented once in `natcommon.Global`. The non-owner "require" semantic is: observable →
  exact match; unobservable (nat64/nat66/det44 enabled with no objects yet, cnat policy, IPFIX domain/port) →
  accepted without touching VPP. Options were (a) accept, (b) fail with "unverifiable". (b) would deadlock every
  dependent of a mandatory enable dependency on a slot, so (a) was chosen.
- **D-DF3-9:** a plugin disable that is not allowed (not empty) is *skipped* (Delete returns nil, plugin left enabled),
  per D-071. An Update that needs disable+enable returns `ErrNotEmpty`, because it cannot be skipped silently.
- **D-DF3-10:** a test slot's address and table range is its standing claim for untagged objects (shared-host rules).
  Production owners own untagged objects only through the ClaimStore.
- **D-DF3-11:** duplicate VPP objects (pnat tuples, MAP domain tags) are reported as `#<index>` extras, following D-066.
  `#` is banned in MAP domain names.
- **D-DF3-12:** the H1 host regression uses a globals-owner instance *only* when the test itself enabled nat44-ed, and
  only while a foreign object exists, so it can never disable NAT that another slot relies on.

---

## Fix round 2: re-review `docs/status/tasks/DF-3-rereview.md` (56b39a9), verdict APPROVE WITH CHANGES

Main was merged first (contracts-v1 and P03b `vrx.model.nat.v1`; per D-078 the descriptors keep their own specs and the
build stays green). `VRX_DF3_DET44` was never set.

| Finding | Fix | Commit | Evidence |
|---|---|---|---|
| **N1** MAP host test compares uninitialised `map_param_get` fields | The test compares only the fields VPP fills (`modelled()` → `ParamsSpec`). `map.md` notes the 4 uninitialised reply fields | e2397fb | 10× runs below (plus 10/10 earlier in the round): 0 failures |
| **N2** excluded-prefix record outlives the default SNAT entry | Record `<key>@<entry identity>`: the D-080 boot identity (`natcommon.BootIdentity`: kernel boot_id, VPP main PID, `/proc/<pid>/stat` start time) + the entry fingerprint (addresses, interface) + an entry generation the owner bumps on every Set/Reset. A miss sends del+add (exactly one instance whatever VPP held). The superseded record is released (I2). Limitation: an external recreate with identical addresses has no VPP-observable identity (documented; D-071: only the owner mutates it) | e2397fb | unit `TestExcludePrefixIdempotentAcrossResyncs`; **host `TestCnatExcludeReaddOnHost`**: 3 resyncs → 1 add; entry deleted+recreated on the same VPP → re-added once; external recreate → re-added once; vppctl shows the prefix present |
| **N3** `dedupeByName` merges different mappings sharing a tag | `natcommon.DedupeTagged` merges only the same mapping (static: local ip/port/proto/vrf; identity: proto/port). Others become `<name>#<n>` extras and are deleted. `#` is rejected in desired names. The fake deletes by endpoint like VPP | e2397fb | unit `TestSameTagDifferentMappings` |
| **N4** fixture disable check-then-act | `nattest.EnsurePlugin` holds `/run/lock/vrx-nat-fixture-<plugin>.lock` shared for the test's lifetime and converts it to exclusive around `Empty` + disable | e2397fb | host log: "disabled again under the exclusive fixture lock" |

### Unit (`go test ./internal/descriptors/...`, DF-3 packages)
```
ok  	ngfw/agent/internal/descriptors/cnat	0.033s
ok  	ngfw/agent/internal/descriptors/det44	0.020s
ok  	ngfw/agent/internal/descriptors/mapnat	0.032s
ok  	ngfw/agent/internal/descriptors/nat44ed	0.039s
ok  	ngfw/agent/internal/descriptors/nat44ei	0.027s
ok  	ngfw/agent/internal/descriptors/nat64	0.028s
ok  	ngfw/agent/internal/descriptors/nat66	0.023s
ok  	ngfw/agent/internal/descriptors/natcommon	0.025s
?   	ngfw/agent/internal/descriptors/natcommon/nattest	[no test files]
ok  	ngfw/agent/internal/descriptors/pnat	0.032s
```

### Host, all packages as non-owner (log `/root/ngfw-wt/logs/DF-3-r2-integration.log`)
```
$ git log --oneline -1: e2397fb fix(DF-3): re-review round 2 — N1 map params compare modelled fields o
restarts before: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
    nat44ed_integration_test.go:59: fixture: nat44-ed enabled for this test
    nat44ed_integration_test.go:73: nat44-ed.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat44ed_integration_test.go:92: globals required only: timeouts/forwarding/enable unchanged (D-071)
    nat44ed_integration_test.go:105: plan for nat44-ed.interface-feature after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:111: plan for nat44-ed.output-feature after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:115: plan for nat44-ed.interface-address after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:121: plan for nat44-ed.address-pool after re-apply: empty (2 objects converged)
    nat44ed_integration_test.go:131: plan for nat44-ed.static-mapping after re-apply: empty (4 objects converged)
    nat44ed_integration_test.go:135: plan for nat44-ed.identity-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:140: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:146: plan for nat44-ed.lb-static-mapping after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:151: plan for nat44-ed.vrf-table after re-apply: empty (1 objects converged)
    nat44ed_integration_test.go:163: nat44-ed session users on host: 0 (asserted for shape only)
    nat44ed_integration_test.go:200: H1: w9b's output-feature interface loop907 present → globals-owner Delete skipped, nat44-ed still enabled
    fixture.go:90: fixture: nat44-ed disabled again under the exclusive fixture lock (previous state restored)
--- PASS: TestNat44EdOnHost (0.40s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ed	0.433s
    nat44ei_integration_test.go:34: fixture: nat44-ei enabled for this test
    nat44ei_integration_test.go:68: nat44-ei.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat44ei_integration_test.go:87: plan for nat44-ei.interface-feature after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:91: plan for nat44-ei.interface-address after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:95: plan for nat44-ei.address-pool after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:100: plan for nat44-ei.static-mapping after re-apply: empty (2 objects converged)
    nat44ei_integration_test.go:104: plan for nat44-ei.identity-mapping after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:110: plan for nat44-ei.output-feature after re-apply: empty (1 objects converged)
    nat44ei_integration_test.go:116: nat44-ei users: 0 (shape only)
    nat44ei_integration_test.go:74: globals required only: timeouts/forwarding/ipfix unchanged (D-071)
    fixture.go:90: fixture: nat44-ei disabled again under the exclusive fixture lock (previous state restored)
--- PASS: TestNat44EiOnHost (0.27s)
PASS
ok  	ngfw/agent/internal/descriptors/nat44ei	0.298s
    nat64_integration_test.go:23: fixture: nat64 enabled for this test
    nat64_integration_test.go:42: nat64.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat64_integration_test.go:65: plan for nat64.prefix after re-apply: empty (1 objects converged)
    nat64_integration_test.go:69: plan for nat64.pool after re-apply: empty (1 objects converged)
    nat64_integration_test.go:74: plan for nat64.interface after re-apply: empty (2 objects converged)
    nat64_integration_test.go:78: plan for nat64.static-bib after re-apply: empty (1 objects converged)
    nat64_integration_test.go:83: nat64 sessions: 0 (shape only)
    fixture.go:90: fixture: nat64 disabled again under the exclusive fixture lock (previous state restored)
--- PASS: TestNat64OnHost (0.26s)
PASS
ok  	ngfw/agent/internal/descriptors/nat64	0.276s
    nat66_integration_test.go:22: fixture: nat66 enabled for this test
    nat66_integration_test.go:41: nat66.enable is write-only: Retrieve → ErrRetrieveUnsupported
    nat66_integration_test.go:48: plan for nat66.interface after re-apply: empty (2 objects converged)
    nat66_integration_test.go:52: plan for nat66.static-mapping after re-apply: empty (1 objects converged)
    fixture.go:90: fixture: nat66 disabled again under the exclusive fixture lock (previous state restored)
--- PASS: TestNat66OnHost (0.13s)
PASS
ok  	ngfw/agent/internal/descriptors/nat66	0.158s
    det44_integration_test.go:22: det44 host test is opt-in (D-064): set VRX_DF3_DET44=1
--- SKIP: TestDet44OnHost (0.00s)
PASS
ok  	ngfw/agent/internal/descriptors/det44	0.025s
    mapnat_integration_test.go:29: plan for map.domain after re-apply: empty (1 objects converged)
    mapnat_integration_test.go:34: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:40: plan for map.rule after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:48: plan for map.interface after re-apply: empty (2 objects converged)
    mapnat_integration_test.go:73: map params required only, unchanged (D-071)
--- PASS: TestMapOnHost (0.26s)
PASS
ok  	ngfw/agent/internal/descriptors/mapnat	0.289s
    cnat_integration_test.go:37: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:44: plan for cnat.translation after re-apply: empty (2 objects converged)
    cnat_integration_test.go:51: plan for cnat.interface-feature after re-apply: empty (1 objects converged)
    cnat_integration_test.go:56: cnat sessions: 0 (shape only)
    cnat_integration_test.go:106: cnat.snat-addresses is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:120: cnat.snat-interface: create re-applied twice without error (idempotent)
    cnat_integration_test.go:121: cnat.snat-interface is write-only: Retrieve → ErrRetrieveUnsupported
    cnat_integration_test.go:120: cnat.snat-exclude-prefix: create re-applied twice without error (idempotent)
    cnat_integration_test.go:121: cnat.snat-exclude-prefix is write-only: Retrieve → ErrRetrieveUnsupported
--- PASS: TestCnatOnHost (0.25s)
    cnat_integration_test.go:196: N2: 3 resyncs → 1 add
    cnat_integration_test.go:208: N2: entry deleted+recreated (same VPP) → 3 resyncs → re-added once (adds 2)
    cnat_integration_test.go:230: N2: entry recreated externally with other addresses → re-added once (adds 3)
--- PASS: TestCnatExcludeReaddOnHost (0.24s)
PASS
ok  	ngfw/agent/internal/descriptors/cnat	0.527s
    pnat_integration_test.go:27: plan for pnat.binding after re-apply: empty (2 objects converged)
    pnat_integration_test.go:36: plan for pnat.attachment after re-apply: empty (2 objects converged)
--- PASS: TestPnatOnHost (0.36s)
PASS
ok  	ngfw/agent/internal/descriptors/pnat	0.379s
exit=0
restarts after: ActiveEnterTimestamp=Thu 2026-09-24 00:26:04 +0330 NRestarts=2 
```

### N1: TestMapOnHost ×10 (log `/root/ngfw-wt/logs/DF-3-r2-map10.log`)
```
$ go test -count=10 -v ./internal/descriptors/mapnat/ -run TestMapOnHost | grep -c '^--- PASS'
10
$ ... | grep -c '^--- FAIL'
0
```

### N2: `vppctl show cnat snat-policy` at the end of `TestCnatExcludeReaddOnHost` (after the external recreate)
```
### [cnat-n2] while the test's objects exist
$ vppctl show cnat snat-policy
Source NAT
  ip4: 10.9.49.2;0
  ip6: local0 (ip4);0

Excluded prefixes:
  Hash table 'snat prefixes'
[43]: heap offset 449162688, len 1, refcnt 1, linear 0
```
After the run: `show cnat snat-policy` → `no default snat policy`; no `loop9xx` left; NRestarts 2 → 2.

### CI (`tools/ci.sh --base main`, log `/root/ngfw-wt/logs/DF-3-r2-ci.log`)
```

CI GATE PASSED
exit=0
```

Decisions: **D-DF3-13**: the N2 entry identity combines the VPP-observable fingerprint with the owner's generation,
because VPP exposes no entry generation; a miss re-applies with del+add, which is exactly-once by VPP semantics.
**D-DF3-14**: DF-3 claim keys are semantic ids (never VPP indices), so the D-080 invalidation applies only to the
D-076 records.
