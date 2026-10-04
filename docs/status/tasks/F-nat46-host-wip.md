# F-nat46-host — WIP

- 2026-09-29 07:52 start; slot 17 (`w17`), branch `task/F-nat46-host`, base main@4741ff86.
- 07:57 **host blocked**: VPP hung since 2026-09-28 19:13:04 (`os_panic() called, aborting.` in the journal, process alive, CLI and
  binapi never answer, NRestarts=2). Details + ask → `F-nat46-host-questions.md` Q1. Nobody but the manager restarts VPP (D-012).
- 08:05 D-210 (tests off) received from the manager: no new tests; host evidence commands stay; gate at load < 30 after merging main.
- 08:20 evidence driver written: `docs/status/tasks/F-nat46-host-evidence/host.sh` (agent level, `ngfw-agentctl`, af_packet rig,
  shared lab lock, NRestarts around every step, no globals touched). Binaries built to `/tmp/g-w17/bin` (ngfw-agent, ngfw-agentctl,
  ngfw-vppcheck).
- next: poll VPP with `ngfw-vppcheck --timeout 20s version`; when it answers → `host.sh run` → status file → merge main → gate.
- 08:17 manager: D-211 INC-vpp-hang-20260928, row parked until the owner restarts VPP; probe stopped, own stuck vppctl killed by PID, WIP committed, handed back (respawn with CONTINUE).
