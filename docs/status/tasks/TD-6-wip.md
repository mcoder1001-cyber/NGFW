# TD-6 — WIP (apply-startup.sh hardening, D-103)

Started 2026-09-24 07:59 (+0330), time box 3 h. Slot 6. Never `--apply` for real; fake-host harness only.

| item | state (final: see TD-6.md) |
|---|---|
| V1 stale dead-man reverts a newer commit | fixed, scenario passes (full harness pending) |
| V2 rollback write failure → VPP stopped, locks held | fixed, scenario passes (full harness pending) |
| V3 `systemctl show` failure after restart kills the run | fixed, scenario passes (full harness pending) |
| V4 NRestarts/MainPID baseline not asserted | fixed, scenario passes (full harness pending) |
| V5 run's own rollback without locks after holder death | fixed, scenario passes (full harness pending) |
| V6 VRX_TEST_ROOT lexical guard | fixed, scenario passes (full harness pending) |
| V7 holder HOLD_MAX from default counts | fixed, scenario passes (full harness pending) |
| V8 neigh probe: fe80 scope, poll bounded by count | fixed, scenario passes (full harness pending) |
| V9 dry run exit 0 when gate refuses | fixed, scenario passes (full harness pending) |
| L1 harness + shellcheck in tools/ci.sh quick | done (do_deploy_vpp, 7ecde6d); CI run pending |

Baseline (main): 101 passed, 8m18s. New scenarios 32–41: all FAIL on main's script, 25/25 pass on the branch.
Continue session 13:18–13:45 (stopped by the manager handover):
- done: continue envelope committed; L1 `do_deploy_vpp` in tools/ci.sh quick (shellcheck -x -P SCRIPTDIR of deploy/vpp/*.sh in
  parallel — all clean; harness in VRX_CI_APPLY_SHARDS=4 parallel shards; failed scenarios get ONE serial rerun → warn if
  green, fail otherwise; green result cached by sha256 of deploy/vpp/* + built vrx-startupgen); docs/agent/renderers/vppstartup.md
  "Hardening before the first real apply (TD-6, D-103)" section.
- full harness (4 shards, 13:30–13:44, host load avg 35–45 from other agents' CI): 102 passed, 24 failed in scenarios
  4 7 12 14 15 16 20 21 22 23 24 28 30 32 37 40 — mostly "bounded: Ns" timing checks and cmd-timeout-1s refusals; NOT yet
  known whether any is a real regression (the pre-handover run had 25/25 for 32–41 and main had 101/101 serial).
- not verified: the ci.sh rerun logic has not been exercised; `tools/ci.sh --base main` not run; docs/status/tasks/TD-6.md not written.
Continue session 3 (ngfw-46 envelope TD-6.continue46-envelope.md, 14:00–):
- serial rerun of the 16 scenarios (old harness, 14:01–14:16, load 17→26): 48 passed, 5 failed (7 12 21 30 40).
  30 fails on an IDLE host too: test race (foreign lock held a fixed 6 s; planner needs ~4 s + --lock-timeout 2) — fixed
  with foreign_lock (held until killed; 29 same pattern). 12 passes 3/3 alone. 7/21/40: bounded limits too tight.
- harness made load-tolerant (never by deleting a check): --cmd-timeout 2 / --svc-timeout 6 (hook 'sleep 2' had 1 s spare
  under svc 3), longer wait-up-to ceilings, bounded checks = idle limit × (1 + 3·min(load/CPUs, 1.6)), 37 checks the
  real property (waiter section and rollback stop..start do not overlap), FAIL lines print load + last verdicts.
- run A (new harness, 4 shards like ci.sh, 14:26–14:34, load 4.2–36.5): 128 passed, 0 failed, 439 s.
- ci.sh: VRX_TEST_ONLY/SHARD/KEEP never leak into the harness; VRX_TEST_APPLY_SCRIPT honoured, never cached.
- rerun-logic exercise found the rerun unreachable (a shard with a failed check exits 1 → counted as dead); fixed
  85a6ed5; re-exercised: flaky → WARN + green, main's script → FAIL after the rerun. Fixture cmd-timeout 3 s.
- `TMPDIR=/tmp/g-td6 tools/ci.sh --base main` at 85a6ed5: CI GATE PASSED, 11m51s, harness 128/128 in 4 shards.
- DONE: docs/status/tasks/TD-6.md written (all evidence pasted). Nothing left in the envelope.
Previous next command (on a quiet host, check `uptime` first):
  cd /root/ngfw-wt/TD-6 && go -C apps/agent build -o /tmp/td6-gen ./cmd/vrx-startupgen && \
  VRX_TEST_ONLY="4 7 12 14 15 16 20 21 22 23 24 28 30 32 37 40" deploy/vpp/test-apply-startup.sh /tmp/td6-gen
  then (if green) `tools/ci.sh --base main`, then write docs/status/tasks/TD-6.md with both outputs.
