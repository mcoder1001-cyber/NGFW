# TD-6 — review (reviewer, slot 6, 2026-09-24)

Branch `task/TD-6` @ b388ca7 against main a8bc1f9 (merge base 5a0ea5c; main has not touched `deploy/vpp` since then).
Everything ran in fake-host mode. `apply-startup.sh --apply` was never run against real paths. VPP was not restarted,
nothing under `/etc` or `/root/ngfw` was written, and no process was killed by pattern. The only real-host reads were
`systemctl show vpp -p …`, `ls -l /etc/vpp/startup.conf` and `command -v driverctl ifup networkctl netplan`. The extra
break scenarios ran from a scratch copy of the harness in `/tmp/g-td6r/x`, outside the repo.

## What I ran

| check | result |
|---|---|
| V1–V4 + V8 against **main's** script: `VRX_TEST_APPLY_SCRIPT=<git show main:deploy/vpp/apply-startup.sh> VRX_TEST_ONLY="32 33 34 35 36 40"` | **4 passed, 13 failed**, wall 472 s, load 102 → 18. 32: the second apply committed, then run 1's late dead-man reverted it (`finished=[rolled-back] live==orig=y`, X4 reproduced). 33/34: `finished=[] vpp=inactive locks-free=n timer-cancelled=n`. 35: `finished=[] locks-free=n` for both the transient and the persistent `show` failure. 36: `committed` with `unit.restart: … MainPID=1051 NRestarts=1`. 40: the fe80 nudge check fails. **But 40's `bounded` check passes on main's script: `ok bounded: 26s (limit 27s … at load 17.86)`** (see F2) |
| the same scenarios against the **branch** script | **17 passed, 0 failed**, wall 553 s, load 44 → 15. States: 32 `superseded`, VPP untouched. 33 `console-needed`, `vpp=active`, never stopped. 34 `console-needed`, VPP started again. 35 `committed` / `rolled-back`. 36 `rolled-back`. 40: `bounded: 7s` |
| `TMPDIR=/tmp/g-td6r VRX_CI_CACHE_DIR=/tmp/g-td6r/cache tools/ci.sh --base main` | **CI GATE PASSED**, rc 0, quick, wall 19m45s (load 44 at the start, peaks above 100 from other agents, 18 at the end), logs `/root/ngfw-wt/logs/ci/TD-6-20260924-152748-706813`. The harness really ran: `green (4 shards; 128 checks passed in the parallel run)`, 7m35s, the same 128 as the pasted run. In this run scenario 40's limit was `28s … at load 19.14`, above main's measured 26 s |
| harness cache | The key of this tree (`018b7837…`) equals the entry that the worker's 15:14 run left in `~/.cache/vrx-ci/apply-startup/`. A plain `tools/ci.sh --base main` would therefore **skip** the harness, so I ran with a fresh `VRX_CI_CACHE_DIR`. The key really is content-only: `go version -m` shows no `vcs.*` stamp, and a scratch build gave the same key. The manager's premise "runs only when deploy/vpp changes" holds, apart from F3 |
| pasted CI in TD-6.md (at 85a6ed5) | matches `/root/ngfw-wt/logs/ci/TD-6-20260924-150235-607258`: shard results 25 + 21 + 33 + 49 = 128 passed, 0 failed |
| shellcheck 0.11.0 | `deploy/vpp/*.sh` (`-x -P SCRIPTDIR`): all 5 clean. `tools/ci.sh`: 12 info-level SC2001/SC2015 findings, the same count as on main. All of them are on pre-existing lines, and none falls in `do_deploy_vpp` (504–573) |
| scope / contract | `git diff --name-only main...task/TD-6`: the two scripts, `docs/agent/renderers/vppstartup.md`, `docs/status/tasks/TD-6*` and `tools/ci.sh`. In `tools/ci.sh` the changes are one header comment line, `do_deploy_vpp` and its call. `v19_preflight` is untouched. There are no hits under `packages/schema`, `packages/proto`, the generated code, `apps/agent/binapi` or `tools/binapi-gen.sh` |
| `git merge-tree --write-tree main task/TD-6` (main a8bc1f9) | rc 0 |
| extra X42/X43 (scratch harness): two `--stage rollback` of the same work dir at once | **both FAIL**: `stops=2 starts=2` with the holder gone and with the holder alive (F1) |
| bash 5.3 EXIT trap on SIGTERM (scratch) | the trap runs at once (the foreground child is orphaned), but `$?` inside it is 0 (F7) |

