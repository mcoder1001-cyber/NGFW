# DF-8 — Descriptors: dhcp, dns, flowprobe/ipfix, sflow, pcap/trace, lcp

Branch `task/DF-8` (worktree `/root/ngfw-wt/DF-8`, slot 5, prefix `w5`), base `main`, merged with main at 2869706
(DF-1 `iface`, DF-4 `acl`). Worker ran directly on the host.

## What was built

22 descriptors in 8 packages (after the review: `prom`/http_static removed, D-077/M5) under `apps/agent/internal/descriptors/` + the shared helper package `dfkit`
(structpb stand-in codec per D-055, interface resolution through DF-1's `iface`, D-071 globals role, D-076 boot-identity
store, errors) and `dfkit/dfkittest` (fake VPP with interface model and VPP identity, diff/empty-plan helpers, host
connection, loopbacks, slot-local globals lock). Object ↔ message tables: `docs/agent/descriptors/{dhcp,dns,ipfix,
flowprobe,sflow,pcap,trace,lcp}.md`.

| Package | Per-owner (`Register`) | VPP-global (`RegisterGlobals`, globals owner only, D-071) | Retrieve |
|---|---|---|---|
| dhcp | `dhcp.proxy`, `dhcp.proxy-vss`, `dhcp.client`, `dhcp.dhcp6-client`, `dhcp.dhcp6-pd-client`, `dhcp.dhcp6-pd-address` | `dhcp.dhcp6-duid` | dumps for proxy/vss/client; dhcp6 + DUID write-only (no dump in VPP) |
| dns | — | `dns.name-server`, `dns.enable` | write-only (no dump/getter) |
| ipfix | `ipfix.exporter`, `ipfix.classify-table` | `ipfix.default-exporter`, `ipfix.classify-stream` | exporter dumps; classify write-only (VPP msg-id bug) |
| flowprobe | `flowprobe.interface` | `flowprobe.params` | dump / getter |
| sflow | `sflow.interface` | `sflow.global` | dump + hw→sw learning/probe / 5 getters |
| pcap | `pcap.capture` | `pcap.filter-function` | write-only (no status message) |
| trace | — | `trace.bpf-filter` | write-only (no getter) |
| lcp | `lcp.itf-pair` | `lcp.default-netns` | `lcp_itf_pair_get` / getter |

Plus action/status helpers: `dhcp.WatchLeases` (dhcp_compl_event), `ClientDescriptor.Leases`,
`dhcp.WatchDHCP6Replies`/`WatchDHCP6PDReplies`, `dhcp.SendDHCP6(PD)ClientMessage`, `dns.ResolveName/ResolveIP`,
`lcp.ReplaceBegin/ReplaceEnd`, `lcp.Pairs`.

Rules applied from the manager's mid-task messages / LOG: D-063 (write-only = `ErrRetrieveUnsupported`, text equal to
P05's sentinel), D-064 (NRestarts checked before/after every first host run: stayed 2 throughout), D-065/D-069
(dependencies on `interface/<logical name>`, resolution via `iface.ResolveName`, `ErrForeignInterface` refused),
D-071 (globals owner + `RegisterGlobals`; claim rule via the shared `iface.Claims`), D-074 (deletes check existence;
index re-verified by logical name right before acting), D-076 (non-idempotent write-only adds — pcap capture,
http_static — keep a boot-identity record, `dfkit.BootStore`; fakes model duplicate adds), Retrieve dedupe, restart
simulation.

## How it was verified (first round, a1a34be — see "Review fixes" for the fix round)

