# agent-allifs-unbound-nics — status

PR: https://github.com/mcoder1001-cyber/NGFW/pull/241 · branch `claude/agent-allifs-unbound-nics` · base `origin/main` `99a774734` · fixes finding #1 of review R4 of [PR #238](https://github.com/mcoder1001-cyber/NGFW/pull/238) ([review file](https://github.com/mcoder1001-cyber/NGFW/blob/c2f898d2b/docs/status/tasks/boot-nic-registration-port-review-R4.md), [tech-debt row dated 2026-10-17 "PR #238 review R4, MAJOR"](https://github.com/mcoder1001-cyber/NGFW/blob/c2f898d2b/docs/tech-debt.md), both on the #238 branch only; `main` carries neither).
Product commits: `9a63c09ce` (the fix), `856bf743a` (review round 1), `1377ae788` (verify round), `b11a29700` (verify round 2) and `f53747c65` (R1's last NITs); the other commits are docs/status. Agent only; no schema/proto/API change; no VPP, lab slot or host service touched.

## What was built

- `apps/agent/internal/desired/acl.go`: `desired.ACL` takes the interface rows the projection keeps (`kept`, from `SplitUnboundPhysical`); nil is the split without a netdev lookup (every engine-owned row kept, released rows dropped). The 15 test callers pass nil.
- `apps/agent/internal/desired/global_blocking.go`: the implicit all-interfaces target set of the block lists — `allInterfaces` lists and the auto-block overlay, which always selects all interfaces — is `sortedKeys(kept)`, computed once per projection and only when a list asks for it. A list that selects all interfaces reports, at the field the operator set (`acl.global-blocking` at `/acl/globalBlocking/lists/<name>/allInterfaces`; Auto-block's overlay at `/security/autoBlock`), the NICs it skips because they are still awaiting the engine (a warning) and the case where it has no interface the engine has yet; a NIC released to the host is not repeated (the interfaces projection's `agent.nic-released` note and the user doc cover it; the UI shows every note like a warning). Commit and DryRun reports carry the warnings; the auto-block runtime reconcile, which returns no report, logs them with their pointer once per change of their text and a true all-clear line (`logSkippedNICsLocked` in `agent/rpc_autoblock.go`), so a binding dropped in an acl-only transaction leaves a trace. Explicit interface lists are unchanged.
- `apps/agent/internal/agent/projection.go`: passes `boundIfs` — the result of the one `SplitUnboundPhysical` call per projection — so the interfaces and acl domains see one observation of the kernel and the acl projection adds no netlink dumps.
- Why: an unbound seeded NIC (still a Linux kernel interface) or a NIC released to the host has no `interface/<row>` alias (never projected), so `acl.interface-binding/<row>` failed the plan with a missing mandatory dependency: every interfaces/security commit was refused and `AutoBlockSet` returned `Unavailable` (new automatic blocks enforced nowhere, VPP and host nftables alike) while an all-interfaces list or live auto-block entries existed. Pre-existing on `main`; #238's boot-time registration makes it reachable on every appliance with kernel NICs.

## How verified (pasted output, unedited)

Red: the committed head with one line of `global_blocking.go` put back to the pre-fix form (`allIfs = sortedKeys(ds.GetInterfaces())` instead of `sortedKeys(kept)`, by `sed`, then `git checkout -- apps/agent/internal/desired/global_blocking.go`); run from `apps/agent`, the agent's own log lines filtered out as marked. Six of the seven tests fail; `TestProjectAutoBlockOverlayAddsNoNetdevLookups` guards the lookup count, which this line does not change, and is red only against an acl-side re-lookup (R1's verify-round mutation A):

