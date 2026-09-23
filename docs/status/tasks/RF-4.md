# RF-4 — Renderers: snmpd, keepalived, rsyslog

Branch `task/RF-4` (base `main@ff6b91a`), worktree `/root/ngfw-wt/RF-4`, slot 8 (`w8`). Worker ran directly on the
host. Not merged.

## What was built

Three agent-side renderers implementing `renderers.Renderer` (Render / Validate / Apply / Retrieve) plus a
`Poller()` for 1 Hz events, and one shared helper package:

| package | what |
|---|---|
| `internal/renderers/rfkit` | `Controller` interface with `SystemdController` (product: `systemctl reload\|restart\|kill <unit>`, never start/enable) and `ProcessController` (tests: signal the spawned PID after checking `/proc/<pid>/exe`; restart hook); D-051 `Secrets` resolver + `Redactor` (also masks each line of multi-line secrets); `ApplyFiles` (snapshot → atomic write → activate → **convergence check** → restore + re-activate on a fresh context); `Poll`; bounded `ReadFileLimit`/`ReadTail`; `Poller` (change events, `EVENT_KIND_ERROR` on poll failures, redacted); D-055 stand-in navigation (`Decode`, `Ext`). |
| `internal/renderers/snmpd` | `services.snmp` → `snmpd.conf` (0600, Secret): agentaddress, sys*, views (numeric OIDs, symbolic allow-list), ro/rw community[6] per source, `createUser` + ro/rwuser, trap2sink/informsink/trapsess, `master agentx` + `agentXSocket`, disk/load monitors. **Validate = daemon parse run** of a staged check copy. Apply = SIGHUP + SNMP convergence. Retrieve = gosnmp GET of the system group with a credential read from the file itself. |
| `internal/renderers/keepalived` | `ha.vrrp` (engine keepalived) → `keepalived.conf` (0640): global_defs (script security, vrrp_version 3), vrrp_script (shipped checks only), vrrp_instance (state, interface, VRID, priority, advert_int incl. centiseconds, nopreempt/preempt_delay, accept, unicast, VIPs, virtual_routes, track_interface, track_script, VRRPv2 PASS auth), notify_* → our helper, vrrp_sync_group. Validate `keepalived -t -f <staged> -s <netns>`. Apply = SIGHUP + convergence from the SIGJSON dump. Retrieve = notify state files + whitelisted dump (never `auth_data`; dump deleted after reading). |
| `internal/renderers/keepalived/cmd/vrx-keepalived-notify` | the notify helper: validates its fixed argv, writes `<state dir>/<instance>.state` atomically (JSON line). |
| `internal/renderers/rsyslog` | `management.syslog` → one RainerScript file: impstats (JSON), fixed RFC 5424 template, per target a ruleset + `omfwd` action (queue LinkedList, resumeRetryCount -1, octet-counted TCP), facility/severity `prifilt`, TLS (ossl driver, CA/cert/key files from `cert/`/`key/` refs, key 0640 Secret). Standalone mode (tests) adds `global()`, imuxsock on the slot socket, imtcp on 127.0.0.1. Validate `rsyslogd -N1`. Apply = restart + convergence from impstats records stamped after the restart. Retrieve = impstats tail (bounded). |

Docs: per-package `README.md`, mapping tables `docs/agent/renderers/{snmpd,keepalived,rsyslog}.md`,
`internal/renderers/ALLOWLIST.md` rows, questions/decisions `docs/status/tasks/RF-4-questions.md`.

### RF-1 review failure patterns — how each is avoided here
- **Injection through the daemon's own parser**: every value passes a typed validator in `BuildModel` *and* a strict
  template helper; line/brace/quote characters are rejected for tokens; rest-of-line text (snmpd sysLocation) is
  printable ASCII only and asserted to stay on its line; keepalived never renders `include`, `$VAR`, `@`, strict/vmac
  options or any script but ours; rsyslog quotes every string and only emits `omfwd`. Goldens assert "no foreign
  directive / balanced braces / only omfwd / only our script paths".
- **Apply success without convergence**: `rfkit.ApplyFiles` requires a daemon-side proof (SNMP GET; keepalived JSON
  dump; impstats records after the restart with hashed action names). Each integration test runs an Apply whose
  reload/restart never reaches the daemon and shows it is refused and rolled back.
