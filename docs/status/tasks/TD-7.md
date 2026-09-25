# TD-7 — apply-startup.sh follow-ups from the TD-6 review (D-116)

Branch `task/TD-7` (worktree `/root/ngfw-wt/TD-7`, slot 6), speculative base `task/TD-6@621d1e2` (D-114). Scope: F1 and F2 of
`TD-6-review.md`; F3–F9 stay in `docs/tech-debt.md`. Nothing ran `apply-startup.sh --apply` against real paths. VPP was not
restarted, nothing under `/etc` was written, and no process was killed by pattern. Every run used the fake-host harness
(`VRX_TEST_ROOT` fixture) with scratch dirs under `/tmp/g-td7`. The worker was stopped once by the usage limit (16:40); its
edits were salvaged in f6e5f9d and this session (16:53, `CONTINUE-quota.md`) verified and finished them.

Files: `deploy/vpp/apply-startup.sh`, `deploy/vpp/test-apply-startup.sh`, `docs/agent/renderers/vppstartup.md`, this file.
`tools/ci.sh` is unchanged: the harness step selects scenarios by number per shard, so 42 and 43 run without registration.

## What changed

| # | fix | where |
|---|---|---|
| F1 per-apply lock | `stage_rollback` opens the apply's work dir on fd 7 (`exec 7<"$WORK"`) and takes `flock -n 7` before anything else: before the `finished` check, `touch deadman-fired`, `secure_locks`, `kill_run` or VPP. A second rollback stage of the same apply logs `a rollback of <work> is already running (pid N) — nothing to do`, sends that to syslog and exits 0 without touching anything. The work dir itself is the lock file: it is keyed by the apply id and nothing has to be created, so a full disk cannot stop the dead-man. The holder's pid goes to `<work>/rollback-pid` | `apply-startup.sh` `stage_rollback` (`:999-1030`) |
| F1 fd hygiene | `tmo`, `svc`, `vppcheck`, `sysfs_write`, `tcp_connect` and the two `flock -w` waits in `secure_locks` close fd 7 (as they already closed 8/9). An orphaned child therefore never keeps the per-apply lock. The log's `tee` is opened before fd 7, so it never inherits it | `:190-196`, `:294-296`, `:517` |
| F1 timer | after the lock, `cancel_deadman` (`systemctl stop <unit>.timer`, only the timer) disarms the armed dead-man before a rollback started by hand touches the run or VPP. This is harmless inside the timer's own service | `:1016` |
| F1 refusal text | the planner (`--apply`), the run (inside the locks) and the dry run now print the safe command through `finish_hint`: `systemctl start vrx-startup-apply-deadman-<stamp>.service`. systemd starts a unit only once, so this cannot run next to the timer's own start, and it runs detached from the SSH session. The stage by hand (`<work>/bin/apply-startup.sh --stage rollback --work <work>`) is printed only when no dead-man unit was recorded | `finish_hint` `:669-681`; `:451`, `:887`, `:1060` |
| F2 | scenario 40 counts the fake's `ip -j neigh show 10.0.0.1 dev ens192` calls instead of a load-scaled time bound. Every read hangs until the 3 s cmd-timeout, so the time-bounded poll makes 1 read (at most 2 are accepted), while the pre-V8 count-bounded poll makes 2 × cmd-timeout = 6 on any host. An outer `timeout 300` turns a regression into an unbounded loop into a failure instead of a stuck harness. The comment on `load_limit` and `vppstartup.md` no longer claim "always below the unbounded case" for every check | `test-apply-startup.sh` `:80-86`, `:851-868` |
| scenarios | 42: holder gone, rollback 1 held inside its `systemctl stop vpp` (fake stop hook), rollback 2 runs meanwhile → refused, VPP stopped and started once, never FORCED. 43: the same with the holder alive (run stuck in `systemctl restart`, dead-man armed) → the timer is disarmed before VPP is stopped, rollback 2 is refused, one stop and one start. 32 gets one more check: the refusal names `systemctl start <dead-man unit>.service` | `two_rollbacks` `:302-321`, `:877-909`, `:737` |
| doc | `vppstartup.md`: the V1 bullet no longer prescribes the hand-run stage; new section "Follow-ups from the TD-6 review (TD-7, D-116)"; the Tests paragraph lists 42–43, names the six load-scaled `bounded` checks and describes scenario 40's count check (scope addition below) | `docs/agent/renderers/vppstartup.md` |

