# Codex task envelope: F-multiwan-host

- Branch: `task/F-multiwan-host-codex`
- Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-multiwan`
- Task scope: Multi-WAN on the lab: two WAN netns, failover time, balance split, per-member NAT
- Dependencies: F-multiwan
- Scope source: `plan/tasks.yaml scope/notes and original task report`
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.
