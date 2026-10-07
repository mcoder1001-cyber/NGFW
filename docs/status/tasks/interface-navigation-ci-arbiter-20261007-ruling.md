# A3 independent ruling: App navigation timeout

Date: 2026-10-07. Case: tester failure disputed as flaky. Parties: integration manager and independent integration tester. Question: does the single navigation heading timeout on PR #199 require a product-code correction before merge?

Product examined: `9d1a0291d7e3d342a5b56047e68e039bec82845b`. Comparison main: `e74c33ebd2c083ef45d733b8d4494a78eb67edd9`. This arbiter has not authored, reviewed or tested this product branch before the arbitration; writes here are documentation only.

## Evidence independently inspected

- Original complete-gate log `/root/ngfw-wt/logs/ci/interfaces-integration-20261007-20261007-110312-2878605/09-turbo.log`: the App navigation case failed at `App.test.tsx:104` after 8135 ms, unable to find the Tunnels heading; body contained an empty div. Exactly one of 627 web cases failed; 626 passed. Turbo reported 34 successful of 35 tasks. This failed gate remains failed and cannot serve as merge evidence.
- `git diff --exit-code e74c33ebd2c083ef45d733b8d4494a78eb67edd9 9d1a0291d7e3d342a5b56047e68e039bec82845b -- apps/web/src/App.test.tsx apps/web/src/App.tsx apps/web/src/router.tsx apps/web/src/test-setup.ts apps/web/vite.config.ts apps/web/src/domains/vpn/tunnels` exited 0. Navigation/router/test setup and tunnels product code are byte unchanged. Shared async heading deadline remains 8000 ms. The tunnels route loads lazily.
- Independently read published tester report from `99cce6192591721c015949ff5ccb64d2b033af9e:docs/status/tasks/interfaces-integration-test-20261007-report.md`: unchanged whole App file passed 12/12; original case passed at 12825 ms total case duration (not a measurement of heading wait). The report preserves the original failure and calls lazy initialization delay an inference.
- Independently queried GitHub using `gh run view <id> --json conclusion,headSha,status,url`: final hosted run 37611599559 completed successfully on exact product SHA; main run 37611310127 completed successfully on exact comparison main. Hosted results support nonreproducibility but do not replace mandatory local whole-gate success.
- Current environment check `df -i /tmp /iac`: /tmp has zero free inodes; root-backed /iac has 4,397,381 free. This justifies the separate short TMPDIR environment fix. It does not prove the original timeout cause.

## Own bounded reproduction

Fresh worktree initially lacked compiled workspace dependencies: the first invocation failed to resolve `@ngfw/schema` and collected no tests. It is an arbiter preparation failure, not a reproduction of the heading timeout. Built unchanged schema, API-client and UI-kit package dist outputs using their existing build scripts, all exit 0; no tracked source changed.

Twice sequentially on exact product SHA, without changing assertions, deadlines, workers, test configuration or source:

```sh
CI=1 TMPDIR=/iac GOMAXPROCS=4 pnpm --filter @ngfw/web exec vitest run src/App.test.tsx -t 'marks exactly one navigation item' --reporter=verbose
```

This targets the originally failing case only; the other 11 cases are explicitly not rerun by the arbiter. It is not a complete quick gate.

Rerun 1: exit 0, 1 passed / 11 unselected, failing-case duration 5174 ms; total process 17.01 s. Log: `/root/ngfw-wt/logs/interface-navigation-arbiter-rerun-1-prepared.log`.
Rerun 2: exit 0, 1 passed / 11 unselected, failing-case duration 5102 ms; total process 17.28 s. Log: `/root/ngfw-wt/logs/interface-navigation-arbiter-rerun-2-prepared.log`.

## Rule and ruling

Apply `prompts/ARBITER-PROMPT.md` disputed tester-failure procedure and two own reruns, `docs/contributing.md` mandatory exact-tree whole quick PASS requirement, and owner AGENTS instructions that real code failures and mandatory quick CI must pass before merge. No policy, test deadline, assertion, linter or script may be weakened.

Final ruling: **FLAKY baseline test occurrence, no actionable product regression demonstrated**. Unchanged failing sources, independent full-file success, exact-head hosted complete-gate success, and bounded reproduction support this classification. Lazy-route initialization/scheduling delay remains an inference; there is no proven precise cause. Current inode exhaustion is separately observed environmental trouble, not an explanation established for the original timeout.

Manager must preserve this original failure evidence, record a test-stability follow-up owned by Web test maintainers / integration manager due 2026-10-08, and obtain `CI GATE PASSED` from the full unchanged local quick gate on the exact final integration tree before merge. A further reproducible failure blocks integration and requires investigation/correction; this ruling never waives a mandatory gate. Hosted required checks must remain green and the expected product/base heads must match when merging. No live lab acceptance or service deployment is claimed.

Manager-owned arbitration-log row proposed (this task's ownership excludes shared log edits):

| 2026-10-07 | A3 | interfaces-discovery final integration | tester failure dispute | One unchanged App navigation heading timeout | FLAKY, conditional on final complete unchanged local quick PASS | ARBITER-PROMPT.md disputed test procedure; contributing mandatory gate; owner AGENTS | Web test maintainers / integration manager: test stability investigation due 2026-10-08; retain failure log |
