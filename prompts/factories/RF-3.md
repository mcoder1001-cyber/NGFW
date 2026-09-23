# Task: RF-3 — Renderers for daemons: kea-dhcp4/6 + ctrl-agent, unbound, chrony   (prepend 00-CONTEXT.md)

## Goal
Write the agent-side **renderer** for each of Kea DHCPv4/v6 + control agent (WBS D7.1), Unbound (D7.3) and chrony (D7.4): desired state →
validated config files → applied through the daemon's own control channel → `Retrieve` from the daemon's JSON/show output → events. Same
pattern for every GPL daemon; P11 (strongSwan) is the reference. Protocol behaviour (a real DHCP lease, DNSSEC validation against the root, NTP
sync quality) belongs to the F-* tasks — you deliver render / validate / apply / retrieve and prove each daemon accepts and reports your config.

## Inputs to read first
- `apps/agent/internal/renderers/renderer.go` + `README.md` + `ALLOWLIST.md` (P05a) — the interface and helpers you implement
- Daemon docs: Kea ARM for the installed version (`kea-dhcp4 -v`; Kea ≥ 2.7.2/3.0 uses `control-sockets` lists, can serve HTTP directly and deprecates
  kea-ctrl-agent — decide per installed version, record it), `man unbound.conf`, `man unbound-control`, `man chrony.conf`, `man chronyc`, `chronyd --help`.
  Installed on this host (disabled): frr, strongswan (stock; vrx build comes from P11), kea-dhcp4/6 + kea-ctrl-agent, unbound (+ `dns-root-data`
  `/usr/share/dns/root.key`), chrony, snmpd, keepalived, rsyslog
- `packages/proto` messages for the domain (P03) — the input type (`services.dhcp`, `services.dns`, `services.ntp`); missing fields → additive `contract/<id>`, questions file, continue
- `docs/lab/shared-host-rules.md` — you are `daemon-owner: kea, unbound, chrony` for this task; slot prefix `w<N>`, addresses `10.<N>.0.0/16`; propose your
  loopback ports in the slot's `3<N>xx` range (e.g. unbound `3<N>53`, chrony `3<N>23`, kea-ctrl-agent `3<N>80`) and record them in your status file

## Scope — build exactly this, per daemon
All paths come from one injected `Paths` struct (product: `/etc/kea/*.conf`, `/etc/unbound/unbound.conf`, `/etc/chrony/chrony.conf`, `/etc/chrony/chrony.keys`;
tests: `/run/vrx-test/w<N>/{kea,unbound,chrony}/…`). Kea configs are JSON: render them by marshalling typed Go structs with `encoding/json` (native escaping),
not `text/template`; golden files still apply. Unbound and chrony are line-based: `text/template` + strict escaping.
1. Templates in `internal/renderers/<daemon>/templates/*.tmpl` rendered with `text/template` and **strict escaping helpers** — no user string reaches the file unescaped.
   - **kea**: `kea-dhcp4.conf` / `kea-dhcp6.conf` (`interfaces-config.interfaces` explicit `name/addr` list; `control-socket(s)` unix `<dir>/kea4.sock`; `lease-database`
     memfile `<dir>/leases4.csv` + lfc-interval; `subnet4/6[]` with pools, `option-data`, reservations by hw-address/duid/client-id, relay; `option-def`; `valid-lifetime`
     / renew / rebind; `hooks-libraries` = `libdhcp_lease_cmds.so` discovered under `/usr/lib/x86_64-linux-gnu/kea/hooks/` at runtime; loggers to `<dir>`), `kea-ctrl-agent.conf`
     (`http-host` `127.0.0.1`, `http-port` slot port, control-sockets). Escaping: JSON via `encoding/json`; still validate hostnames, option strings (printable ASCII, ≤ 255),
     MAC/DUID hex, and that every interface name is `w<N>-*` in tests.
   - **unbound**: one `unbound.conf` (`server:` directory/chroot ""/username ""/pidfile/`interface: <ip>@<port>`/`port`/access-control/`auto-trust-anchor-file`/harden-*/
     cache sizes/`do-daemonize: no` in tests, `remote-control:` `control-interface: <dir>/unbound.ctl`, `control-use-cert: no`, `forward-zone:` (name, forward-addr with
     `@port`, forward-tls-upstream), `stub-zone:`, `local-zone:` + `local-data:` built from typed RRs, `view:` blocks with `view-first`). Escaping: values quoted, no newline
     / `"` / leading `include:`; domain names validated as DNS names; the token `include` is never emitted from user data.
   - **chrony**: `chrony.conf` (`bindaddress`, `port`, `bindcmdaddress <dir>/chronyd.sock`, `cmdport 0`, `driftfile`, `pidfile`, `keyfile`, `ntsdumpdir`, `makestep`, `allow`/`deny`,
     `local stratum` for server mode, `sourcedir <dir>/sources.d`, `log`) + `sources.d/vrx.sources` (`server|pool|peer <host> iburst minpoll maxpoll key <id> nts`) +
     `chrony.keys` (0600, ids + `SHA256 HEX:…`; fixtures `VRX_TEST_PSK_<id>`). Escaping: hostnames/IPs typed; option keywords from an allow-list; no free text.
2. `Validate()` — `kea-dhcp4 -t <cfg>`, `kea-dhcp6 -t <cfg>`, `kea-ctrl-agent -t <cfg>`; `unbound-checkconf <cfg>`; `chronyd -p -f <cfg>` (prints parsed config and exits on
   chrony ≥ 4 — confirm with `chronyd --help`; if absent, structural validation only, documented).
