# F-nat46-host — questions / blockers

## Q1 (blocker, host incident): VPP on ngfw-a is hung since 2026-09-28 19:13:04 — CLI and binary API unresponsive

Found at task start (2026-09-29 07:57). Facts (read-only, all pasted from the host):

```
$ systemctl is-active vpp; systemctl show vpp -p NRestarts -p ActiveEnterTimestamp
active
ActiveEnterTimestamp=Mon 2026-09-28 14:13:14 +0330
NRestarts=2
$ journalctl -u vpp --since '2026-09-28 19:13:00' --no-pager | tail -1
Sep 28 19:13:04 ubuntu-26.04 vpp[1847057]: os_panic() called, aborting.
$ tail -c 200 /var/log/vpp/vpp.log            # last CLI commands VPP logged before the panic
2026/09/28 19:12:26:804[0]: show map domain
2026/09/28 19:12:26:812[0]: show map stats
2026/09/28 19:12:26:812[0]: quit
$ ps -o pid,stat,pcpu,etimes,wchan -p $(pidof vpp)
    PID STAT %CPU ELAPSED WCHAN
1847057 Rsl  73.4   63987 -                    # alive, main thread spinning, no core, no restart
$ ps -eo pid,etimes,cmd | grep '[v]ppctl'
2776596     450 S    vppctl show int           # slot 16's worker, stuck since 07:51:59
2829627     136 S    vppctl show map domain    # mine, stuck (killed by me afterwards)
$ /tmp/g-w17/bin/ngfw-vppcheck --timeout 20s version      # binapi with a deadline (never vppctl)
ngfw-vppcheck: cannot reach VPP at /run/vpp/api.sock: waiting for VPP at /run/vpp/api.sock: context deadline exceeded
exit=2
```

`os_panic()` was called right after a `show map stats` (19:12:26) — the last two CLI commands were `show map domain` / `show map stats`
(not mine: my task started 07:52 the next morning; the slot-16 row F-det44-map-dslite-cnat-host runs MAP steps in parallel). The process
did not exit (no coredump, `NRestarts` still 2), so every vppctl and every govpp connect blocks forever. Until the manager restarts VPP
(D-012 handover rule: only the manager, under `flock -x /run/lock/ngfw-vpp.lock`, then the D-203 sentinel host check), **no host step of
this row can run**: `TestNat46OnHost`, the af_packet packet test, the restart simulation and the rollback are all against this VPP.

What I did instead: the complete evidence driver `docs/status/tasks/F-nat46-host-evidence/host.sh` (agent level only — slot 17 has no
Valkey database, envelope) is written and syntax-checked; it runs every owed step in one shared-lock window as soon as VPP answers.
I poll VPP (`ngfw-vppcheck --timeout 20s version`, deadline-bounded, never vppctl) inside my time box and run the driver the moment it
answers. If the time box ends first, the status file says so and the driver is the hand-over.

Ask: restart VPP (manager window), re-run the classify sentinel check (TD-H21), then re-launch this row (or run
`eval "$(tools/lab env 17)"; docs/status/tasks/F-nat46-host-evidence/host.sh run` yourself — ~3 min, pastes into `-evidence/`).

## Q2 (owed section item that cannot be done on slot 17): API e2e + NAT46 tab screenshot

`docs/status/tasks/F-nat46.md` "Remains (host)" also lists the API e2e on the host PostgreSQL and a screenshot of the NAT46 tab.
The envelope says slot 17 has no Valkey database → Go/agent level only, no API stack; D-175 says screenshots for -host rows are
taken by T4 on the integrated main stack after merge. Both stay owed to the manager (T4 / a slot with a Valkey db); the agent-level
duplicate-IPv4 refusal (dryrun with `pointer`) stands in for the 400 problem+json check here.

## Q3 (D-210, tests off)

Received mid-task: "TESTS OFF — stop writing or running tests; host evidence commands are feature output, keep them". Read as: no new
test code (none was planned — `files_owned` is `docs/status/tasks/F-nat46-host*` only), the existing `TestNat46OnHost` run stays as the
owed host evidence (step 1 of the driver), everything else is the real agent + `ngfw-agentctl` + rig packets (feature output).