## V1–V9 / L1 against the verify review

| # | verdict |
|---|---|
| V1 | **Fixed.** Both the planner (`:1027-1031`) and the run under the locks (`:868-869`) refuse while an installed work dir has no result. `claim_current` (`:664-678`) records `current` and disarms dead-men that never installed. `not_ours` (`:679-689`) makes a stale dead-man write `superseded`/`console-needed` without touching VPP. Main fails scenario 32 and the branch passes it. The recovery path that the refusal prescribes is racy → F1 |
| V2 | **Fixed.** Both files are staged in the same directory before the install (`:897-899`), so install and restore are renames. If the old file cannot be staged, VPP is not stopped (`:720-731`). If the rename and the copy both fail, VPP is started again anyway (`:736-752`). `rollback_steps \|\| true` (`:709`) turns errexit off, so the tail (`:711-714`: marker, timer, locks) always runs. Main fails 33 and 34; the branch passes both |
| V3 | **Fixed.** `read_unit_restart` (`:212-227`) makes 3 tries, and a failure means an immediate rollback. The `run_exit` trap (`:839-851`) covers other unexpected exits after the install. I audited the post-install path of `stage_run` (`:919-960`) and found no other unguarded command. Main fails 35; the branch passes it |
| V4 | **Fixed.** The tuple must be complete, `NRestarts=0` and `MainPID≠0` (`:220-226`). On vrx-a `systemctl show vpp` gives `Restart=always RestartUSec=100ms NRestarts=0`, which is consistent with the reset-on-explicit-restart premise. Main fails 36; the branch passes it |
| V5 | **Fixed, with a residual (F5).** `secure_locks` (`:504-518`) is shared by the run's rollback and the dead-man |
| V6 | **Fixed** for what the verify review showed (`realpath -m`; root `/` refused; real startup.conf, sysfs and systemctl refused). The guard is still incomplete for mutating tools (F4) |
| V7 | **Fixed.** `hold-until` (`:891`, `:495-501`). Scenario 39 |
| V8 | **Fixed in the script** (`:293-300`, `:313-318`). **The test for the time bound is not a regression test under load** (F2) |
| V9 | **Fixed** (`:443`, plus the unfinished-apply line `:445-446`) |
| L1 | **Done** (`tools/ci.sh:504-573`). The rerun cannot mask a deterministic failure: a rerun runs exactly the scenarios parsed from `== N.`/`  FAIL ` (all 41 `scen N` numbers match their headers), a FAIL line without a scenario fails the gate, and so does a shard without a result line. Minor gaps: F3, F6, F8 |

## Findings, ranked

### F1 — MEDIUM (fix before the first real `--apply`): the recovery that V1's refusal prescribes can run two rollbacks of the same apply at once
`apply-startup.sh:981-999` (`stage_rollback` excludes nothing per work dir). The refusal texts at `:869` and `:1029` and
`docs/agent/renderers/vppstartup.md:155` tell the operator to "finish it now with … `--stage rollback --work <dir>`"
while that work dir's dead-man timer is still armed.

The failure: the operator follows the refusal late in the dead-man window (≈30–60 min on vrx-a), and the timer fires
while the manual rollback runs (RB_BUDGET is several minutes). With the holder alive, both processes log "lock holder …
still owns the locks" and continue. With the holder gone, the second waits `--deadman-lock-timeout` for its twin, then
goes **FORCED** and continues without the locks. Both processes stop VPP, rename or copy the backup, and start VPP. On
the real host one rollback's `wait_api`/`check_mgmt` can land in the other's stop, which gives a spurious
`console-needed` next to `rolled-back` plus a second data-plane outage. Reproduced (scratch harness, branch script):
```
== 42. X: … overlap (holder gone)
    stops=2 starts=2 markers=rolled-back
    | dead-man: lock holder gone — took the locks exclusively before touching VPP
    | dead-man: FORCED — locks held by someone else for 1s (…lslocks: … lab.lock)     ← its own twin
== 43. X: same overlap while the holder is alive (run hung in systemctl restart)
    stops=2 starts=2 markers=rolled-back
    | dead-man: lock holder 738765 still owns the locks            (twice)
```
Fix (a few lines + one scenario):
- At the top of `stage_rollback`, before `touch deadman-fired`, take a per-apply lock: `exec 7>>"$WORK/rollback.lock"; flock -n 7 || { say "a rollback of $WORK is already running — nothing to do"; exit 0; }`. Close fd 7 in `tmo`/`svc` like 8/9.
- Then `cancel_deadman`, which is harmless when the process is the timer's own service.
- Better still, have the refusal text say `systemctl start <deadman>.service` (systemd starts a running unit only once) instead of running the script by hand.
- Add a scenario like X42/X43 that asserts exactly one `systemctl stop vpp`.

