# Independent RA correctness/contracts/dataplane/packaging review

Reviewer branch `codex/review-ra-correctness-20261007`, worktree `/root/ngfw-wt/resume-p12-20261007`. Owned file: this report only. Reviewer did not author RA. P12 developer branch remains unchanged at `ff3c0c3f3f84df32ba0d6421f86bb5888b361d4a`.

Reviewed source: `9f2b3043399fb6f95242ed94c20effce8f6e2c27`, tree `c41d6661081cca876ea537ba7438e0139ccdbd7a`, against current fetched main `0ec397e327123cadfd5d278a9a1cda37532fdc2c` (main is an ancestor). This is the preliminary integration review, NOT approval of the future freeze that the manager will supply. No RA product edits, live guest/namespace/service activation or heavy/full gate by this reviewer.

Read REVIEW-PROMPT and R1/R3/R4/R8, approved independent engine decision, capability readiness, mount/typed-FD handoff, fixed IPC root and bounded PID1 readback decisions. The approved independent Linux-XFRM engine supersedes the historical kernel-vpp RA assumption; no obsolete VPP crypto-SA requirement is imposed. Owner permission to defer actual lab-only acceptance does not turn an unresolved production default READY failure into a lab prerequisite.

## Grades

| Aspect | Verdict on reviewed checkpoint | Limits |
| --- | --- | --- |
| R1 correctness/tests | **BLOCK** | Bounded source controls passed; actual canonical default initialization still fails, with no demonstrated environmental prerequisite explaining it. B1 must be resolved or precisely classified with evidence. |
| R3 contracts/API | **APPROVE, source scope** | Existing protobuf fields/numbers retained; additive transport/policy/RPCs have prior contract commits; owner-first/error/page/uint64 controls and semantic validation inspected. Future freeze, generation equality and actual licensed/PKI/secret production HTTP acceptance remain unapproved. |
| R4 dataplane/ownership | **APPROVE, source scope** | Failed-stop transport barriers and disjoint ownership controls passed; no generated VPP binding, original agent unit or CI weakening in integration delta. Physical TAP/FIB, encrypted policy, namespace isolation and real rollback remain unexecuted here. This does not close B1 or claim actual native acceptance. |
| R8 packaging/operability | **BLOCK** | Offline artifact/ABI/package controls passed; original-unit readiness remains unresolved (B1), inherited mandatory history scanner is independently red (B2). Actual appliance package/upgrade/isolation proof and current hosted quick remain outstanding. |

Combined: **BLOCK for merge at this checkpoint**. No newly demonstrated source logic defect beyond the unresolved integrated readiness outcome is asserted. Source primitives are reviewable; an unconditional source-ready/final-feature verdict is not supported yet.

## Ranked blockers

**B1 — BLOCKER, R1/R8: canonical installed default supplier has not reached READY within the unchanged production caller budget.** `apps/agent/internal/ra_vpn/namespace_openfile_server.go:229` advances checkpoint16=6 immediately before fresh `proof.Verify(ctx)`; publication-complete is sent only at line239. `apps/agent/internal/subsystems/ra_vpn_controller.go:625` refuses failed supplier initialization and sets initialization ready only at636. The actual outer connect hook uses its original30s at `apps/agent/internal/agent/agent.go:443`, definition487; child40s publication contexts are correctly clipped by that parent, not a promise of40 usable seconds.

Independently read Boot38 `/root/ngfw-source-vm-20261005/boot-38.log`, SHA256 `ea98ea4708bae6e59f49f1e865b444751cd937df6724e0b5cd366ed2a48c21b2`. Safe original excerpts:

```text
publisher-client phase=preflight caller_deadline_exceeded=true ... elapsed_ms=26825
publisher-client phase=publish ... ready_received_ms=19565 ready_proof_done_ms=20352 request_sent_ms=20357
supplier initialization refused ... stage=6 publisher_stage=19 deadline_exceeded=true
initialization unavailable ... reason=engine-not-ready
publisher-server checkpoint16=6 deadline_exceeded=false canceled=true elapsed_ms=10778
```