```
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestGlobalBlockingTwoAllInterfacesLists|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake' ./internal/desired/ ./internal/agent/ 2>&1 | grep -vE '^[0-9]{4}/[0-9]{2}/[0-9]{2} [0-9:]+ (INFO|WARN|DEBUG|ERROR) '
--- FAIL: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
    global_blocking_test.go:268: a released NIC is not an all-interfaces target
--- FAIL: TestGlobalBlockingAllInterfacesNothingBound (0.00s)
    global_blocking_test.go:306: want the unbound-NIC warning and the not-enforced warning at the field the operator set, got [W /acl/globalBlocking/lists/all/allInterfaces acl.global-blocking block list "all" selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192]
--- FAIL: TestGlobalBlockingTwoAllInterfacesLists (0.00s)
    global_blocking_test.go:339: the unbound NIC is not bound
FAIL
FAIL	ngfw/agent/internal/desired	0.172s
--- FAIL: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.02s)
    physical_nics_acl_test.go:110: no binding for ens192: its interface alias is never projected
--- FAIL: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.03s)
    physical_nics_acl_test.go:172: auto-block set with an unbound and a released NIC: <nil> rpc error: code = Unavailable desc = auto-block enforcement: validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present
--- FAIL: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.02s)
    physical_nics_acl_test.go:255: status APPLY_STATUS_FAILED, want APPLY_STATUS_APPLIED: validation failed: acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present results=[key:"acl.interface-binding/ens161"  code:OBJECT_RESULT_CODE_DEPENDENCY_MISSING  message:"mandatory dependency interface/ens161 is neither desired nor present"  pointer:"/acl/globalBlocking/lists/bad"  subsystem:"acl" key:"acl.interface-binding/ens192"  code:OBJECT_RESULT_CODE_DEPENDENCY_MISSING  message:"mandatory dependency interface/ens192 is neither desired nor present"  pointer:"/acl/globalBlocking/lists/bad"  subsystem:"acl"] validation=txn_id:"all1"  errors:{pointer:"/acl/globalBlocking/lists/bad"  message:"acl.interface-binding/ens161: mandatory dependency interface/ens161 is neither desired nor present"  severity:ISSUE_SEVERITY_ERROR  rule:"agent.dependency-missing"}  errors:{pointer:"/acl/globalBlocking/lists/bad"  message:"acl.interface-binding/ens192: mandatory dependency interface/ens192 is neither desired nor present"  severity:ISSUE_SEVERITY_ERROR  rule:"agent.dependency-missing"}  errors:{pointer:"/acl/globalBlocking/lists/bad/allInterfaces"  message:"block list \"bad\" selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192"  severity:ISSUE_SEVERITY_WARNING  rule:"acl.global-blocking"}  errors:{pointer:"/interfaces/ens161"  message:"physical NIC \"ens161\" is released to the host (physical.owner = host): nothing is configured on the data plane for it"  severity:ISSUE_SEVERITY_INFO  rule:"agent.nic-released"}  errors:{pointer:"/interfaces/ens192"  message:"physical NIC \"ens192\" is not bound to the data plane yet (still a Linux kernel interface); its configuration is applied once the NIC is handed to VPP"  severity:ISSUE_SEVERITY_WARNING  rule:"agent.nic-not-bound"}  summary:{}
FAIL
FAIL	ngfw/agent/internal/agent	0.242s
FAIL
exit=1
```

Green, the committed head (the seven tests plus the neighbouring global-blocking and physical-NIC tests):

```
$ env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestGlobalBlockingTwoAllInterfacesLists|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake|TestGlobalBlocking|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	4.908s
ok  	ngfw/agent/internal/agent	0.423s
exit=0
```

