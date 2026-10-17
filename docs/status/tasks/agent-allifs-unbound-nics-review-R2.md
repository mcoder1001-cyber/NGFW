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

## Verify round (head `873f1d946`)

- **Reviewer:** R2 (security), verify round. This was read-only: no repository or worktree file was created, modified or deleted. Scratch files are only under `/tmp/claude-0/…/scratchpad/review-R2-verify/`.
- **Head verified:** `873f1d946`.
- **Delta checked:** `git diff 552b9bcce 873f1d946`.
  - Product commit `856bf743a`: `projection.go`, `desired/acl.go`, `desired/global_blocking.go` and two test files.
  - Docs commit `0b57dc070`: D-251, the DEFERRED-ACCEPTANCE section, the five tech-debt rows, the user-doc sentence and the envelope.
  - Status files and the saved R1/R2/R4/R7 reports.

### Round-1 findings

**MINOR-1: partially fixed.**
- **(b) fixed.**
  - `projection.go:242` splits once. `projection.go:411` passes that `boundIfs` to `desired.ACL`.
  - `globalBlocking` (`global_blocking.go:111-131`) no longer does a netdev lookup. It re-splits only when `kept == nil`, and only with a nil lookup.
  - `projectOwned` is the only production caller (grep below). Neither `ds` nor `boundIfs` is reassigned or mutated between lines 242 and 411.
  - `TestProjectAutoBlockOverlayAddsNoNetdevLookups` proves the acl projection adds no lookups. So the "two observations disagree" variant is gone.
- **(c) fixed as a tech-debt row.** `docs/tech-debt.md:226`: decide bound or unbound by `physical.pci`; owner manager; due 2026-11-14 (proposed). The main scenario still exists by design: an acl-only transaction drops the binding of a name-clashed row that VPP still owns. This is the recorded debt.
- **(a) partially fixed.**
  - The warning exists: `global_blocking.go:163-172`, one per all-interfaces list, at `/acl/globalBlocking/lists/<name>/interfaces`. It reaches DryRun/validate and commit responses (API `commit.service.ts` returns `warnings`). It is tested in `desired` and `agent`.
  - **Residual (MINOR, open):** the two agent-internal acl transactions throw the warning away.
    - The auto-block reconcile (`rpc_autoblock.go:112-115`) keeps only `resp.Status` and `resp.Message` and ignores `resp.Validation`. It runs from `AutoBlockSet` and from the 1 s `watchAutoBlock` loop.
    - The connect resync (`agent.go:491-493`) logs only status and summary.
    - Projection issues are never logged (`service.go:561-566` attaches them to the response only).
  - **Scenario:** the MINOR-1 name clash appears, and the next acl transaction is the auto-block reconcile, usually triggered by a new entry from outside activity. The block lists and `_gb.auto-block.*` leave the live VPP interface with no log line, metric or response anyone reads. The operator learns of it only at the next validate or commit.
  - Two texts say the drop is "never silent" / "not silent" and name the auto-block reconcile first, which is not yet true: the comment at `global_blocking.go:106-108` and the tech-debt row at `docs/tech-debt.md:226`.
  - **Fix (any one):**
    - In `reconcileAutoBlockLocked`, log `resp.GetValidation()` warnings of rule `acl.global-blocking` at Warn, once per change of the skipped set (dedupe on the message) so the 1 Hz loop does not spam.
    - Do the same in `fullResync`.
    - Or reword the comment and the debt row to "named in validate and commit responses; the auto-block reconcile and resync do not surface it", and extend the row.

**MINOR-2: partially fixed.**
- **Fixed:**
  - The status report (`docs/status/tasks/agent-allifs-unbound-nics.md:155`) now says "fail-closed for commits, fail-open for auto-block".
  - Tech-debt row `docs/tech-debt.md:227` covers (i) a schema refusal of, or an agent skip-with-warning for, explicit references to dropped rows, and (ii) decoupling host nftables auto-block from VPP binding errors. Owner manager, due 2026-11-14 (proposed).
