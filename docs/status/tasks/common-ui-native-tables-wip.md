# Common UI native tables — work in progress

Branch: codex/common-ui-migration-20261008. User authorized replacing all NGFW tables with real ReactGeereedMaterial from Common UI, including handwritten composition tables. Existing integration branch reused; no main merge.

Owned root files: packages/common-ui native compatibility/integration; all apps/web table imports; docs. Workers own isolated adapter, composition tests and native selection/filter changes; commits integrated sequentially.

Completed: actual native source closure imported, controlled selection and server sort/paging adapter, composition parser preserving rows/cells/spans, page helper expansion, 56 page files migrated to shared markers. No production preview API fallback. Offline server remains 5190.

Actual verification: package ESM build succeeded; declaration build and web typecheck in progress. Earlier adapter readonly errors fixed by integrated worker checkpoint. Full native integration tests/build still required.

Remaining: finish column filter adapter, validate table interactions and production bundle budget, review, checkpoint/publish. Remote publication previously HTTP403 (authenticated account lacks repository push permission); do not claim published. Main mandatory quick CI still not available due missing tooling/network from earlier task.

Next: run package build/typecheck, UI-kit ServerDataGrid tests and web build; inspect offline preview.

## Verification checkpoint

- Common UI build/declarations and strict typecheck pass; Common UI lint passes after test lint cleanup.
- Web and UI-kit typecheck pass; web lint/logical CSS pass.
- Native grid integration: 9/9 tests, including actual column-filter UI, server page totals, row selection guards, action buttons and keyboard tabIndex.
- Composition contracts: 7/7 tests; focused actual page tests: 32/32 across interfaces, subinterfaces, QoS, load balancer and SNMP.
- Production build: 589.1 KB gzip under unchanged 600 KB limit; preview/dev routes excluded. Final empty-state change requires final-tree rebuild before signoff.
- Independent shell review approved after keyboard correction. Composition review audited all 56 files; requested English empty-state suppression is implemented.
- Offline browser verified real native interfaces and tunnel tables; local server HTTP200. Original preview tab became unresponsive to browser control; fresh tab9 works.
- Full web suite still running; initial timeout failures under concurrent compilation are NOT accepted as passing. Fresh isolated retries pending.

## Final local result

Product checkpoint: 02a10082 plus the subsequent strict test-reference correction. All visible page tables render through actual ReactGeereedMaterial; eight pointer-cursor rules now target native row metadata. All changes remain on the existing integration branch.

Verification: final production build and preview exclusion check PASS, unchanged initial bundle budget PASS; Common UI build/typecheck/lint PASS; web typecheck/lint PASS; UI-kit typecheck PASS. Native grid integration9/9 and composition7/7 pass. Focused page tests32/32 pass.

Full web run:643 tests,634 passed and9 failed across6 files. Fresh six-file run54/56 passed; remaining2 navigation failures exposed an initial collapsed-group race. Seeded initial expansion from current route without changing test assertions. Final App/Bonds run20/20 passed, covering both remaining failures; all643 cases have now passed across the full run and corrective reruns. Full run was not green in a single execution. Policy gate tools/ci.sh check --base origin/main PASS (212 tasks).

Independent reviews: native grid reviewed and approved after keyboard correction; composition reviewed across all56 pages and empty-state correction implemented. Navigation correction independently re-review requested but worker reached its usage limit; existing20 navigation/bond tests passed. Hosted quick CI remains unavailable as recorded in the parent migration status; no main merge or deployment performed.

Remote publication: push attempted, HTTP403 denied to authenticated hightwig account. No remote checkpoint SHA exists for this work; local commits retained and remote publication is blocked. API/device functional acceptance remains unavailable; development-only offline transport blocks writes and device actions.

Preview: http://127.0.0.1:5190/interfaces, browser tab9 verified actual native component and saved /private/tmp/ngfw-common-ui-native-tables.png. Server HTTP200. Exact next command if live verification requested: connect the authorized device API, start normal web dev server without NGFW_UI_PREVIEW, and run live acceptance; do not treat sample data as live state.
