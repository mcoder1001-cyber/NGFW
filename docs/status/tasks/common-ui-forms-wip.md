# Common UI schema form migration WIP

Role: implementer. Branch: `codex/common-ui-forms-20261008`.
Base SHA: `417e8fcd`. Local checkpoint SHA: `ad7457d0`. Remote SHA: unpublished. `git push -u origin codex/common-ui-forms-20261008` failed HTTP 403: repository denies current authenticated account. Authorized GitHub connector tools are unavailable in this worker.
Owned files: `packages/ui-kit/src/schema-form/**`, `docs/status/tasks/common-ui-forms*`.

Task envelope: migrate shared schema form presentation to real Common UI controls while preserving RHF/Zod/schema validation, optional values, API pointer errors, focus, RTL and en/fa. No API changes, no root packaging changes. Dedicated worktree `/Users/mohammad/.codex/worktrees/common-ui-forms/NGFW`.

Completed: controlled PgText text/numeric/constant/JSON primitives, Common UI form actions, grouped inset panels. Completed also: real PgForm wrapper and controlled checkbox variant. Existing schema resolver and RHF remain authoritative; usePgForm dirty tracking cannot safely own nested schema state.
Actual checks: `tools/ci.sh check --base main` passed contract/forbidden-pattern checks, failed task-board validation because host python3 lacks PyYAML. Integrated root tree read-only validation authorized by manager: `node node_modules/vitest/vitest.mjs run src/schema-form --maxWorkers=2` from `packages/ui-kit`: 7 files / 63 tests passed, duration 106.00s. Covers schema validation/defaults, escaped server pointers, preserved hidden config, optional containers, RTL/en/fa, reset behavior and nested editor focus. TypeScript after initial declarations found exactOptional mismatches for optional form id and checkbox inputRef: callers now omit undefined props. Remaining PgFormButtons button-type error came from stale Common UI declarations (source accepts button/submit/reset, initial dist only submit). Manager widened adapter optional prop contract, so conditional omission is safe but optional. Rebuild library then recheck. ESLint found literal action metadata nested in JSX; moved translated descriptor array outside JSX without relaxing lint.
Remaining: manager independent review and mandatory complete quick CI. No remaining consumer implementation. Publication remains denied by configured GitHub identity.
Current dependency: none for consumer checks. Root has integrated implementation and lint fix; worker owns no root product edits.
Next command: manager independently reviews consumer diff and runs unchanged complete `tools/ci.sh quick` before merge.

## Final integrated validation, 2026-10-08

Manager explicitly authorized read-only checks on integrated root tree.
- `node node_modules/typescript/bin/tsc -p packages/ui-kit/tsconfig.json --noEmit`: PASS exit 0 (Common UI declarations rebuilt).
- From `packages/ui-kit`, `node ../../node_modules/eslint/bin/eslint.js src`: PASS exit 0 after lint fix integration. Existing Node module-type warning printed; no lint findings.
- From `packages/ui-kit`, `node node_modules/vitest/vitest.mjs run --maxWorkers=2`: 18 files / 90 tests; 84 passed, six WebSocket server tests initially failed with sandbox `listen EPERM 127.0.0.1`. Schema form 63 passed; server grid four passed.
- Required sandbox escalation retry: `node node_modules/vitest/vitest.mjs run src/ws/client.test.ts src/ws/client-auth.test.ts --maxWorkers=2`: two files / six tests PASS, exit 0, duration 760 ms.
- All 90 existing UI-kit tests therefore verified across full run and approved failed-subset retry. Full repository quick CI is still manager-owned and has not been claimed passing.
- Independent review of root controlled adapter: controlled PgText/PgCheck preserve caller value/change/ref/blur/errors/slotProps; manager fixed context-only legacy PgCheck/PgNumber callbacks and normal PgForm id/noValidate/sx forwarding. Reviewer did not approve own consumer code.

## Additional manager-authorized performance regression scope

Owned additional file: `packages/ui-kit/src/theme/NgfwThemeProvider.test.tsx` only; root implements cache lifetime code independently. Reviewed document-scoped direction cache lifetime: SSR isolated; no StrictMode cleanup flushing. Regression verifies retained sheet identities across repeated application mounts in both LTR/RTL. Isolated unchanged Interface DHCP toggle test passed in 35.006s versus 79.709s timeout in full suite, supporting cumulative stylesheet overhead investigation. Regression test execution pending manager integration.
