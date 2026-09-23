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
VRX_METRICS_PORT=91<N>1         # agent prometheus (9111, 9121, …)
VRX_AGENT_SOCKET=/run/vrx-test/w<N>/agent.sock
VRX_PG_DATABASE=vrx_w<N>        # own database in the shared PostgreSQL; own Valkey db index N
VRX_VPP_TABLE_BASE=<N>000       # VRF/table ids a worker may allocate: N000–N999
```
Ports 3000 / 5173 / 9101 and `/run/vrx/agent.sock` belong to the **integrated main build** only (the manager's integration check).

## 2. Shared VPP
- Create only objects named/numbered inside your prefix/range: loopbacks `loop<N>xx`, host-interfaces `w<N>-*`, tables in your `VRX_VPP_TABLE_BASE` range, NAT pools in `10.<N>.0.0/16`, veth/netns names `w<N>-*`.
- Never touch `local0`, the management path, or anything without your prefix. Never `vppctl clear`/`show runtime clear` globally.
- Every integration test cleans up in `t.Cleanup`; the manager's nightly check deletes leftovers by prefix and files an issue against the slot.
- Tests that need a plugin that is not loaded (`linux_cp`, `linux_nl`, `npt66`) `t.Skip` with the reason — they must not fail the gate.

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
