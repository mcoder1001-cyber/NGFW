# F-backup-restore — independent R3 contract review

Reviewed source: `93e743a2217aeef5bc0b081401bb63c5ead35801`, root `28c439b57` plus independently cherry-picked developer correctness commits `9c1f1f3a9` and `784f6b66b`. Reviewer owns evidence files only in isolated `/root/ngfw-wt/f-backup-contract-review-20261005`, branch `codex/f-backup-contract-review-20261005`. No product edits.

## Findings

1. **MAJOR — binary HTTP contract differs from runtime**, `apps/api/src/features/backup-restore/backup-restore.controller.ts:294` and `:103`. Upload OpenAPI advertises JSON string and no headers, whereas runtime requires raw `application/vnd.ngfw.update` plus `x-ngfw-filename`. Generated `schema.d.ts:20639` confirms `header?: never` and only JSON request media. Download advertises JSON string while returning octet-stream; generated `schema.d.ts:20010` confirms JSON success media. Generated clients/Swagger cannot construct the documented upload and may decode binary backup as JSON. Fix consumes, required filename header schema, download produces, regenerate, and provide a concrete shell binary-upload equivalent because generic CLI Call.Body always marshals JSON. Manager has agreed to fix; closure pending exact source.
2. **MINOR — stale internal read pointer**, `docs/status/tasks/F-backup-restore-ui-contract.md:10` names GET `/config/management/backup`; candidate read is `/config/candidate/management/backup`. Main user documentation is correct. Correct internal text.
3. **MINOR — documented error statuses incomplete**, controller restore declares 400/403/409 although write-ahead audit refusal produces 503. Add 503 to the protected response documentation when touching the route.

## Checked contracts

Schema/protobuf changes are additive: management notifications remains tag7, backup8/templates9; actions upgrade20/support_bundle21; UpgradeOp retains unspecified0 and adds closed values1–5. Existing field names and numbers were not reshaped. New defaults permit historical management documents to load. Public template patch object converts explicitly to validated internal patchJson; arbitrary patch remains subject to ordinary candidate schema validation. Contract commits and F-backup-restore-contract.md exist. Migration0010 adds only a nullable candidate pin column and a durable schedule history table. Generated CLI lists all ten added routes; it supplies JSON operations, not a binary transport.

## Executed evidence

`pnpm install --frozen-lockfile --prefer-offline`: exit0, Done in8.4s.

`NGFW_CI_TASK_CONCURRENCY=2 GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh gen-check`: exit0:

```text
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated packages/yang/generated apps/cli/internal/api/operations_gen.go
gen-check PASSED (1m44s)
```

`GOMAXPROCS=2 GOFLAGS=-p=2 go test ./internal/contracttest -count=1` from apps/agent after generation: exit0:

```text
ok ngfw/agent/internal/contracttest 0.333s
```

An earlier concurrent Go attempt raced the generator deleting its generated directory and exited1; it is superseded by the sequential rerun above. `git status --short` after generation was empty. Complete quick and integration are owned by the manager/other independent testers; this report does not claim them.

Initial verdict: **APPROVE WITH CHANGES** — superseded by focused closure below.


## Focused final closure

Product source manager `9bd8f2949`, independently applied in this worktree as `8c3bc538f2ffb62d03a410d8fff7f92d2ac735b7`. The complete additive product remains the reviewed source above plus this media/header/doc correction; only reviewer evidence differs between branches.

MAJOR1 **CLOSED**: upload consumes only application/vnd.ngfw.update, x-ngfw-filename is required with a pattern matching the runtime parser; backup success produces application/octet-stream. Generated client reflects the mandatory header and both actual media types. User docs provide concrete curl --data-binary and explicitly explain generic CLI JSON transport limits. MINOR2 **CLOSED**: internal candidate read pointer corrected. MINOR3 **accepted optional follow-up**: the manager explicitly retains existing 503 runtime behavior and fail-closed tests, deferring additional response documentation in this scope.

Independent repeated command: `NGFW_CI_TASK_CONCURRENCY=2 GOMAXPROCS=2 GOFLAGS=-p=2 tools/ci.sh gen-check`, exit0:

```text
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated packages/yang/generated apps/cli/internal/api/operations_gen.go
gen-check PASSED (1m39s)
```

An independently executed Python assertion against regenerated packages/api-client/openapi.json checked upload media, required header, valid filename, traversal filename refusal, and download success media. Exit0:

```text
Independent actual OpenAPI media, required filename header, valid filename and traversal refusal: PASS
```

An earlier assertion before generation completed read the old ignored OpenAPI file and failed; after generator completion the assertion above passed. `git status --short` after final generation was empty. No full quick or live integration claimed by this R3 report.

Final verdict: **APPROVE** — zero unresolved BLOCKER/MAJOR findings; one optional documented response-status MINOR remains.
