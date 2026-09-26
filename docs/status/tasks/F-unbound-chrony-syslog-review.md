# F-unbound-chrony-syslog: review

- Reviewed: `task/F-unbound-chrony-syslog` @ c94e90d9 (main 4f472cc7 merged in at e117766f; CI passed at cf6a2635, and the
  four later commits change only docs/status files).
- Contract commits: 18801ce6 `contract(schema)` and 0e380d31 `contract(proto)`.
- Reviewer: 2026-09-25, read-only. No host rerun, no VPP DNS call, no daemon or unit touched. VPP source read in
  `/root/vpp/src/plugins/dns` (26.06, c3200b88d).

## Verdict: APPROVE WITH CHANGES

The branch is careful work. The singleton descriptors and the embedded-input Retrieve are sound. The slot/product
path split protects the host. The evidence is real and complete. The 04:27 guards work for every agent on the shared
host: no slot agent and no tools/app agent can send any dns.api message.

The guard is still incomplete for the globals owner, which is the product agent on a real box:

- **H1:** a VPP cache with only IPv6 upstreams crashes VPP 26.06 through the same `ip4_sas` NULL dereference. The
  schema, the agent and the lookup guard all accept that configuration.
- **H2:** `dns_lookup` decides "ready" from the stored document, not from VPP. After a VPP restart, or while a
  resync is failing, it can send the crashing call.

Both fixes are small. The V-item states the wrong crash condition (M1). The log explorer serves the whole host
journal to the readonly role (M2), and its RPCs are bounded but not serialised (M3).

Neither H item can fire on the shared host today, because no agent there is the globals owner. So this is not a
BLOCK. It is a must-fix round before the merge, followed by a focused re-verify of H1, H2 and M1–M3. The branch
still merges after TD-13 (D-125; D-136 released only F-kea and F-host-acl from that gate), so the fix round costs no
merge slot.

## What I ran

```
$ cd apps/agent && TMPDIR=/tmp/g-rv10 go test -race -count=1 ./internal/renderers/{unbound,chrony,rsyslog}/ \
    ./internal/descriptors/dns/ ./internal/subsystems/ ./internal/agent/ ./internal/desired/ ./internal/actions/unbound-chrony-syslog/
ok  	ngfw/agent/internal/renderers/unbound	1.890s
ok  	ngfw/agent/internal/renderers/chrony	1.695s
ok  	ngfw/agent/internal/renderers/rsyslog	7.931s
ok  	ngfw/agent/internal/descriptors/dns	1.090s
ok  	ngfw/agent/internal/subsystems	6.745s
ok  	ngfw/agent/internal/agent	13.290s
ok  	ngfw/agent/internal/desired	1.200s
ok  	ngfw/agent/internal/actions/unbound-chrony-syslog	1.130s
```
The first run used the long scratchpad TMPDIR. Seven `internal/agent` tests failed with `bind: invalid argument`,
because the unix socket path went over 108 characters. That is an environment problem, not the code; with a short
TMPDIR everything passed.
The web suite needs the workspace packages built: the worker deleted `dist/` after CI, so
`pnpm turbo run build --filter='@ngfw/web^...' --filter='@ngfw/api^...'` ran first.
```
apps/web  $ npx vitest run                  Test Files  15 passed (15)   Tests  102 passed (102)
apps/api  $ npx vitest run   (unit)         Test Files  11 passed (11)   Tests  102 passed (102)
packages/schema $ npx vitest run src/semantic/unbound-chrony-syslog.test.ts   Tests  21 passed (21)
```
The API has no unit test file of its own for this module; it is covered by the e2e test, which I did not run
because it needs the slot database. `git merge-tree` checks against F-kea-dhcp-relay, F-wireguard, P12,
F-vrf-static-ecmp and TD-23 are in §5.

## 1. The 04:27 incident (D-137)

