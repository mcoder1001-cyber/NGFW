# Notification recovery race fix

Branch `codex/notifications-recovery-fix-20261003`; base `3863734b` (includes schema lint correction). Worktree `/root/.codex/worktrees/0b16/developers/Notifications-recovery-fix`. Final local checkpoint is this document's commit. Root owns publication/tests/independent review.

Owned only notifications.service.ts, notifications.test.ts, this document. Configuration health now remains in runtimeError; delivery deadline health uses a separate flag. Error reporting prioritizes configuration health. Emit/test/wake/run block on either cause. Delivery timeout and final settlement cannot overwrite or clear a failed configuration reload; only successful reload clears configuration failure. A standalone delivery timeout still clears when its promise settles, preserving ordinary worker recovery.

Added three deterministic fake-timer regressions: resolve/reject late delivery after failed reload and fired deadline both preserve configuration-unavailable, refuse manual test/events and hold queued work until successful reload; isolated healthy-configuration timeout recovers on settlement. Mocked transport and datastore only; no sends/DNS/listeners.

Actual verification: Prettier and git diff --check passed. Direct small offline probe of extracted actual run method (only TS annotations/assertions removed), with fake timeout/deferred delivery, now leaves configuration-unavailable intact and clears only deliveryTimedOut. No Vitest/heavy test or typecheck run by worker.

Root finite commands on immutable checkpoint:

```sh
pnpm --filter @ngfw/api exec vitest run src/features/notifications/notifications.test.ts
pnpm --filter @ngfw/api typecheck
pnpm --filter @ngfw/api exec eslint src/features/notifications/notifications.service.ts src/features/notifications/notifications.test.ts
```

Remaining: root-owned automated verification, independent re-review, unchanged hosted quick gate. No schema/transport/global guard changes.