- **Not fixed:** the PR body.
  - `gh pr view 241` (headRefOid `873f1d946…`, 0 comments) still says at body line 10: "a list that names an unbound NIC still binds it (and fails closed, like any explicit reference …)".
  - The status report's review table ("#2 wording corrected") therefore over-states this.
  - **Fix:** `gh pr edit 241` with the corrected sentence before merge.

**MINOR-3: fixed.**
- `docs/user/security/global-blocking.md:27` now says:
  - a NIC awaiting the engine is covered once bound;
  - a released NIC is never covered by the list, and its traffic to the appliance is covered by *Protect the box*;
  - automatic blocking targets the same set.
- This is accurate for released NICs. For an awaiting NIC, "covered once it is bound" implies it is not covered before.
- The status report now says #238's paragraph is "replaced (not dropped)", as I asked.
- The wording itself is R6's call. See V-1 for a new over-claim in the same sentence.

**MINOR-4: fixed as a tech-debt row.** `docs/tech-debt.md:228` (sub-interfaces; owner manager; due 2026-11-14, proposed). The row correctly states the top-level-rows-only behaviour and that `in__gb` still covers host-destined traffic.

### Change-specific checks requested

**Warning content.**
- The text is the list name (`%q`) plus a `%s` join of interface row names. No auto-block entry, prefix or address appears in it.
- The row names are format arguments, never the format string.
- They are schema-restricted to `^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9a-fA-F]+)*$`, max 63 characters (`packages/schema/src/domains/interfaces.ts:43-49`). No newline, quote or `;` can be injected.
- The message goes to a protobuf `ValidationIssue`; there is no shell, file or log sink.
- Size is bounded by rows × 63 bytes, with one warning per all-interfaces list. Nothing leaks.

**Classification of rows absent from `kept`.**
- `SplitUnboundPhysical` (`hostnics.go:18-36`) excludes exactly two kinds of row, both only for KindExisting rows with `physical` set:
  - `owner == "host"` → released, decided by configuration alone;
  - an engine-owned row whose netdev lookup returns exists with no kind → unbound.
- `globalBlocking` (`global_blocking.go:118-127`) re-labels absent rows with the same predicate (`physical.owner == "host"` → released, else unbound). The label therefore always matches the reason and depends only on the configuration.
- The kernel observation can only move an engine-owned row from kept to unbound. That is the pre-existing name-based decision (MINOR-1 (c) row). It cannot relabel or release a row, and a lookup error keeps the row.
- Auto-block entries, the attacker-influenced input, only add the `auto-block` list (`autoblock/runtime.go:123`). They never touch interface rows.

### New findings

**V-1 (MINOR): the new user-doc sentence over-claims coverage for VLAN sub-interfaces.**
- **Where:** `docs/user/security/global-blocking.md:27`.
- **Problem:** "*All interfaces* means every interface the data plane has". VLAN sub-interfaces are interfaces the data plane has, but they are never targeted (MINOR-4, `tech-debt.md:228`).
- **Scenario:** an operator relies on this sentence and believes transit traffic arriving on a dot1q sub-interface is filtered by an all-interfaces list or by auto-block. It is not. Only host-destined traffic is dropped, by `in__gb`.
- The schema help (`packages/schema/src/domains/ext/global-blocking.ts:66`, "every L3 interface") already over-claimed. This PR restates it more strongly in new text.
- **Fix:** say "every top-level interface the data plane has (VLAN sub-interfaces are not covered yet)". The `tech-debt.md:228` row already lists this as a fix option. Wording is for R6.

