# Remaining branch queue, 2026-10-07

Base main: `0ec397e327123cadfd5d278a9a1cda37532fdc2c`; actual hosted complete quick run `37583587580` SUCCESS. Manager branch `codex/resume-manager-20261007`; initial local/remote checkpoint pending first publication. Owned files: envelope, this WIP, individual board/status/acceptance changes after independent review.

Verified live workers spawned this run:

- PPPoE developer Beauvoir `01a11538-3cc4-7f92-91fa-6f5b1ac54948`, branch `codex/resume-pppoe-20261007`, worktree `/root/ngfw-wt/resume-pppoe-20261007`, initial `992b264b2084a8adfcee755d2b5650b771a6a8f1` (PR196).
- P12 developer Mill `01a11538-3ffa-7be1-b158-22943ad6eacd`, branch `codex/resume-p12-20261007`, worktree `/root/ngfw-wt/resume-p12-20261007`, initial `661cbc21bf4bead10abb93333f493406f4142db1` (PR180).
- RA developer Kepler `01a11538-7beb-7db0-956a-576f15d96886`, branch `codex/resume-ra-20261007`, worktree `/root/ngfw-wt/resume-ra-20261007`, initial reviewed union `711e18e12ac2776ad4fc15066f811b60599e63f2` (PR193 head older `b0367efb48339808359c4c82e2675060d8c1ddd4`).

Task state is separate from observed worker activity. No old worker is claimed alive. Current board: 202 merged, 1 review, 1 running, 8 parked; 212 unique tasks. PR179 proposes bounded MPLS/SRv6 historical acceptance closeout. AutoBlock production/test files from PR181 are byte-identical to current main, but PR181 also carries unmerged P12 evidence; do not merge duplicate whole branch. Recovery branch `codex/recover-eight-review-20261004` predates current main: inspect ancestry and original delta before adopting any changes.

Tests actually observed: current main hosted quick SUCCESS; PR196 hosted quick `37583104528` SUCCESS on original head. These do not validate changed integration candidates. Current disk 27 GiB free; focused Go tests bounded while cache rebuilds; complete mandatory hosted quick remains unchanged. Shared VPP handover pending.

Remaining: independent evidence/security review of PR179 and redundant PR181, recover remaining code, final squashed current-main integration candidates, exact hosted quick, sequential merges and post-merge main CI; record laboratory criteria honestly. Next command: independent reviewers inspect PR179 evidence and PR181 equivalence, then build current-main docs candidate.
