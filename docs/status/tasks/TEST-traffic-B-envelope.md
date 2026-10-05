# TEST-traffic-B execution envelope

Assigned 2026-10-05 by current manager. Base origin/main d314f0728.
Branch codex/test-traffic-b-20261005, isolated worktree /root/ngfw-wt/TEST-traffic-B-20261005.
Owned files test/topology/traffic-b/** and docs/status/tasks/TEST-traffic-B*.
Slot27 prefix w27, HTTP12700, web16700, tables27000–27999. No existing /run/ngfw-test/w27 observed at start.
Disposable VPP/netns daemons only. No host system daemon ownership. No shared VPP mutation, restart, global trace or management interfaces.
Keep developing until full source composition is implemented; no claim of Done for missing functionality.

## Fresh reassignment after A3 ruling (2026-10-05)

Developer /root/traffic_finish_repair owns branch codex/test-traffic-b-finish-20261005
and isolated worktree /root/ngfw-wt/test-traffic-b-finish-20261005. Starting compiled
source 3e94010253e78fc68fb742e3b98132cfb5a870ad, with original author's docs-only
handoff 33ab0be397ddef85b5645588bcfd9c7d8d348dc4 retained. Additional bounded
ownership: test/topology/kea-dhcp-relay/{stack_test.go,dhcp_test.go,commit_guard_test.go}
for DHCP acceptance guards; traffic-b/run.py and test_scenario.py for actual DHCP
proof readback. No production API/agent edits. Slot27 is reserved for the corrected
pinned campaign only after manager coordination; independent tester owns slot28.
