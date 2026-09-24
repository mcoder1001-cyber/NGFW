# Task: RF-4 — Renderers for daemons: snmpd, keepalived, rsyslog   (prepend 00-CONTEXT.md)

## Goal
Write the agent-side **renderer** for each of net-snmp `snmpd` (WBS D7.5), keepalived (D9.1 — the VRRP path next to VPP's native plugin) and rsyslog
(D7.7 syslog export): desired state → validated config files → applied through the daemon's own control channel → `Retrieve` from the daemon's
own output → events. Same pattern for every GPL daemon; P11 (strongSwan) is the reference. Protocol semantics (private MIB contents, two-node VRRP
failover on VPP interfaces, log explorer) belong to the F-* tasks — you deliver render / validate / apply / retrieve and prove each daemon accepts and
reports your config.

## Inputs to read first
- `apps/agent/internal/renderers/renderer.go` + `README.md` + `ALLOWLIST.md` (P05a) — the interface and helpers you implement
- Daemon docs: `man snmpd.conf`, `man snmpd`, `man keepalived.conf`, `keepalived --help` (check for `--enable-json`, `--dump-file-name`/`-D` options),
  rsyslog RainerScript docs (`omfwd`, `impstats`, `imuxsock`, `imtcp`, `rsyslogd -N`). Installed on this host (disabled): frr, strongswan (stock; vrx build
  comes from P11), kea-dhcp4/6 + kea-ctrl-agent, unbound, chrony, snmpd (+ `snmp` client tools), keepalived, rsyslog
- `packages/proto` messages for the domain (P03) — the input type (`system.snmp`, `ha.vrrp` keepalived variant, `logging.export`); missing fields → additive `contract/<id>`, questions file, continue
- `docs/lab/shared-host-rules.md` — you are `daemon-owner: snmpd, keepalived, rsyslog` for this task; slot prefix `w<N>`, addresses `10.<N>.0.0/16`, VR ids from
  your slot range; propose loopback ports in the slot's `3<N>xx` range (e.g. snmpd `3<N>61`, rsyslog imtcp `3<N>14`, test collector `3<N>15`) and record them

## Scope — build exactly this, per daemon
All paths come from one injected `Paths` struct (product: `/etc/snmp/snmpd.conf`, `/etc/keepalived/keepalived.conf`, `/etc/rsyslog.d/50-vrx-export.conf`;
tests: `/run/vrx-test/w<N>/{snmpd,keepalived,rsyslog}/…`). Apply's control channel is behind one `Controller` interface with two implementations:
systemd (`systemctl reload|restart <unit>`, product) and child-process (signal / restart the PID you spawned, tests).
1. Templates in `internal/renderers/<daemon>/templates/*.tmpl` rendered with `text/template` and **strict escaping helpers** — no user string reaches the file unescaped.
   - **snmpd**: `snmpd.conf` (`agentaddress udp:<ip>:<port>[,udp6:[<ip6>]:<port>]`, `sysName/sysLocation/sysContact/sysServices`, `view` definitions, `rocommunity/rocommunity6
     <community> <source> -V <view>`, v3: `createUser <user> SHA-256 "<auth>" AES "<priv>"` (goes into the persistent-store file the daemon reads once) + `rouser <user> priv -V <view>`,
     `trapsess`/`trap2sink`, `master agentx` + `agentXSocket <dir>/agentx.sock` for the future private-MIB subagent, `disk`/`load` monitors). Communities and passphrases are
     **secrets**: file 0600, redacted in logs and goldens (`VRX_TEST_PSK_<id>`). Escaping: tokens `[A-Za-z0-9_.-]{1,64}`, no whitespace/newline/quotes; OIDs numeric or from a
     fixed symbolic allow-list; sources typed CIDR; free-text fields (`sysLocation`, `sysContact`) printable ASCII ≤ 255 without newline.
   - **keepalived**: `keepalived.conf` (`global_defs { router_id, enable_script_security, script_user root, vrrp_version 3, vrrp_garp_master_refresh }`,
     `vrrp_script <name> { script "<fixed path>" interval weight fall rise }` — script paths only from the shipped allow-list (product `/usr/libexec/vrx/checks/*`, tests
     `<dir>/checks/*`), **never user-provided script text**; `vrrp_instance <name> { state, interface, virtual_router_id, priority, advert_int, preempt|nopreempt, preempt_delay,
     unicast_src_ip, unicast_peer {}, virtual_ipaddress { <ip>/<len> dev <if> }, virtual_routes, track_interface, track_script, notify_master|backup|fault|stop "<our helper>" }`,
     `vrrp_sync_group`). The `notify_*` target is *our* shipped helper (`vrx-keepalived-notify`, fixed path from `Paths`) that writes `<state dir>/<instance>.state` — that is the
     state and event channel. Escaping: names `[A-Za-z0-9_.-]{1,32}`, interfaces `w<N>-*` in tests, IPs typed, no quotes/newline/braces from user data.
   - **rsyslog**: one RainerScript file (`global(workDirectory=…)`, inputs `imuxsock` `SysSock.Name=<dir>/log.sock` or `imtcp` on `127.0.0.1:<port>`, `module(load="impstats"
     interval="1" format="json" log.file="<dir>/impstats.json" log.syslog="off")`, RFC 5424 `template()`, `ruleset()` per export target with facility/severity filters and
     `action(type="omfwd" target= port= protocol="udp|tcp" template= queue.type="LinkedList" queue.size= action.resumeRetryCount="-1")`). Allowed action types: `omfwd`, `omfile`
     (paths under `Paths` only). Never emit `omprog`, `omshell`, `omusrmsg`, `$IncludeConfig`, `include()` from user data. Escaping: RainerScript string quoting (`"`, `\`),
     targets typed IP/hostname, ports numeric, template names from a fixed set.
2. `Validate()` — snmpd: **no offline checker exists** (document): strict structural validation of the typed model; keepalived: `keepalived -t -f <cfg>`; rsyslog: `rsyslogd -N1 -f <cfg>`.
3. `Apply()` — write files atomically (temp + rename, correct owner/mode: snmpd 0600 root, keepalived 0640 root, rsyslog 0644 root), then apply via the control channel:
   snmpd `systemctl reload snmpd` (SIGHUP), keepalived `systemctl reload keepalived` (SIGHUP re-reads config), rsyslog `systemctl restart rsyslog` (rsyslog cannot reload
   config — HUP only reopens files; document the ~100 ms gap). Tests: `SIGHUP` / restart the child PID. **Fixed argv, no shell**; list every binary you invoke in
   `internal/renderers/ALLOWLIST.md` (`/usr/sbin/snmpd`, `/usr/sbin/keepalived`, `/usr/sbin/rsyslogd`, `/usr/bin/systemctl` product-only, `/usr/bin/ip` test-only, the notify helper).
4. `Retrieve()` — daemon state as structured data: snmpd via a Go SNMP client (`github.com/gosnmp/gosnmp`, BSD-2 — state in the PR why: communities/passphrases must never
   appear in argv) reading `sysName.0`, `sysDescr.0`, `sysUpTime.0`, `sysLocation.0` on `127.0.0.1:<port>`; keepalived: the `<instance>.state` files written by the notify helper
   (MASTER/BACKUP/FAULT + timestamp) plus `ip -j addr show dev <if>` for VIP presence (test-only cross-check); rsyslog: parse `<dir>/impstats.json` (per-action submitted/failed/
   suspended, queue sizes) into typed state.
5. Events where the daemon exposes them (poll at 1 Hz otherwise): keepalived state transitions arrive through the notify helper's state files (watch or poll at 1 Hz → `StreamEvents`);
   snmpd and rsyslog: poll at 1 Hz (sysUpTime reachability, impstats deltas) and emit on change.
6. Unit tests with golden files (`testdata/*.golden`) — every template path covered (v2c + v3 users + views + traps; one and two VRRP instances, unicast, sync group, tracking; udp+tcp
   exports with filters), including hostile strings: `"; rm -rf /`, `\nrocommunity public\n`, `script "/bin/sh -c …"`, `action(type="omprog" …)`, `"` and `\` in RainerScript strings, unicode,
   5 KB values → rejected or escaped, asserted; communities/passphrases never appear in logs or non-secret files.
7. Integration test on this host, **never through the system units or `/etc` paths**: start each daemon as a child process with a test-scoped config dir and pidfile:
   `snmpd -f -Lo -C -c <dir>/snmpd.conf -p <dir>/snmpd.pid udp:127.0.0.1:3<N>61` (env `SNMP_PERSISTENT_DIR=<dir>/persist SNMPCONFPATH=<dir>`), `ip netns exec ns-w<N>-a keepalived -n -l -P -f
   <cfg> -p <dir>/keepalived.pid -r <dir>/vrrp.pid -c <dir>/checkers.pid` (single instance on `w<N>-a`, VIP `10.<N>.240.1/24`, inside the rig namespace so VRRP adverts never reach
   `ens192`), `rsyslogd -n -iNONE -f <dir>/rsyslog.conf` (imuxsock at `<dir>/log.sock`, export `omfwd` → a Go UDP listener the test opens on `127.0.0.1:3<N>15`), bound **only to
   `127.0.0.1:<slot port>` or to rig veths inside `ns-<prefix>-*`** — assert the rendered interface/listen list before starting; never `ens192`. Checks: snmpd answers `sysName.0` with the
   rendered value, `SIGHUP` after a `sysLocation` change is reflected without a PID change; keepalived state file reaches MASTER within 5 s and the VIP is on `w<N>-a`, gone after kill;
   rsyslog forwards one message written to `log.sock` to the collector as RFC 5424 and `impstats` shows `submitted=1`. You own snmpd, keepalived, rsyslog for this task; kill by the PID
   you spawned; the system units stay stopped and disabled.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/{snmpd,keepalived,rsyslog}/...` green, integration included (paste the SNMP reply, the `.state` file, the collected RFC 5424 line)
- [ ] `grep -rn "sh -c\|bash -c" internal/renderers/{snmpd,keepalived,rsyslog}` is empty; `ALLOWLIST.md` updated; no `omprog`/`omshell`/user script paths reachable (test)
- [ ] A rendered config with `"; rm -rf /` in a description field is rejected or escaped (test present, per daemon)
- [ ] No child daemon left running after tests (`pgrep -f /run/vrx-test/w<N>/snmpd`, `…/keepalived`, `…/rsyslog` empty); `systemctl is-active snmpd keepalived rsyslog` unchanged from before
      (rsyslog may be the host's own active unit — never touch it; assert its PID is unchanged); `/etc/snmp`, `/etc/keepalived`, `/etc/rsyslog*` untouched (`stat` before/after)
- [ ] `grep -rn "VRX_TEST_PSK" <test log>` finds nothing outside the 0600 snmpd file

## Out of scope (do not build)
API/UI, schema changes beyond additive `contract/<id>`, private MIB / AgentX subagent contents (F-snmp), trap semantics and receivers, VPP-native VRRP
descriptors (DF-7), keepalived on linux-cp/VPP interfaces and two-node failover (F-vrrp), notify-script *contents* beyond writing the state file, log explorer
API/UI and log storage (F-logging), rsyslog TLS/RELP, journald `ForwardToSyslog` and logrotate (P10 packaging), FRR/strongSwan/Kea/Unbound/chrony (RF-1/2/3),
any write under `/etc/snmp`, `/etc/keepalived`, `/etc/rsyslog*`, `systemctl start/enable/restart` of the host's units, binding to `ens192`.
