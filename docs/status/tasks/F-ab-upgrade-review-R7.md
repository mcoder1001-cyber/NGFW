# F-ab-upgrade independent R7 review

Reviewed source `be1448118afd246f39b0d6a7db01d2f986448482` on 2026-10-05. Independent reviewer owns only this report; no product or host edits.

## Findings

MINOR — refresh canonical/current recovery status with final 17-test signing-key/health corrections and observed prior quick PASSac421 rather than its superseded running status; keep exactbe144 gate pending until evidence arrives. MINOR — docs/09-os-packages.md omits new direct runtime tools (R8 finding). MINOR — unrelated historical hardening/P11 review reports should be archived outside final A/B product scope.

No BLOCKER or MAJOR findings in docs/evidence/scope.

## Evidence inspected

Read-only commands actually run included `cat` of the task canonical/wip/status files and install documentation, `git -C /root/ngfw-wt/ready-ab-20261005 diff --name-only origin/main...be1448118afd246f39b0d6a7db01d2f986448482`, and the relevant independent test/review reports. This is evidence inspection; the reviewer does not claim to have independently executed the full test gate.

Canonical/wip/question reports, install guide, touched-path list, committed unit/storage/loop evidence and independent R8 report inspected. Actual read output: R8 deploy/upgrade/tests/run.sh Ran17 tests3.293s OK; packaging Ran35 tests38.347s OK. Source canonical older test counts are identified chronologically by wip additions, not a new test pass. Sparse96GiB loop evidence proves signed/tampered refusal, one-shot/confirm/failure env-file lifecycle and unchanged host; explicitly ext4 EFI/tiny handmade root, not vfat firmware boot. Real populated DB migration/crypt unlock/watchdog remain appliance acceptance. Install guide includes signing-key-in-root/hardlink refusal and fixed180s health rollback behavior.

Task defaults authorize writable ext4 and dedicated Ed25519; questions record alternatives/default rationale. Offline EFI trampoline and shared /data/ngfw bind fit existing layout and preserve state, with no automatic host provisioning. Root-parent storage correction is owner-authorized existing privilege boundary enforcement, not a new root API. Source status explicitly prohibits generic privileged executor and automatic DB overwrite.

The manager owns board updates and the shared deferred-acceptance ledger. This branch does not change the board. Current-main exact integration quick, hosted quick and remaining mandatory reviewers remain merge requirements; this report does not claim those complete.

Verdict: APPROVE.

## Exact-source test evidence update

Inspected independent T1 exactbe1448118afd246f39b0d6a7db01d2f986448482 unchanged full quick PASS9m49s, upgrade wrapper20.323s invokes17 regressions/Go probe race. Normal generation/agent/CLI complete; standard green fake-host cache reuse is identified honestly. No firmware/DB acceptance inherited. Runtime OS package reference remains optional minor until manager integration adds the declared dependency list; actual Debian dependencies exist. Verdict remains APPROVE; final latest-main integration and hosted gate pending.
