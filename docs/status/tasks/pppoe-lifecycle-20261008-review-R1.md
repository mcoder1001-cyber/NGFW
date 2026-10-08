# PPPoE lifecycle R1 correctness and tests

Initial reviewed source: `955b6b8bdb8b9958c580db07276f246d87acd67f`. Final source recheck: `d08aae29b535671e37e5caeae6d1abc590d80101` with no uncommitted product-source changes.

## Findings

Resolved MAJOR — `apps/agent/internal/renderers/pppoe/supervisor.go:83-132`: a changed installed session is explicitly stopped and fenced before desired files are written. If subsequent daemon-reload fails, identical retry sees matching peer/unit/helper files, sets changed=false, and never retries reload, ResumeIPv6 or restart. The session remains stopped/fenced after retry returns success. Persist pending transitions through successful restart, or derive retry eligibility from an explicit fence/pending record, and add a recording-runner failure-then-identical-retry regression. Final repair writes persistent `ipv6-transitions/<host>` before stopping, treats that marker as changed regardless of disk equality, and clears it only after successful restart. Both reload failure and restart failure therefore retry; credential changes now rely on the same renderer path. `TestApplyIdenticalRetryCompletesFailedTransition` tests both failure points, successful identical retry, cleared markers and a subsequent no-op. Source inspection resolves this finding; test execution remains pending.

Resolved MAJOR — intermediate `supervisor.go:196`: recovery inventory lookup swallowed non-ENOENT errors after a removed unit had been deleted. Final inventory lookup returns errors for both transition and unit inventory failures, ignoring absent directories only. `TestApplyRefusesUnavailableTransitionInventory` uses a regular file as the inventory directory and asserts empty desired Apply fails before issuing commands.

Final removal recovery includes pending hosts after unit-file deletion, retries daemon-reload, then retires admission/fence/pending evidence only after completion. Removal retry and subsequent no-op are covered by `TestApplyRemovalRetryReloadsDeletedUnit`. Departed-parent up returns without overwriting replacement state; down verifies the parent against the current writer record. A dedicated replacement-preservation process regression covers both hooks.

## Validation

Independent `git diff --check`: PASS (exit 0, no output). Added process regressions exercise late up both before/after PID publication, rotating admission and inherited parent pidfd lifetime. Independent source/helper control: `python3 -I docs/status/tasks/pppoe-lifecycle-20261008-check.py` returned exit 0:

```text
rendered Python AST: PASS
retired and blocked admission: PASS
immutable parent pidfd despite numeric identity reuse: PASS
Full Go/race/golden/lifecycle execution: NOTRUN; hosted gate still required
```

This bounded control parses the rendered helper, checks retired/fenced admissions cannot reach writer operations, and verifies an original inherited pidfd observes parent death while a controlled numeric identity still appears live. It does not execute the full lifecycle or Go suite. Independent compilation, Go/race regressions, golden comparison and unchanged quick gate remain pending, with required publication awaiting owner resolution. No pass is claimed for those checks.

Source-only R1 verdict: APPROVE at `d08aae29b535671e37e5caeae6d1abc590d80101`; no remaining correctness finding identified in the scoped source repair. Merge eligibility remains BLOCKED pending required compilation, lifecycle/retry tests, golden comparison and unchanged complete quick gate. This source approval does not certify feature acceptance or replace execution evidence.
