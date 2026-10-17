# agent-allifs-unbound-nics — status

PR: https://github.com/mcoder1001-cyber/NGFW/pull/241 · branch `claude/agent-allifs-unbound-nics` · base `origin/main` `99a774734` · fixes finding #1 of review R4 of [PR #238](https://github.com/mcoder1001-cyber/NGFW/pull/238) ([review file](https://github.com/mcoder1001-cyber/NGFW/blob/c2f898d2b/docs/status/tasks/boot-nic-registration-port-review-R4.md), [tech-debt row dated 2026-10-17 "PR #238 review R4, MAJOR"](https://github.com/mcoder1001-cyber/NGFW/blob/c2f898d2b/docs/tech-debt.md), both on the #238 branch only; `main` carries neither).
Product commits: `9a63c09ce` (the fix) and `856bf743a` (review changes); the other commits are docs/status. Agent only; no schema/proto/API change; no VPP, lab slot or host service touched.

## What was built

- `apps/agent/internal/desired/acl.go`: `desired.ACL` takes the interface rows the projection keeps (`kept`, from `SplitUnboundPhysical`); nil is the split without a netdev lookup (every engine-owned row kept, released rows dropped). The 15 test callers pass nil.
- `apps/agent/internal/desired/global_blocking.go`: the implicit all-interfaces target set of the block lists — `allInterfaces` lists and the auto-block overlay, which always selects all interfaces — is `sortedKeys(kept)`, computed once per projection and only when a list asks for it. A list that selects all interfaces warns once (`acl.global-blocking` at `/acl/globalBlocking/lists/<name>/interfaces`), naming the rows it skips ("not bound to the data plane yet: …; released to the host: …"), so an acl-only transaction never drops a binding silently. Explicit interface lists are unchanged.
- `apps/agent/internal/agent/projection.go`: passes `boundIfs` — the result of the one `SplitUnboundPhysical` call per projection — so the interfaces and acl domains see one observation of the kernel and the acl projection adds no netlink dumps.
- Why: an unbound seeded NIC (still a Linux kernel interface) or a NIC released to the host has no `interface/<row>` alias (never projected), so `acl.interface-binding/<row>` failed the plan with a missing mandatory dependency: every interfaces/security commit was refused and `AutoBlockSet` returned `Unavailable` (new automatic blocks enforced nowhere, VPP and host nftables alike) while an all-interfaces list or live auto-block entries existed. Pre-existing on `main`; #238's boot-time registration makes it reachable on every appliance with kernel NICs.

## How verified (pasted output, unedited)

Red: the committed head with one line of `global_blocking.go` put back to the pre-fix form (`allIfs = sortedKeys(ds.GetInterfaces())` instead of `sortedKeys(kept)`, by `sed`, then `git checkout -- apps/agent/internal/desired/global_blocking.go`); run from `apps/agent`:

