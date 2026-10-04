# LAB-vpp-per-slot-a — `tools/lab vpp up|down|status <slot>`: a small per-slot VPP for tests (tooling half of LAB-vpp-per-slot)
Source: REVIEW-2026-09-24 item **6.5** (`docs/status/review-2026-09-24-verdicts.md` row 99; row created by D-125) and
`docs/decisions/PENDING-vpp-host-hardening.md` option **F**. That file's decision line (2026-09-29): option **B applied** (4096 × 2 MiB
hugepages, rmem/wmem raised), F unparked; option **A (vSphere memory reservation) is still open** and the ESXi balloon holds ~61 GB
(D-219 (7), D-220 (2)) — so this row ships the tool with a hard instance cap; raising it waits for A. D-218 dropped this row as a dep of
P12-fib-proof. Split by M-prompts: **-a** = tooling (6 h, this file), **-b** = migrate the tests (4 h, `LAB-vpp-per-slot-b.md`).
The shared VPP (`vpp.service`, `/run/vpp/*`) stays for tools/app and every non-migrated test; nothing here may touch it (D-012).
## Do
1. **Rendering** (additive, product output byte-identical — golden files unchanged): new `vppstartup.Settings` fields + `ngfw-startupgen`
   flags for a lab instance — `unix { runtime-dir, cli-listen, log, poll-sleep-usec }`, `api-segment { prefix w<N> }`,
   `socksvr { socket-name }`, `statseg { socket-name }`, small `memory { main-heap-size 512M }` and `buffers` — all paths under
   `/run/ngfw-test/w<N>/vpp/`; plugins: `dpdk`, `linux_cp`, `linux_nl` disabled (af_packet stays on for the rig). Unit tests: a slot rendering
   contains **no** `/run/vpp/` path and no `socksvr { default }`, sockets keep `gid vpp`; defaults render exactly as today.
2. **`tools/lab vpp up <N>`** (slots 1-12, 14-32; refuses 13 and the shared VPP's paths): renders the conf with `ngfw-startupgen --no-host`
   (+ the fact flags it then needs; dpdk is off, so no NIC is rendered) into the runtime dir, **re-greps it fail-closed** for `/run/vpp/`
   and `socksvr { default }`, then starts `/usr/bin/vpp -c <conf>` as the transient
   unit `ngfw-vpp-w<N>` (`systemd-run --collect -p MemoryMax=1G -p Restart=no`); waits for its CLI socket (`timeout`-bounded).
   Refuses when: `tools/mem-canary.sh` fails (D-224: under the ESXi balloon MemAvailable alone is no headroom signal), MemAvailable
   after the instance would fall under 8 GiB (D-219), HugePages_Free does not cover its buffers, or
   `NGFW_LAB_VPP_MAX` (default **2**, hard ceiling 4 until PENDING option A is answered) instances already run. Takes the shared lab lock
   (`flock -s`) only while starting/stopping.
3. **`tools/lab vpp down <N>`** stops only `ngfw-vpp-w<N>` (unit name checked against `^ngfw-vpp-w[0-9]+$`; never `vpp.service`, never
   pkill), then removes its runtime dir and `/dev/shm/w<N>-*`. **`tools/lab vpp status [<N>]`**: unit state, PID, RSS, sockets.
4. **`tools/lab env <N>`** additionally exports `NGFW_VPP_API_SOCKET`, `NGFW_VPP_CLI_SOCKET`, `NGFW_VPP_STATS_SOCKET`,
   `NGFW_AGENT_VPP_API_SOCKET`, `NGFW_AGENT_VPP_STATS_SOCKET` and `NGFW_VPPCTL` (`vppctl -s <cli>`): the slot instance's paths when its unit
   is active, else today's `/run/vpp/*` (unchanged behaviour). The rig's vppctl helpers (`vpp_cli_ok`/`vpp_ifnames`/`vpp_if_addrs`/
   `vpp_plugins`, ~:147-160) honour `NGFW_VPP_CLI_SOCKET`. Keep `python3 tools/slot-check.py` green; add the per-slot socket formula there.
5. **Slot 12 (CI)**: `tools/lab vpp up 12` works; wiring it into `tools/ci.sh full` is the manager's file → put the exact lines in
   `docs/status/tasks/LAB-vpp-per-slot-a-questions.md`.
6. **Docs**: a new § "Per-slot VPP" in `docs/lab/shared-host-rules.md` (paths, cap, memory floor, who may start one, D-128 still applies).
7. **Host evidence** (your slot, one instance, ≤ 15 min): before/after `systemctl show vpp -p NRestarts` + `timeout 5 vppctl show version`
   of the shared VPP (unchanged); `ls /dev/shm` before/after (only `w<N>-*` new); MemAvailable before/after; idle CPU of the instance
   (`top -b -n1 -p <pid>`); `vppctl -s … show version` / `show plugins | grep -c`; `tools/lab rig up w<N>` + `down` against the slot VPP;
   `NGFW_INTEGRATION=1 ../../tools/heavy.sh go test ./internal/descriptors/core/...` (from apps/agent) with the slot sockets (it already
   reads `NGFW_VPP_API_SOCKET`); then `vpp down` and proof nothing is left (unit, runtime dir, shm files). Evidence as `.txt` under
   `docs/status/tasks/LAB-vpp-per-slot-a-evidence/` (D-175).
## Out of scope
Migrating the tests (-b); editing `tools/ci.sh`, `/etc/vpp/*`, `vpp.service`, hugepage sysctls or anything of the shared VPP; DPDK/NICs in
slot instances; raising the cap above 4 or making per-slot VPP the default for every run (parked on PENDING option A); performance numbers.
## Rules
- Files you own: `tools/lab` (new `cmd_vpp`, its dispatch/usage lines, the `cmd_env` exports, the vppctl helpers ~:147-160 — list your line
  ranges under `## Shared hunks`; TD-19 edits the provision functions of the same file), `apps/agent/internal/renderers/vppstartup/**`,
  `apps/agent/cmd/ngfw-startupgen/{main,main_test}.go` (F-dataplane-apply-flow adds `preview_parity_test.go` there), `tools/slot-check.py`
  (one formula hunk), `docs/lab/shared-host-rules.md` (new § only),
  `docs/status/tasks/LAB-vpp-per-slot-a*`. Everything else read-only; a needed edit elsewhere → `docs/status/tasks/LAB-vpp-per-slot-a-questions.md`.
- Shared VPP: never restart, kill or reconfigure it; prefixed objects only in your slot; `timeout 10` on every vppctl; packet trace banned
  (D-128, also inside slot instances — the ci.sh guard has no escape hatch). Stop every instance and process you started before finishing.
- D-210a: write tests for your change and get them passing in your package (paste output); no full suite, no lint, no other packages' tests;
  run them through `tools/heavy.sh` (D-224), e.g. `../../tools/heavy.sh go test ./internal/renderers/vppstartup/... ./cmd/ngfw-startupgen/...` from apps/agent.
- No contract change (the renderer settings are agent-internal). Permission refusals (e.g. `systemd-run`) are final: record them in the questions file.
- Finish: `tools/ci-slot.sh --base main` green (compile-only gate, D-220/D-222; never tools/ci.sh directly), commit on your branch,
  `docs/status/tasks/LAB-vpp-per-slot-a.md` with pasted real output.
