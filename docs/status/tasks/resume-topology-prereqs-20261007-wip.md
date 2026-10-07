# Topology prerequisite recovery WIP

## Final audit handoff (supersedes initial checkpoint notes below)

Audit deliverable complete; neither feature is declared fully accepted. Product edits: none. Owned files: this WIP and its adjacent envelope only. No live topology, shared VPP API/CLI attempt, daemon launch, package/unit/sysctl/security change, P12 runner change or VPP C-track edit.

Local/verified published initial checkpoint: `9c7b063e04c46bfc6e7c10c2faa23610373eba64`, tree `3f85d51409ed32bfb5edb287310f39aceb46b144`. Initial CLI push received a generic remote rejection; immediate retry succeeded and read-only ls-remote independently returned that exact SHA. Final audit documentation checkpoint descends from it. Resolve its exact SHA with `git rev-parse HEAD` and compare `git ls-remote origin refs/heads/codex/resume-topology-prereqs-20261007`; these two documents cannot embed their own containing commit hash. Local and remote receipts will be reported after publication.

### Recovery sources and current truth

Base main `3ddb1680e475e94d43e8036cd3776bc60c87208b`: hosted CI gate run 37590128647 SUCCESS. Board validation finds 212 tasks; both requested host rows are parked. This audit does not mutate board or deferred ledger. The old OSPF row still cites the September shared-VPP wedge, while newer private acceptance evidence is already in main. PPPoE row calls its blocker laboratory-only, which is insufficient given current product failures. Manager should reconcile these descriptions without marking either whole feature DONE.

Original RV-A R1/R4/R7 reports are absent from current main but were recovered read-only and read from commit `5afe17976493f72db07e5628ad628fcbe33d127f` using `git show <commit>:docs/status/tasks/RV-A-review-R{1,4,7}.md` (one file per command). Their OSPF owed list is FRR canonical render/readers, Full peers, 100 routes in VPP, multicast delivery, withdrawal, restart, rollback, API validation, gate and cleanup. Existing sources and evidence replace rebuilding those tests.

Current PPPoE recovery branch observed `4bcf9f4977e2a9c248548de23cf552e4bcef94da`; its authoritative frozen WIP names tested source `88fca3168e775fd0bbf699bdc9e82c2295091f00`, tree `afbd6626295e20d290519ecbd10da4df932dfbef`. Original PR196 remains `992b264b2084a8adfcee755d2b5650b771a6a8f1` with Mandatory quick SUCCESS (run 37583104528). That original gate is not the final recovery integration gate. Correctness-review remote observed `abf6a64252e5591842cc0a23bda14bacfe912175`, WIP says independent race/review pending; security remote `e222dfd27b7044a5741932de05b789bded5ab662` contains provisional findings on older source. Do not reuse its old BLOCK findings as proof the frozen fixes are still missing, or author controls as an independent APPROVE. Manager must obtain exact frozen-source verdicts and unchanged complete hosted quick on its final integration tree. Other worker liveness: unverifiable; published progress does not certify a live process.

### OSPF recovered evidence and acceptance path

| Existing evidence on main | Proven scope and limitation |
|---|---|
| `F-ospf-host-evidence/frrtest-ospf.txt`, host status | Historical real FRR canonical config/DryRun/readers/poller/50-prefix withdrawal pass. September topology failure was real, not a current need to restart shared VPP. |
| `closeout-ospf-evidence/netns.log`, matching WIP | Private netns v2 PASS 63.99s, matching veth/TAP MTU9000, Full/100 FRR routes/restart7.4s/rollback; FRR RIB only. |
| `closeout-ospf-fib-evidence/root-private.log` | PASS59.16s; two Full peers, FRR100/VPP lcp-rt-dynamic100; withdraw50 in300ms/reannounce100; post-restart checks repeated; restart7.6s; rollback FRR/VPP0; shared MainPID1014/NRestarts0 unchanged. |
| `closeout-ospf6-private-evidence/actual-ipv6.txt` | Source a4340a8f6a21710c9ab9b3e22aa9e405045045a5, PASS57.48s; actual ospf6d Full parser, IPv6 FRR100/VPP100; withdrawal93ms/restoration118ms; restart7.48s; rollback zero/config absence/Retrieve nil; shared service unchanged. |

Raw v2/v3 logs were inspected for actual Full/FIB counts, withdrawal, rollback, PASS and shared-service receipts. They are historical source-specific proofs, not freshly executed acceptance. Driver/evidence provenance is reachable in main via `e3517a98353dd081558465adcf2bac04eaae3aad` (v2) and `e9de20c0699dd0c865e1e4ce1af4887d569032ad` (v3). `closeout-api-final-additional-review.md` explicitly APPROVEs the private v2 driver/regular-file daemon launcher boundary and reviews raw evidence; it does not certify every current OSPF obligation.

