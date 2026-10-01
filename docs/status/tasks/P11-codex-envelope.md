# Codex task envelope: P11

- Branch: `task/P11-codex`
- Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-P11`
- Task scope: Wave B (day 10-12): strongSwan+VPP build (staging sysroot) + IPsec S2S + tunnel dashboards
- Dependencies: P08, RF-2, DF-5, W-seed, TD-8
- Scope source: `prompts/P11-strongswan-vpp.md`
- Manager alone updates board and merges. Worker commits code, tests and real evidence.
- Architecture/security: `prompts/00-CONTEXT.md`; gates: `docs/contributing.md`.
- Cloud execution; shared production lab is unreachable. Do not alter host VPP or claim live tests passed.
- Preserve pending secret channel and host hardening decisions. Develop independent components only.
- Generated code is regenerated only; additive contract commits precede implementation.
- No branch is merged without required test gate and independent review approval.
