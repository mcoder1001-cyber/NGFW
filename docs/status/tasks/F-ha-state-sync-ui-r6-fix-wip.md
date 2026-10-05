# F-ha-state-sync: assigned R6 UI repair

Branch: codex/ui-ha-r6-fix-20261005. Worktree: /dev/shm/ui-ha-fix-20261005. Frozen source: 834eed6fb94497321aece0f3b77d9fec055afe71.
Owner: /root/ui_bfd_ha_r6_fix. Own only the relevant apps/web domain components/tests, en/fa locales and this repair envelope/status.

Implemented bounded R6 corrections; regressions preserve all existing assertions. Dependency installation passed. Dependency build and focused tests/typecheck/lint remain pending. No independent approval claimed. No shared daemon, package, service, main, or VPP changes.

Next: focused Vitest suites, web typecheck and scoped ESLint, tools/ci.sh check --base 834eed6fb94497321aece0f3b77d9fec055afe71. Full final integration gate belongs to manager.
Remote checkpoint: publication pending for this coherent change; exact result follows immediately.

## Completed developer verification

Initial published checkpoint: local ae0de6f87; remote c94f76ba8437b48df309fa75372d5e164d0a25a2; tree c962fa1f9bf97a9d247dfbf66f7c32902c040a22. Connector publication confirmed exact tree. Final regression supplement follows in the branch history.

`pnpm --filter @ngfw/web exec vitest run src/domains/system/ha src/domains/system/ha-state-sync --maxWorkers=2`: PASS 2 files / 12 tests, 15.84s. All original 9 tests retained. Added actual TanStack cached success → failed refresh (cached data verified retained internally, active reading suppressed, resync disabled), recovery, nonempty observationError, in-flight refresh suppression of action, operator denial.
`pnpm --filter @ngfw/web typecheck`: PASS on final tests. Scoped ESLint: PASS zero findings (Node emits existing config module-type warning). `tools/ci.sh check --base 834eed6fb94497321aece0f3b77d9fec055afe71`: PASS 10s. Dependency build 12/12 PASS 2m22.214s; generated outputs unchanged.

No browser, real appliance, live VPP or complete quick CI claimed. Fresh independent R6/T4 must verify. Full exact integration gate is manager's next stage. Next command: independently rerun Panel.test.tsx and the original R6 cached-refresh replay against this branch.
