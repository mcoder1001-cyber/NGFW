# RF-4 review — renderers: snmpd, keepalived, rsyslog + rfkit

Reviewer: independent review agent (did not write this code). Branch `task/RF-4` on `main@ff6b91a`,
worktree `/root/ngfw-wt/RF-4`, run directly on the host, slot 8.

## What I ran
- `tools/ci.sh --base main` myself → **CI GATE PASSED** (quick, ~1m01s, log
  `/root/ngfw-wt/logs/ci/RF-4-20260924-022333-1997412`). Matches the output pasted in `RF-4.md`.
- `VRX_INTEGRATION=1 go test -run Integration ./internal/renderers/{snmpd,keepalived,rsyslog}/` on
  slot 8 under the lab lock → all three PASS (snmpd 3.99s, keepalived 9.92s, rsyslog 7.39s). SNMP
  reply, `vi1.state` MASTER, RFC-5424 line and impstats all reproduced; the three "apply without a
  reload/restart is refused and rolled back" cases fired.
- Hygiene after the runs: `/etc/snmp`, `/etc/keepalived`, `/etc/rsyslog.conf`, `/etc/rsyslog.d`
  byte-for-byte unchanged (stat diff empty); `snmpd`/`keepalived` inactive, host `rsyslog` still
  `active` MainPID **1014 unchanged**; no `ns-w8-*`, `/run/vrx-test/w8` empty, no stray snmpd/
  keepalived/rsyslogd under `/proc`.
- Adversarial probe of my own: a staged snmpd (my temp dir under `/run/vrx-test/w8`, killed and
  removed) with `agentaddress udp:127.0.0.1:3862`, then edited to `3863` + SIGHUP. Result below.

## Findings, ranked by severity

### H1 — snmpd `Apply` uses SIGHUP for `agentaddress`/bind/port changes, but snmpd does not reopen sockets on SIGHUP; the convergence check only proves sys* values, not the listen set
`snmpd/renderer.go:375-398` (Apply → `r.ctl.Reload`, SIGHUP) with the convergence check in
`snmpd/state.go:123-131` (`matches` = sysName/sysLocation/sysContact only) and the queried endpoint
parsed from the *rendered* file in `snmpd/state.go:211-231` (`localEndpoint`).

**Proof (direct, this host):** snmpd on `udp:127.0.0.1:3862`, edited to `3863`, `kill -HUP`. After the
HUP the daemon still answered on **3862** with the *new* `sysLocation` ("two"), and **3863 was
refused** (`connection refused`). SIGHUP re-reads sys* in place but never reopens the listening
socket — exactly the D-079 / RF-3 H1 / RF-1 H2 pattern.

**Failure scenario, two directions:**
- **False success (security):** operator narrows the default `udp:0.0.0.0:161` to a single address
  `udp:10.x.x.x:161`. `localEndpoint` picks that address; the query reaches the still-open `0.0.0.0`
  socket and sys* match → **Apply reports converged** while snmpd keeps answering on *all* interfaces.
  The intended restriction silently did not take effect.
- **False failure:** a pure port change → the query goes to the new port, which is refused →
  `ErrNotConverged` → rollback. The bind/port can never be applied through the normal path, and the
  rollback SIGHUP does not restore it either.

The README (`snmpd/README.md:12,50-54`) documents SIGHUP + the sys* convergence but never states that
a listen change is not honoured.

**Fix:** diff the previous rendered file against the new one; when `agentaddress` (or `engineID` /
`agentXSocket`) changed, either use `Restart` or return a persisted restart-request
(`ActionRequired{restart}`, the chrony M2 / D-079 pattern the LOG says "applies to every renderer"),
and extend the convergence proof to the actually-bound socket set — not only sys* on one endpoint.

### M1 — snmpd default listen is `0.0.0.0:161` + `[::]:161` when `services.snmp.listen` is empty
`snmpd/model.go:324-328`. The task explicitly asks "bind addresses (never 0.0.0.0 by default?)". On an
NGFW this exposes SNMP on every interface (mgmt + any data-plane linux-cp side) out of the box.
**Fix:** default to loopback (`127.0.0.1`/`::1`) or require an explicit `listen`; warn on `0.0.0.0`.
(Good: there is **no** implicit `public`/`private` community — communities exist only when configured.)

### M2 — rsyslog `Apply` restarts rsyslog unconditionally, even for an unchanged config → repeated log loss (idempotency, D-076)
`rsyslog/renderer.go:310-318` (`activate` always calls `r.ctl.Restart`). A rsyslog restart drops the
~100 ms of local logs (documented, `README:31`). Because the renderer never compares the on-disk file
set to the rendered one, an idempotent re-commit (or any commit that does not touch syslog) still
restarts the host's own system logger and loses messages. Violates D-076 (idempotent write-only).
**Fix:** short-circuit when the config + TLS files already match on disk; restart only on a real delta.

