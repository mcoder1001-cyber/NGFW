# F-startup-apply — verify review of fix round 2 (independent review agent, 2026-09-24)

Branch `task/F-startup-apply` @ 5e0d8fe (fix 028eda4 salvage + 2d622eb, evidence 5e0d8fe, merge of main 0f27b57), against
the re-review `F-startup-apply-rereview.md` (3c0bafc: N1–N7). Everything below ran on the host **read-only**:
`apply-startup.sh` was never run with `--apply` against real paths. VPP was not restarted, no NIC was bound or unbound,
nothing was written under `/etc`, and `/root/ngfw` was only read (the gate's `git show main:`). The break attempts
ran from a scratch copy of the fake-host harness outside the repo.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` (slot 6) | **CI GATE PASSED**, quick, wall 3m40s, logs `/root/ngfw-wt/logs/ci/F-startup-apply-20260924-073553-3230429`. Matches the pasted run (3m46s). The only warning is about the reviewers' two `review(...)` subjects |
| `shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh` | exit 0 |
| `go test -run TestBootID -v ./cmd/vrx-vppcheck` | PASS (an incomplete identity now exits 1) |
| `deploy/vpp/test-apply-startup.sh <branch-built vrx-startupgen>` | **`apply-startup tests: 101 passed, 0 failed`**, wall 483 s. Matches the pasted 101 |
| contract / binapi | `git diff --name-only main...HEAD` touches only owned files: the two scripts, `cmd/vrx-vppcheck`, the renderer doc and the status files. It touches none of `packages/schema`, `packages/proto`, the generated code, `apps/agent/binapi` or `tools/binapi-gen.sh` |
| read-only dry run on vrx-a (branch binaries, `--i-have-product-owner-approval PENDING-handover`) | rc 0, wall 12 s. Output: ens192 ifupdown, exact restore plan (`addr replace 172.30.126.195/24 …`, `-4 route replace default via 172.30.126.1 dev ens192 onlink`), `will use: neigh (passes now)`, session with 172.30.126.196 shown as a signal only, preflight `present: local0`, identity `b7712a53-…/3181292/6750698`, and the gate line `REFUSED: … no D-row on main answering PENDING-handover names this rendering (sha256 e0b5945e…)`. The already-executed D-060 approval no longer covers a new change |
| extra break scenarios X1–X5 (scratch harness, below) | 4 new holes reproduced (V1–V4) plus one design gap (V5) |

## Re-review findings

| # | verdict | evidence |
|---|---|---|
| N1 ssh-peer → rollback loop + false "console needed" | **FIXED** | `ssh-peer` is gone. `auto` = `neigh`: every snapshotted default gateway must be REACHABLE after a nudge (`:269-281`). `session_signal` is informational only (`:306-312`). The rollback is one attempt; if it is not healthy → `console-needed`, the timer is cancelled and the locks are released (`:571-599`). The dead-man does nothing once any result marker exists (`:778,783`). Worst case per apply is 1 restart + 1 run rollback + at most 1 dead-man rollback (only if the run's rollback was interrupted). Scenario 11 (session closes → commits, 1 restart) and scenario 13 (path stays dead → 2 starts, late dead-man is a no-op) both pass. On vrx-a `neigh` passes now (ARP answered although ICMP is dropped) |
| N2 setsid fallback frees the locks | **FIXED** | No fallback. The run unit, the holder unit and the timer each need `systemd-run`, otherwise exit 3 before any change (`:421-424`, `:662-664`, `:718-721`, `:825`, dry run `:403`). The holder is a separate unit, so it is never among the run's descendants that `kill_run` walks. The holder's wait is `sleep 1 … \|\| true` (`:451`). Scenarios 25 and 26 pass |
| N3 crash before the first identity read | **FIXED, with a narrow residual (V4) and a new `set -e` hole right after the restart (V3)** | `unit.restart` is read right after the restart job (`:732`). After the settle time the unit tuple must be unchanged and the identity new, complete and equal to MainPID (`:484-494`). Every window round compares against the same tuple (`:498`), with at least 2 reads (`:744`). Scenario 10a/10b pass |
| N4 approval not bound to the change | **FIXED** | The decision column must *start* with the PENDING id (`:340-343`) **and** the row must contain `--expect-new-sha256` (`:344-345`). A committed work dir whose gate names the same PENDING and sum makes the approval spent (`:346-351`). A used work dir cannot be replayed (`:680`). The dry run evaluates the gate against its own rendering (`:401`). Scenario 4 (mention-only, other change, spent) and scenario 15 (replay) pass. Real host: `PENDING-handover` is refused. Acceptable by design: an approval stays usable after a *rolled-back* apply, and the gate is re-evaluated before the locks are taken, but `--expect-sha256` inside the lock stops a second concurrent planner |
| N5 run never checks the holder | **FIXED** (residual → V5) | `holder_alive` runs before install (`:725`), in every round (`:496`) and before commit (`:752`). Scenario 27 passes |
| N6 budgets / `--window 0` | **FIXED** (residual → V7) | Budgets scale with the drivers, interfaces, plan lines and gateways (`:602-616`). `WINDOW ≥ INTERVAL > 0` (`:157`) |
| N7 lows | **FIXED** | Loopback or off-management TCP targets are refused (`:290-291`, scenario 2). `bootid` exits 1 when the identity is incomplete (TestBootID). The harness uses a fixture canonical repo (`VRX_TEST_ROOT`). The fixture guard has a hole → V6 |
| L1 harness + shellcheck in `tools/ci.sh` | **OPEN** (manager-owned file) | `grep test-apply-startup tools/ci.sh` → nothing |

## New findings (ranked)

### V1 — LOW/MEDIUM: the armed dead-man of a dead run later reverts a *different*, committed apply
`stage_rollback :779-801` (no check that the live file is still this work dir's install), `rollback :579`

The failure: a run dies after `installed` (OOM, `systemctl stop` of the run unit, SIGKILL). Its holder keeps both locks,
and its timer stays armed for `DEADMAN_AFTER` (≈1940 s ≈ 32 min with the defaults on vrx-a). An operator who "unsticks"
the locks by stopping the holder unit and re-applies gets a commit. Then the old dead-man fires: it finds the locks free,
"takes them exclusively", sees `installed` in *its* work dir, installs *its* `backup.conf` over the newer committed file
and restarts VPP. The router silently reverts to the configuration from before both applies, and the log says
"rolled back". Reproduced (X4):
```
run1 rc=137 installed=y finished=[] holder1-alive=y          # run killed during the restart; operator kills the holder
run2 rc=0 finished=[committed ] live==new2=y                 # a new plan (expect-sha256 = run1's file) commits
dead-man1 rc=1 finished=[rolled-back ] live==new2=n live==orig=y extra-vpp-starts=1
  dead-man: lock holder gone — took the locks exclusively before touching the run
  ROLLBACK: dead-man: the apply did not finish within 402s
```
The same thing happens on the FORCED path, and there without the locks.

Fix (small):
- The dead-man rolls back only if `sha256(live) == sha256($WORK/new.conf)`. If the live file already equals
  `backup.conf`, there is nothing to restore. Otherwise it writes `superseded` / `console-needed` and changes nothing.
- The planner refuses `--apply` while any work dir is `installed` but has no result marker. It names that dir and its timer unit.
- The doc gets one line on how to abort a stuck apply: stop `<deadman>.timer`, then the holder.

### V2 — LOW/MEDIUM: if the rollback cannot write startup.conf, VPP is left stopped, with no marker and the locks held
`rollback :575-579`: `svc stop vpp` runs, then the unguarded `install -m 0644 backup.conf …` runs under `set -e`.

If `install` fails (ENOSPC on `/` during the window, or a read-only `/etc/vpp`), the rollback exits after stopping VPP.
Nothing starts VPP again, and no `console-needed` or `rolled-back` marker is written. The timer is not cancelled, and the
holder keeps both locks until `HOLD_MAX` (~65 min, CI blocked). The dead-man then repeats the same abort. Reproduced (X3b:
the restart hook removes `lan2` and makes `/etc/vpp` a plain file):
```
run rc=1 finished=[] vpp=inactive holder-alive=y locks-free=n timer-cancelled=n
  install: invalid target '…/etc/vpp/startup.conf': No such file or directory
dead-man rc=1 finished=[] vpp=inactive holder-alive=y locks-free=n
vpp start calls after the apply's restart: 0
```
On vrx-a the management NIC stays kernel-owned, so SSH survives, but the data plane is down with no alarm.

Fix: guard the restore: `install … || cp … || restore_failed=1`. Always continue to `reset-failed` + `start`. If the
restore failed, write `console-needed` naming it. `cancel_deadman` + `release_locks` must always run: an `ERR`/`EXIT` trap
inside `rollback`, or no `set -e` there.

### V3 — LOW: a failing `systemctl show` right after the restart kills the run under `set -e`; the new file then stays unverified until the dead-man
`stage_run :732` runs `unit_ident > "$WORK/unit.restart"` unguarded. `unit_ident` is `tmo … | sort | tr` with `pipefail`.
A D-Bus timeout (or any non-zero exit) of `systemctl show` exits the run between the restart and the first check. No
rollback runs. VPP keeps the new, unverified file and both locks stay held until the dead-man fires (≈32 min with the
defaults). Reproduced (X2):
```
run rc=1 finished=[] installed=y live==new=y vpp=active holder-alive=y locks-free=n
last log line: … installed …/startup.conf; restarting VPP
dead-man rc=1 finished=[rolled-back ] live==orig=y locks-free=y   # only when the timer fires
```
Fix: `unit_ident > "$WORK/unit.restart" || true`. An empty or garbled baseline then fails the settle comparison, which
leads to an immediate rollback. Audit the rest of the post-install path for other unguarded commands.

### V4 — LOW: N3 residual: a crash plus systemd auto-restart that finishes before `unit.restart` is read is accepted
`:732`, `vpp_identity_ok :491`. The code compares the tuple for *equality*. It never asserts what the doc and the status
file claim ("NRestarts is 0 then"). If VPP dies within the first moments and systemd (`RestartUSec=100ms`) has already
restarted it when `systemctl show` runs, the baseline is `NRestarts=1` and the second instance is accepted. Reproduced (X1):
```
rc=0 finished=[committed ] unit.restart=[ActiveEnterTimestampMonotonic=78009 MainPID=1051 NRestarts=1 ]
```
The window is narrow: the crash has to happen before the `show` call, and it is ~100 ms after `restart` returns. A
deterministic startup crash is still caught, because it repeats in the window. Fix (one line): refuse unless
`unit.restart` contains `NRestarts=0 ` and `MainPID` ≠ 0; otherwise roll back.

### V5 — LOW: after the holder dies, the run's own rollback restarts VPP without any lock
`check_health :496` → `rollback :750`. The rollback takes no lock. After the holder is gone, a queued `flock -s` integration
run gets the lab lock, and the rollback stops and starts VPP under it. Reproduced (X5, the `calls` log in order):
`WAITER-IN` (16) → `systemctl stop vpp` (22) → `systemctl start vpp` (29) → `WAITER-OUT` (40). N5 asked for "re-take or
roll back". Rolling back is right, but it should first take the locks the way the dead-man does (`:789-794`: exclusive
and bounded; FORCED is logged only on a foreign holder). Reuse that block.

### V6 — LOW: the `VRX_TEST_ROOT` guard is lexical, so a deliberate env setting points the gate at an arbitrary "canonical repo"
`canon_root :113-120` checks `[[ $p == "$VRX_TEST_ROOT"/* ]]` without resolving paths. With `VRX_TEST_ROOT=/.` and
`VRX_STARTUP_CONF=/./etc/vpp/startup.conf`, `VRX_SYSFS=/./sys`, `VRX_SYSTEMCTL=/./usr/bin/systemctl` and
`VRX_SYSTEMD_RUN=/./usr/bin/systemd-run`, every real path passes: `canon_root → /./canon`. `/etc/..` works the same way;
checked by sourcing the script and calling the function. The handover flag and PENDING/LOG are then read from `/canon`,
and the planner-tree source is skipped when a test root is set (`:326`). It needs root and intent, so it is not an
accident, but the gate is meant to be the one thing a worker cannot talk its way past. Fix: `realpath -m` every path and
`VRX_TEST_ROOT` before the prefix test. Refuse a test root that resolves to `/`, and refuse when the resolved startup.conf
is `/etc/vpp/startup.conf` or the resolved sysfs is `/sys`.

### V7 — LOW: the holder's `HOLD_MAX` uses the default counts, not this host's
The holder computes `HOLD_MAX` in `load_settings → budgets` before `drivers`/`mgmt.ifs` exist (`:604-607`, `:638`, `:450`).
The run recomputes the budgets after the snapshot (`:704`) and arms the dead-man with them. With 1 management interface
the holder's 3911 s covers the dead-man's worst case (3203 s). With 3 management interfaces the dead-man's worst-case end
is 4587 s, while the holder still expires at 3911 s. The locks can then go free during the dead-man's rollback (computed by
sourcing `budgets` with synthetic work dirs). Fix: after `budgets`, the run writes `hold-until`, and the holder re-reads it
on every loop.

### V8 — LOW: `neigh` probe details
- `:272` sends the nudge with `/dev/tcp/$gw/9`. For an IPv6 link-local gateway (`fe80::…`, the usual RA default) the
  address has no scope, so `connect` fails and nothing is sent. REACHABLE then depends on other traffic. On dual-stack
  hosts the result is a refusal, or a false rollback followed by `console-needed`. Use `"$gw%$dev"` for `fe80::/10`.
- `:274` bounds the poll by iteration count, not by time. If `ip neigh` hangs (rtnl contention), one gateway costs
  20 × (`CMD_TIMEOUT`+2) = 240 s, while `ITER` assumes 3 × 12 s. Use a `SECONDS` deadline.
- Information: on vrx-a the gateway drops the SYN, so every health round spends the full `--cmd-timeout` (10 s) in the
  nudge (the dry run took 12 s wall). That is within `ITER`, but the doc could say it.

### V9 — nit: the dry run exits 0 when the gate would refuse
`:402` runs `gate | sed … || true` and never sets `rc=3`. The header (`:15`) says "Exit 3 when --apply would refuse". The
real dry run with `PENDING-handover` printed `REFUSED` and exited 0. Fix: `gate … || rc=3`, or reword the header and doc
step 2.

## Scope / security / rules
- Scope: only owned files changed. No new dependencies. No VPP API name outside the generated bindings.
- No user input reaches a shell. The document goes only to the Go generator. Names, plugins and PCI ids taken from the
  rendering are regex-limited before use as argv. Restore-plan tokens are whitelisted. Settings with a newline are
  refused. `SSH_CONNECTION`'s peer is appended after `parse_args` validation, but it comes from sshd and is only used as argv.
- No secrets in files or logs. Nothing kills processes by pattern. `kill_run` checks `/proc/<pid>/cmdline` before it kills.

## Summary
Fix round 2 does what the re-review asked. N1: the verdict no longer depends on the operator's session, and VPP is never
restarted in a loop. N2: no fallback, and the holder is a separate unit. N4: the approval is bound to one rendering,
cannot be spent twice, and the real `PENDING-handover` is refused. CI and the 101-check harness reproduce.

What remains is on rare paths, but on an unattended router they matter. V1 (a stale dead-man reverts a later commit) and
V2 (a failed restore leaves VPP stopped with the locks held) are each a few lines, and V3/V4 are one line each. **They
should land before the first real `--apply`.** None of this blocks merging, because nothing can apply on vrx-a until an
approval that names a rendering exists (handover is pending, and D-060's approval is refused as shown).

**APPROVE WITH CHANGES**
