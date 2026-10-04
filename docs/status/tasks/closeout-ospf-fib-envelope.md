# Private OSPF FIB proof envelope

- Branch `codex/closeout-ospf-fib`, base `6619ae6418f3fee5c81e6a546900a6079ced67e6`; worktree `/root/ngfw-wt/codex-closeout-ospf-fib`.
- Ownership expanded by manager to rootFRR daemon launcher in `apps/agent/internal/agent/ospf_topology_integration_test.go` only; own new `test/topology/ospf/private-fib.py` and `docs/status/tasks/closeout-ospf-fib*`.
- Slot 6, heavy scheduler, unchanged existing OSPF root-mode test and its locks.
- Root-for-FRR is wholly owned network namespace: never the host namespace. Disposable VPP only; private /run/netns, /run/frr and /run/ngfw-test. No physical NIC, host route or installed service changes.
- No assertion weakening. Report FIB proof only if original VPP dynamic-route/withdrawal checks pass.
