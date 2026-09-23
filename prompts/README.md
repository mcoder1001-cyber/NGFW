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

After every PR: run `REVIEW-PROMPT.md` in a fresh agent. Every evening: `INTEGRATOR-PROMPT.md`.

## Filled examples

`features/F-vlan-qinq.md` and `features/F-nat44-ed.md` show a completed template — note how
much of each is *fence* (out of scope, verify-in-binapi, paste-the-evidence).

## Writing a new feature prompt

Copy `FEATURE-TEMPLATE.md`, fill every `<...>`, and — most importantly — fill the
**Out of scope** section. Agents over-build; the fence is the most valuable paragraph.
