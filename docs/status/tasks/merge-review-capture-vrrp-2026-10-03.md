# Capture and VRRP merge-readiness review — 2026-10-03

Baseline inspected: `origin/main` at `29f9cae41` (PR #130). Reviewed complete working-tree scope relative to this baseline, including required untracked implementation/regression files and startup wiring. **No actionable merge blocker found in the scoped Capture/VRRP changes.** This report does not claim that the rebase, merge, appliance deployment or all parent feature rows are complete.

Reviewer provenance: Capture was independently reviewed by this reviewer. This reviewer previously authored the VRRP product fixes; the VRRP portion here is a readiness/regression re-review, not a substitute for another author's independent review.

Capture: checked stop ownership/202 asynchronous acknowledgment, late-stream cleanup, failed VPP stop and filter-restore retries, boot-bound globals restoration, terminal metadata-save failures, already-moved file recovery, retention exclusions, administrative route/audit behavior and UI polling/confirmation. Startup wiring passes the resolved globals role and shared boot store. Prior scoped approval remains applicable. Existing temporary-file permissions, buffered download and real capture/screenshots follow-ups remain separate; they are not inferred complete by this merge review.

VRRP: checked active owner/Master/accept-only VIP filtering and startup registration through the actual descriptor registry, fail-closed dump errors, metadata address-order restoration only on equal live membership, current claim-first adoption compatibility, vanished-interface claim/applied-record cleanup, documented native pool residue, and read-only stopped-keepalived checks before daemon writes. Shared-host engine gates remain explicit. Signed-license real HTTP evidence already proves Master and disabled states produce empty full drift; isolated scheduler acceptance proves enabled=false succeeds. F3 daemon-start privileges remain a separate owner decision; the prompt's non-privileged implementation is complete.

Fresh tests against this final workspace, through the heavy semaphore:

```text
tools/heavy.sh go -C apps/agent test -race -count=1 ./internal/actions/capture-trace/ ./internal/descriptors/vrrp/ ./internal/renderers/keepalived/
ok ngfw/agent/internal/actions/capture-trace 13.686s
ok ngfw/agent/internal/descriptors/vrrp 1.302s
ok ngfw/agent/internal/renderers/keepalived 3.263s
```

Scoped diff whitespace checks passed. No shared VPP write/restart, other-worktree edit or product/plan edit was performed for this review.

Snapshot/rebase requirements: include currently untracked `virtual_addresses.go`, `core/vrrp_vip_test.go`, `subsystems/vrrp_product_test.go`, `vrrp/product_fixes_integration_test.go` and API `capture-stream.test.ts`, together with existing desired/core/subsystem tests and generated API contracts. Revalidate affected compilation/tests after resolving any rebase conflict; this pre-rebase approval cannot certify edits made during conflict resolution. The wider origin/main-to-integration delta and unrelated feature rows require their own checks.

## Integration conflict-resolution audit (in progress)

Read-only inspection of `.scratch/merge-integration` found intermediate merge blockers, delivered directly to the integrating coder. This temporary tree is not yet approved:

- VRRP registration referenced missing gate functions/constants/atomic state and undeclared `vppEngine`; old unconditional registration remained around the new gated block, duplicating descriptors and bypassing disabled-engine semantics.
- Keepalived stage called missing `RenderWith`, `CheckOptions` and `Check` renderer foundations; stage constructor still took the old mapper argument while imported tests used the new value-local mapping constructor.
- Core import resolution duplicated `errors`; retain the new idempotent address wrapper and promoted VIP-classification setter while fixing imports.
- The intermediate VRRP Create still used old claim-after-add behavior, missing the tested claim-first/adopt foundation. Restore that targeted hunk and matching tests rather than discarding unrelated latest-main behavior.

Routing-only tunnel metadata staging, Retrieve scope expansion, stored-tunnel fallback and capture boot/foreign-kept recovery were present in the examined intermediate files. Findings describe a moving unmerged tree; the coder must resolve and compile/test before these blocks can be closed. No file in the integration tree was modified by this reviewer.

## Conflict-resolution source re-review

The integrating coder restored the reviewed foundations. Read-only reinspection closes the intermediate source findings: exactly one gated registration of VRRP/meta/keepalived remains; gate helpers and `VrrpEnv` projection are present; the value-local mapping constructor and renderer Check/RenderWith methods agree; core has one errors import and retains the idempotent address wrapper; targeted claim-first/adopt logic is restored.

Latest-main `guardRegistry.Get` forwarding and tolerant VRRP descriptor wrapping remain present, allowing the promoted core VIP setter to be reached while retaining descriptor ownership checks. Capture recovery and routing-only tunnel context remain intact. No additional scoped source blocker was found in this corrected tree. Final integration compilation/tests are still required and owned by the integrating coder; this paragraph closes source-review findings without inventing a test result.
