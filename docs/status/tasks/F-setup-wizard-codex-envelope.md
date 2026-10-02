# Codex task envelope: F-setup-wizard

- Branch: `task/F-setup-wizard-codex`
- Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-setup`
- Task scope: First-boot setup wizard: language/time, admin password, WAN (DHCP/static/PPPoE), LAN + DHCP, safe defaults, one commit
- Dependencies: F-system-identity, F-management-ui, F-kea-dhcp-relay, F-nat44-ed-sessions, F-host-acl-nftables
- Scope source: `prompts/features/F-setup-wizard.md`
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.