### F2 — LOW/MEDIUM: scenario 40's `bounded` check stops detecting the pre-V8 count-bounded poll at load ≈ 18
`test-apply-startup.sh:840` (`bounded 10`) together with `load_limit` (`:85-87`). The comment at `:80-84` and the doc claim
that every limit stays below the unbounded case. That holds for the `sleep 300/600/1000` cases (×5.8 at most = 174 s <
300 s), but not here: the pre-V8 cost is only ~21–29 s. The limit is 10·(1+3·load/32), so it exceeds 26 s from load ≈
17 upward, and CI runs at loads of 17–45. Measured on main's script: `ok bounded: 26s (limit 27s = 10s idle-host
limit × load factor at load 17.86)`. The fe80 check in the same scenario still catches main today. A regression of only
the SECONDS deadline would not be caught.

Fix: make the check load-independent. Count the reads in the fake's call log:
`(( $(grep -c '^ip -j neigh show 10.0.0.1 dev ens192' "$T/calls") <= 2 ))`. With CT=3 the old code makes 2·CT = 6
reads and the new code at most 2. Alternatively run this sub-case with `--cmd-timeout 10`: old ≈ 20 × 10.5 s = 210 s
against a limit of at most 25 × 5.8 = 145 s. Also correct the "always below the unbounded case" sentence in the
comment and in `vppstartup.md`.

### F3 — LOW: the harness cache key misses the harness's fixtures
`tools/ci.sh:528` hashes `deploy/vpp/*` and the generator binary. The harness also reads `FIX =
apps/agent/internal/renderers/vppstartup/testdata` (`test-apply-startup.sh:29`: `host-startup.conf`,
`plugins-vrx-a.txt`, `cases/six-nic-sample.json`). These files are not embedded in the binary (`go:embed` covers only
`templates/*.tmpl`). A branch that edits them gets "unchanged since a green run — skipped", and because the cache is
shared by every worktree, so does main. Fix: add `find apps/agent/internal/renderers/vppstartup/testdata -type f` to the
hashed list, or hash exactly the three files.

### F4 — LOW: the V6 guard still lets real mutating tools run under a test root
`apply-startup.sh:121` requires startup.conf, sysfs, systemctl, systemd-run, ip, the state dir and the locks to be
inside `VRX_TEST_ROOT`. It does not cover `VRX_DRIVERCTL`, `VRX_IFUP`, `VRX_NETWORKCTL` and `VRX_NETPLAN`, whose
defaults are the real tools, and the host has `/usr/sbin/driverctl`, `/usr/sbin/ifup` and `/usr/bin/networkctl`. The
harness sets all four to fakes, so nothing is exposed today. But a future scenario or ad-hoc test that forgets one gets
a real `driverctl unset-override 0000:0b:00.0` (the fixture uses vrx-a's real management PCI address) or a real
`ifup --force ens192` from `rebind_drivers`/`restore_mgmt` as soon as the fake `ip` reports the interface broken. Fix:
add those four (and `VRX_ETC`) to the loop. A value that resolves to a missing file inside the root, such as the
harness's `netplan.absent`, still passes.

### F5 — LOW (V5 residual): after 60 s the run's holder-gone rollback is FORCED under a running integration test
`secure_locks` (`:512-517`) with the default `--deadman-lock-timeout 60`. For the run's own rollback the only reason is
"the lock holder is gone": VPP and the management path passed the last health round, so nothing is urgent. A queued
`flock -s` integration run typically holds the lab lock for minutes, so in practice V5 still ends as a FORCED VPP
stop..start under a running test. Scenario 37 passes only because its waiter holds the lock for 6 s against
`--deadman-lock-timeout 20`. Fix: when the reason is holder-gone, wait up to `LOCK_TIMEOUT` (600 s). RB_BUDGET and
HOLD_MAX then need that term. Or document that FORCED is expected in this case.

### F6 — LOW: nothing bounds the harness in CI
`tools/ci.sh:540` and `:559` run the shards and the rerun with no `timeout`. A regression that really hangs, such as an
unbounded wait in the script or a fixture that blocks, stalls `tools/ci.sh` (and the manager's `full` barrier behind it)
instead of failing it. The Go integration step has `-timeout 30m`. Fix: `timeout -k 30 1800` in front of
`deploy/vpp/test-apply-startup.sh` in both places. The shard then has no result line → "died" → FAIL.

### F7 — LOW: the `run_exit` path has no scenario, and two details are off
`:839-851`, `detach :830`. (a) No scenario sends SIGTERM to the run after `installed`, which is what `systemctl stop
<run unit>` does, to show the trap's rollback and the release of the locks. (b) After a fatal signal, bash gives the
EXIT trap `$?` = 0, so the log says "stopped unexpectedly (exit 0)". (c) The run unit keeps systemd's default
`TimeoutStopSec` of 90 s, and with `KillMode=process` the trap's rollback (up to RB_BUDGET) is SIGKILLed after 90 s.
The dead-man then makes a second rollback. Fix: add a scenario; give the run unit `--property=TimeoutStopSec=<default-count
RB_BUDGET>` (or `infinity`); log "signal" when `rc` is 0 in the trap.

### F8 — nit: two rerun/cache details
- A result that went green only on the serial rerun is cached (`:571`). Later cache hits print that note with `say`
  (`:535`), not `warn`, so the flake is visible exactly once. Either do not cache when `note` is set, or `warn` on a
  hit whose entry says "green only on the serial rerun".
- The rerun is accepted on its exit code alone. Also require its result line to show `P > 0 passed, 0 failed` and every
  number in `$again` to have a `== N.` header in the rerun log. The scenario numbering matches today; this only guards
  against drift.

### F9 — nit: rollback edge cases
- When nothing was placed (the run died, or `claim_current` failed, between `installed` and the rename), the live file
  equals `backup.conf`. `not_ours` then returns 1 and `rollback_steps` stops and starts VPP on an identical file: a
  needless outage. You could skip it when `live == backup` **and** the boot identity still equals `ident.before`.
- If the in-place fallback `install` (`:737`) fails half-way on a full disk, GNU `install` has already replaced the
  live file. The `console-needed` text "VPP was started again on the new file" is then wrong. Print the live sha256
  instead.

### Pre-existing, not TD-6 (tech-debt)
- The rollback's final verification (`:753-755`) never checks that VPP really restarted, i.e. that its boot identity
  differs from the failed instance's. If `svc stop` and `svc kill` both fail, `start` is a no-op and the old instance
  can pass `check_mgmt`, so the result is `rolled-back`.
- The harness's `kill_recorded` sends `kill -KILL` to every PID recorded by earlier cases, including ones reaped long
  ago. With `pid_max` 4194304 that is unlikely to hit a recycled PID, but it is not "only PIDs you spawned".

## Manager answers already taken
- **Load-average scaling:** keep it. It is bounded (×5.8 at most) and printed on failure, and it cannot turn a
  `sleep 300–1000` hang into a pass. The exception is scenario 40, where the "hang" being guarded against is only ~3×
  the fixed time. Fix that check (F2).
- **4 shards + cache:** correct. The key is content-only, as verified above: an unchanged `deploy/vpp` skips the harness.
  The fixture gap is F3.

## Verdict
V1–V4 (and V5–V9) are fixed. Each fails on main's script and passes on the branch. I re-ran 32–36 and 40 on both scripts
myself. L1 is wired, shellcheck is clean, and the scope is correct. F1 is on the recovery path that the V1 refusal sends
the operator down, and the fix is a few lines: it should land before the first real `--apply`, together with F2's
one-line test fix. F3–F9 can go to tech-debt.

**APPROVE WITH CHANGES**
