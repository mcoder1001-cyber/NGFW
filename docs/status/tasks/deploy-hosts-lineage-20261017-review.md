# Combined review: deploy-hosts-lineage-20261017 (PR #240)

Reviewed tree: `1649dd100` (branch `fix/deploy-hosts-lineage-routes-20261017`, base `99a774734`).
Panel per `prompts/REVIEW-PROMPT.md` for a `deploy/**` change: R1, R2, R7 (every branch) and R8
(deploy). Each reviewer ran in a fresh read-only agent; the developer saved the reports verbatim.

| reviewer | verdict | BLOCKER | MAJOR | MINOR / NIT | report |
|---|---|---|---|---|---|
| R1 correctness & tests | BLOCK | 0 | 3 | 5 / 2 | [review-R1](deploy-hosts-lineage-20261017-review-R1.md) |
| R2 security | BLOCK | 0 | 1 | 4 / 2 | [review-R2](deploy-hosts-lineage-20261017-review-R2.md) |
| R7 docs & evidence | APPROVE WITH CHANGES | 0 | 2 | 7 / 7 | [review-R7](deploy-hosts-lineage-20261017-review-R7.md) |
| R8 operability & packaging | APPROVE WITH CHANGES | 0 | 1 | 7 / 4 | [review-R8](deploy-hosts-lineage-20261017-review-R8.md) |

**Combined verdict on `1649dd100`: BLOCK** (R1 and R2 BLOCK). No tech-debt row was requested;
every MAJOR is fixed in this branch (checkpoint 2) and R1/R2 re-check only their MAJORs.

## Disposition of the MAJORs (checkpoint 2)

| finding | fix in checkpoint 2 |
|---|---|
| R1-1 / R2-2: lineage verdict not tied to the version replaced under the lock | the preflight version is passed as a validated 5th remote argument; under the lock every package must still carry it ("installed version changed since preflight"); the remote receives the override literal `1` only when that host's lineage needed it; fixture `race` |
| R1-2 / R2-1 / R8-1: `--allow-divergent` also accepted a same-history rollback and applied to every host | `check_lineage` refuses when the new source is an ancestor of the installed one, with or without the flag; the flag is accepted with exactly one host; fixtures `rollback` and the two-host rejection |
| R1-3: `build-info.json` provenance path untested | the stale-branch and ancestor fixtures now carry `build-info.json`; the 40-character source commit reaches `git rev-parse` |
| R7-1: amendment outside the decision-log convention | logged as D-249 ("Amends D-248") in `LOG.md` with options, why, reversal cost and task; DEC-248 section retitled |
| R7-2 / R8-1: squash-merge consequence and review procedure undocumented | `docs/install/deploy-hosts.md` and DEC-248 describe the D-112 consequence, the three refusal cases, the printed review commands and `git fetch origin pull/<N>/head`; the refusal message prints merge-base, per-side commit counts and the commands |

## MINOR / NIT items also fixed in checkpoint 2

R2-3/R8-3 fetch once per run with `GIT_TERMINAL_PROMPT=0`, `--no-tags`, `--no-write-fetch-head`,
a 120 s limit and the last error line logged · R2-4 abbreviation prefix check in `resolve_commit`
· R2-5 hostile preflight fixtures · R2-6 `python3 -I` · R2-7 128-character cap · R1-4 device- and
metric-only route fixtures · R1-6/R7-5/R8-5 10 s window, API counter fixture, before/after counters
in the message, docs qualified · R1-7 unparseable `~dev` versions fail closed · R1-8/R2-other/R8-7
route type, `src`, `weight`, `via inet6` and grouped multipath next hops (swap fixture) · R1-10 test
name and docstring · R7-3/R7-13 flag scope and unresolvable new source documented · R7-4 DEC
sentence qualified · R7-6 fetch wording · R7-8 envelope SHA pinned · R7-9/R8-9/R1-other empty
staging directory removed on refusal and named if removal fails · R7-10/11/12 wording · R7-14
`override.txt` in the recovery directory · R8-2 work-tree and shallow checks with their own
messages, `-p` requirement documented · R8-4 `service-status.txt` on failure, counters read before
`is-active` · R8-6 route after-state saved and both forms printed, `ip` failure fails under errexit
· R8-8 mixed versions printed with a recovery line · R8-10 cross-reference comment and a fixture
pinning `prepare.sh`'s version format · R8-11 option messages.

## Not done in this branch

- R1-5 (M7): the remote `$VERSION =~ development` condition is defence in depth and is now
  unreachable through the script's own flow (the remote receives `1` only after the local check
  accepted a development-to-development lineage); it has no dedicated fixture.
- R7-7 WIP sections: updated in checkpoint 2; hosted CI result recorded when available.
- R7-16 `plan/tasks.yaml` row and R7-1's decision number: manager to confirm D-249.
- R8-5 agent-readiness anchor (`/run/ngfw/agent.sock`): not used; the socket path is
  overridable by `agent.env` and may persist across restarts (`RuntimeDirectoryPreserve`).
- R7-other / R8-12: hosted packaging job duration to be confirmed from the PR's CI run.
- The originating review's 2-الف.4 minor items remain out of scope (see the WIP file).

Verify round: R1 and R2 re-check their MAJORs on checkpoint 2; R7 and R8 confirm their MAJORs.
