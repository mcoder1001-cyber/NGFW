# PPPoE IPv6 PR196 recovery envelope

Developer worktree: `/root/ngfw-wt/resume-pppoe-20261007`.
Branch: `codex/resume-pppoe-20261007`; initial source `992b264b2084a8adfcee755d2b5650b771a6a8f1`.
Scope: recover PR196 IPv6 diff, merge latest origin/main without rewriting source history,
audit and fix PPPoE IPv6 regressions, run bounded focused tests, commit and publish checkpoints.
Owned product files: PPPoE IPv6 diff only, principally renderer PPPoE state/hooks/tests and
subsystem PPPoE watcher/state/tests. Owned recovery docs: this envelope and matching WIP.
No P12/RA/board/main edits; no original PR branch changes, new PR, squash, main merge,
whole local quick, package installation, shared daemon/VPP mutation or privilege changes.
Manager performs independent review and unchanged complete hosted quick on final integration.
Lab execution may defer; actual product failures remain explicit.

Manager extension after independent review: own `deploy/debian/ngfw/debian/control`
for the DHCPv6 runtime dependency and `resume-pppoe-20261007-contract.md` for the
inherited contract status. Mandatory PID ownership, bounded verified shutdown,
publication/revocation serialization, failure reporting and package controls are
in scope. Prior provisional review snapshots do not approve later source.
