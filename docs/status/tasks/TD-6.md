# TD-6 — apply-startup.sh hardening before the first real --apply (D-103)

Branch `task/TD-6` (worktree `/root/ngfw-wt/TD-6`, slot 6). Envelopes: `TD-6.envelope.md`, `TD-6.continue-envelope.md`,
`TD-6.continue46-envelope.md`. Findings are from `F-startup-apply-verify.md` (V1–V9, L1).
Nothing ran `apply-startup.sh --apply` against real paths. VPP was not restarted, no NIC was bound or unbound, and nothing
was written under `/etc`. Every run used the fake-host harness (`VRX_TEST_ROOT` fixture) and scratch dirs under `/tmp/g-td6`.

Commits: ff520d3 (V1–V9 + scenarios 32–41), 7ecde6d + 59fb54e (L1: ci.sh step, doc), 4ccf665 (load-tolerant harness),
85a6ed5 (ci.sh: the serial rerun is reachable; fixture cmd-timeout 3 s), this file.

## Per finding

| # | state | how | proof (fails on main's script, passes on the branch) |
|---|---|---|---|
| V1 stale dead-man reverts a newer commit | **fixed** | the planner and the run (inside the locks) refuse while another work dir is `installed` with no result marker, naming the dir and its timer. `claim_current` writes `<state>/current` and disarms the older unfinished dead-men. A rollback checks ownership first (`not_ours`): `newer:` gives `superseded`, `foreign:` gives `console-needed`, and VPP is not touched in either case | scenario 32 |
| V2 rollback cannot write startup.conf | **fixed** | the new file and the rollback copy are staged next to the live file before the install, so install and restore are renames. If the rollback copy cannot be staged, VPP is not stopped (`ROLLBACK IMPOSSIBLE`). If the rename fails while VPP is stopped, VPP is started again anyway (`ROLLBACK INCOMPLETE`). Every path writes a marker, cancels the timer and releases the locks | scenarios 33, 34 |
| V3 `systemctl show` failure after the restart | **fixed** | `read_unit_restart`: up to 3 reads. A persistent failure is a failed apply → immediate rollback. A `run_exit` EXIT trap rolls back a run that stops unexpectedly after installing | scenario 35 |
| V4 crash + auto-restart before the first read | **fixed** | the baseline tuple must be complete, have `MainPID` ≠ 0 and `NRestarts=0` | scenario 36 |
| V5 run's rollback without locks | **fixed** | `secure_locks`, shared with the dead-man: exclusive and bounded; FORCED only when a foreign process holds the locks | scenario 37 |
| V6 lexical `VRX_TEST_ROOT` guard | **fixed** | `realpath -m` on every path and on the root. A root of `/` is refused, as is the real startup.conf/sysfs/systemctl/systemd-run | scenario 38 |
| V7 holder HOLD_MAX from default counts | **fixed** | after its snapshot the run writes `<work>/hold-until`, and the holder re-reads it every second | scenario 39 |
| V8 neigh probe (fe80 scope, count-bounded poll) | **fixed** | the nudge goes to `gw%dev` for `fe80::/10` and is bounded at ≤ 3 s. The poll is bounded by a `SECONDS` deadline | scenario 40 |
| V9 dry run exit 0 when the gate refuses | **fixed** | `gate … \|\| rc=3` | scenarios 41 and 1 |
| L1 harness + shellcheck in CI | **done** | `tools/ci.sh` quick → `do_deploy_vpp`: `shellcheck -x` on `deploy/vpp/*.sh`, the harness in 4 parallel shards, one serial rerun of failed scenarios (green → WARN, red → FAIL), a green result cached by the sha256 of `deploy/vpp/*` + the built generator. Self-contained; the v19_preflight part (TD-3) is untouched | exercised below |

"Fails on main's script" is shown by the fail-path exercise below: `VRX_TEST_APPLY_SCRIPT` = `git show main:deploy/vpp/apply-startup.sh`
makes the checks in scenarios 1 and 32–41 fail. Scenario 28 also fails there, but only because the branch reworded one log line.

## Continue-46 item 1: the 16 scenarios that failed under load (run at 13:30–13:44, load 35–45). Timing or real?

