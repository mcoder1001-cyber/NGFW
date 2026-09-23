# RF-3 — Renderers: kea-dhcp4/6 + ctrl-agent, unbound, chrony

Branch `task/RF-3` (worktree `/root/ngfw-wt/RF-3`, slot 6 / `w6`), base `main@179676a`. Not merged.
Installed daemons (decided per version): **Kea 3.0.3**, **Unbound 1.24.2**, **chrony 4.8**.

## What was built

| package | files | render | validate | apply | retrieve / events |
|---|---|---|---|---|---|
| `apps/agent/internal/renderers/kea` | `kea-dhcp4.conf`, `kea-dhcp6.conf`, `kea-ctrl-agent.conf` | typed Go structs → `encoding/json` (no templates) | `kea-dhcp4 -t`, `kea-dhcp6 -t`, `kea-ctrl-agent -t` on a staged copy | `config-set` over each server's unix control socket (Go client, no process); ctrl-agent `config-reload` over HTTP 127.0.0.1 when its file changed; restore + re-set on failure | `status-get`, `config-get`, `statistic-get-all`, paged `lease4/6-get-page` (1000/page); 1 Hz stats poller |
| `…/renderers/unbound` | `unbound.conf` | `templates/unbound.conf.tmpl` + strict helpers | `unbound-checkconf` | `unbound-control -c <conf> reload_keep_cache`; restore + reload on failure | `status`, `stats_noreset`, `list_forwards`, `list_stubs`, `list_local_zones`, `list_local_data` → typed; 1 Hz poller |
| `…/renderers/chrony` | `chrony.conf`, `sources.d/vrx.sources`, `chrony.keys` (Secret, 0600) | three templates + strict helpers; keys from `WithSecrets` (hex) | `chronyd -p -f` on staged conf and sources; keys structurally | `chronyc -h <sock> reload sources` / `rekey`; any chrony.conf change → typed restart request | `chronyc -c tracking|sources|sourcestats|serverstats` (CSV → typed); 1 Hz poller |

Common: `Paths` injected (`ProductPaths()` = `/etc/…`, `TestPaths()` = `/run/vrx-test/w6/…`), fixed argv only, `Apply` is
snapshot → atomic write → control channel → restore on failure, idempotent (`Apply(previous)` = rollback, exercised in every
integration test). A daemon that must be (re)started is reported as a typed `*ActionRequired{Unit, Action}` (files stay
written) — the renderers never run `systemctl`. `Retrieve` returns a `structpb.Struct` (no state message in the proto yet, Q2).
Docs: package `README.md`s, `docs/agent/renderers/{kea,unbound,chrony}.md` (desired state ↔ directives), `ALLOWLIST.md`
section "Active — RF-3".

Slot ports (recorded here as required): kea-ctrl-agent **127.0.0.1:3680**, unbound **127.0.0.1:3653**, chrony server
**127.0.0.1:3623**; Kea servers bind only `w6-a` inside `ns-w6-a` (10.6.10.1/24, fd00:6:10::1/64).

## How it was verified (real output)

### Unit tests (golden files, hostile strings, argv, Apply/rollback) — `go test ./internal/renderers/...`

```
$ go test -count=1 ./internal/renderers/kea/... ./internal/renderers/unbound/... ./internal/renderers/chrony/... ./internal/renderers/
ok  	ngfw/agent/internal/renderers/kea	0.138s
ok  	ngfw/agent/internal/renderers/unbound	0.085s
ok  	ngfw/agent/internal/renderers/chrony	0.089s
ok  	ngfw/agent/internal/renderers	0.331s        (TestAllowlistDocumented: every RF-3 binary listed)
```
228 passing (sub)tests. Goldens: kea 7 cases × 3 files (empty, v4, v6, both, bindaddr, disabled, hostile-description),
unbound 7 (empty, full = forward/TLS/local zones/all RR types, views, dnssec-off, dnssec-static, disabled,
hostile-description), chrony 6 × 2 files (disabled, client with keys/NTS/pool, server with allow/deny/ratelimit/orphan,
product paths, local-only, test source port). chrony.keys is asserted structurally, never stored as a golden.

