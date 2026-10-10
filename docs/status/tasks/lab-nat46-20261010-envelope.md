# NAT46 acceptance envelope

Branch `codex/lab-nat46-20261010`, worktree `/root/ngfw-wt/lab-nat46-20261010`, source main `4908716b4`. Owner: NAT46 acceptance agent; manager owns board. Owned files: `docs/status/tasks/lab-nat46-20261010*`; minimal NAT46 harness fixes only if necessary. Slot 17, prefix w17, table IDs 17000–17999. Test only independently spawned disposable VPP and owned agent/API/browser processes. Shared VPP, management path and .250 untouched. Routing worker authorizes second concurrent private instance. No trace or performance claims. Evidence distinguishes embedded-server fallback from arbitrary-server SIIT.

Additional owned harness: `docs/status/tasks/F-nat46-host-evidence/host.sh` capture-readiness fix. Runtime uses owned `/run/ngfw-test/w17/nat46` because rootdisk full prevents dropped-privilege tcpdump output into worktree-backed runtime. Temporary Go compilation uses owned executable tmpfs `/run/ngfw-nat46-buildtmp` via `/tmp/g-w17`; clean mount before handoff.
