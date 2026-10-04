# MPLS rollback closeout envelope

- Branch: `codex/closeout-mpls`; base: `664544349`.
- Worktree: `/root/ngfw-wt/codex-closeout-mpls`.
- Owner: MPLS rollback investigation; slot 7, disposable VPP only.
- Owned files: `apps/agent/internal/descriptors/mpls/mpls.go`, `mpls_test.go`, `docs/status/tasks/closeout-mpls*`.
- Shared VPP, system service, installed plugin/source and startup configuration must remain untouched.
- Report actual test results; preserve the host rollback assertion. Independent manager review before integration.