**V-2 (NIT): released NICs raise a warning on every acl-carrying projection.**
- **Where:** `global_blocking.go:163-172`.
- **Problem:** on a typical appliance the management NIC is released to the host. With auto-block enabled, every security or interfaces commit and every validate now carries an `acl.global-blocking` warning for the overlay list `auto-block`. That is a permanent, expected state, which the interfaces projection reports only as Info (`agent.nic-released`, `projection.go:250`).
- **Risk:** the warning becomes noise, and operators learn to ignore the one that matters in MINOR-1's name-clash case.
- **Fix (optional):** emit the warning only when `unbound` is non-empty, naming released rows inside it, or drop the released rows since the user doc now documents them. Adjust the two test assertions that expect the released-only warning.

### Notes for other reviewers
- **[other: R7]**
  - The PR #241 body is still the round-1 text:
    - it describes D-250 and "`desired.ACL` takes the Linux netdev lookup … re-split", not D-251 with kept rows;
    - "The user-doc exception paragraph can then be dropped" contradicts the status report's "replaced (not dropped)";
    - "`PR #@@N@@`" is unfilled;
    - it still says "quick: running" and review "in progress".
  - The status report marks R7 #7 (PR body) as handled.
  - `gh` reports the PR `mergeable: CONFLICTING` at `873f1d946`. I did not check this further; read-only means no merge-tree.
- **[other: R6]** In commits and validates with auto-block enabled, the new warning's pointer is `/acl/globalBlocking/lists/auto-block/interfaces`. That is the runtime overlay list, not a list the user can see in the configuration.

### Commands run (outputs)