The second server had activated the target supplier but had not sent publication-complete. The client expired; peer closure canceled the server. This is distinct from Boot36's later post-ACK checkpoint15. It does not prove hashing dominates latency, a protocol/type mismatch, or any specific CPU/disk cause. The producer receipt recorded by the developer pins union711e; comparison `git diff 711e18e12..9f2b30433 --` publisher server/pipeline is empty. Current recovery changes only the two protected-positive readback fixtures in that area; they do not constitute a runtime readiness fix.

Required closure: diagnose the actual canonical default boundary and demonstrate readiness under the original caller/proof/security/unit contracts, or establish a concrete missing/misconfigured installation prerequisite with falsifiable readback and obtain manager classification. No timeout increase, proof removal, injected ready callback, or relabeling failed default initialization as lab-only. Missing engine on the shared developer host legitimately yields unavailable; that separate case does not explain an installed immutable guest that progressed through probe and publish.

**B2 — BLOCKER, R8 integration gate: history scanner remains red.** Independent `gitleaks git --redact --no-banner --config .github/gitleaks.toml --log-opts='origin/main..HEAD'` scanned175 commits/~2.21MB and returned1, `leaks found:4`. Before review commit, actual `tools/ci.sh check --base origin/main` also returned1 at mandatory gitleaks; contract and shell/VPP/FFI/Docker/kill-pattern/secret-shape checks passed first. Its report `/root/ngfw-wt/logs/ci/resume-p12-20261007-20261007-073549-2289841/gitleaks-report.json` independently locates four generic-api-key documentation findings in3cf8e830 `F-ra-vpn-final-review-plan.md:9`,2f4f80515/fd2728dde `F-ra-vpn-api-ui-wip.md:27` and83e9b07a same file:17. Only RuleID/file/line/commit metadata was extracted; no matched content is copied here and no exposed-secret/safe-prose classification is asserted. Mandatory scanner cannot be waived. Manager/R2 must classify findings, retain archive refs and perform authorized D112 consolidation/sanitization if warranted, then run unchanged mandatory exact-head gate. Reviewer changes no allowlist/history/product source.

## Source findings and meaningful controls

Installation proof holds four actual root-owned no-follow single-link files, hashes the complete helper, compares immutable unit/socket pins, verifies creating process/full boot and fresh canonical file stamps at each boundary, and closes on failure. Positive held-proof verification precedes replacement/content/mode/hardlink/symlink negatives. Streaming cancellation refuses canceled input. Engine/ABI readiness and helper unit digest tests passed; shared missing installation stays false. No persistent proof cache or success from callback registration alone.

Publisher cancellation watcher is joined before its socket FD can close/reuse. Peer watcher ignores queued legitimate ACK input, detects HUP/error and cancels bounded work, and has idempotent stop/join. Terminal ownership retires watcher without extending original context. DBus wrapper uses generic no-FD transport, exact property interfaces/variants, total/auth/frame bounds, closes ancillary rights, and authenticates fresh protected PID1/socket/boot readback. Pipeline retains each SDK call until buffered completion and context retirement; role Id brackets remain sequential and fixed role fields are bounded. Reordered, foreign/duplicate reply serial, wrong types, pre/post Id and cancellation negatives are executable public-byte/socket tests, not an assumed SDK guarantee.

Runtime stores partial owned generations; failed Stop prevents credential/store/namespace/TAP cleanup. Scheduler undo continues but transport guards reject mutations until positive inactivity. Tests force the actual scheduler undo path and assert remaining transport, retained generation and degraded result. Restart/unknown-unit/foreign boot barriers are inspected in runtime/controller; normal and RA descriptor families are disjoint. Actual SystemdUnits.Stop requires exact owned unit/typed NSFS/executable snapshot before dispatch and verifies MainPID0, retired original PID and empty/inactive cgroup afterward. These source controls do not establish physical daemon stop.

Canonical fixture explicitly asserts owned physical TAP/FIB removal, preserved foreign TAP/routes/classifiers and namespace disappearance; actual Stop fault fixture keeps charon alive while synthetic fixed dispatch rejects Stop, then removes fault and demands positive stop. Both require owned guest execution and are **NOT RUN** by this reviewer. Logical rollback PASS is not substituted for actual original-agent physical rollback. Supplemental controlled Stop campaign remains separately labeled.

