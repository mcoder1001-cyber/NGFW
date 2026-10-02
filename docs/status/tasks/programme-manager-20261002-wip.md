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

## Executive streams update

Root explicitly split execution into three managers: programme_manager owns product/API/UI (dashboard, notifications, identity); ci_delivery_manager owns the confirmed YANG build graph race and CI delivery; network_delivery_manager owns next WAN/network scope. Root coordinates integration and the seven-agent concurrency limit. Multiple managers do not imply unlimited developer/reviewer slots.

Observed roster at this checkpoint: product manager, CI manager, network manager; two product developers (identity_finish, notifications_finish); one independent reviewer tools_restore now reviewing identity; root coordinator. Dashboard developer finished reviewable remote checkpoint148a7cc8 with approved R1/R6/R7 delta. PR58 updated to that exact head, hosted complete gate required.

Main2312 postmerge CI37013687601 FAILED: actual YANG declaration was observed transiently empty during duplicate unscheduled dependency builds. This is independently reproduced (9 empty reads across3 successful actual tsc builds), not lab-related. CI manager owns isolated fix/review; root investigates baseline failure before any revert decision.

Identity contracts and coherent implementation remotely preserved393fe955 thena372beb4; generated outputs checkpoint upload in progress. Fresh review found actual UI fixture health-field loss, nullable older-agent compatibility, locale number formatting and documentation gaps; author fixing before merge. Independent API13tests and Go renderer race tests passed. Notifications reports29 meaningful dispatcher/SSRF tests passed; full generation and durable next checkpoint pending. No product merge is claimed at this checkpoint.
