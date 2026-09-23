# rsyslog renderer (RF-4, WBS D7.7 — syslog export)

`management.syslog[]` → one RainerScript file (+ TLS material files) → `rsyslogd -N1` → **restart** →
convergence from impstats → state from impstats → 1 Hz events. Mapping table: `docs/agent/renderers/rsyslog.md`.

## Paths and files

| | product (`ProductPaths`) | tests (`TestPaths("w8", 3814)`) |
|---|---|---|
| config | `/etc/rsyslog.d/50-vrx-export.conf`, `root:root 0644` — an include of the host's rsyslog | `/run/vrx-test/w8/rsyslog/rsyslog.conf`, standalone |
| impstats | `/var/spool/rsyslog/vrx-impstats.json` (rsyslog drops privileges to `syslog`) | `<dir>/impstats.json` |
| TLS material | `/etc/vrx/rsyslog-tls/export-<i>-{ca,cert}.pem` 0644, `-key.pem` `root:syslog 0640` `Secret` | `<dir>/tls/` |
| inputs, `global()` | none — the host's `/etc/rsyslog.conf` owns them (imuxsock, `$WorkDirectory`) | `imuxsock` on `<dir>/log.sock` (`SysSock.Use="off"`: never `/dev/log`), `imtcp` on `127.0.0.1:3<N>14`, `global(workDirectory=…)` |
| control channel | `systemctl restart rsyslog` | stop the child PID, start a new child |

Each target is a `ruleset(name="vrx_export_<i>_<hash>") { action(type="omfwd" …) }` called from the main flow
under `if prifilt("<facilities>.<severity>")`. The name carries an FNV-32a hash of the target's settings, so a
changed target reports to impstats under a new name — this is what lets Apply tell the new configuration from
the old one. Template: our fixed `vrx_rfc5424` (`<PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID SD MSG`) or the
built-in `RSYSLOG_TraditionalForwardFormat` (stand-in `format: rfc3164`); TCP RFC 5424 uses octet-counted framing
(RFC 6587). Queue: `LinkedList`, `queue.size` 10000 (stand-in `queueSize`), `action.resumeRetryCount="-1"`.

Only `omfwd` is ever emitted; never `omprog`, `omshell`, `omusrmsg`, `omfile`, `$IncludeConfig`, `include()`,
legacy `$` directives or backticks (tests assert on every golden). Every string value is RainerScript-quoted
(`Quote`: `"` and `\` escaped, control characters rejected) after its own typed validation (IP via `netip`,
hostname regex, numeric ports, facility/severity/template/auth-mode from fixed sets).

## rsyslog cannot reload

SIGHUP only reopens output files; a new configuration needs a restart. `systemctl restart rsyslog` stops and
starts the host's system logger: local senders block or buffer in the kernel socket for the ~100 ms gap
(imuxsock), journald keeps its own copy. The restart happens only when the rendered file changed (the commit
engine applies a renderer whose files differ) — documented for P10/F-logging.

## Validate

`rsyslogd -N1 -f <staged file>` (rejects unknown parameters, bad syntax — verified live). Two cases are handled
before that:
- no action at all (`management.syslog` empty): rsyslogd refuses configs without output (error -2103) although
  the empty export is valid → structural validation only;
- TLS targets: `-N1` does **not** load the netstream driver, so it accepts `StreamDriver="ossl"` on this host
  although `lmnsd_ossl.so` is not installed. Validate checks `<ModuleDir>/lmnsd_ossl.so` and refuses the export
  otherwise ("needs package rsyslog-openssl") — an honest failure instead of an action that could never connect.

## Apply / Retrieve / events

Apply: TLS dir → snapshot → atomic write → restart → **convergence**: impstats records stamped in a later second
than the moment the restart returned must name exactly the rendered actions (10 s). The old process writes a
last batch while stopping — seen live on the first attempt — which is why records are filtered by their own
timestamp, not only by file offset. On failure: restore + restart. TLS files of removed targets are deleted after
success.

impstats appends every second forever: reads are bounded (newest 256 KiB), and Retrieve truncates the file above
8 MiB (rsyslog writes with `O_APPEND`; an Apply in progress holds the lock so its window is never truncated).
logrotate for it is P10.

Retrieve: per rendered action `reported`, `processed`, `failed`, `suspended`, `suspendedDuration`, `resumed`,
queue `size`/`enqueued`/`full`/`discarded*`/`maxqsize`; `inputs` (`submitted` per input); `error`.
`Poller()`: per action `reported`, `failed`, `suspended`, `discarded` and `queue` = `empty|backlog` — a collector
that stops accepting shows up as a backlog (rsyslog keeps retrying, `suspended` stays 0 for a while; seen live).

## Secrets

TLS CA/certificate (`cert/…`) and private key (`key/…`) are D-051 references resolved at Render, PEM-shaped
(≤ 64 KiB). The key is written only to its own file (0640, owner `root:syslog` in the product — rsyslog opens it
after dropping privileges), marked `Secret`; the config names the files, never the material. The Redactor also
remembers every substantial line of a multi-line secret, so a tool that echoes a key re-wrapped (e.g. joined with
`|`) is still masked — a failing unit test found that case.

## Integration test

`TestRsyslogIntegration`: child `rsyslogd -n -iNONE -f <dir>/rsyslog.conf` (no pidfile, killed by PID), Go
collectors on `127.0.0.1:3<N>15` (UDP) and `:3<N>16` (TCP). The rendered inputs are asserted to be the slot socket
and `127.0.0.1` only before start. Checks: `-N1` accepts and rejects a bogus parameter; TLS refused on this host;
messages written to `log.sock` (and one via imtcp) arrive as RFC 5424 (UDP) and octet-counted RFC 5424 (TCP);
facility/severity filters hold; impstats shows the counts; a second commit restarts (new PID) and converges; a
restart that never happens is refused and rolled back; a closed collector produces a `backlog` event; the host's
`rsyslog.service` MainPID is unchanged.