API uses authenticated owner before lookup/mutation, bounded opaque session IDs/page100, shared renderer/runtime session1024, exact decimal uint64 counters, stale cursor conflict, verified disconnect before success and audited admin action. Schema tests reject transport/address/policy/pool conflicts. Generated output equality is reported by developer's normal13-task generation; reviewer did not rerun generation or hand-edit outputs. Hosted gate must reconfirm on future frozen tree.

Packaging pins6.1.0 authenticated source/ABI, exact runtime dependencies and fixed templates/helper receipts; offline host-root/ABI/source/symlink/existing-output negatives passed. Debian install policy uses no-start/no-enable; engine package has no maintainer activation. Byte-sensitive two helpers retain strip exclusions. Native S2S template isolation and unchanged agent hardening are preserved. Full installed-appliance upgrade/remove and actual daemon asset visibility are not inferred from staging fixtures.

## Independent bounded command receipts

```text
GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 90s -run 'Test(NumericPublisher|TargetsOpenFile|ObserverOpenFile|InstallationUnit|DefaultMissingInstalled|LifecycleFailureStop|SchedulerContinuingUndo|LifecycleUnloadFailure|PartialQuiesce|FailedStoreInventory|ReadinessFirstActivation|InitializationGate|UnitStartWithout|ManagerDispatch)' ./internal/ra_vpn
ok ngfw/agent/internal/ra_vpn 2.261s; exit0

GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 90s -run 'Test.*(RemoteAccess|RA.*(Guard|Roll|Stop|Scope|Foreign|Plan)|RAScope|RAController|Readiness)' ./internal/agent ./internal/subsystems
ok agent 1.479s; ok subsystems 1.383s; exit0

GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 45s -run 'Test(RARPCOwnerFirstUnavailableAndMalformedBoundaries|RAVerifiedSnapshotPaginationBeyondTwoHundred|LifecycleSessions)' ./internal/agent ./internal/ra_vpn
ok agent 1.411s; ok ra_vpn 1.466s; exit0

python3 -B -m unittest discover -s deploy/ra-vpn -p 'test_*engine.py' -v
22 tests OK 0.273s; includes imported duplicate fixture cases, not22 distinct checks

pnpm --filter @ngfw/schema exec vitest run src/semantic/ra-vpn.test.ts
10 tests PASS; 1 file; 5.30s
```

Additional exact initializer/reconnect/scoped replay `GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 45s -run 'TestRA(Scoped|Reconnect|Initialization|TargetInitialization|EnvironmentFailed)' ./internal/subsystems`: PASS1.480s/exit0. This closes gaps in the broader name filter above, including initialization order, original caller clipping, failed persistent construction/close and reconnect barriers.

Focused HTTP suite `pnpm --filter @ngfw/api exec vitest run src/features/ra-vpn/ra-vpn.test.ts`: fourteen tests PASS,493ms tests/16.55s total,exit0. Actual local Nest/Fastify HTTP and scripted gRPC observations exercise response validation, auth/admin boundaries, exact counters and errors; this is not real installed RA/PG/PKI/license/credential packet acceptance. Initial API attempts ran **no tests**, failing package resolution first for unbuilt `@ngfw/proto`, then unbuilt `@ngfw/schema`. Standard narrow dependency build scripts completed exit0 and prepared ignored outputs only; these are review checkout prerequisites, not RA product fixes. Failures are retained explicitly, not counted as PASS. Tracked product source remains clean. No developer heavy/full quick repeated.

## Scoped cancellation repair: source-ready

Latest steering asks a separate grade of the current eight-line Verify repair while preserving whole-engine BLOCK. Reviewer fast-forwarded the independent branch to author checkpoint `0a34a8b1269bb23329bc7f0217c07479e07b7c2e`, tree `874ba05e88818d210f0b88ccd99c3e5fbbafc97d`, without writing product code. Delta from9f2b: eight production lines plus65 test lines and developer WIP. The preceding protected-positive fixture changes are included in baseline and independently replayed.