- **Secrets in Retrieve/DryRun/errors/daemon logs**: resolved at Render only, written only to Secret files (0600 /
  0640), Redactor on every error/state/event; keepalived dump whitelisted and deleted; snmpd queried in-process
  (no argv); `keepalived -G` / `snmpd -Lf` / rsyslog `-n` so test daemons never log to the host syslog. Planted-secret
  unit tests per daemon + integration scans of the slot directories and the test log.
- **Unbounded reads**: pidfiles 64 B, configs 1 MiB, keepalived dump 4 MiB, state files 4 KiB, snmpd parse-run log
  256 KiB, impstats tail 256 KiB (+ truncation above 8 MiB).
- **Harness cross-kill**: each integration test takes an exclusive `/run/vrx-test/<prefix>/<daemon>.lock`, kills
  only its `exec.Cmd` PID; the parse-run snmpd is stopped only after `/proc/<pid>/exe` + cmdline (staging dir) match;
  `ProcessController` refuses a PID running another binary (unit test with a real process).

## How it was verified

(outputs pasted below; all runs on the host, slot 8)

### Integration (real daemons, slot 8) and acceptance checks
Full log: `/root/ngfw-wt/logs/RF-4-integration.log`. Test daemons were started as children of the test, never via
units or `/etc`; the rendered listen/interface lists were asserted before each start (snmpd `udp:127.0.0.1:3861`
only; keepalived interfaces `w8-*` inside `ns-w8-a` only; rsyslog inputs = slot socket + `127.0.0.1:3814`, targets
loopback only).