Hostile strings (`"; rm -rf /`, `\ninclude: /etc/passwd`, `"}]}` JSON break-out, CR/LF, NUL, ESC, U+2028, invalid UTF-8,
unicode, 5 KB) in every user field of every daemon:
- **kea**: descriptions are JSON-escaped and ASCII-escaped (`hostile-description.kea-dhcp4.conf.golden` line 121:
  `"description": "\"}]} , \"Dhcp4\": {\"hooks-libraries\": [{\"library\": \"/tmp/evil.so\"}]} \\u2603 \\\\"`,
  `"server-description": "\"; rm -rf /"`); `TestHostileDescriptionEscaped` parses the file and checks the hook list is
  unchanged; control characters / 5 KB rejected (`TestRejects/desc-*`); interface names, MAC, DUID, hostnames, pools,
  option data (printable ASCII only) rejected when hostile; `ens192` rejected by the test prefix `w6-`.
- **unbound**: `TestRejects/zone-include` (`include: /etc/passwd` as a zone) and every `\ninclude:` variant → `ErrInvalid`;
  a TXT record `v=spf1 "quoted" \ back ; semi 'apos' include: /etc/passwd` is escaped
  (`local-data: 'txt… TXT "v=spf1 \034quoted\034 \092 back \059 semi \039apos\039 \105nclude\058 /etc/passwd"'`);
  `assertNoInjection` checks that no golden line outside a comment contains `include`; the description
  `"; rm -rf /` is only in a quoted comment (`# resolver "lan": "\"; rm -rf /"`).
- **chrony**: no description field exists; `"; rm -rf /`, `x\ninclude /etc/passwd`, unicode, 5 KB in server/pool/keyRef/
  allow/listen → `ErrInvalid` (`TestRejects/*-hostile-*`); `TestKeys` asserts no raw/hex key material outside chrony.keys;
  `TestSecretErrorsDoNotLeak` asserts resolver errors and oversized keys never echo the value; `Files.Redacted()` hides it.

### Integration against test-scoped daemons (`VRX_INTEGRATION=1`, lab lock shared, children killed by PID)

