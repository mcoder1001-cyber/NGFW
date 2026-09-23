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
| _none yet_ | | | | |

## Planned (documented ahead of use; move a row to *Active* when the renderer lands)

| binary | renderer | purpose | argv shape | task |
|---|---|---|---|---|
| `/usr/bin/vtysh` | frr | validate (`-C -f <staged>`), state (`-c "show ... json"`) | `vtysh -C -f <file>` / `vtysh -c <show cmd>` | P12 / RF |
| `/usr/lib/frr/frr-reload.py` | frr | apply rendered `frr.conf` | `frr-reload.py --reload <file>` (`--test` for dry-run) | P12 / RF |
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