| check | result |
|---|---|
| `dns_lookup` answers 409 unless this agent is the globals owner and enabled the cache with an upstream | **partly.** Non-owners are refused before any VPP call (`rpc_dns.go:95` → `lookup.go:49`, 409 in the host run). For the owner, "enabled" means "the stored document says enabled" (**H2**) and "an upstream" includes IPv6-only upstreams (**H1**). |
| DF-8 `ResolveName`/`ResolveIP` take `Ready` and send nothing without it | yes (`dns.go:269-275`, `293-296`; `TestResolveHelpersRefuseWithoutReady`). Callers of `Ready` state it themselves, so the flag is only as good as the caller's proof (H2). Across every worktree, only DF-8's own package and this action use the dns binapi. |
| a name-server delete disables the switch first | yes (`dns.go:231-236`, owner only). `Enable.Upstreams` turns a changed set into an update that re-enables the switch after the creates. `TestUpstreamChangesNeverLeaveAnEnabledResolverWithoutServers` drives the real scheduler through a replacement of the last server, and I re-ran it with `-race`. Disable-first also clears pending cache entries (`dns_cache_clear`), so no retry timer later sends to a stale server. |
| DF-8 host test opt-in (`VRX_DNS_VPP_HOST=1`) | yes (`integration_test.go:27-29`), and also `VRX_DF8_GLOBALS=1` and the exclusive globals lock. Nothing in `tools/`, `deploy/` or `test/` sets either variable. Until this merges, main and every other worktree still carry the old DF-8 test, gated only by `VRX_DF8_GLOBALS=1`: the manager should not open a DF-8 window before the merge. |
| the V-item is correct | **no (M1).** The crash condition is misstated, and IPv6-only servers and the CLI crash are missing. |
| any other dns.api message that crashes the same way | **yes: `dns_resolve_ip`** (`dns.c:1515` → the same `vnet_dns_resolve_name`; already guarded by `Ready`). **`dns_name_server_add_del` does not crash** in any state: it only does vector operations (`dns.c:131-217`). **`dns_enable_disable` does not crash**: disabling a never-enabled plugin returns early in `dns_cache_clear` (`dns.c:49`), and enabling without servers is refused with `NO_NAME_SERVERS` (`dns.c:79-81`). The same NULL dereference is also reachable by: an IPv4 UDP-53 request to any VPP address while enabled (`request_node.c:160,234`, which checks only `is_enabled`); **IPv6-only name servers** (H1); and the CLI `show dns servers` with IPv6-only servers (`dns.c:2244-2246` formats `ip4_name_servers + i`). |
| can the product agent or a slot agent still send it | **Slot agents and tools/app (`VRX_GLOBALS_OWNER=0`, `tools/app:108`): no.** The lookup is refused, and the dns.* descriptors run in require mode (`subsystems/dns.go:18-20`; `Require(nil)` returns `ErrNotGlobalsOwner` and sends nothing, `dfkit/globals.go:37-39`; `Delete` is a no-op, `dns.go:139-142,196-199`). **The globals owner: yes**, through H1 (deterministic, reachable from the data plane) and H2 (a lookup after a VPP restart, or while DEGRADED). An agent started on the dev host with the default owner `vrx` and no `VRX_GLOBALS_OWNER=0` would be the globals owner (`agent.go:73-79`). That is the same exposure D-071 accepted; see Q8. |

The exact crash condition, read from the source. `vnet_send_dns_request` (`dns.c:576-624`) starts every cache entry
with `server_af = 0`. When there is no IPv4 server, it falls through to
`vnet_dns_send_dns4_request(dm->ip4_name_servers + rotor)` (`dns.c:621-623`). When no IPv4 server has been added
since VPP started, that vector is NULL, and `ip4_sas` dereferences it; that is the 04:27 backtrace.

A vector that deletes have emptied is still allocated. It does not crash, but it sends to stale memory, which is a
deleted server's address. `is_enabled` plays no part on the API path: `vnet_dns_resolve_name` (`dns.c:780`) never
checks it.

Three more defects that do not crash:

- IPv6 upstreams never work: `vnet_dns_send_dns6_request` builds the frame (`dns.c:351`) but never calls
  `vlib_put_frame_to_node`.
- After the first enable, UDP 53 stays registered to the dns nodes for good (`dns.c:84-97`). After a disable, port-53
  packets to VPP addresses are punted.
