# F-mpls-ldp-host independent R7 review — 2026-10-04

Reviewed final local source 339f400c against local main. Review covered requested
docs/evidence/scope aspect only; no product files edited. Read AGENTS/shared
review rules and applicable reviewer prompt.

## Findings

BLOCKER — docs/status/tasks/F-mpls-ldp-host.md:8 and WIP evidence: final report summarizes PASS without pasted command output and does not record current aggregate quick outcome. WIP timings are prose, not pasted output; R7 requires real command/output evidence. Fix final report with actual final scoped commands/output and honest full gate/environment status. MAJOR — docs/user/routing/mpls-ldp.md:39: existing user guidance omits new EOS-only and explicit-null restrictions, 256-route cap/hold-down behavior and still says sync arrives with host build. Update supported product scope. EOS-only IPv4 limitation and lab NOT RUN are otherwise explicitly recorded in implementation/renderer docs; no real forwarding proof claimed.

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

Final R7 verdict: **APPROVE**.
