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

## Planned (documented ahead of use; move a row to *Active* when the renderer lands)

| binary | renderer | purpose | argv shape | task |
|---|---|---|---|---|
| `/usr/sbin/swanctl` | strongswan | load / list SAs when VICI is unavailable | `swanctl --load-all --noprompt`, `swanctl --list-sas --raw` | P11 |
| `/usr/bin/systemctl` | keepalived, snmpd, rsyslog | reload the unit **owned by this task's envelope** | `systemctl reload <unit>` / `systemctl restart <unit>` | RF |
| `/usr/sbin/keepalived` | keepalived | config check | `keepalived -t -f <file>` | RF |
| `/usr/sbin/snmpd` | snmpd | integration test child process only | `snmpd -f -c <cfg> -p <pid> 127.0.0.1:<slot port>` | RF |

## Active — RF-3 (kea, unbound, chrony)

Production allowlists are `kea.Binaries()`, `unbound.Binaries()`, `chrony.Binaries()`; each package's
`TestProductAllowlistHasNoTrampoline` keeps `ip`, `env`, shells and `systemctl` out of them. Restarts are
never executed by these renderers: `Apply` returns a typed `*ActionRequired{Unit, Action}` and the commit
engine (product: systemd) acts on it.

| binary | renderer | purpose | argv shape | added by |
|---|---|---|---|---|
| `/usr/sbin/kea-dhcp4` | kea | validate a staged kea-dhcp4.conf (Kea 3.0.3; env `KEA_CONTROL_SOCKET_DIR`/`KEA_DHCP_DATA_DIR`/`KEA_LOG_FILE_DIR`) | `kea-dhcp4 -t <staged kea-dhcp4.conf>` | RF-3 |
| `/usr/sbin/kea-dhcp6` | kea | validate a staged kea-dhcp6.conf | `kea-dhcp6 -t <staged kea-dhcp6.conf>` | RF-3 |
| `/usr/sbin/kea-ctrl-agent` | kea | validate a staged kea-ctrl-agent.conf | `kea-ctrl-agent -t <staged kea-ctrl-agent.conf>` | RF-3 |
| `/usr/bin/ip` | kea (**test-only**, never in `kea.Binaries()`) | run the DHCP checkers inside the rig namespace (`Paths.Netns`, tests only); create/delete `ns-<prefix>-a` and its veths; start the test servers in it | `ip netns exec ns-<prefix>-a /usr/sbin/kea-dhcp4\|kea-dhcp6 -t <staged file>`; test harness: `ip netns add\|del ns-<prefix>-a`, `ip -n <ns> link\|addr …`, `ip netns exec <ns> kea-dhcp4\|kea-dhcp6 -c <cfg>` | RF-3 |
| `/usr/sbin/unbound-checkconf` | unbound | validate a staged unbound.conf | `unbound-checkconf <staged unbound.conf>` | RF-3 |
| `/usr/sbin/unbound-control` | unbound | apply and read state over the unix control socket | `unbound-control -c <unbound.conf> reload_keep_cache\|status\|stats_noreset\|list_forwards\|list_stubs\|list_local_zones\|list_local_data` | RF-3 |
| `/usr/sbin/chronyd` | chrony | validate staged chrony.conf and sources file (chrony 4.8 `-p`: parse, print, exit) | `chronyd -p -f <staged chrony.conf>` / `chronyd -p -f <staged vrx.sources>` | RF-3 |
| `/usr/bin/chronyc` | chrony | reload sources / keys, read state over the unix command socket | `chronyc -h <chronyd.sock> reload sources\|rekey` / `chronyc -h <chronyd.sock> -c sources\|sourcestats\|tracking\|serverstats` | RF-3 |

Test-only children started by the RF-3 integration tests (killed by PID in `t.Cleanup`): `/usr/sbin/kea-dhcp4`,
`/usr/sbin/kea-dhcp6` (`-c <cfg>`, inside `ip netns exec ns-<prefix>-a`), `/usr/sbin/kea-ctrl-agent -c <cfg>`,
`/usr/sbin/unbound -d -c <cfg>`, `/usr/sbin/chronyd -f <cfg> -n -x -l <log>` (`-x` mandatory: never touch the host clock).

Not a binary: `kea.DefaultHooksDir` = `/usr/lib/x86_64-linux-gnu/kea/hooks` is only searched for
`libdhcp_lease_cmds.so`, which Kea itself loads from the rendered `hooks-libraries`.

Integration tests that start a daemon as a child process (`zebra -N`, `kea-dhcp4 -c`,
`unbound -c`, `chronyd -f -x`, `charon`) use the same runner and the same rule: the binary is
listed here, the argv is fixed, and the process is killed by the PID the test spawned.
