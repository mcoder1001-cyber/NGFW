# Renderer binary allowlist

The only way agent code may start a process is `renderers.SystemRunner.Run` with a fixed
argv whose `Path` is in the runner's `Allowlist` (00-CONTEXT rule 9: no user input ever
reaches a shell). **Every binary a renderer invokes is listed here, by absolute path.**
`TestAllowlistDocumented` (`helpers_exec_test.go`) fails the build when a non-test Go file
under `internal/renderers/` names a binary path that is missing from this file, calls
`exec.Command` outside `helpers_exec.go`, or spawns `sh -c` / `bash -c`.

Rules for an entry:

- Absolute, clean path of the binary as installed on Ubuntu 26.04 (`command -v`, then
  `readlink -f` only if the packaged path is a symlink you must follow).
- Fixed argv shape: name every flag; rendered content goes through files or stdin, never argv.
- No shells, no interpreters with inline code (`python3 -c`, `perl -e`), no `sudo`, no `systemctl
  restart` of a unit another worker owns (see `docs/lab/shared-host-rules.md` §3).
- Add the row in the same commit that adds the call; the renderer's README documents why.

## Active

| binary | renderer | purpose | argv shape | added by |
|---|---|---|---|---|
| `/usr/bin/vtysh` | frr | validate a staged frr.conf; read state | `vtysh --config_dir <dir> --vty_socket <rundir> [-N <ns>] -C -f <staged frr.conf>` / `… -c "<constant show command>"` | RF-1 |
| `/usr/lib/frr/frr-reload.py` | frr | apply (diff, no restart) / dry-run diff | `frr-reload.py --reload\|--test --log-level critical --logfile <log> --bindir /usr/bin --confdir <dir> --rundir <rundir> --vty_socket <rundir> [--pathspace <ns>] <frr.conf>` | RF-1 |
| `/usr/bin/ip` | frr (test harness `frr/frrtest` only — **test-only; never in a production allowlist**: `ip netns exec` runs any binary; `frr.Binaries()` excludes it, `TestProductAllowlistHasNoTrampoline`) | create/delete the slot netns + dummy/vrf links; start the test daemons inside it | `ip netns add\|delete ns-<prefix>-frr`, `ip -n <ns> link\|addr …`, `ip netns exec <ns> <daemon> <fixed daemon argv>` | RF-1 |
| `/usr/lib/frr/mgmtd` | frr (test harness only) | test-scoped daemon, child of `ip netns exec` | `mgmtd -d -N <prefix> --vty_socket <dir> -i <pid> -A 127.0.0.1 -P 0 --log file:<log> --log-level warn` | RF-1 |
| `/usr/lib/frr/zebra` | frr (test harness only) | test-scoped daemon | same as mgmtd + `-z <dir>/zserv.api -f <empty cfg>` | RF-1 |
| `/usr/lib/frr/staticd` | frr (test harness only) | test-scoped daemon | same as mgmtd + `-z <dir>/zserv.api` | RF-1 |
| `/usr/lib/frr/bgpd` `/usr/lib/frr/ospfd` `/usr/lib/frr/ospf6d` `/usr/lib/frr/bfdd` `/usr/lib/frr/pimd` `/usr/lib/frr/isisd` `/usr/lib/frr/ripd` `/usr/lib/frr/ldpd` | frr (test harness only, for P12/F-*) | protocol daemons started only when a test names them in `frrtest.Options.Daemons` — **unused until P12 (bgpd), F-ospf (ospfd/ospf6d), F-bfd-redistribution (bfdd), F-igmp-mfib (pimd), F-isis-rip (isisd/ripd), F-mpls-srmpls (ldpd)** | same as staticd | RF-1 (ahead of P12/F-*) |

`vppstartup` (F-startup-gen) runs no process: `Validate` is structural and `Apply` refuses (VPP restart = manager step,
manual procedure in `docs/agent/renderers/vppstartup.md`, tooling in task F-startup-apply); the `vrx-startupgen` CLI only reads files. `/usr/lib/x86_64-linux-gnu/vpp_plugins` in its
source is the plugin **directory** it lists (host facts), not a binary.

## Planned (documented ahead of use; move a row to *Active* when the renderer lands)

| binary | renderer | purpose | argv shape | task |
|---|---|---|---|---|
| `/usr/sbin/swanctl` | strongswan | load / list SAs when VICI is unavailable | `swanctl --load-all --noprompt`, `swanctl --list-sas --raw` | P11 |
| `/usr/sbin/kea-dhcp4` | kea-dhcp4 | config syntax check | `kea-dhcp4 -t <file>` | RF |
| `/usr/sbin/kea-dhcp6` | kea-dhcp6 | config syntax check | `kea-dhcp6 -t <file>` | RF |
| `/usr/sbin/unbound-checkconf` | unbound | config syntax check | `unbound-checkconf <file>` | RF |
| `/usr/sbin/unbound-control` | unbound | reload, stats | `unbound-control -c <cfg> reload` / `stats_noreset` | RF |
| `/usr/bin/chronyc` | chrony | reload sources, state | `chronyc reload sources` / `chronyc -c sources` | RF |
| `/usr/sbin/chronyd` | chrony | config check | `chronyd -p -f <file>` (if the installed version supports `-p`) | RF |
| `/usr/bin/systemctl` | keepalived, snmpd, rsyslog | reload the unit **owned by this task's envelope** | `systemctl reload <unit>` / `systemctl restart <unit>` | RF |
| `/usr/sbin/keepalived` | keepalived | config check | `keepalived -t -f <file>` | RF |
| `/usr/sbin/snmpd` | snmpd | integration test child process only | `snmpd -f -c <cfg> -p <pid> 127.0.0.1:<slot port>` | RF |

Integration tests that start a daemon as a child process (`zebra -N`, `kea-dhcp4 -c`,
`unbound -c`, `chronyd -f -x`, `charon`) use the same runner and the same rule: the binary is
listed here, the argv is fixed, and the process is killed by the PID the test spawned.