### Unit tests (fake VPP, table-driven: create, idempotent re-apply, update/ErrRecreate, delete ×2, dependencies, Retrieve decoding incl. other owners filtered, VPP errors, globals owner vs non-owner, claims on untagged NICs, boot records across VPP restart)
```
$ cd apps/agent && go test -count=1 ./internal/descriptors/{dfkit/...,dhcp,dns,ipfix,flowprobe,sflow,prom,pcap,trace,lcp}/
ok  	ngfw/agent/internal/descriptors/dfkit	0.028s
?   	ngfw/agent/internal/descriptors/dfkit/dfkittest	[no test files]
ok  	ngfw/agent/internal/descriptors/dfkit/restarttest	0.022s
ok  	ngfw/agent/internal/descriptors/dhcp	0.035s
ok  	ngfw/agent/internal/descriptors/dns	0.036s
ok  	ngfw/agent/internal/descriptors/ipfix	0.028s
ok  	ngfw/agent/internal/descriptors/flowprobe	0.026s
ok  	ngfw/agent/internal/descriptors/sflow	0.036s
ok  	ngfw/agent/internal/descriptors/prom	0.029s
ok  	ngfw/agent/internal/descriptors/pcap	0.034s
ok  	ngfw/agent/internal/descriptors/trace	0.035s
ok  	ngfw/agent/internal/descriptors/lcp	0.026s
```

### Host integration (VRX_INTEGRATION=1, shared lab lock, slot 5, all packages in parallel as `go test ./...` runs them)
```
$ eval "$(tools/lab env 5)"; VRX_INTEGRATION=1 go test -count=1 ./internal/descriptors/{dfkit/...,dhcp,dns,ipfix,flowprobe,sflow,prom,pcap,trace,lcp}/
NRestarts before: 2
ok  	ngfw/agent/internal/descriptors/dfkit	0.024s
ok  	ngfw/agent/internal/descriptors/dfkit/restarttest	0.307s
    --- SKIP: TestDHCPOnHost/dhcp6_duid (0.00s)        # opt-in VRX_DF8_DUID=1: VPP-global without getter/reset
ok  	ngfw/agent/internal/descriptors/dhcp	2.391s
ok  	ngfw/agent/internal/descriptors/dns	2.293s
ok  	ngfw/agent/internal/descriptors/ipfix	0.084s
ok  	ngfw/agent/internal/descriptors/flowprobe	2.316s
ok  	ngfw/agent/internal/descriptors/sflow	0.140s
--- SKIP: TestHTTPStaticOnHost (0.01s)                  # opt-in VRX_DF8_HTTP_STATIC=1 (irreversible, Q3)
ok  	ngfw/agent/internal/descriptors/prom	0.025s
ok  	ngfw/agent/internal/descriptors/pcap	0.051s
ok  	ngfw/agent/internal/descriptors/trace	0.146s
ok  	ngfw/agent/internal/descriptors/lcp	0.288s
NRestarts after: 2
```
lcp is tested for real (D-060): `plugin linux_cp loaded: 4 message(s) compatible` → pair created, retrieved, deleted
(`skip-unless-plugin-loaded` via `CheckCompatiblity` remains in place for hosts without the plugin).

