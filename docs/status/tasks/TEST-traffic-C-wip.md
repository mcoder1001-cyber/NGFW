# TEST-traffic-C recovery

Branch: `codex/test-traffic-c`; worktree: `/tmp/ngfw-traffic-c`.
Owned: `test/topology/traffic-c/**`, `docs/status/tasks/TEST-traffic-C*`.
Base: `d314f0728`; initial local checkpoints `718598479`, `3daf1023a`.
Published checkpoint: remote SHA `7452f8be9c838167c14cec8371bb5ab5b91a5be8` via
GitHub connector, same source tree as local `3daf1023a`. CLI push returned 403.
New complete executor/globals-helper source checkpoint publication is pending.
No live processes/daemon/slot ownership. No shared host modifications.
Completed: leased product commit/rollback, MPLS/SRv6/VRRP/QoS/rider orchestration,
private evidence capture, exact global restore, residue checks, TD-H18 host-test
invocation, source/unit checks. Actual lab acceptance and independent review pending.
Constraint: combined global-owner execution requires absent table0 at entry;
see questions file. No acceptance claim for an existing shared table0.
Tests: 13 Python checks pass; execute --dry-run passes; globals Go vet passed.
Globals Go unit test and mandatory complete quick CI not yet recorded.
Next command: `tools/heavy.sh go -C test/topology/traffic-c/globals test -count=1 ./...`.
