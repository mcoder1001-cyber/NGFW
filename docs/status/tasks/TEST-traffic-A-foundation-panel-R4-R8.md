# TEST-traffic-A final foundation panel — R4 / R8

2026-10-02. Independent reviewer; no product authorship or arbitration role.
Reviewed exact integration `ca486c2f9197483efaa7683384625e85e973c510`
in isolated branch `task/traffic-foundation-panel-review` and worktree
`NGFW-traffic-foundation-panel-review`. Owned output: this report only.

## Findings and verdict

**APPROVE R4 / R8 for the inactive source-foundation integration only.**
No new BLOCKER, MAJOR or MINOR in these panel scopes. Historical descendant
BLOCK, its independent closure and A3 finish/split ruling remain preserved.
This approval neither grades the separate correlator continuation nor completes
whole TEST-traffic-A, and does not replace exact-head hosted CI.

`git diff fda0ddc7728b1928f78f331a379ac4ec5b45dde6 HEAD -- test/topology/traffic-a`
was empty: approved frozen source/README is unchanged. Comparison to integration
base db15515d shows only traffic support, its scoped CI runner/workflow and
review/status/arbitration documents; no production Go, YANG, units, privileges
or live host command activation is introduced.

R4: README and plan distinguish separate support tests from composed forwarding.
All seven stages have executor=None. The run CLI checks opt-ins/slot constraints
then refuses missing implementation before lease, boot-id reads or commands.
Documentation makes shared locks, lease renewal, protected run directories and
capture lifecycle future requirements. It does not claim the current lease
reader alone establishes live authority. Global VPP ownership, all-ID range,
reserved slots and retained global integration flags remain refused. Root0600
lease and bounded capture/log contracts are stated with their actual limits;
stdout byte cap remains explicitly split and required before live activation.
The source-only Python scope creates no new agent/API runtime contract.

R8: workflow uses pinned checkout, contents:read, persist-credentials:false,
Ubuntu 24.04 and five-minute job timeout. It invokes only the repository Python
source-fixture runner, with no package installation or rig credentials/commands.
Explicit two-module discovery requires sixteen cases; accepted() rejects zero,
failed/errored/skipped/expected-failure/unexpected-success outcomes. The job and
runner describe inactive source fixtures, not laboratory acceptance.
Central DEFERRED-ACCEPTANCE records real lab NOT RUN separately from seven
NOTIMPLEMENTED executors and other genuine source gaps; whole-chain and packet
outcome proof remain false. Envelope explicitly requires unchanged full hosted
quick plus the dedicated strict source gate on the exact integration head and
post-merge main verification.

## Actual independent checks

No redundant sixteen-case rerun was performed; prior independent source result
and manager-reported new wrapper/control results are attributed, not re-labelled
as this review execution. Personally executed plan/refusal assertions:

- `python3 test/topology/traffic-a/run.py plan --slot 14`: parsed JSON has exactly
  seven NOTIMPLEMENTED stages and whole_chain_proven=false — PASS.
- Invoked run mode with all valid slot-14 environment values and both live opt-ins:
  exit 1, NOTIMPLEMENTED lists all seven stages — PASS. Source ordering confirms
  refusal precedes host reads and command execution.
- Frozen source identity diff — empty. Scoped workflow, strict runner, README,
  CLI/environment validation, central campaign, integration envelope/WIP and
  preserved arbitration/source reports inspected.

No SSH, VPP, nft, capture, host installation, live lease or production mutation
was executed. Hosted quick and strict gate on this exact integration head were
not queried or run by this reviewer: manager must attach their actual results
before merge. Actual laboratory acceptance remains NOT RUN; absent executors,
correlation and lifecycle remain source work, not owner-waived lab-only work.