### Idempotency (re-applying the same desired state plans nothing) and VPP CLI while the objects exist / after delete
Captured with `VRX_DF8_EVIDENCE_HOLD=5s` (objects held while the CLI ran; test log lines abbreviated to `file:line`):
```
### [during] show dhcp proxy
    RX FIB       Src Address  Servers FIB,Address
     5801         10.5.80.2   0,10.5.80.1
### [during] show dhcpv6 proxy
     5801       fd00:5::80:2  0,fd00:5::80:1
### [during] show dhcp vss
 fib_table: 5801  oui: 658188 vpn_index: 77
### [during] show dhcp client verbose
[1] loop581 state DHCP_DISCOVER installed 0 dscp 10 no address
### [during] show dhcp6 clients
sw_if_index: 18
Retrieve dhcp.proxy/5801/0/10.5.80.1 = {"rx_vrf":5801,"server":"10.5.80.1","server_vrf":0,"src":"10.5.80.2"}
Retrieve dhcp.proxy-vss/ip4/5801 = {"family":"ip4","oui":0,"type":"ascii","vpn_ascii_id":"w5-vpn","vpn_index":0,"vrf":5801}
plan for dhcp.proxy after re-apply of 2 object(s):      (empty plan)
plan for dhcp.proxy-vss after re-apply of 1 object(s):  (empty plan)
Retrieve dhcp.client/loop581 = {"client_id":"w5-cid","dscp":10,"hostname":"w5-host","interface":"loop581",...} (meta {SwIfIndex:18})
plan for dhcp.client after re-apply of 1 object(s):     (empty plan)
### [after] show dhcp proxy / show dhcpv6 proxy / show dhcp vss / show dhcp client / show dhcp6 clients: empty

### [during] show dns servers
ip4 name servers:
10.5.53.1
ip6 name servers:
a05:3501::            # VPP CLI bug: prints the ip4 vector in the ip6 list (API state is fd00:5::53)
### [after] show dns servers: No name servers configured...

Retrieve ipfix.exporter/10.5.90.1 = {"collector":"10.5.90.1","collector_port":4739,"path_mtu":1400,"src":"10.5.90.2","template_interval":20,...}
plan for ipfix.exporter after re-apply of 1 object(s):  (empty plan)
Retrieve ipfix.default-exporter/global = {"collector":"10.5.91.1","collector_port":4739,"path_mtu":1400,...,"vrf":4294967295}
plan for ipfix.default-exporter after re-apply of 1 object(s): (empty plan)
(VPP 26.06 has no "show ipfix" CLI; AssertAbsent after delete passed)

### [during] show flowprobe params
 l3 l4 active: 10 passive: 60
### [during] show flowprobe feature
 loop583 ip4 rx tx
plan for flowprobe.params after re-apply of 1 object(s):    (empty plan)
plan for flowprobe.interface after re-apply of 1 object(s): (empty plan)
expected: flowprobe_set_params: flowprobe is enabled on some interface (VPP allows changes only while none is): VPPApiError: Unsupported (-126)
### [after] show flowprobe params: active: 15 passive: 120 (unset) · show flowprobe feature: empty

### [during] show sflow
sflow sampling-rate 1000
sflow direction both
sflow polling-interval 30
sflow header-bytes 192
sflow drop-monitoring enable
sflow enable loop584
  interfaces enabled: 1
Retrieve sflow.interface/loop584 = {"interface":"loop584"} (meta {SwIfIndex:18 HwIfIndex:17})
fresh descriptor (probe) meta {SwIfIndex:18 HwIfIndex:17}
plan for sflow.global after re-apply of 1 object(s):    (empty plan)
plan for sflow.interface after re-apply of 1 object(s): (empty plan)
### [after] show sflow: sampling-rate 10000 / direction rx / polling-interval 20 / header-bytes 128 / drop-monitoring disable / interfaces enabled: 0

### [during] show lcp
lcp default netns '<unset>'
itf-pair: [1] loop586 tap4096 w5-lcp0 330 type tap
pair loop586 ↔ w5-lcp0: {PhySwIfIndex:18 HostSwIfIndex:16 VifIndex:330}
Retrieve lcp.itf-pair/loop586 = {"host_if_name":"w5-lcp0","host_if_type":"tap","interface":"loop586","netns":""}
plan for lcp.itf-pair after re-apply of 1 object(s):    (empty plan)
Retrieve lcp.default-netns/global = {"netns":"ns-w5-lcp"}
plan for lcp.default-netns after re-apply of 1 object(s): (empty plan)
### [after] show lcp: default netns '<unset>', no itf-pair; `ip link show w5-lcp0`: Device "w5-lcp0" does not exist.

### [during] pcap trace status
pcap rx and tx dispatch capture enabled: 0 of 10 pkts...
capture to file /tmp/w5-df8.pcap
### [after] pcap trace status: pcap dispatch capture disabled

### [during] show bpf trace filter
(000) ldh      [12]
(001) jeq      #0x800           jt 2	jf 4
... (compiled "tcp and (port 179 or port 22)")
### [after] show bpf trace filter: bpf trace filter is not set
```

