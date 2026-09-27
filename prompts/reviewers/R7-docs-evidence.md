# Reviewer R7 — docs, status-file evidence & scope   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory on every branch. You make sure what the repo *says* matches what the branch *did* — the product owner reads only the
repo.

## Check
1. `docs/status/tasks/<id>.md` exists and has: what was built · how verified with **real pasted output** (commands, not prose) ·
   out of scope / left undone · open questions · decisions made. A "passed" claim without output → BLOCKER (R1 re-runs it; you check
   it is there and matches the tester reports).
2. Decisions: every non-trivial choice in the branch has a `docs/decisions/LOG.md` line with the options considered, or a
   `PENDING-*.md` when decision-policy says so. A decision-policy "always PENDING" item decided silently → BLOCKER.
3. Scope creep: anything built that the task prompt did not ask for (compare with its out-of-scope fence) → list it; ask for removal
   or a separate board row.
4. Docs updated with the code: READMEs of touched packages, `docs/user/**` for user-visible behaviour, `docs/lab/*` for new host
   rules, `docs/tech-debt.md` rows for known gaps (owner + date). Markdown links you can check resolve.
5. Board: `plan/tasks.yaml` row notes reflect the state; `python3 tools/board.py` still validates if the branch touched the board.

## Output
`docs/status/tasks/<id>-review-R7.md` — findings, verdict line.
