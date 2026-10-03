# Selective Notifications integration — 2026-10-03

Branch `codex/notifications-integration-20261003`; base `origin/main` a2378278; worktree `/root/.codex/worktrees/0b16/developers/Notifications-integration`.
Source feature checkpoint `origin/codex/notifications-tested-checkpoint-20261003` 0d691dce; verified UI followup 7f4300c8. Contract-first source checkpoint c4ac7fec.

Owned transfer: `packages/schema/src/domains/ext/notifications*`, schema management field/export; `apps/api/src/features/notifications/**`; anchored notification hooks in app.module, commit.service, validation.service, datastore/documents, global-blocking.service, state.controller; notifications UI/tab/management tests/en+fa notification locale keys; API nodemailer dependency additions/lockfile; user notifications docs; this status. No P10/bundle, Telegram support, other historical main differences, shared service operation, push or merge transferred.

Generated API client, YANG and CLI operations require regeneration on this tree. Static read identified a checkpoint omission: optional management.notifications still needs a proto mirror for the structural schema→proto drift guard (no schema→proto exceptions). Parent notified; additive ManagementConfig.notifications field 7 currently free. Added the notification-specific proto mirror and a populated email/webhook roundtrip regression in contract commit 00ac9901 before committing consumers. No heavyweight test or live delivery executed in this integration worktree. Prior followup evidence: regression 2/2, ManagementPage 9/9, web typecheck/build dependencies passing; that evidence applies to prior checkpoint only.

Next: root regenerates generated outputs and executes focused contract/API/UI validation on the immutable consumer checkpoint, then commits generated outputs and real evidence. ETA: 10–15 minutes preparation plus root validation duration.


Root finite validation commands (run under the root-owned heavy lane; reviewer/worker has not executed these):

```bash
pnpm install --frozen-lockfile
pnpm gen
make -C apps/cli gen
pnpm --filter @ngfw/api-client --filter @ngfw/ui-kit run build
pnpm --filter @ngfw/schema exec vitest run src/domains/ext/notifications.test.ts --maxWorkers=1
pnpm --filter @ngfw/proto exec vitest run test/parsed-documents.test.ts --maxWorkers=1
pnpm --filter @ngfw/api exec vitest run src/features/notifications --maxWorkers=1
pnpm --filter @ngfw/web exec vitest run src/domains/system/management/NotificationsTab.test.tsx src/domains/system/management/ManagementPage.test.tsx --maxWorkers=1
pnpm --filter @ngfw/api typecheck
pnpm --filter @ngfw/web typecheck
env -u NGFW_INTEGRATION go -C apps/agent test ./internal/contracttest
```

Expected tracked generated outputs: packages/proto/gen/ts, apps/agent/gen/ngfw/v1, packages/api-client/src/generated/schema.d.ts, packages/yang/generated/ngfw-management.yang, apps/cli/internal/api/operations_gen.go. Regeneration may report other baseline dirt; assess separately and do not copy unrelated historical outputs. Mandatory quick CI and independent review of the complete resulting tree are still pending.