### Restart simulation (fresh connection + fresh descriptors → empty plan; simulated loss → re-create → empty plan)
```
$ VRX_INTEGRATION=1 go test -count=1 -v -run OnHost ./internal/descriptors/dfkit/restarttest/
    created loop593 sw_if_index 18 tag "w5r:loop593"   (loop594, loop595 likewise)
    == agent 1: apply the desired state
    plan dhcp.proxy              create dhcp.proxy/5901/0/10.5.95.1
    plan dhcp.client             create dhcp.client/loop593
    plan ipfix.exporter          create ipfix.exporter/10.5.96.1
    plan flowprobe.params        create flowprobe.params/global
    plan flowprobe.interface     create flowprobe.interface/loop594
    plan sflow.interface         create sflow.interface/loop594
    plan lcp.itf-pair            create lcp.itf-pair/loop595
    plan <all 7>                 (empty)
    == agent restart: fresh connection, fresh descriptors → empty plan
    plan dhcp.proxy             (empty)
    plan dhcp.client            (empty)
    plan ipfix.exporter         (empty)
    plan flowprobe.params       (empty)
    plan flowprobe.interface    (empty)
    plan sflow.interface        (empty)
    plan lcp.itf-pair           (empty)
    == simulated loss: delete the objects through the binary API
    == fresh agent after the loss: plan = re-create everything, apply, then empty plan
    plan dhcp.proxy              create dhcp.proxy/5901/0/10.5.95.1
    plan dhcp.client             create dhcp.client/loop593
    plan ipfix.exporter          create ipfix.exporter/10.5.96.1
    plan flowprobe.params        create flowprobe.params/global
    plan flowprobe.interface     create flowprobe.interface/loop594
    plan sflow.interface         create sflow.interface/loop594
    plan lcp.itf-pair            create lcp.itf-pair/loop595
    plan <all 7>                 (empty)
--- PASS: TestRestartSimulationOnHost (0.26s)
```
(The sflow probe path — a fresh descriptor that learned nothing finds the enabled interface — is shown above:
`fresh descriptor (probe) meta {SwIfIndex:18 HwIfIndex:17}`.)

### No VPP CLI / exec in the descriptor packages
```
$ grep -rn "vppctl\|exec.Command" internal/descriptors/{dfkit,dhcp,dns,ipfix,flowprobe,sflow,prom,pcap,trace,lcp}; echo "grep exit $?"
grep exit 1
```

### Leftovers after the runs
`show interface` has no `loop5xx`, `show lcp` no pair, `show dhcp proxy` empty, `show flowprobe feature` empty,
`show sflow` interfaces enabled: 0, `pcap trace status` disabled, bpf filter not set, no DNS servers, no `/tmp/w5*`.
NRestarts stayed 2 for every run (D-064).

### CI
```
$ tools/ci.sh --base main          # on 5c61b1b
== contract guard: HEAD vs main ==  no contract files changed
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: gitleaks — scanned ~401623 bytes (401.62 KB) in 1.05s no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total
== apps/agent: make lint test build ==
ok  ngfw/agent/internal/descriptors/dfkit · dfkit/restarttest · dhcp · dns · flowprobe · ipfix · lcp · pcap · prom · sflow · trace (+ all other agent packages)
== summary (quick) ==
  mode quick · wall time 1m04s · logs /root/ngfw-wt/logs/ci/DF-8-20260924-012547-1354318

CI GATE PASSED
```

## Out of scope / left undone
- **Missing in VPP 26.06 on this host** (Q1): tracedump (`trace_set_filters`, `trace_v2_dump`, …) and tracenode are
  not built — no binapi; Trace Path has no API; the prom exporter has no API. Dropped by D-077 (V18); the `prom`
  package (http_static) was removed in the fix round (M5).
