# Notifications review fixes

Branch: `codex/notifications-fix-20261003`; base: `20941c61`.
Worktree: `/root/.codex/worktrees/0b16/developers/Notifications-fix`.
Contract checkpoint: `52e80e09`; final consumer checkpoint is this document's commit.
Publication and validation are manager owned; this local checkpoint is not published or test verified.

Owned files: notification schema and schema tests; notification SMTP transport and transport tests; this status document only.

SMTP username/password reference must now both be present or both absent. Validation points to the missing nested field. Each rule rejects repeated channel references at the duplicate index, preventing repeated delivery from a single rule. The transport independently refuses partial authentication before DNS, secret lookup, or socket/transport creation for stale or bypassed input. Explicit complete credentials are passed to SMTP; deliberate unauthenticated relays remain supported.

Added regressions cover both partial configurations, no-auth and full-auth acceptance, distinct channels, nonadjacent duplicate targets, nested issue paths, runtime rejection before side effects, authenticated transport options, and deliberate anonymous transport options. No host calls or live deliveries performed. `git diff --check` passed. Automated tests, generation, typecheck, and independent re-review pending manager execution. No global guards changed.

Finite validation commands on this immutable tree (prepare dependencies as needed):

```sh
pnpm --filter @ngfw/schema test -- src/domains/ext/notifications.test.ts
pnpm --filter @ngfw/schema build
pnpm --filter @ngfw/api exec vitest run src/features/notifications/transport.test.ts
pnpm --filter @ngfw/schema typecheck
pnpm --filter @ngfw/api typecheck
```

Schema generation/contract guards and the unchanged hosted quick gate remain required before merge. Generated JSON schema artifacts have not been regenerated here; manager must regenerate in its validation worktree. No remaining implementation work identified for these two findings; reviewer confirmation and tests remain.