**R1/R4 APPROVE for this exact cancellation delta; source-ready. R3/R8 unchanged source boundaries, no new contract/packaging finding.** Verify now refuses cancellation before each artifact traversal and rechecks context after the final synchronous proc identity read. Existing entry/process/full-boot identity, held descriptor/stamps, canonical fresh opens, checked close, final identity predicate and order are preserved. No timeout, goroutine, detached worker, mutable cache, observer field or unit change. Caller continues to own held descriptors on failure.

Positive/negative replay:

```text
GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 30s -run '^Test(NumericPublisherProofRejectsCancellationAfterSuccessfulSample|NumericPublisherHeldInstallationRejectsChanges|TargetsOpenFileReadbackProtectsOwnedFile|ObserverOpenFileReadbackProtectsOwnedFile)$' ./internal/ra_vpn
ok ngfw/agent/internal/ra_vpn 1.219s; exit0
```

Independent meaningful mutation control used only ignored `.scratch` files and Go overlay, leaving tracked product source unchanged. Remove only the final added context check while retaining all four added per-artifact checks; execute only `TestNumericPublisherProofRejectsCancellationAfterSuccessfulSample/final-identity-read`:

```text
GOMAXPROCS=2 go test -p 2 -race -count=1 -timeout 30s -overlay=/root/ngfw-wt/resume-p12-20261007/.scratch/review-ra-cancel-overlay.json -run '^TestNumericPublisherProofRejectsCancellationAfterSuccessfulSample/final-identity-read$' ./internal/ra_vpn
FAIL final-identity-read: cancellation racing a successful sample was accepted <nil>
FAIL ngfw/agent/internal/ra_vpn 0.146s; expected exit1
```

The test uses a canceled real parent context with a deliberately modeled successful earlier Err sample; no timing sleep or process-read injection. Unmodified positive control verifies proof before negative cases and again afterward, proving failure retained valid owned files. The mutant establishes that the final guard materially prevents a canceled proof success. The repair does not promise preemption of a synchronous kernel file/proc syscall; it refuses success after observing cancellation.

This resolves a source cancellation hole. It neither explains Boot38 cancellation nor establishes defaultREADY: B1 remains **BLOCK**, and B2 remains red. Author diagnostic contract `e851196e99a058e094240477febb5b6aa3c023de` was inspected but not merged/graded as a completed consumer patch; final same-scope diagnostics will receive separate review. No raw identity/path/error diagnostic input is authorized. Final future freeze remains pending.

## Historical independent evidence and remaining acceptance

Preserved reviewer objects read: `6e81d2de3` integrated source coverage (explicitly not whole approval); `a65875734` manager triplet review (previous subprocess query scope); `0ef9fbdba` shared session-bound correction review; current source caller-budget and descriptor-close reports. The old triplet grade does not approve the later native-DBus pipeline automatically; this reviewer separately inspected and executed its transport/pipeline/cancel primitives. Earlier report's session200 finding is corrected to shared1024 and replayed here. None is relabeled actual canonical READY or packet acceptance.

After B1/B2 and future exact-source review: manager-hosted unchanged complete quick; manager-assigned isolated engine first activation/defaultREADY, original-unit mount handoff/asset visibility/isolation, EAP-MSCHAPv2 and valid/revoked/foreign TLS, pools/DNS/split routes, ESP/ACL allow+deny, owned session disconnect, original-agent restart/reconnect<30s, actual VICI pool/connection rollback and physical owned cleanup. Actual HTTP licensing/PKI/sealed credentials/roles/audits/second-pool400 pointer and real backend browser en/fa modes are separate missing acceptance, eligible for explicit lab-only deferral only once source/prerequisites are established. RADIUS remains rendered/golden only, as authorized; no unsolicited live throughput gate is added.

Exact next action: manager supplies future frozen source SHA/tree; reviewer compares its delta to9f2b30433 and rechecks changed scopes. Default READY remains failed/unresolved; no live reexecution until manager assigns the owned guest slot. Report publication cannot be used as final future-freeze approval.