- http_static full host run and the DHCPv6 DUID host run are opt-in (irreversible VPP-globals, Q3/Q4); both are covered
  by unit tests and a message-compatibility check on the host.
- `ipfix.classify-*` are write-only because of a VPP message-id bug (Q2); the decoder for the fixed dump is unit-tested.
- No registry wiring (P05/P08 call `Register`/`RegisterGlobals`), no API/UI/schema (per prompt).
- Persisted ClaimStore/BootStore: in-memory defaults; P05/P08 install persisted ones (`iface.SetClaimStore`,
  `dfkit.SetBootStore(owner, dfkit.NewFileBootStore(path))`).

## Open questions
See `docs/status/tasks/DF-8-questions.md` (Q1 missing plugins/binapi, Q2 VPP bugs for vpp-code-track, Q3 http_static
host run, Q4 DUID host run, Q5 dfkit package, Q6 sentinel alias, Q7 dhcp relay ownership).

## Decisions (for the LOG)
| id | decision | options | why |
|---|---|---|---|
| D-DF8-1 | Shared helpers in a new package `descriptors/dfkit` (+ `dfkittest`, `restarttest`) instead of nine copies | (a) dfkit (b) copy per package | one codec/resolver/globals/boot implementation; no existing file touched |
| D-DF8-2 | dns ordering reversed vs the prompt: `dns.enable` has no dependency, name servers are registered first | (a) prompt's arrow (b) reversed | VPP refuses enable with NO_NAME_SERVERS |
| D-DF8-3 | flowprobe depends on `ipfix.default-exporter` (exporter 0), not on additional exporters | — | flowprobe/node.c uses exporter 0 only |
| D-DF8-4 | `flowprobe.params` Update = ErrRecreate | (a) in-place set (b) recreate | VPP refuses set_params while any interface is enabled |
| D-DF8-5 | sflow hw_if_index gap: learn at Create, probe owned interfaces in Retrieve when unlearned indexes exist | (a) assume hw==sw (b) learn+probe (c) write-only | (a) wrong once sub-interfaces exist; (c) loses drift detection |
| D-DF8-6 | `ipfix.classify-stream/table` write-only; broken dumps never sent | (a) send and parse (b) write-only | VPP sends the details with the wrong message id |
| D-DF8-7 | `lcp_itf_pair_get` (v1) instead of v2 | — | v2 with ~0 answers with the v1 reply id |
| D-DF8-8 | pcap capture file is `/tmp/<name>` (VPP forces /tmp); tests use `/tmp/<prefix>-df8.pcap` and remove it | — | `unformat_vlib_tmpfile` rejects paths |
| D-DF8-9 | http_static: Update → ErrNotSupported, Delete no-op, full host run opt-in | — | no disable/reconfigure API; enabling is irreversible and turns the session layer on |
| D-DF8-10 | dhcp relay (`dhcp.proxy`, `-vss`) stays a per-owner object scoped by rx VRF (`WithVRFScope`), not a VPP-global | (a) per-VRF owner scope (b) globals-owner only | relays are per-VRF tables; the VRF range is the slot's ownership unit (README) — manager to confirm (Q7) |
| D-DF8-11 | Non-owner globals descriptors "require" (getter compare, else ErrNotGlobalsOwner) and are write-only for the reconciler | (a) refuse registration (b) require mode | D-071 allows requiring; RegisterGlobals is still owner-only |
| D-DF8-12 | Integration tests of one slot serialise VPP-global tests with `/run/vrx-test/<prefix>/df8-globals.lock`; the restart simulation uses its own owner (`w5r`) | — | `go test ./...` runs packages in parallel |

## Review fixes (review 463dc8a, APPROVE WITH CHANGES; main merged at 5e4b83c incl. DF-2 and contracts-v1)