### M3 — rsyslog product file loads `impstats` inside an include of the host rsyslog; a second impstats load fails the whole logger
`rsyslog/templates/rsyslog.conf.tmpl` emits `module(load="impstats" …)` and the product config is
`/etc/rsyslog.d/50-vrx-export.conf`, an include of the host's `/etc/rsyslog.conf` (`README:10`). If the
base config or another drop-in already loads impstats, `rsyslogd` errors on module re-load and, after
the Apply restart, the host logger does not come back. `rsyslogd -N1` runs on the staged file alone and
cannot see the conflict. **Fix (P10/F-logging):** own impstats once globally, or detect the host's
impstats before emitting ours; document the packaging requirement. On the board, does not block the
branch's own tests.

### L1 — Redactor masks every secret *substring* everywhere (RF-2 L2 recurrence)
`rfkit/secrets.go:149-167` (`Redact` = `ReplaceAll` per value). `checkCommunity` allows a community of
length 1 (`snmpd/model.go:158-162`, `tokenRe{1,64}`). A short or common-word community then blanks
every matching substring in Retrieve/State/events, corrupting output and giving an oracle.
**Fix:** raise the minimum community length and/or redact whole tokens/values only.

### L2 — snmpd parse-run problem detection is a broad keyword regex with a small allow-list
`snmpd/renderer.go:289-323` flags any log line matching `error|warning|unknown|line N|cannot|failed|
invalid|bad ` minus six benign patterns. A legitimate but noisy snmpd warning outside the allow-list
would false-reject a valid commit. Common cases pass (integration green; the benign "duplicate
registration … ipAddressTable" line I saw is correctly *not* matched), but the heuristic is fragile.
**Fix:** key on snmpd's structured `line N: Error:` form, or grow the benign list from observed output.

### L3 — keepalived `Retrieve` sends a SIGJSON dump on every call
`keepalived/state.go:544-560` → `dump()` signals the daemon and writes/reads/removes a dump holding
`auth_data` each Retrieve. The dump is whitelisted and deleted (good), but frequent Retrieve polling
adds signal + dump-file churn. The 1 Hz `Poller` correctly reads only state files. Consider caching or
rate-limiting the dump.

## Checks that passed (no finding)
- **Injection (task focus 6):** snmpd template emits **no** `exec`/`extend`/`pass` directive at all;
  keepalived renders only our shipped `vrx-keepalived-notify` and allow-listed checks
  (`model.go:196-200`, `checkRe` + `o.Checks`), never `include`/`$VAR`/`vrrp_strict`/`use_vmac`/user
  script text; rsyslog emits only `omfwd`, never `omprog`/`omshell`/`omusrmsg`/`omfile`/`$IncludeConfig`/
  `include()`. Every free-text value passes a typed validator in `BuildModel` **and** a strict template
  helper (tokens reject whitespace/quote/brace/newline; sysLocation/sysContact = printable ASCII on one
  line; RainerScript strings escaped by `Quote`). Goldens include `"; rm -rf /`, CR/LF/NUL, the
  `action(type="omprog")` string, `$IncludeConfig`, `script "/bin/sh -c"`. No path produces an
  executable directive. `grep sh -c|bash -c` empty.
- **Secrets:** resolved only at Render, written only to `Secret` files (snmpd 0600, keepalived 0640,
  rsyslog key 0640 `root:syslog`); gosnmp keeps communities/passphrases off every argv; keepalived
  dump `auth_data` never surfaces (whitelisted + deleted, `state.go:376-498`); gitleaks clean;
  `VRX_TEST_PSK`/`RF4tpsk*` absent from the integration log.
- **Convergence + rollback (7):** all three prove daemon-side state before success (SNMP GET; SIGJSON
  dump instance/iface/vrid/base-priority; impstats action names stamped after the restart) — except
  the snmpd **listen** case in H1. Rollback runs on a fresh context (`rfkit/apply.go:205`).
- **VRRPv2-for-auth (D-RF4-3 / D-086):** correctly forced to IPv4 + whole-second advert + PASS ≤ 8
  (`model.go:357-376`). PASS is plaintext on the wire; the renderer restricts and documents it, but
  **F-vrrp must surface that plaintext warning in the schema/UI** — not done here (out of scope).
- **rfkit shared base / gosnmp go.mod:** only `github.com/gosnmp/gosnmp v1.43.1` (BSD-2) added, as the
  task required; rfkit is a clean helper layer (accepted D-086; consolidation with the RF-1 framework
  later). **Contract:** no `packages/schema` / proto / gen changes — stand-ins via structpb (D-055),
  documented in `RF-4-questions.md` Q2. Compliant.
- **Shared-host rules:** test daemons are children spawned by the test, killed by their own PID with a
  `/proc/<pid>/exe` guard (`rfkit/controller.go:113-132`); slot-prefixed; keepalived confined to
  `ns-w8-a` with both veth ends inside the namespace (VRRP never reaches `ens192`); host rsyslog never
  touched. Verified clean after my runs.

## Required before merge
- **H1** — fix the snmpd listen/bind reload (restart or persisted restart-request + real socket
  convergence), or reject listen changes until that lands.
- **M1** — stop defaulting the bind to `0.0.0.0`.

## On the board (follow-ups, do not block)
- M2 (rsyslog idempotent restart), M3 (impstats double-load / P10), L1–L3, and the F-vrrp plaintext-
  auth UI warning.

**APPROVE WITH CHANGES**
