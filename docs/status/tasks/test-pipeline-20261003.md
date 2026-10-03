# Nonblocking validation pipeline — 2026-10-03

Owner requested parallel testing and transferring test waits from developers to the coordinator.

## Active host arrangement

Existing heavy.sh allowed three jobs from one pool. A long lab job could repeatedly take a freed slot ahead of short unit/type checks. Created the previously absent /run/lock/ngfw-heavy-max with value2: heavy.sh admits long jobs to slots1/2; tools/test-fast.sh admits short host-independent tests to the SAME slot3 flock. Total remains3. Existing holders drain normally; no process was killed or existing lock stolen. Emergency cap1 disables fast admission too. Resource admission requires >=8GiB available and load<20. Fast executions have a default5-minute timeout; queue wait is bounded10 minutes.

The reservation is intentionally host-wide. Other worktrees using existing heavy.sh honor max2 automatically. Do not launch jobs outside wrappers or expand the global cap. To retire this arrangement, stop submitting fast jobs, wait until slot3 is unheld, verify this coordinator's originally-created cap is still2, then remove /run/lock/ngfw-heavy-max. Preserve any later emergency cap or another manager's setting. No 24-hour AI supervisor is claimed.

## Handoff

Developer commits a coherent checkpoint, leaves that worktree clean/frozen, and continues in a separate worktree. Coordinator submits an argv command (no shell interpolation) and exact SHA:

```bash
python3 tools/test-handoff.py submit --cwd /absolute/frozen/worktree --head <sha> --lane fast -- go -C apps/agent test -race -count=1 ./internal/descriptors/lcp
python3 tools/test-handoff.py status /root/.cache/ngfw-test-jobs/<job>.json
```

Use lane heavy for long generation/build/package suites. Live VPP/host integration belongs to the laboratory's existing exclusive queue, never this worker. Logs and JSON results are persisted under /root/.cache/ngfw-test-jobs on disk. The detached finite local worker survives completion of this chat turn. It is not an AI agent or an auto-resuming development service.

Result states: submitted/checking/queued_or_running, passed, failed, stale or lost. Commands have a queue+execution deadline and owned-process-group TERM/KILL cleanup, including descendants retaining lock FDs. HEAD and source cleanliness are verified before/after validation; edited snapshots cannot receive a valid pass. Ignored dependencies and cooperative frozen-tree ownership remain assumptions. Recheck a generated tree at its NEW committed SHA rather than claiming generated changes validated an earlier SHA.

## Evidence

Independent source review: developer_iso report in its own worktree, commit962989e8, APPROVE WITH LIMITS.

Isolated scheduling tests: python3 tools/test_test_handoff.py, 7 tests passed in14.546s. Cases cover fast admission while long slots1/2 held, failure exit propagation, initial dirty/untracked sources, source mutation during validation, emergency/live-integration refusal, stubborn descendant cleanup and lock release after timeout. No lab or global locks touched by fixture tests.

Actual LCP root-owned job7280ef645e00451a880e23df1d955235: SHA2d939b7a, fast slot3 after0s, go LCP -race passed1.421s; result passed, execution finished2026-10-03T10:27:22Z (13:57:22 Tehran).

PKI root-owned job34e8de89c0a141389126c5bf5f898b6b: immutable18cfd819, finite API/guard/typecheck/lint suite in long lane. Developer moved to separate PKI-api-dto worktree rather than waiting. Notifications sourceef1a7392 remains frozen; coordinator's separate Notifications-validation worktree prepares dependencies/generation; developer continues successor UI/docs. ISO developer independently re-reviews PKI security fixes without running expensive duplicate tests.

Expected benefit: short jobs avoid long-job starvation once the former slot3 holder drains; coding/review continues while tests execute. Long suites still queue behind two long jobs and live lab tests still serialize. This removes developer idle time, not test requirements. Hosted quick CI and independent applicable review remain mandatory before merge.
