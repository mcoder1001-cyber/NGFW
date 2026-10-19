# Common UI focused repair — 2026-10-09

User authorized focused repairs while retaining RHF/SchemaForm; do not migrate NGFW to usePgForm. Existing Common UI integration changes preserved and left uncommitted. No Git commit, rebase, push or remote publication performed.

NGFW branch codex/common-ui-migration-20261008 at local HEAD 726c1e79ddccbc6c25f888821ef9e5e96b05e197. Common UI source /Users/mohammad/webngf/common-ui, issues/PGWUI-3339 at 2438f35be69d107e4a10503adcb5f60232345184. Remote SHAs unverified.

Owned NGFW files: packages/ui-kit/src/data-grid/{GeereedDataGrid.test.tsx,CommonUiCompatibility.test.tsx}, local yalc/dependency refresh, this status/envelope. Library owned repair paths documented in documentation/ngfw-focused-repair-wip.md. Source pre-repair copies preserved at /private/tmp/common-ui-focused-repair-baseline.

Completed local repair and verification. RHF/SchemaForm remains the NGFW form owner; Common UI legacy APIs remain available. Independent reviewer found no remaining introduced blockers after corrections. Common UI source changes are uncommitted, including all preexisting integration edits.

Common UI unified ESM/CJS/DTS build passed. Local yalc signature ff1badb6519a295a8974ffb4781bdd19. All 68 built distribution files match NGFW .yalc and installed node_modules byte-for-byte. Offline dependency installation completed without registry downloads or lockfile changes.

Actual final checks:
- Common UI, NGFW web and UI-kit noEmit typechecks passed; UI-kit build passed.
- Common UI broad suite 74/75 initially passed. Fixed the legacy native-grid test's unawaited page assertions/outdated lodash debounce mock; affected source rerun 23/23 passed. Final built-package shared-context tests 2/2 passed. Full detail in Common UI documentation/ngfw-focused-repair-wip.md.
- Complete NGFW UI-kit: 102/102 tests, 21 suites passed.
- Selected web interfaces/ACL/setup/widgets/object-picker: 43/43 tests, 6 suites passed.
- Final installed-package form/grid recheck after adapter deduplication: 26/26 tests, 4 suites passed.
- Changed-test ESLint passed.
- Production build, no-dev-routes check, and unchanged bundle budget passed. Initial gzip total 614384/614400 bytes; only 16 bytes headroom. Budget was not increased.

All test/build logs: /private/tmp/ngfw-review-20261009. Remaining code: none within reviewed scope. Current failure: none in final relevant checks. Full hosted quick CI, complete web suite and real API/device acceptance remain unverified. No Git commits, rebase, push, merge or remote publication performed.

Next action: owner review of local changes. Optional next verification: real API/device acceptance; do not claim offline/test-fixture coverage as device acceptance.
