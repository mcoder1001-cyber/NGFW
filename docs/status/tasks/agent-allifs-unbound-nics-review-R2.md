# agent-allifs-unbound-nics: review R2 (security)

- **Reviewer:** R2 (security). This is an independent, read-only panel review; no repository or worktree file was created, modified or deleted.
- **Branch / PR:** `claude/agent-allifs-unbound-nics`, PR #241
- **Base:** `origin/main` `99a774734`
- **Head reviewed:** `552b9bcce`. The product commit is `9a63c09ce`; `8d34740d4` and `552b9bcce` are docs/status only.
- **New commit during the review:** `7afee7aef` ("docs(status): quick gate PASSED") landed in the worktree while I was reviewing. It changes only `docs/status/tasks/agent-allifs-unbound-nics{,-wip}.md`. I secret-scanned it as well. The Go tree is identical to `552b9bcce`, so my targeted test runs, which ran at `7afee7aef`, test the reviewed code.
- **Scope checked:** secrets in code, docs, status files and the decision-log line; injection and shell-outs; authz; file modes; dependencies; input limits. I also checked the security question specific to this change: does skipping the VPP ACL binding for an unbound or released physical row weaken enforcement anywhere?

## Summary answer to the change-specific question

On the realistic paths, enforcement is not weakened. The pre-fix behaviour was the bigger weakness.

**Unbound NIC**
- It has no VPP interface, so a VPP ACL binding on it could never enforce anything. Its traffic is handled on the Linux side.
- **Traffic to the box** is covered by the nftables chain `in__gb`, which:
  - is on the input hook at priority -300, before conntrack;
  - matches no `iifname`, so it applies on every host interface (`apps/agent/internal/renderers/nftables/blocking.go:10-12,115`);
  - is rendered for every list with `protectHost`. The auto-block overlay always sets `ProtectHost: true` (`apps/agent/internal/autoblock/runtime.go:123`).
- **Transit traffic:** the repo sets `ip_forward` only inside the RA-VPN private namespace (`apps/agent/internal/ra_vpn/firewall.go:62,78`), never in the host namespace. So the default deployment has no kernel-forwarded transit path that the data-plane lists would need to cover.
- **Interface configuration:** the interfaces projection drops the row through the same `SplitUnboundPhysical`, so the row's own configuration (admin-up, addresses) is never applied either. VPP creates DPDK interfaces admin-down. A NIC that binds later therefore carries no traffic until the next projection that includes interfaces. That projection also re-adds the block-list binding, in the same transaction.

**Released NIC (`physical.owner: host`)**
- It is host-owned, and the host side is covered as above.
- It may still be in VPP until the next start-up apply. In that case the same transaction deletes its `interface.admin-state`, which sets it admin-down (`apps/agent/internal/descriptors/interface/attributes.go:204-214`), and removes its addresses. VPP therefore forwards nothing on it without the binding.

**Fail-open vs fail-closed before the fix**
- The acl-domain plan failed as a whole: `CodeDependencyMissing` on `acl.interface-binding/<row>`.
- That transaction also carries the host nftables descriptor (`Domains["acl"]`, `subsystems/host_acl.go:23`; `projection.go:414-416`). So new auto-block entries were enforced neither in VPP nor on the host: `AutoBlockSet` returned `Unavailable` (`rpc_autoblock.go:112-114`).
- Meanwhile every interfaces/security commit was refused (`service.go:1385-1387` adds `acl` to those commits whenever auto-block holds entries).
- Auto-block entries are created by detection of outside activity. An external attacker could therefore both freeze the administrator's security configuration and escape new automatic blocks. That was fail-open for auto-block plus an attacker-influenced configuration denial of service.
- The fix removes both. Except for the narrow case in MINOR-1, the resulting enforcement is a superset of what the pre-fix code could ever apply.

## Findings (ranked by severity)

No BLOCKER and no MAJOR.

### MINOR-1: the all-interfaces target set now follows a live netlink name check, and in acl-only transactions it can silently remove block lists from a live VPP interface

