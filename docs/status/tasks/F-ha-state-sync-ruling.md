# F-ha-state-sync third-round A1 ruling

Date: 2026-10-05 UTC. Fresh independent A1; no authorship, source edits, test edits or runtime execution. Case: round limit (ARBITER-PROMPT type 6), accepted R3 API finding, not a disputed flake. Parties: lead manager/source author; independent R3/T2 reviewer; fresh correction developer to be assigned by manager.

## Pins and ownership

Own branch: codex/arbiter-ha-api-round3-20261005; worktree: /dev/shm/ngfw-arbiter-ha-api-20261005. Own files only: this ruling, F-ha-state-sync-dispute.md, and append to docs/decisions/ARBITRATION-LOG.md. Parent: 492c0156e38ee73acca235527d7979b447067bb8; parent tree: e9d811f0639b375b37d95995a01332693fa44c39.
Source: 48e5af12fd6a42323f3c83d04129eb6563cef650; tree: c42eb2959321f7a7bd135b9312b595c37f0baef8. Published R3/T2 evidence: codex/review-r3-api-contracts-20261005 at 180e61da0814d1e024c419412eaf7abf57b9dfdf; tree: 5da29ed6b2d65cbf60c1240e734556798bfd654d.

## Evidence inspected and actual arbiter checks

Read AGENTS.md, prompts/00-CONTEXT.md, prompts/ARBITER-PROMPT.md, docs/contributing.md, decision-policy.md and manager dispute. `git fetch origin codex/review-r3-api-contracts-20261005` succeeded; `git show --stat origin/codex/review-r3-api-contracts-20261005` returned exact 180e61. `git rev-parse` returned the pins above. Inspected reports F-ha-state-sync-review-R3.md and F-ha-state-sync-test-T2.md and ha/ha.test.ts, ha/restart.test.ts, ha-denial-first.txt, ha-denial-repro.txt and ha-t2-restart.txt using git show at that ref. These are inspected reviewer executions, not tests run by this arbiter.

`git show 48e5:apps/api/src/agent/agent.client.ts` (full SHA used) confirms agentProblem omits PERMISSION_DENIED and defaults to 502 with grpcCode. Source rpc_ha_sync.go lines 93–94 returns PermissionDenied before execution when !GlobalsOwner. Source HA controller uses admin guard, documents 403/409/502/503, sets audit resource before dispatch, and rejects missing/failed completion. The refusal is intentional; transport classification is wrong.

First actual real API/private PostgreSQL18/private Valkey wave: 19 passed, 1 failed, expected403/received502, 41.94s. Fresh focused replay: 1 failed, 19 skipped, same mismatch, 28.38s. Fake agent reproduces the real agent gRPC refusal through actual AgentClient; these do not prove native dataplane execution. Readonly/operator 403 and unavailable503 passed; successful injected completion/audit and transactional regressions passed. Supplemental actual distinct-principal concurrent writes passed.

Restart supplemental log: 1 failed/1 passed, 415 instead of200 before reaching restart. Published corrected fixture sends object JSON to /config/system but was not rerun. API restart is UNVERIFIED, not a proven source failure or PASS. Earlier readonly spelling and TCP preflight failures were fixture errors. No extra arbiter rerun or heavy build is warranted for this manager-accepted finding. Reported complete source quick PASS (32m02) does not override R3 BLOCK/T2 FAIL.

## Ruling: REASSIGN; R3-HA-01 upheld

Prefer reassignment over conditional FINISH by the same source author: R1 probe correction and R2 redirect correction already consumed two rounds, and a third real API defect remains. Lead manager assigns one fresh developer a bounded correction on a new named branch/isolated worktree from the preserved source; root does not enter a fourth self-fix loop. This is continuation of the existing HA task, no new WBS item and no board edit by the arbiter. Do not merge the current failing source.

Correction scope: translate deliberate gRPC PERMISSION_DENIED to HTTP403 with a distinct permission problem. Prefer the existing generic API helper mapping after checking its other consumers; HA-specific translation is permissible only if justified by existing policy. Add meaningful regression through actual AgentClient action transport and real API replay, not only direct controller mocks. No schema/proto reshape, privilege gain, socket permission change or new authorization boundary. Preserve FAILED_PRECONDITION/ABORTED409, UNAVAILABLE503, other existing mappings, application/problem+json, admin-only dispatch, withheld error/completion semantics, and audit failure/success semantics without secret leakage. Failed/ended streams must not produce successful completion or success audit.

Developer publishes coherent correction and exact test evidence immediately. Independent affected R1/R2/R3 reviewers confirm closure and no regressions in their applicable areas; T2 replays denied admin, readonly/operator pre-dispatch denial,409,503, failure/withheld completion and audit behavior on the corrected SHA. T2 must report corrected restart fixture execution honestly, keeping restart UNVERIFIED until run; actual lab-only acceptance may be separately deferred under AGENTS, never relabeled PASS. Preserve existing honest unsupported/counter/uint64 observations.

Manager preserves reviewed history in archive/remote checkpoints, publishes narrow reviewable PR, verifies the current-main integration tree, and requires unchanged complete quick plus hosted green gate and applicable independent approvals before sequential merge (D112). No waiver from prior source quick or arbitration. Manager alone updates task/envelope/board and live roles; supplied inventory is four workers, not independently observed by this arbiter. Release this arbiter slot before assigning the fresh developer if needed.

Decision policy always-PENDING #4 excludes implementing existing specified boundaries; this restores an existing error response and does not alter the boundary. No routine owner permission or PENDING file is required. Escalate only if actual implementation expands into a listed boundary/contract change. This case-specific ruling creates no general D-row or tech-debt waiver.

## Durable recovery

Publication is via authorized GitHub connector if CLI403 persists. The commit and publication receipt must pin parent/tree and actual remote SHA; never infer a push from a local commit. Remaining product correction is explicitly unfinished. Exact next manager action: provision fresh API developer envelope for R3-HA-01 from source48e5 and this ruling, then independent closure and current-main gates. No product tests were run by A1; documentation check: git diff --check.
