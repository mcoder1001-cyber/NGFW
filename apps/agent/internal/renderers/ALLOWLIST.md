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
| `/usr/bin/ip` | strongswan (test harness `strongswan/swantest` only — **test-only**; never `ip netns exec`: daemons enter the netns by `setns` on a locked thread) | create/delete `ns-<prefix>-{a,b,v}`, the veth pair `<prefix>-a`↔`<prefix>-b`; read xfrm state/policy (keys stripped); flush it in its own namespaces after a (simulated) charon crash | `ip netns add\|delete <ns>`, `ip link add <prefix>-a netns <ns> type veth peer name <prefix>-b netns <ns>`, `ip -n <ns> addr add\|link set …`, `ip -n <ns> xfrm state\|policy [flush]` | RF-2 |
| `/usr/sbin/charon-systemd` | strongswan (test harness only) | test-scoped IKE daemon (no pid file, unlike `/usr/lib/ipsec/charon`); started via the per-instance symlink `/run/vrx-test/<prefix>/swan/<x>/charon-systemd` so `pgrep -f` finds exactly the test daemons; stopped by SIGTERM to that PID | no arguments; `STRONGSWAN_CONF=<instance>/strongswan.conf` in the environment | RF-2 |
| `/usr/sbin/swanctl` | strongswan | Validate's optional integration checker only (`WithChecker`): load a staged copy into a **scratch** charon, never the live one; apply/retrieve/events use VICI (govici), not this binary | `swanctl --load-all --noprompt --file <staged swanctl.conf> --uri unix://<scratch socket>` | RF-2 |
| `/usr/lib/frr/mgmtd` | frr (test harness only) | test-scoped daemon, child of `ip netns exec` | `mgmtd -d -N <prefix> --vty_socket <dir> -i <pid> -A 127.0.0.1 -P 0 --log file:<log> --log-level warn` | RF-1 |
| `/usr/lib/frr/zebra` | frr (test harness only) | test-scoped daemon | same as mgmtd + `-z <dir>/zserv.api -f <empty cfg>` | RF-1 |
| `/usr/lib/frr/staticd` | frr (test harness only) | test-scoped daemon | same as mgmtd + `-z <dir>/zserv.api` | RF-1 |
| `/usr/lib/frr/bgpd` `/usr/lib/frr/ospfd` `/usr/lib/frr/ospf6d` `/usr/lib/frr/bfdd` `/usr/lib/frr/pimd` `/usr/lib/frr/isisd` `/usr/lib/frr/ripd` `/usr/lib/frr/ldpd` | frr (test harness only, for P12/F-*) | protocol daemons started only when a test names them in `frrtest.Options.Daemons` — **unused until P12 (bgpd), F-ospf (ospfd/ospf6d), F-bfd-redistribution (bfdd), F-igmp-mfib (pimd), F-isis-rip (isisd/ripd), F-mpls-srmpls (ldpd)** | same as staticd | RF-1 (ahead of P12/F-*) |
| `/usr/sbin/snmpd` | snmpd | Validate *parse run* (net-snmp has no offline checker): a daemonised instance on a staged check copy whose agentaddress/agentXSocket point to unix sockets in the staging dir and whose trap sinks are removed; stopped by its pidfile PID (verified `/proc/<pid>/exe` + cmdline) right after | `snmpd -C -c <staging>/check/snmpd.conf -Lf <staging>/check/snmpd.log -p <staging>/check/snmpd.pid -m "" -M <staging>/check/mibs` | RF-4 |
| `/usr/sbin/keepalived` | keepalived | config check (interfaces, script security); SIGJSON number | `keepalived -t -f <staged keepalived.conf> [-s <netns>]` / `keepalived --signum=JSON` | RF-4 |
| `/usr/sbin/rsyslogd` | rsyslog | config validation run | `rsyslogd -N1 -f <staged file>` | RF-4 |
| `/usr/bin/systemctl` | snmpd, keepalived, rsyslog (product control channel, `rfkit.SystemdController`) | reload / restart / signal **the unit this renderer owns** — never start, enable or disable | `systemctl reload snmpd` · `systemctl reload keepalived` · `systemctl kill --kill-whom=main --signal=<n> keepalived` · `systemctl restart rsyslog` | RF-4 |
| `/usr/libexec/vrx/vrx-keepalived-notify` | keepalived — **not executed by the agent**: the only `notify_*` target keepalived.conf ever names (source `keepalived/cmd/vrx-keepalived-notify`, packaged by P10) | writes `<state dir>/<instance>.state` (the state/event channel) | keepalived runs `vrx-keepalived-notify <state-dir> INSTANCE <name> MASTER\|BACKUP\|FAULT\|STOP` | RF-4 |
| `/usr/libexec/vrx/checks` | keepalived — directory of the **shipped** `vrrp_script` executables (keepalived runs them; a script is chosen by name from the renderer's allow-list, never user text or paths; empty until F-vrrp ships checks) | track scripts | `vrrp_script <name> { script "/usr/libexec/vrx/checks/<check>" }` | RF-4 |
| `/usr/lib/x86_64-linux-gnu/rsyslog` | rsyslog — module directory, **nothing executed**: `stat` of `lmnsd_ossl.so` before accepting a TLS export | TLS driver presence check | — | RF-4 |
| `/usr/bin/ip` | keepalived integration test only (`_test.go`, never a renderer allowlist) | slot netns + veth pair, keepalived child inside it | `ip netns add\|delete ns-<prefix>-a`, `ip -n ns-<prefix>-a link\|addr …`, `ip netns exec ns-<prefix>-a keepalived -n -l -P -G -f <cfg> -p … -r … -c …` | RF-4 |
| (VPP `cli_inband` binary-API message — not an exec'd binary, no process) | lb (`descriptors/lb.GarbageCollect`, called only by the globals owner, `subsystems/lb.go`) | run VPP's lb garbage collection after lb deletes: removed VIPs/ASes are freed only there (D-090 (2), V20) | the constant `lb vip 0.0.0.0/32 del` (`lb.GCCommand`): parses, runs `lb_garbage_collection()`, then fails its lookup of the never-configured sentinel VIP (0.0.0.0/8 is refused as a VIP by schema and projection); no user input | F-lb |

`vppstartup` (F-startup-gen) runs no process: `Validate` is structural and `Apply` refuses (VPP restart = manager step,
manual procedure in `docs/agent/renderers/vppstartup.md`, tooling in task F-startup-apply); the `vrx-startupgen` CLI only reads files. `/usr/lib/x86_64-linux-gnu/vpp_plugins` in its
source is the plugin **directory** it lists (host facts), not a binary.

## Planned (documented ahead of use; move a row to *Active* when the renderer lands)

| binary | renderer | purpose | argv shape | task |
|---|---|---|---|---|
| `/usr/bin/systemctl` | keepalived, snmpd, rsyslog | reload the unit **owned by this task's envelope** | `systemctl reload <unit>` / `systemctl restart <unit>` | RF |
| `/usr/sbin/keepalived` | keepalived | config check | `keepalived -t -f <file>` | RF |
| `/usr/sbin/snmpd` | snmpd | integration test child process only | `snmpd -f -c <cfg> -p <pid> 127.0.0.1:<slot port>` | RF |
| `/usr/sbin/swanctl` | strongswan | load / list SAs when VICI is unavailable | `swanctl --load-all --noprompt`, `swanctl --list-sas --raw` | P11 |
| `/usr/libexec/vrx/vrx-upgrade` | backup-restore | stage/activate/confirm/rollback an update bundle (packaged by P10) | `vrx-upgrade status [--json]\|stage <bundle>\|activate\|confirm\|rollback` | F-backup-restore |
| `/usr/libexec/vrx/vrx-support-collect` | backup-restore | assemble a support bundle (config, logs, audit rows) | `vrx-support-collect --out <path> [--since <sec>] [--audit-rows]` | F-backup-restore |

## Active — RF-3 (kea, unbound, chrony)

Production allowlists are `kea.Binaries()`, `unbound.Binaries()`, `chrony.Binaries()`; each package's
`TestProductAllowlistHasNoTrampoline` keeps `ip`, `env`, shells and `systemctl` out of them. Restarts are
never executed by these renderers (unit tests also spawn `/usr/bin/sleep` as a stand-in "restarted daemon" process): `Apply` returns a typed `*ActionRequired{Unit, Action}` and the commit
engine (product: systemd) acts on it.

| binary | renderer | purpose | argv shape | added by |
|---|---|---|---|---|
| `/usr/sbin/kea-dhcp4` | kea | validate a staged kea-dhcp4.conf (Kea 3.0.3; env `KEA_CONTROL_SOCKET_DIR`/`KEA_DHCP_DATA_DIR`/`KEA_LOG_FILE_DIR`) | `kea-dhcp4 -t <staged kea-dhcp4.conf>` | RF-3 |
| `/usr/sbin/kea-dhcp6` | kea | validate a staged kea-dhcp6.conf | `kea-dhcp6 -t <staged kea-dhcp6.conf>` | RF-3 |
| `/usr/bin/ip` | kea (**test-only**, never in `kea.Binaries()`) | run the DHCP checkers inside the rig namespace (`Paths.Netns`, tests only); create/delete `ns-<prefix>-a` and its veths; start the test servers in it | `ip netns exec ns-<prefix>-a /usr/sbin/kea-dhcp4\|kea-dhcp6 -t <staged file>`; test harness: `ip netns add\|del ns-<prefix>-a`, `ip -n <ns> link\|addr …`, `ip netns exec <ns> kea-dhcp4\|kea-dhcp6 -c <cfg>` | RF-3 |
| `/usr/sbin/unbound-checkconf` | unbound | validate a staged unbound.conf | `unbound-checkconf <staged unbound.conf>` | RF-3 |
| `/usr/sbin/unbound-control` | unbound | apply and read state over the unix control socket | `unbound-control -c <unbound.conf> reload_keep_cache\|status\|stats_noreset\|list_forwards\|list_stubs\|list_local_zones\|list_local_data` | RF-3 |
| `/usr/sbin/chronyd` | chrony | validate staged chrony.conf and sources file (chrony 4.8 `-p`: parse, print, exit) | `chronyd -p -f <staged chrony.conf>` / `chronyd -p -f <staged vrx.sources>` | RF-3 |
| `/usr/bin/chronyc` | chrony | reload sources / keys, read state over the unix command socket | `chronyc -h <chronyd.sock> reload sources\|rekey` / `chronyc -h <chronyd.sock> -c sources\|sourcestats\|tracking\|serverstats` | RF-3 |

Test-only children started by the RF-3 integration tests (killed by PID in `t.Cleanup`): `/usr/sbin/kea-dhcp4`,
`/usr/sbin/kea-dhcp6` (`-c <cfg>`, inside `ip netns exec ns-<prefix>-a`) — no kea-ctrl-agent (D-079),
`/usr/sbin/unbound -d -c <cfg>`, `/usr/sbin/chronyd -f <cfg> -n -x -l <log>` (`-x` mandatory: never touch the host clock).

Not a binary: `kea.DefaultHooksDir` = `/usr/lib/x86_64-linux-gnu/kea/hooks` is only searched for
`libdhcp_lease_cmds.so`, which Kea itself loads from the rendered `hooks-libraries`.

Integration tests that start a daemon as a child process (`zebra -N`, `kea-dhcp4 -c`,
`unbound -c`, `chronyd -f -x`, `charon`) use the same runner and the same rule: the binary is
listed here, the argv is fixed, and the process is killed by the PID the test spawned.