What the seven tests prove: `TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs` (desired) — an all-interfaces list plus an explicit list over bound, unbound and released rows binds the kept rows only, the unbound NIC only through the explicit list, one warning names the NIC awaiting the engine (the released one is not reported), and without a lookup the released row is still dropped silently; `TestGlobalBlockingAllInterfacesNothingBound` — every row unbound or released: nothing bound, no error, the list reported at its field as having no interface the engine has yet; `TestGlobalBlockingTwoAllInterfacesLists` — two lists in one projection warn once each and name every NIC once (the memo flag); `TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs` (agent) — the auto-block overlay binds the bound NIC only, keeps the nic-not-bound / nic-released notes and reports the NIC awaiting the engine at `/security/autoBlock` as a warning; `TestProjectAutoBlockOverlayAddsNoNetdevLookups` — one lookup per engine-owned row, none for a released one, none added by the acl projection; `TestAutoBlockWithUnboundAndReleasedNICsOnFake` (fake VPP) — `AutoBlockSet` succeeds and the bound NIC carries `_gb.auto-block.i00`, an interfaces commit with live entries has no DryRun error and is APPLIED (the new interface bound too), the runtime reconcile logs one WARN line for the NIC awaiting the engine (captured logger), nothing for a repeated snapshot, a true all-clear line for an empty snapshot, the WARN again when entries return and the all-clear line after the bind, a NIC handed to VPP later is bound by the resync together with its own configuration, a second resync changes nothing, and a released NIC loses our binding and goes admin-down; `TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake` — a user all-interfaces list: the bound NIC and the loopback bound, `Retrieve` equals desired, re-apply and resync empty, a later bind picked up.

Agent gate on the committed head `6ba91e042` (product commit `b11a29700`): `env -u NGFW_INTEGRATION make -C apps/agent lint test`, run from the worktree root, nothing committed while it ran; log `agent-gate-4.log` (session scratchpad). Pasted unedited except the marked elisions:

```
make: Entering directory '/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19/apps/agent'
NGFW_TIMING BEGIN agent.vet 2026-10-17T12:51:08.641130556Z
go vet ./...
NGFW_TIMING END agent.vet 2026-10-17T12:51:23.030890864Z
NGFW_TIMING BEGIN agent.lint-dispatch 2026-10-17T12:51:23.054829215Z
0 issues.
NGFW_TIMING END agent.lint-dispatch 2026-10-17T12:51:34.454526764Z
NGFW_TIMING BEGIN agent.race-test 2026-10-17T12:51:34.476985595Z
go test -race -count=1 ./...
ok  	ngfw/agent/internal/agent	59.462s
ok  	ngfw/agent/internal/descriptors/acl	1.330s
ok  	ngfw/agent/internal/desired	28.553s
ok  	ngfw/agent/internal/subsystems	31.794s
# 139 further "ok" lines, 166 "?  … [no test files]" lines and the final "make: Leaving directory" line elided — grep -c '^ok' agent-gate-4.log → 143; grep -c -E '^(FAIL|--- FAIL)' → 0
NGFW_TIMING END agent.race-test 2026-10-17T12:53:19.936999185Z
AGENT_GATE_EXIT=0
```

Complete `tools/ci.sh quick` (unchanged, from the worktree root, `env -u NGFW_INTEGRATION`) on the same head `6ba91e042`, nothing committed while it ran; log `ci-quick-4.log` (session scratchpad) and `/root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-125345-2113815`. Header, the turbo line and the summary pasted unedited:

```
branch    claude/agent-allifs-unbound-nics @ 6ba91e042
Tasks:    35 successful, 35 total Cached:    28 cached, 35 total Time:    56.373s  
== summary (quick) ==
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   0m57s
  forbidden patterns (+ gitleaks)                    0m16s
  packet-trace ban on the shared VPP (D-128)         0m00s
  ip classify reset after every shell interface create (D-185)   0m02s
  slot resource scheme (1..32, no collisions)        0m04s
  lint · typecheck · unit tests · build (turbo)   0m57s
  apps/agent: make lint test build                   1m31s
  apps/cli: make lint test build                     0m09s
  test/ Go modules, unit mode (test/integration/reachability test/integration/smoke test/topology/ab-upgrade test/topology/acl test/topology/backup-restore test/topology/bonding test/topology/bridge-l2 test/topology/det44 test/topology/ha-state-sync test/topology/hardening-lite test/topology/host-acl-nftables test/topology/images test/topology/interfaces test/topology/ipfix-sflow test/topology/kea-dhcp-relay test/topology/loopback-bvi-gso-lldp-span test/topology/nat44-ed-sessions test/topology/nat44-ei-64-66-nptv6 test/topology/neighbors-ra test/topology/object-model test/topology/qos-flat test/topology/system-identity test/topology/traffic-a/globals test/topology/traffic-c/globals test/topology/unbound-chrony-syslog test/topology/vlan-qinq test/topology/vrf-static-ecmp)   1m07s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m06s
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
  mode quick · wall time 5m12s · logs /root/ngfw-wt/logs/ci/funny-meninsky-c9ef19-20261017-125345-2113815

CI GATE PASSED
CI_QUICK_EXIT=0
```