## Proof: the new checks fail on the old scripts and pass on this branch

All three runs use this branch's harness. `VRX_TEST_APPLY_SCRIPT` selects the script under test. The reference copies are
`git show 621d1e2:deploy/vpp/apply-startup.sh` (TD-6) and `git show main:deploy/vpp/apply-startup.sh` (main, pre-TD-6;
main has not touched `deploy/vpp` since 2d622eb), both checked with `cmp` against git. Generator built from this tree.
The three runs ran in parallel at 16:54 (load 12 → 35).

### F1 on TD-6's script (`VRX_TEST_ONLY="32 42 43"`) — 6 FAIL
```
 16:54:22 up  7:06,  2 users,  load average: 12.13, 5.74, 12.98
$ TMPDIR=/tmp/g-td7/tmp VRX_TEST_ONLY="32 42 43" VRX_TEST_APPLY_SCRIPT=/tmp/g-td7/ref/td6-apply-startup.sh deploy/vpp/test-apply-startup.sh /tmp/g-td7/gen/vrx-startupgen
== 32. V1: a dead run's armed dead-man never reverts a newer apply — the planner refuses while an apply is unfinished; a superseded dead-man changes nothing
  ok   run 1 died after installing; its holder is gone; its dead-man is still armed
    state: rc=3 finished=[] vpp=active locks-free=y timer-cancelled=n live==new=y live==orig=n
  ok   a new apply is refused while run 1 is unfinished (exit 3; dir and timer named), nothing changed
  FAIL TD-7 F1: the refusal points at the safe command, the dead-man's own unit (systemctl start <unit>.service), not a second rollback by hand
  ok   run 1's dead-man makes its one rollback (exit 1): original file back
  ok   then the new apply commits (exit 0) and is recorded as the current one
    state: rc=1 finished=[superseded] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=n
  ok   late dead-man of run 1: superseded, the newer committed file stays (exit 1)
  ok   VPP neither stopped nor restarted by it, locks released
== 42. F1: two `--stage rollback` of one apply at once, holder gone (the operator finishes it by hand while its dead-man fires) → the second is refused at once; VPP stopped and started exactly once,
  ok   the apply installed and has no result; its holder is gone
    rollback 1 (pid 1189696): rc=1, inside its VPP stop while rollback 2 ran: y/y; rollback 2: rc=1; VPP stops=2 starts=2
    | rollback 2: apply-startup: 2026-09-24 16:56:12 dead-man: FORCED — locks held by someone else for 1s ($T/bin/lslocks: 4242 flock WRITE $T
    | rollback 2: apply-startup: 2026-09-24 16:56:12 ROLLBACK: dead-man: the apply did not finish within 1174s
    | rollback 2: apply-startup: 2026-09-24 16:56:16 rolled back to $T/apply/20260924-165543-1186066/backup.conf; VPP and the management path are healthy (manager session: no peer k
    state: rc=1 finished=[rolled-back] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=y
  FAIL the second rollback, started while the first is inside its VPP stop, is refused at once and touches nothing (exit 1)
  ok   the first one rolls back alone (exit 1): original file, VPP active, locks released
  FAIL VPP stopped and started exactly once (stops=2 starts=2); the locks taken exclusively, never FORCED
== 43. F1: the same overlap while the holder is alive (the run hangs in `systemctl restart`, its dead-man armed) → the rollback disarms the timer first; the second is refused; VPP stopped and starte
  ok   the run is stuck in 'systemctl restart'; holder 1194662 owns the locks; the dead-man vrx-startup-apply-deadman-20260924-165628-1192888 is armed
    rollback 1 (pid 1196829): rc=1, inside its VPP stop while rollback 2 ran: y/y; rollback 2: rc=1; VPP stops=2 starts=2
    | rollback 2: apply-startup: 2026-09-24 16:56:38 dead-man: lock holder 1194662 still owns the locks
    | rollback 2: apply-startup: 2026-09-24 16:56:38 ROLLBACK: dead-man: the apply did not finish within 3574s
    | rollback 2: apply-startup: 2026-09-24 16:56:45 rolled back to $T/apply/20260924-165628-1192888/backup.conf; VPP and the management path are healthy (manager session: no peer k
    state: rc=1 finished=[rolled-back] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=y
  FAIL the second rollback (the timer firing while the operator's runs) is refused at once and touches nothing (exit 1)
  FAIL the rollback disarmed the armed dead-man timer (calls line 37) before it stopped VPP (line 18)
  ok   the first one killed the run and rolled back alone (exit 1) under the holder's locks, then released them
  FAIL VPP stopped and started exactly once (stops=2 starts=2)

apply-startup tests: 10 passed, 6 failed
rc=1
 16:56:55 up  7:09,  2 users,  load average: 25.19, 13.72, 14.92
```

