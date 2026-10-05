# F-hardening-lite independent R7 review

Reviewed source `726d92141402459505ee27fdc6fafc7aeef94295` on 2026-10-05. Independent reviewer owns only this report; no product or host edits.

## Findings

MINOR — canonical source report correctly labels its 17m58s gate as older source73b6c32f and says final T1 is required; now integrate the independent exact726 T1 PASS36m12s evidence and refresh the wip current-state opener. No false final-source pass is asserted.

No BLOCKER or MAJOR findings in docs/evidence/scope.

## Evidence inspected

Read-only commands actually run included `cat` of the task canonical/wip/status files and install documentation, `git -C /root/ngfw-wt/ready-hardening-20261005 diff --name-only origin/main...726d92141402459505ee27fdc6fafc7aeef94295`, and the relevant independent test/review reports. This is evidence inspection; the reviewer does not claim to have independently executed the full test gate.

Canonical status, install guide, touched-path list and independent T1 report inspected. Actual read output: independent exact-source quick PASS36m12s; topology wrapper PASS1.386s, all ten Python cases; agent/CLI and all deploy checks PASS149/0. The guide describes offline systemd259 scores API3.0→1.7 and agent5.0→3.8 as fixture measurements and explicitly defers appliance daemon compatibility/signed install. Optional SSH/rp_filter controls, inactive packaged profiles and signed-byte/key-rotation boundaries agree with source and tests. Source-complete is distinguished from lab acceptance and certification.

No silent security-boundary change: task-requested opt-in offline profile staging neither activates shared-host restrictions nor changes agent sandbox privileges. Decisions are conservative implementation of the task baseline; excluded VPP/retired strongSwan and existing host nftables ownership are explicit.

The manager owns board updates and the shared deferred-acceptance ledger. This branch does not change the board. Current-main exact integration quick, hosted quick and remaining mandatory reviewers remain merge requirements; this report does not claim those complete.

Verdict: APPROVE.
