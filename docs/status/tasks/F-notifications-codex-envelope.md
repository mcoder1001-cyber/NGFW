# Codex task envelope: F-notifications

- Historical source branch: `task/F-notifications-codex`
- Historical source worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-notifications`
- Task scope: Notifications: email (SMTP) and signed HTTPS webhook for alarms, commits, link/VPN state and global-blocking fetch failures. Telegram was removed by the owner instruction in `AGENTS.md` (2026-10-02), which supersedes the historical Telegram scope in the feature prompt.
- Dependencies: F-dashboard-prom-alarms, F-management-ui
- Scope source: `prompts/features/F-notifications.md`, as amended by the owner instruction in `AGENTS.md`.
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.

## Merged source reconciliation — 2026-10-03

Reviewed base `6b7fdda2`, including Notifications merge `62cc2832`. The SMTP/webhook dispatcher, admin running-channel test action, Management tab, rules and bounded delivery history are implemented. The strongSwan/IPsec adapter is also implemented: `apps/api/src/features/notifications/notifications.service.ts:211–228` maps tunnel, rekey, daemon and recovered poll events to VPN notices; `notifications.test.ts:280–336` contains mapping/redaction and malformed/resync event regressions. English/Persian guides now describe this existing behavior instead of listing it as missing. This documentation change does not claim new execution of those tests.

Only default API network namespace routing is supported. Non-default management VRFs are refused by schema and transport; explicit socket binding remains unimplemented. Live SMTP/webhook delivery, actual VPN/event producer integration, browser workflows, routing and database restart acceptance remain NOT RUN in `docs/status/DEFERRED-ACCEPTANCE.md`. Documentation reconciliation does not substitute for that acceptance or introduce additional channel types.