```
$ eval "$(tools/lab env 6)"; VRX_INTEGRATION=1 go test -count=1 -v ./internal/renderers/kea/... ./internal/renderers/unbound/... ./internal/renderers/chrony/...
kea_integration_test.go:215: /run/vrx-test/w6/kea/etc/kea-dhcp4.conf listens on [w6-a/10.6.10.1] (inside ns-w6-a)
kea_integration_test.go:215: /run/vrx-test/w6/kea/etc/kea-dhcp6.conf listens on [w6-a] (inside ns-w6-a)
kea_integration_test.go:215: kea-ctrl-agent listens on 127.0.0.1:3680
kea_integration_test.go:221: kea-dhcp4 -t / kea-dhcp6 -t / kea-ctrl-agent -t: accepted
kea_integration_test.go:234: kea-dhcp4 -t rejected the broken file: kea: daemon error: kea-dhcp4 -t rejected kea-dhcp4.conf: Syntax check failed with: /run/vrx-test/w6/kea/etc/kea-dhcp4.conf:73.47: syntax error, unexpected integer, expecting boolean
kea_integration_test.go:242: Apply before start: kea: kea-dhcp4 must be started (kea-dhcp4-server): configuration has interfaces but the server is not running
kea_integration_test.go:244: started pid 1284850: /usr/bin/ip netns exec ns-w6-a /usr/sbin/kea-dhcp4 -c /run/vrx-test/w6/kea/etc/kea-dhcp4.conf
kea_integration_test.go:245: started pid 1284851: /usr/bin/ip netns exec ns-w6-a /usr/sbin/kea-dhcp6 -c /run/vrx-test/w6/kea/etc/kea-dhcp6.conf
kea_integration_test.go:246: started pid 1284852: /usr/sbin/kea-ctrl-agent -c /run/vrx-test/w6/kea/etc/kea-ctrl-agent.conf
kea_integration_test.go:279: dhcp4: config-get vs rendered (normalised subset diff): 0 differences
kea_integration_test.go:279: dhcp6: config-get vs rendered (normalised subset diff): 0 differences
kea_integration_test.go:286: lease4-get-all: result=3 text="0 IPv4 lease(s) found." arguments={ "leases": [  ] }
kea_integration_test.go:299: Retrieve: keys dhcp4/dhcp6=true/true, dhcp4.running=true, dhcp4.leases=0
kea_integration_test.go:309: kea-ctrl-agent config-get (service dhcp4): hostile description round-trips verbatim
kea_integration_test.go:317: first poll: 14 events (e.g. kea-dhcp4 pkt4-ack-sent: "" -> "0")
kea_integration_test.go:330: dhcp4: config-get vs rendered (normalised subset diff): 0 differences
kea_integration_test.go:330: dhcp6: config-get vs rendered (normalised subset diff): 0 differences
kea_integration_test.go:335: after change: config-get has subnet 10.6.11.0/24
kea_integration_test.go:340: poll after change: [kea-dhcp4 subnet[3146464543].assigned-addresses: "" -> "0" … total-addresses: "" -> "11"]
kea_integration_test.go:346: dhcp4: config-get vs rendered (normalised subset diff): 0 differences      (after rollback)
kea_integration_test.go:346: dhcp6: config-get vs rendered (normalised subset diff): 0 differences
kea_integration_test.go:101: stopped pid 1284852 / 1284851 / 1284850
--- PASS: TestKeaIntegration (1.35s)
ok  	ngfw/agent/internal/renderers/kea	1.449s

unbound_integration_test.go:83: rendered listen list: [127.0.0.1@3653]
unbound_integration_test.go:88: unbound-checkconf: accepted
unbound_integration_test.go:93: unbound-checkconf rejected the broken file: … unbound.conf:4: error: unknown keyword 'no-such-option'
unbound_integration_test.go:101: Apply before start: unbound: unbound needs start (unbound): configuration has resolvers but unbound is not running
unbound_integration_test.go:109: started pid 1284775: /usr/sbin/unbound -d -c /run/vrx-test/w6/unbound/unbound.conf
unbound_integration_test.go:141: unbound-control list_forwards:
    corp.example.test. IN forward 192.0.2.1 192.0.2.2
unbound_integration_test.go:162: net.Resolver @127.0.0.1:3653 gw.rig.example.test → [10.6.10.1]
unbound_integration_test.go:167: TXT with quotes/backslash/include: round-trips verbatim: "v=spf1 \"quoted\" \\ back ; semi 'apos' include: /etc/passwd"
unbound_integration_test.go:174: first poll: [unbound running: "" -> "true" unbound total.num.cachehits: "" -> "3" … total.num.queries: "" -> "3" …]
unbound_integration_test.go:198: after change: new.rig.example.test → [10.6.10.2]; list_local_data has 215 records
unbound_integration_test.go:213: after rollback: new.rig.example.test no longer resolves
unbound_integration_test.go:121: stopped pid 1284775
--- PASS: TestUnboundIntegration (10.61s)
ok  	ngfw/agent/internal/renderers/unbound	10.703s

chrony_integration_test.go:164: server listens on [bindaddress 127.0.0.1 port 3623]; client [port 0] (no NTP server socket)
chrony_integration_test.go:174: chronyd -p (chrony.conf + vrx.sources): accepted for server and client
chrony_integration_test.go:185: chronyd -p rejected the broken sources file: … Fatal error : Invalid option in server directive at line 3 in file /run/vrx-test/w6/chrony/client/sources.d/vrx.sources
chrony_integration_test.go:192: Apply before start: chrony: chronyd needs start (chrony): services.ntp is enabled but chronyd is not running
chrony_integration_test.go:196: started pid 1284794: /usr/sbin/chronyd -f /run/vrx-test/w6/chrony/server/chrony.conf -n -x -l /run/vrx-test/w6/chrony/server/log/chronyd.log
chrony_integration_test.go:197: started pid 1284836: /usr/sbin/chronyd -f /run/vrx-test/w6/chrony/client/chrony.conf -n -x -l /run/vrx-test/w6/chrony/client/log/chronyd.log
chrony_integration_test.go:216: client chronyc -c sources: ^,*,127.0.0.1,10,-1,17,1,0.000004099,0.000003995,0.000034882
chrony_integration_test.go:218: client chronyc -c tracking: 7F000001,127.0.0.1,11,1790200278.901734687,-0.000019323,-0.000000122,0.000000122,-34.430,-0.000,929.296,0.000068810,0.001087783,0.1,Normal
chrony_integration_test.go:226: server serverstats: ntpPacketsReceived=4; server tracking stratum=10
chrony_integration_test.go:233: both chronyd logs: "Disabled control of system clock"
chrony_integration_test.go:240: first poll: [chrony leap: "" -> "Normal" chrony reference: "" -> "127.0.0.1" chrony running: "" -> "true" chrony source/127.0.0.1: "" -> "*" chrony stratum: "" -> "11"]
chrony_integration_test.go:264: after reload sources + rekey: sources [127.0.0.1 192.0.2.123]
chrony_integration_test.go:280: conf change: chrony: chronyd needs restart (chrony): chrony.conf changed (only sources and keys reload at run time)
chrony_integration_test.go:282: started pid 1285248: /usr/sbin/chronyd -f /run/vrx-test/w6/chrony/server/chrony.conf -n -x -l …   (test restarts its own child)
chrony_integration_test.go:298: after server restart (local stratum 9): client chronyc -c sources: ^,-,127.0.0.1,9,-1,75,1,… | ^,?,192.0.2.123,0,6,0,…
chrony_integration_test.go:308: rollback: client back to one source
chrony_integration_test.go:108: stopped pid 1285248 / 1284836
--- PASS: TestChronyIntegration (2.33s)
ok  	ngfw/agent/internal/renderers/chrony	2.448s
```
Full log: `/root/ngfw-wt/logs/RF-3-integration.log`.

