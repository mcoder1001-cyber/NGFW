# F-ospf-host — questions / blockers for the manager

## Q1 (BLOCKER, host-wide): the shared VPP (pid 1847057) is wedged — main thread spinning since ~19:12:27 on 2026-09-28

Found on the CONTINUE after the D-208 outage (2026-09-29 07:55). Not restartable by a worker (D-012 handover pending; only
the manager under `flock /run/lock/ngfw-vpp.lock`).

```
# systemctl is-active vpp; systemctl show vpp -p NRestarts -p ActiveEnterTimestamp
active
ActiveEnterTimestamp=Mon 2026-09-28 14:13:14 +0330
NRestarts=2
# top -b -n1 -H -p 1847057            (07:55:35, 17 h 42 min after start; 12 h 59 min CPU = spinning since ~19:13 the day before)
    PID USER      PR  NI    VIRT    RES    SHR S  %CPU  %MEM     TIME+ COMMAND
1847057 root      20   0  273.2g 449624  33540 R  99.9   0.5   12,59 vpp_main
1847399 root      20   0  273.2g 449624  33540 S   0.0   0.5   0:00.00 dpdk-in+
# awk on /proc/1847057/task/1847057/stat → state=R utime=1063683 stime=3619844   (mostly SYSTEM time: a syscall loop)
# timeout 40 vppctl show version        → rc=124 (hangs; cli.sock connects, no answer)
# timeout 10 vpp_get_stats ls           → rc=143 (the stats segment lock is held by the spinning main thread)
# ngfw-agent (my test, 19:14:20)         → dial unix /run/vpp/api.sock: connect: resource temporarily unavailable
# journalctl -u vpp --since "6 hours ago" → no entries; dmesg: only workqueue "hogged CPU" notices
# tail /var/log/vpp/vpp.log             → last CLI answered: 19:12:26.812 "show map stats" (slot 16, F-det44-…-host);
#                                          my own last VPP writes: 19:12:23 af_packet_delete host-w11w0 / host-w11l0 via the
#                                          binary API after the LCP pairs were deleted (topology-netns.txt lines 83-97)
# ps: a `vppctl show interface` from `tools/lab rig down w16` (pid 1892330, parent 1892328, slot 16) has been stuck
#     since 19:13 — not mine, left alone
```

No gdb/strace/perf on the host, so no backtrace. The wedge started within ~60 s after (a) my cleanup's LCP-pair deletes +
af_packet deletes through the API (19:12:23, netns mode, ns-w11-frr existed while the pairs were deleted, then `rig down`
deleted ns-w11-lan/wan) and (b) slot 16's `show map …` CLI burst (19:12:21-26). Either could be the trigger (V24/D-101
af_packet delete and linux-cp netns teardown are both known-fragile); I cannot tell which without a backtrace.

**Ask:** restart VPP (manager, vpp lock), then re-run the sentinel check (D-203). Until then every host step of this row
that needs the binary API is blocked: topology test (both modes), agent-restart simulation, rollback on the rig, the
tier-3 (agent DryRun) half of the API-400 evidence. Please CONTINUE this row after the restart: everything is committed
and the drivers are ready (`test/topology/ospf/run.sh`, `test/topology/ospf/api400.sh`).

## Q2: two test files live outside `files_owned`

- `apps/agent/internal/renderers/frr/ospf/integration_test.go` (frrtest live test; R1 asked for exactly this file)
- `apps/agent/internal/agent/ospf_topology_integration_test.go` (the rig topology test; it needs the agent-internal
  frrtest harness and the P12 topology helpers, which a module under `test/` cannot import)

Both are test-only (`NGFW_INTEGRATION=1`, skip otherwise), touch no product code and are listed under "Shared hunks" in the
status file. If the envelope's file fence is strict, tell me where they should live.

## Q3 (D-210): "no new unit tests"

The two files above are host-evidence drivers (they skip without `NGFW_INTEGRATION=1` and run only on the real VPP/FRR),
not unit tests; I read D-210 as allowing them. Say so if not.
