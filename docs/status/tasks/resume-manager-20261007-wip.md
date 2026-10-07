# Remaining branch queue, 2026-10-07

Base main: `0ec397e327123cadfd5d278a9a1cda37532fdc2c`; actual hosted complete quick run `37583587580` SUCCESS. Manager branch `codex/resume-manager-20261007`; last local/remote published checkpoint `c0922dcd9da509c7ca5b14f9121fb0ed9b6d75aa` (CLI push verified). Owned files: envelope, this WIP, individual board/status/acceptance changes after independent review.

Verified live workers spawned this run:

- PPPoE developer Beauvoir `01a11538-3cc4-7f92-91fa-6f5b1ac54948`, branch `codex/resume-pppoe-20261007`, worktree `/root/ngfw-wt/resume-pppoe-20261007`, initial `992b264b2084a8adfcee755d2b5650b771a6a8f1` (PR196).
- P12 developer Mill `01a11538-3ffa-7be1-b158-22943ad6eacd`, branch `codex/resume-p12-20261007`, worktree `/root/ngfw-wt/resume-p12-20261007`, initial `661cbc21bf4bead10abb93333f493406f4142db1` (PR180).
- RA developer Kepler `01a11538-7beb-7db0-956a-576f15d96886`, branch `codex/resume-ra-20261007`, worktree `/root/ngfw-wt/resume-ra-20261007`, initial reviewed union `711e18e12ac2776ad4fc15066f811b60599e63f2` (PR193 head older `b0367efb48339808359c4c82e2675060d8c1ddd4`).

Task state is separate from observed worker activity. No old worker is claimed alive. Published main board: 202 merged, 1 review, 1 running, 8 parked; 212 unique tasks. Proposed manager board: 205 merged, 1 running, 6 parked, conditional on checked merge. PR179 proposes bounded MPLS/SRv6 historical acceptance closeout. AutoBlock production/test files from PR181 are byte-identical to current main, but PR181 also carries unmerged P12 evidence; do not merge duplicate whole branch. Recovery branch `codex/recover-eight-review-20261004` predates current main: inspect ancestry and original delta before adopting any changes.

Tests actually observed: current main hosted quick SUCCESS; PR196 hosted quick `37583104528` SUCCESS on original head. These do not validate changed integration candidates. Current disk 27 GiB free; focused Go tests bounded while cache rebuilds; complete mandatory hosted quick remains unchanged. Shared VPP handover pending.

Independent R2 reviewer Locke approved exact candidate c0922dcd9 and AutoBlock equivalence, report published `461c3f1911f94cc12e5acf76c6a29d6a6d5ac716`. R7 reviewer Lagrange is checking original raw receipts and recovery ancestry. Reviewers own isolated branches; their completion is not a persistent worker claim. Manager exact current candidate diff-check and redacted gitleaks (2 commits) PASS; board check valid212. Full candidate hosted quick remains outstanding.

Remaining: R7 review, recover remaining code, final squashed current-main integration candidates, exact hosted quick, sequential merges and post-merge main CI; record laboratory criteria honestly. Next command: integrate approved reviewer reports, preserve pre-squash archive, create single-commit docs integration on actual main, update PR179 with lease and run complete unchanged hosted quick.
