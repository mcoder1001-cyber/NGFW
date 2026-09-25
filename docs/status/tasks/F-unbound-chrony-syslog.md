# F-unbound-chrony-syslog: Unbound DNS + VPP DNS cache, chrony NTP, syslog export + log explorer

Branch `task/F-unbound-chrony-syslog` (worktree `/root/ngfw-wt/F-unbound-chrony-syslog`), slot 10 (`w10`). Base was main@4e2b21d;
main@4f472cc7 was merged in at e117766f (TD-11b ownership guard, TD-8 seams, TD-7, TD-4). `tools/ci.sh --base main`:
**CI GATE PASSED** (cf6a2635, below; fix round 1: 53840d3f). Questions: `F-unbound-chrony-syslog-questions.md` (Q2 is an incident; read it first).
Contract: `F-unbound-chrony-syslog-contract.md`.

## Fix round 1 (review b665904f: APPROVE WITH CHANGES)
Every code fix has a test that fails on the old code. I checked that by putting the pre-review file (b665904f) back
under the new test, one file at a time (below), and then restoring it.

| finding | fix | test (result on the old code) |
|---|---|---|
| H1 IPv6-only upstreams crash VPP | Zod refinement on `services.dns.vppCache` → 400 at `/services/dns/vppCache/upstreams` "VPP DNS cache needs at least one IPv4 upstream (VPP 26.06 defect, D-137)" (`contract(schema)` 48a00bfc); projection error `services.dns-vpp-cache-upstream`, same pointer and text (`desired/dns.go`); `dns.enable` refuses enable=1 with `ErrNoIPv4Upstream` and sends nothing unless an IPv4 server was added on the running VPP; the lookup needs that fact too (H2) | schema `services.dns.vppCache (D-137)` (old services.ts: **1 failed**); `TestVPPCacheNeedsAnIPv4Upstream` (old desired/dns.go: **FAIL**); `TestIPv6OnlyUpstreamsAreNeverEnabled` — the model now keeps v4 and v6 vectors apart (crash = request while no IPv4 server was ever added), and the test also drives the old shape (server add + raw enable) and sees the crash window |
| H2 readiness from the stored document | `dns.Readiness` (`descriptors/dns/readiness.go`): recorded only when VPP accepted an IPv4 server add and `dns_enable_disable(1)`, bound to the VPP boot identity (boot_id, VPP PID, start time); cleared on disable, delete, identity change or unreadable identity. `dnsLookup` = `GlobalsOwner && !DEGRADED && Readiness.Ready`; `DnsVppCacheState.appliedByThisAgent` from the same fact | `TestReadinessDoesNotSurviveAVPPRestart` (simulated VPP restart: not ready, lookup sends nothing, ready again after the resync); `TestDNSLookupReadinessIsLiveNotStored` (stored document enabled + fresh fake VPP → FAILED_PRECONDITION, zero dns_* messages; old rpc_dns.go: **FAIL**, it sent `dns_resolve_name`) — also covers L6 |
| M1 V-item wrong/incomplete | rewritten from the review's source analysis: trigger "no IPv4 name server added since VPP started" (`is_enabled` plays no part), `dns_resolve_ip`, UDP-53 packets, IPv6-only servers and `show dns servers` crash; add/del and enable/disable are safe; the non-crashing defects; upstream fix list; fallback list. Still titled V-new (the merger numbers it, L8). The manager corrects D-137 in the LOG | docs (`docs/vpp-code-track.md`, `docs/agent/descriptors/dns.md`) |
| M2 readonly reads the whole journal | `@MinRole('admin')` on `GET /api/v1/state/logs`; the Logging tab shows a notice instead of the explorer for non-admins and never queries it; user page says so | `route-guard.test.ts` ADMIN_ONLY row (old controller: **FAIL**, readonly got through to 501); e2e logs test (readonly 403, operator 403, admin 200); `LoggingTab.test.tsx` (old tab: **2 failed**, readonly/operator) |
| M3 state RPCs not serialised | one walk in flight per kind (`rpc_dns_walk.go`: dns, ntp, syslog state, journal); a second request waits up to 3 s, then `UNAVAILABLE` → API 503 (D-132) | `TestStateWalksAreSerialised` (old rpc_dns.go: **FAIL**, second walk ran) |
| M5 open resolver on every VPP address | VPP 26.06 has no per-interface switch or client ACL for the dns plugin (ports registered globally, `dns.c:84-97`), so: DryRun warning `services.dns-vpp-cache-exposure` at `/services/dns/vppCache/enabled` naming the addresses it answers on, and a user-page warning (block UDP 53 on untrusted interfaces with an ACL) | `TestHostServicesProjection` expects the warning (old desired/dns.go: **FAIL**) |
| M4 nothing acts on unbound/chrony restart requests on a real box | not a code change (product gap). **Hand-off:** on a real box, unbound's and chrony's start/restart requests stay pending: nothing runs `systemctl start|restart` for them (unbound.service ships disabled). This belongs to PENDING-agent-privileges / P10; the review recommends (a) the globals owner runs allow-listed `systemctl start|restart unbound|chrony` with RF-3's convergence check | — |
| L1 | `Enable` doc comment no longer claims a dependency | — |
| L4 | topology harness refuses a slot outside 1..12 (`w0`/`w00` would take the product stack's ports) | `go vet` (the harness is the test) |
| L7 | source-scan guard: `DNSResolveName`/`DNSResolveIP` only in `descriptors/dns/dns.go`, no `show dns servers` CLI string anywhere in the agent | `TestResolveMessagesOnlyThroughTheGuardedHelpers`, `TestResolveGuardFindsDirectCalls` (planted violations found) |
| L9 | `TODO(PENDING-secret-channel)` on the projection-test exemption | — |

Not done in this round: L2 (Validator on non-owners) and TD-13 `Stage`/`Validate` at the TD-13 rebase; L3 (syslog `vrf`
≠ default: note or error) waits for the manager's decision; L5 (pin `since`/`--until` across explorer pages) is open;
L8/L11 are the manager's. At the rebase after F-kea: the fold per review Q5 (keep my list-safe `unsupported()` helper);
after TD-23: `registerActionHandler('dnsLookup', …)` once at module load (Q6).

**Host run:** none in this round (TD-25 has not landed; no VPP DNS-plugin call was made). After TD-25 I re-run
`TestUnboundChronySyslog` on the fix-round SHA (L10); it sends no VPP DNS call.

Commits: 9119000a (agent H1/H2/M3/M5/L1), 48a00bfc (contract(schema) H1), 907d266a (API/web M2), 6226fc97 (L4),
3c20004a (M1/M5 docs, L7, L9), b5181423 (web M2 test), the regenerated api-client (`contract(api-client)`) and
CLI operation table (53840d3f), and this status update.

`TMPDIR=/tmp/g-w10 tools/ci.sh --base main` at 53840d3f (the first two runs failed on generated output: the explorer's
OpenAPI summary changed, so the api-client and the CLI operation table were regenerated and committed):
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m01s
  generate + generated-output gate                   1m37s
  forbidden patterns (+ gitleaks)                    0m05s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   1m37s
  apps/agent: make lint test build                   0m34s
  apps/cli: make lint test build                     0m20s
  test/ Go modules, unit mode (… test/topology/unbound-chrony-syslog)   0m08s
  deploy/vpp: shellcheck + apply-startup fake-host harness   0m12s
  mode quick · wall time 4m39s · logs /root/ngfw-wt/logs/ci/F-unbound-chrony-syslog-20260925-103020-852438
CI GATE PASSED
```
Also run: `go test -race -count=1` of descriptors/dns, subsystems, agent, desired, actions/unbound-chrony-syslog and the
three renderers (all ok); API e2e on the slot's PostgreSQL + fake agent, `unbound-chrony-syslog.e2e.test.ts`: 4 passed
(readonly and operator get 403 from `/state/logs`).

## ⚠ Incident (Q2): my run crashed the shared VPP at 2026-09-25 04:27:21 (NRestarts 1 → 2)
The topology run sent one `dns_resolve_name` (through `POST /actions/dns-lookup`) to the shared VPP, where the dns plugin
had never been enabled. VPP 26.06 dereferenced a NULL name server (`vnet_dns_resolve_name` → `vnet_send_dns4_request` →
`ip4_sas`, dns.c:234). I stopped at once. Fixes on this branch, all with unit tests on a fake VPP that models the defect:
- `dns_lookup` is refused with FAILED_PRECONDITION (API 409) unless the agent is the globals owner and its applied
  configuration enables the VPP cache with an upstream (`TestLookupRefusedWithoutAReadyCache`). Fix round 1 replaced
  "applied configuration" with the live `dns.Readiness` fact and requires an IPv4 upstream (above).
- DF-8 `dns.ResolveName` / `ResolveIP` take a `dns.Ready` precondition. Without it they send nothing (D-137,
  `TestResolveHelpersRefuseWithoutReady`).
- The DF-8 descriptors never leave the resolver enabled without a server. A server delete disables the switch first, and
  `dns.enable` carries the upstream set, so the transaction re-enables it after the new servers exist (the scheduler runs
  deletes before creates). `TestUpstreamChangesNeverLeaveAnEnabledResolverWithoutServers` drives the real scheduler.
- The DF-8 host test is opt-in twice (`VRX_DNS_VPP_HOST=1`, D-064). V-item in `docs/vpp-code-track.md`.
Every later run: NRestarts stayed 2 (pasted below).

## What was built
| layer | what |
|---|---|
| contract (schema) | `management.syslog[i].{facilities, format, queueSize, tls{caRef, certRef, keyRef, authMode, permittedPeers}}` (D-086, `domains/ext/syslog.ts`, all optional); rules `services.unbound-chrony-syslog-vpp-cache-port`, `-forwarder-loop`, `management.unbound-chrony-syslog-tls` |
| contract (proto) | SyslogTarget 6–9 (+ `SyslogTls`), ActionRequest 7 `dns_lookup`, rpc `DnsState`, `NtpState`, `SyslogState`, `SyslogEntries` + messages in the feature section; proto.md §11 |
| agent | singleton descriptors `unbound.config/vrx`, `chrony.config/vrx` (domain `services`) and `rsyslog.config/vrx` (domain `management`, which this branch introduces). Each Value is the render input embedded in the rendered file (`# vrx-input:`), proven on Retrieve by re-rendering byte for byte; drift → re-apply. Start/restart requests are recorded, not failures (D-079). `services.dns.vppCache` → DF-8 `dns.*` (owner: applied, others: required). Secret refs refused at DryRun. Slot/product path split (Q8). State RPCs, journald log explorer, dns_lookup action. `RecordsNoOwnership()` on all five descriptors (TD-11b) |
| API | `GET /api/v1/state/{dns,ntp,syslog,logs}`, `POST /api/v1/actions/dns-lookup` (static route, operator, audited), fake agent (P5), e2e, OpenAPI + api-client + CLI operation table regenerated |
| UI | Services › DNS / NTP / Logging (tab registry): schema-driven forms, live status, pending-daemon-action banner, VPP cache panel, lookup, log explorer on `ServerDataGrid` (severity/facility/window/text, server paging); polls ≥ 30 s + Refresh (D-132); en + fa |
| docs | `docs/user/services/unbound-chrony-syslog.md` (5 screenshots), agent sections in `docs/agent/renderers/{unbound,chrony,rsyslog}.md`, `docs/agent/descriptors/dns.md` |
| evidence | `test/topology/unbound-chrony-syslog` (real agent + API + slot unbound/chronyd/rsyslogd, run.sh) |

## Acceptance (pasted from the runs)

Slot end-to-end run: `eval "$(tools/lab env 10)"; VRX_UCS_SHOTS=<script> VRX_UCS_SHOTS_OUT=docs/user/services/img
test/topology/unbound-chrony-syslog/run.sh -run TestUnboundChronySyslog`, 2026-09-25 04:50 (before the 05:05 af_packet
problem; no host run since):
```
ucs_test.go:254: systemctl show vpp -p NRestarts (before): NRestarts=2
ucs_test.go:256: host units (before): Id=chrony.service ActiveState=active UnitFileState=enabled MainPID=995 Id=rsyslog.service ActiveState=active UnitFileState=enabled MainPID=1033 Id=unbound.service ActiveState=inactive UnitFileState=disabled MainPID=0
ucs_test.go:328: commit ucs-1: revision 1, status applied
ucs_test.go:335: GET /state/dns before any daemon runs: running=false pendingActions=[{"action": "start", "daemon": "unbound", "reason": "configuration has resolvers but unbound is not running", "unit": "unbound"}]
ucs_test.go:341: PUT resolver with forwarder = listen address → 400 {"type":"https://vrx.dev/problems/validation","title":"Validation failed","status":400,"detail":"the document does not match the schema","instance":"/api/v1/config/services/dns/resolvers/lab","errors":[{"pointer":"/services/dns/resolvers/lab/forwarders/0","message":"a forwarder must not be one of the listen addresses (loop)"}]}
ucs_test.go:349: started unbound pid 3782682: /usr/sbin/unbound -d -c /run/vrx-test/w10/unbound/unbound.conf
ucs_test.go:354: started chronyd(server) pid 3782683: /usr/sbin/chronyd -f /run/vrx-test/w10/chrony/server/chrony.conf -n -x -l /run/vrx-test/w10/chrony/server/chronyd.log
ucs_test.go:356: started chronyd(agent) pid 3782684: /usr/sbin/chronyd -f /run/vrx-test/w10/chrony/agent/chrony.conf -n -x -l /run/vrx-test/w10/chrony/agent/log/chronyd.log
ucs_test.go:359: started rsyslogd pid 3782686: /usr/sbin/rsyslogd -n -f /run/vrx-test/w10/rsyslog/rsyslog.conf -i /run/vrx-test/w10/rsyslog/rsyslogd.pid
ucs_test.go:375: net.Resolver{127.10.0.53:4053}.LookupHost(gw.lab.example) = [10.10.53.1]
ucs_test.go:388: $ chronyc -n -h /run/vrx-test/w10/chrony/agent/chronyd.sock sources
    MS Name/IP address         Stratum Poll Reach LastRx Last sample
    ===============================================================================
    ^* 127.0.0.1                    10  -1     7     0   -108ns[  +13us] +/-   15us
ucs_test.go:404: $ logger -u /run/vrx-test/w10/rsyslog/log.sock -p local7.notice -t vrx-ucs-w10 'hello from w10 ucs-83d41b01'
    collector 127.0.0.1:4016 received: "97 <189>1 2026-09-25T04:50:44.292599+03:30 ubuntu-26 vrx-ucs-w10 - - -  hello from w10 ucs-83d41b01"
    (a daemon.info line sent next was not forwarded: the facilities filter local7 holds)
ucs_test.go:432: GET /api/v1/state/dns → {"configPath":"/run/vrx-test/w10/unbound/unbound.conf","forwards":[{"zone":"corp.example.","kind":"forward","addresses":["10.10.99.53"]}],
    "localZones":[{"zone":"lab.example.","type":"static"}],"localData":["gw.lab.example. 3600 IN A 10.10.53.1"],"pendingActions":[],"running":true,
    "vppCache":{"configured":false,"appliedByThisAgent":false,"upstreams":[],"live":false}, …}
ucs_test.go:432: GET /api/v1/state/ntp → {"running":true,"sources":[{"mode":"^","state":"*","name":"127.0.0.1","stratum":10,"reach":"377",…}],
    "tracking":{"refName":"127.0.0.1","stratum":11,"leap":"Normal","systemTime":-0.000013072,…},"pendingActions":[]}
ucs_test.go:432: GET /api/v1/state/syslog → {"inputs":{"imuxsock":2},"pendingActions":[],"targets":[{"index":0,"action":"vrx_export_0_6cb32106",
    "target":"127.0.0.1:4016","protocol":"tcp","reported":true,"processed":1,"failed":0,"queueSize":0,…}]}
ucs_test.go:432: GET /api/v1/state/logs?severity=notice&pageSize=3 → {"items":[{"time":"2026-09-25T01:20:43.567Z","severity":"error","facility":"daemon",
    "identifier":"vpp","unit":"vpp.service","message":"vnet_set_flow_classify_intfc:68: …"}, …],"page":1,"pageSize":3,"total":5000,"scanned":5000,"truncated":true,"source":"journald"}
ucs_test.go:437: POST /api/v1/actions/dns-lookup (slot agent, not the globals owner) → 409 {"type":"https://vrx.dev/problems/agent-precondition",…,"grpcCode":"FAILED_PRECONDITION",
    "detail":"agent: dns_lookup needs the VPP DNS cache enabled with an upstream by this agent (the globals owner, D-071): VPP 26.06 crashes on dns_resolve_name while its dns plugin has no name server (SIGSEGV in ip4_sas, docs/vpp-code-track.md), so the lookup is never sent otherwise"}
ucs_test.go:444: after the listen-port commit (revision 2): pendingActions=[{"action":"restart","daemon":"unbound","reason":"listen addresses, port, views, paths or remote-control changed (unbound applies them only at startup)","unit":"unbound"}]
harness_test.go:160: stopped vrx-agent pid 3782465
ucs_test.go:454: agent stopped; deleted /run/vrx-test/w10/unbound/unbound.conf, /run/vrx-test/w10/chrony/agent/sources.d/vrx.sources, /run/vrx-test/w10/rsyslog/rsyslog.conf (simulated loss)
ucs_test.go:457: started vrx-agent pid 3783213
ucs_test.go:468: files re-rendered 0.25 s after the agent start
ucs_test.go:470: agent log excerpt after the restart:
    {"time":"2026-09-25T04:50:50.352+03:30","level":"INFO","msg":"host services wired","owner":"w10","feature":"unbound-chrony-syslog","product_paths":false,"unbound_conf":"/run/vrx-test/w10/unbound/unbound.conf",…,"vpp_dns_cache":"required only"}
    {"time":"2026-09-25T04:50:50.378+03:30","level":"INFO","msg":"reconcile start","owner":"w10","mode":"resync","domains":["interfaces","vrfs","routing","services","management"]}
    {"time":"2026-09-25T04:50:50.448+03:30","level":"WARN","msg":"unbound configuration written; the daemon must act on it","daemon":"unbound","unit":"unbound","action":"restart","reason":"listen addresses, port, views, paths or remote-control changed (unbound applies them only at startup)",…}
    {"time":"2026-09-25T04:50:50.485+03:30","level":"INFO","msg":"chrony configuration applied","daemon":"chronyd","conf":"/run/vrx-test/w10/chrony/agent/chrony.conf","enabled":true}
    {"time":"2026-09-25T04:50:50.517+03:30","level":"INFO","msg":"reconcile done","mode":"resync","status":"APPLY_STATUS_APPLIED","summary":"created:3 unchanged:4",…}
ucs_test.go:472: after the agent restart: pendingActions=[{"action":"restart","daemon":"unbound","reason":"listen addresses, port, views, paths or remote-control changed (unbound applies them only at startup)","unit":"unbound"}]
harness_test.go:160: stopped unbound pid 3782682
ucs_test.go:349: started unbound pid 3783391
harness_test.go:160: stopped rsyslogd pid 3782686
ucs_test.go:359: started rsyslogd pid 3783426
ucs_test.go:493: after restarting unbound and rsyslogd: dns pendingActions=[] syslog pendingActions=[]
ucs_test.go:502: revision 3: list_local_data=["gw.lab.example. 3600 IN A 10.10.53.1","new.lab.example. 3600 IN A 10.10.53.2"]; new.lab.example → [10.10.53.2]
ucs_test.go:507: POST /api/v1/config/rollback/2 → status applied, revision id 4 (kind rollback, parentId 3)
ucs_test.go:510: after rollback: list_local_data=["gw.lab.example. 3600 IN A 10.10.53.1"]; new.lab.example → … no such host
ucs_test.go:515: GET /state/drift after rollback (Retrieve vs running): {"changes":[],"subsystems":["interfaces","vrfs","routing","services","management"]}
ucs_test.go:526: screenshots:
    ucs-dns-en.png  html dir/lang=ltr/en  rows=2  alerts=["VPP has no getter for its DNS cache: this is the configuration as committed, not a live re"]  pageErrors=0
    ucs-ntp-en.png  html dir/lang=ltr/en  rows=2  alerts=[]  pageErrors=0
    ucs-logging-en.png  html dir/lang=ltr/en  rows=26  firstCells=["9/25/26, 4:51:14 AM","info","authpriv","runuser","pam_unix(runuser:session): session closed for user postgres"]  pageErrors=0
    ucs-dns-fa-rtl.png  html dir/lang=rtl/fa  rows=2  alerts=["VPP برای حافظهٔ نهان DNS خود خواندنی ندارد: …"]  pageErrors=0
    ucs-logging-fa-rtl.png  html dir/lang=rtl/fa  rows=26  firstCells=["۱۴۰۵/۷/۳, ۴:۵۱:۳۱","خطا","daemon","vpp","af_packet: fd 33 reason af_packet_fd_error: Network is down"]  pageErrors=0
ucs_test.go:316: cleanup commit → 200 applied
ucs_test.go:532: after the cleanup commit unbound.conf is idle: true
harness_test.go:160: stopped rsyslogd / unbound / chronyd(agent) / chronyd(server) / vrx-api / vrx-agent (by PID)
ucs_test.go:171: pg-test drop w10: ok nothing named vrx_w10 / vrx_w10 remains
ucs_test.go:258: systemctl show vpp -p NRestarts (after): NRestarts=2
ucs_test.go:262: host units (after, unchanged): Id=chrony.service ActiveState=active UnitFileState=enabled MainPID=995 Id=rsyslog.service ActiveState=active UnitFileState=enabled MainPID=1033 Id=unbound.service ActiveState=inactive UnitFileState=disabled MainPID=0
--- PASS: TestUnboundChronySyslog (64.67s)
ok  	ngfw/test/topology/unbound-chrony-syslog	64.699s
```

- [x] **Slot Unbound answers a local record** at its slot address/port through a Go `net.Resolver` pinned there:
      `net.Resolver{127.10.0.53:4053}.LookupHost(gw.lab.example) = [10.10.53.1]`. **chrony**:
      `chronyc -n -h /run/vrx-test/w10/chrony/agent/chronyd.sock sources` lists (and selects, `^*`) the server.
- [x] **rsyslog** test instance forwards a `logger -u <slot log.sock>` line to the slot TCP collector (octet-counted
      RFC 5424 `<189>` = local7.notice). The log explorer page shows lines from its source (journald), in
      `docs/user/services/img/ucs-logging-{en,fa-rtl}.png`.
- [x] **Agent-restart simulation**: the rendered files were deleted while the agent was down. They were re-rendered
      0.25 s after start (log above), and the unbound restart request was still pending until unbound restarted.
- [x] **Rollback restores the previous rendering**: the rollback to revision 2 removes `new.lab.example` from
      `list_local_data` and the resolver answers NXDOMAIN; `/state/drift` (Retrieve vs running) shows no changes.
- [x] **Forwarder equal to a listen address → 400 problem+json with a pointer** (real API above; also the e2e: a forward-zone
      loop → semantic 400 at commit with `/services/dns/resolvers/lan/forwardZones/0/forwarders/0`; TLS without a CA →
      `/management/syslog/0/tls`).
- [x] **`tools/ci.sh --base main` green** (below).
- UI screenshots of all three tabs against the real endpoints: `docs/user/services/img/` (DNS/NTP/Logging en, DNS/Logging fa-RTL);
  the pending-change bar shows the uncommitted NTP pools change in each shot.

API e2e on the slot's PostgreSQL + fake agent (`apps/api/test/e2e/unbound-chrony-syslog.e2e.test.ts`, 09:3x):
```
 ✓ test/e2e/unbound-chrony-syslog.e2e.test.ts (4 tests) 12069ms
   ✓ … commits DNS, NTP and syslog through the pointer routes and shows their live state  1260ms
   ✓ … log explorer: severity/facility/text filters, paging, bounded query validation
   ✓ … dns-lookup resolves through the (fake) VPP cache; refused without it; operators only; name validated  632ms
   ✓ … forwarder equal to a listen address → 400 problem+json with the pointer  350ms
 Test Files  1 passed (1)      Tests  4 passed (4)
```
Unit tests of this feature (`go test -count=1`, vitest):
```
ok  	ngfw/agent/internal/renderers/unbound	0.440s
ok  	ngfw/agent/internal/renderers/chrony	0.311s
ok  	ngfw/agent/internal/renderers/rsyslog	4.072s
ok  	ngfw/agent/internal/descriptors/dns	0.020s
ok  	ngfw/agent/internal/desired	0.115s
ok  	ngfw/agent/internal/actions/unbound-chrony-syslog	0.031s
ok  	ngfw/agent/internal/agent	11.734s
ok  	ngfw/agent/internal/subsystems	5.620s
 ✓ src/semantic/unbound-chrony-syslog.test.ts (21 tests)          (packages/schema)
 ✓ src/domains/services/unbound-chrony-syslog/model.test.ts (3 tests)   (apps/web; whole web suite 14 files / 99+3 tests green)
```
CI (`TMPDIR=/tmp/g-w10 tools/ci.sh --base main` at cf6a2635):
```
== summary (quick) ==
  contract guard: HEAD vs main                       0m00s
  tools (golangci-lint, gitleaks)                    0m02s
  install (pnpm --frozen-lockfile --prefer-offline)   0m00s
  generate + generated-output gate                   1m53s
  forbidden patterns (+ gitleaks)                    0m05s
  packet-trace ban on the shared VPP (D-128)         0m01s
  lint · typecheck · unit tests · build (turbo)   1m47s
  apps/agent: make lint test build                   1m06s
  apps/cli: make lint test build                     0m19s
  test/ Go modules, unit mode (test/integration/smoke test/topology/interfaces test/topology/unbound-chrony-syslog)   0m08s
  deploy/vpp: shellcheck + apply-startup fake-host harness   4m28s
  mode quick · wall time 9m51s · logs /root/ngfw-wt/logs/ci/F-unbound-chrony-syslog-20260925-051405-16977
CI GATE PASSED
```

## Obligations (envelope) and where they are met
| obligation | evidence |
|---|---|
| D-079 unbound listen/port/interface change → persisted restart request + convergence check; pending until acted on | RF-3 renderer (unchanged), `unbound.Descriptor.Pending`, `TestDescriptorRestartPendingUntilActedOn`, run above (pending survives the agent restart, cleared after the unbound restart) |
| never restart a system unit from a test slot | slot agents: RF-3 returns requests only; rsyslog `DeferredController` (`TestDescriptorDeferred`: no `systemctl` call); host units unchanged before/after (pasted) |
| D-086 stand-ins → contract; TLS built but untested until the driver is packaged | contract commits; renderer reads typed fields; TLS refused at DryRun (secret channel) and by Validate without `lmnsd_ossl.so` |
| D-050 NTP only in `services.ntp` | projection reads only `services.ntp` |
| D-063/D-076/D-071 dns globals write-only, registered by the owner, non-owners require | `registerDNSCache` (owner `RegisterGlobalsReady`, else require-mode descriptors); DryRun note on `vppCache`; D-137 guards |
| D-082 opt-in VPP dns host test holds `flock -x` globals lock | DF-8 test unchanged (globals lock) + now also `VRX_DNS_VPP_HOST=1`; it was **not** run |
| RF-4 M2 (no restart for an unchanged rendering) / M3 (host impstats) | unchanged in the renderer; the deferred path keeps M2 (`applyDeferred` returns nothing / the still-pending request for unchanged files) |
| TD-11b ownership declaration | `RecordsNoOwnership()` on `unbound.config`, `chrony.config`, `rsyslog.config`, `dns.name-server`, `dns.enable`; agent tests green after merging main with the guard |
| D-132 no UI timer < 30 s, Refresh button | `STATE_POLL_MS = 30_000`, Refresh on every panel and the explorer |
| D-128 no packet trace | none used; CI trace ban step passed |

## Decisions taken (for the LOG, with options)
| id | decision | options | why |
|---|---|---|---|
| UCS-1 | renderer stage = one singleton descriptor per renderer | (a) singleton descriptors (b) shared service.go stage | D-109 d; no agent-core change |
| UCS-2 | Retrieve Value = render input embedded in the file, proven by re-render | (a) embedded input (F-kea pattern) (b) in-memory last applied (c) parse daemon state | (a) survives restarts, detects hand edits, never reports a foreign file |
| UCS-3 | only the globals owner uses the product paths / host units; others render into `/run/vrx-test/<owner>` (Q8) | (a) GlobalsOwner (b) new env var (c) owner=="vrx" | the dev host's main stack runs `VRX_GLOBALS_OWNER=0`: (a) keeps it off the host's rsyslog/chrony without new config |
| UCS-4 | log explorer = journald, fixed argv, text matched in the agent (Q3) | (a) journald (b) omfile in RF-4 | (b) breaks RF-4's template rule |
| UCS-5 | VPP cache shown as configured; DryRun `agent.unsupported-field` note (Q4) | (a) note + "configured" (b) error (c) silent | write-only; drift must not compare it |
| UCS-6 | syslog `vrf` ≠ default: noted, not applied (target rendered) | (a) note (b) DryRun error | the daemons live in the host namespace; valid schema examples must project |
| UCS-7 | secret refs (`ntp.servers[].keyRef`, `syslog[].tls`) refused at DryRun (`agent.secret-channel-pending`) | (a) refuse (b) render without | envelope / PENDING-secret-channel |
| UCS-8 | slot rsyslog = `DeferredController` (restart request + pidfile) | (a) deferred (b) agent spawns daemons (c) ProcessController | the agent never starts processes outside the runner |
| UCS-9 | slot chrony sources use port 3<N>23 | (a) slot port (b) 123 | a slot never queries the host's chronyd |
| UCS-10 | `dns_lookup` only where the agent enabled the cache on the running VPP (live `dns.Readiness`, fix round 1); DF-8 `Ready` precondition; IPv4 upstream required; disable before a server delete (D-137) | — | the 04:27 crash; review H1/H2 |
| UCS-11 | dns.* registered in require mode on non-owners | (a) require mode (b) not registered + warning | a slot document enabling vppCache fails loudly instead of being ignored |

## Shared hunks (outside the owned files)
- `apps/agent/internal/subsystems/subsystems.go`: `Services`/`Management` consts, `Domains` entries, one Register call (A1 anchors).
- `apps/agent/internal/agent/projection.go`: one call in `project()`, one in `assemble()` (A2).
- `apps/agent/internal/agent/server.go`: `Action` parameter `_` → `stream`; one case `ActionRequest_DnsLookup` (A4, Q7).
- `apps/agent/internal/agent/{service_test,projection_test}.go`: registry-derived domain assertions; the secret-channel rule
  is allowed for valid examples (Q7).
- `packages/proto/vrx/v1/dataplane.proto` (RPCs, ActionRequest 7, SyslogTarget 6–9, feature section), `docs/contracts/proto.md`,
  `packages/proto/test/fixtures/unbound-chrony-syslog-full.json` (C4-C6), generated files (C7).
- `packages/schema/src/domains/management.ts` (import + spread, SY4), `packages/schema/src/index.ts` (C3), `semantic/index.ts` (C2).
- `apps/api/src/app.module.ts` (P1), `agent/agent.client.ts` (P4), `testing/fake-agent.ts` (import + spread, P5, Q6).
- `apps/web/src/{i18n.ts,nav/nav.ts,nav/nav.test.ts,domains/services/tabs.ts}` (W2, W3, tab registry).
- `apps/agent/internal/renderers/ALLOWLIST.md` (journalctl row, SY5), `docs/vpp-code-track.md` (V-new, A7).
- `apps/cli/internal/api/operations_gen.go` regenerated.

## Out of scope (not built)
Kea/DHCP, SNMP, alarms/dashboards, support bundle, PTP, NTS server certificates, DNS views beyond RF-3, a log database;
CLI `show dns|ntp|logs` commands (apps/cli is P13's); running the opt-in VPP dns host test.

## Left for the manager / after F-kea
- **Product gap (review M4, for PENDING-agent-privileges / P10):** on a real box nothing acts on unbound's or chrony's
  start/restart requests; they stay pending (unbound.service ships disabled). rsyslog's product path restarts itself.
- Fold with F-kea-dhcp-relay when it lands (Q5): I merge main then and keep one `ServicesImplemented`/`ServicesUnsupported`, one
  `Services` const + Domains entry, one nav item.
- Fake-agent `action` chaining with F-vrf-static-ecmp (Q6).

## Cleanup
Every process was started by the tests and stopped by PID. The slot database was dropped (pg-test). `vppctl show interface | grep -c loop10` → 0. No systemd unit was touched.
`dist/` (apps and packages) and `apps/agent/bin` were deleted after the last CI run. **Not removed:** the permission prompt
refused the `rm` of `/run/vrx-test/w10/ucs/` (48 KiB tmpfs: agent-state, agent/API logs, the run's test JWT/secret key file
0600). Please delete it, or allow it and I will.
