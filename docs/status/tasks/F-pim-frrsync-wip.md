# F-pim-frrsync checkpoint

Branch task/F-pim-frrsync-20261004; local code SHA: 4c006935; remote checkpoint SHA: 95013463923f0d68ff2bf73ed5fd6d2fc8e9afed (manager verified publication).
Owned files and scope: see F-pim-frrsync-envelope.md.

Completed: RP/interface renderer; daemon projection; upstream-shaped bounded JSON adapter; synchronized cache with failure preservation and real empty withdrawal; exclusive named MFIB ownership; production S1 wiring with table-zero ID guard; unit tests and scoped opt-in harness.

Actual tests: focused PIM renderer/source/MFIB race suites PASS; TestPim subsystem ownership/projection race tests PASS. Whole desired race suite PASS (35.051s); focused package vet PASS. tools/ci.sh check --base main PASS (gitleaks absent warning; not aggregate gate). Full subsystems suite fails existing TestPBRPolicyNamesFACLList missing ACL dependency (unrelated to PIM); reproduction on baseline requested from manager. Hosted quick CI, real pimd integration, VPP packet forwarding and restart test not run.

Next command: PATH=/workspace/scratch/e4f791ef53f7/go/bin:$PATH go test -race -count=1 ./internal/renderers/frr/pim ./internal/frrsync/pim ./internal/descriptors/mfib ./internal/desired (apps/agent); independent review and manager remote publication; full quick gate attempted, failed tool installer tar ownership under container (golangci-lint/gitleaks); manager handles environment tool availability.

R5 follow-up: implemented 256-record runtime cap with unchanged 10k parser guard; tested boundary acceptance and 257 rejection/cache preservation. Scoped source/MFIB/renderer race suites and source vet PASS. Accepted scale debt owner Codex manager, due 2026-10-11, in task-specific debt note. Manager publishes this follow-up before final R5 review.

R8 follow-up: normal-level structured warning and existing ERROR event emitted once on failure transition; readable fixed messages and safe attributes contain no raw error payload. Successful read+sync resets/reports recovery, allowing a later outage to notify. Source callbacks receive completed successful outcomes; shutdown cancellation does not trigger an outage. Tests cover repeated failures/no spam, recovery/new failure, payload non-disclosure and source read/scheduler retries. Baseline aggregate quick remains BLOCKED-ENV (Unix socket permissions/tool/ownership environment); this is distinct from passing scoped tests.