```
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake' ./internal/desired/ ./internal/agent/
--- FAIL: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
    global_blocking_test.go:266: a released NIC is not an all-interfaces target
--- FAIL: TestGlobalBlockingAllInterfacesNothingBound (0.01s)
    global_blocking_test.go:303: want the skipped-NICs warning and the not-enforced warning, got [W /acl/globalBlocking/lists/all/interfaces acl.global-blocking block list "all" selects all interfaces; the physical NICs the data plane does not have are skipped (not bound to the data plane yet: ens192; released to the host: ens161)]
FAIL
FAIL	ngfw/agent/internal/desired	0.182s
--- FAIL: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.02s)
    physical_nics_acl_test.go:108: no binding for ens192: its interface alias is never projected
2026/10/17 11:53:14 INFO VPP VRRP engine off (NGFW_VRRP_VPP=off, or unset without NGFW_VPP_ID_RANGE=all): ha.vrrp engine=vpp instances are reported unapplied, never written to VPP
2026/10/17 11:53:14 INFO keepalived engine off (NGFW_KEEPALIVED=off, or neither a lab slot prefix nor NGFW_VPP_ID_RANGE=all): ha.vrrp engine=keepalived instances are reported unapplied, never staged
2026/10/17 11:53:14 INFO PKI file materialiser wired root=/tmp/TestAutoBlockWithUnboundAndReleasedNICsOnFake785941171/001/pki-w7 source=true
2026/10/17 11:53:14 WARN MPLS: no VPP id range; this agent creates no MPLS table err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO not the globals owner: lldp.global and nsim are not registered; services.lldp globals and services.nsim are reported as unsupported (D-071)
2026/10/17 11:53:14 WARN neighbors: no VPP id range — proxy-ARP ranges (vrfs.<vrf>.proxyArpRanges) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 WARN rpf-adl-pbr: no VPP id range — ABF policies (routing.pbr) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO services.autoSdl is not applied by this agent: auto_sdl_config is a VPP-global and this agent is not the globals owner (D-071)
2026/10/17 11:53:14 INFO fqdn state reloaded family=objects file=/tmp/TestAutoBlockWithUnboundAndReleasedNICsOnFake785941171/001/objects-fqdn-w7.json hosts=0 fresh=0 due=0 dormant=0 due_spread_over=30s
2026/10/17 11:53:14 INFO objects domain wired store=/tmp/TestAutoBlockWithUnboundAndReleasedNICsOnFake785941171/001/objects-w7.json fqdn_objects=0 fqdn_refresh=1m0s fqdn_max_stale=24h0m0s dns=127.0.0.1:9
2026/10/17 11:53:14 INFO acl domain wired family=acl stats=/run/vpp/stats.sock reproject_every=1m0s globals_owner=false resync_hook=true
2026/10/17 11:53:14 INFO host firewall wired table="inet ngfw_w7" mode=check netns="" rules_file=/tmp/TestAutoBlockWithUnboundAndReleasedNICsOnFake785941171/001/host-acl-w7.nft
2026/10/17 11:53:14 WARN DHCP relay owns no VRF: the agent has no VPP id range err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO kea descriptors off (NGFW_KEA_MODE=off)
2026/10/17 11:53:14 INFO host services wired feature=unbound-chrony-syslog product_paths=false unbound_conf=/tmp/ngfw-agent-test-1301843099/unbound/unbound.conf chrony_conf=/tmp/ngfw-agent-test-1301843099/chrony/agent/chrony.conf rsyslog_conf=/tmp/ngfw-agent-test-1301843099/rsyslog/rsyslog.conf vpp_dns_cache="required only"
2026/10/17 11:53:14 WARN qos: no VPP id range — egress maps (services.qos.maps) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO system identity wired product_paths=false hostname_file=/tmp/ngfw-agent-test-1301843099/sysident/etc/hostname
2026/10/17 11:53:14 INFO pppoe client wired feature=pppoe-client globals_owner=false product_paths=false
2026/10/17 11:53:14 WARN remote-access initialization unavailable reason=engine-not-ready
2026/10/17 11:53:14 INFO VPP boot identity identity=6e5470e2-2a93-49f1-9a7e-91932d253fe0/0/? complete=false expired_claims=0
2026/10/17 11:53:14 INFO reconcile start txn_id=nics-config mode=apply domains="[interfaces security]"
2026/10/17 11:53:14 INFO reconcile done txn_id=nics-config mode=apply domains="[interfaces security]" status=APPLY_STATUS_APPLIED summary="created:1  unchanged:1" reapplied=0 duration=8.916893ms err=""
2026/10/17 11:53:14 INFO reconcile start txn_id="" mode=auto-block domains=[acl]
2026/10/17 11:53:14 INFO reconcile done txn_id="" mode=auto-block domains=[acl] status=APPLY_STATUS_FAILED summary=failed:2 reapplied=0 duration=0s err="validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present"
--- FAIL: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.03s)
    physical_nics_acl_test.go:165: auto-block set with an unbound and a released NIC: <nil> rpc error: code = Unavailable desc = auto-block enforcement: validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present
2026/10/17 11:53:14 INFO VPP VRRP engine off (NGFW_VRRP_VPP=off, or unset without NGFW_VPP_ID_RANGE=all): ha.vrrp engine=vpp instances are reported unapplied, never written to VPP
2026/10/17 11:53:14 INFO keepalived engine off (NGFW_KEEPALIVED=off, or neither a lab slot prefix nor NGFW_VPP_ID_RANGE=all): ha.vrrp engine=keepalived instances are reported unapplied, never staged
2026/10/17 11:53:14 INFO PKI file materialiser wired root=/tmp/TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake3372876071/001/pki-w7 source=true
2026/10/17 11:53:14 WARN MPLS: no VPP id range; this agent creates no MPLS table err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO not the globals owner: lldp.global and nsim are not registered; services.lldp globals and services.nsim are reported as unsupported (D-071)
2026/10/17 11:53:14 WARN neighbors: no VPP id range — proxy-ARP ranges (vrfs.<vrf>.proxyArpRanges) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 WARN rpf-adl-pbr: no VPP id range — ABF policies (routing.pbr) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO services.autoSdl is not applied by this agent: auto_sdl_config is a VPP-global and this agent is not the globals owner (D-071)
2026/10/17 11:53:14 INFO fqdn state reloaded family=objects file=/tmp/TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake3372876071/001/objects-fqdn-w7.json hosts=0 fresh=0 due=0 dormant=0 due_spread_over=30s
2026/10/17 11:53:14 INFO objects domain wired store=/tmp/TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake3372876071/001/objects-w7.json fqdn_objects=0 fqdn_refresh=1m0s fqdn_max_stale=24h0m0s dns=127.0.0.1:9
2026/10/17 11:53:14 INFO acl domain wired family=acl stats=/run/vpp/stats.sock reproject_every=1m0s globals_owner=false resync_hook=true
2026/10/17 11:53:14 INFO host firewall wired table="inet ngfw_w7" mode=check netns="" rules_file=/tmp/TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake3372876071/001/host-acl-w7.nft
2026/10/17 11:53:14 WARN DHCP relay owns no VRF: the agent has no VPP id range err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO kea descriptors off (NGFW_KEA_MODE=off)
2026/10/17 11:53:14 INFO host services wired feature=unbound-chrony-syslog product_paths=false unbound_conf=/tmp/ngfw-agent-test-1301843099/unbound/unbound.conf chrony_conf=/tmp/ngfw-agent-test-1301843099/chrony/agent/chrony.conf rsyslog_conf=/tmp/ngfw-agent-test-1301843099/rsyslog/rsyslog.conf vpp_dns_cache="required only"
2026/10/17 11:53:14 WARN qos: no VPP id range — egress maps (services.qos.maps) cannot be created err="no VPP id range: set NGFW_VPP_TABLE_BASE (a test slot, or a reserved range on a shared host: docs/lab/shared-host-rules.md §12) or NGFW_VPP_ID_RANGE=all (the product agent on a box of its own)"
2026/10/17 11:53:14 INFO system identity wired product_paths=false hostname_file=/tmp/ngfw-agent-test-1301843099/sysident/etc/hostname
2026/10/17 11:53:14 INFO pppoe client wired feature=pppoe-client globals_owner=false product_paths=false
2026/10/17 11:53:14 WARN remote-access initialization unavailable reason=engine-not-ready
2026/10/17 11:53:14 INFO VPP boot identity identity=6e5470e2-2a93-49f1-9a7e-91932d253fe0/0/? complete=false expired_claims=0
2026/10/17 11:53:14 INFO reconcile start txn_id=all1 mode=apply domains="[interfaces acl]"
2026/10/17 11:53:14 INFO reconcile done txn_id=all1 mode=apply domains="[interfaces acl]" status=APPLY_STATUS_FAILED summary=failed:2 reapplied=0 duration=6.014537ms err="validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present"
--- FAIL: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.01s)
    physical_nics_acl_test.go:226: status APPLY_STATUS_FAILED, want APPLY_STATUS_APPLIED: validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present results=[key:"acl.interface-binding/ens161"  code:OBJECT_RESULT_CODE_DEPENDENCY_MISSING  message:"mandatory dependency interface/ens161 is neither desired nor present"  pointer:"/acl/globalBlocking/lists/bad"  subsystem:"acl" key:"acl.interface-binding/ens192"  code:OBJECT_RESULT_CODE_DEPENDENCY_MISSING  message:"mandatory dependency interface/ens192 is neither desired nor present"  pointer:"/acl/globalBlocking/lists/bad"  subsystem:"acl"] validation=txn_id:"all1"  errors:{pointer:"/acl/globalBlocking/lists/bad"  message:"acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present"  severity:ISSUE_SEVERITY_ERROR  rule:"agent.dependency-missing"}  errors:{pointer:"/acl/globalBlocking/lists/bad"  message:"acl.interface-binding/ens192: mandatory dependency interface/ens192 is neither desired nor present"  severity:ISSUE_SEVERITY_ERROR  rule:"agent.dependency-missing"}  errors:{pointer:"/acl/globalBlocking/lists/bad/interfaces"  message:"block list \"bad\" selects all interfaces; the physical NICs the data plane does not have are skipped (not bound to the data plane yet: ens192; released to the host: ens161)"  severity:ISSUE_SEVERITY_WARNING  rule:"acl.global-blocking"}  errors:{pointer:"/interfaces/ens161"  message:"physical NIC \"ens161\" is released to the host (physical.owner = host): nothing is configured on the data plane for it"  severity:ISSUE_SEVERITY_INFO  rule:"agent.nic-released"}  errors:{pointer:"/interfaces/ens192"  message:"physical NIC \"ens192\" is not bound to the data plane yet (still a Linux kernel interface); its configuration is applied once the NIC is handed to VPP"  severity:ISSUE_SEVERITY_WARNING  rule:"agent.nic-not-bound"}  summary:{}
FAIL
FAIL	ngfw/agent/internal/agent	0.257s
FAIL
exit=1
```

