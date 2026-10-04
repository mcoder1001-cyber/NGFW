# Running-task completion campaign — 2026-10-04

Owner request: complete and merge the eight running board rows. Origin baseline
4c8d1b247. Working branch codex/complete-running-20261004, worktree
/root/.codex/worktrees/8c19/NGFW. Full/hosted CI waiver inherited from the owner's
next-five campaign (docs/status/tasks/next-five-review-plan-20261004.md on remote
codex/next-five-manager-20261004); focused verification and independent review retained.

Completed source on integration branch:
- F-tunnels: combined live readback contract, protected REST, engine allocation
  identity UI, optional endpoints/FIBs/counters, advanced kinds and unavailable state.
- F-isis-rip: additive families/RIPng/version/auth contracts, semantic checks,
  renderer and observed state/API/UI, Event21 and prerequisite Event20 mapping.
  Found and closed production authentication delivery: reference-only sealed
  transaction-selected password adapter now powers both projection and FRR runtime.
- P11/F-ikev2-native: added native SA transition/rekey watcher using owner-filtered
  safe-state readback. Failure retains previous baseline; counter/uptime changes
  do not create events; canceled lifetime drains. Event12 now relays to ipsec.events.
  Certificate trust/key contract still requires the pending decision; not DONE.

Actual verification:
- focused agent Tunnel/Isis/Routing/EventOf/Drift checks: agent, FRR, desired,
  subsystems and contracttest PASS; lcp_osi/isis/rip packages initially had zero
  name matches, therefore ran their complete focused package suites next: PASS.
- lcp_osi, frr/isis, frr/rip, frr/ripng, contracttest full package suites PASS.
- new sealed resolver and FRR/Tunnel agent tests PASS (subsystems 0.436s, agent1.166s).
- native SA snapshot/rekey/failure/cancellation and native empty-state tests:
  go test -race ./internal/agent -run 'TestNativeSAEvents|TestNativeSAWatcher|TestIpsecStateNative'
  PASS1.758s; no host mutation.
- API tunnels4, isis-rip12, secret-delivery12 =28 PASS; web tunnels6+isis-rip2 =8 PASS.
- schema IS-IS/RIPng contract+semantic =4 PASS.
- turbo dependency builds:12/12 PASS; API/web typechecks+dependencies15/15 PASS.
- tools/ci.sh check --base origin/main PASS11s; gitleaks5.39MB no leaks.
- combined proto/API client/YANG generated with repository generators.

Other work: separate HA and WAN agents implementing their outstanding source;
packaging agent investigating real artifacts, corrected TD19 fixture regression.
P10 privilege/file ownership and native certificate semantics questions are
pending owner response. No shared VPP restart, startup/package/unit edit performed.
No task marked merged, no merge yet; independent frozen-source review is next.
CLI push failed403; connector publication queued, no remote SHA asserted yet.