| Finding | Fix | Commit | Tests |
|---|---|---|---|
| H1 claim before add / adopting foreign objects | `dfkit.Target`: resolve without claiming; claim only after VPP accepted the add; on "exists" adopt only with our tag or an existing claim (`ErrNotOurs` otherwise); Delete re-resolves and never touches an unclaimed untagged object (`ResolveForDelete`). lcp pair, dhcp client, dhcp6 ×3, flowprobe, sflow | 581b61e | unit: `TestClientForeignOnUntagged`, `TestInterfaceForeignOnUntagged` (sflow), flowprobe/lcp foreign cases, `TestTargetClaims`; host: lcp `foreign pair on an untagged interface (H1)` |
| H2 ipfix classify table by raw index | `ClassifyTable.Table` is DF-2's table **name**, key `ipfix.classify-table/<name>`, mandatory dep `classify.table/<name>`; index from `classify.LiveTables` (instance + geometry verified) right before every call; a gone table is a no-op delete | 581b61e | `TestClassifyWriteOnly`; host (opt-in) via DF-2 `classify.TableDescriptor` |
| M1 sflow map survives VPP restart / racy learning | map bound to the D-080 boot identity; learn only from an unambiguous dump difference, else by toggling our own interface | 581b61e | `TestInterfaceLearnReadOnlyRetrieve` (RestartVPP + reused hw index, concurrent enabler) |
| M2 sflow Retrieve toggles the data plane | Retrieve read-only; learning moved to Create (write path, own interface only) | 581b61e, 478bb48 | same + host `fresh descriptor (learned in Create)`; restart simulation shows the one re-learn |
| M3 host tests change VPP-wide settings | DNS, BPF filter, pcap filter function, IPFIX classify stream, lcp default netns: opt-in `VRX_DF8_GLOBALS=1`; lcp replace: opt-in `VRX_DF8_LCP_REPLACE=1`; read-first global tests under the lab-wide `/run/lock/vrx-globals.lock` (Q9) | 581b61e | host run below (SKIPs) |
| M4 in-memory BootStore orphans own capture | BootStore is an explicit `NewCapture`/`Register` argument (nil panics); `FileBootStore` writes+fsyncs before updating memory; records bound to the D-080 identity | 581b61e, 995e48e | `TestCapture` (lost store → documented busy), `TestFileBootStore`; host: restart with the persisted store recognises its own capture |
| M5 prom/http_static leftover (D-077) | package `prom` and `prom.md` removed; `ErrNotSupported` removed | 581b61e | — |
| M6 DHCPv4 lease events after restart/reconnect | event pid = agent PID + connection generation; stale pid = drift → recreate; `Reconnected()` hook for P05 | 581b61e | `TestClientEventSubscriptionPerConnection` |
| L1 pcap file | name must start with `<owner>-`; 0664 documented for F-capture-trace | 581b61e | `TestCapture` |
| L2 mandatory deps on globals-only keys | `flowprobe.params` and `ipfix.classify-stream` deps optional | 581b61e | flowprobe/ipfix dependency asserts |
| L3 untagged lcp host tap | left untagged on purpose (tagging makes DF-1 delete the tap) → Q10/P12 | — | — |
| L4 dfkit/df2 overlap | follow-up (Q10); FileBootStore write-before-memory + fsync done | 995e48e | `TestFileBootStore` |
| L5 dropped types in docs | trace.md/questions updated, prom docs gone, Q8 listed | a581278 | — |
| L6 dhcp proxy src flip-flop | second `src` for a relaying rx VRF/family refused | 581b61e | `TestProxyRefusesSecondSrc` |
| L7 PID identity | **D-080** implemented: `dfkit.BootIdentity` = kernel boot_id / VPP main PID / start time (`/proc/<pid>/stat` field 22); claims (holder qualified with identity + sw_if_index), boot records and the sflow map expire on change | 581b61e | `TestBootIdentity`, `TestTargetClaims` |
| (new) lcp default netns getter garbage | VPP returns uninitialised bytes when unset; invalid names = unset (VPP bug 5, Q2) | 478bb48 | host lcp run |

