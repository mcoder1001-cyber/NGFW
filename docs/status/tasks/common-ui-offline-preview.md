# Common UI offline preview

Owner explicitly requested a development-only sample-data mode to inspect the
full migrated UI without API access (2026-10-09). This is visual design acceptance,
not a working gateway or backend integration acceptance. Existing production/API
behavior stays unchanged. Historical no-production-mocks requirement is preserved.

Branch: codex/common-ui-migration-20261008 (reuse consolidated branch).
Root owns entry/build guards, preview transport/App, tests and documentation.
Grid worker owns only preview fixtures in its existing isolated branch.

Enable explicitly: NGFW_UI_PREVIEW=1 NGFW_WEB_PORT=5190 pnpm --filter @ngfw/web dev.
Normal dev server does not enable preview. Production builds force flag false even
if the environment variable is present, and artifact guard checks fixture/preview
markers do not ship. Preview uses a separate browser origin/port to isolate browser
settings and pending-commit storage. Transport intercepts API requests, never forwards
them, blocks mutation/device action routes, and reports unsupported reads as missing
sample data. A separate memory-only Session uses no cross-tab auth channel; WebSocket
client makes no live connection. Banner labels every page as sample data.

Implemented:30explicit state route fixtures and schema-valid14domain configuration.
Verified: types/lint;3transport isolation tests plus7existing session tests pass.
Production build with NGFW_UI_PREVIEW=1 excludes preview markers/chunks and passes
unchanged588.5kB/600kB bundle budget. Browser verifies real Interfaces3rows and VRFs
2rows, persistent sample banner and complete navigation. Preview URL5190/interfaces;
Vite session67136. Unsupported routes show missing-sample notices; no live counters.
Final verification:4/4transport tests pass, including proof no API or external
action request reaches nativefetch. Independent review approves transport/session/
stream and production exclusion. Flag additionally requires Vite command=serve,
so even a development-mode build cannot package the preview. Preview remains dev
server only. TypeScript passes after test update. Local checkpoint pending below.
