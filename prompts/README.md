# VRX agent prompts

**Start here if you are the human:** `cd /root/ngfw && cat prompts/00-CONTEXT.md prompts/MANAGER-PROMPT.md | claude` — the manager agent runs everything else (see `docs/13-handoff-fa.md`).

Every task prompt is **self-contained** once you prepend `00-CONTEXT.md`. Run each in
its own git worktree with Claude Code from the repo root:

```bash
git worktree add ../vrx-P05 -b feat/P05-agent-core
cd ../vrx-P05
cat ../vrx/prompts/00-CONTEXT.md ../vrx/prompts/P05-agent-core.md | claude
```

## Order and parallelism

| Wave | Run in parallel | Gate before next wave |
|---|---|---|
| W1 (day 1) | P01, P04, P02 ×3, P03 ×2, P09 | `tools/lab up single` green, empty CI green |
| W2 (day 2-3) | P02, P03, P09 | **Human approves contracts; they are frozen** |
| W3 (day 3-5) | P05, P06, P07 | each layer's own tests green against the contracts |
| W4 (day 6-7) | P08 | ping through VPP, live counters in UI, commit/rollback on MTU, kill -9 vpp → reconcile |
| W5+ | `FEATURE-TEMPLATE.md` instances (F-*), P10–P14 | see docs/11-compressed-plan-fa.md (21-day plan, VM lab, FAST MODE) |

After every PR: `REVIEW-PROMPT.md` dispatches the reviewer panel and the testers, in fresh agents. Every evening: `INTEGRATOR-PROMPT.md`.

## Team roster (D-156)

| role | count (max at once) | prompt file | reports to | writes feature code? |
|---|---|---|---|---|
| Lead manager | 1 | [`MANAGER-PROMPT.md`](MANAGER-PROMPT.md) | product owner | only to unblock |
| Arbiters (internal managers) A1 contracts/API/web · A2 data-plane/agent/daemons · A3 process/CI/packaging/scheduling | 3 | [`ARBITER-PROMPT.md`](ARBITER-PROMPT.md) | lead manager | never |
| Developers (workers) | ≤ 30, one slot each (1–11, 14–32) | task prompt + [`WORKER-OPS.md`](WORKER-OPS.md) | lead manager | yes, own branch only |
| Review dispatcher | 1 per branch | [`REVIEW-PROMPT.md`](REVIEW-PROMPT.md) | lead manager | never |
| Reviewers R1 correctness & tests · R2 security · R3 contracts/API · R4 data-plane/shared host · R5 performance · R6 UX/web/i18n · R7 docs & evidence · R8 operability/packaging | 8 aspects, spawned per branch (R1, R2, R7 always) | [`reviewers/`](reviewers/) | lead manager; disputes → arbiter | never |
| Testers T1 unit/contract/CI · T2 API e2e (PostgreSQL/Valkey) · T3 data-plane/topology/traffic · T4 web e2e/screenshots/regression | 4 | [`TESTER-PROMPT.md`](TESTER-PROMPT.md) | lead manager; disputes → arbiter | never (tests only with `add-tests`) |
| Integrator | 1 (daily) | [`INTEGRATOR-PROMPT.md`](INTEGRATOR-PROMPT.md) | lead manager | never |

Flow: developer done → review panel + testers in parallel → combined verdict (any BLOCK or FAIL blocks) → merge by the lead
manager. Any dispute → arbiter → lead manager → product owner (PENDING, `docs/decisions/decision-policy.md`). Rulings:
[`docs/decisions/ARBITRATION-LOG.md`](../docs/decisions/ARBITRATION-LOG.md). Persian summary: [`docs/14-team-roster-fa.md`](../docs/14-team-roster-fa.md).

## Filled examples

`features/F-vlan-qinq.md` and `features/F-nat44-ed.md` show a completed template — note how
much of each is *fence* (out of scope, verify-in-binapi, paste-the-evidence).

## Writing a new feature prompt

Copy `FEATURE-TEMPLATE.md`, fill every `<...>`, and — most importantly — fill the
**Out of scope** section. Agents over-build; the fence is the most valuable paragraph.