### Acceptance checks

```
$ grep -rn "sh -c\|bash -c" internal/renderers/{kea,unbound,chrony}
(exit 1 — no match)
$ pgrep -af /run/vrx-test/w6/
(empty)
$ systemctl is-active kea-dhcp4-server kea-dhcp6-server kea-ctrl-agent unbound chrony
inactive
inactive
inactive
inactive
active          ← host timesync, active+enabled before RF-3 started and never touched (Q1)
$ systemctl is-enabled kea-dhcp4-server kea-dhcp6-server kea-ctrl-agent unbound chrony
disabled
disabled
disabled
disabled
enabled
$ diff /run/vrx-test/w6/etc-stat-before.txt /run/vrx-test/w6/etc-stat-after.txt     # stat of every file in /etc/kea /etc/unbound /etc/chrony
(identical, 15 files)
$ ip netns list | grep w6
(nothing — ns-w6-a deleted in t.Cleanup)
```
Host clock: every chronyd child ran with `-x` (argv asserted in the test) and both logs contain
"Disabled control of system clock"; `rtcsync` is never rendered for test instances.

### CI gate

```
$ tools/ci.sh --base main
branch    task/RF-3 @ 940e51e   (base: main)
no contract files changed in the 10 commit(s) of HEAD since main (179676a)
…
ok: gitleaks — scanned ~306154 bytes (306.15 KB) in 1.22s no leaks found
== apps/agent: make lint test build ==
ok  ngfw/agent/internal/renderers	1.703s; ok  ngfw/agent/internal/renderers/chrony	1.498s; ok  ngfw/agent/internal/renderers/kea	1.637s; ok  ngfw/agent/internal/renderers/unbound	1.462s; …
CI GATE PASSED
```
(re-run on the final commit is in the summary message / WIP log.)

## Findings on the installed versions (they shaped the code)

- Kea 3.0 refuses control sockets / lease files / logs outside `/run/kea`, `/var/lib/kea`, `/var/log/kea` unless
  `KEA_CONTROL_SOCKET_DIR`, `KEA_DHCP_DATA_DIR`, `KEA_LOG_FILE_DIR` point elsewhere, and requires the socket dir ≤ 0750
  (`kea.Env`, `kea.NewRunner`).
- `kea-dhcp4 -t` checks that a subnet's `interface` exists → the checker must run in the server's namespace (test: `ip netns exec`,
  test-only runner).
