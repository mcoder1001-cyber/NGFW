# Independent T1 focused lifecycle evidence

Tester/root independent of developer. Local tested30536224c959b42762b05a753a173020c0f61c78; initial published163872a58896925052b3a9ebaa0b78fe382f3e66 treeeb580de9c468cf63057fa47476e1a8e804b03ff9. Worktree work/NGFW-management-lifecycle.

Actual focused command/output:
```text
pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
RUN v3.2.7 .../NGFW-management-lifecycle/apps/api
✓ src/features/mgmt-tls/mgmt-tls.test.ts(13tests)650ms
Test Files1passed(1)
Tests13passed(13)
Start at12:16:17
Duration9.56s(transform3.34s, setup0ms, collect7.66s, tests650ms, environment1ms, prepare272ms)
```

Real local TLS/Fastify tests exercise late firstcertificate startup and actual enabledstate, bindfailure EADDRINUSE then successfulretry, retained actualcertificate/state after reference removal, shutdown disabledstate and sequenced reload/finalcertificate. Credentials/refusal/stream delivery and validator tests also included. Mock desired-state/secret/auth stores used; no DB/browser/hardware acceptance claimed.

Focused verdictPASS at30536224. Overall T1 integration PENDING; parentPR99 fullquick failed TS2345 exact optional test subject type despite focuspassing. Developer correcting type in99 and100, so final correctedSHA must be typechecked and focus reverified. Applicable fresh panel absent/runtime cap; neither candidate is merge-ready.

## Final strict typing correction independent rerun
Local eabeb473fc2b2e51af8ed2c17253e82593cae697.
Published df83a412789457cb6a6b52a3412379132604f850.
Tree3c49a16c56e20a2440dc5305bff85e7e9721b57b independently verified by GitHub API.
```text
pnpm --filter @ngfw/api typecheck
$ tsc -p tsconfig.json
(exit0, no errors)
pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
✓ src/features/mgmt-tls/mgmt-tls.test.ts(13tests)633ms
Test Files1passed(1)
Tests13passed(13)
Start at12:25:04
Duration9.29s
git rev-parse HEAD
eabeb473fc2b2e51af8ed2c17253e82593cae697
git status --short
(no output)
```
Corrected final source fixture/types verdictPASS. Full hosted quick and fresh panel remain pending; stacked dependency99 must integrate first and final current-main tree revalidate.
