# CI build DAG recovery

Branch: task/ci-build-dag-20261002. Base: main 2312bd4a. Owned files: turbo.json, packages/api-client/package.json, apps/api/package.json, docs/contributing.md and task reports. No feature files or board edits.

Observed failure: main hosted run 37013687601 reported YANG declaration TS2306. Independent diagnostic reproduced 9 transient empty declaration reads during three actual YANG builds. API-client generation launched dependency compiles outside Turbo while readers/builders were scheduled concurrently.

Fix: generation waits for dependency builds; API-client emits OpenAPI from the already built API, with no nested compiler. API standalone openapi remains available. No gates, assertions or concurrency are weakened.

Actual validation:

```text
pnpm turbo run lint typecheck test build --dry=json
API-client gen -> API build; YANG gen -> schema build; proto gen -> schema build. Graph resolved without cycles.
pnpm gen
Tasks: 13 successful, 13 total; Cached: 0; Time: 37.421s. Generated working tree unchanged afterward.
pnpm turbo run lint typecheck test build --force
Tasks: 30 successful, 35 total; failed @ngfw/api#test.
API: 10 files failed, 54 passed; 8 tests failed, 299 passed, 63 skipped.
Actual local failure: listen EPERM on Unix-domain test sockets. No full local quick PASS is claimed.
Schema: 1577 passed; proto: 104 passed; YANG: 11 passed; UI kit: 90 passed.
```

Hosted full quick runs 37015698798 and 37015679546 started on remote 02c75a212b3cc0297cde3d3125593b273dc4ea18; not yet complete. Independent final review pending. Lab irrelevant to this build failure.

Next: finish generation, verify unchanged generated outputs, run the unchanged full gate, independent review, publish PR and merge only after hosted green on current main integration tree.