Green, the committed head (the six tests above plus the neighbouring global-blocking and physical-NIC tests):

```
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake|TestGlobalBlocking|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	4.990s
ok  	ngfw/agent/internal/agent	0.398s
exit=0
```

What the six tests prove: `TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs` (desired) — an all-interfaces list plus an explicit list over bound, unbound and released rows binds the kept rows only, the unbound NIC only through the explicit list, one warning names both skipped rows, and without a lookup the released row is still dropped; `TestGlobalBlockingAllInterfacesNothingBound` — every row unbound or released: nothing bound, no error, the list reported as not enforced; `TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs` (agent) — the auto-block overlay binds the bound NIC only, keeps the nic-not-bound / nic-released notes and names the skipped rows at the overlay list; `TestProjectAutoBlockOverlayAddsNoNetdevLookups` — one lookup per engine-owned row, none for a released one, none added by the acl projection; `TestAutoBlockWithUnboundAndReleasedNICsOnFake` (fake VPP) — `AutoBlockSet` succeeds and the bound NIC carries `_gb.auto-block.i00`, an interfaces commit with live entries has no DryRun error and is APPLIED (the new interface bound too), a NIC handed to VPP later is bound by the resync together with its own configuration, a second resync changes nothing, and a released NIC loses our binding and goes admin-down; `TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake` — a user all-interfaces list: the bound NIC and the loopback bound, `Retrieve` equals desired, re-apply and resync empty, a later bind picked up.

