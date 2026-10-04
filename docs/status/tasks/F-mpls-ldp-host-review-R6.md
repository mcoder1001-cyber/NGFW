# F-mpls-ldp-host independent R6 review — 2026-10-04

Reviewed final local source 339f400c against local main. Review covered requested
docs/evidence/scope aspect only; no product files edited. Read AGENTS/shared
review rules and applicable reviewer prompt.

## Findings

MAJOR — docs/user/routing/mpls-ldp.md:39-56: user guide does not explain EOS-only IPv4 switching and explicit-null/non-EOS rejection, cap256 and overflow/hold-down; it still describes sync as future host-build delivery. CLI example uses logical VPP interface TenGigabitEthernet0 rather than mapped host FRR interface. Users can apply a misleading daemon configuration or assume unsupported stacks are handled. Fix guide and mapped-Linux CLI example. No UI code changed; screenshots and RTL not applicable. Read-only table-zero topology command correctly requires both shared lab/globals locks and does not authorize mutation.

## Review evidence

Read task final/WIP reports, scope envelope and relevant renderer/user/topology
docs, and compared described limits with committed source.
`git diff --stat main...HEAD` confirms no web/UI changes in these deliveries.
Pasted developer test evidence was inspected; this reviewer did not rerun those
tests, full quick CI or live lab. No new test-success claim is made.
Remote publication equivalence must be checked separately by manager.

Initial verdict: **BLOCK**.

## Verify round

Reviewed exact local source head `cb2560ed`; no product edits by reviewer.

Prior findings resolved. The user guide now states EOS IPv4 only, non-EOS/explicit-null unsupported, 256-route cap, failed/oversized-read 60-second hold-down, deferred lab acceptance, and mapped Linux FRR interface `host-wan0` distinct from NGFW logical name. Final report includes real scoped race/vet/source-check command outputs after installed-count correction. It distinguishes unavailable live acceptance and baseline environment failure from a branch full-quick pass. Reviewer independently matched quoted chown EINVAL and Unix-socket EPERM lines to the manager baseline 07-turbo.log. Installed-count source correction is within state RPC accuracy scope; guide reports current ownership-filtered Retrieve and unavailable failures accurately. No unproven aggregate CI or live acceptance claim remains.

`git diff --check main...HEAD` ran in this task worktree: no output, exit 0.
No product tests rerun in this docs/evidence verify round.
Remote published tree equivalence remains manager responsibility.

Final R6 verdict: **APPROVE**.