```
$ git rev-parse HEAD          -> 873f1d946a0c8e65f08603db4891025560bcaa66
$ git log --oneline 552b9bcce..873f1d946
873f1d946 docs(status): report and WIP after the review fixes and the gates on 0b57dc070 (PR #241)
0b57dc070 docs: D-251, deferred acceptance, tech-debt rows and the user-doc sentence for PR #241
856bf743a fix(agent): acl block lists reuse the projection's kept rows and name the NICs they skip
20c3303fd docs(status): save review R4 hand-back for agent-allifs-unbound-nics (APPROVE WITH CHANGES)
c8f3a608a docs(status): save review R1 and R2 hand-backs for agent-allifs-unbound-nics (both APPROVE)
736c45aa9 docs(status): save review R7 hand-back for agent-allifs-unbound-nics (BLOCK: D-250 clash, merge order)
7afee7aef docs(status): quick gate PASSED for agent-allifs-unbound-nics
$ git diff --stat 552b9bcce 873f1d946   -> 16 files changed, 1263 insertions(+), 128 deletions(-)

$ grep -rn "desired\.ACL(\| ACL(" apps/agent --include=*.go   (production callers)
apps/agent/internal/agent/projection.go:411:		desired.ACL(p, ds, in, boundIfs)
  (all other hits are *_test.go, passing nil or a kept map from SplitUnboundPhysical)
$ grep -rn "SplitUnboundPhysical\|LinuxNetdevKind" apps/agent --include=*.go | grep -v _test.go
  projection.go:242 (the one split with the lookup); global_blocking.go:115 (nil lookup, only when kept == nil); subsystems wiring
$ awk 'NR>=193&&NR<=520' projection.go | grep -nE "\bds\s*=|ds\.Interfaces\s*=|boundIfs\s*=|boundIfs\[" -> exit 1 (no reassignment)
$ grep -nE "\bifs\[[^]]+\]\s*=|delete\(ifs|boundIfs\[[^]]+\]\s*=|delete\(boundIfs" desired/interfaces.go desired/lcp*.go desired/pppoe*.go agent/projection.go -> exit 1 (kept map not mutated)
$ sed -n 100,117p apps/agent/internal/agent/rpc_autoblock.go
	resp, _ := s.applyLocked(ctx, modeRuntime, "", s.st.desired, []string{"acl"}, 0)
	if resp.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		return 0, status.Errorf(codes.Unavailable, "auto-block enforcement: %s", resp.GetMessage())
	}            <- resp.Validation (the projection warnings) is not read
$ grep -rn "GetValidation()" apps/agent/internal --include=*.go | grep -v _test   -> (no output)
$ sed -n 489,494p apps/agent/internal/agent/agent.go
	resp := a.svc.Resync(ctx)
	if resp != nil {
		a.log.Info("resync finished", "why", why, "status", ..., "summary", ...)

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json title,state,headRefOid,baseRefName,mergeable,body
fix(agent): all-interfaces block lists skip unbound and released NICs
OPEN
873f1d946a0c8e65f08603db4891025560bcaa66
main
CONFLICTING
$ grep -n -i "fail.\{0,3\}closed\|fail.\{0,3\}open" <pr-body> docs/status/tasks/agent-allifs-unbound-nics.md docs/status/tasks/agent-allifs-unbound-nics-wip.md
docs/status/tasks/agent-allifs-unbound-nics.md:155:- Explicit references ... — fail-closed for commits, fail-open for auto-block. ...
<pr-body>:10:- Explicit interface lists are unchanged: a list that names an unbound NIC still binds it (and fails closed, like any explicit reference to an interface the data plane does not have).
$ gh pr view 241 --json comments,updatedAt,commits --jq ...
{"comments":[],"lastcommits":["856bf743a","0b57dc070","873f1d946"],"ncomments":0,"updatedAt":"2026-10-17T12:09:43Z"}

# secret scan of the delta: scratch copy (git show 873f1d946:<f>) of all 16 changed files
$ gitleaks version -> 8.30.1
$ gitleaks detect --no-git -s <scratch>/copy --redact --no-banner -v
INF scanned ~474971 bytes (474.97 KB) in 855ms
INF no leaks found                         (exit 0)
$ git diff 552b9bcce 873f1d946 | gitleaks stdin --redact --no-banner
INF scanned ~166347 bytes (166.35 KB) in 611ms
INF no leaks found                         (exit 0)
$ grep -rnE 'BEGIN (RSA|EC|OPENSSH) PRIVATE|password\s*[:=]\s*[^<]' <scratch>/copy   -> exit 1 (no match)
$ git diff 552b9bcce 873f1d946 | grep '^+' | grep -niE 'ghp_|github_pat|gho_|Bearer [A-Za-z0-9]|xox[abp]-|AKIA[0-9A-Z]{12}|-----BEGIN|api[_-]?key\s*[:=]|secret\s*[:=]|token\s*[:=]|@gmail|172\.30\.|psk'
  2 hits, both my own round-1 grep commands quoted in the saved R2 report (no secret)
$ grep -rnE 'x-access-token|https?://[^ /]*:[^ /]*@|AUTH(ORIZATION)?=|_TOKEN=|_SECRET=|passwd|private key' <scratch>/copy -> exit 1
  added-line IPs: 10.71.1.1/24 (test), 127.0.0.1 (pasted test log), 192.0.2.7/32, 198.51.100.1/32, 203.0.113.9/32 (documentation ranges)
  added-line paths: the worktree path, /root/ngfw-wt/logs/ci/..., go-test /tmp dirs in pasted output; no emails; URLs are github.com/mcoder1001-cyber/NGFW only

# interface-name alphabet (warning injection)
$ grep -n "parentInterfaceName\s*=" -A6 packages/schema/src/domains/interfaces.ts
  .string().min(1).max(63).regex(/^[A-Za-z][A-Za-z0-9_-]*(?:\/[0-9a-fA-F]+)*$/, ...
$ grep -n "AllInterfaces" apps/agent/internal/autoblock/runtime.go
123: out.Acl.GlobalBlocking.Lists[ListName] = &ngfwv1.GlobalBlockingList{... AllInterfaces: true, ... ProtectHost: true ... Entries: prefixes ...}   (overlay adds a list only; interface rows untouched)

# targeted tests (from apps/agent, head 873f1d946)
$ env -u NGFW_INTEGRATION go test -count=1 -v -run 'TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs|TestGlobalBlockingAllInterfacesNothingBound|TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs|TestProjectAutoBlockOverlayAddsNoNetdevLookups|TestAutoBlockWithUnboundAndReleasedNICsOnFake|TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake|TestSplitUnboundPhysical|TestProjectPhysicalNic' ./internal/desired/ ./internal/agent/ | grep -E '^(--- |ok|FAIL|PASS)'
--- PASS: TestGlobalBlockingAllInterfacesSkipsUnboundAndReleasedNICs (0.01s)
--- PASS: TestGlobalBlockingAllInterfacesNothingBound (0.00s)
PASS
ok  	ngfw/agent/internal/desired	0.162s
--- PASS: TestProjectAutoBlockOverlaySkipsUnboundAndReleasedNICs (0.02s)
--- PASS: TestProjectAutoBlockOverlayAddsNoNetdevLookups (0.00s)
--- PASS: TestAutoBlockWithUnboundAndReleasedNICsOnFake (0.08s)
--- PASS: TestAllInterfacesBlockListWithUnboundAndReleasedNICsOnFake (0.05s)
--- PASS: TestProjectPhysicalNicAwaitingDataplane (0.00s)
--- PASS: TestProjectPhysicalNicBoundProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicNoLookupProjectsNormally (0.00s)
--- PASS: TestProjectPhysicalNicLcpTapIsNotUnbound (0.00s)
--- PASS: TestProjectPhysicalNicUnboundHasNoLcpPair (0.00s)
--- PASS: TestProjectPhysicalNicReleasedToHost (0.00s)
PASS
ok  	ngfw/agent/internal/agent	0.314s
$ env -u NGFW_INTEGRATION go test -count=1 -run 'ACL|GlobalBlocking|AutoBlock|HostACL|PhysicalNic|SplitUnbound|NICs' ./internal/desired/ ./internal/agent/
ok  	ngfw/agent/internal/desired	7.397s
ok  	ngfw/agent/internal/agent	7.283s
$ git rev-parse HEAD -> 873f1d946a0c8e65f08603db4891025560bcaa66 ; git status --porcelain | wc -l -> 0
```

