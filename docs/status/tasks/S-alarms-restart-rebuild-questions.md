# S-alarms-restart-rebuild — questions (none blocking; the work is complete without them)

## Q1 — clear rebuilt rows whose rule still exists but no longer matches them?
The rebuild seeds every active row. A row whose rule still exists but no longer matches it is never sampled again, so
it stays `active`. This happens when the rule's `metric` changed, or when its `interface` filter now excludes the row's
instance. The same gap exists today when such an edit is committed while the API runs: `pruneRules` only checks
existence and `enabled`. The task scopes orphans to "rule no longer exists", so I did not add this. If wanted, it is a
small follow-up in `apps/api/src/features/dashboard-prom-alarms/`: clear with reason `rule-changed`, at rebuild and on
reload.

## Q2 — cleanup refused by the permission system (manager action needed)
After the gate passed, my cleanup command was denied. It would have removed the git-ignored build outputs in this
worktree plus the CI temp dir. Per the envelope a refusal is final, so I did not retry another way. Left behind:
- **`/tmp/g-w10` — 552 MB on tmpfs (RAM)**: the envelope says to delete it before finishing. Please `rm -rf /tmp/g-w10`.
- Build outputs (all git-ignored; the shared-host rules ask to delete them): `apps/api/dist`, `apps/web/dist`,
  `packages/{api-client,proto,schema,ui-kit,yang}/dist`, `apps/agent/bin` under `/root/ngfw-wt/S-alarms-restart-rebuild`.
Everything else is clean: every process I started was stopped by PID, ports 4000 and 4051 are free, the fake-agent
socket is gone, the Valkey keys under `ngfw:w10:alarmsverify:` were deleted, and the worktree has no uncommitted changes.

## Note — prompt naming
The prompt says `ruleKey(rule, instance)` "exactly as in engine.ts". `engine.ts` names that function `stateKey`, and I
used it (same key: `rule + '\u0000' + instance`).
