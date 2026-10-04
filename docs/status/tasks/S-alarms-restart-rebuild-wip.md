# S-alarms-restart-rebuild — WIP

Started 2026-10-01 20:25 (+0330), slot 10 (w10), branch task/S-alarms-restart-rebuild, base main@55a18e0f.

## Plan
1. `AlarmsService.start()`: seed the engine state from `alarm` rows with `state='active'`
   (`stateKey(rule, instance)` → `{since: raisedAt, raised: true}`) before the rules load and before `openStats()`.
2. Orphans (rule removed, or disabled) are cleared by the existing `pruneRules` path inside `reload()`, now with a
   reason (`rule-removed` / `rule-disabled`) on the system event, the bus event and the webhook payload — one
   `cleared` per row.
3. Unit test `alarms.service.test.ts` (fake Drizzle db, fake agent stream, mocked webhook `deliver`).
4. Verify once on ngfw_w10 with the API on port 4000; paste psql output.
5. Status file, `/root/NGFW/tools/ci-slot.sh --base main` (D-224).

## Progress
- [x] read context, task, envelope; read feature code
- [x] pnpm install (frozen, prefer-offline); deps built (proto, schema, yang)
- [x] fix (commit aa915672)
- [x] unit test: 6/6 pass with the fix, 6/6 fail against the unfixed service
- [x] slot verification on ngfw_w10 / port 4000 (fixed: cleared + 1 webhook; unfixed contrast: row stays active);
      every process stopped by PID, Valkey keys removed
- [x] prettier-format + typecheck (exit 0) + own tests re-run (12/12 pass), commit 65cdd7c8
- [x] status file written; CI gate PASSED on ccf7048f (/root/NGFW/tools/ci-slot.sh --base main, 14m49s)
- DONE 2026-10-01 21:30 — see docs/status/tasks/S-alarms-restart-rebuild.md
