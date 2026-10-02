# Recovery checkpoints and merge queue

Authoritative process: [../../AGENTS.md](../../AGENTS.md) and the owner instruction dated2026-10-02. All rows below are recovery references, not claims of completion or active runtime.

| Scope | Remote branch / PR | Recovery state |
|---|---|---|
| Main baseline | main471c61fa / PR57 | Complete hosted quick gate PASS |
| Dashboard metrics | codex/dashboard-recovered-20261002 / PR58 | Reviewed code, focused race PASS; hosted gate required |
| Manager audit/campaign | codex/manager-recovery-20261002 | Seven stale complete rows and seven partial rows reconciled; central deferred campaign |
| Notifications SMTP/webhook | codex/notifications-rebuild-20261002 | Contract checkpoint published; implementation/tests/review remain |
| Setup | task/setup-rebuild, local checkpoint24ecd113 | Source checkpoint; manager must confirm remote publication before durability claim |
| System identity | task/identity-current, local checkpointdd2bfe73 | Generated checkpoint; manager must confirm remote publication before durability claim |
| Multi-WAN | task/F-multiwan-recovery | Development started; publish first coherent checkpoint |

On resume: fetch remote branches; read each task's WIP/envelope; compare actual head to this historical table; refresh live inventory; inspect incomplete merges/worktrees; assign unfinished code and independent reviews. Update this file after publication/merge. If a local-only checkpoint disappeared, acknowledge the loss and reconstruct only the missing scope from repository specifications.

Merge order follows readiness rather than task age. No lab-only wait. No weakened assertions/lint/gates. Keep failures and deferred test evidence in `DEFERRED-ACCEPTANCE.md`.
