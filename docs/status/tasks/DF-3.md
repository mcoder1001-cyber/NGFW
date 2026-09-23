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