Serial rerun with the old harness, as the WIP file says (load 17 → 26, other agents' CI running):
```
$ uptime (before)
 14:01:39 up  4:14,  2 users,  load average: 17.30, 10.92, 13.13
$ VRX_TEST_ONLY="4 7 12 14 15 16 20 21 22 23 24 28 30 32 37 40" deploy/vpp/test-apply-startup.sh $TMPDIR/td6-gen
== 7. VPP hung BEFORE the apply → preflight refuses within the timeout, nothing installed
  FAIL bounded: 15s
== 12. --mgmt-probe tcp:… → commits; path lost → rollback judged by the same probe
  FAIL exit 1, lost TCP path → rollback, healthy afterwards by the same probe
== 21. VPP HANGS after the restart (socket accepted, no answer) → bounded wait, rollback
  FAIL bounded: 35s (cmd-timeout 1s, api-wait 2s)
== 30. lab lock held by someone else at apply time → refused, nothing changed
  FAIL exit 0, lock busy (holder named), file unchanged
== 40. V8: neigh probe — … a hanging `ip neigh` is bounded by time
  FAIL ip neigh hangs (cmd-timeout 3s): refused within 15s (exit 3)
apply-startup tests: 48 passed, 5 failed
rc=1 wall=889s
$ uptime (after)
 14:16:28 up  4:29,  2 users,  load average: 25.68, 26.81, 24.86
```
Then each remaining failure alone, on a quieter host (load 22 → 2):

| scenario | verdict | evidence |
|---|---|---|
| 30 | **real harness bug (test race), not a script bug** | it fails on an idle host too (`VRX_TEST_ONLY=7 21 30 40` at load 2.85: 30 FAIL, 7/21/40 ok). The fixture held the foreign lab lock with a fixed `flock -x … sleep 6`. The kept case log shows the planner starting at 14:22:37 and the holder unit getting both locks at 14:22:41 (4 s + `--lock-timeout 2`), after the 6 s had run out. The script behaved correctly: the lock was free, so it applied. Fixed: `foreign_lock` holds until killed and returns only once the lock is observed held. Scenario 29 had the same `sleep 0.3` race and also left an orphaned `sleep 300` holding the lock (flock's child): fixed the same way |
| 12 | **timing** | 3/3 passed alone (load 22, 12, 7: `2 passed, 0 failed`, 36–51 s each). The old harness printed no verdict on FAIL, so the exact check that tripped under load is not recorded. The fixture's timeouts were 1 s per command and 3 s per systemctl job (`hook restart 'sleep 2'` in 24/37 had 1 s spare). It passed in every later run: run A, both exercises and CI |
| 7, 21, 40 | **timing** (bounded limits) | idle-host times with the new fixture (load ~4–5): 7 = 10 s, 21 = 16 s, 22 = 18 s, 23 = 21 s, 26 = 9 s, 29 = 6 s, 40 = 7 s. Under load: 21 = 42 s at load 30.6, 22 = 64 s at load 32.2 (3.6× idle) |
| the other 11 (4 14 15 16 20 22 23 24 28 32 37) | **timing** | green on the serial rerun at load 17–26 and in every run since |

No bug in `apply-startup.sh` came out of this. The harness was made load-tolerant without deleting any check (4ccf665, 85a6ed5):
- fixture `--cmd-timeout 3 --svc-timeout 6` (was 1/3). With 2 s, one `vrx-vppcheck` (CT+2 = 4 s) still missed its limit
  in a burst to load 37 (scenario 11, first exercise run below)
- wait-up-to ceilings raised (`work_wait` 30 s, `waitfor` 30–60 s). They return as soon as the file exists
- "bounded" checks = idle-host limit × load factor `1 + 3·min(load1/CPUs, 1.6)` (1 idle, 4 at load = CPUs, 5.8 at load
  51 on 32). The check line names the unbounded case (`sleep 300/600/1000`). Scenario 40's limit (10 s idle) still
  catches the pre-V8 poll: on main's script it measured 29 s against a 27 s limit at load 18 (below)
- scenario 37 now checks the real V5 property: the waiter's lab-lock section and the rollback's VPP stop..start do
  not overlap (plus "took the locks exclusively", not FORCED). Under load the run may legitimately win the lock race
- every FAIL prints the host load and the script's last verdict lines (ROLLBACK/REFUSED/CONSOLE/…)

Run A: the full harness exactly as ci.sh runs it (4 parallel shards), fixture CT=2, load sampled every 15 s:
```
 14:26:54 up  4:39,  2 users,  load average: 4.97, 14.38, 19.23
shard pid 227105 rc=0
shard pid 227106 rc=0
shard pid 227107 rc=0
shard pid 227108 rc=0
wall=439s
 14:34:13 up  4:46,  2 users,  load average: 3.96, 11.22, 16.58
load samples min/max: 4.23 / 36.46
apply-startup tests: 21 passed, 0 failed
apply-startup tests: 33 passed, 0 failed
apply-startup tests: 25 passed, 0 failed
apply-startup tests: 49 passed, 0 failed
  ok   bounded: 42s (limit 87s = 30s idle-host limit × load factor at load 30.63; unbounded: wait_api on a VPP that never answers: sleep 1000 per probe)
  ok   bounded: 64s (limit 90s = 30s idle-host limit × load factor at load 32.24; unbounded: a health read on a hung VPP: sleep 1000)
```
(These limits were computed with the first factor, 1 + 2r. The 64 s case is why the factor became 1 + 3r.)

## Continue-46 item 2: the ci.sh rerun logic, exercised

Driver: `/tmp/g-td6/ex/driver.sh` evals the exact text of `do_deploy_vpp` and of the helpers it uses from `tools/ci.sh`
and runs it in this worktree (scratch cache dir). Injections go through `VRX_TEST_APPLY_SCRIPT`, which the step honours
but never caches:
- `flaky-apply.sh` accepts scenario 3's `--window 0` (exit 0 instead of 2) only while `VRX_TEST_SHARD` is set, i.e. in
  the parallel run. The serial rerun gets the real script
- `main-apply-startup.sh` = `git show main:deploy/vpp/apply-startup.sh`

**First run (at 4ccf665) found a bug: the rerun was unreachable.** A shard with a failed check exits 1, and
`wait "$pid" || rc=1` took that for a dead shard:
```
  FAIL --window 0 refused (exit 0)
  FAIL exit 1, committed with neigh; closed session logged only; one VPP restart
CI GATE FAILED — apply-startup fake-host harness failed (126 passed; a shard without a result line died, or a scenario failed again on the serial rerun) — logs /tmp/g-td6/ex/logs-warn/01-apply-startup-*.log
```
(All shards had result lines, and no "one serial rerun" line appeared. The second FAIL is a real load flake with CT=2:
`load 36.97 17.13 16.77 … ROLLBACK: VPP API does not answer within 2s (hung?) or identity incomplete`.)
Fixed in 85a6ed5: a shard counts as dead only when it has no result line, or exits non-zero with 0 failures.

After the fix, WARN path (flaky, load 33 → 8):
```
WARN apply-startup harness: VRX_TEST_APPLY_SCRIPT=/tmp/g-td6/ex/flaky-apply.sh is exercised instead of deploy/vpp/apply-startup.sh — result not cached
  FAIL --window 0 refused (exit 0)
apply-startup harness: 1 check(s) failed in scenario(s) 3 — one serial rerun
WARN apply-startup harness: scenario(s) 3 failed in the parallel run and passed on a serial rerun (host load?) — logs /tmp/g-td6/ex/logs-warn2/01-apply-startup-*.log
rerun: apply-startup tests: 5 passed, 0 failed
apply-startup harness: green (4 shards; 127 checks passed in the parallel run; scenario(s) 3 green only on the serial rerun)
DRIVER: do_deploy_vpp returned 0 after 6m46s; warnings: 2
driver rc=0 wall=406s
```
FAIL path (main's pre-TD-6 script, load 33 → 18):
```
apply-startup harness: 23 check(s) failed in scenario(s) 1 28 32 33 34 35 36 37 38 39 40 41 — one serial rerun
  FAIL exit 0 (the gate would refuse --apply: TD-6 V9); unified diff shows the new dev lines
  FAIL a new apply is refused while run 1 is unfinished (exit 0; dir and timer named), nothing changed
  FAIL late dead-man of run 1: superseded, the newer committed file stays (exit 1)
  FAIL exit 1, console-needed names the failed restore
  FAIL VPP left running (never stopped), locks released, timer cancelled
  FAIL exit 1, console-needed: startup.conf not restored
  FAIL persistent failure: exit 1, rolled back at once, locks released
  FAIL exit 0, the auto-restarted instance is not accepted
  FAIL the waiter's lab-lock section [16,34] and the rollback's VPP stop..start [22,29] do not overlap (calls lines)
  FAIL test root '/.' with /./etc/vpp/startup.conf refused (exit 0)
  FAIL a holder follows hold-until (released after ~2 s)
  FAIL dual stack, gateway fe80::1: nudged as fe80::1%ens192, neigh viable (exit 3)
  FAIL bounded: 29s (limit 27s = 10s idle-host limit × load factor at load 18.20; unbounded: …)
  FAIL handover pending, no approval: exit 0 (sums still printed)
  … (23 in the parallel run)
CI GATE FAILED — apply-startup fake-host harness failed (105 passed; a shard without a result line died, or a scenario failed again on the serial rerun) — logs /tmp/g-td6/ex/logs-fail2/01-apply-startup-*.log
--- last 60 lines of /tmp/g-td6/ex/logs-fail2/01-apply-startup-rerun.log ---
apply-startup tests: 18 passed, 22 failed
driver rc=1 wall=716s
```
Also in `do_deploy_vpp`: `VRX_TEST_ONLY`, `VRX_TEST_SHARD` and `KEEP` are stripped from the harness's environment, so an
inherited selector can never cache a partial run as green.

## Continue-46 item 3: `TMPDIR=/tmp/g-td6 tools/ci.sh --base main`

(The manager's correction: use a short TMPDIR, because the unbound/chrony tests bind unix sockets, and those paths are
limited to 108 characters.)

```
$ uptime
 15:02:35 up  5:15,  2 users,  load average: 25.41, 21.49, 20.62
$ TMPDIR=/tmp/g-td6 tools/ci.sh --base main   # at 85a6ed5
…
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
apply-startup harness: green (4 shards; 128 checks passed in the parallel run)

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   1m43s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m44s
  apps/agent: make lint test build                   0m28s
  test/ Go modules, unit mode (test/integration/smoke)   0m08s
  deploy/vpp: shellcheck + apply-startup fake-host harness   7m40s
  mode quick · wall time 11m51s · logs /root/ngfw-wt/logs/ci/TD-6-20260924-150235-607258

CI GATE PASSED
ci.sh rc=0
$ uptime
 15:14:26 up  5:26,  2 users,  load average: 11.52, 12.95, 15.93
```

An earlier run at 4ccf665 (logs `/root/ngfw-wt/logs/ci/TD-6-20260924-143810-308976`) passed every step up to the harness.
The harness then failed with `test-apply-startup.sh: line 480: syntax error near unexpected token '('`. I had rewritten
the harness file (the CT change) while its shards were reading it; bash reads a script incrementally. That was my own
interference, not a defect. The files were left alone during the final run.

## Out of scope
- any real `--apply`, a VPP restart, `/etc/vpp`: never touched (envelope)
- TD-3's `v19_preflight` part of `tools/ci.sh`: untouched. `git merge-tree --write-tree main HEAD` → rc 0 (merges cleanly
  with main's `do_cli` addition next to `do_deploy_vpp`)
- performance of the harness beyond making it green next to other workers

## Open questions (none blocking)
1. The load factor reads `/proc/loadavg` (1-min). That is deterministic enough in practice, and it keeps the limits tight on
   an idle host. The alternative is a fixed limit about 6× the idle time. Manager: keep the load factor?
2. Budget: `tools/ci.sh quick` says < 6 min. When `deploy/vpp/*` changed, the harness step alone takes about 6–7 min in 4
   shards under load (run A 439 s, WARN exercise 406 s including the rerun, final CI 7m40s at load 25 → 12). Otherwise it is cached (0 s). The shards are
   uneven (49/33/25/21 checks). `VRX_CI_APPLY_SHARDS=6` would shorten it. Should the default go to 6?
3. In a hard burst a single fixture command can still miss its timeout. That happened once with CT=2 (load 37); with CT=3
   none was seen at loads up to 33. The serial rerun is the backstop, and it surfaces as a WARN, never as a silent pass.