These are code-reading claims, not tested: that the auto-block reconcile and the resync discard the warning (V-1 residual of MINOR-1 (a)), and that the scheduler deletes the binding in an acl-only transaction (unchanged from round 1). No experiment was run, because the allowed command set does not include building a modified copy.

### Other security checks on the delta
- **Injection / shell-outs:** no `exec.Command`, shell, `vppctl` or file writes added.
- **AuthN/authz:** no route, API, proto or schema change; no auth change.
- **Dependencies:** `go.mod`/`go.sum` unchanged.
- **Input limits:** unchanged; the warning is bounded by row count.
- **DEFERRED-ACCEPTANCE:** the section keeps the shared-host boundary ("never on the shared host's real NICs").

### Verdict

**APPROVE**: 0 BLOCKER, 0 MAJOR.

| Finding | Status |
|---|---|
| MINOR-1 | partially fixed: (b) fixed, (c) debt row; (a) residual open, the warning is dropped by the auto-block reconcile and resync |
| MINOR-2 | partially fixed: the PR body still says "fails closed" |
| MINOR-3 | fixed |
| MINOR-4 | tracked as tech-debt |
| V-1 (new) | MINOR, open: "every interface the data plane has" over-claims for VLAN sub-interfaces |
| V-2 (new) | NIT, open: released NICs warn on every acl-carrying projection |

Open: 3 MINOR (the two residuals and V-1) and 1 NIT, all optional under the shared rules. I recommend before merge, as cheap and in scope:
- correct the PR body;
- either log the warning in `reconcileAutoBlockLocked` or reword the "never silent" comment and the debt row;
- qualify "every interface" in the user doc.
