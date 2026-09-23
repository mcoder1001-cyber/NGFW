# RF-4 — WIP (renderers: snmpd, keepalived, rsyslog) — slot 8 (w8)

Updated: 2026-09-24 02:30 (+03:30) — DONE, see RF-4.md

## Host facts established by experiment (01:33–01:40)
- Baseline before any test: snmpd inactive/disabled, keepalived inactive/disabled, rsyslog active/enabled MainPID 1014;
  `/etc/snmp`, `/etc/keepalived`, `/etc/rsyslog*` stat recorded (scratchpad baseline.txt, pasted into RF-4.md at the end).
- net-snmp 5.9.4.pre2: `-C` makes snmpd ignore the persistent store (`createUser` in `$SNMP_PERSISTENT_DIR/snmpd.conf` is
  never read with `-C`); `createUser` in the main snmpd.conf works, and a passphrase change takes effect on SIGHUP.
  A removed user stays in USM memory until restart, but loses its VACM `rouser` line on SIGHUP (no access).
  `agentaddress` in the file + an endpoint on argv = double bind → "Error opening specified endpoint": the test does not
  pass an endpoint on argv (the rendered `agentaddress` is the only listen list). `snmpd -C -H` lists the directives the
  daemon understands (lower-case). `sysLocation "x"` keeps the quotes → rest-of-line, no quoting. Privacy: DES|AES only
  (no AES-256 in this build).
- keepalived 2.3.4: `-t` checks interface existence in the current netns → `-t -s <ns>` works (no `ip netns exec`
  trampoline). `/run` is `noexec`: notify/check executables cannot live under /run/vrx-test (tests put them in a
  root-owned 0755 dir under /tmp). VRRPv3 has no authentication ("does not support authentication. Ignoring." rc 5) →
  auth needs `version 2` per instance and `auth_pass` ≤ 8 chars. SIGJSON (`--signum=JSON` = 36) writes
  `$TMPDIR/keepalived.json` (mode 0600) **including `auth_data` in plaintext** → Retrieve redacts and deletes it.
  SIGHUP reloads in place (same PID). With `-s <ns>` keepalived creates `/run/keepalived/<ns>/` (removed on exit).
- rsyslog 8.2512: `-N1` accepts `StreamDriver="ossl"` although `lmnsd_ossl.so`/`lmnsd_gtls.so` are not installed →
  TLS cannot run on this host; `-N1` rejects unknown parameters (rc 1) and configs with no action (-2103).
  impstats `format="json"` appends every interval (unbounded growth → bounded tail read + truncation).

## Plan / status
- [x] experiments, go.mod: github.com/gosnmp/gosnmp v1.43.1 (task-mandated; secrets never in argv)
- [x] keepalived notify helper `internal/renderers/keepalived/cmd/vrx-keepalived-notify`
- [x] rfkit shared helpers (Controller systemd/process, secrets+redactor, apply/rollback, bounded read, poller)
- [x] snmpd renderer + goldens + hostile + integration (parse-run Validate, gosnmp Retrieve, SIGHUP same PID)
- [x] keepalived renderer + goldens + hostile + integration (netns ns-w8-a, veth w8-a/w8-b; MASTER in 1.8 s)
- [x] rsyslog renderer + integration (UDP+TCP collectors, impstats, restart convergence by timestamped impstats)
- [x] rsyslog unit tests (goldens, hostile, TLS secret files)
- [x] ALLOWLIST.md rows, docs/agent/renderers/*.md, questions file, RF-4.md, CI gate PASSED

Slot ports: snmpd 127.0.0.1:3861, rsyslog imtcp 127.0.0.1:3814, test collector 127.0.0.1:3815; VRIDs 81–89; VIP 10.8.240.1/24.