- Kea's JSON is byte-oriented: UTF-8 text comes back from `config-get` as `\u00XX` per byte → free text is ASCII-escaped
  (`DecodeText` reverses it). Kea reports aligned pools as CIDR and hex option data without `0x`; `raw` socket type is omitted.
- Unbound leaves its control socket behind after exit → the renderer probes the socket (stale = not running); same for chrony.
- chronyc drops to `_chrony` before connecting and chronyd refuses a command-socket directory it does not own → the test instance
  directories use the product ownership (`_chrony:_chrony 0750`).

## Out of scope / not done

API/UI; schema/proto changes (gaps listed in Q4); DHCP relay + VPP DNS cache (DF-8); real DORA / DNSSEC / NTP quality (F-*);
NTS-KE server certificates (`ntsServer` refused, Q5); stub zones (no schema field); per-VRF daemon instances (Q6); the commit
engine acting on `ActionRequired` (P05, Q3); `tools/ci.sh full` (manager, CI slot).

## Decisions (for the LOG)

| id | decision | options | why |
|---|---|---|---|
| D-RF3-1 | Kea: one kea-dhcp4 + one kea-dhcp6 process for all enabled servers of a family; server settings pushed down to subnets; one VRF per family | (a) per-server process (b) merged per family | Kea serves many interfaces/subnets per process; per-VRF needs a netns launcher (Q6) |
| D-RF3-2 | Kea apply = `config-set` over the unix socket, **no `config-write`** | (a) config-set + config-write (prompt) (b) config-set only (c) config-reload | the file is already the rendered bytes; config-write would overwrite it with Kea's canonical dump |
| D-RF3-3 | Kea subnet id = FNV-32a(`server/subnet`) | (a) sequential (b) hash | leases reference ids: adding a subnet must not renumber others |
| D-RF3-4 | Unknown DHCP option codes: `0x…` → raw hex; text → string `option-def vrx-<code>`; standard codes use Kea's definition (probed list for 3.0.3) | (a) require hex (b) typed option-def per code | schema has no option type |
| D-RF3-5 | Free text in Kea JSON is ASCII-escaped (`\uXXXX` as text) | (a) reject non-ASCII (b) drop descriptions (c) escape | fa users write Persian descriptions; Kea mangles UTF-8 |
| D-RF3-6 | Unbound: one instance; several resolvers → `view:` per resolver + `interface-view`; instance settings must agree | (a) one process per resolver (b) views | matches the prompt's `view:` path without a process launcher |
| D-RF3-7 | TXT data `\DDD`-escaped incl. the first letter of `include` | (a) reject quotes (b) escape | TXT must carry arbitrary printable text; token `include` never appears |
| D-RF3-8 | chrony: only sources (`reload sources`) and keys (`rekey`) change at run time; everything else returns `ActionRequired{restart}` | (a) always restart (b) typed restart only when needed | chrony has no other runtime reload |
| D-RF3-9 | Restart/start is a per-package typed `*ActionRequired` with `NeedsRestart() (unit, action)`; renderers never call systemctl | (a) systemctl in renderer (b) typed result | envelope forbids unit actions; commit engine owns process control (Q3) |
| D-RF3-10 | `ip` is test-only (never in a production allowlist) | — | `ip netns exec` is a trampoline (same rule as RF-1) |
| D-RF3-11 | `Retrieve` → `structpb.Struct` stand-in | (a) wait for proto (b) structpb (D-055) | no DHCP/DNS/NTP state messages yet (Q2) |
| D-RF3-12 | `ntsServer` refused | (a) ignore (b) refuse | a half-configured NTS server is worse than a clear error (Q5) |

## Open questions

`docs/status/tasks/RF-3-questions.md` (Q1 host chrony.service active; Q2 state messages; Q3 ActionRequired in the contract;
Q4 schema gaps; Q5 ntsServer; Q6 per-VRF instances; Q7 ALLOWLIST.md outside the file set).

---

## Review fixes (fix round after `RF-3-review.md`, APPROVE WITH CHANGES; D-079)

`git merge main` first (RF-1, P03b; `ALLOWLIST.md` merged cleanly: merge commit `00ef805`).

