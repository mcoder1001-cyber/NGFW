# Notifications resume — R3 contracts/API review

Reviewed local product `97f6863056c630069f48750104ca8d4d8392fe7a`, tree `03d0466129e90c91c00d7ac1cec7f7ed088a5793`, against main `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`. Manager identifies corresponding published product as `426b16030202aaff4630cb254d03c77c3c576213` (PR 66); this reviewer used the local frozen product. Independent worktree `/workspace/scratch/de92de7d9874/NGFW-r3-review`, branch `review/notifications-r3-20261002`. No product changes.

## Findings

- **MINOR**, `apps/api/src/features/notifications/notifications.controller.ts:39`: `SafeParamPipe('name')` returns 400 problem+json for invalid path text, but `@Protected(404, 409, 503)` omits 400 from OpenAPI and generated client responses. Add 400 and regenerate the client. This does not break runtime error handling or existing clients. Manager has requested the small correction in the developer's next checkpoint; it is not a merge-blocking contract reshape.

No BLOCKER or MAJOR in the reviewed R3 scope.

## Contract assessment

- The `contract(notifications)` commit and `F-notifications-contract.md` are present. Relative to the reviewed main, notifications is wholly additive; existing proto numbers and names remain intact. `ManagementConfig.notifications = 7` is new, `NotificationChannel` reserves 4 and `telegram` permanently. Owner removal of Telegram takes precedence over the historical feature prompt.
- Optional `management.notifications` preserves existing stored documents and old-client requests. Empty channel/rule arrays and channel/rule defaults are deterministic. Rule seconds become milliseconds only at the dispatcher scheduling boundary. Schema bounds, unique names and rule-to-channel references are checked.
- SMTP `passwordRef` and webhook `secretRef` use existing typed secret-store references. They are deliberately visible references, not write-only secret values; GET/edit round trips must preserve them. Existing secret discovery/existence validation reaches both nested leaves. Actual secret ciphertext remains in the existing secret table and is decrypted only for delivery. No new secret storage contract or migration is introduced.
- API config remains in the transactional candidate/running document. API-owned notifications is omitted through the shared desired-state projector, now also used by drift; projection does not mutate the stored config. No agent implementation is implied by the new protobuf message.
- New state and action routes follow state/actions separation, use the shared problem responses, and have generated client entries. State returns a bounded history (500), not an unbounded database list. No existing route, event code or response field was renamed.
- Reviewed matching schema, protobuf, generated Go/TS, API DTOs, generated client, UI consumers and YANG output. The notification config's generated refinements are enforced by the root Zod write path, as for sibling config domains.

## Actual scoped verification

All commands below ran in the isolated worktree (or the noted package subdirectory). Dependencies were reused read-only from the developer worktree; all generated output was written into this isolated worktree. Pinned protobuf binaries came from `/workspace/scratch/96b8b6fbc8a7/toolchain/bin`; `XDG_CACHE_HOME=/tmp/ngfw-r3-cache`.

1. In `packages/proto`: `buf generate && buf generate --template buf.gen.model.yaml`; exit 0, no output. `git status --porcelain -- ../../apps/agent/gen gen` produced no output. An initial attempt lacked the package-local plugin symlink and failed before generation; correcting reviewer dependency setup resolved it.
2. In `packages/schema`: `./node_modules/.bin/tsc -p tsconfig.json && node dist/gen.js && ./node_modules/.bin/vitest run src/domains/ext/notifications.test.ts`:

```text
schema: wrote root + 14 domain schemas + openapi components
Test Files  1 passed (1)
     Tests  5 passed (5)
Duration 306ms
```

3. Root: `./apps/api/node_modules/.bin/tsc -p apps/api/tsconfig.build.json && node apps/api/dist/openapi.js packages/api-client/openapi.json && ./packages/api-client/node_modules/.bin/openapi-typescript packages/api-client/openapi.json -o packages/api-client/src/generated/schema.d.ts && ./node_modules/.bin/prettier --write packages/api-client/src/generated/schema.d.ts`:

```text
api: OpenAPI written to packages/api-client/openapi.json
openapi-typescript 7.13.0
packages/api-client/openapi.json → packages/api-client/src/generated/schema.d.ts [693.1ms]
packages/api-client/src/generated/schema.d.ts 2397ms
```

4. Root: `./packages/yang/node_modules/.bin/tsc -p packages/yang/tsconfig.json && node packages/yang/dist/gen.js`:

```text
yang: wrote 14 modules to generated/
```

5. Final `git diff --exit-code -- apps/agent/gen packages/proto/gen packages/schema/dist packages/api-client/src/generated packages/yang/generated`: exit 0, no output. Generated tracked outputs are unchanged.
6. Two `node --input-type=module` assertion probes against the newly compiled schema/API checked old-document omission, defaults, unchanged reference redaction, rejection of duplicate names/missing channels/raw secrets, secretRefs/missingSecretIssues and immutable agent projection. Actual output:

```text
R3 contract probe PASS: legacy omission, defaults/units, reference-preserving redaction, duplicate and missing-channel rejection, raw-secret rejection
R3 API probe PASS: both refs discovered, missing refs reject with pointers, redaction keeps refs, agent projection omits notifications without mutating stored doc
```

The initial workspace `pnpm --filter @ngfw/schema gen` attempt hit the environment's pnpm auto-install SQLite store-path failure; direct package generator commands above succeeded. This is scoped generation/contract evidence, not a successful full `pnpm gen` scheduler run or quick gate. No duplicate broad test run, hosted-CI claim, live SMTP/HTTPS reception, database restart, management VRF or lab acceptance is claimed. Product/acceptance gaps documented by the developer remain outside this R3 approval; full mandatory hosted quick and the other applicable reviewers remain merge requirements.

**Verdict: APPROVE** for contracts/API of the frozen bounded slice, with the optional MINOR above. Recheck any later contract delta before integration.