- **Where:** `apps/agent/internal/desired/global_blocking.go:102-114,143-146`, `apps/agent/internal/agent/projection.go:242,411`, `apps/agent/internal/agent/rpc_autoblock.go:112`, `apps/agent/internal/desired/hostnics.go:30`
- **What changed:** the split is re-run inside `globalBlocking`. That is a second `RTM_GETLINK` dump, and it decides "unbound" only by the row's name: a kernel netdev of that name exists and has no rtnetlink kind.
- **Why acl-only transactions matter:** these include the auto-block reconcile (`[]string{"acl"}`, `rpc_autoblock.go:112`), security-only commits and scheduled block-list refreshes. In them, interfaces-domain objects (admin-state, addresses) are out of scope and stay as last applied. A row now classified "unbound" still loses its `acl.interface-binding` (in scope, present, undesired, so the plan deletes it).
- **Scenario:** VPP still owns an interface with logical name X (bound, admin-up, carrying the all-interfaces lists and `_gb.auto-block.*`). Then a kind-less kernel netdev also named X appears. Possible causes:
  - kernel name reuse after a NIC is handed back from DPDK or hot-added under `ethN`-style naming (the class of name clash PR #238 `c5356454c` guards against);
  - a bifurcated PMD such as mlx5, whose kernel netdev persists.
- **Effect:** the next acl-only transaction deletes every all-interfaces block list and the auto-block ACL from X while X keeps forwarding, until the next interfaces commit.
  - No warning is emitted: `agent.nic-not-bound` appears only when `in["interfaces"]` (`projection.go:243-250`), and `globalBlocking` reports nothing for skipped rows.
  - Before the fix, the binding stayed, because the observe-only alias `interface/X` was present and satisfied the dependency.
- **Smaller variant:** the two lookups in one projection (`projection.go:242` vs `global_blocking.go:110`) can disagree. D-250's rationale ("the same input the interfaces projection gets") is true of the function but not of the observation.
- **Severity reasoning:** the trigger needs host-level (root, hardware or udev) changes, and no privilege boundary is crossed. It is MINOR because the direction is fail-open and silent.
- **Fix, any of:**
  - (a) Emit a warning at `/acl/globalBlocking/lists/<name>/interfaces` for each row an all-interfaces list skips, so the drop is never silent.
  - (b) Compute the split once in `projectOwned` and pass the result (or the unbound set) to `ACL`, so one observation drives both domains in a transaction.
  - (c) Harden the shared classification: treat X as unbound only when the kernel netdev's PCI address (`/sys/class/net/X/device`) equals the row's `physical.pci`. This is the pre-existing "decides unbound by name" debt from #238.
- At minimum, extend the existing #238 debt row to state that, after this PR, a name clash is fail-open for block lists in acl-only transactions.

### MINOR-2: the same defect class remains through explicit references, and the PR calls it "fails closed"

- **Where:** PR #241 body, section "The fix"; `apps/agent/internal/desired/global_blocking.go:143-146`; `packages/schema/src/semantic/global-blocking.ts:16-25`
- **What remains:** an explicit block list, or an ACL attachment, that names a released NIC passes `acl.global-blocking-interfaces-exist`, because the row exists. The release commit itself can succeed while the NIC is still in VPP, since the alias is present.
- **Scenario:** after the next start-up apply the NIC leaves VPP. From then on every acl-domain transaction fails with `CodeDependencyMissing`. Auto-block stops enforcing new entries on every interface, VPP and host nftables alike. That is exactly the symptom this PR fixes, now reached through explicit lists.
- **Same for unbound:** a NIC that was bound when the list was committed and later stays a kernel NIC (for example a failed bind after reboot) has the same effect.
- **Why the wording matters:** the PR text "fails closed" is accurate for configuration commits but wrong for auto-block, which fails open.
- **Fix:**
  - Correct the wording in the PR and status file.
  - Record a tech-debt row with an owner and date covering:
    - (i) a semantic refusal of explicit block-list/attachment references to `physical.owner: host` rows (R3 territory, schema);
    - (ii) decoupling the host nftables auto-block part from unrelated VPP binding errors, so host protection never depends on the validity of VPP bindings.
- Out of this PR's declared scope ("explicit lists unchanged"), so not blocking.

### MINOR-3: the user-facing coverage statement for "All interfaces" is now incomplete

- **Where:** `docs/user/security/global-blocking.md:27,29`, `packages/schema/src/domains/ext/global-blocking.ts:64-67` (help: "every L3 interface, including interfaces added later")
- **What changed:** after this change, "All interfaces" means the data-plane rows the projection keeps. A NIC awaiting engine or released to the host gets no data-plane ACL. It is protected only for traffic to the box, and only through *Protect the box*; with `protectHost: false` it is not protected at all.
- **Why it matters:** the only text that explains this interplay is the exception paragraph in `docs/user/interfaces/default-dataplane-nics.md` on the #238 branch. The PR says that paragraph "can then be dropped". Once it is gone, an operator could reasonably assume a released management NIC is covered by an all-interfaces list.
- **Fix:** add one sentence to `global-blocking.md` (here, or in #238's replacement text): "All interfaces = the data-plane interfaces; a NIC awaiting engine or released to the host is covered only by Protect the box (traffic to the appliance)." Do not simply delete the #238 paragraph. A schema help-text change is R3's call.

### MINOR-4 (pre-existing, not introduced by this diff): all-interfaces lists never cover VLAN sub-interfaces

- **Where:** `apps/agent/internal/desired/global_blocking.go:110-111` (and `sortedKeys(ds.GetInterfaces())` on `main` before it)
- **Problem:** the target set is the top-level interface keys only. Sub-interfaces `<parent>.<id>` (`desired/interfaces.go:~185-205`) are separate VPP sw_if_indexes. The acl plugin's input/output features run per RX/TX sw_if_index, so an ACL on the parent does not see traffic demultiplexed to a dot1q sub-interface.
- **Scenario:** a listed source arriving on a VLAN sub-interface is not dropped by an all-interfaces list or by auto-block in the data plane. Host-destined traffic is still dropped by `in__gb`. This contradicts the schema help "every L3 interface".
- **Fix:** open a tech-debt row or follow-up task to include sub-interfaces in the all-interfaces set, plus a test. Not attributable to this PR; recorded so it is not lost.

## Notes for other reviewers

- **[other: R5]** (not on this panel): `subsystems.LinuxNetdevKind` (`internal/subsystems/netdev.go:30-31`) dumps all links on every call. Each projection carrying acl now runs one dump per physical row a second time, in the auto-block reconcile too. This is negligible for typical NIC counts; noting it only because the auto-block loop can run at 1 Hz.
- **[other: R7]**
  - The PR body's suggested debt-row text has an unfilled placeholder `PR #@@N@@`.
  - The status file cites logs "in the session scratchpad", which is not durable. The `/root/ngfw-wt/logs/ci/...` path is cited as well.
  - D-250 is unique across refs. D-249 is used by two different commits on other branches (`f1679314c` deploy lineage guard, `814532721` NIC registration), which does not concern this PR.

## Security checklist results

- **Secrets:** none in the diff, the status files (including the new `7afee7aef`) or the D-250 line. Whole-worktree gitleaks: 10 `generic-api-key` hits, all in pre-existing test fixtures this branch does not touch. Intersection with changed files: empty.
  - The envelope contains the worktree path `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`. That is not a secret, and 15 other envelopes do the same.
- **Shell / injection:** none added. The lookup is netlink by name with no shell. Row names reach it only for KindExisting rows with a read-only `physical` marker.
- **AuthN/authz:** no route, API or proto change.
- **File modes:** no new file writes in product code. Test state uses `t.TempDir()`.
- **Dependencies:** `go.mod`/`go.sum` unchanged.
- **Input limits:** unchanged. Rows are bounded by the configuration, and auto-block entries are capped by `autoblock.MaxEntries`.

## Evidence (commands run, outputs)

```
$ git log --oneline -5 && git rev-parse HEAD origin/main      # at start
552b9bcce docs(status): task report for agent-allifs-unbound-nics (PR #241)
8d34740d4 docs(status): envelope, WIP and decision D-250 for agent-allifs-unbound-nics
9a63c09ce fix(agent): all-interfaces block lists target only the rows the projection keeps
99a774734 fix(deploy): complete package upgrades and audit unfinished branches
4f70b3315 fix(interfaces): restore Ethernet names across web and CLI
552b9bcce6e33847cd61ba245b83556966cca8e4
99a774734e2cf455f71563ff0dba5afb58051685

$ git diff --stat 99a774734...552b9bcce
 .../agent/internal/agent/physical_nics_acl_test.go | 122 +++++++++++++++++++++
 apps/agent/internal/agent/projection.go            |   5 +-
 apps/agent/internal/desired/acl.go                 |   9 +-
 apps/agent/internal/desired/acl_test.go            |  10 +-
 apps/agent/internal/desired/global_blocking.go     |  18 ++-
 .../agent/internal/desired/global_blocking_test.go |  58 +++++++++-
 apps/agent/internal/desired/rule_expiry_test.go    |   6 +-
 apps/agent/internal/subsystems/acl_pbr_test.go     |   2 +-
 docs/decisions/LOG.md                              |   2 +
 .../tasks/agent-allifs-unbound-nics-envelope.md    |   9 ++
 docs/status/tasks/agent-allifs-unbound-nics-wip.md |  10 ++
 docs/status/tasks/agent-allifs-unbound-nics.md     |  68 ++++++++++++
 12 files changed, 297 insertions(+), 22 deletions(-)

$ gitleaks version  -> 8.30.1
$ gitleaks detect --no-git -s <scratch copy of every file changed in 99a774734...552b9bcce, at 552b9bcce> --redact --no-banner -v
INF scanned ~293867 bytes (293.87 KB) in 592ms
INF no leaks found

$ gitleaks detect --no-git -s /root/NGFW/.claude/worktrees/funny-meninsky-c9ef19 --redact --no-banner -r <scratch>/gitleaks-worktree.json
INF scanned ~123640472 bytes (123.64 MB) in 12.3s
WRN leaks found: 10            (exit 1)
generic-api-key apps/agent/internal/renderers/rfkit/fixround_test.go 20
generic-api-key apps/agent/internal/renderers/strongswan/apply_test.go 375
generic-api-key apps/agent/internal/renderers/rsyslog/rsyslog_test.go 31
generic-api-key apps/agent/internal/scheduler/validator_test.go 371
generic-api-key apps/agent/internal/scheduler/validator_test.go 372
generic-api-key apps/cli/internal/api/transport_test.go 82
generic-api-key packages/schema/dist/domains/vpn.test.js 154
generic-api-key packages/schema/dist/domains/vpn.test.js 220
generic-api-key packages/schema/src/domains/vpn.test.ts 197
generic-api-key packages/schema/src/domains/vpn.test.ts 265
findings in changed files: []

$ grep -rnE 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]' docs/status/tasks/agent-allifs-unbound-nics* $(git diff --name-only 99a774734...552b9bcce)
grep exit=1   (no match)
$ grep -nE '172\.30\.|/root/|token|ghp_|github_pat|Bearer|@gmail|PSK|secret' docs/status/tasks/agent-allifs-unbound-nics*
docs/status/tasks/agent-allifs-unbound-nics-envelope.md:4:Worktree: /root/NGFW/.claude/worktrees/funny-meninsky-c9ef19
$ grep -l "^Worktree: /root/" docs/status/tasks/*envelope* | wc -l   -> 15   (established convention)
$ git diff 99a774734...552b9bcce | grep -nE '^\+.*(exec\.Command|os\.WriteFile|os\.Create|Chmod|0o6|0o7|sh -c|vppctl|cli_inband|http\.|net\.Dial)'
grep3 exit=1   (no match)
$ git diff 99a774734...552b9bcce -- apps/agent/go.mod apps/agent/go.sum | wc -l   -> 0

$ (apps/agent) env -u NGFW_INTEGRATION go test -count=1 -run 'TestGlobalBlocking|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestProjectPhysicalNic|TestSplitUnboundPhysical' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	4.972s
ok  	ngfw/agent/internal/agent	0.266s

$ (apps/agent) env -u NGFW_INTEGRATION go test -count=1 -v -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAutoBlock|TestHostACL.*Block|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/   (filtered to results)
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.02s)
ok  	ngfw/agent/internal/desired	0.158s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.01s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.07s)
--- PASS: TestProjectPhysicalNicAwaitingDataplane (0.00s)
--- PASS: TestProjectPhysicalNicBoundProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicNoLookupProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicLcpTapIsNotUnbound (0.00s)
--- PASS: TestProjectPhysicalNicUnboundHasNoLcpPair (0.00s)
--- PASS: TestProjectPhysicalNicReleasedToHost (0.00s)
--- PASS: TestAutoBlockRuntimeLifecycleOnFake (0.12s)
--- PASS: TestAutoBlockJournalAuthority (0.00s)
--- PASS: TestAutoBlockConfirmedInactivePublicationDoesNotSchedule (0.02s)
--- PASS: TestAutoBlockInactivePublicationStillClearsRetainedEntries (0.05s)
--- PASS: TestAutoBlockOmittedOwnerSurvivesRestart (0.01s)
--- PASS: TestAutoBlockObservationPreservesPortThroughStream (0.00s)
ok  	ngfw/agent/internal/agent	0.452s

$ (apps/agent) env -u NGFW_INTEGRATION go test -count=1 -run 'ACL|GlobalBlocking|AutoBlock|HostACL|PhysicalNic|SplitUnbound' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	7.649s
ok  	ngfw/agent/internal/agent	7.176s
$ git rev-parse HEAD   -> 7afee7aef318514139fecfa6e0bb5a45f42f5098   (head moved during the review)
$ git log --oneline 552b9bcce..HEAD && git diff --stat 552b9bcce HEAD
7afee7aef docs(status): quick gate PASSED for agent-allifs-unbound-nics
 docs/status/tasks/agent-allifs-unbound-nics-wip.md |  7 +++--
 docs/status/tasks/agent-allifs-unbound-nics.md     | 34 +++++++++++++++++++++-
 2 files changed, 37 insertions(+), 4 deletions(-)
$ gitleaks detect --no-git -s <scratch copy of both files at 7afee7aef> --redact --no-banner  -> INF no leaks found
$ grep -rnE 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]|ghp_|github_pat|Bearer [A-Za-z0-9]' <same copy>   -> exit 1 (no match)
$ git status --porcelain | wc -l   -> 0   (worktree untouched by this review)

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json title,state,headRefOid,baseRefName,body,mergeable
fix(agent): all-interfaces block lists skip unbound and released NICs OPEN 552b9bcce6e33847cd61ba245b83556966cca8e4 main MERGEABLE
```

These are code-reading claims, not run: the acl-only deletion path in MINOR-1 and the explicit-reference failure in MINOR-2. They are derived from `scheduler/reconciler.go:7-15,497-512` (in-scope undesired objects are deleted; out-of-scope ones are untouched), `descriptors/interface/alias.go:104-106` (the alias is observe-only and present while VPP has the interface) and `service.go:1385-1387`. No experiment was run, because the allowed command set does not include building a modified copy.

## Verdict

**APPROVE**: 0 BLOCKER, 0 MAJOR, 4 MINOR, 0 NIT. The MINORs are optional under the shared rules. I recommend, as cheap and in scope: the skipped-row warning from MINOR-1 (a), the wording correction from MINOR-2, and the user-doc sentence from MINOR-3. MINOR-1 (c), MINOR-2 (i)/(ii) and MINOR-4 should become tech-debt rows with an owner and date.