Agent gate on the committed head `0b57dc070` (product commit `856bf743a`): `env -u NGFW_INTEGRATION make -C apps/agent lint test`, run from the worktree root, nothing committed while it ran; log `agent-gate-2.log` (session scratchpad). Pasted unedited; the `ok` lines of the other packages are elided as marked:

```
make: Entering directory '/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19/apps/agent'
NGFW_TIMING BEGIN agent.vet 2026-10-17T11:54:27.898005760Z
go vet ./...
NGFW_TIMING END agent.vet 2026-10-17T11:54:35.932604774Z
NGFW_TIMING BEGIN agent.lint-dispatch 2026-10-17T11:54:35.957683557Z
0 issues.
NGFW_TIMING END agent.lint-dispatch 2026-10-17T11:54:47.867627112Z
NGFW_TIMING BEGIN agent.race-test 2026-10-17T11:54:47.892439377Z
go test -race -count=1 ./...
ok  	ngfw/agent/internal/agent	58.388s
ok  	ngfw/agent/internal/desired	28.072s
ok  	ngfw/agent/internal/subsystems	31.805s
# 140 further "ok" lines elided — grep -c '^ok' agent-gate-2.log → 143; grep -c -E '^(FAIL|--- FAIL)' → 0
NGFW_TIMING END agent.race-test 2026-10-17T11:56:32.851502536Z
AGENT_GATE_EXIT=0
```

