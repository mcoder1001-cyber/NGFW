# Rules for many workers on ONE host

Up to 12 worker agents run at once on 172.30.126.195, in separate git worktrees, against the **same**
VPP, PostgreSQL, Valkey, daemons, ports and disk. These rules are part of every TASK ENVELOPE.

## 1. Every worker gets a numeric slot `N` (1–12) from the manager
Derived values, exported by the manager in the envelope and by `tools/lab env <N>`:
```
VRX_SLOT=N
VRX_TEST_PREFIX=w<N>            # every VPP/DB/daemon object a worker creates carries this prefix
VRX_HTTP_PORT=3<N>00            # api dev/test port (3100, 3200, …), never 3000
VRX_WEB_PORT=5<N>00             # vite dev port (5100, 5200, …), never 5173
VRX_METRICS_PORT=$((9100+10*N+1))  # agent prometheus: 9111 … 9191, 9201, 9211, 9221 (slots 10–12) — `tools/lab env <N>` computes it
VRX_AGENT_SOCKET=/run/vrx-test/w<N>/agent.sock
VRX_PG_DATABASE=vrx_w<N>        # own database in the shared PostgreSQL; own Valkey db index N
VRX_VPP_TABLE_BASE=<N>000       # VRF/table ids a worker may allocate: N000–N999
```
Ports 3000 / 5173 / 9101 and `/run/vrx/agent.sock` belong to the **integrated main build** only (the manager's integration check).

## 1b. Locks
`/run/lock/vrx-lab.lock`: integration test harnesses and the rig take a **shared** lock (`flock -s`); `tools/ci.sh full` takes the **exclusive** lock only as a barrier (waits for running tests to finish) and then holds it shared while it runs (D-038); a VPP restart (after handover only) takes an exclusive lock for its whole duration. `/run/lock/vrx-vpp.lock`: manager-only VPP restarts. Prefix length:
`VRX_TEST_PREFIX` ≤ 6 chars (Linux IFNAMSIZ is 15).

## 2. Shared VPP
- Create only objects that **carry your prefix** (`VRX_TEST_PREFIX`, e.g. `w3`): loopbacks `loop<N>xx`, host-interfaces `host-<prefix>l0`/`host-<prefix>w0` (the rig's names), veths `<prefix>l0…`, namespaces `ns-<prefix>-lan|wan`, tables in your `VRX_VPP_TABLE_BASE` range, NAT pools and rig addresses in `10.<N>.0.0/16`.
- Never touch `local0`, the management path, or anything without your prefix. Never `vppctl clear`/`show runtime clear` globally.
- Every integration test cleans up in `t.Cleanup`; the manager's nightly check deletes leftovers by prefix and files an issue against the slot.
- Tests that need a plugin that is not loaded (`linux_cp`, `linux_nl`, `npt66`) `t.Skip` with the reason — they must not fail the gate.
- **Nobody restarts or kills VPP while handover is pending** (D-012). Restart-safety = stop *your* agent, delete *your* prefixed objects, restart *your* agent.
- Integration tests run only with `VRX_INTEGRATION=1` and under `flock -s /run/lock/vrx-lab.lock`; `pnpm test`/`make test` are unit-only.

## 3. Daemons (frr, strongswan, kea, unbound, chrony, snmpd, keepalived)
Exactly **one** worker at a time owns a daemon (the manager declares it in the envelope: `daemon-owner: frr`). Others mock or skip. The owner leaves the daemon **stopped and disabled** when the task ends. Never edit `/etc/vpp/*` (handover rule).

## 4. Disk and RAM
- Worktrees live in `/root/ngfw-wt/<id>`; pnpm uses the shared store (`pnpm install --prefer-offline`), so each worktree costs ~300 MB, not 1.5 GB. Delete your worktree's `dist/` and `apps/agent/bin` before finishing.
- The manager caps concurrency at `free -g` > 10 GB and load < 20.

## 5. Processes
- Start dev servers only on your slot ports; stop them by PID (`kill $PID`), **never** `pkill -f <pattern>` — patterns match other workers' shells.
- No `systemctl restart/kill vpp`, no reboots, no `rm -rf` outside your worktree and `/run/vrx-test/w<N>`.

## 6. Git
- Only your branch, only your worktree. No `git push`/`pull` (no remote). No history rewriting anywhere. Commit often; the manager merges.

## 7. VPP-global settings in tests (D-071, D-082)
Test slots are never the globals owner. A test that must read a VPP-wide setting holds `flock -s /run/lock/vrx-globals.lock`; a test that
changes one (only behind its opt-in env var, e.g. `VRX_DF7_GLOBALS=1`, `VRX_DF8_GLOBALS=1`) holds `flock -x` on it, saves the previous
value and restores exactly that value (never VPP defaults). The lab lock (`/run/lock/vrx-lab.lock`) stays the VPP-instance lock.

## 8. Slot 12 is reserved for the manager's `tools/ci.sh full` (D-087)
`tools/ci.sh full` runs the integration suite on main as CI slot 12. Workers are assigned slots 1–11 only; a worker never uses slot 12.
Host tests run one Go package at a time against the shared VPP (never `go test ./...` with VRX_INTEGRATION=1 in a worker).

## 9. Shared daemons: owner-prefix scoping (D-089)
When several slots' tests share one daemon instance (e.g. a charon), renderers operate only on objects carrying their owner prefix
(`WithOwnerPrefix`); a renderer never unloads, flushes or restarts what another prefix loaded.

## 10. Lab lock scope (D-094)
`flock -s /run/lock/vrx-lab.lock` is held only for the duration of an actual integration/E2E run — never by a long-lived dev stack
(API/agent/vite left running between runs). Workers stop every process they started (by PID) before they finish or pause; a stack left
running blocks the manager's `tools/ci.sh full` barrier.

## 11. Packet trace is banned on the shared VPP (D-128)
`trace add`, `show trace` and `clear trace` (vppctl, `cli_inband`, the tracedump API) are **banned** on the shared VPP — for tests,
scripts and humans alike. Why: the 2026-09-24 18:41 crash (SIGSEGV PC 0x0, very likely 07:27 too) was a `show trace`. Trace records
keep the node index of per-interface output/tx nodes; when an interface is deleted its nodes are renamed `interface-N-*-deleted` and
recycled by the next interface, which may have no trace formatter (Loopback). `format_vlib_trace` (`vlib/trace.c:159-162`) then calls a
NULL formatter for the old record → VPP down for every slot. Records of any slot's interface churn trigger it, so no per-test care
(unique packet size, own `clear trace`) makes a dump safe. Evidence: `/root/ngfw-wt/logs/crash-20260924-1841/` (core, journal, run
logs), F-vlan-qinq review H1; upstream fix: `docs/vpp-code-track.md` (V-item of TD-20).
Prove a forwarding path instead with (all read-only, all scoped to your own objects):
- interface counters of your prefixed interfaces: `vppctl show interface <yours>` or the stats segment — rx on the ingress, tx on the
  egress side rise by exactly the packets you sent (size them so they stand out from ARP; `test/topology/interfaces` `echoFrames`);
- the FIB: `ip_route_lookup` (exact) / `show ip fib table <T> <prefix>` — the entry, its path/interface, and its load-balance
  `to:[packets:bytes]` counter, which ip4-lookup bumps for every packet it forwards through that entry;
- the packet itself: `tcpdump` inside your rig's netns (`ip netns exec ns-<prefix>-wan tcpdump …`), never on a shared interface.
`pcap trace` / `pcap dispatch trace` are VPP-global captures and are no substitute on the shared host. `tools/ci.sh` (check, quick,
full) fails on any trace command or tracedump API call outside docs and the generated bindings; there is no escape hatch. The ban holds
until VPP carries the NULL guard, and even then the trace buffer stays VPP-global (a single-tenant tool for a per-slot VPP).
