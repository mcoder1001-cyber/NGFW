# Notifications resume — independent R1 delta review

Reviewed frozen HEAD `b0b646e531298d09a823b3fd923b0a59969dee7f`, tree `96964d7f936d9014768e7b234e2297901f6d8fb1`. Reviewer branch `codex/notifications-correctness-review-20261002`, isolated worktree `/workspace/scratch/de92de7d9874/NGFW-notifications-r1`. No product edits.

Scope assigned by manager: fresh correctness review of resumed throttle/schema/UI continuation, without duplicating pending complete gate. Read R1 prompt, task, recovery WIP and actual changes from recovered `aeefa86c`. Security reviewed separately; this report does not replace whole-feature acceptance.

## Findings

**MINOR — inherited UI permission mismatch**, `apps/web/src/domains/system/management/NotificationsTab.tsx:80`. The form uses `!perms.editConfig`; that capability permits operators, whereas notification config is server-enforced admin-only. An operator is shown an editable form whose save necessarily fails authorization. No privilege bypass: API authorization remains correct. Suggested correction: use `perms.role !== 'admin'` and verify an operator sees a read-only notification form. Reported to developer; optional minor correction, not a security blocker.

No BLOCKER/MAJOR in the assigned continuation delta. The reload fix now retains each existing channel's test throttle while removing keys for deleted rules/channels. `test:` key namespace cannot collide with object names (colon excluded by schema). Reconfigure retains the deadline rather than extending it; expiry permits the next test. Disabled and missing running channels still reject. Scalar regex slash escape removal is equivalent. SMTP username refinement rejects CR/LF via regex and NUL explicitly, retains existing length limits and channel discrimination. UI namespace constant evaluates to exactly the prior field prefix and preserves localization behavior.

Clarification of requested UI scope: at this exact head there is no changed active/disabled-channel counter. Queue count renders `state.queued`; configuredChannels counts all configured channels in API state. Test controls render candidate channel names and dispatch against enabled running state, as the notice says; API rejects missing/disabled running channels. No newly implemented active-count behavior is claimed.

## Independent commands and results

Pinned PATH `/workspace/scratch/96b8b6fbc8a7/toolchain/bin`; source tests run in this isolated worktree. node_modules reused via symlinks to worker installs; workspace package builds resolve through that installation. These are targeted source tests, not clean-install or complete build evidence.

```text
# apps/api
node node_modules/vitest/vitest.mjs run src/features/notifications/notifications.test.ts src/features/notifications/transport.test.ts
notifications.test.ts (30 tests) 116ms
transport.test.ts (24 tests) 43ms
Test Files 2 passed (2)
Tests 54 passed (54)
Duration 6.34s

# packages/schema
node node_modules/vitest/vitest.mjs run src/domains/ext/notifications.test.ts
notifications.test.ts (5 tests) 8ms
Test Files 1 passed (1)
Tests 5 passed (5)
Duration 592ms

# apps/web
node node_modules/vitest/vitest.mjs run src/domains/system/management/ManagementPage.test.tsx
ManagementPage.test.tsx (6 tests) 2890ms
Test Files 1 passed (1)
Tests 6 passed (6)
Duration 6.76s
```

All commands exited 0; `git diff --check main...HEAD` exited 0. Management suite emitted existing HydrateFallback warning. Its six tests verify Management tab integration and existing TLS behavior, not comprehensive NotificationsTab interactions; do not misrepresent it as dedicated notification UI coverage.

Dispatcher tests cover 100-event throttling, bounded retries/backoff, history/queue limits, rejected disabled or missing running channels, reload throttle retention/expiry, queued-rule revalidation, singleflight reload/shutdown, event sanitization and one-worker concurrency. Transport tests use mocked DNS/nodemailer, not real SMTP/HTTP fixtures. Direct schema tests verify CR/LF/NUL, accepted normal accounts and Telegram rejection.

Whole-feature acceptance is still incomplete as the existing WIP states: real host-independent SMTP/HTTPS fixture reception and dedicated notifications UI tests remain absent; non-default management VRF binding and IPsec event mapping remain unimplemented. Those are not transformed into PASS or laboratory-only deferrals by this delta review. Complete unchanged quick gate was not duplicated per manager assignment and is still required on the final integration tree.

Verdict: **APPROVE WITH CHANGES — bounded R1 continuation delta; one optional MINOR above**. No whole-feature acceptance or merge-gate PASS claimed.
