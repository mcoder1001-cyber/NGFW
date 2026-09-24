# RF-4 — questions and decisions for the manager

Worker slot 8 (w8), branch `task/RF-4`. Nothing below blocks the branch; each item states what was done.

## Q1 — files outside the envelope's list (process)
The envelope lists `internal/renderers/{snmpd,keepalived,rsyslog}/**`. I also touched:
- **`apps/agent/internal/renderers/rfkit/`** (new package): Controller (systemd + child-process), secret refs +
  Redactor, `ApplyFiles` (snapshot → write → activate → convergence → rollback on an own context), bounded reads,
  1 Hz Poller, D-055 stand-in navigation. Options: (a) one shared package, like `descriptors/dfkit` (D-077) —
  **chosen**; (b) three copies inside the daemon packages (≈ 600 lines × 3, the redaction/rollback fixes would
  drift). New directory only, no conflict with any other branch.
- **`apps/agent/go.mod` / `go.sum`**: `github.com/gosnmp/gosnmp v1.43.1` (BSD-2), required by the task prompt
  (Retrieve over SNMP without putting communities/passphrases in an argv).
- **`internal/renderers/ALLOWLIST.md`**: rows only (expected by the envelope): three Planned rows moved to
  Active, new rows for rsyslogd, the notify helper, the checks dir, the rsyslog module dir and the test-only `ip`.

## Q2 — contract gaps, handled with D-055 stand-ins (P03b / additive `contract/` branch)
Read from a `*structpb.Struct` document at the path the schema would use; strict (unknown keys are errors).
Needed in `packages/schema` + proto for the UI/API to reach them:
- `services.snmp`: `views.<name>{include[],exclude[]}`, `communities.<n>.view`, `v3Users.<n>.view`,
  `sysServices`, `monitors{disks[{path,minPercent}], load{max1,max5,max15}}`.
- `ha.keepalived{routerId, garpMasterRefresh, scripts.<name>{check,interval,weight,fall,rise}, syncGroups.<name>[]}`
  and `ha.vrrp.<n>.keepalived{authRef (psk/), preemptDelay, unicastSrcIp, prefixLength, virtualRoutes[], trackScripts[]}`.
  Note `ha.vrrp.<n>.addresses` are bare IPs: without `prefixLength` a VIP is /32 (/128).
- `management.syslog[i]`: `facilities[]`, `format` (rfc5424|rfc3164), `queueSize`, `tls{caRef (cert/),
  certRef (cert/), keyRef (key/), authMode, permittedPeers[]}` — the schema already allows `protocol: "tls"` but
  has no place for the TLS material.

## Q3 — VRRP authentication means VRRPv2 (decision D-RF4-3)
RFC 5798 (VRRPv3) has no authentication; keepalived 2.3.4 says "VRRP version 3 does not support authentication.
Ignoring." and `-t` fails. Options: (a) an instance with `authRef` is rendered `version 2` + PASS (IPv4, whole
seconds, key ≤ 8 characters) — **chosen**; (b) refuse authentication entirely; (c) global `vrrp_version 2`.
PASS is cleartext on the wire (misconfiguration guard only) — F-vrrp should say so in the UI.
**Fixture exception:** an 8-character key cannot hold the `VRX_TEST_PSK_<id>` literal; the tests use `RF4tpskA`,
`RF4tpskB`, `RF4tpsk8` (obviously fake, no secret shape; gitleaks clean). Please accept or name another rule.

## Q4 — `acceptMode: false` is not enforced for keepalived instances
keepalived's non-strict default accepts packets to the VIPs; `no_accept`/`vrrp_strict` make keepalived install
nftables/iptables rules on the host, which the renderer never renders (firewall owner: F-host-acl-nftables,
D-057). `acceptMode: true` renders `accept`. For F-vrrp + F-host-acl-nftables to decide.

## Q5 — rsyslog TLS: the task file says out of scope, the manager's instruction says "TLS syslog keys are secret refs"
Done: TLS targets render (`StreamDriver="ossl"`, CA/cert/key files from `cert/`/`key/` references, key
`root:syslog 0640`, Secret), `rsyslogd -N1` checks them. `-N1` does not load the netstream driver, so Validate also
requires `lmnsd_ossl.so` — **not installed on this host** (no `rsyslog-openssl`/`-gnutls`), so a TLS export is
refused here with a clear error and could not be tested end to end. P10: ship `rsyslog-openssl`.

## Q6 — snmpd `createUser` in the main snmpd.conf, not the persistent store (decision D-RF4-4)
The task text says the persistent-store file. Measured: with `-C` snmpd never reads the store; without `-C` snmpd
consumes and rewrites that file itself (a rendered copy drifts, and Apply could not be idempotent). In the main
file `createUser` is re-read on SIGHUP and a passphrase change applies. Options: (a) main file — **chosen**;
(b) persistent store (drift, restart needed for changes).

## Q7 — snmpd Validate is a parse run of the real daemon (decision D-RF4-5)
The task file: "no offline checker exists — structural validation"; the manager's instruction: "parse run".
Implemented: a daemonised `snmpd -C` on a staged check copy (listen/AgentX → unix sockets in the staging dir, trap
sinks removed), log checked, instance stopped by its verified PID. It catches unknown tokens, bad OIDs, short
passphrases (verified). Cost ≈ 0.2 s per commit and a short-lived root snmpd process during validation.
Options: (a) parse run — **chosen**; (b) structural only; (c) `snmpd -H` token list only.

## Q8 — P10 packaging items found here
(review M3) The host rsyslog base config must not load a non-JSON impstats: either no impstats in
`/etc/rsyslog.conf`/`rsyslog.d` (the export file loads it) or `format="json"` + an absolute `log.file` readable by the
agent; the renderer detects both and refuses the legacy form. snmpd is restarted by the engine on a restart request
(listen changes): the unit's ExecReload (SIGHUP) is not enough for those.

`/usr/libexec/vrx/vrx-keepalived-notify` (+ `checks/`), `TMPDIR` for keepalived.service (dump files hold
`auth_data`), dirs `/run/vrx/keepalived`, `/run/vrx/snmpd`, `/etc/vrx/rsyslog-tls`, logrotate (or the agent's
truncation) for `/var/spool/rsyslog/vrx-impstats.json`, package `rsyslog-openssl`. The Ubuntu snmpd unit reads the
persistent store (no `-C`) — keep it that way (engineBoots persistence matters for USM).

## Q9 — non-default VRFs rejected
`services.snmp.vrf`, `ha.vrrp.<n>.vrf` (keepalived), `management.syslog[i].vrf` other than `default` are errors
("F-snmp / F-vrrp / F-logging bind the daemon to a VRF"). vdom.md guardrail #1 keeps the field explicit.

## Q10 — events
No daemon-specific `EventKind` exists: changes are `EVENT_KIND_UNSPECIFIED` with `source/key/old/new` attributes,
poll failures `EVENT_KIND_ERROR` (same choice as RF-1 Q3).
