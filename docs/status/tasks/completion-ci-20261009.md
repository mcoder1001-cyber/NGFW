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

## Corrected source prepared for the next exact-head gate

Product `1d7f3d96` (author receipt `c41bc225`) corrects all established source
findings. Pinned golangci-lint2.13.2 reports **0 issues**, and complete `go vet
./...` succeeds. Source generation completed13/13. Independent R4 receipts
approve the bounded lint/path corrections, constructor-only inventory fixture
injection and explicit SNMP transaction-owner binding. No workflow, linter setting,
production privilege or safety guard was weakened.

The independent complete local agent run on the first candidate failed:
128 packages passed,15 failed,166 had no tests. Concrete source/fixture failures
were corrected: explicit proto scalar presence, fake wiring inventory, SNMP owner
selection, exact syslog refusal pointer, valid PD lease fixture, and a BFD all-range
nil dereference found by the corpus. Socket, UID and process namespace failures
remain actual local failures and must pass unchanged on the hosted runner.
The original inventory is retained in recovery-20261009-review-R2.md; final delta
review and the next hosted outcome must be read with it rather than overwriting it.

## Final integrated source receipt — 2026-10-09

Source candidate `30de26ee6a5697a3713fe4375399b86c5588a472` passed the unchanged complete hosted quick gate
[37903333143](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333143) and all applicable fixture workflows.
[PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) merged as `d58db1a673d716ebcf595a9e49847a54991c58da`; merge tree equals tested tree
`d1de8b7c01220ce03b311d0a7dcafd5fe12ab1ad`. Reviewed history is preserved at
`codex/archive-completion-corrected-20261009` (`3f98838119ed8d67aab9c226308829ac49af9def`).
Final R2/R4 correction receipts approve the scoped source; initial failed CI and
local environment failures remain historical evidence, not retroactive PASS.

| Final check | Result |
|---|---|
| Mandatory quick | PASS, complete unchanged hosted gate |
| [Packaging37903333245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333245) | 81 fixtures, zero failures/errors/skips; seven gate-policy controls PASS |
| [Provisioning37903333184](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333184) | 46 strict +23 offline Debian +11 trusted installer +18 portable export PASS |
| [Python37903333309](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333309) | 14 synthetic-wheel +5 release-contract tests PASS |

Board: **205 merged,7 parked,0 review,0 running**, total212. The eight reviewed
source rows are merged. PPP-host and MultiWAN-host source is integrated; their
native acceptance joins the five already parked rows. No live source worker is
claimed. All older pending-source/CI statements in this document are historical
and superseded by this receipt.

Native acceptance of this cumulative source remains **NOT RUN**. Known prior RA
supplier/post-ACK identity failures and P12 mgmtd startup failure at the unchanged
30-second deadline before the200-route proof still require diagnosis, any necessary
fix and rerun on the real target. No unit/fixture result closes these cases.
Product license text/name/copyright authority remains a separate release input
under `docs/decisions/PENDING-P10-product-license.md`; no license is invented.
Plan exclusions remain unchanged. This is source completion, not release certification.

Observed hosted quick summary (job113730761797, 2026-10-09T08:25:13Z):

```text
Tasks: 35 successful, 35 total
apps/agent: make lint test build 5m55s
apps/cli: make lint test build 0m10s
test/ Go modules, unit mode 0m22s
deploy/vpp: shellcheck + apply-startup fake-host harness 1m35s
mode quick · wall time 14m07s
CI GATE PASSED
```

Integration modules ran in unit mode: compile/vet and applicable unit tests are
proven, not native acceptance. Existing test-only OpenSSL `ALLOW:` notices remain
visible in the gate log; no zero-warning or native-no-skips assertion is made.
