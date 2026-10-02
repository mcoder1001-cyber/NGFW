# Independent CI build DAG review

Reviewed local SHA `4c288a2e8dac82788298a7328e75f5fe54f540c9`, remote product SHA `02c75a212b3cc0297cde3d3125593b273dc4ea18`, base `2312bd4a6d540ba9ae873dd049e59d375589e905`. Reviewer did not author the change. Scope R1/R2/R3/R7/R8; no product edits.

## Findings and verdicts

- R1 APPROVE code: root `gen` now orders dependency builds first. API-client generation waits for API build and no longer launches nested dependency/API TypeScript writers outside Turbo. Schema and YANG own `gen` compiles precede their own build; consumers wait for those builds. The actual graph is acyclic and closes schema/proto/YANG/API dependency chains. Full hosted quick remains mandatory before merge.
- R2 APPROVE: no dependencies, shell inputs, credentials, permissions, authentication, or runtime security boundaries change. Independent forbidden-pattern and gitleaks gate passed.
- R3 APPROVE: no schema/protobuf/API route or generated wire contract changes. Standalone API `openapi` still compiles before emit; new `openapi:built` consumes built output. Root `pnpm gen` remains the supported orchestrator.
- R7 APPROVE after documentation verification: WIP now records actual generation output, local Unix-domain socket EPERM failures, test counts, and pending hosted runs without asserting a full quick PASS. `docs/contributing.md` documents root/scheduler generation, filtered scheduler invocation, direct-script preconditions, and standalone API OpenAPI compatibility. Previously requested evidence and compatibility clarification are resolved. Hosted green must still be recorded before merge.
- R8 APPROVE: checks, tests, linter rules, dependency pins, concurrency, and generation drift gate remain unchanged. DAG ordering addresses the writer race instead of suppressing checks. No service or lab changes.

No BLOCKER or MAJOR product findings. All reviewed aspects APPROVE. Final merge eligibility still requires the unchanged complete hosted gate on the exact final integration tree and its result recorded durably.

## Independent commands and actual output

```text
pnpm exec turbo run gen --dry=json
@ngfw/api-client#gen -> @ngfw/api#build
@ngfw/api#build -> @ngfw/api#gen, @ngfw/proto#build, @ngfw/schema#build, @ngfw/yang#build
@ngfw/yang#build -> @ngfw/schema#build, @ngfw/yang#gen
@ngfw/yang#gen -> @ngfw/schema#build
@ngfw/proto#build -> @ngfw/proto#gen, @ngfw/schema#build
@ngfw/schema#build -> @ngfw/schema#gen

tools/ci.sh check --base origin/main
no contract files changed in the 1 commit(s) of HEAD since origin/main (2312bd4a)
ok: gitleaks — scanned ~1931 bytes (1.93 KB) in 134ms no leaks found
check PASSED (0m02s)

pnpm gen
Tasks: 13 successful, 13 total
Cached: 6 cached, 13 total
Time: 17.001s

git diff --exit-code -- packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
exit 0, no output

git diff --check
exit 0, no output
```

Reviewer generation overlapped a separately started manager full-build invocation before coordination notice. Generation succeeded without drift, but that overlap is explicitly NOT proof of writer safety across multiple independent Turbo processes. Ordinary CI runs its invocations sequentially. Hosted PR run [37015698798](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37015698798) was observed `in_progress`, not PASS. No full quick or lab PASS is asserted.
