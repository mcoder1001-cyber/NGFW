# F-backup-restore initial independent R6 review

Reviewed source: fb2f36f01. Reviewer owns only review/evidence files in isolated worktree `/root/ngfw-wt/f-backup-ux-review-20261005`, branch `codex/f-backup-ux-review-20261005`. Date: 2026-10-05.

## Findings

1. **MAJOR** — `apps/web/src/domains/system/backup-restore/BackupRestorePage.tsx:227`: run history interpolates raw `run.at` rather than the existing `useFormatters().dateTime` helper. Persian users see ISO timestamps and ASCII digits instead of locale/calendar formatting. Apply the formatter before `runSummary`; verify Persian history with a real run.
2. **MAJOR** — missing feature user documentation under `docs/user/`: no page explains encrypted backup, restore-to-candidate and ordinary commit, schedule secret references/destinations, template parameters, support redaction or upload/trial/confirm/rollback controls. Add an accurate page including CLI/API equivalents. Existing management and bundle installation pages do not describe these screens.
3. **MINOR** — `apps/web/src/domains/system/backup-restore/BackupRestorePage.tsx:243`: selecting “New template” after an existing template clears its name but retains description, parameter definitions and patch. The new template may silently inherit old configuration. Reset editor metadata in the selector's empty branch, or explicitly offer duplicate-template behavior.
4. **MINOR** — backup schedule/history/templates and upgrade status lack explicit loading/empty indicators. During slow first loads, the cards show largely blank contents; empty run history is indistinguishable from a pending request. Add translated loading and empty feedback.

## Source checks

The schedule uses the existing management JSON schema and ordinary config PUT. Restore and template apply stage into candidate and display diff; no immediate commit endpoint is called. Admin-only wrappers prevent non-admin privileged queries. Upgrade status rejects malformed/nonzero responses, binary upload precedes stage, state gates control availability, and activation/confirmation/rollback have translated confirmation dialogs. Both locale key sets match with genuine Persian text. Added layout uses logical maxInlineSize and no physical directional CSS.

Real-stack screenshots are **not produced yet**: final backend integration and private stack setup are pending at the manager. This is an initial source review, not UI acceptance.

**Verdict: BLOCK** — two unresolved MAJOR findings, and real-stack screenshot evidence pending.

## Manager follow-up

Manager reports final integration `056ea2273` adds `docs/user/system/backup-restore.md`; documentation finding applies to the initial reviewed snapshot only and awaits verification on that exact final source.

## Independently run focused verification

Commands in reviewer worktree after building workspace dependencies:

```text
pnpm --filter @ngfw/web exec vitest run src/domains/system/backup-restore/transport.test.ts src/domains/system/backup-restore/workflows.test.tsx

 RUN  v3.2.7 /root/ngfw-wt/f-backup-ux-review-20261005/apps/web

stdout | src/domains/system/backup-restore/workflows.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stdout | src/domains/system/backup-restore/transport.test.ts
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

 ✓ src/domains/system/backup-restore/transport.test.ts (5 tests) 96ms
 ✓ src/domains/system/backup-restore/workflows.test.tsx (4 tests) 3913ms
   ✓ backup and upgrade workflows > restores to candidate, clears the passphrase and shows diff without committing  1638ms
   ✓ backup and upgrade workflows > reads candidate schedule but replaces the management node through the config PUT route  1681ms
   ✓ backup and upgrade workflows > enables confirm only after the trial slot is actually active  469ms

 Test Files  2 passed (2)
      Tests  9 passed (9)
   Start at  16:15:26
   Duration  13.53s (transform 4.64s, setup 1.09s, collect 14.39s, tests 4.01s, environment 2.05s, prepare 476ms)


pnpm --filter @ngfw/web typecheck
$ tsc -p tsconfig.json --noEmit
exit code 0
```

## Final source verification (2026-10-05)

Exact product source `5eb2378df`, tree `eea361b241567947405742458d6817e39ca133b8`, merged into isolated reviewer worktree only. DateTime now goes through useFormatters; New template resets all metadata; history/templates/schedule have loading or empty feedback. Feature user documentation is present and matches restore candidate/normal commit, archive limits, schedule references, templates/support and upgrade controls. Prior two MAJOR findings and metadata MINOR resolved. Remaining MINOR: upgrade status still lacks explicit initial loading feedback; disabled Refresh button is the only fetching indication. No physical directional CSS found in the changed feature.

Final independent focused tests: nine tests passed; web typecheck exited 0. Actual output:

```text

 RUN  v3.2.7 /root/ngfw-wt/f-backup-ux-review-20261005/apps/web

stdout | src/domains/system/backup-restore/workflows.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stdout | src/domains/system/backup-restore/transport.test.ts
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

 ✓ src/domains/system/backup-restore/transport.test.ts (5 tests) 86ms
 ✓ src/domains/system/backup-restore/workflows.test.tsx (4 tests) 3969ms
   ✓ backup and upgrade workflows > restores to candidate, clears the passphrase and shows diff without committing  1680ms
   ✓ backup and upgrade workflows > reads candidate schedule but replaces the management node through the config PUT route  1696ms
   ✓ backup and upgrade workflows > enables confirm only after the trial slot is actually active  407ms

 Test Files  2 passed (2)
      Tests  9 passed (9)
   Start at  16:31:25
   Duration  16.36s (transform 6.72s, setup 1.44s, collect 17.11s, tests 4.06s, environment 3.15s, prepare 478ms)

$ tsc -p tsconfig.json --noEmit
exit code 0
```

Browser attempt uses actual Vite 16600 proxy to actual Nest API 12600. Both independent attempts failed at real login because API 12600 stopped: Vite returned HTTP 500 for auth endpoints; direct socket connection refused (errno 111). No screenshot or browser PASS claimed. Manager notified to restart its owned API fixture.

Final source findings verdict: **APPROVE** (remaining optional MINOR); screenshot acceptance awaits actual stack rerun.

## Actual browser closure

Manager restarted its actual private API; eight screenshots and one candidate workflow screenshot now committed. en/fa RTL and light/dark render checks, backend-derived upgrade gates and real template stage/apply preview pass. Zero pageerrors. Expected anonymous-refresh401 and navigation aborts documented in T4 report. No feature endpoint failure on the valid flow. No real upgrade mutations. Final R6 **APPROVE**; T4 bounded UI **PASS**, see F-backup-restore-test-T4.md for runtime fixture limits.
