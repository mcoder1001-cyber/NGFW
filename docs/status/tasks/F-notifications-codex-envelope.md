# Codex task envelope: F-notifications

- Branch: `task/F-notifications-codex`
- Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-notifications`
- Task scope: Notifications: email (SMTP), Telegram and webhook for alarms, commits and link/VPN state
- Dependencies: F-dashboard-prom-alarms, F-management-ui
- Scope source: `prompts/features/F-notifications.md`
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.
