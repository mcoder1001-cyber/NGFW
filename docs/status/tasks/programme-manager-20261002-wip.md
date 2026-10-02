# NGFW programme manager recovery — 2026-10-02

Verified main: 2312bd4a6d540ba9ae873dd049e59d375589e905; PR59 merged by root. This is a recovery checkpoint, not evidence of completed product work.

## Live execution at checkpoint

- Programme manager: programme_manager, branch codex/programme-manager-20261002; coordinates separate worktrees and merge queue.
- Developer dashboard_fix: branch codex/dashboard-fix-20261002; investigates PR58 Persian tab test failure without disabling assertions.
- Developer notifications_finish: branch codex/notifications-finish-20261002; recovers dd820ce3 contract and implements SMTP/HMAC webhook only. Telegram removal explicitly authorized.
- Developer identity_finish: branch codex/identity-finish-20261002; completes missing operational identity state and login banner.
- Independent reviewers: none assigned at this checkpoint; fresh reviewers must be assigned before product merge.

Each developer must publish coherent checkpoints at least every 15 minutes with exact remote SHA; local-only work is not durable. Root monitors post-merge main CI. No permanent background runner is claimed. Later readers must verify live roster rather than treating this dated snapshot as current activity.

## Queue and gates

1. PR58: genuine test failure investigated before unchanged full hosted quick gate; rebase final single commit on current main, preserve archive and obtain independent delta review.
2. Notifications: complete generator output and runtime/UI; fresh security/contracts/correctness review and complete hosted quick gate.
3. Identity: additive contracts first, operational state and banner, meaningful tests, applicable independent reviews and complete hosted quick gate.
4. Start next ready work when slots open; preserve one independent review slot.

Live lab tests remain NOT RUN and are recorded only in docs/status/DEFERRED-ACCEPTANCE.md. Lab-only delay does not block merge. Real code defects, security boundaries and mandatory quick CI remain gates. Missing pinned local Go/buf toolchain is being coordinated; implementation continues independently.
