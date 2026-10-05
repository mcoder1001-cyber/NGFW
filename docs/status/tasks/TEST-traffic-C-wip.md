# TEST-traffic-C recovery

Branch: `codex/test-traffic-c`; worktree: `/tmp/ngfw-traffic-c`.
Owned: `test/topology/traffic-c/**`, `docs/status/tasks/TEST-traffic-C*`.
Base: `d314f0728`; initial local checkpoints `718598479`, `3daf1023a`.
Published checkpoint: remote SHA `7452f8be9c838167c14cec8371bb5ab5b91a5be8` via
GitHub connector, same source tree as local `3daf1023a`. CLI push returned 403.
Published full source checkpoint: `3bd70b3941b14b03b7a822355c54c6d427aca0b0`.
Draft PR: https://github.com/mcoder1001-cyber/NGFW/pull/186.
Publication ancestry note: initial connector checkpoint parent was updated main
`aebfc46f`, while the isolated local branch was based on `d314f0728`. Current
publication must preserve the original remote parent main tree and compare
owned source paths, rather than reversing newer board/progress rows. No main
history is rewritten. Reviewed transport/process fixes published at remote `3c3cd9fe93a63e772d164e9f95f15f718a7dcfd7` (local `5e2c24732`). Owned source/status paths were verified identical after git fetch; only main board/progress baseline differs. Draft PR has16ownedfiles and no board reversions.
No live processes/daemon/slot ownership. No shared host modifications.
Completed: leased product commit/rollback, MPLS/SRv6/VRRP/QoS/rider orchestration,
private evidence capture, exact global restore, residue checks, TD-H18 host-test
invocation, source/unit checks. Actual lab acceptance and independent review pending.
Constraint: combined global-owner execution requires absent table0 at entry;
see questions file. No acceptance claim for an existing shared table0.
Tests: 16 Python checks pass (including real redirects/descendant/TERM regressions); execute --dry-run passes; globals Go vet passed.
Globals Go unit tests passed (2 refusal tests), current check passed10s. Mandatory complete quick CI pending root union gate.
Next: independent reviewer recheck and root complete union gate; live manager window remains unrun.
