# Final combined validation campaign — 2026-10-09

Owner requested no intermediate CI and one final campaign after source work.
The first complete candidate is `69e288596de0167c9921e4808530a99bc4c88302`, tree
`359021b3db157e8bdb887ce350c0cd92095dab3c`, one commit on main `417e8fcd`.
Independent source reviews are committed. The reviewed history is preserved at
`codex/archive-completion-reviewed-20261009` (`bbf0d530`).

## First exact-head results

| Check | Result | Run |
|---|---|---|
| Mandatory quick | FAIL at agent lint: 62 findings; no successful full Go gate claimed | [37895358809](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37895358809) |
| Packaging | PASS: 81 fixtures, zero errors/failures/skips; seven gate-policy controls also pass | [37895358803](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37895358803) |
| Provisioning | PASS: 46 strict fixtures, 23 offline Debian, 11 trusted installer, 18 portable export | [37895358856](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37895358856) |
| Python release lock | PASS: 14 synthetic-wheel controls and five committed-release contracts | [37895358863](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37895358863) |

The quick gate completed generated-output validation, forbidden-pattern/secret
checks and all 35 TypeScript lint/typecheck/test/build tasks, then `go vet` before
lint failed. Agent findings: errcheck1, gosec13, revive47, unused1. The full agent
log is retained in [completion-ci-20261009-agent-lint.txt](completion-ci-20261009-agent-lint.txt).
No linter setting or test is disabled to resolve this. Actual source fixes and
applicable independent review must precede the next candidate in this campaign.

Real foreign-owned private-file capability and migration fixtures passed in the
hosted packaging job; their earlier local environment failure is not relabelled
as a local PASS. Native appliance, kernel PPP/VPP packets, installed service and
browser acceptance remain NOT RUN on this candidate. Product license remains a
separate release input. No merge has occurred at this checkpoint.