Safe existing path: `test/topology/ospf/private-fib.py` unshares network AND mount namespaces, rejects the host network inode, allows only initial lo/ip6tnl0, binds private /run/netns and /run/frr, then calls `hardware-smoke/isolated-vpp.py`. That inner driver starts an owned disposable VPP with linux_cp/linux_nl enabled and binds its sockets over /run/vpp only in its private mount namespace; `NGFW_ISOLATED_TEST_RUN=1` also privatizes /run/ngfw-test. The same private network view contains zebra and linux_nl, making root-mode FIB proof possible without shared-host root FRR. Existing locks, strict FIB assertions, MTU checks, PID-owned teardown and service-before/after checks remain. `ospf6-private/run.py` reuses that boundary with a temporary Go overlay and actual IPv6 helper; it rejects skip-only success and requires `REAL_OSPF6_PRIVATE_IPV6_LIFECYCLE=PASS`.

Exact prerequisites for the manager's acceptance worker:

1. Reserve slot6, isolated worktree/branch and FRR ownership; verify slot6 has no running transient VPP/daemon/worker. Both private drivers hard-code slot6. No slot is allocated to this auditor. Check published driver/source matches the intended integration head; preserve all assertions.
2. Use a host permitting root mount/network unshare and existing vpp/vppctl/FRR binaries. No shared service restart, installation or privilege change is needed for this path. This audit verified binaries/syntax, not runtime unshare capability or resource availability.
3. After `eval "$(tools/lab env 6)"`, socket exports must name `/run/vpp/{api,cli,stats}.sock` and agent counterparts, which the private child remaps. Refuse an active slot6 VPP: tools/lab env would instead point to external slot sockets, and ospf6/run.py refreshes those exports itself. Clear inherited NGFW_ISOLATED_PLUGIN_PATH and unrelated test overrides. Do not run the bare NGFW_OSPF_FIB=root driver on shared host. Ordinary `tools/lab vpp up 6` disables linux_cp/linux_nl, so it is not this acceptance path.
4. Run sequentially under tools/heavy.sh, with worktree-local TMPDIR/GOTMPDIR and bounded timeout. Record exact integration SHA, no SKIP, namespace/PID cleanup and shared MainPID/NRestarts before/after. These are future commands, NOT executed here:

```sh
# In the acceptance worker's own checkout, after the prerequisites above:
eval "$(tools/lab env 6)"
timeout 900s tools/heavy.sh python3 test/topology/ospf/private-fib.py
timeout 900s python3 test/topology/ospf6-private/run.py
```

Remaining OSPF acceptance: authenticated peers/key rotation (existing sealed channel; do not request a new secret architecture), authenticated real API neighbors matching FRR and UI en/fa, licensed API undefined-area/backbone-stub 400 pointers and candidate/commit/rollback workflow. September API evidence proved undefined-area 400 but backbone-stub returned503. Existing api400.sh is NOT itself a complete isolated launch: it starts slot DB/API/agent against the selected host. Manager must assign a worker with full private stack ownership/approved launcher before that run; standalone execution is not authorized here. Private FIB logs do not prove general IPv6 packets, non-default VRF/scale or physical NIC/DPDK. Recover current browser/API evidence before scheduling any missing case; no fresh screenshot or API success claimed by this audit. P12 runner repairs remain manager-owned and are not a prerequisite to rebuilding these existing OSPF drivers.

### PPPoE current product gaps, historical mismatch and human request

Read-only local prerequisite observation: `/usr/sbin/pppd`, `/usr/sbin/pppoe-server`, `/usr/sbin/dhcpcd`; installed ppp2.5.2-1+1.2, pppoe4.0-1ubuntu2, dhcpcd-base1:10.3.0-7, FRR10.7.1-0~ubuntu26.1; `/dev/ppp` is a character device. Shared VPP service active/MainPID1014/NRestarts0. These observations do not prove dialing; no package request is presently justified for these tools.

Current main and the published recovery client mirror add addresses, ordinary physical-interface FIB paths and MSS clamp; these are not a PPPoE packet encapsulation path. Namespace-supervised production clients still refuse `pppoe.netns-unavailable`; slot clients render with `pppoe.not-supervised` and cannot use host systemctl. Existing versioned sealed password delivery is implemented; the old nil-resolver/config-wiring blockers are superseded.

Verified read-only source mechanism:

