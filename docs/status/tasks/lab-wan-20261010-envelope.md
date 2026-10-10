# WAN laboratory acceptance envelope

User authorized every feasible acceptance test on 172.30.126.195 and .250.
Worker branch codex/lab-wan-20261010; worktree /root/ngfw-wt/lab-wan-20261010.
Slot20; dedicated private VPP/private namespaces only, two-instance budget coordinated with root.
Owned: lab-wan-20261010 status/evidence and minimal existing PPP/MultiWAN harness fixes. Board manager-owned.
No shared VPP restart, service/package/management-network change. .250 installed audit separate from current-source acceptance.

Manager expands scope for proven native session destruction on resync: scheduler optional read-only requirement interface/dispatch, natcommon nonowner opt-in and real 1000session regression. Routing releases scheduler files and owns only PPP delegation polling. No schema/proto or privileges change.

Manager-expanded production ownership: scripts/pppoe-kernel-carrier.py, scripts/tests/pppoe-kernel-carrier.py for exact typed pristine kernel fallback guard; root independent review required before native activation.

2026-10-10 manager-approved repair contracts, before consumers:
- Empty MultiWAN source synchronizes once initially and once after configured groups withdraw; failures retry every tick. Configured groups always retain periodic repair, including all members down and unchanged configuration. Configuration presence comes from authoritative committed monitor groups, never dry-run projection or learned gateway availability.
- Applied PPP manifest retrieval with a typed unavailable old password returns explicit drift, never a verified-file claim. Structurally valid old carrier metadata is remembered without credentials for fixed-unit teardown. Desired creation still requires its current secret. Missing committed credentials cannot be accepted as a healthy resync; the repair fails closed.
Owned added files: apps/agent/internal/agent/wan_routes.go and its focused poll test; apps/agent/internal/multiwan/runtime.go accessor; apps/agent/internal/descriptors/pppoe/client_config.go and focused readback test. Independent review and behavioral red/green controls precede live consumption.