| finding | fix |
|---|---|
| **H1** Unbound listen/port change reported success | `Apply` compares the startup-only directives of the old and new file (`interface`, `port`, `interface-view`, `username`, `chroot`, `directory`, `pidfile`, `do-daemonize`, `control-*`). If they differ it returns `*ActionRequired{restart}` without a reload. Otherwise it runs `reload_keep_cache` plus a **convergence check**: every rendered forward zone must be in `list_forwards`, every global local zone in `list_local_zones` with its type, and every `interface` must accept TCP. A failure restores and reloads |
| **M1** Kea subnet ids renumbered on collision | An existing subnet keeps the id Kea runs. The assignment is persisted in the applied config itself (`user-context.vrx` next to `id`) and re-read by `New` and after every `Apply`, so it survives agent restarts and rollbacks. New subnets get FNV-32a, salted `#1`, `#2`… until free. Existing names are placed first and are never renumbered. Tested with the reviewer's pair |
| **M2** pending restart lost | `unbound` and `chrony` persist the request in `Paths.PendingFile` (product `/run/vrx/renderers/<daemon>.pending`). It holds uptime ticks and the daemon pid. Every later `Apply` returns it again until the daemon's process started after it (from `/proc/<pid>/stat` starttime, or a new pid within the same 10 ms tick). Kea only ever needs "start", which is re-derived from the socket on every Apply, so it cannot be lost |
| **M3 / D-079** kea-ctrl-agent | Removed everywhere: no `kea-ctrl-agent.conf`, no HTTP client, no `CtrlAgentBin`, no ALLOWLIST row, and no agent in the tests. The DHCP servers are driven over their own unix sockets only. `Client` refuses (`ErrInsecure`) a socket that grants anything to others, and a socket directory with group write or any access for others |
| **M4** product unbound paths | Product control socket is `/run/unbound.ctl` and pidfile `/run/unbound.pid` (the Debian defaults, no `/run/unbound`). `Apply` creates missing parent directories. `TestProductPaths` pins the product render. The integration test runs `unbound-checkconf` on the staged product render (host: `/var/lib/unbound/root.key` present) |
| L1 output cap | `unbound-control` / `chronyc` output at the runner's capture limit is now an error (`unbound.ErrOutputTruncated`, `chrony.ErrDaemon`). `unbound.State` sets `localDataTruncated` instead of returning a partial `list_local_data` |
| L2 rollback context | Rollback runs on `context.WithoutCancel(ctx)` with its own timeout, in kea, unbound and chrony |
| L3 leases | Leases are no longer part of Kea `State` / `Retrieve`; counts come from the statistics. `Leases(ctx, fam, limit)` is paged (1000 per message), with limit ≤ 100 000 and a truncated flag |
| L4 leftovers | The stale `unbound.ctl` is gone (it is removed by the fix-round runs). **`/run/vrx-test/w6/{c1,x}` are still there: every `rm -rf` of them was denied by this session's permission system.** Please remove them (they are mine: hand-made chrony/unbound/kea test configs, no running process) |
| L5 idle bind | `Paths.IdlePort`: product 53, tests 3<slot>53. An idle test instance never renders `@53` |
| L6 chrony key ids | Id = FNV-32a(ref) in 1..2³²−1. Adding a key does not renumber the others (`TestKeyIDsStable`). A collision is refused |
| L7 Kea default mapper | The product default is `NoMapper`, which refuses every interface until the linux-cp mapper is injected. Tests use `IdentityMapper` |

### Evidence