### F2 on main's pre-TD-6 script (`VRX_TEST_ONLY=40`) — FAIL, 6 reads
```
 16:54:22 up  7:06,  2 users,  load average: 12.13, 5.74, 12.98
$ TMPDIR=/tmp/g-td7/tmp VRX_TEST_ONLY=40 VRX_TEST_APPLY_SCRIPT=/tmp/g-td7/ref/main-apply-startup.sh deploy/vpp/test-apply-startup.sh /tmp/g-td7/gen/vrx-startupgen
== 40. V8: neigh probe — a link-local IPv6 default gateway is nudged with its scope; a hanging `ip neigh` is bounded by time
      NONE VIABLE — --apply will refuse: no viable management reachability check on this host: next hop fe80::1 on ens192 is not REACHABLE (neighbour state STALE) — pass --mgmt-probe tcp:HOST:PORT
  FAIL dual stack, gateway fe80::1: nudged as fe80::1%ens192, neigh viable (exit 3)
    elapsed 26s at load 20.58, 6 `ip neigh` read(s):   NONE VIABLE — --apply will refuse: no viable management reachability check on this host: next hop 10.0.0.1 on ens192
  ok   ip neigh hangs (cmd-timeout 3s): refused (exit 3)
  FAIL the poll is bounded by time, not by count: 6 hanging `ip neigh` read(s) of 3 s within the 3 s deadline (pre-V8: 2 × cmd-timeout = 6 reads) — independent of the host load

apply-startup tests: 1 passed, 2 failed
rc=1
 16:55:06 up  7:07,  2 users,  load average: 23.74, 9.35, 13.84
```
The elapsed time was 26 s at load 20.6. The old check's limit at that load was 10 · (1 + 3 · 20.6/32) ≈ 29 s, so the old
`bounded 10` would have passed here. That is exactly the gap the review describes. The count does not depend on the load.
(The first FAIL in this scenario, the fe80 nudge, is V8's other half and was already caught before.)

### This branch (`VRX_TEST_ONLY="32 40 42 43"`) — 19 passed, 0 failed
```
 16:54:22 up  7:06,  2 users,  load average: 12.13, 5.74, 12.98
$ TMPDIR=/tmp/g-td7/tmp VRX_TEST_ONLY="32 40 42 43" deploy/vpp/test-apply-startup.sh /tmp/g-td7/gen/vrx-startupgen
== 32. V1: a dead run's armed dead-man never reverts a newer apply — the planner refuses while an apply is unfinished; a superseded dead-man changes nothing
  ok   run 1 died after installing; its holder is gone; its dead-man is still armed
    state: rc=3 finished=[] vpp=active locks-free=y timer-cancelled=n live==new=y live==orig=n
  ok   a new apply is refused while run 1 is unfinished (exit 3; dir and timer named), nothing changed
  ok   TD-7 F1: the refusal points at the safe command, the dead-man's own unit (systemctl start <unit>.service), not a second rollback by hand
  ok   run 1's dead-man makes its one rollback (exit 1): original file back
  ok   then the new apply commits (exit 0) and is recorded as the current one
    state: rc=1 finished=[superseded] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=n
  ok   late dead-man of run 1: superseded, the newer committed file stays (exit 1)
  ok   VPP neither stopped nor restarted by it, locks released
== 40. V8: neigh probe — a link-local IPv6 default gateway is nudged with its scope; a hanging `ip neigh` is bounded by time
      will use: neigh (passes now); manager session: no peer known
  ok   dual stack, gateway fe80::1: nudged as fe80::1%ens192, neigh viable (exit 0)
    elapsed 12s at load 20.64, 1 `ip neigh` read(s):   NONE VIABLE — --apply will refuse: no viable management reachability check on this host: next hop 10.0.0.1 on ens192
  ok   ip neigh hangs (cmd-timeout 3s): refused (exit 3)
  ok   the poll is bounded by time, not by count: 1 hanging `ip neigh` read(s) of 3 s within the 3 s deadline (pre-V8: 2 × cmd-timeout = 6 reads) — independent of the host load
== 42. F1: two `--stage rollback` of one apply at once, holder gone (the operator finishes it by hand while its dead-man fires) → the second is refused at once; VPP stopped and started exactly once,
  ok   the apply installed and has no result; its holder is gone
    rollback 1 (pid 1192662): rc=1, inside its VPP stop while rollback 2 ran: y/y; rollback 2: rc=0; VPP stops=1 starts=1
    | rollback 2: apply-startup: 2026-09-24 16:56:27 a rollback of $T/apply/20260924-165607-1189172 is already running (pid 1192662) — nothing to do
    state: rc=1 finished=[rolled-back] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=y
  ok   the second rollback, started while the first is inside its VPP stop, is refused at once and touches nothing (exit 0)
  ok   the first one rolls back alone (exit 1): original file, VPP active, locks released
  ok   VPP stopped and started exactly once (stops=1 starts=1); the locks taken exclusively, never FORCED
== 43. F1: the same overlap while the holder is alive (the run hangs in `systemctl restart`, its dead-man armed) → the rollback disarms the timer first; the second is refused; VPP stopped and starte
  ok   the run is stuck in 'systemctl restart'; holder 1198255 owns the locks; the dead-man vrx-startup-apply-deadman-20260924-165636-1196655 is armed
    rollback 1 (pid 1199554): rc=1, inside its VPP stop while rollback 2 ran: y/y; rollback 2: rc=0; VPP stops=1 starts=1
    | rollback 2: apply-startup: 2026-09-24 16:56:55 a rollback of $T/apply/20260924-165636-1196655 is already running (pid 1199554) — nothing to do
    state: rc=1 finished=[rolled-back] vpp=active locks-free=y timer-cancelled=y live==new=n live==orig=y
  ok   the second rollback (the timer firing while the operator's runs) is refused at once and touches nothing (exit 0)
  ok   the rollback disarmed the armed dead-man timer (calls line 17) before it stopped VPP (line 19)
  ok   the first one killed the run and rolled back alone (exit 1) under the holder's locks, then released them
  ok   VPP stopped and started exactly once (stops=1 starts=1)

apply-startup tests: 19 passed, 0 failed
rc=0
 16:57:21 up  7:09,  2 users,  load average: 34.68, 16.76, 15.89
```

## Full harness (4 parallel shards, as `tools/ci.sh` runs it)
```
 17:00:42 up  7:13,  2 users,  load average: 60.06, 33.77, 22.70
$ for i in 1 2 3 4; do TMPDIR=/tmp/g-td7/tmp VRX_TEST_SHARD=$i/4 timeout -k 30 1800 deploy/vpp/test-apply-startup.sh /tmp/g-td7/gen/vrx-startupgen > full-shard$i.log & done; wait
wall 933s
 17:16:15 up  7:28,  2 users,  load average: 14.24, 51.67, 56.18
shard 1: NO RESULT LINE — died in: == 5. --stage run verifies the sealed plan (forged gate / altered sett rc=1
shard 2: apply-startup tests: 29 passed, 0 failed rc=0
shard 3: apply-startup tests: 26 passed, 0 failed rc=0
shard 4: apply-startup tests: 49 passed, 1 failed rc=1
  FAIL handover: done → accepted without approval (exit 3)
       load 168.50 103.01 54.82 on 32 CPUs; last verdicts in $T:
       | apply-startup: 2026-09-24 17:06:54 REFUSED: VPP did not list its plugins

# serial reruns once the load fell (what tools/ci.sh does for a FAIL; a died shard would fail its gate outright):
 17:17:05 up  7:29,  2 users,  load average: 11.70, 45.30, 53.78
$ TMPDIR=/tmp/g-td7/tmp VRX_TEST_SHARD=1/4 deploy/vpp/test-apply-startup.sh …   # the whole of shard 1 (1 5 9 … 41)
apply-startup tests: 33 passed, 0 failed
rc=0
 17:23:18 up  7:35,  2 users,  load average: 12.42, 22.91, 40.70
$ TMPDIR=/tmp/g-td7/tmp VRX_TEST_ONLY=4 deploy/vpp/test-apply-startup.sh …
apply-startup tests: 13 passed, 0 failed
rc=0
 17:19:13 up  7:31,  2 users,  load average: 13.39, 34.49, 48.68
```
Scenarios 42, 43 and 40 passed in this parallel run on the branch: 42 and 43 with `stops=1 starts=1` and the second
rollback refused (exit 0); 40 with `1 \`ip neigh\` read(s)` at load 14. All 138 checks (TD-6's 128, plus 1 in 32, 4 in 42
and 5 in 43) are green: 33 + 29 + 26 + 49 in the shards and reruns, plus scenario 4's check on its rerun.

**The two misses are load, not TD-7.** The run overlapped another agent's burst: load 60 at the start, 168.5 at 17:06,
14 at the end. Scenario 4 failed with `REFUSED: VPP did not list its plugins`, which means the fake `vrx-vppcheck` did
not answer within the 3 s cmd-timeout. Shard 1 died in scenario 5, which TD-6 left unchanged: its first line
`apply >/dev/null 2>&1` has no `|| rc=$?`, so under `set -e` the same load-induced refusal ends the whole shard with no
result line. `tools/ci.sh` gives a FAIL one serial rerun, but it fails a died shard outright. Both went green in a
serial rerun at load 12. This is outside F1/F2, so it is only recorded under open questions below.

## shellcheck
```
$ shellcheck --version | sed -n 2p; shellcheck -x -P SCRIPTDIR deploy/vpp/*.sh && echo SHELLCHECK-CLEAN
version: 0.11.0
SHELLCHECK-CLEAN
```

## CI
```
 17:23:53 up  7:36,  2 users,  load average: 10.19, 21.18, 39.46
$ TMPDIR=/tmp/g-td7 VRX_CI_CACHE_DIR=/tmp/g-td7/cache tools/ci.sh --base main      # fresh cache dir: the harness really runs
== deploy/vpp: shellcheck + apply-startup fake-host harness ==
shellcheck ok: ./apply-startup.sh ./build.sh ./lib.sh ./test-apply-startup.sh ./verify.sh
  FAIL the run records hold-until ≥ its dead-man deadline + lock wait + one rollback
apply-startup harness: 1 check(s) failed in scenario(s) 39 — one serial rerun
WARN apply-startup harness: scenario(s) 39 failed in the parallel run and passed on a serial rerun (host load?) — logs /root/ngfw-wt/logs/ci/TD-7-20260924-172353-1345975/09-apply-startup-*.log
rerun: apply-startup tests: 2 passed, 0 failed
apply-startup harness: green (4 shards; 137 checks passed in the parallel run; scenario(s) 39 green only on the serial rerun)

== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m16s
  generate + generated-output gate                   1m46s
  forbidden patterns (+ gitleaks)                    0m03s
  lint · typecheck · unit tests · build (turbo)   1m32s
  apps/agent: make lint test build                   1m07s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  deploy/vpp: shellcheck + apply-startup fake-host harness   8m46s
  warnings:
    - commit subject(s) not in Conventional Commits form (type(scope): subject):
      review(TD-6): verdict
    - apply-startup harness: scenario(s) 39 failed in the parallel run and passed on a serial rerun (host load?) — logs /root/ngfw-wt/logs/ci/TD-7-20260924-172353-1345975/09-apply-startup-*.log
  mode quick · wall time 13m36s · logs /root/ngfw-wt/logs/ci/TD-7-20260924-172353-1345975

CI GATE PASSED

ci rc=0 wall 816s
 17:37:29 up  7:50,  2 users,  load average: 26.31, 38.65, 38.32

$ grep -h 'apply-startup tests:' /root/ngfw-wt/logs/ci/TD-7-20260924-172353-1345975/09-apply-startup-*.log
09-apply-startup-shard1.log: apply-startup tests: 33 passed, 0 failed
09-apply-startup-shard2.log: apply-startup tests: 29 passed, 0 failed
09-apply-startup-shard3.log: apply-startup tests: 25 passed, 1 failed
09-apply-startup-shard4.log: apply-startup tests: 50 passed, 0 failed
09-apply-startup-rerun.log: apply-startup tests: 2 passed, 0 failed
```
CI ran on 146194f (code identical to the final HEAD). While it ran, 187bfb9 changed only `vppstartup.md` and this file.
`*.md` is in `.prettierignore`, docs are excluded from the forbidden-pattern scan, and the harness cache key covers only
`deploy/vpp/*` + the generator, so no CI step depends on those two files. 137 + 1 = 138 checks, matching the full-harness
count above. The shellcheck part is also shown above (`shellcheck ok: … 5 files`).

**Scenario 39's parallel-run miss was not TD-7 code, but it is a real finding (new, LOW, fails safe).** The check needs
the scenario's healthy apply to commit. At load 70 it rolled back: `ROLLBACK: vpp.service restarted since the apply
(ActiveEnterTimestampMonotonic=78000 MainPID=1001 NRestarts=0 → MainPID=1001 )`. `check_health` (`apply-startup.sh:568`,
TD-6 code with no TD-7 diff) compares a single `unit_ident` read with the baseline. That read is `systemctl show` under the
3 s cmd-timeout, with no retry. The fake's `show` was killed after its first line, and the partial tuple counted as a
restart. On the real host a transient D-Bus timeout during the watch window would do the same thing: V3 made only the
baseline read retry. The result is a needless rollback (one more VPP restart), never a false commit. See the questions
below.

## Out of scope
F3–F9 of the TD-6 review (`docs/tech-debt.md`), any real `--apply`, `/etc/vpp`, `vpp.service`, `tools/ci.sh`.

Two items stay tech-debt and are **not fixed in TD-7** (manager, pre-merge check of TD-6). Both are pre-existing: the
TD-6 review lists them under "Pre-existing, not TD-6". They are not yet in main's `docs/tech-debt.md` (its TD-6 line lists
F3–F9 only), and that file is outside this task's files, so they are recorded here for the manager to add:
- **The rollback's final verification never checks that VPP really restarted** (`apply-startup.sh` `:771-773` on this
  branch, `:753-755` at TD-6): it runs `wait_api`, `is-active` and `check_mgmt`, but never compares the boot identity
  with the failed instance's `ident.*`. If `svc stop` and `svc kill` both fail, `start` is a no-op, and the old instance
  can pass `check_mgmt` and be marked `rolled-back`.
- **The harness's `kill_recorded` may SIGKILL PIDs that were reaped long ago** (`test-apply-startup.sh:54`): it sends
  `kill -KILL` to every PID recorded by earlier cases. With `pid_max` 4194304 a recycled PID is unlikely, but it is not
  strictly "only PIDs you spawned".

## Scope addition (manager, during the task): the scenario-40 sentence in `vppstartup.md`
TD-6's Tests paragraph said the load-scaled limits are "always below the unbounded case". The salvaged commit had already
removed that. The paragraph now describes the checks exactly. Only the six `bounded` checks (scenarios 7, 21, 22, 23, 26,
29; idle limits 15–30 s) are load-scaled. Each guards a fake `sleep 300`/`sleep 1000`, and the largest limit is
30 s × 5.8 = 174 s < 300 s. Scenario 40 has no time bound: it counts the fake's `ip -j neigh show` reads (≤ 2 pass; the
count-bounded poll makes 6), independent of the load. Verified with
`awk '/^echo "== [0-9]+\./ {h=$3} /^bounded [0-9]/ {print NR": scenario "h" limit "$2}' deploy/vpp/test-apply-startup.sh`:
```
436: scenario 7. limit 15
593: scenario 21. limit 30
602: scenario 22. limit 30
611: scenario 23. limit 30
664: scenario 26. limit 20
697: scenario 29. limit 15
```

## Notes / open questions
- The per-apply lock is `flock` on the work dir itself rather than on a `rollback.lock` file inside it, as the review
  suggested. The effect is the same and it needs no write, so a dead-man on a full disk still takes it.
- A refused second rollback exits 0. In the timer's service that means the timer's start does not fail while the
  operator's rollback does the work. The operator's hand-run stage prints the pid of the one that is running.
- `systemctl start <dead-man>.service` for a pending `systemd-run --on-active` timer: systemd loads the transient
  service together with the timer, so the unit is startable before the timer fires. This was not exercised on the real
  host (no real apply is allowed). The doc names the hand-run stage as the fallback if systemd no longer knows the unit.
- New, for the manager (not fixed here, outside F1/F2): `check_health` treats a failed or partial `systemctl show` read
  in the watch window as "vpp.service restarted" (see CI above). Fix idea: retry an unparsable tuple the way
  `read_unit_restart` does, and roll back only on a *complete* tuple that differs. Tech-debt, or a TD-8 row with the
  scenario-5/28/29 harness guard below?
- For tech-debt (not fixed here, outside F1/F2): harness scenario 5's first `apply >/dev/null 2>&1` (unchanged since
  before TD-6) should be `|| true`. Then a refusal under extreme load becomes a FAIL that gets the serial rerun instead of
  a died shard that fails the CI gate. The same unguarded line is in scenarios 28 (`:678`) and 29 (`:690`).
