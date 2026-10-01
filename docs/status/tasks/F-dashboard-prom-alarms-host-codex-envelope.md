# Codex task envelope: F-dashboard-prom-alarms-host

- Branch: `task/F-dashboard-prom-alarms-host-codex`
- Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-dashboard`
- Task scope: Dashboard/Prometheus/alarms on the lab VPP: real StatsSource + collector/listener wiring, rig acceptance
- Dependencies: F-dashboard-prom-alarms
- Scope source: `plan/tasks.yaml scope/notes and original task report`
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.