### Unit tests (fix round)
```
$ go test -count=1 -v -run 'Foreign|EventSubscription|LearnReadOnly|SecondSrc|ClassifyWriteOnly|TestCapture|TargetClaims|BootIdentity|FileBootStore' ./internal/descriptors/{dfkit,dhcp,flowprobe,sflow,lcp,ipfix,pcap}/
--- PASS: TestBootIdentity (0.00s)
--- PASS: TestFileBootStore (0.00s)
--- PASS: TestTargetClaims (0.00s)
--- PASS: TestClientRefusesForeignInterfaces (0.00s)
--- PASS: TestClientForeignOnUntagged (0.00s)
--- PASS: TestClientEventSubscriptionPerConnection (0.00s)
--- PASS: TestProxyRefusesSecondSrc (0.00s)
--- PASS: TestInterfaceLearnReadOnlyRetrieve (0.00s)
--- PASS: TestInterfaceForeignOnUntagged (0.00s)
--- PASS: TestClassifyWriteOnly (0.00s)
--- PASS: TestCapture (0.00s)
(flowprobe/lcp foreign cases run inside TestInterface / TestItfPair)
$ go test -count=1 ./...   → all ok
```

### Host integration (fix round; all packages in parallel; opt-ins unset)
```
NRestarts before: 2
ok  	ngfw/agent/internal/descriptors/dfkit	0.018s
ok  	ngfw/agent/internal/descriptors/dfkit/restarttest	0.397s
    --- SKIP: TestDHCPOnHost/dhcp6_duid (0.00s)
ok  	ngfw/agent/internal/descriptors/dhcp	0.759s
--- SKIP: TestDNSOnHost (0.00s)                    # opt-in VRX_DF8_GLOBALS=1
ok  	ngfw/agent/internal/descriptors/dns	0.022s
--- SKIP: TestIPFIXClassifyOnHost (0.00s)          # opt-in VRX_DF8_GLOBALS=1
ok  	ngfw/agent/internal/descriptors/ipfix	0.038s
ok  	ngfw/agent/internal/descriptors/flowprobe	0.469s
ok  	ngfw/agent/internal/descriptors/sflow	0.446s
--- SKIP: TestPcapFilterFunctionOnHost (0.00s)     # opt-in VRX_DF8_GLOBALS=1
ok  	ngfw/agent/internal/descriptors/pcap	0.051s
--- SKIP: TestBPFFilterOnHost (0.00s)              # opt-in VRX_DF8_GLOBALS=1
ok  	ngfw/agent/internal/descriptors/trace	0.020s
    --- SKIP: TestLCPOnHost/default-netns (0.00s)  # opt-in VRX_DF8_GLOBALS=1
    --- SKIP: TestLCPOnHost/replace_helpers (0.00s) # opt-in VRX_DF8_LCP_REPLACE=1
ok  	ngfw/agent/internal/descriptors/lcp	0.702s
NRestarts after: 2
    integration_test.go:87: refused as expected: {Interface:loop597 HostIfName:w5-lcp9 HostIfType:tap Netns:}
    integration_test.go:87: refused as expected: {Interface:loop597 HostIfName:w5-for0 HostIfType:tap Netns:}
    integration_test.go:105: foreign pair on loop597 kept: not adopted, not reported, not deleted
    integration_test.go:54: agent restart with the persisted boot store: own capture recognised, not re-added
    integration_test.go:63: fresh descriptor (learned in Create) meta {SwIfIndex:15 HwIfIndex:20}
```
Leftovers afterwards: no `loop5xx`, no lcp pair, sflow interfaces enabled 0, flowprobe feature empty, pcap disabled,
no `/tmp/w5*`, no `w5-*` netdev.

