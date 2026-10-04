# Three ready rows: source integration evidence

Selected by the owner's instruction: F-mpls-ldp-host, F-pim-frrsync and F-bruteforce-detectors. Their task implementations and independent R1–R8 reviews are preserved in source PRs #142, #143 and #144 and archive refs. D-233 consolidates the reviewed branches into single-commit PR #145.

## Exact candidate

- PR: https://github.com/mcoder1001-cyber/NGFW/pull/145
- Head: `47c91a1efd85a66b97926d0a38f5b76c6e77345f`
- Tree: `98cac96651d6dc0e3560cf5e3fc75838547ca9d7`
- Base main: `06e4368ce857e2259fd820fc2bcccc3b00194b8e`
- Preserved reviewed history: `archive/three-ready-reviewed-20261004` at `f05f19a6db02e5b8e59a43c2bbebcc6fe4022ac4`
- Previous full hosted quick: https://github.com/mcoder1001-cyber/NGFW/actions/runs/37177580917 — FAIL: only TestRequestedResyncsAreRateLimited; no merge or aggregate PASS asserted. All 35 turbo tasks, agent lint, LDP/PIM/detector packages and all subsystem tests passed. The initial inactive AutoBlock heartbeat emitted an unrelated authoritative ACL transaction; the initial inactive-runtime guard correction is now independently approved without changing the original rate-limit assertions.

- Current complete hosted quick: https://github.com/mcoder1001-cyber/NGFW/actions/runs/37178873092 — PASS, job111367152248; gate wall time17m30s. Every mandatory phase passed: generated-output/contract/security/slot checks,35 turbo tasks, complete agent and CLI lint/race-test/build, all19 test modules, shellcheck,149 apply-startup fake-host checks across2 shards.

## Source and verification

LDP and PIM use dedicated bounded dynamic sources, retain known state after read failures, retry failed applies and preserve ownership boundaries. Detectors add validated destination ports and bounded distinct-port sliding windows through the existing trusted API subscriber. Supported source limits and unsupported forwarding families are explicit in task reports and the accepted scaling debt.

The first hosted source runs found exported-comment lint errors and an existing main auto-block ACL/PBR regression. Comments and a checked installed-route counter corrected lint. The actual ACL regression was unconditional descriptor scope expansion on unrelated interface/security requests. Conditional effective projection scope now preserves unrelated ACL references, refreshes active overlays and cleans up removed overlays. Apply, DryRun, drift and dynamic planning agree while requested/persisted authority remains unchanged. The original PBR assertions are retained; new preservation/active-overlay/removal/inactive-context regressions pass. Independent R1 and R2 approve frozen fix `009bdda0dd9be21799832ce33d08f4c7c8990a17`.

Focused Go race/vet, pinned lint, API 31 tests, topology-driver 4 tests, source/security checks and actual generation checks passed. Local complete quick was NOT PASS because this environment blocks Unix sockets/netlink and chown; those results are not substituted for hosted quick. Workflow/gate scripts and tracked generated contracts were not changed.

Real FRR/VPP/nft/PostgreSQL/Valkey and packet acceptance remains NOT RUN in `docs/status/DEFERRED-ACCEPTANCE.md`. Source completion does not assert forwarding or enforcement acceptance. LDP: 256 EOS IPv4 dynamic routes, no NEOS/IPv6. PIM: 256 dynamic routes in global table 0, no non-default VRFs. Detector bounds fail unavailable rather than inventing enforcement.

Task developers and reviewers completed their session work; they are not a persistent background service. Other board running rows do not prove live workers. Exactly these three rows are reconciled as merged after an observed approved green merge. Source progress is160/211 tasks; no additional ready row was started.

Failed candidate head is preserved as `archive/three-ready-ci1-20261004`. Full agent artifact: https://github.com/mcoder1001-cyber/NGFW/actions/runs/37177580917/artifacts/11294565135. No laboratory case or aggregate gate is promoted to PASS.

The inactive-maintenance fix `7a98a307fdab4ad4eb1a84b773114c2c764ba9ca` received R1/R2 approval. Real watcher regressions cover unconfigured/disabled inactivity, dirty-empty cleanup and active cached-entry expiry in both VPP and host projections. Independent GOMAXPROCS=2 race count3 passed16.277s. The original resync test is unchanged and passed within the complete hosted agent race suite; local Unix socket restrictions were not treated as a pass.

Independent remaining gate evidence: all19 test-module formatting/vet/unit phases passed, CLI lint/race tests passed, and supplemental compile passed. Exact local build/VCS, shellcheck and fake-host socket limits are NOT PASS; the complete hosted gate now passed those exact phases. Test report checkpoint `dc2f3e136ed9465986f9aca81adda59de7e9d4e5` is preserved on `test/remaining-gate-20261004`.

## Observed integration

PR #145 was squash-merged to main at `1a4b7f766076059f05fbfb842415514b300767e6` after approved green head `47c91a1efd85a66b97926d0a38f5b76c6e77345f`. The main tree is exactly `98cac96651d6dc0e3560cf5e3fc75838547ca9d7`, matching the tested candidate. Board reconciliation marks only the three selected source rows merged; lab acceptance remains NOT RUN. The post-merge documentation/board update does not change product sources, workflows or gate scripts.

Post-merge main CI was observed in progress at https://github.com/mcoder1001-cyber/NGFW/actions/runs/37179863662 on the product merge SHA. This is distinct from the already successful required pre-merge full quick run; no second main-run PASS is asserted. The tested and merged product trees match exactly. Final task counts: merged160, review8, running13, ready7, parked8, failed0, todo15; overall estimated-hours source completion75.3%.