3. `Apply()` — write files atomically (temp + rename, correct owner/mode: kea `_kea` 0640, unbound `unbound` 0640, chrony `_chrony` 0640 + keys 0600 in product; root in tests),
   then apply via the control channel: **Kea** JSON commands over the unix control socket from Go (`config-set` with the rendered config, then `config-write`; ctrl-agent HTTP
   `config-set` on `127.0.0.1:<port>` only where the product needs the remote path) — **Unbound** `unbound-control -c <cfg> reload_keep_cache` (≥ 1.13, else `reload`) —
   **chrony** `chronyc -h <dir>/chronyd.sock reload sources` for source changes; any other directive needs a daemon restart: return a typed `NeedsRestart` result the
   commit engine surfaces (product: `systemctl restart chrony`, tests: restart your child). **Fixed argv, no shell**; list every binary you invoke in
   `internal/renderers/ALLOWLIST.md` (`/usr/sbin/kea-dhcp4`, `/usr/sbin/kea-dhcp6`, `/usr/sbin/kea-ctrl-agent`, `/usr/sbin/unbound`, `/usr/sbin/unbound-checkconf`,
   `/usr/sbin/unbound-control`, `/usr/sbin/chronyd`, `/usr/bin/chronyc`, `/usr/bin/systemctl` product-only, `/usr/bin/ip` test-only).
4. `Retrieve()` — daemon state as structured data: Kea `config-get`, `status-get`, `statistic-get-all`, `lease4-get-all` / `lease6-get-all` (page with `lease4-get-page`
   above 1k leases — never one giant message); Unbound `unbound-control -c <cfg> status`, `stats_noreset`, `list_forwards`, `list_stubs`, `list_local_zones`, `list_local_data`;
   chrony `chronyc -h <sock> -c sources`, `-c sourcestats`, `-c tracking`, `-c serverstats` (CSV → typed).
5. Events where the daemon exposes them (poll at 1 Hz otherwise): none of the three pushes events — poll `statistic-get` (pkt4-received, pool utilisation),
   `stats_noreset` (num.queries), `tracking` (stratum/leap/offset) and emit on change.
6. Unit tests with golden files (`testdata/*.golden`) — every template path covered (v4+v6 subnets with reservations and options, ctrl-agent; forward/stub/local/view;
   client-only and server-mode chrony with keys/NTS), including hostile strings: `"; rm -rf /`, `\ninclude: /etc/passwd`, `"}]}` JSON break-out, unicode, 5 KB values →
   rejected or escaped, asserted; chrony keys never appear in `chrony.conf` or logs.
7. Integration test on this host, **never through the system units or `/etc` paths**: start each daemon as a child process with a test-scoped config dir and pidfile:
   `ip netns exec ns-w<N>-a kea-dhcp4 -c <cfg>` (env `KEA_PIDFILE_DIR=<dir> KEA_LOCKFILE_DIR=<dir>`; interfaces `w<N>-a/10.<N>.10.1` inside the rig namespace),
   `unbound -c <cfg> -d` on `127.0.0.1:3<N>53`, two `chronyd -f <cfg> -n -x` (server: `local stratum 10`, `port 3<N>23`, `allow 127.0.0.1`; client: `server 127.0.0.1 port 3<N>23
   iburst`, `port 0`; `-x` = no clock changes, mandatory), bound **only to `127.0.0.1:<slot port>` or to rig veths inside `ns-<prefix>-*`** — assert the rendered
   interface/listen list before starting; never `ens192`. Checks: Kea `config-get` equals the rendered config (normalised) and `lease4-get-all` returns an empty list;
   Unbound `list_forwards` shows the rendered zone and Go's `net.Resolver` pointed at `127.0.0.1:3<N>53` resolves a rendered `local-data` name; chrony client `sources`
   lists the server and `tracking` shows a non-zero stratum within 30 s; then change → Apply → Retrieve reflects it. You own kea, unbound, chrony for this task; kill by
   the PID you spawned; the system units stay stopped and disabled.

## Acceptance (paste the evidence)
- [ ] `go test ./internal/renderers/{kea,unbound,chrony}/...` green, integration included (paste `config-get` diff = empty, `list_forwards`, `chronyc -c tracking`)
- [ ] `grep -rn "sh -c\|bash -c" internal/renderers/{kea,unbound,chrony}` is empty; `ALLOWLIST.md` updated
- [ ] A rendered config with `"; rm -rf /` in a description field is rejected or escaped (test present, per daemon); `include:` injection rejected for unbound
- [ ] No child daemon left running after tests (`pgrep -f /run/vrx-test/w<N>/kea`, `…/unbound`, `…/chrony` empty); `systemctl is-active kea-dhcp4-server kea-dhcp6-server
      kea-ctrl-agent unbound chrony` all `inactive`; `/etc/kea`, `/etc/unbound`, `/etc/chrony` untouched (`stat` before/after); system clock never stepped (`-x` in argv)

## Out of scope (do not build)
API/UI, schema changes beyond additive `contract/<id>`, DHCP relay/client and the VPP caching DNS plugin (DF-8), DHCP lease browser API/UI and a real
DORA exchange (F-dhcp), DNSSEC validation runs against the Internet and DNS filtering/blocklists (F-dns), PTP and NTS server certificates (F-ntp), Kea
MySQL/PostgreSQL lease backends, `kea-dhcp-ddns`, Kea HA hooks, unbound `dnstap`, FRR/strongSwan/snmpd/keepalived/rsyslog (RF-1/2/4), any write under
`/etc/kea`, `/etc/unbound`, `/etc/chrony`, `systemctl start/enable` of any of them, binding to `ens192`, changing the host clock.