- `/root/vpp/src/plugins/pppoe/pppoe.c:739-743` registers session0x8864 and discovery0x8863 at plugin init. `linux-cp/lcp_interface.c:1246-1247` rejects an already registered EtherType; LcpEthertypeEnable cannot override it.
- `pppoe_cp_node.c:132-144` sends CP-originated packets by destination-MAC lookup in server learned-link state (broadcast client PADI has no learned AC destination); physical-side discovery targets the global `pem->cp_if_index` at174, not each WAN's LCP pair. Global server PppoeAddDelCp is not an approved multi-WAN client solution.
- `apps/agent/internal/descriptors/pppoe/client.go` defaultRoute uses an ordinary interface FibPath; no client session/encapsulation DPO is created. Server session creation `pppoe.c:308-315` requires a learned client MAC and link entry. Current hooks lack the AC/session identity needed to implement that mapping.
- Published `claude-pppoe-ipv6-evidence/13-product-dhcpv6-run.txt` records real product FAIL127.49s, discovery unable to complete/no PADO, exit1, owned cleanup and shared service unchanged. Diagnostic SLAAC/DHCPv6 passes bypassed the product discovery conflict; they cannot count as product dial or LAN/NAT forwarding. Recovery user docs explicitly disclose both IPv4/IPv6 transit unsupported.

Historical contradiction recovered: commit `4d0260dc7` contains F-pppoe-client-host.md marked DONE with reported IPv4 NAT PASS59.50s. Its report explicitly requires native session/AC metadata, a private rebuilt plugin and `deploy/vpp/patches/0002-pppoe-safe-cp-removal-and-session-dump.patch` revision2. Current main has no such patch and no `test/topology/pppoe` directory; its 0002 patch is IKEv2. Preserve that older report as evidence of a different implementation, not acceptance of current main or authorization to restore VPP C/security changes. A prior Linux-masquerade pass in the same historical report likewise does not prove VPP NAT. Current published driver reconstruction instructions point to historical source plus a driver patch; there is no current turnkey product topology driver in main to run here.

Actionable manager workplan:

1. Finish exact-source independent re-review and hosted quick for the already published bounded IPv6 lifecycle candidate; do not rebuild it or duplicate author fixes. Keep full PPPoE feature incomplete.
2. Assign a separate product design task to resolve discovery/server coexistence and IPv4/IPv6 LAN forwarding/encapsulation. First compare current mirror-only code with historical 4d0260dc7 and document which previously reviewed mechanism, if any, remains permissible. The human request is: **approve the intended supported PPPoE client datapath and server coexistence scope (including single vs multiple WANs), and explicitly authorize any required VPP C/version or host privilege/security boundary changes in a separate envelope.** Manager may prepare/read-only compare options now; this auditor has no authority for those implementation changes. Plugin disabling/global registration changes or Linux NAT are not product fixes to claim accepted here.
3. After product mechanism approval/implementation, assign an owned fully private mount/network/VPP/API/agent/PPP/AC acceptance environment. Preserve packaged dhcpcd runtime/privsep descendants, private /etc/ppp, /var/lib/dhcpcd, /run/dhcpcd, /var/lib/kea and runtime unit paths; prior probe records disclose shared-file writes, so those probes must not be rerun bare. Normal slot netns refusal must remain until a separately reviewed supported supervision path exists.
4. Run actual config/secret→dial; VPP NAT IPv4 and IPv6 LAN traffic/counters/capture; server restart≤holdoff+10s; explicit reconnect; wrong-password exit19/actionable state; credential rotation; agent restart with sealed versions; down/removal/rollback including no late writers; password absence in GET/logs/audit without printing matched material; browser real-state en/fa. A hook stand-in or mirrored default route clears only its stated wiring acceptance. Delegated prefix is reported, not assigned to LAN; LAN-PD distribution needs a separately specified contract/owner scope if requested.

### Actual host-independent verification on this audit branch

Executed with NGFW_INTEGRATION/NGFW_OSPF_TOPOLOGY unset; no live integration skips are counted as acceptance:

```text
tools/ci.sh check --base origin/main
check PASSED (0m14s); board valid212; slot scheme30 developers+CI12,964ports,32ranges; gitleaks no leaks

cd apps/agent
env -u NGFW_INTEGRATION -u NGFW_OSPF_TOPOLOGY -u NGFW_HEAVY_HELD ../../tools/heavy.sh timeout 240s go test -race -p 2 -count=1 -timeout 120s ./internal/renderers/frr/ospf ./internal/renderers/pppoe ./internal/descriptors/pppoe -run 'Test(RenderThroughFramework|VRFAndEmpty|RenderErrors|ParseNeighbors|OSPF6.*|RenderGolden|ReadState|SecretsAreSecretAndNotInPeerFile|HostileInputRejected|Client.*)$'
ok ngfw/agent/internal/renderers/frr/ospf 1.262s
ok ngfw/agent/internal/renderers/pppoe 1.198s
ok ngfw/agent/internal/descriptors/pppoe 1.387s

Python ast.parse of private-fib.py, ospf6-private/run.py, isolated-vpp.py: all PASS, no bytecode/file writes
bash -n test/topology/ospf/run.sh test/topology/ospf/api400.sh: exit0
shellcheck same two scripts: exit0
git diff --check: exit0
```

Final documentation-tree `tools/ci.sh check --base origin/main` rerun: check PASSED (0m26s), gitleaks no leaks, board/slot validation passed; accompanying `git diff --check` exit0. Final remote main re-observation remains 3ddb1680e, CI37590128647 SUCCESS; PPPoE source branch remains4bcf9f497.

No full local quick or final hosted gate executed by this audit. Required complete quick before any integration remains unchanged; green main/original PR and these focused checks do not substitute. No product test failure observed in selected checks. Current unresolved failures are PPPoE discovery/transit; remaining laboratory obligations listed above are explicit. Manager owns board/ledger reconciliation and independent documentation review; no merge performed.

Exact next recovery command: `cd /root/ngfw-wt/resume-topology-prereqs-20261007 && git status --short && git rev-parse HEAD HEAD^{tree} && git ls-remote origin refs/heads/main refs/heads/codex/resume-topology-prereqs-20261007`. Then hand this plan to assigned acceptance/design owners; do not execute a live attempt from this read-only envelope.

## Initial published checkpoint (historical)

Branch/worktree and file ownership: see adjacent envelope. Base local and remote main: `3ddb1680e475e94d43e8036cd3776bc60c87208b`. First audit checkpoint publication pending; no successful publication claimed yet. Exact local SHA: `git rev-parse HEAD`; exact remote SHA: `git ls-remote origin refs/heads/codex/resume-topology-prereqs-20261007`. A commit cannot contain its own hash; use these commands for descendant documentation checkpoints.

Completed: required instructions/prompts read; origin fetched; GitHub main/branches/open PRs/CI inspected; requested branch/worktree created from origin/main. Only these two documents are writable. Main hosted CI run 37590128647 SUCCESS at the base above. Open PRs observed: 180 (P12), 193 (RA), 196 (PPPoE IPv6). Worker liveness inventory unverifiable; board owners are not live-process evidence.

Recovered OSPF evidence: closeout-ospf-wip.md netns PASS 63.99s (FRR RIB only), closeout-ospf-fib-wip.md private v2 FIB PASS 59.16s (100 dynamic routes, withdrawal/recovery, restart 7.6s, rollback zero), closeout-ospf6-private-wip.md private v3 PASS 57.48s (100 IPv6 routes, restart 7.48s, rollback zero). Historical evidence only, no rerun claimed. Existing private-fib.py and ospf6-private/run.py avoid shared network/root FRR. Both require slot 6; manager must reserve it before any authorized acceptance worker uses them. Generic tools/lab vpp up disables linux_cp/linux_nl and is not sufficient for these proofs.

Recovered PPPoE source: original PR196 `992b264b2084a8adfcee755d2b5650b771a6a8f1`, recovery branch observed `4bcf9f4977e2a9c248548de23cf552e4bcef94da`; original PR Mandatory quick SUCCESS. Read published recovery WIP and support disclosures. Product discovery and IPv4/IPv6 LAN transit remain unsupported. Current read-only command lookup finds /usr/sbin/pppd, /usr/sbin/pppoe-server, /usr/sbin/dhcpcd; historical package-install blocker alone is insufficient. Shared VPP read-only service observation: active, MainPID1014, NRestarts0. No live attempt or VPP API operation performed.

Actual tests: pending focused host-independent checks. Current failure: stale OSPF board/deferred descriptions and PPPoE product failures, not a demonstrated missing package. Remaining audit: verify raw evidence/provenance, inspect isolated boundaries and mirror source, run existing focused unit/static checks, publish exact actionable acceptance/human prerequisites. Remaining product code: none authorized here; PPPoE discovery/coexistence and forwarding mechanism require separate assignment/design.

Exact next command: `cd /root/ngfw-wt/resume-topology-prereqs-20261007 && tools/ci.sh check --base origin/main` followed by commit/push of these two files only. No P12 runner change.
