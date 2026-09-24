# F-startup-apply — review (independent review agent, 2026-09-24)

Branch `task/F-startup-apply` @ 00315f4, base main. Run on the host, read-only: `apply-startup.sh` was **never** run with
`--apply` against real paths; VPP was not restarted; no NIC was bound or unbound; nothing was written under `/etc`. Probes
reused the fake-host harness from a scratch copy (outside the repo). The only real-host actions: `vrx-vppcheck` read-only
calls, `ip -j addr/route` reads, one `ping` to the gateway, and one throwaway transient unit (`systemd-run
--unit=review-nrestarts-<pid> /bin/sh -c 'sleep 0.3; exit 1'`, stopped and collected) to check NRestarts semantics.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` | **CI GATE PASSED** (quick, wall 3m22s, logs `/root/ngfw-wt/logs/ci/F-startup-apply-20260924-034940-2573389`) — matches the pasted run |
| `shellcheck deploy/vpp/apply-startup.sh deploy/vpp/test-apply-startup.sh` | clean (exit 0) |
| `deploy/vpp/test-apply-startup.sh` (builds the generator) | `72 passed, 0 failed` (4m05s) — matches |
| `vrx-vppcheck` against the real VPP | `version` → `vpp 26.06-release` 0; `ifaces local0` → present 0; `ifaces local0 wan` → `missing: wan` 1; `ifaces loc` → `missing: loc` 1 (exact match, not substring); `plugins` → 87 rows; `--socket /run/vpp/cli.sock --timeout 2s version` → exit 2 after 2.02 s |
| contract / binapi | no diff under `packages/schema`, `packages/proto`, generated code, `apps/agent/binapi`, `tools/binapi-gen.sh`; the checker uses only generated bindings (`vpe.ShowVersion`, `vlib.CliInband`, `interface.SwInterfaceDump`) |
| security grep | no `exec.Command` in `vrx-vppcheck`; no `eval` in the script; settings loaded with `printf -v`; restore plan token-checked before use |
| committed `.pyc` / `vpp-iface-check.py` | gone (`git ls-files deploy/vpp` = the two scripts) |
| scope | only owned files touched |

## Original re-review findings (F-startup-gen-rereview.md)

| # | verdict | evidence |
|---|---|---|
| N1 hung VPP / no timeouts | **FIXED** | every call through `tmo`/`svc`/`vppcheck` (`apply-startup.sh:141-145`) with lock fds closed; checker has ctx deadline + hard `os.Exit` backstop (`main.go:114-121`); dead-man kills the run (`:550-566`), bounded lock wait, rolls back. Fake scenarios 5, 15, 16, 17, 20 pass. But the "rolls back without the lock" choice creates M1 |
| N2 ifupdown mgmt restore | **FIXED** (restore) | exact `ip addr/route replace` plan from `ip -j` snapshot (`:194-218, 358-389`), then `ifup --force` / `networkctl reconfigure` / `netplan apply`; scenarios 8–11, 25. The *verification* of the path that follows the restore is broken on vrx-a — H2 |
| N3 checker cannot run on vrx-a | **FIXED** | Go binary on the agent's client; real read-only run above; preflight `ifaces local0` in the dry run and inside the lock (`:272, 493`) |
| N4 mgmt = default route only | **PARTIAL** | script side done: v4+v6 defaults, route to `$SSH_CONNECTION`/`--mgmt-peer`, `--mgmt-if`, device-less taps skipped (`:165-178`). Generator side (`hostfacts.go`) not in this task's files — still open; L2 |
| N5 rendering not pinned | **FIXED** | `--expect-new-sha256` compared inside the lock (`:490-492`); generator, checker, script and doc copied into `<work>/bin` (`:452-460`); scenario 4 |
| N6 D-084 omission rolled back | **FIXED** | exemption `:333-335`; scenario 13 |
| N6(env) env through systemd-run | **FIXED** | `<work>/settings` authoritative + `--setenv` (`:426-450`); scenario 18 runs the unit under `env -i` |
| N8 handover gate | **FIXED, with a hole** | caller refuses without `handover: done` or the approval flag (`:238-248, 604`); see M2 |
| N9 fallback dead-man holds locks | **FIXED** | `8>&- 9>&-` (`:525`), killed on commit; scenario 24 |
| N10 `.pyc` | **FIXED** | removed |
| N11 start-limit | **FIXED** | `reset-failed` before start (`:407`) |
| N7, N12 | not addressed (generator files, outside this task) | — |

## New findings (by severity)

### H1 — HIGH (blocking): every apply on vrx-a rolls back — `systemctl restart` resets `NRestarts`
`deploy/vpp/apply-startup.sh:504` (recorded before the restart), `:323` (compared after)

`NRestarts` is recorded **before** `systemctl restart vpp` and must stay equal afterwards. systemd resets `NRestarts` to 0
on an explicit start/restart job. Verified on this host with a throwaway transient unit (`Restart=always`, exits 1):
`NRestarts` 4 → `systemctl restart` → **0**. vrx-a's `vpp.service` has `NRestarts=4` today (D-087 crashes). So the first
`check_health` after a perfectly healthy restart reports "restarted by itself (NRestarts 4 → 0)" and rolls back (two VPP
restarts for nothing). Reproduced in the fake harness by modelling the reset (`echo 4 > nrestarts`; restart hook writes 0):
```
rc=1
ROLLBACK: vpp.service restarted by itself (NRestarts 4 → 0)
```
The harness keeps NRestarts constant across `restart`, which is why scenario 6 passes. Fix: record `NRestarts` **after**
`svc restart vpp` returns (before `wait_api`), or compare `ExecMainPID`/`ExecMainStartTimestampMonotonic` against the
values read right after the restart; model the reset in the fake `systemctl` and add the scenario.

### H2 — HIGH (blocking): the gateway on vrx-a does not answer ping → every apply rolls back, and the rollback ends "incomplete"
`apply-startup.sh:315-318` (`check_mgmt`), used by `check_health` (`:342`) **and** the rollback verdict (`:412`)

A management interface is healthy only if `ping -c 1 -W 2 -I <dev> <gw>` answers. On vrx-a the gateway 172.30.126.1 does
not answer ICMP (3/3 lost, `ip neigh` = `REACHABLE`, SSH works). Consequence on the real host: healthy apply → "gateway
does not answer" → rollback → rollback verification uses the same check → `rollback-incomplete` (dead-man left armed) →
the dead-man fires later, kills nothing, rolls back **again** (a third VPP restart), again "incomplete", and the manager is
told "console access may be needed" while everything is fine. Reproduced (fake `ping` exits 1):
```
ROLLBACK: gateway 10.0.0.1 on ens192 does not answer
ROLLBACK INCOMPLETE (gateway 10.0.0.1 on ens192 does not answer) — the dead-man stays armed and retries
```
The dry run never pings (P2b: no gateway line), so nothing warns before `--apply`. Fix: baseline the check — probe the
gateway in the dry run and in the snapshot; only require it after the restart if it answered before; use neighbour
reachability (`ip neigh` REACHABLE/`arping`) rather than ICMP; show the baseline in the dry run.

Together H1+H2 mean the tool cannot commit any change on the host it was written for — the same class as re-review N3.

### M1 — MEDIUM: the forced dead-man rollback restarts VPP under a concurrent integration run
`apply-startup.sh:578-584`, `take_locks :285-289`

The run holds `flock -x` on both locks; the dead-man first **kills** the run (which releases them) and only then asks
for the locks. Any waiter already queued — `tools/ci.sh full` (`flock -x -w`), a harness's `flock -s` (smoke_test), `tools/lab
rig` — wins the race, starts its tests, and after `--deadman-lock-timeout` (60 s) the dead-man stops/starts VPP under it:
exactly the D-087 failure mode (concurrent VPP disruption → SIGSEGV / false failures). Reproduced with a queued
`flock -s` waiter:
```
rc=1 it-running=yes it-done=
dead-man: FORCED — locks still busy after 3s; rolling back without them
systemctl stop vpp
```
Fix: never let the locks go free between the run and the dead-man — hold them in a separate holder (e.g. a
`systemd-run --unit=<stamp>-lock flock -x vpp.lock flock -x lab.lock sleep infinity`, or pass the fds to the dead-man)
that is released only after commit/rollback; the dead-man kills the run, not the holder. Keep FORCED only for a holder
that is not ours, and log the holder (`lslocks`).

### M2 — MEDIUM: approval flag is format-only, and `--stage run` does not re-check the gate
`apply-startup.sh:131-132, 242-245, 483`

`--i-have-product-owner-approval PENDING-anything` passes as long as it matches the regex — nothing checks that
`docs/decisions/PENDING-<slug>.md` exists and is answered, or that `D-nnn` exists in `LOG.md` (and on this project the
operators are agents, who could type it). `stage_run` accepts **any** non-empty `<work>/gate` file: reproduced — a work dir
with `gate` = `bogus` run through `--stage run` commits (`bogus … COMMITTED`). Fix: resolve the id against `/root/ngfw/docs/
decisions/` (PENDING file with an answer / LOG row) and record its sha; re-evaluate the gate inside `stage_run` after the
locks (handover state or a resolvable approval), not a free-text file.

### M3 — MEDIUM: the rollback can overrun the dead-man margin; "retries" is a single retry
`apply-startup.sh:516-518, 539, 397-423`

Margin between the run's budget and the timer is `iter + 30` s (≈155 s with defaults), but a rollback inside the run can
take `SVC_TIMEOUT` (stop) + kill + 10 s + `SVC_TIMEOUT` (start) + `API_WAIT` + checks ≈ 330 s, and the budget is only tested
after a whole `check_health` (up to several `CMD_TIMEOUT`s). A late failure therefore gets its rollback killed mid-way by
the dead-man, which starts over (idempotent, but another stop/start of VPP). The timer is one-shot: after the dead-man's
own rollback is incomplete nothing retries again, contrary to the log line and the doc ("it retries"). Fix: re-arm the
timer (or extend it) when the run enters rollback; budget = worst-case rollback; say "one retry" or loop with a bound.

### L1 — LOW: the fake-host test and shellcheck are not in the CI gate
`tools/ci.sh` runs neither `deploy/vpp/test-apply-startup.sh` nor shellcheck on it; a later edit can break the safety
scenarios silently. Add them to the gate (quick mode, ~4 min — or a trimmed timing).

### L2 — LOW: dry run and apply protect different interface sets
`apply-startup.sh:461` adds `$SSH_CONNECTION`'s peer only in `detach`; the dry run (the reviewed plan) does not include it.
Add it in `dry_run` too.

### L3 — LOW: small robustness items
- `netmgr_of` (`:184`) interpolates the interface name into an ERE; `.` in `ens192.10` matches any char. Escape it.
- `dry_run` leaks its `mktemp -d` when `set -e` exits mid-way (e.g. snapshot → `sha`); add a `trap`.
- Restore plan skips multipath and policy-routing tables (documented) and drops address flags (`noprefixroute`, lifetimes).
- Harness gaps behind H1/H2: fake `systemctl` never resets NRestarts, fake gateway always answers.
- `--foreground` is allowed from an SSH session; refuse it when `$SSH_CONNECTION` is set unless `--console` is given.

## Answers to the focus questions
- Order of operations: gate (caller) → detach → locks → sha(live) → render + sha(new) → preflight → snapshot/drivers/plugins/
  NRestarts → backup → dead-man → install → restart → checks → commit/cancel. Correct, except NRestarts timing (H1).
- Timer unit fails to start: the apply does **not** refuse; it falls back to a `setsid` dead-man without lock fds (scenario 24).
  Acceptable; a failed arm of both would still be caught only by `set -e`.
- Forced rollback under another lock holder: yes, it can disrupt a concurrent integration run (M1).
- Rollback completeness: file, VPP stop (SIGKILL on hang)/reset-failed/start, driver rebind to the recorded driver, exact
  mgmt address+routes, then ifupdown/networkd/netplan — complete for single-path mgmt; verification broken by H2.
- SSH death before handover: before `systemd-run` only the work dir exists (nothing changed); after it the unit/setsid run
  is independent (scenarios 18/19).
- Flag abuse: M2. Temp files/permissions: work dir 0750, doc 0640, root-owned; L3 tmp leak. Injection: none found.
- `set -euo pipefail`: health/rollback checks run in `if`/`$(…)` contexts with explicit returns; no swallowed error found that
  hides a failure, only `|| true` on best-effort restore steps (intended).
- vrx-vppcheck: healthy = `show_version` answers, `show plugins` has ≥1 row, every name exact-matches `sw_interface_dump`;
  deadline on connect + requests + hard exit. Hang *after* connect is not unit-tested (acknowledged), backstop covers it.

**BLOCK**
