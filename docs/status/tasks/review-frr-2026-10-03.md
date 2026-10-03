# Independent FRR review — 2026-10-03

Reviewed the current working diff in `internal/renderers/frr/isis/isis.go`, `renderer.go`, `isis/render_test.go`, `isis/testdata/full.golden` and `isis_removal_test.go`. This reviewer did not author these changes. No product edits, new test/build slot or shared VPP operation were performed.

**Approved within this scope; no actionable correctness finding.**

Canonicalization in `isis.go:112–122` omits the FRR 10.7 default router level `level-1-2` and default wide metric style. The level-1/level-2 explicit rendering branches remain. In `:225–229`, only metric 10 is omitted; explicit nondefault metrics remain after the existing range validation. Removing explicit nondefaults from the desired file permits FRR reload to restore defaults. The updated full golden, explicit level-2 VRF expectation and metric10/metric20 regression preserve this distinction. The live FRR10.7.1 evidence shows the canonical target has an empty diff; this approval does not assert identical defaults on arbitrary FRR versions.

The reload-error recovery in `renderer.go:243–259` requires that the prior file contain an IS-IS router and the desired file remove all IS-IS router blocks. A reload error alone cannot produce success: a second, separately executed `--test` against the complete desired FRR files must succeed with an empty normalized diff. Remaining circuit lines, changes in other managed sections, tool errors and canceled contexts continue into the existing snapshot restore/reload path. The change does not remove the ordinary post-reload convergence check or its rollback. The established hostname normalization exception is unchanged; “full convergence” here means the existing normalized managed FRR configuration, not unmanaged runtime properties.

`TestISISRemovalReloadErrorRequiresObservedConvergence` exercises both outcomes: a rejected redundant circuit-removal command with no residual diff succeeds after one reload; a residual circuit fails with `ErrDaemon`, restores the original file and executes rollback reload. The test is not merely an assertion of the new branch. It checks retained/restored file contents and reload counts.

Inspected existing validation evidence rather than rerunning it:

- [Focused renderer tests](F-isis-rip-host-2026-10-03-evidence/unit.txt): IS-IS package PASS0.075s; FRR renderer PASS (cached).
- [Live IS-IS acceptance](F-isis-rip-host-2026-10-03-evidence/live-isis.txt): PASS58.70s, real rendering/Apply/DryRun/removal, disposable VPP stopped.
- [Strict production-agent topology](F-isis-rip-host-2026-10-03-evidence/topology.txt): PASS154.20s, real IS-IS removal through the agent and subsequent unchanged Apply, RIP withdrawal/relearning/restart/rollback. Shared VPP NRestarts remained 0; the private VPP stopped. Existing slot-directory artifacts shown by the harness are not claimed removed by this review.

Approval excludes enabling shared OSI punt, shared appliance deployment, unsupported routing features and a new whole-repository test run. The narrow renderer correction is consistent with the recorded FRR version and preserves failure/rollback behavior outside observed-converged IS-IS removal.
