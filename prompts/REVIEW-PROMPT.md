# Task: Review branch task/<id>   (prepend 00-CONTEXT.md)

This file is the **entry point** of the review. Since D-156 a branch is reviewed by a **panel of specialised reviewers**, each
with its own prompt in [`reviewers/`](reviewers/). You are either the **dispatcher** (the envelope says `role: review-dispatch`)
or one **panel reviewer** (the envelope says `role: reviewer R<n>`; read the shared rules below, then your `reviewers/R<n>-*.md`).
Git is local-only: the "PR" is the branch `task/<id>` plus `docs/status/tasks/<id>.md`. You did not write this code. Be adversarial
and specific. Reviewers never write feature code; do not fix the code yourself unless the envelope says `--fix`.

## The panel

| id | aspect | prompt |
|---|---|---|
| R1 | correctness & tests | [`reviewers/R1-correctness-tests.md`](reviewers/R1-correctness-tests.md) |
| R2 | security (secrets, authz, injection, shell rules) | [`reviewers/R2-security.md`](reviewers/R2-security.md) |
| R3 | contracts / API & backward compatibility | [`reviewers/R3-contracts-api.md`](reviewers/R3-contracts-api.md) |
| R4 | data-plane / VPP safety & shared-host rules | [`reviewers/R4-dataplane-sharedhost.md`](reviewers/R4-dataplane-sharedhost.md) |
| R5 | performance & scale | [`reviewers/R5-performance-scale.md`](reviewers/R5-performance-scale.md) |
| R6 | UX / web / i18n (en/fa, RTL, logical CSS) | [`reviewers/R6-ux-web-i18n.md`](reviewers/R6-ux-web-i18n.md) |
| R7 | docs & status-file evidence, scope | [`reviewers/R7-docs-evidence.md`](reviewers/R7-docs-evidence.md) |
| R8 | operability & packaging | [`reviewers/R8-operability-packaging.md`](reviewers/R8-operability-packaging.md) |

## Who is mandatory for which change (the dispatcher applies this to `git diff --name-only main...task/<id>`)

| changed paths / kind of change | mandatory reviewers |
|---|---|
| **every branch** | R1, R2, R7 |
| `packages/schema/**`, `packages/proto/**`, `packages/api-client/**`, any `*/gen/**`, `apps/api/src/**` routes/DTOs, `docs/04-api-datamodel.md`, `contract(` commits | R3 |
| `apps/agent/**` (descriptors, renderers, subsystems, vpp), `deploy/vpp/**`, `test/topology/**`, `tools/lab`, anything that creates VPP objects, daemons, ports, tables or DB rows on the shared host | R4 |
| a hot path: per-packet/per-object loops, reconcile/dump, WS streams, DB queries in list endpoints, anything with "scale", "bulk", "10k", timers < 5 s | R5 |
| `apps/web/**`, `packages/ui-kit/**`, locales, screenshots, user docs under `docs/user/**` | R6 |
| `deploy/**`, `tools/**`, `packaging`, systemd units, `.deb`/ISO (P10/P14), `tools/ci.sh`, migrations, logging/metrics/alarms | R8 |

The dispatcher may add any reviewer when the diff touches its aspect in a place the table does not list (e.g. R2 is already
mandatory; add R5 for a new list endpoint). It never removes a mandatory one. Small branches (≤ 50 changed lines, docs-only) still
get R1, R2, R7 — a docs-only branch is R7 + R2 (secrets in docs) only.

**Dispatcher output:** `docs/status/tasks/<id>-review-plan.md` — the list of changed paths grouped by area, the reviewers to
spawn (mandatory + added, one reason each) and the testers the change needs (see `TESTER-PROMPT.md` §Who runs). The manager spawns
them in parallel, each in a fresh agent.

## Shared rules for every panel reviewer
- Review only your aspect; if you see something serious outside it, record it as `[other: R<n>]` — do not grade it.
- Each finding: **severity**, file:line, the failure scenario, the fix. Severities:
  - **BLOCKER** — must be fixed before merge: wrong behaviour, a broken rule in `00-CONTEXT.md` / shared-host rules / decision
    policy, a contract reshape, a secret leak, unproven claims, a test that does not run.
  - **MAJOR** — must be fixed before merge unless the manager (or an arbiter) accepts a tech-debt row in `docs/tech-debt.md` with
    an owner and a date.
  - **MINOR / NIT** — optional; the developer may fix or answer "won't fix" with one line.
- Evidence: any claim that something "passes" must be a command you ran in `/root/ngfw-wt/<id>` with its output pasted.
- Verdict per reviewer: **BLOCK** (≥ 1 BLOCKER, or a MAJOR without an accepted debt row) / **APPROVE WITH CHANGES** (MAJORs the
  developer agreed to fix in this branch, MINORs) / **APPROVE**.
- Write `docs/status/tasks/<id>-review-R<n>.md` in the worktree: findings ranked by severity, then the verdict line.

## Combining the verdicts
The manager (or dispatcher) writes `docs/status/tasks/<id>-review.md`: one row per reviewer (verdict, number of BLOCKER/MAJOR/
MINOR, file link) and the **combined verdict**:
1. Any mandatory reviewer **BLOCK** → **BLOCK**. An added (non-mandatory) reviewer's BLOCK counts the same.
2. Else any **APPROVE WITH CHANGES** → **APPROVE WITH CHANGES**: the developer fixes the listed MAJORs; the reviewers who raised them
   re-check only those findings (a verify round, not a full review).
3. Else **APPROVE**. Merge also needs the tester reports to be PASS (`TESTER-PROMPT.md`).
4. Two reviewers contradicting each other (one demands what the other forbids), a developer disputing a BLOCKER, or a third round
   → do not loop: the manager hands it to the arbiter (`ARBITER-PROMPT.md`). The ruling is final for the branch.

## Check list kept from the single review (every item now has an owner)
1. Contract compliance → R3 · 2. real verification (VPP state, not mocks) → R1 + R4 · 3. restart safety, `Retrieve` → R4 ·
4. VPP API provenance (`apps/agent/binapi/` only, untouched) → R4 · 5. shared-host rules → R4 · 6. security greps → R2 ·
7. transaction semantics / rollback → R1 · 8. UI honesty (real endpoints, screenshot) → R6 · 9. scope creep → R7 ·
10. i18n, logical CSS → R6 · 11. tests actually run (`tools/ci.sh --base main` yourself, compare with the pasted output) → R1
(R7 checks the status file carries it).
