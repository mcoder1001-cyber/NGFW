# P11-host independent R7 review

Reviewed source `f59c3ea7` on 2026-10-05. Independent reviewer owns only this report; no product or host edits.

## Findings

MINOR — wip still ends at a running earlier packet rerun and canonical opener says full quick pending; prepend current frozen-source evidence and manager quick result when integrated. Canonical later section already records successful final campaign, so no absent packet acceptance is being claimed.

No BLOCKER or MAJOR findings in docs/evidence/scope.

## Evidence inspected

Read-only commands actually run included `cat` of the task canonical/wip/status files and install documentation, `git -C /root/ngfw-wt/ready-p11-host-20261005 diff --name-only origin/main...f59c3ea7`, and the relevant independent test/review reports. This is evidence inspection; the reviewer does not claim to have independently executed the full test gate.

Canonical/wip reports, committed native-production-summary.json, topology README and touched-path list inspected. Actual JSON read: source7efd712f, passed:true, peer_loss_requested:true; responder/initiator/shared-VPP-unchanged-and-slot-clean all exit0; MainPID1014 and NRestarts0 before/after; two capture SHA256s. Canonical report identifies assertion26eb/report7efd separately from unchanged production implementation875, describes explicit authoritative rollback domains and honest earlier failed omitted-domain run. Privacy-preserving summary exposes no SA/secrets. Certificate peer and installed-appliance/browser UX remain separate acceptance.

DEC-ipsec-route-based supersedes retired kernel-vpp/policy requirements; scope is existing native production agent plus isolated acceptance wrapper/assertions, not a new tunnel engine. Shared-host ownership rules and fixedslot8 locks/cleanup are explicit. No restart of shared VPP or new security-boundary decision is claimed.

The manager owns board updates and the shared deferred-acceptance ledger. This branch does not change the board. Current-main exact integration quick, hosted quick and remaining mandatory reviewers remain merge requirements; this report does not claim those complete.

Verdict: APPROVE.