```
$ VRX_INTEGRATION=1 go test -count=1 -v -run Integration ./internal/renderers/{snmpd,keepalived,rsyslog}/   (slot 8, under flock -s /run/lock/vrx-lab.lock)
=== RUN   TestSnmpdIntegration
    snmpd_integration_test.go:85: parse run rejects a broken line: snmpd: daemon error: snmpd rejected snmpd.conf: <staging>/check/snmpd.conf: line 34: Error: bad SUBTREE object id
    snmpd_integration_test.go:107: snmpd child pid 1927092: /usr/sbin/snmpd -f -Lf /run/vrx-test/w8/snmpd/snmpd.log -C -c /run/vrx-test/w8/snmpd/snmpd.conf -p /run/vrx-test/w8/snmpd/snmpd.pid -m  -M /run/vrx-test/w8/snmpd/mibs
    snmpd_integration_test.go:110: SNMP reply (127.0.0.1:3861 via v3 user u1): sysName.0="w8-snmpd" sysLocation.0="RF-4 rack one" sysContact.0="noc@example.net" sysUpTime.0=46 sysDescr.0="Linux ubuntu-26.04 7.0.0-31-generic #31-Ubuntu SMP PREEMPT_DYNAMIC Sat Aug  1 04:26:38 UTC 2026 x86_64"
    snmpd_integration_test.go:136: after Apply+SIGHUP: sysLocation.0="RF-4 rack two", pid 1927092 unchanged
    snmpd_integration_test.go:150: Apply without a reload is refused and rolled back: rfkit: daemon did not converge: snmpd reports sysLocation "RF-4 rack two", rendered "never applied"
    snmpd_integration_test.go:163: event: snmpd sysLocation: RF-4 rack two -> RF-4 rack three
--- PASS: TestSnmpdIntegration (4.19s)
PASS
ok  	ngfw/agent/internal/renderers/snmpd	4.246s
=== RUN   TestKeepalivedIntegration
    keepalived_integration_test.go:80: rendered keepalived.conf:
    keepalived_integration_test.go:108: keepalived child pid 1927774: /usr/bin/ip netns exec ns-w8-a /usr/sbin/keepalived -n -l -P -G -f /run/vrx-test/w8/keepalived/keepalived.conf -p /run/vrx-test/w8/keepalived/keepalived.pid -r /run/vrx-test/w8/keepalived/vrrp.pid -c /run/vrx-test/w8/keepalived/checkers.pid
    keepalived_integration_test.go:112: vi1.state (MASTER after 1.801s): {"name":"vi1","type":"INSTANCE","state":"MASTER","time":"2026-09-23T22:49:37.651070336Z"}
    keepalived_integration_test.go:116: ip -n ns-w8-a -j addr show dev w8-a: 10.8.240.1/24 present
    keepalived_integration_test.go:122: Retrieve (state file + keepalived JSON dump): {"Name":"vi1","State":"MASTER","Since":"2026-09-23T22:49:37.651070336Z","Dump":{"name":"vi1","interface":"w8-a","vrid":81,"state":"MASTER","basePriority":150,"effectivePriority":150,"vipsSet":true,"vips":["10.8.240.1/24"],"version":3,"lastTransition":1790203777.636595,"advertSent":1,"advertRcvd":0,"becomeMaster":1,"releaseMaster":0,"authFailure":0}}
    keepalived_integration_test.go:162: after Apply+SIGHUP (pid 1927774 unchanged): [{"Name":"vi1","State":"MASTER","Since":"2026-09-23T22:49:41.436986117Z","Dump":{"name":"vi1","interface":"w8-a","vrid":81,"state":"MASTER","basePriority":160,"effectivePriority":160,"vipsSet":true,"vips":["10.8.240.1/24"],"version":3,"lastTransition":1790203781.420859,"advertSent":3,"advertRcvd":0,"becomeMaster":2,"releaseMaster":0,"authFailure":0}},{"Name":"vi2","State":"MASTER","Since":"2026-09-23T22:49:41.432787049Z","Dump":{"name":"vi2","interface":"w8-b","vrid":82,"state":"MASTER","basePriority":120,"effe
    keepalived_integration_test.go:179: Apply without a reload is refused and rolled back: rfkit: daemon did not converge: keepalived runs 2 instances [vi1 vi2], rendered 1
    keepalived_integration_test.go:187: events after SIGTERM: [keepalived vi1: MASTER -> STOP keepalived vi2: MASTER -> STOP]
    keepalived_integration_test.go:202: after kill: vi1.state {"name":"vi1","type":"INSTANCE","state":"STOP","time":"2026-09-23T22:49:43.53614624Z"}; VIP gone from w8-a
--- PASS: TestKeepalivedIntegration (10.30s)
PASS
ok  	ngfw/agent/internal/renderers/keepalived	10.356s
=== RUN   TestRsyslogIntegration
    rsyslog_integration_test.go:118: rendered rsyslog.conf:
    rsyslog_integration_test.go:135: TLS export on this host: rsyslog: daemon error: TLS export needs the rsyslog ossl netstream driver (lmnsd_ossl.so, package rsyslog-openssl), not installed in /usr/lib/x86_64-linux-gnu/rsyslog
    rsyslog_integration_test.go:149: rsyslogd child pid 1926857: /usr/sbin/rsyslogd -n -iNONE -f /run/vrx-test/w8/rsyslog/rsyslog.conf
    rsyslog_integration_test.go:157: UDP collector 127.0.0.1:3815 received (RFC 5424):
    rsyslog_integration_test.go:159:   "<14>1 2026-09-24T02:19:34.710286+03:30 ubuntu-26 vrxtest - - -  hello RF-4 user.info\n"
    rsyslog_integration_test.go:159:   "<13>1 2026-09-24T00:00:00Z h vrxtcp - - - hello via imtcp\n"
    rsyslog_integration_test.go:159:   "<156>1 2026-09-24T02:19:34.710424+03:30 ubuntu-26 vrxtest - - -  hello RF-4 local3.warning\n"
    rsyslog_integration_test.go:168: TCP collector 127.0.0.1:3816 received (octet-counted): "91 <156>1 2026-09-24T02:19:34.710424+03:30 ubuntu-26 vrxtest - - -  hello RF-4 local3.warning"
    rsyslog_integration_test.go:178: impstats vrx_export_0_ce31a4a2 → 127.0.0.1:3815/udp: processed=3 failed=0 suspended=0 queue.enqueued=3
    rsyslog_integration_test.go:178: impstats vrx_export_1_2ec2d6d7 → 127.0.0.1:3816/tcp: processed=1 failed=0 suspended=0 queue.enqueued=1
    rsyslog_integration_test.go:180: impstats inputs: map[imtcp(3814):1 imuxsock:3]
    rsyslog_integration_test.go:201: Apply restarted rsyslogd: pid 1926857 → 1927746; impstats reports [vrx_export_0_703a90a2 vrx_export_1_2ec2d6d7]
    rsyslog_integration_test.go:223: Apply without a restart is refused and rolled back: rsyslog: daemon error: rfkit: daemon did not converge: impstats reports actions [vrx_export_0_703a90a2 vrx_export_1_2ec2d6d7] after the restart, rendered [vrx_export_0_141cbd05]
    rsyslog_integration_test.go:248: events: [rsyslog vrx_export_1_2ec2d6d7/queue: empty -> backlog]
    rsyslog_integration_test.go:254: host rsyslog.service MainPID 1014 unchanged
--- PASS: TestRsyslogIntegration (7.48s)
PASS
ok  	ngfw/agent/internal/renderers/rsyslog	7.525s

$ grep -rn "sh -c\|bash -c" internal/renderers/{snmpd,keepalived,rsyslog,rfkit}; echo exit=$?
exit=1

$ grep -rn "VRX_TEST_PSK\|RF4tpsk" /root/ngfw-wt/logs/RF-4-integration.log; echo exit=$?
exit=1

$ for p in /proc/[0-9]*; do exe=$(readlink $p/exe); case $exe in /usr/sbin/snmpd|/usr/sbin/keepalived|/usr/sbin/rsyslogd) echo pid exe cmdline;; esac; done
1014 /usr/sbin/rsyslogd /usr/sbin/rsyslogd -n -iNONE 
(only the host's own rsyslog.service, pid 1014; no snmpd, no keepalived, no test rsyslogd. `pgrep -f /run/vrx-test/w8/…` from the tool
harness matches the wrapper shell whose command line carries the pattern, so the check is done by executable.)
$ ip netns list | grep w8; ls -A /run/vrx-test/w8; ls -d /tmp/vrx-w8-* /run/keepalived 2>&1
ls: cannot access '/tmp/vrx-w8-*': No such file or directory
ls: cannot access '/run/keepalived': No such file or directory

$ systemctl is-active snmpd keepalived rsyslog; systemctl is-enabled …; systemctl show rsyslog -p MainPID   (before → after)
inactive inactive active 
disabled disabled enabled 
MainPID=1014
→
inactive inactive active 
disabled disabled enabled 
MainPID=1014
$ stat of /etc/snmp /etc/keepalived /etc/rsyslog.conf /etc/rsyslog.d (name mode size mtime): diff 01:33 baseline vs after → empty
diff exit=0

$ golangci-lint run ./internal/renderers/...
0 issues.
$ go test -count=1 ./internal/renderers/...
ok  	ngfw/agent/internal/renderers	0.336s
ok  	ngfw/agent/internal/renderers/frr	0.415s
ok  	ngfw/agent/internal/renderers/frr/frrtest	0.026s
ok  	ngfw/agent/internal/renderers/keepalived	0.765s
ok  	ngfw/agent/internal/renderers/keepalived/cmd/vrx-keepalived-notify	0.015s
ok  	ngfw/agent/internal/renderers/rfkit	0.097s
ok  	ngfw/agent/internal/renderers/rsyslog	3.315s
ok  	ngfw/agent/internal/renderers/snmpd	0.947s
```

