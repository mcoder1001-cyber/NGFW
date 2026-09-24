# TASK ENVELOPE — TD-3 (FIX ROUND 2 — the last; host manager 2026-09-24 08:5x)
id: TD-3   branch: task/TD-3   worktree: /root/ngfw-wt/TD-3
FIX ROUND 2: branch task/TD-3 holds fix round 1 and the re-review acce6e1 (APPROVE WITH CHANGES). Keep everything that is fixed; do only what is listed here.
read first: docs/status/tasks/TD-3-rereview.md; docs/status/tasks/TD-3.md; /root/ngfw/docs/decisions/LOG.md D-095, D-101
must do (before merge):
  - first `git merge main` (brings DF-5 ipsec/wireguard, F-startup-apply, tools/lab V24 quiesce)
  - H1: wire the sanitizer (Acquire on create, BeforeDelete on delete) into apps/agent/internal/descriptors/ipsec/itf.go (~:67) and wireguard/interface.go (~:89), same pattern as the others; add a guard test that fails when any descriptor under apps/agent/internal/descriptors calls a VPP interface-create message without going through ifsanitize (static scan over the package tree is fine)
  - M1: bound the placeholder loop — re-read the classify table list when an index is taken meanwhile, lower the cap (≤ 16 per create), count cap hits in the metric, fail closed (ErrNoCleanIndex → quarantine path) on cap
  - M2: the quarantine holder loopback uses a reserved high instance range (create_loopback_instance with is_specified; VPP caps instances at LOOPBACK_MAX_INSTANCE = 16384, src/vnet/ethernet/interface.c:740 — use 16000–16383 and document it as reserved; check the schema does not already allow users to name loop16000+ without a validation error, else add it to TD-3-contract questions), never the lowest free instance; document the range in docs/agent/descriptors/interface.md
  - cheap lows if time allows: L5 ci.sh pre-flight — run the after-tests check even when a suite failed, print all FAIL lines (no tail -20 truncation)
not now (manager tracks them): M3 exact per-interface binding readback instead of probing (→ tech-debt, before any production image); L7 af_packet rollback quiesce (→ TD-5); Release wiring (→ P08)
merged deps you can rely on: DF-1, DF-2, DF-5, DF-7, DF-8, P05, TD-1, F-startup-apply
slot: 2 → eval "$(tools/lab env 2)". Host runs: one package at a time, NO packets, NRestarts before/after every run pasted; before deleting any af_packet interface bring its veth down (D-101); skip tests that delete af_packet with the veth up and say so. Stop host runs if NRestarts rises.
files you own exclusively: as in docs/status/tasks/TD-3.continue-envelope.md, plus the create/delete wiring lines (and their tests) in apps/agent/internal/descriptors/{ipsec,wireguard}/ — nothing else in those two packages
time box: 3 h
finish by: "Fix round 2" section in docs/status/tasks/TD-3.md answering H1/M1/M2 (+ any lows) with pasted real output (unit, guard test, host tests with NRestarts) · tools/ci.sh --base main green · everything committed · final message = 10-line summary
commit WIP at least every 45 min and keep docs/status/tasks/TD-3-wip.md current — you may be killed and respawned with a CONTINUE envelope
questions: docs/status/tasks/TD-3-questions.md — write and keep going; never wait for a human
never: merge to main · touch /root/ngfw (main) or other worktrees · restart/kill VPP · edit /etc/vpp, startup.conf, vpp.service · Docker · pkill · secrets in files/logs · install packages