Complete `tools/ci.sh quick` (unchanged, from the worktree root, `env -u NGFW_INTEGRATION`) on the same head `0b57dc070`, started 11:57:04Z, nothing committed while it ran; log `ci-quick-2.log` (session scratchpad) and `/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-115704-1958055`. Header, the turbo line and the summary pasted unedited:

```
branch    claude/agent-allifs-unbound-nics @ 0b57dc070
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total Time:    53.588s  
== summary (quick) ==
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m56s
  forbidden patterns (+ gitleaks)                    0m15s
  packet-trace ban on the shared VPP (D-128)         0m00s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   0m54s
  apps/agent: make lint test build                   1m31s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/ab-upgrade test/topology/acl test/topology/backup-restore test/topology/bonding test/topology/bridge-l2 test/topology/det44 test/topology/ha-state-sync test/topology/hardening-lite test/topology/host-acl-nftables test/topology/images test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/traffic-a/globals test/topology/traffic-c/globals test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp)   1m08s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m07s
  warnings:
    - control-plane lines exempted with 'ALLOW:' — reviewer, check each justification:
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:5:import { execFileSync } from 'node:child_process'; // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:19:    execFileSync('openssl', ['version'], { stdio: 'ignore' }); // ALLOW: test-only openssl cert
      apps/api/src/features/mgmt-tls/mgmt-tls.test.ts:26:const openssl = (args: string[]) => execFileSync('openssl', args, { stdio: 'ignore' }); // ALLOW: test cert
      apps/api/src/features/notifications/sinks.test.ts:1:import { execFileSync } from 'node:child_process'; // ALLOW: fixed-argument openssl creates temporary test-only TLS certificate; no runtime exec, shell or committed key
      apps/api/src/features/notifications/sinks.test.ts:103:  execFileSync( // ALLOW: fixed-argv test-only OpenSSL generates a temporary SMTP TLS certificate; no runtime shell
      apps/api/src/features/pki/testkit.test.ts:3:import { execFileSync, spawnSync } from 'node:child_process'; // ALLOW: test-only verifier (openssl), never product code
      apps/api/src/features/pki/testkit.test.ts:32:  const result = spawnSync('openssl', args, options); // ALLOW: test-only fixed-argv verifier
      apps/api/src/features/pki/testkit.test.ts:44:  return execFileSync('openssl', args, { input, stdio: ['pipe', 'pipe', 'pipe'] }); // ALLOW: test-only verifier
  mode quick · wall time 5m09s · logs /root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-115704-1958055

CI GATE PASSED
CI_QUICK_EXIT=0
```

Docs on `main` checked: `docs/user/interfaces/default-dataplane-nics.md` ("the commit succeeds", "does not make commits fail") describes the fixed behaviour and needs no change; `docs/user/security/global-blocking.md` got the one sentence below.

## Out of scope / left undone

- **Merge order with PR #238 — decided by the manager (2026-10-17): #240 → #242 → #241 → #238.** #241 merges before #238. The tech-debt row, the user-doc exception paragraph and the DEFERRED-ACCEPTANCE case (e) that describe this defect live on #238's branch and are updated there by the #238 session (informed by the manager): the row closes with a pointer to PR #241, the exception paragraph is replaced (not dropped: the explicit-reference case remains), the parenthetical "(except through an all-interfaces block list or automatic blocking …)" of the removed-NIC bullet goes, and case (e) is reworded to the target state pointing at the section added here. Suggested texts:
  - tech-debt row: *"2026-10-17 (PR #238 review R4, MAJOR, pre-existing): an all-interfaces block list or active auto-block entries targeted unbound and released physical rows and refused every interfaces/security commit — fixed by PR #241 (`desired.ACL` takes the rows `SplitUnboundPhysical` keeps; regression tests in `internal/desired` and `internal/agent`; residual explicit-reference case recorded in the 2026-10-17 PR #241 rows)."*
  - user-doc paragraph (R4 wording): *"Explicit references (a block list's interfaces, ACL attachments, zones) to an awaiting-engine or released NIC are refused at commit; if one exists when the NIC leaves the engine, commits touching interfaces or security are refused and new automatic blocks are not enforced until the reference is removed. All interfaces and automatic blocking skip such NICs."* The sentence "a released row … does not make commits fail" needs the qualifier "unless a block list, ACL attachment or zone names it explicitly".
