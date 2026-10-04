# Agent integration fixture closeout envelope

- Owner: management_acceptance child agent, task reassigned by manager.
- Branch/worktree: `codex/closeout-agent-fixtures`, `/root/ngfw-wt/codex-closeout-agent-fixtures`.
- Base/source: `6645443499d111d7cbbaa8ed2d5f6bd9105544af`.
- Owned: `apps/agent/internal/agent/agent_integration_test.go`, `rpc_lb_integration_test.go`, `docs/status/tasks/closeout-agent-fixtures*`.
- Scope: strict current supported-domain expectations and isolated lifetime of persisted LB test ownership stores; test code only.
- Slot9, disposable VPP only, opt-in LB GC on disposable instance. No shared service restart, no MPLS edits.
- Review root generated YANG/CI gate and root closeout runner read-only. No root product writes.
- Remote publication blocked by manager's HTTP403; no credential workarounds or PR/merge.
