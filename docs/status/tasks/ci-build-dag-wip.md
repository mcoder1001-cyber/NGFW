# CI build DAG recovery

Branch: task/ci-build-dag-20261002. Base: main 2312bd4a. Owned files: turbo.json, packages/api-client/package.json, apps/api/package.json, tools/test-build-dag.mjs and this report. No feature files or board edits.

Observed failure: main hosted run 37013687601 reported YANG declaration TS2306. Independent diagnostic reproduced 9 transient empty declaration reads during three actual YANG builds. API-client generation launched dependency compiles outside Turbo while readers/builders were scheduled concurrently.

Fix: generation waits for dependency builds; API-client emits OpenAPI from the already built API, with no nested compiler. API standalone openapi remains available. No gates, assertions or concurrency are weakened.

Actual validation: dry-run graph resolves without cycles and API-client gen depends on API build; YANG gen and proto gen depend on schema build. Full generation currently running; hosted full quick not yet run. Independent review not yet complete. Lab irrelevant to this build failure.

Next: finish generation, verify unchanged generated outputs, run the unchanged full gate, independent review, publish PR and merge only after hosted green on current main integration tree.
