# F-bfd-redistribution: assigned R6 UI repair

Branch: codex/ui-bfd-r6-fix-20261005. Worktree: /dev/shm/ui-bfd-fix-20261005. Frozen source: 97f67a2cd3a3584fea37f7956bb302cd722b0a52.
Owner: /root/ui_bfd_ha_r6_fix. Own only the relevant apps/web domain components/tests, en/fa locales and this repair envelope/status.

Implemented bounded R6 corrections; regressions preserve all existing assertions. Dependency installation passed. Dependency build and focused tests/typecheck/lint remain pending. No independent approval claimed. No shared daemon, package, service, main, or VPP changes.

Next: focused Vitest suites, web typecheck and scoped ESLint, tools/ci.sh check --base 97f67a2cd3a3584fea37f7956bb302cd722b0a52. Full final integration gate belongs to manager.
Remote checkpoint: publication pending for this coherent change; exact result follows immediately.

## Completed developer verification

Initial published checkpoint: local c45ba3d3b; remote 091a1db0e7ee929cc2c315efb644f0868b1c0dd9; tree 99b43a82cc24d4018f8e01201c00b1e9c7789f2e. Connector publication confirmed exact tree. Final regression supplement follows in branch history.

Initial App+BfdPage+BfdRedistributionPage suites PASS 3 files / 17 tests, 131.57s. Original Down-with-backend-error/no-synthetic-Up assertion then restored as a separate retained case: focused BFD suites PASS 2 files / 6 tests, 14.51s. Final empty wording guard suppresses false empty reports on agentError; final wrapper suite PASS 4 tests, 16.55s. Tests cover every observed known state up/down/init/admin-down, unknown and unexpected-state localized fallback, actual en/fa render, VRF context, localized empty state, errors with no synthetic healthy reading. All original assertions preserved; labels updated to localized Down/Up.

`pnpm --filter @ngfw/web typecheck`: PASS. Scoped ESLint: PASS zero findings (existing Node config-module warning). `tools/ci.sh check --base 97f67a2cd3a3584fea37f7956bb302cd722b0a52`: PASS 10s. Dependency build 12/12 PASS 2m29.522s; generated outputs unchanged. One rerun accidentally omitted TMPDIR and failed before collecting tests with /tmp inode ENOSPC; corrected owned RAM TMPDIR rerun above passed. No cleanup of shared caches/temp.

No browser, real appliance, live VPP or complete quick CI claimed. Fresh independent R6/T4 must verify. Full exact integration gate belongs to manager. Next command: independently replay original R6 Persian-state probe against this branch.