### CI gate
`tools/ci.sh --base main` (log `/root/ngfw-wt/logs/RF-4-ci-2.log`; the first run failed on a secret-shaped PEM literal in a
hostile-value test, fixed in `test(RF-4): hostile PEM derived from the VRX_TEST_PSK fixture`):

```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m01s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m18s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m19s
  apps/agent: make lint test build                   0m43s
  test/ Go modules, unit mode (test/integration/smoke)   0m03s
  mode quick · wall time 1m30s · logs /root/ngfw-wt/logs/ci/RF-4-20260924-021718-1892466

CI GATE PASSED
```
(Re-run after the final commit: see the last section.)

### Unit tests (no daemon, always on)
Per daemon: golden files for every template path (snmpd 7, keepalived 6, rsyslog 6 in `testdata/`), hostile strings
(`"; rm -rf /`, CR/LF, NUL, ESC, U+2028, invalid UTF-8, unicode, 5 KB, `\nrocommunity public\n`, shell script values,
`action(type="omprog" …)`, `"` and `\` in RainerScript strings, `$IncludeConfig`, backticks) rejected with `ErrInput`
or rendered verbatim on one line where the field is legal free text, planted-secret tests (values never in errors,
Validate output, Retrieve, events; secret only in the Secret file), Validate argv (staged path, never the live file),
Apply convergence + rollback with fake daemons, bounded reads. 717 test cases pass (`go test -v … | grep -c -- '--- PASS'`).

## Out of scope / not done
- API/UI, schema/proto changes (stand-ins listed in RF-4-questions.md Q2), private MIB/AgentX subagent (F-snmp),
  trap semantics, VPP-native VRRP (DF-7), keepalived on linux-cp interfaces and two-node failover (F-vrrp),
  log explorer/storage (F-logging), RELP, journald forwarding, logrotate/packaging (P10).
- rsyslog TLS end-to-end: rendered and validated, but the ossl driver is not installed on this host, so a TLS
  export is refused by Validate and was not run against a collector (Q5).
- `acceptMode: false` for keepalived instances is not enforced (needs firewall rules; Q4).
- Non-default VRFs are rejected by all three renderers (Q9).
- Wiring into the agent's commit engine/registry (P05/P08) — the renderers are constructed with `New(...)` and
  options only.

## Open questions
See `docs/status/tasks/RF-4-questions.md`: Q1 files outside the envelope (rfkit, go.mod, ALLOWLIST), Q2 stand-in
fields for P03b, Q3 VRRPv2 for auth + 8-character test keys, Q4 accept mode, Q5 TLS scope, Q6/Q7 snmpd createUser
placement and parse-run validation, Q8 P10 packaging list, Q9 VRFs, Q10 event kinds.

## Decisions (for the LOG)

| id | decision | options | why |
|---|---|---|---|
| D-RF4-1 | Shared helper package `internal/renderers/rfkit` for the single-file daemon renderers | (a) shared package (b) three copies | one secret/rollback/convergence implementation; same pattern as dfkit (D-077) |
| D-RF4-2 | Convergence proof is mandatory in Apply for all three daemons: snmpd SNMP GET of rendered sys* values; keepalived SIGJSON dump (instances, interface, VRID, base priority); rsyslog impstats records stamped after the restart naming exactly the rendered actions (action names carry a hash of the target's settings) | (a) trust reload exit status (b) daemon-side proof | RF-1 H2; the "lost reload" tests show (a) would report success |
| D-RF4-3 | VRRP authentication ⇒ VRRPv2 per instance (IPv4, whole seconds, PASS ≤ 8) | (a) v2 per instance (b) no auth (c) global v2 | RFC 5798 has no auth; keepalived ignores it under v3 |
| D-RF4-4 | snmpd `createUser` in the main snmpd.conf | (a) main file (b) persistent store | store is ignored with -C and rewritten by snmpd (drift); main file reloads on SIGHUP |
| D-RF4-5 | snmpd Validate = parse run of a staged, network-free check copy | (a) parse run (b) structural only (c) `-H` token list | catches unknown tokens, bad OIDs, short passphrases with snmpd's own parser |
| D-RF4-6 | keepalived Validate uses keepalived's own `-s <netns>`; the product allowlist has no `ip` | (a) `-s` (b) `ip netns exec` | no exec trampoline (RF-1 L1) |
| D-RF4-7 | keepalived Retrieve reads a whitelist of dump fields and deletes the dump; the dump wait is not cut by the caller's deadline | (a) whitelist + delete (b) pass the dump through with redaction | the dump contains `auth_data`; a late dump must not stay on disk (found in the first integration run) |
| D-RF4-8 | Product interface mapping for keepalived defaults to `NoMapper` (instances rejected until F-vrrp passes linux-cp names) | (a) NoMapper (b) identity | RF-1 L4 |
| D-RF4-9 | rsyslog: only `omfwd`; fixed template set; TLS refused unless the ossl driver exists; impstats file bounded read + truncation | — | task rules; `-N1` does not load the driver |
| D-RF4-10 | Controller for tests = PID the test spawned with `/proc/<pid>/exe` check; product = systemctl reload/restart/kill of the owned unit only | — | shared-host §5, RF-1 M4 |
| D-RF4-11 | VRRPv2 test keys `RF4tpsk*` instead of `VRX_TEST_PSK_<id>` | — | 8-character limit (Q3) |

## Final gate
`tools/ci.sh --base main` on `6fe7c05` (log `/root/ngfw-wt/logs/RF-4-ci-3.log`, step logs `/root/ngfw-wt/logs/ci/RF-4-20260924-022058-1953729`):
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m01s
  generate + generated-output gate                   0m21s
  forbidden patterns (+ gitleaks)                    0m04s
  lint · typecheck · unit tests · build (turbo)   0m23s
  apps/agent: make lint test build                   0m26s
  test/ Go modules, unit mode (test/integration/smoke)   0m02s
  mode quick · wall time 1m20s · logs /root/ngfw-wt/logs/ci/RF-4-20260924-022058-1953729

CI GATE PASSED
```
Only docs change after this run. Test daemons stopped, `ns-w8-a` removed, `/run/vrx-test/w8` empty, `apps/agent/bin` removed.

## Review fixes (fix round after `RF-4-review.md`, APPROVE WITH CHANGES)

`git merge main` first (TD-1 `vpp/bootid`, RF-3 ALLOWLIST rows; one conflict in the *Planned* table resolved by
dropping the rows both branches had moved to *Active*).

| finding | fix | proof |
|---|---|---|
| **H1** snmpd listen/port change applied by SIGHUP, convergence only on sys* | startup-only directives (`agentaddress`, `agentXSocket`, `agentXPerms`, `master`, `exactEngineID`) changed → `*rfkit.ActionRequired{restart}` (RF-3/D-079 shape, `NeedsRestart()`), persisted in `Paths.PendingFile` with kernel boot_id + start tick (TD-1 `bootid.Reader`), repeated by every Apply until snmpd's main process started after it; files stay written. Reload path convergence now also requires the **actually bound** UDP sockets of snmpd's main PID (`rfkit.UDPListeners`: `/proc/<pid>/fd` → `net/udp{,6}`) to equal the rendered listen set; `Converged(ctx)` for the engine after a restart | live below: reviewer's 3862→3863 case reproduced (raw SIGHUP keeps 3862), restart request, restart, convergence on `[udp:127.0.0.1:3863]`; unit: stray `0.0.0.0:161` socket → not converged |
| **M1** default bind `0.0.0.0`/`[::]` | no `listen` → `udp:127.0.0.1:161,udp6:[::1]:161` only; `# WARNING:` lines for that default and for explicit wildcards, `snmpd.Warnings()` | `TestDefaultListenIsLoopbackWithWarning`, golden `listen-any` |
| **M2** rsyslog restarts for an unchanged config | Apply is a no-op when every file is on disk with the same SHA-256 + mode (D-076) | unit: 3 Applies → 1 restart; live: two re-Applies, rsyslogd PID unchanged |
| **M3** second impstats load breaks the host config | `ScanHost` (product `/etc/rsyslog.conf`, `/etc/rsyslog.d/*.conf`, own file excluded, comments ignored) at `New`; host loads JSON impstats → no load rendered, stats read from the host file (never truncated); legacy/non-JSON → Validate refuses; Validate re-scans and refuses a stale rendering. P10 note in Q8 | `TestHostImpstats` fixtures; live `TestRsyslogHostImpstatsIntegration`: host main + include passes `rsyslogd -N1` and forwards; the pre-fix rendering fails `-N1` ("already in this config") |
| **L1** substring redaction, 1-char communities | `Redactor` masks whole tokens only; communities ≥ 8 characters | `TestRedactWholeTokensOnly`, hostile-secret table |
| **L2** broad keyword problem regex | only snmpd's structured forms (`line N: Error\|Warning`, leading `Error:`/`Warning:`, `Unknown token`, `Error opening`) | `TestParseProblems`, live parse run still rejects a bad OID |
| **L3** SIGJSON per Retrieve | Retrieve reuses a dump ≤ 5 s old; Apply always takes a fresh one and drops the cache | `TestRetrieveDumpIsRateLimited` (5 Retrieves → 1 signal) |

### Integration after the fixes (slot 8, test daemons only)
```
$ VRX_INTEGRATION=1 go test -count=1 -v -run Integration ./internal/renderers/{snmpd,keepalived,rsyslog}/
=== RUN   TestSnmpdIntegration
    snmpd_integration_test.go:85: parse run rejects a broken line: snmpd: daemon error: snmpd rejected snmpd.conf: <staging>/check/snmpd.conf: line 34: Error: bad SUBTREE object id
    snmpd_integration_test.go:110: snmpd child pid 2153410: /usr/sbin/snmpd -f -Lf /run/vrx-test/w8/snmpd/snmpd.log -A -C -c /run/vrx-test/w8/snmpd/snmpd.conf -p /run/vrx-test/w8/snmpd/snmpd.pid -m  -M /run/vrx-test/w8/snmpd/mibs
    snmpd_integration_test.go:113: SNMP reply (127.0.0.1:3861 via v3 user u1): sysName.0="w8-snmpd" sysLocation.0="RF-4 rack one" sysContact.0="noc@example.net" sysUpTime.0=42 sysDescr.0="Linux ubuntu-26.04 7.0.0-31-generic #31-Ubuntu SMP PREEMPT_DYNAMIC Sat Aug  1 04:26:38 UTC 2026 x86_64"
    snmpd_integration_test.go:139: after Apply+SIGHUP: sysLocation.0="RF-4 rack two", pid 2153410 unchanged
    snmpd_integration_test.go:153: Apply without a reload is refused and rolled back: rfkit: daemon did not converge: snmpd reports sysLocation "RF-4 rack two", rendered "never applied"
    snmpd_integration_test.go:166: event: snmpd sysLocation: RF-4 rack two -> RF-4 rack three
    snmpd_integration_test.go:184: listen 3861→3862: Apply → snmpd needs restart (unit snmpd): listen addresses, AgentX or engine id changed (snmpd applies them only at startup; SIGHUP keeps the old sockets)
    snmpd_integration_test.go:195: after a raw SIGHUP: :3861 answers=true, :3862 error=error reading from socket: read udp 127.0.0.1:36024->127.0.0.1:3862: recvfrom: connection refused, sockets [udp:127.0.0.1:3861]
    snmpd_integration_test.go:202: Converged before the restart: snmpd: daemon error: rfkit: daemon did not converge: snmpd listens on [udp:127.0.0.1:3861], rendered [udp:127.0.0.1:3862]
    snmpd_integration_test.go:217: after restart + Apply: pending cleared, snmpd listens on [udp:127.0.0.1:3862], :3861 refused
    snmpd_integration_test.go:232: 3862→3863: restart request, restart, Apply converged: sockets [udp:127.0.0.1:3863], :3862 refused
--- PASS: TestSnmpdIntegration (5.29s)
PASS
ok  	ngfw/agent/internal/renderers/snmpd	5.345s
=== RUN   TestKeepalivedIntegration
    keepalived_integration_test.go:108: keepalived child pid 2153630: /usr/bin/ip netns exec ns-w8-a /usr/sbin/keepalived -n -l -P -G -f /run/vrx-test/w8/keepalived/keepalived.conf -p /run/vrx-test/w8/keepalived/keepalived.pid -r /run/vrx-test/w8/keepalived/vrrp.pid -c /run/vrx-test/w8/keepalived/checkers.pid
    keepalived_integration_test.go:112: vi1.state (MASTER after 1.801s): {"name":"vi1","type":"INSTANCE","state":"MASTER","time":"2026-09-23T23:12:17.120079325Z"}
    keepalived_integration_test.go:116: ip -n ns-w8-a -j addr show dev w8-a: 10.8.240.1/24 present
    keepalived_integration_test.go:122: Retrieve (state file + keepalived JSON dump): {"Name":"vi1","State":"MASTER","Since":"2026-09-23T23:12:17.120079325Z","Dump":{"name":"vi1","interface":"w8-a","vrid":81,"state":"MASTER","basePriority":150,"effectivePriority":150,"vipsSet":true,"vips":["10.8.240.1/24"],"version":3,"lastTransition":1790205137.107601,"advertSent":1,"advertRcvd":0,"becomeMaster":1,"releaseMaster":0,
    keepalived_integration_test.go:162: after Apply+SIGHUP (pid 2153630 unchanged): [{"Name":"vi1","State":"MASTER","Since":"2026-09-23T23:12:20.822957255Z","Dump":{"name":"vi1","interface":"w8-a","vrid":81,"state":"MASTER","basePriority":160,"effectivePriority":160,"vipsSet":true,"vips":["10.8.240.1/24"],"version":3,"lastTransition":1790205140.809908,"advertSent":3,"advertRcvd":0,"becomeMaster":2,"releaseMaster":0,"
    keepalived_integration_test.go:179: Apply without a reload is refused and rolled back: rfkit: daemon did not converge: keepalived runs 2 instances [vi1 vi2], rendered 1
    keepalived_integration_test.go:187: events after SIGTERM: [keepalived vi1: MASTER -> STOP keepalived vi2: MASTER -> STOP]
    keepalived_integration_test.go:202: after kill: vi1.state {"name":"vi1","type":"INSTANCE","state":"STOP","time":"2026-09-23T23:12:22.893811143Z"}; VIP gone from w8-a
--- PASS: TestKeepalivedIntegration (9.87s)
PASS
ok  	ngfw/agent/internal/renderers/keepalived	9.919s
=== RUN   TestRsyslogIntegration
    rsyslog_integration_test.go:135: TLS export on this host: rsyslog: daemon error: TLS export needs the rsyslog ossl netstream driver (lmnsd_ossl.so, package rsyslog-openssl), not installed in /usr/lib/x86_64-linux-gnu/rsyslog
    rsyslog_integration_test.go:149: rsyslogd child pid 2153467: /usr/sbin/rsyslogd -n -iNONE -f /run/vrx-test/w8/rsyslog/rsyslog.conf
    rsyslog_integration_test.go:157: UDP collector 127.0.0.1:3815 received (RFC 5424):
    rsyslog_integration_test.go:159:   "<14>1 2026-09-24T02:42:14.353265+03:30 ubuntu-26 vrxtest - - -  hello RF-4 user.info\n"
    rsyslog_integration_test.go:159:   "<156>1 2026-09-24T02:42:14.353687+03:30 ubuntu-26 vrxtest - - -  hello RF-4 local3.warning\n"
    rsyslog_integration_test.go:159:   "<13>1 2026-09-24T00:00:00Z h vrxtcp - - - hello via imtcp\n"
    rsyslog_integration_test.go:168: TCP collector 127.0.0.1:3816 received (octet-counted): "91 <156>1 2026-09-24T02:42:14.353687+03:30 ubuntu-26 vrxtest - - -  hello RF-4 local3.warning"
    rsyslog_integration_test.go:178: impstats vrx_export_0_ce31a4a2 → 127.0.0.1:3815/udp: processed=3 failed=0 suspended=0 queue.enqueued=3
    rsyslog_integration_test.go:178: impstats vrx_export_1_2ec2d6d7 → 127.0.0.1:3816/tcp: processed=1 failed=0 suspended=0 queue.enqueued=1
    rsyslog_integration_test.go:180: impstats inputs: map[imtcp(3814):1 imuxsock:3]
    rsyslog_integration_test.go:201: Apply restarted rsyslogd: pid 2153467 → 2153606; impstats reports [vrx_export_0_703a90a2 vrx_export_1_2ec2d6d7]
    rsyslog_integration_test.go:211: two more Applies of the same files: pid 2153606 unchanged (no restart)
    rsyslog_integration_test.go:233: Apply without a restart is refused and rolled back: rsyslog: daemon error: rfkit: daemon did not converge: impstats reports actions [vrx_export_0_703a90a2 vrx_export_1_2ec2d6d7] after the restart, rendered [vrx_export_0_141cbd05]
    rsyslog_integration_test.go:258: events: [rsyslog vrx_export_1_2ec2d6d7/queue: empty -> backlog]
    rsyslog_integration_test.go:264: host rsyslog.service MainPID 1014 unchanged
--- PASS: TestRsyslogIntegration (7.35s)
=== RUN   TestRsyslogHostImpstatsIntegration
    rsyslog_integration_test.go:334: host scan: {Loaded:true File:/run/vrx-test/w8/rsyslog-host/host-impstats.json Source:/run/vrx-test/w8/rsyslog-host/rsyslog.conf}
    rsyslog_integration_test.go:353: rsyslogd -N1 -f <host main + our include>: err=<nil>
    rsyslog_integration_test.go:365: rsyslogd -N1 with a second impstats load (pre-fix rendering): err=exit status 1
    rsyslog_integration_test.go:379: collector 127.0.0.1:3817: "<14>1 2026-09-24T02:42:21.680735+03:30 ubuntu-26 vrxtest - - -  via the host config\n"
    rsyslog_integration_test.go:389: Apply converged via /run/vrx-test/w8/rsyslog-host/host-impstats.json; Retrieve: vrx_export_0_e3fea578 reported=true
--- PASS: TestRsyslogHostImpstatsIntegration (1.59s)
PASS
ok  	ngfw/agent/internal/renderers/rsyslog	8.992s

$ grep -c "VRX_TEST_PSK\|RF4tpsk" /root/ngfw-wt/logs/RF-4-integration-fix.log
0
$ # snmpd/keepalived/rsyslogd processes by executable; netns; slot dir
1014 /usr/sbin/rsyslogd   (host rsyslog.service)
ns-w8-*: 0   /run/vrx-test/w8 entries: 0
$ systemctl is-active/is-enabled snmpd keepalived rsyslog; MainPID (before → after)
inactive inactive active 
disabled disabled enabled 
MainPID=1014 →
inactive inactive active 
disabled disabled enabled 
MainPID=1014
$ stat diff /etc/snmp /etc/keepalived /etc/rsyslog.conf /etc/rsyslog.d vs the 01:33 baseline: identical
```