### Restart simulation (fix round)
```
    150: created loop593 sw_if_index 15 tag "w5r:loop593"
    150: created loop594 sw_if_index 14 tag "w5r:loop594"
    150: created loop595 sw_if_index 9 tag "w5r:loop595"
    153: == agent 1: apply the desired state
    156: plan dhcp.proxy              create dhcp.proxy/5901/0/10.5.95.1
    156: plan dhcp.client             create dhcp.client/loop593
    156: plan ipfix.exporter          create ipfix.exporter/10.5.96.1
    156: plan flowprobe.params        create flowprobe.params/global
    156: plan flowprobe.interface     create flowprobe.interface/loop594
    156: plan sflow.interface         create sflow.interface/loop594
    156: plan lcp.itf-pair            create lcp.itf-pair/loop595
    159: plan dhcp.proxy             (empty)
    159: plan dhcp.client            (empty)
    159: plan ipfix.exporter         (empty)
    159: plan flowprobe.params       (empty)
    159: plan flowprobe.interface    (empty)
    159: plan sflow.interface        (empty)
    159: plan lcp.itf-pair           (empty)
    163: == agent restart: fresh connection, fresh descriptors, fresh in-memory claim store
    170: plan dhcp.proxy             (empty)
    170: plan dhcp.client            (empty)
    170: plan ipfix.exporter         (empty)
    170: plan flowprobe.params       (empty)
    170: plan flowprobe.interface    (empty)
    170: plan sflow.interface         create sflow.interface/loop594
    170: plan lcp.itf-pair           (empty)
    173: plan dhcp.proxy             (empty)
    173: plan dhcp.client            (empty)
    173: plan ipfix.exporter         (empty)
    173: plan flowprobe.params       (empty)
    173: plan flowprobe.interface    (empty)
    173: plan sflow.interface        (empty)
    173: plan lcp.itf-pair           (empty)
    177: == simulated loss: delete the objects through the binary API
    180: == fresh agent after the loss: plan = re-create everything, apply, then empty plan
    186: plan dhcp.proxy              create dhcp.proxy/5901/0/10.5.95.1
    186: plan dhcp.client             create dhcp.client/loop593
    186: plan ipfix.exporter          create ipfix.exporter/10.5.96.1
    186: plan flowprobe.params        create flowprobe.params/global
    186: plan flowprobe.interface     create flowprobe.interface/loop594
    186: plan sflow.interface         create sflow.interface/loop594
    186: plan lcp.itf-pair            create lcp.itf-pair/loop595
    190: plan dhcp.proxy             (empty)
    190: plan dhcp.client            (empty)
    190: plan ipfix.exporter         (empty)
    190: plan flowprobe.params       (empty)
    190: plan flowprobe.interface    (empty)
    190: plan sflow.interface        (empty)
    190: plan lcp.itf-pair           (empty)
```

### CI (fix round)
```
$ tools/ci.sh --base main
  apps/agent: make lint test build                   0m29s
  mode quick · wall time 1m38s · logs /root/ngfw-wt/logs/ci/DF-8-20260924-015450-1678634

CI GATE PASSED
```

### Decisions added in the fix round
| id | decision | why |
|---|---|---|
| D-DF8-13 | Claims are recorded after a successful add and qualified with the D-080 identity + sw_if_index; existing objects on untagged interfaces are adopted only with our claim | review H1, D-080 |
| D-DF8-14 | sflow learns hw→sw only in Create (dump difference or own-interface toggle); Retrieve read-only; after an agent restart one re-create per sflow interface re-learns | review M2 (the reviewer's "disable, dump, re-enable" option) |
| D-DF8-15 | lcp host tap stays untagged (tagging lets DF-1 delete it) | review L3 → P12 |
| D-DF8-16 | getter-less VPP-global host tests opt-in; read-first ones under `/run/lock/vrx-globals.lock` | review M3, Q9 |