Earlier gates on the previous product versions (`9a63c09ce`, `0b57dc070`, `0e96796ae`) passed as well; their pasted outputs are in this file's history (`git log -p -- docs/status/tasks/agent-allifs-unbound-nics.md`).

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

### Verify round (head `873f1d946`) and the changes it asked for

| reviewer | verdict on `873f1d946` | handled in `1377ae788` / `0e96796ae` |
|---|---|---|
| R1 | APPROVE (1 MINOR, 3 NIT) | V1 reconcile log; V2 two-lists test; V3 severity split + `/security/autoBlock`; V4 "six fail" → six of seven, the lookup-count test explained |
| R2 | APPROVE (3 MINOR, 1 NIT open) | #1a reconcile log; #2 PR body republished through the REST API (`gh pr edit` was failing on a GraphQL deprecation error — R7's check prints `[false,false,false,true]`); V-1 doc cell; V-2 released NICs no longer warned |
| R4 | APPROVE (2 MINOR, 2 NIT) | #4 PR body; N1 reconcile log; N2 released NICs no longer warned; N3 overlay at `/security/autoBlock`, "Auto-block" |
| R6 | BLOCK (1 MAJOR, 4 MINOR, 4 NIT) | MAJOR-1 / MINOR-2 / NIT-6 the cell reworded with R6's text and links; #3 and #5 tech-debt rows extended (locale keys, picker); #4 warning split, overlay wording; NIT-7 pointer `…/allInterfaces`; NIT-9 one sentence in `auto-block.md`; NIT-8 won't fix (one cell, as R6 allowed) |
| R7 | APPROVE WITH CHANGES (1 MAJOR, 3 MINOR, 3 NIT) | N1 PR body published + checked; N2 six of seven; N3 cell; N4 relayed to the manager for the #238 session (this session posts no comment on #238); N5 envelope; N6 elision note; N7 heading + "(on PR #238's branch until it merges)" |

### Verify round 2 (head `b74ad2b1b`) and the changes it asked for

| reviewer | verdict on `b74ad2b1b` | handled in `b11a29700` |
|---|---|---|
| R6 | APPROVE (MINOR-4 partial, NIT-10 new; follow-ups accepted) | MINOR-4: released NICs no longer reported by the block lists (the UI shows every note like a warning; `agent.nic-released` and the doc cover them); NIT-10: the "no interface" warning reported at the same field with "has no interface the engine has yet"; "Auto-block" wording; its note on the log lines: pointer logged with each line, generic "block lists:" prefix, true all-clear line |
| R1 | APPROVE (N1 MINOR, N2/N3 NIT) | N1: the all-clear line is true in every case ("no list on all interfaces skips a physical NIC any more"), user-list warnings logged with their pointer; N2: the test captures the logger (one WARN, none for a repeat, all-clear for an empty snapshot, WARN again, all-clear after the bind); N3: same as R6 NIT-10 |

### Verify round 3 (head `b11a29700`)

| reviewer | verdict | handled in `f53747c65` |
|---|---|---|
| R1 | APPROVE (0 MINOR, 3 NIT N4–N6); N1–N3 and R6's log-line note confirmed fixed | N4 all-clear line "no list on all interfaces skips a NIC awaiting the engine any more"; N5 the nothing-bound test pins the explicit-list wording and the Protect-the-box clause (only when the setting is on) with exact lines; N6 comments corrected. Gated again below. |

@@GATES5@@
