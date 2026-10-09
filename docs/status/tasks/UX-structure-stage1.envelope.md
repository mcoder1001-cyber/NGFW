# UX-structure-stage1 — task envelope

Owner request: adapt navigation and component placement to `/frontend`, one reviewable stage; preserve current styling and product semantics. NAT remains independent. Do not merge to main until the owner explicitly approves.

- Developer: root agent; branch `codex/ux-structure-stage1-20261007`.
- Worktree: `/root/ngfw-wt/ux-structure-stage1-20261007`.
- Base: `origin/main` at `19052bb130ab46977dc5b7aa7be25e976fb5aaf9`.
- Owned files: web navigation/shell/router, routing objects page, en/fa navigation/ACL/BGP locales, applicable web tests, task-specific status/user documentation.
- Reference source is read-only; do not copy product libraries or backend behavior.
- Scope: Policy naming with legacy ACL URL compatibility; independent Objects menu with existing tab links; routing categories and reusable prefix-list/route-map editors outside BGP.
- No schema/API/agent changes, NAT embedding, new zone-pair behavior, visual redesign, or deployment.
- Validation: targeted web navigation/page regressions, web lint/typecheck/build, unchanged quick gate; independent review before merge readiness.
- Host services, VPP and production UI must not be modified.