```
$ go test -count=1 -v -run 'TestSubnetIDCollision|TestSocketPrivacy|TestProductPaths|TestApply|TestKeyIDsStable' ./internal/renderers/{kea,unbound,chrony}/
    --- PASS: TestApply/config-set_failure_restores_files_and_previous_config (0.00s)
    kea_test.go:604: after adding the colliding subnet: map[vlan1340126:1204698144 vlan957918:2851699104]
    kea_test.go:604: after adding the colliding subnet: map[vlan1340126:1204698144 vlan957918:2851699104]   (new Renderer = agent restart)
--- PASS: TestSubnetIDCollision (0.00s)
--- PASS: TestSocketPrivacy (0.00s)
ok  	ngfw/agent/internal/renderers/kea	0.054s
    --- PASS: TestApply/reload_with_convergence_check (0.01s)
    --- PASS: TestApply/listen_change_needs_restart_and_stays_pending (0.05s)
    --- PASS: TestApply/stale_socket_is_not_running (0.00s)
    --- PASS: TestApply/reload_failure_restores (0.00s)
    --- PASS: TestApply/output_cap_is_an_error (0.08s)
--- PASS: TestProductPaths (0.00s)
ok  	ngfw/agent/internal/renderers/unbound	0.209s
--- PASS: TestKeyIDsStable (0.00s)
    --- PASS: TestApply/sources_and_keys_reload_and_conf_restart (0.05s)
    --- PASS: TestApply/chronyc_failure_restores (0.04s)
ok  	ngfw/agent/internal/renderers/chrony	0.123s

$ VRX_INTEGRATION=1 go test -count=1 -v ./internal/renderers/{kea,unbound,chrony}/...     (full log: /root/ngfw-wt/logs/RF-3-integration-fix.log)
kea_integration_test.go:266: dhcp4: config-get vs rendered (normalised subset diff): 0 differences   (also after change and rollback)
kea_integration_test.go:288: control socket /run/vrx-test/w6/kea/run/kea4.sock mode -rwxrwx---, dir mode -rwxr-x--- (D-079: private unix socket, no HTTP)
unbound_integration_test.go:61: product paths (control-interface /run/unbound.ctl, pidfile /run/unbound.pid, /var/lib/unbound/root.key): unbound-checkconf accepted the staged render
unbound_integration_test.go:239: listen change, Apply #1: unbound: unbound needs restart (unbound): listen addresses, port, views, paths or remote-control changed (unbound applies them only at startup)
unbound_integration_test.go:239: listen change, Apply #2: unbound: unbound needs restart (unbound): … (still pending: unbound has not restarted since)
unbound_integration_test.go:245: before restart: 127.0.0.1:3654 refused (dial tcp 127.0.0.1:3654: connect: connection refused) — and Apply did not report success
unbound_integration_test.go:262: after restart: Apply → nil (pending cleared, convergence check passed); net.Resolver @127.0.0.1:3654 gw.rig.example.test → [10.6.10.1]
chrony_integration_test.go:280: conf change: chrony: chronyd needs restart (chrony): chrony.conf changed (only sources and keys reload at run time)
chrony_integration_test.go:284: second Apply, same files, chronyd not restarted: chrony: chronyd needs restart (chrony): … (still pending: chronyd has not restarted since)
chrony_integration_test.go:292: Apply after the restart: nil (pending request cleared: chronyd started after it)
ok  	ngfw/agent/internal/renderers/kea	1.215s
ok  	ngfw/agent/internal/renderers/unbound	11.368s
ok  	ngfw/agent/internal/renderers/chrony	1.908s

$ pgrep -af /run/vrx-test/w6/            → (none)
$ systemctl is-active kea-dhcp4-server kea-dhcp6-server unbound chrony
inactive inactive inactive active        (chrony = host timesync, untouched; review I1 accepts it)
$ diff etc-stat-before.txt <stat of /etc/kea /etc/unbound /etc/chrony now>   → identical (15 files)
$ ip netns list | grep w6                → (none)

$ tools/ci.sh --base main
branch    task/RF-3 @ 53b8e80   (base: main)
no contract files changed in the 16 commit(s) of HEAD since main (d30c543)
ok: gitleaks — scanned ~396899 bytes (396.90 KB) in 1.08s no leaks found
CI GATE PASSED
```

Decisions added in this round: D-RF3-2 still holds (`config-set` without `config-write`). D-RF3-3 is replaced by the M1 scheme (ids persisted in the applied config, salted hashing only for new names; renaming a subnet gives it a new id). D-RF3-13: the pending restart request lives in `/run/vrx/renderers/<daemon>.pending` and is cleared by a daemon process start after it (options: (a) conf mtime vs start time, which fails for reload-only changes; (b) persisted request + process start time [taken]). D-RF3-14: Kea control sockets are accepted only when private (no access for others, no group write on the directory). Q1 is closed by review I1. Q3 stays open (P05 to hoist `ActionRequired`, with pending persistence, into `renderers`).