- A lookup while the plugin is disabled but has servers sends a real query. The reply is then punted
  (`reply_node.c:148`), so the caller waits until its deadline.

## 2. Architecture

- **Embedded-input Retrieve (F-kea pattern): I ratify it as a real Retrieve.**
  - What it does: `unbound/descriptor.go:134-157`, `chrony/descriptor.go:127-161` and
    `rsyslog/descriptor.go:128-161` read the file the daemon loads and decode `# vrx-input:` (base64, deterministic
    protobuf). They re-render it and byte-compare every file in the set.
  - How drift shows: a hand edit, a deleted file or a renderer change gives a drift Value (`structpb`), which never
    equals a desired Value, so the reconciler re-applies. The host run proved the deletion case: files deleted while
    the agent was down were re-rendered 0.25 s after it started.
  - Why it is not an echo: the Value comes from disk, not from memory, which satisfies D-063. A file without an
    input line (Debian's `unbound.conf`) is never reported, so a foreign file is never touched until resolvers
    exist.
  - Its limit: it observes the file layer, not what the daemon has loaded. `Pending` closes that gap. It reports
    persisted restart requests (D-079) and a "start" when an active configuration is written but the daemon is not
    running, and the state RPCs (`list_local_data`) serve as proof.
- **Start/restart requests persisted and shown (D-079): yes.** unbound and chrony use `PendingFile` and clear it when
  the daemon's start time is later than the request. The slot rsyslog uses `DeferredController`
  (`rsyslog/deferred.go`), which verifies the PID's executable so that a recycled PID never counts. The product
  rsyslog restarts itself (RF-4; M2 of that review holds, with no restart when nothing changed). In the host run,
  the request was still pending after the agent restarted, and was cleared after unbound restarted. **Gap for the
  product (M4):** nothing ever acts on an unbound or chrony request on a real box.
- **Q8, product paths only for the globals owner: I ratify it** (`subsystems/unbound.go:99-147`, `chrony.go:18-52`,
  `rsyslog.go:16-35`). Conditions are under Q8 below.
- **dns.* on non-owners: require mode, correct** (UCS-11). A document that enables `vppCache` fails loudly, but only
  at Apply; see L2 for moving that to DryRun.
- **Rules from 00-CONTEXT all hold:**
  - Node never reaches VPP: the API calls only the agent gRPC.
  - VPP names come only from binapi.
  - One schema: the web types come from the OpenAPI document and `@ngfw/schema`.
  - GPL daemons run as separate processes.
  - No user input reaches a shell: journalctl gets a fixed argv, and the renderers go through the allowlist.
  - Secrets never reach the agent.
- **Restart-safety with `management` implemented:** the persisted `management` subtree carries no secret. The proto
  has `password_hash` reserved, and the AAA and TLS fields are references. The resync after the agent restart covered
  `[interfaces vrfs routing services management]`.
- **TD-13 adoption at the rebase.** On all three singleton descriptors, add `Stage() scheduler.Stage { return
  scheduler.StageDaemon }` and a `Validate` that renders and then calls the renderer's staged checker, never the
  `prepare` hook:
  - unbound: `unbound-checkconf <staged unbound.conf>`
  - chrony: `chronyd -p -f <staged chrony.conf>`, with the staged `sources.d`
  - rsyslog: `rsyslogd -N1 -f <staged rsyslog.conf>`. On the product this is the host's `rsyslog.conf` plus the
    staged include. TLS also needs the `lmnsd_ossl.so` presence check.
  - DF-8 `dns.*`: no daemon checker; it stays in StageVPP. The IPv4-upstream rule from H1 is a projection error. On
    non-owners, a Validator on `dns.enable` should return `InvalidAt(/services/dns/vppCache/enabled)` (L2).

## 3. Security

- **Secret references are refused at DryRun:**
  - `services.ntp.servers[].keyRef` (`desired/ntp.go:41-47`) and `management.syslog[].tls` (`desired/syslog.go:37-41`),
    under rule `agent.secret-channel-pending`.
  - The syslog TLS key waits for PENDING-secret-channel, option 1 once decided.
- **Hostile input, no newline path found:**
  - The only new template input is the base64 input line. The `b64` helpers reject anything else
    (`unbound/input.go:63-68`, `rsyslog/renderer.go:183-188`, chrony `b64`).
  - `facilities`, `format` and `queueSize` are enums or bounds in Zod (`ext/syslog.ts`) and in Go
    (`rsyslog/model.go`: the facility list, the rfc5424/rfc3164 enum, 100..1000000).
  - `permittedPeers` are checked with `hostnameRe`.
  - The RF-3 and RF-4 escaping is unchanged. `renderers.Line` rejects control characters; `rrQuote` rejects quotes,
    stray backslashes and `include:`; `sockaddr` and `fwdaddr` re-parse the address.
- **Journald explorer:**
  - The argv is fixed (`journal.go:146-156`) and runs through its own allow-listed runner (ALLOWLIST row), with no
    shell.
  - Severity and facility are enums; `since` is numeric and clamped to 30 days.
  - The free text is at most 128 characters, contains no control characters, and is matched inside the agent, never
    passed to journalctl.
  - Paging bounds: page ≤ 5000, pageSize ≤ 500, 5000 entries scanned, 16 MiB of output, 20 s.
  - **Other units' entries: yes, all of them (M2).** The explorer reads the whole host journal as root, including
    auth and authpriv (sudo, sshd, PAM; the committed screenshot shows `pam_unix` lines). The route
    (`controller.ts:68`) has no `@MinRole`, so the GET default `readonly` applies (`auth/auth.guard.ts:11-12`), and
    reads are not audited.
- **File modes:**

  | file | mode |
  |---|---|
  | `unbound.conf` | 0640 root:unbound |
  | chrony conf and sources | 0640 root:_chrony |
  | `chrony.keys` | 0600 _chrony |
  | rsyslog export | 0644 |
  | TLS keys | 0640 |

  The paths validate the modes. The rsyslog export at 0644 is fine: it holds only references, and the embedded input
  holds the same references.
- **VPP cache on WAN (M5):** enabling it opens a resolver on every VPP address, WAN included (see M5).

## 4. D-132

- **Bounded: yes.**
  - unbound-control and chronyc run with timeouts and the `DefaultMaxOutput` cap (`LocalDataTruncated`).
  - The journal scan has the bounds listed in §3.
- **Polls and Refresh: yes.**
  - Polls: `STATE_POLL_MS = 30_000` (`queries.ts:35`, used at 49, 58 and 67).
  - Refresh buttons: every panel (`common.tsx:57`) and the explorer (`LoggingTab.tsx:313`).
  - The explorer does not poll.
- **Serialised: no (M3).** There is no in-flight limit in the agent or the API.

## 5. Contract

- **Additive, and the numbers match the allocation:**
  - wave-A-hotspots §2 (lines 83-84) allocates `SyslogTarget` 6–9 and `ActionRequest` 7 `dns_lookup`, and the
    commits use exactly those.
  - The RPCs take no numbers.
  - The new messages sit in the `// ----- F-unbound-chrony-syslog -----` section.
  - The schema keys are all optional with no default, and a test checks that pre-D-086 documents parse unchanged.
  - The drift fixture is `unbound-chrony-syslog-full.json`.
- **No collision with the other branches.** Each adds only its own RPCs and messages:

  | branch | its additions |
  |---|---|
  | F-kea | `DhcpLeases` and its messages |
  | F-wireguard | `WireguardState`, EventKind 13, WireguardInterface 12 |
  | P12 | `ListRoutes`/`RoutingState`, EventKind 14–15, Interface 22, StaticRoute 7–8, NextHop 4 |
  | F-vrf-static-ecmp | `ListRoutes` (P12 carries the same) |

- **`git merge-tree` results:**
  - `dataplane.proto` merges cleanly with all of them.
  - Conflicts appear only in generated files (regenerate them) and in `service_test.go` against F-kea and
    F-wireguard (identical code, so keep main's).
  - `docs/vpp-code-track.md` conflicts against F-wireguard and F-vrf-static-ecmp (see L8).
  - `subsystems.go` and `desired` merge textually but will not compile (duplicate const and map key) until the Q5
    fold below. That failure is loud, which is good.
- Q5, Q6 and Q7 are under "Questions" below.

## 6. Tests and evidence

- The unit coverage is good:
  - the fake-VPP crash model driven through the real scheduler
  - restart-pending lifecycles
  - the deferred controller refusing a foreign PID
  - projection refusals and round trips
  - the journal argv and paging
  - the e2e test with problem+json pointers
- The coverage is missing:
  - a test of the IPv6-only case (H1)
  - a server-level test of how `dnsLookup` derives readiness (L6)
- **The pasted evidence is credible:** it is internally consistent, carries real PIDs, ports and timestamps, and
  matches the code.

  | claim | pasted evidence |
  |---|---|
  | resolver answers a local record | `net.Resolver{127.10.0.53:4053}` → 10.10.53.1 |
  | chrony selects the slot server | `^*` on `127.0.0.1` via the slot socket (port 4023) |
  | a logger line reaches the collector | octet-counted `<189>` (local7.notice), and a `daemon.info` line was filtered out, which proves the facility filter |
  | re-render after the agent restart | 0.25 s, with the pending restart surviving |
  | rollback | `list_local_data` before and after, NXDOMAIN, and `/state/drift` empty |
  | the 400 pointer | `/services/dns/resolvers/lab/forwarders/0` |
  | host units unchanged | same MainPID and state before and after; NRestarts 2 → 2 |
- **One caveat:** the run (04:50) predates `96c02fd4` (Ready) and the main merge `e117766f` (the TD-11b guard). Unit
  tests and CI cover both. Re-run `test/topology/unbound-chrony-syslog` on the fix-round SHA once host runs resume
  after TD-25. That test sends no VPP DNS call: it asserts the 409.

## Findings

| id | sev | where | finding | fix |
|---|---|---|---|---|
| H1 | **H** | `desired/dns.go:51-76`; `rpc_dns.go:95`; `packages/schema/src/domains/services.ts:514`; `descriptors/dns/crashguard_test.go:52` | A VPP cache with **only IPv6 upstreams** is accepted by the schema (`ipAddress`), the projection and the lookup guard. VPP 26.06 then dereferences `ip4_name_servers` (NULL) on every resolve: `dns.c:576-624` falls through to the IPv4 send because `server_af` starts at 0. That covers the API lookup and **any IPv4 UDP-53 query from a client to a VPP address**, the same crash as 04:27, with no user action needed. IPv6 upstreams also never work, because `dns.c:351` never dispatches the frame. The crash model decodes every server as IPv4 (it ignores `IsIP6`), so it cannot see this. | Until the V-item fix, accept only IPv4 upstreams, or at least require one. (1) Add a projection error (`services.dns-vpp-cache-upstream`, pointer `/services/dns/vppCache/upstreams/<i>`). (2) Add a Zod semantic rule with the same pointer so the API returns 400 first. (3) Make `ready` require an IPv4 upstream. (4) Give the model separate v4 and v6 vectors, with crash = resolve while no IPv4 server was ever added, plus a test that an IPv6-only document is refused and sends nothing. |
| H2 | **H** | `agent/rpc_dns.go:84-97`, `33-43`; contract at `descriptors/dns/dns.go:254-257` | Readiness is `GlobalsOwner && stored desired enabled && upstreams > 0`. The stored document (`st.desired`) survives a VPP crash or restart and a failed, DEGRADED resync (retried hourly). So on the globals owner, a lookup made before the resync re-creates the servers, or while it keeps failing, sends `dns_resolve_name` to a VPP whose IPv4 vector is NULL: the 04:27 crash again. `Ready`'s own contract ("both calls succeeded on this VPP") is not what the caller proves. | Make readiness a DF-8 fact. `EnableDescriptor` records a boot- or connection-scoped flag, set only after `dns_enable_disable(1)` succeeded with at least one IPv4 server added in the current VPP connection (boot identity, as DF-2 does), and cleared on disable, Delete, any server delete and VPP disconnect. `dnsLookup` then uses `hs.GlobalsOwner && enable.Ready()`, and also refuses while the agent is DEGRADED. Test: stored document enabled plus a fresh fake VPP (reconnect) gives FAILED_PRECONDITION with zero messages sent. After the fix, `DnsVppCacheState.appliedByThisAgent` should come from the same fact (`rpc_dns.go:76`). |
| M1 | M | `docs/vpp-code-track.md:36-51` | The V-item's condition is wrong ("does not check `is_enabled` / `vec_len`"). The trigger is "no IPv4 name server ever added since VPP start", and `is_enabled` does not matter on the API path. It is also incomplete. It misses: IPv6-only servers (H1); `vppctl show dns servers` with IPv6-only servers (`dns.c:2244-2246`); the IPv6 send that is never dispatched; UDP 53 registered for good (`dns.c:84-97`, so the DF-8 host test cannot "restore the previous value", D-082); and a disabled-plugin lookup that sends a real query and hangs until its deadline. It also does not say which messages are safe. | Rewrite it with the §1 analysis. Upstream fix: choose the address family by which vectors are non-empty, return NO_NAME_SERVERS when both are empty, put the IPv6 frame, fix the `show` index, and unregister the ports on disable. Make it a numbered row (next free V-number at merge). |
| M2 | M | `apps/api/src/features/unbound-chrony-syslog/controller.ts:68`; `actions/unbound-chrony-syslog/journal.go:146-156` | The log explorer serves the whole host journal (every unit, auth and authpriv, sudo, sshd, PAM, the agent, API and PostgreSQL logs) to the **readonly** role. Reads are not audited. | Put `@MinRole('admin')` on `GET /state/logs`, or `operator` if the manager prefers, and say so in the user page. Alternatively, keep readonly but drop auth and authpriv for non-admins, filtered in the agent. Manager decision; default admin. |
| M3 | M | `agent/rpc_logs.go:64-78`; `rpc_dns.go`, `rpc_ntp.go`, `rpc_logs.go:32-61` | The state RPCs are bounded but **not serialised**. Every explorer request starts its own journalctl (up to 20 s, 16 MiB captured and parsed in memory), so N users or tabs means N processes. Every state poll starts its own unbound-control or chronyc runs. | Allow one journal scan in flight per agent: a semaphore acquired with ctx, or RESOURCE_EXHAUSTED mapped to 429 in the API. Use `singleflight` for DnsState, NtpState and SyslogState. One unit test each. |
| M4 | M (product; not a merge blocker) | `renderers/unbound/descriptor.go:117-122`, `chrony/descriptor.go:111-121`; `renderers/ALLOWLIST.md` (no `systemctl` row for unbound or chrony) | On a real box, nothing acts on an unbound or chrony start/restart request. unbound.service ships disabled, so configured resolvers stay "start pending" forever. A listen change keeps the old sockets. The rsyslog product path restarts itself, so the three daemons are inconsistent. D-079 says "pending until acted on", but no actor exists. | Manager decision, recorded against P10 or tech-debt: (a) the globals owner runs allow-listed `systemctl start\|restart unbound\|chrony` with RF-3's convergence check; (b) an operator action "apply pending restart" (audited); (c) a P10 path unit. I recommend (a) for the owner only, because it matches rsyslog. |
| M5 | M | `docs/user/services/unbound-chrony-syslog.md:55-59`; `desired/dns.go:45-46` | Enabling `vppCache` makes VPP answer UDP 53 on **every** VPP address in every FIB, WAN included (`udp_register_dst_port`, global). The plugin has no client ACL, so it is an open resolver and an amplification risk. Neither the docs nor the DryRun say so. | Add a user-page warning and a DryRun warning note on `/services/dns/vppCache/enabled` saying that the resolver answers on all VPP addresses and needs an ACL for UDP 53 on untrusted interfaces. |
| L1 | L | `descriptors/dns/dns.go:42-44` | The `Enable` doc comment says the upstreams "make dns.enable depend on" the servers, but `Dependencies()` returns nil (`:103`). | Fix the comment. |
| L2 | L | `subsystems/dns.go:18-20`; `desired/dns.go:45` | A non-owner's `vppCache.enabled` fails only at Apply (`ErrNotGlobalsOwner`), not at DryRun. | At the TD-13 rebase, give `dns.enable` a Validator that returns `InvalidAt(/services/dns/vppCache/enabled)` on non-owners. |
| L3 | L | `desired/syslog.go:42-48` | A syslog `vrf` other than default is stripped with only a warning, so logs leave through the host's default route. RF-4 refused this case. | Fail closed (DryRun error) or keep UCS-6. Manager decision; I lean towards the error. |
| L4 | L | `test/topology/unbound-chrony-syslog/harness_test.go:36` | `w0`/`w00` pass the prefix regex, which gives ports 3000, 5000 and 9101: the product stack's. | Refuse N outside 1..12. |
| L5 | L | `LoggingTab.tsx:393` | Every page recomputes `since` and rescans, so rows shift between pages. | Pin `since`, and add `--until`, when the filters change. |
| L6 | L | `agent/rpc_dns_test.go` | No server-level test covers how `dnsLookup` derives readiness (owner false, stale store, IPv6-only). | Add it with H1 and H2. |
| L7 | L | new test in `descriptors/dns` | Nothing stops a later feature from calling `DNSResolveName`/`DNSResolveIP` through binapi directly and bypassing `Ready`. | Add a source-scan test, D-128 style, that allows those identifiers only in `descriptors/dns/dns.go` (and `_test.go`). |
| L8 | L | `docs/vpp-code-track.md` | The item is titled "V-new", under the table rather than in it. F-wireguard and F-vrf-static-ecmp edit the same file (merge-tree conflict). | The merger numbers it as a table row. |
| L9 | L | `agent/projection_test.go:44` | Blanket exemption for `agent.secret-channel-pending` on valid schema examples. | Add a TODO naming PENDING-secret-channel, and remove the exemption when the channel lands. |
| L10 | L | status evidence | The host run predates `96c02fd4` and the main merge. | Re-run the topology test on the fix-round SHA after TD-25. It sends no VPP DNS call. |
| L11 | L | `/run/vrx-test/w10/ucs/` | Left over from the run: 0700 root, with a test JWT/secret file (the worker's rm was refused). Also left by this review: gitignored `dist/` in `apps/api` and `packages/{schema,proto,api-client,ui-kit}`, and `/tmp/g-rv10`, because my `rm -rf` was refused too. | The manager deletes them. |

## Questions Q2–Q8: recommendations

- **Q1 (information).** Acknowledged.
- **Q2 (incident).** The four fixes are the right first step; add H1, H2 and M1. Proposed D-137 text for the LOG:

  > "`dns_resolve_name`/`dns_resolve_ip` — and enabling the dns plugin at all — are allowed only on the globals
  > owner, after it added at least one **IPv4** name server and `dns_enable_disable(1)` succeeded **in the current VPP
  > boot**. Readiness is a DF-8 boot-scoped fact, never derived from the stored document. IPv6-only upstreams are
  > refused until the V-item lands. A server delete disables first. DF-8 host tests need `VRX_DNS_VPP_HOST=1` and
  > `VRX_DF8_GLOBALS=1`, in a manager window only."

  The slot owners were told in the 09:25 status. No further action.
- **Q3 (log explorer source).** Ratify (a), journald through a fixed argv. Reject (b), omfile in RF-4, which breaks
  RF-4's template rule for no gain. Apply M2 (role) and M3 (serialisation) in the fix round.
- **Q4 (VPP cache state).** Ratify "configured": `live: false`, the "as committed" alert, and the
  `agent.unsupported-field` note so that drift never compares a write-only leaf. After H2,
  `appliedByThisAgent` must mean "applied in this VPP boot", taken from the DF-8 fact, not `enabled && owner`.
- **Q5 (the F-kea fold).** F-kea is approved and lands first. Fold at the rebase as follows:
  1. Delete `desired/dns_services.go:17-22`, both declarations. Keep F-kea's `ServicesImplemented` (`kea.go:26`) and
     `ServicesUnsupported`.
  2. **Keep** this branch's generic `unsupported()` helper (`dns_services.go:25-…`): move it into `desired/syslog.go`.
     `ManagementUnsupported` needs it, and unlike F-kea's loop it skips list and map fields. F-kea's loop calls
     `.Message()` on every message-kind field and would panic on a repeated field such as `management.users`. Make
     F-kea's `ServicesUnsupported` body `unsupported(s, "services", svc, ServicesImplemented)`.
  3. Remove this branch's `ServicesUnsupported(s, ds.GetServices())` call from `HostServices` (`desired/syslog.go:76`):
     F-kea's DHCP builder already makes it, under the same `in["services"]` gate.
  4. In `subsystems.go`, drop this branch's `Services = "services"` (`:64`) and keep `Management`. Change the entry to
     `Services: append([]string{kea.NameDhcp4, kea.NameDhcp6, dhcp.NameProxy, dhcp.NameProxyVSS, dhcp.NameRelay}, servicesDescriptors...)`
     and keep `Management: managementDescriptors`.
  5. Keep one `'services'` in `nav.ts:65` and `nav.test.ts:63`.
  6. `tabs.ts`, `i18n.ts`, `projection.go` (F-kea's DHCP call first, then `HostServices`), `app.module.ts`,
     `agent.client.ts` and `ALLOWLIST.md` auto-merge. Keep both.
  7. `service_test.go`: identical code, so take F-kea's, including its `"services": {"dhcp": {}}` canonical document.
  8. Regenerate the generated files.
- **Q6 (fake-agent `action`).** It is obsolete once TD-23 (`registerActionHandler`) lands, and no chaining with
  F-vrf-static-ecmp is needed. The changes:
  1. Remove `action` from `features/unbound-chrony-syslog/fake.ts:308`
     (`return { dnsState, ntpState, syslogState, syslogEntries }`).
  2. Remove `Partial<Pick<DataplaneServer, 'action'>>` from its return type (`:79`).
  3. Register the lookup **once, at module load**, in `fake.ts`:
     `registerActionHandler('dnsLookup', (call) => dnsLookup(ucsHost, call))`. `unboundChronySyslogFake(host)` sets
     the module-level `ucsHost = host`.
  4. The anchor line in `fake-agent.ts:665` stays: the spread of the four state RPCs.

  Do not register inside `unboundChronySyslogFake(this)`. `impl()` runs once per FakeAgent, and TD-23 throws on a
  second registration. Suggestion for TD-23's owner: pass the FakeAgent to handlers (`handler(call, agent)`) so that
  no feature needs a module-level host.
- **Q7 (edits outside the anchors).** I accept all four:
  1. `server.go`, `_` → `stream`: required for any action case, and the same one-token change the other action
     branches make.
  2. `service_test.go`, registry-derived: D-129 F5; identical to F-kea's.
  3. `projection_test.go`: acceptable while PENDING-secret-channel is open (L9).
  4. `management.ts`, one import and one spread: wave-BC-numbers SY4 assigns the `SyslogTargetSchema` line to this
     task.
- **Q8 (whose daemons).** **I ratify (a): the globals owner is the box owner, for the host daemons too.**
  - Why it is safe: the default owner `vrx` is the owner (`agent.go:73-79`), so P10's unit needs no new setting.
    tools/app sets `VRX_GLOBALS_OWNER=0` (`tools/app:108`, D-107/D-136) and so never touches the host's rsyslog,
    chrony or `/etc/unbound`.
  - Record it in the LOG as an extension of D-071: "host daemons (unbound, chrony, rsyslog, and every later renderer
    of a box-wide daemon) follow the globals-owner role; non-owners render into `/run/vrx-test/<owner>` and never
    start, restart or signal a daemon".
  - Add one line to shared-host-rules §3: never start an agent on the dev host without `VRX_GLOBALS_OWNER=0` or a
    slot owner.
  - The product-side executor for pending requests is M4.

## Must fix before the merge (focused re-verify)

1. H1, H2, M1, M2 and M3, with the tests from L6 and preferably L7.
2. The comment fix L1, the harness fix L4, and the documentation M5.
3. At the rebase, after TD-13 and F-kea: TD-13 adoption (§2), the Q5 fold and the Q6 change.
4. The manager decides M4, L3 and the M2 role, and deletes L11.
