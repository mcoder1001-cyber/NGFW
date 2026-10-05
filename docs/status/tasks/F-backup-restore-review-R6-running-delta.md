# Independent R6/T4 narrow running-status closure

Reviewed/tested source: `2560e85119e2c3ea08e6523d6f77a61927aa0423`, tree `920a0d37c7d9288820884fde232a67e7deff4c14`. Date2026-10-05. Isolated reviewer worktree `/root/ngfw-wt/f-backup-ux-review-20261005`, branch `codex/f-backup-ux-review-20261005`. Own files are review/test evidence only; no product edits.

The changed UI explicitly recognizes running, success and failure. English running label is “In progress”; Persian is «در حال اجرا». Only actual failure receives the error color. Existing date localization is preserved. Documentation now accurately explains that a crash can leave a running history row indefinitely; it directs users to inspect the destination rather than interpreting the row as success. No actionable R6 issue found in this delta.

Meaningful independent React DOM tests render all three states together in en and fa. They assert that running has the translated progress label, does not say failure, uses the same computed text color as success, and differs from failure. These are component tests with mocked transport data, not real-backend screenshots. Test/config evidence remains under docs/status/tasks and uses existing installed Vitest/React/UI dependencies. Existing nine workflow/transport tests are rerun in the same invocation. No shared root browser process was changed. The prior eight real screenshots remain evidence for unchanged layout and controls, not a new running-state browser observation.

Commands:

```sh
# cwd: reviewer worktree/apps/web
node node_modules/vitest/vitest.mjs run --config ../../docs/status/tasks/F-backup-running.vitest.config.ts
# cwd: reviewer worktree
pnpm --filter @ngfw/web typecheck
```

Actual final output:

```text

 RUN  v3.2.7 /root/ngfw-wt/f-backup-ux-review-20261005/apps/web

stdout | src/domains/system/backup-restore/workflows.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stdout | ../../docs/status/tasks/F-backup-running.review.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stdout | src/domains/system/backup-restore/transport.test.ts
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

 ✓ src/domains/system/backup-restore/transport.test.ts (5 tests) 93ms
 ✓ ../../docs/status/tasks/F-backup-running.review.test.tsx (2 tests) 2098ms
   ✓ renders running as localized non-error status in en  1229ms
   ✓ renders running as localized non-error status in fa  865ms
 ✓ src/domains/system/backup-restore/workflows.test.tsx (4 tests) 3804ms
   ✓ backup and upgrade workflows > restores to candidate, clears the passphrase and shows diff without committing  1658ms
   ✓ backup and upgrade workflows > reads candidate schedule but replaces the management node through the config PUT route  1448ms
   ✓ backup and upgrade workflows > enables confirm only after the trial slot is actually active  504ms

 Test Files  3 passed (3)
      Tests  11 passed (11)
   Start at  17:25:27
   Duration  13.27s (transform 4.33s, setup 1.37s, collect 20.81s, tests 5.99s, environment 3.59s, prepare 670ms)


Scope: all 8 workspace projects
✓ Lockfile passes supply-chain policies (verified 26m ago)
Lockfile is up to date, resolution step is skipped
Progress: resolved 1, reused 0, downloaded 0, added 0
Packages: +6 -9
++++++---------
Progress: resolved 6, reused 6, downloaded 0, added 6, done
.../node_modules/@swc/core postinstall$ node postinstall.js
.../node_modules/protobufjs postinstall$ node scripts/postinstall
.../node_modules/protobufjs postinstall: Done
.../node_modules/@swc/core postinstall: Done
Done in 11.5s using pnpm v12.5.1
$ tsc -p tsconfig.json --noEmit
exit code 0
```

Earlier evidence harness attempts had environment/setup failures: automatic package synchronization overlapped and docs-directory JSX runtime initially lacked resolution. Finished package synchronization and explicit evidence-only package aliases resolve these; final invocation completes11/11. No product test was weakened or edited. All owned commands completed before handoff.

**R6 verdict: APPROVE. T4 narrow delta verification: PASS through meaningful rendered component assertions; no new browser run claimed.**
