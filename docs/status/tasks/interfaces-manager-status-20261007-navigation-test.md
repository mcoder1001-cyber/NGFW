# Independent App navigation timeout investigation

Verdict: **PASS isolated unchanged App suite; original occurrence FLAKY (not reproduced)**. This is an independent targeted investigation, not a substitute for the manager's required successful whole quick gate on the final integration tree.

Tested source: `9d1a0291d7e3d342a5b56047e68e039bec82845b`; actual main comparison: `e74c33ebd2c083ef45d733b8d4494a78eb67edd9`. Tester branch: `codex/interfaces-integration-test-20261007`; own worktree reused at `/root/ngfw-wt/wizard-test-20261007`. Previous tester branch/checkpoint preserved. No product source, test code, assertions or deadlines changed.

## Actual failed whole-suite evidence

Read `/root/ngfw-wt/logs/ci/interfaces-integration-20261007-20261007-110312-2878605/09-turbo.log`. It reports exactly one failing web case; all other 626 web tests and 34 Turbo tasks passed. Actual excerpts:

```text
× App frame > marks exactly one navigation item as the current page on nested domain paths (review L4) 8135ms
Unable to find role="heading" and name "Tunnels"
<body style=""><div /></body>
✓ App frame > opens the group of a deep-linked page, marks its entry current, and lets the user close it (D-117) 14989ms
Test Files  1 failed | 106 passed (107)
Tests  1 failed | 626 passed (627)
Tasks: 34 successful, 35 total
Failed: @ngfw/web#test
```

Failing assertion: `apps/web/src/App.test.tsx:104`. Unchanged shared test setup configures `asyncUtilTimeout: 8000`; router loads `/vpn/tunnels` with a lazy module and does not define a hydration fallback. The next same-route App case passed in that failed suite. These facts support a lazy-route initialization/scheduling delay as an **inference**, not proof of a measured exact cause.

## Unchanged source proof

`git diff --quiet origin/main HEAD -- apps/web/src/App.test.tsx apps/web/src/App.tsx apps/web/src/router.tsx apps/web/src/test-setup.ts apps/web/vite.config.ts apps/web/src/domains/vpn/tunnels` exited 0. Exact blob comparison:

| Path | Main blob | Same in tested source |
|---|---|---|
| apps/web/src/App.test.tsx | da79d9fc77c17c2675293c1066b76ad5b8f0db77 | True |
| apps/web/src/App.tsx | 17a43bd96be250f59999ff6be6b639d2540a78e7 | True |
| apps/web/src/router.tsx | 492aca8b55eae9aed31b2bef86c6b99c18176c54 | True |
| apps/web/src/test-setup.ts | 1aac52c2352e9da0fb6ec839e706c565b0a02ec0 | True |
| apps/web/vite.config.ts | b64e6a8b6ae7abadcd8c84025510bd6c013466ca | True |
| apps/web/src/domains/vpn/tunnels/TunnelsPage.tsx | 15ec8c21a14dd06b7cc57fbad6c94fb8af76cccf | True |

New interface discovery product files do not alter the failing test, app/router, tunnels domain or shared 8-second wait deadline. Generated API-client changes are TypeScript declarations; API-client dependency build completed successfully before the independent run.

## Independent isolated execution

Exact command: `CI=1 TMPDIR=/wzt GOMAXPROCS=4 pnpm --filter @ngfw/web exec vitest run src/App.test.tsx` (exit 0). Full unchanged 12-case file ran, with no case exclusion or deadline alteration. Actual output:

```text

 RUN  v3.2.7 /root/ngfw-wt/wizard-test-20261007/apps/web

stdout | src/App.test.tsx
🌐 i18next is made possible by our own product, Locize — consider powering your project with managed localization (AI, CDN, integrations): https://locize.com 💙

stderr | src/App.test.tsx > App frame > renders the shell with navigation groups built from the schema root keys
No `HydrateFallback` element provided to render during initial hydration

 ✓ src/App.test.tsx (12 tests) 147936ms
   ✓ App frame > renders the shell with navigation groups built from the schema root keys  8199ms
   ✓ App frame > shows "not yet available" for unbuilt domain screens, with the schema title and no data  3824ms
   ✓ App frame > switches to Persian: RTL direction, lang attribute and translated labels  3071ms
   ✓ App frame > code-splits the developer routes and renders the SchemaForm demo  21641ms
   ✓ App frame > has no /dev routes and no Developer nav group when dev routes are off (production builds, review M1)  5589ms
   ✓ App frame > marks exactly one navigation item as the current page on nested domain paths (review L4)  12825ms
   ✓ App frame > opens the group of a deep-linked page, marks its entry current, and lets the user close it (D-117)  14522ms
   ✓ App frame > opens the group of a page reached by in-app navigation, and reopens it after the user closed it (D-117)  19284ms
   ✓ App frame > phone drawer: unique ids per list and the same expanded groups as the side drawer  22559ms
   ✓ App frame > keeps groups collapsed until their header is clicked, and toggles them closed again  29156ms
   ✓ App frame > placeholder titles follow a language switch without navigating (review L3)  5863ms
   ✓ App frame > renders a not-found page for unknown paths  1398ms

 Test Files  1 passed (1)
      Tests  12 passed (12)
   Start at  11:19:35
   Duration  212.86s (transform 22.59s, setup 5.08s, collect 47.35s, tests 147.94s, environment 9.97s, prepare 910ms)

```

Previously failing navigation case passed (whole case 12.825 s). The whole App file passed all 12 cases (test time 147.936 s; total process 212.86 s). These elapsed case durations include additional assertions/interactions and do not measure only the heading wait.

## Independent remote CI verification

Read-only `gh run view <id> --json status,conclusion,headSha,url` results:

```json
{"conclusion":"success","headSha":"9d1a0291d7e3d342a5b56047e68e039bec82845b","status":"completed","url":"https://github.com/mcoder1001-cyber/NGFW/actions/runs/37611599559"}
{"conclusion":"success","headSha":"e74c33ebd2c083ef45d733b8d4494a78eb67edd9","status":"completed","url":"https://github.com/mcoder1001-cyber/NGFW/actions/runs/37611310127"}
```

Manager separately reported an isolated unchanged App run on the exact final source (12/12 PASS, 142.26 s), earlier complete product preflight (35/35 tasks; API 724/web 627 tests PASS), and bare actual-main quick PASS. I independently verified the two hosted results above and my own isolated result; I do not claim to have rerun the manager's earlier preflight or final whole-gate retry.

## Current environment observation

The first attempt to create an output file under `/tmp` failed before Vitest started (`No space left on device`). Actual inode inventory: `/tmp` 1,048,576 used / 1,048,576 total, 0 free; root-backed `/wzt` had 4,434,746 free inodes. Retried without deleting other workers' files using `/wzt` for both temporary data and logs. This current inode exhaustion is real environment evidence; it does not establish the cause of the earlier heading timeout.

Conclusion: the original failure is preserved, but did not reproduce in the independent unchanged App file, the manager's isolated check, or exact-final hosted complete quick. There is no actionable product regression found in the touched interface discovery code from this investigation. Classify the single baseline lazy-route timing occurrence **FLAKY**, with manager-owned tech-debt tracking, and require the final unchanged whole local gate to be green before merge. No real lab/API configuration or host service changed; no live acceptance was performed.
