# TEST-traffic-A source foundation envelope

- Branch: `task/TEST-traffic-A-source-20261002`.
- Worktree: `NGFW-traffic-a`; base `6414528b61d343e74945dac20eef20b8cf163083`.
- Owned: `test/topology/traffic-a/**`, `docs/status/tasks/TEST-traffic-A*` only.
- Worker: CI/release stream packaging developer; manager owns board/publication.
- Live slot: none assigned or used; fixture-only development. No daemon owner.
- First checkpoint budget:15 minutes; commit each coherent delta.

Read AGENTS.md, shared context, contributing, decision policy, topology README
and shared-host rules. All12 board dependencies are actually MERGED on this base;
row READY/unowned. No standalone prompt exists; board scenario is authoritative.

Actual findings: reusable fixed Go topology suites provide VLAN optional ping/
counters, bridge/BVI structural reconciliation (explicitly no packets), VRF route
checks, ACL ping, and NAT44 ED/EI namespace TCP/tcpdump. uRPF/PBR lives in the
agent subsystem integration package, not a topology module. Running those suites
sequentially does not prove a single composed VLAN→BVI→VRF→policy→ACL→NAT path.
That remains a genuine source coverage gap, not merely deferred lab execution.

Bounded first implementation: explicit fixed stage inventory and strict evidence
orchestration. Plan/refusal/evidence state fixtures remain host independent.
Future live execution requires explicit integration/host opt-in, manager slot
lease, validated tools/lab slot exports, shared lab lock plus slot exclusivity,
bounded commands/timeouts, only fixed repository test selectors, and no skipped
selected tests counted as passing. Never CI slot12 or nonexistent13. Capture
contracts scope tcpdump to ns-w<N>-lan/wan and their peer devices, retain packet
and test evidence independently from process exit status. No VPP packet-trace
commands/global capture, restarting VPP, SSH/globalhost/service/package changes.

No fabricated completed-chain evidence or wave-A PASS is permitted. Pure fixture
success proves orchestration guards/parsers only; real forwarding remains NOT
RUN. Manager records live cases in the single DEFERRED-ACCEPTANCE campaign.
No public schema, generic rig/scheduler or another feature's files are changed.
Independent review and unchanged exact-head hosted quick required before merge.
