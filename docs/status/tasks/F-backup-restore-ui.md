# F-backup-restore UI implementation

Author role UI developer; cannot provide independent R6/T4 review. Own isolated branch codex/f-backup-restore-ui-20261005. Authored frontend commits66d1ac2cc and7b7fe1ac8; borrowed contract527bd51ef and backendd23d80a66 are build-only bases and must not be integrated again as UI work.

Implemented both admin-only pages with real authenticated endpoints: encrypted backup download; restore upload -> candidate diff -> existing pending-change commit/discard bar; contract-driven export schedule; template JSON definition editor and schema-driven parameter application; redacted support bundle download; upgrade slot status, binary Blob streaming upload/stage and activation/confirmation/rollback dialogs. en/fa strings, logical sizing, routes/nav registered. Binary retries reconstruct fresh requests from reusable Blob, never clone/tee multiGiB uploads. Passphrases cleared on submission, never written to storage.

## Evidence

Commands ran in /root/ngfw-wt/f-backup-restore-ui-20261005 (through tools/heavy.sh for build/test).

`tools/ci.sh check --base origin/main`:
```
check PASSED (0m10s)
```

`pnpm --filter @ngfw/web typecheck` after dependency builds, source7b7fe1ac8 and final generated-request delta dfc463089:
```
$ tsc -p tsconfig.json --noEmit
```
exit0.

`pnpm --filter @ngfw/web exec eslint src/domains/system/backup-restore src/i18n.ts src/router.tsx src/nav/nav.ts`: exit0. Existing repository Node module-type warning; no rule failures.

`pnpm --filter @ngfw/web exec vitest run src/domains/system/backup-restore`:
```
 Test Files  2 passed (2)
      Tests  9 passed (9)
   Start at  15:58:13
   Duration  12.49s
```
Tests cover raw upload and401 token-refresh binary replay, problem pointers, archive-limit rejection before reading, malformed upgrade status rejection, operator cannot fetch admin endpoints, restore clears passphrase and stages diff without commit, candidate schedule GET versus config-node PUT, trial confirmation disabled before slot boots.

## Required follow-up

Browser screenshots NOT RUN: backend private PostgreSQL/Valkey authenticated stack has not yet been provided; T4 needs both pages in en/fa light/dark, real candidate endpoints and safe upgrade fake-argv status fixture. No browser/live acceptance is claimed. Slot28 ports verified by tools/lab env: HTTP12800 WEB16800 metrics9381 database ngfw_w28; no servers or host mutations performed.

The complete mandatory hosted quick gate and independent review remain manager prerequisites. Backend current generated success responses for templates/runs/upgrade are absent and restore has only staged without diff; backend notified. Frontend template input and upgrade operation types derive generated paths. Own `turbo run gen --filter=@ngfw/api-client` passed9 tasks in2m6.209s with missing200 response warnings; no handwritten generated edits. Generated client artifacts are backend-owned; no handwritten generated edits.
