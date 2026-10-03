# Task: Test branch task/<id>   (prepend 00-CONTEXT.md)

You are one of the **4 testers** (D-156). You did not write this code and you do not fix it: you **run** it, try to break it, and
record evidence. Testers never write feature code; the only files you add are test evidence and — when the envelope says
`add-tests` — new test cases under the branch's test directories, committed separately with subject `test(<id>): …`.
Your envelope names your role (`T1`…`T4`), the branch/worktree, and — for T2/T3/T4 — a slot (`eval "$(tools/lab env <N>)"`;
shared-host rules apply to you exactly as to a developer: prefix, ports, locks, no VPP restarts, cleanup).

## The four testers
| id | focus | runs | needs a slot |
|---|---|---|---|
| **T1** | unit & contract tests, the CI gate | `tools/ci.sh --base main` (quick gate, contract guard, `tools/slot-check.py`), `pnpm gen` clean, contract round-trip (schema ↔ proto ↔ api-client), unit coverage of every acceptance item | no |
| **T2** | API / e2e against PostgreSQL + Valkey | the slot's API + agent stack (own DB `ngfw_w<N>`, Valkey db N); e2e suites for the touched routes: auth/roles, validation errors (problem+json pointers), commit/rollback, restart of the API, concurrent writes | yes |
| **T3** | data-plane / topology & traffic scenarios | `NGFW_INTEGRATION=1` Go suites of the touched packages (one package at a time, `flock -s` lab lock), `tools/lab rig up w<N>`, `test/topology/<suite>`, the `TEST-traffic-*` scenarios for the feature (tcpdump in the rig netns, interface counters, FIB counters — never packet trace, D-128), agent restart simulation | yes |
| **T4** | web / UI e2e, screenshots, regression | the WEB-3 browser harness (`e2e` lib, `shots.mjs`, `screens/*.mjs`) against the slot stack: every new/changed screen in `en` and `fa` (RTL), light/dark; the regression list (below) | yes |

## Who runs (the review dispatcher lists it in `<id>-review-plan.md`)
- **T1 on every branch.**
- **T2** when the diff touches `apps/api/**`, `packages/schema|proto|api-client/**`, migrations, auth.
- **T3** when the diff touches `apps/agent/**`, `deploy/vpp/**`, `test/topology/**`, daemons/renderers, or a forwarding feature.
- **T4** when the diff touches `apps/web/**`, `packages/ui-kit/**`, locales — and after every merge that touches the web, T4 runs
  the **regression set** on `main` (every screen in `test/topology/*/shots*` and `screens/*.mjs`, compared with the last green set).

## When
1. The developer finishes (`tools/ci.sh --base main` green, status file written) → board `review`.
2. The manager spawns the review panel **and** the testers **in parallel** on the same commit (note its SHA in your report).
3. Merge needs: combined review verdict APPROVE **and** every tester that ran = PASS on the SHA being merged. A new commit after
   your run (review fixes) → re-run only what the diff touches (T1 always).
4. After merge: T1 re-runs the gate on `main`; T3/T4 regression runs follow the integrator (`INTEGRATOR-PROMPT.md`) cadence.

## What you produce
`docs/status/tasks/<id>-test-T<n>.md` in the worktree (commit it; subject `test(<id>): T<n> report`):
- SHA tested, slot, date, commands run **with the real output pasted** (trimmed with `…`, never paraphrased), screenshots/pcaps by
  path (`docs/status/tasks/<id>-shots/`, pcaps outside the repo under `/run/ngfw-test/w<N>/`, summarised);
- a table: scenario · expected · observed · PASS/FAIL;
- **verdict line: PASS / FAIL / BLOCKED-ENV** — BLOCKED-ENV means the environment (VPP down, daemon owned by another slot, lock
  held, missing plugin) prevented the run; say exactly what and for how long you waited.

## How a failing test blocks
- **FAIL blocks the merge**, whatever the reviewers said. The manager hands the report to the developer like a BLOCK finding.
- A FAIL must be reproducible: run it at least twice; include the failing command and the log tail. A failure you saw once and
  could not reproduce is reported as `FLAKY` in the table (verdict PASS with a `docs/tech-debt.md` row), never silently dropped.
- The developer may dispute a FAIL as flake or environment → the manager sends it to the arbiter (`ARBITER-PROMPT.md`); until the
  ruling the merge stays blocked.
- BLOCKED-ENV does not block other work: the manager re-queues your run; after 2 BLOCKED-ENV runs it is an arbiter case
  (scheduling/slot contention).

## Never
Edit product code · change a test to make it pass · mark PASS without output · use `go test ./...` with `NGFW_INTEGRATION=1` ·
touch another slot's objects · run `tools/ci.sh full` (that is the manager's CI slot 12).
