# LAB-vpp-per-slot — WIP / handoff (claude/vpp-per-slot-20261006)

- branch: `claude/vpp-per-slot-20261006` from origin/main 75434a3e5; worktree `/root/ngfw-wt/claude-vpp-per-slot`
- local commits only (no push, per instruction); remote SHA: none. Local HEAD: the commit that adds this file (`git log -1`).
- state: implementation complete, live-proven on slot 20, `tools/ci.sh quick --base origin/main` **CI GATE PASSED** (18m44s, tree
  2c6f24d9b; logs `/root/ngfw-wt/logs/ci/claude-vpp-per-slot-20261006-104202-1411341`). Needs independent review.

## Host changes
**None.** The owner approval (2026-10-06) to change `/etc/vpp` / `vpp.service` was not needed: slot VPPs run without hugepages
(main heap and buffers on 4k pages), without DPDK, as transient units. `/etc/vpp/startup.conf`, `vpp.service` and the shared VPP
process are untouched: MainPID 1014, NRestarts 0, ActiveEnterTimestamp 2026-10-03 09:18:02 UTC before and after; HugePages_Free 4020
with and without the instance. The product stack (vrx-agent, apps/api on /run/vpp) is unaffected.

## Design
- `vppstartup.Settings` gains optional lab fields (runtime-dir, poll-sleep-usec, api-segment prefix, socksvr socket-name, statseg size,
  memory main-heap-size/page-size, buffers page-size); zero values render nothing, so every existing golden file is byte-identical.
  `LabSlotSettings(root, N)` (paths under `<root>/w<N>/vpp`, prefix `w<N>`, 512M heap, 4k pages, statseg 32M) and the fail-closed
  `CheckLabRendering` (no `/run/vpp`, no `socksvr { default }`, no dpdk section, own runtime-dir/prefix/sockets).
- `ngfw-startupgen --lab-slot N [--lab-root D]`: renders with those settings, refuses a document that keeps dpdk_plugin.so enabled,
  re-checks the rendering.
- `tools/lab vpp up|down <N>`, `vpp status [<N>]`: transient unit `ngfw-vpp-w<N>` (systemd-run --collect, MemoryMax=1024M,
  Restart=no); document dpdk/linux_cp/linux_nl off, npt66 on, 4096 buffers, main core `2+(N-1) mod (cpus-2)`; refuses 13/0/>32,
  more than NGFW_LAB_VPP_MAX (default 2, ceiling 4; CI slot 12 not counted), MemAvailable-1G < 8G, a failing mem-canary (if present);
  shared lab lock while starting/stopping; down removes exactly the runtime dir and `/dev/shm/w<N>-*` and proves no leftovers.
- `tools/lab env N`: also `NGFW_VPP_{API,CLI,STATS}_SOCKET`, `NGFW_AGENT_VPP_{API,STATS}_SOCKET`, `NGFW_VPPCTL`: slot paths while
  the unit runs, else `/run/vpp/*`. Rig helpers follow `NGFW_VPP_CLI_SOCKET`; status/provision keep describing the shared VPP.
- No `poll-sleep-usec`: measured ~17 % of a core idle with 100 us (vlib/file.c: fixed sleep + epoll timeout 0), ~3 % without.
- `tools/slot-check.py`: socket exports must be all-shared or all-own-slot; slot runtime dirs unique and outside /run/vpp.
- `tools/ci.sh`: slot_env accepts `NGFW_VPPCTL="vppctl -s <path>"`; the V19 pre-flight dials `$NGFW_VPP_API_SOCKET`;
  `NGFW_CI_SLOT_VPP=1` makes `full` bring up/down the CI slot 12 instance (opt-in: not yet validated with a full run).
- Test migration (where practical): `vpptest.APISocket/CLISocket/StatsSocket/VPPCtl` (+ unit test); fixtures acl, df2test, df6test,
  df7test, dfkittest, ifacetest, nattest, vpntest, agent stats/wireguard, desired/ikev2*; topology `apiSocket` consts in acl, bonding,
  bridge-l2, det44, interfaces, ipfix-sflow, kea-dhcp-relay, loopback-bvi-gso-lldp-span, nat44-ed-sessions, nat44-ei-64-66-nptv6,
  neighbors-ra, qos-flat, vlan-qinq, vrf-static-ecmp and integration/smoke (+stats) now follow the env (default unchanged).

## Test results (real output in `claude-vpp-per-slot-evidence/`)
- `go test ./internal/renderers/vppstartup/... ./cmd/ngfw-startupgen/... ./internal/vpp/vpptest/` ok; go vet of every touched package ok.
- `python3 -m unittest tools/tests/test_lab_vpp.py` (fakes: up/env/status/down, refusals, shm scoping, bad rendering): 6 tests OK.
- `python3 tools/slot-check.py` ok (31 slots verified).
- Live slot 20: up in ~1 s, RSS ~310 MiB, 84 plugins, `vppctl -s /run/ngfw-test/w20/vpp/cli.sock show version` ok (01, 02);
  rig up/ping/down on the slot VPP, shared VPP had no host-w20* (03); idle CPU 16-18 % -> 0-3 % without poll-sleep-usec (04);
  `NGFW_INTEGRATION=1 go test ./internal/descriptors/core/...` against the slot VPP: all PASS (05); down: unit gone, runtime dir and
  /dev/shm/w20-* gone, no process, shared VPP unchanged (06; the empty `/run/ngfw-test/w20` parent was removed by hand).

## Remaining / follow-ups
- Run `NGFW_CI_SLOT_VPP=1 tools/ci.sh full` once (manager, exclusive lock) before making it the default.
- Not migrated: shell/Python harnesses with `vppctl` or `/run/vpp` (ipsec/api-flow.py, lcp-multicast-reconcile.py, ospf/api400.sh,
  traffic-a/c globals, tunnels/delete-owned.go, multiwan acceptance reading /run/vpp/startup.conf, isolated-vpp/traffic-b mount
  designs), Go `exec "vppctl"` calls without `-s`, and tests that compare with `/run/vpp/api.sock` on purpose (p12/ospf/capture).
- Raising the instance cap waits for PENDING-vpp-host-hardening option A.
