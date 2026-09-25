# rsyslog renderer — desired state ↔ rendered directives (RF-4)

Code: `apps/agent/internal/renderers/rsyslog` (README there: restart semantics, impstats convergence, TLS, tests).
File: `/etc/rsyslog.d/50-vrx-export.conf`, `root:root 0644` (no secret), an include of the host's rsyslog; TLS
material in `/etc/vrx/rsyslog-tls/` (key `root:syslog 0640`). Validated with `rsyslogd -N1 -f <staged>`; applied
with `systemctl restart rsyslog` (rsyslog cannot reload a configuration); state from impstats.

Stand-ins (D-055) on `management.syslog[i]`: `facilities`, `format`, `queueSize`, `tls` (RF-4-questions.md Q2).

| desired state (JSON path) | rendered | validation |
|---|---|---|
| any target present | `module(load="impstats" interval="1" format="json" log.file="<stats>" log.syslog="off" resetCounters="off")` — **omitted** when the host rsyslog already loads impstats (then its JSON `log.file` is read) — + `template(name="vrx_rfc5424" …)` | fixed |
| `management.syslog` empty | a comment only (nothing exported; no impstats) | — |
| `management.syslog[i]` | `ruleset(name="vrx_export_<i>_<hash>") { action(type="omfwd" name=… …) }` + `if prifilt("<sel>") then { call vrx_export_<i>_<hash> }` | ≤ 16 targets, unique (host, port, protocol) |
| `.address` | `target="<ip or hostname>"` | `netip` (not unspecified/multicast) or hostname (lower-cased) |
| `.port` | `port="<n>"` | 1–65535 (default 514) |
| `.protocol` udp / tcp / tls | `protocol="udp"` / `protocol="tcp" TCP_Framing="octet-counted"` / tcp + TLS driver below | fixed set |
| `.severity` | `prifilt("*.<sev>")` — emergency→emerg, critical→crit, error→err, … (that severity and above) | fixed set (default info) |
| `.facilities[]` *(stand-in)* | `prifilt("<f1>,<f2>.<sev>")` | kern, user, mail, daemon, auth, syslog, lpr, news, uucp, cron, authpriv, ftp, local0–7 |
| `.format` *(stand-in)* rfc5424 / rfc3164 | `template="vrx_rfc5424"` / `template="RSYSLOG_TraditionalForwardFormat"` + `TCP_Framing="traditional"` | fixed set |
| `.queueSize` *(stand-in)* | `queue.size="<n>"` (default 10000); always `queue.type="LinkedList" action.resumeRetryCount="-1"` | 100–1000000 |
| `.tls.{caRef,certRef,keyRef}` *(stand-in; cert/…, cert/…, key/…)* | files `export-<i>-ca.pem`, `-cert.pem`, `-key.pem`; `StreamDriver="ossl" StreamDriverMode="1" StreamDriver.CAFile/CertFile/KeyFile="<file>"` | CA required; cert+key together; PEM ≤ 64 KiB; the ossl driver must be installed (Validate) |
| `.tls.authMode`, `.tls.permittedPeers[]` *(stand-in)* | `StreamDriverAuthMode="x509/name\|x509/certvalid"`, `StreamDriverPermittedPeers="a,b"` | anonymous TLS refused; peers are hostnames |
| `.vrf` | — | only `default` |

Every string is RainerScript-quoted (`"`/`\` escaped, control characters rejected). Only `omfwd` actions; never
`omprog`, `omshell`, `omusrmsg`, `omfile`, `$IncludeConfig`, `include()`. Tests (standalone) additionally render
`global(workDirectory)`, `imuxsock` on the slot socket (`SysSock.Use="off"`) and `imtcp` on `127.0.0.1:<port>`.

Retrieve per action: `reported`, `processed`, `failed`, `suspended`, `suspendedDuration`, `resumed`, queue
`size`/`enqueued`/`full`/`discardedFull`/`discardedNf`/`maxQueueSize`; `inputs`; `error`.

## In the agent (F-unbound-chrony-syslog)

Singleton descriptor `rsyslog.config/vrx` (`renderers/rsyslog/descriptor.go`), domain `management` (first user of the domain;
its other leaves are `agent.unsupported-field`):

| | |
|---|---|
| Value | `*vrxv1.ManagementConfig{syslog}` (`rsyslog.Input`); no object without a target |
| D-086 keys | `facilities`, `format`, `queue_size`, `tls` are read from the typed proto (SyslogTarget 6–9); a `*structpb.Struct` input keeps the strict RF-4 stand-in checks |
| Create / Update | Render → Validate (`rsyslogd -N1`) → Apply (product: restart + impstats convergence; unchanged files: nothing) |
| Delete | the empty export |
| Retrieve | embedded input in the export file → re-rendered → the file (and TLS files) byte-equal |
| Refused at DryRun | `tls` (`agent.secret-channel-pending`); a non-default `vrf` is noted (`agent.unsupported-field`) and not applied |
| Ownership | `RecordsNoOwnership()` (TD-11b) |

Non-owner agents render a standalone configuration under `<base>/rsyslog` (imuxsock on `log.sock`, never `/dev/log`) driven by a
`DeferredController`: Apply writes the files and records a restart request (a start when no instance runs); the pidfile
`<base>/rsyslog/rsyslogd.pid` (`rsyslogd -i`, verified to run `/usr/sbin/rsyslogd`) tells when it was acted on. The agent never
restarts or signals an rsyslogd it did not start, and never the host's.