- Explicit references to an unbound or released row (a block list's `interfaces`, an ACL or MACIP attachment, a zone) still bind it and fail the plan once the NIC leaves the engine — fail-closed for commits, fail-open for auto-block. Pre-existing and outside this task's fence ("explicit interface lists unchanged"); tech-debt row added (owner manager, due 2026-11-14 proposed), with the four other rows the reviews found (name-based bound/unbound decision in acl-only transactions, VLAN sub-interfaces, other per-row projections, admin-up before block lists after a VPP restart).
- Not run: `tools/ci.sh full`, lab or appliance acceptance (unit tests on the fake VPP only, by the task's fence). The lab and packet-level evidence to collect is the new section "All-interfaces block lists and auto-block with unbound and released NICs — 2026-10-17" in `docs/status/DEFERRED-ACCEPTANCE.md` (R4 #3).
- A NIC bound after the fact (its kernel netdev handed to DPDK by the start-up apply, which restarts VPP) gets its binding with the connect resync, in the same transaction as its own configuration — not at an auto-block tick, which reacts only to entry changes (R4 NIT 2). Before this fix no projection succeeded at all.
- The agent still decides "unbound" by the row's logical name (`desired/hostnics.go`); unchanged here (tech-debt row above and the #238 rows).

## Open questions

None. The merge order is decided (#240 → #242 → #241 → #238); the manager merges sequentially after each main CI pass, never this session. Merge only with the owner's go-ahead.

## Decisions

- D-251 (`docs/decisions/LOG.md`): the projection splits once and passes the kept rows to `desired.ACL` (chosen after R1 #1 / R4 #1 over the first version, which took the lookup and re-split, and over a process-global env field). Numbered D-251 because D-250 was already taken on `origin/fix/deploy-hosts-minors-20261017` (R7 #1).

## Review

Independent panel per `prompts/REVIEW-PROMPT.md`, read-only reviewer agents, hand-backs saved unedited as `agent-allifs-unbound-nics-review-R<n>.md`:

| reviewer | head | verdict | handled |
|---|---|---|---|
| R1 correctness & tests | `552b9bcce` | APPROVE (2 MINOR, 3 NIT) | #1 kept rows passed from the projection; #2 bound-later / idempotence / release test; #3 doc comment; #4 `computed` flag; #5 nothing-bound test |
| R2 security | `552b9bcce` | APPROVE (4 MINOR) | #1 (a) warning naming the skipped rows, (c) tech-debt row; #2 wording corrected + tech-debt row; #3 user-doc sentence; #4 tech-debt row |
| R4 data plane | `552b9bcce` | APPROVE WITH CHANGES (5 MINOR, 3 NIT) | #1 kept rows; #2 tech-debt row; #3 DEFERRED-ACCEPTANCE section; #4 wording + residual debt row; #5 fake-VPP tests; NIT 1 lookup stub, NIT 2 wording, NIT 3 PCIs |
| R7 docs & evidence | `552b9bcce` | BLOCK (2 MAJOR, 7 MINOR, 6 NIT) | #1 D-251; #2 merge order fixed by the manager (#241 before #238), #238-side edits assigned to the #238 session; #3 row text in this report; #4 WIP SHAs; #5 unedited pastes; #6 gate result in this report; #7 PR body; #8 user-doc sentence; #9 DEFERRED section; NITs 10–11 in the D112 squash message, 12 names aligned, 13 envelope, 14 pattern, 15 pinned links |

### Verify round

On the head that carries this file (product tree = `856bf743a`): R7 (MAJORs #1, #2 and the MINORs/NITs), R1 and R4 (the signature change and the new tests), R2 (its MINORs), R6 (the user-doc sentence). Hand-backs are appended unedited to the same `agent-allifs-unbound-nics-review-R<n>.md` files under "Verify round"; the result is recorded here and in the PR when it is in.
