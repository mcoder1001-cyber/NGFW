# F-unbound-chrony-syslog: focused verify of fix round 1

- Verified: `task/F-unbound-chrony-syslog` @ 06ad0939 (fix round 1 on top of review b665904f). CI passed at 53840d3f;
  06ad0939 adds only docs to it (`git diff 53840d3f 06ad0939 -- ':!docs'` is empty).
- Reviewer: 2026-09-25, 45-minute time box. No host run, no VPP DNS call. VPP source read-only (`/root/vpp`, 26.06).
- Method: every "fails on the old code" claim was re-checked by mutation in a scratch copy (`git archive 06ad0939` into
  `/tmp/g-rv10/m`). Nothing in the worktree was edited. The worktree is clean after all runs.

## Verdict: APPROVE

All nine items are fixed, and every code fix has a test that fails on the old code. The one exception is the new
DEGRADED clause of the lookup guard: it has no test (V1). I checked its behaviour with a scratch test (below), so this
does not block the merge.

Three things ride on the rebase that the Q5 fold and TD-13 need anyway:

- V1: a test for the DEGRADED clause.
- V2: the V-item gets the ikev2 path and the corrected attribution.
- V3: the L3 decision.

One new crash path turned up in the VPP source (V2). It is in the ikev2 plugin, not in this branch, so it is a
hand-off to D-139, P11 and F-ikev2-native.

## Items

| item | verdict | evidence |
|---|---|---|
| **H1** IPv4 upstream required | **fixed** | **Zod:** `DnsVppCacheSchema.superRefine` (48a00bfc, `services.ts`) returns 400 at `/services/dns/vppCache/upstreams`; old `services.ts` in scratch: `× … needs at least one IPv4 upstream`, new: ✓. **Builder:** `desired/dns.go` raises error `services.dns-vpp-cache-upstream` at the same pointer and emits no `dns.*` object. Mutation M-b (old `dns.go`): `TestVPPCacheNeedsAnIPv4Upstream` FAIL. **Descriptor:** `dns.enable` returns `ErrNoIPv4Upstream` before sending anything, unless an IPv4 add was recorded on the running VPP. Mutation M-c (check removed): `TestIPv6OnlyUpstreamsAreNeverEnabled` FAIL ("IPv6-only cache applied") and `TestDNSLifecycle` FAIL. The crash model now keeps the v4 and v6 vectors apart, with crash = request while no IPv4 server was ever added, which matches `dns.c:576-624`. Nit: Zod treats `::ffff:a.b.c.d` as IPv6 while Go unmaps it. Zod is the stricter of the two, so this is harmless. |
| H1: dns.c re-check for any other path to the NULL dereference | **covered, plus one new path outside dns.api (V2)** | The dereference needs a first send while `ip4_name_servers` is NULL. **UDP-53 client queries:** IPv4 requests reach `vnet_dns_resolve_name` only while enabled (`request_node.c:160,234`). The descriptors never enable without a recorded IPv4 add, so this is safe. IPv6 requests are dropped as UNIMPLEMENTED (`request_node.c` `is_ip6` branch), which leaves no IPv6 client path. **`show dns servers`:** it crashes only with IPv6-only servers (`dns.c:2244-2246`). The fix makes that state unreachable through the agent, and the L7 guard bans the CLI string in the agent. **Retries and CNAME follow-ups** (`resolver_process.c:108-124,259,289`; `dns.c:1125`) run only after a first send succeeded. After the first IPv4 add, the vector stays allocated for the whole boot (`vec_delete` never frees), so they cannot reach NULL. **Static cache CLI and `show dns cache`** check `is_enabled` and send nothing. **New: the ikev2 plugin** calls the exported `dns_resolve_name` through `vlib_get_plugin_symbol("dns_plugin.so", "dns_resolve_name")` (`ikev2.c:5826-5827`) in `ikev2_resolve_responder_hostname` (`:4870-4897`). That runs on `ikev2_initiate_sa_init` for a profile with an unresolved responder hostname (`:4929-4933`), and on the pending SA-init retry (`:5544-5546`). With no IPv4 DNS server this is the same crash. main already has both halves: DF-5's `ikev2.responder-hostname` descriptor (`descriptors/ikev2/responder_hostname.go`, `ikev2_set_responder_hostname`) and `ikev2.InitiateSAInit` (`descriptors/ikev2/actions.go:16`). Today only a fake-VPP unit test calls `InitiateSAInit`, and the DF-5 host test sets a hostname but never initiates, so nothing sends it now. Hand-off in V2. |
| **H2** live readiness | **fixed** (DEGRADED clause untested, V1) | `dns.Readiness` (`descriptors/dns/readiness.go`): a fact is recorded only after VPP accepted an IPv4 `dns_name_server_add_del` (`serverAdded`, IPv6 ignored) and `dns_enable_disable(1)` (`setEnabled`). It is keyed by the `bootid` identity (kernel boot_id, VPP PID, start time; this host's real agent logs `complete=true`, with a PID). `Ready` re-reads the identity on every call, and an identity change, an unreadable identity, a disable or a server delete clears the fact. The lookup requires `GlobalsOwner && !DEGRADED && Readiness.Ready` (`rpc_dns.go`), and `appliedByThisAgent` uses the same fact. **Old code fails:** mutation M-a (old `rpc_dns.go`) makes `TestDNSLookupReadinessIsLiveNotStored` FAIL, because the old code sent `dns_resolve_name`. Mutation M-d (facts survive an identity change) makes `TestReadinessDoesNotSurviveAVPPRestart` FAIL ("ready on a restarted VPP before the resync"). **Mutation M-f (DEGRADED check removed): every test still passes, so this clause has no test (V1).** **Simulated here** with a reviewer scratch test, not committed, on the agent with the globals-owner wiring, `coretest` plus dns handlers and a switchable identity: (1) apply `vppCache{enabled, [192.0.2.53]}` → lookup OK, 1 resolve. (2) `setDegraded(true)` → 409, 0 new resolves. (3) VPP restart (new identity, empty plugin) before the resync → 409. (4) resync with a failing `dns_name_server_add_del` gives `APPLY_STATUS_ROLLED_BACK degraded=true` → 409. (5) good resync gives `APPLIED degraded=false` → lookup OK. Result: 0 crashes; the dns message sequence was add, enable, resolve, add (failed), add, enable, resolve. |
| **M1** V-item | **fixed, two corrections (V2)** | The trigger, the crashing paths, the safe messages, the non-crash defects, the upstream fix and the fallback all match `dns.c` and my review. Corrections: (a) "The call that fired it was a `dns_resolve_name` from this task's DF-8 host test" is wrong. It came from the topology run's `POST /api/v1/actions/dns-lookup` through the slot agent (the branch's own status and questions Q2 say so). (b) The ikev2 path is missing (above). Nit: "answers on every VPP address" means every IPv4 address, because IPv6 queries are dropped. |
| **M2** journal explorer admin-only | **fixed** | API: `@MinRole('admin')` on `GET /api/v1/state/logs`, plus an `ADMIN_ONLY` row in `route-guard.test.ts`. The worker reports the old controller fails it, which follows from the guard default of `readonly` for GET. The e2e test has readonly → 403 and operator → 403. Web: `LoggingTab` renders the explorer only for `perms.manageUsers` (`role ≥ admin`, `AuthProvider.tsx:64`). `LoggingTab.test.tsx` checks that readonly and operator see the notice and make **no** `/state/logs` call, and that admin gets the explorer. en and fa strings are present. Nit: `manageUsers` stands in for "is admin"; an `isAdmin` flag would read better. |
| **M3** one walk in flight per kind | **fixed** | `rpc_dns_walk.go`: one slot per kind (DNS, NTP, syslog state, journal). A second caller waits `walkWait` (3 s), then gets `UNAVAILABLE`, which `agentProblem` maps to **503** (`agent.client.ts:240`). Mutation M-a (old `rpc_dns.go`) fails `TestStateWalksAreSerialised` for dns; mutation M-e (journal slot removed) fails it for journal. The test loops over all four kinds. Nit: a busy walk shows as the problem type `agent-unavailable`; a dedicated "busy, retry" detail would be clearer in the UI. |
| **M5** exposure warning + ACL note | **fixed; claim confirmed in the source** | `desired/dns.go` warns `services.dns-vpp-cache-exposure` at `/services/dns/vppCache/enabled` and lists the answering IPv4 addresses. Mutation M-b makes `TestHostServicesProjection` fail. The user page tells users to block UDP 53 with an ACL. **VPP source:** `udp_register_dst_port` for 53 is global (`dns.c:85-97`) and never unregistered (no `udp_unregister_dst_port` in the plugin). The request node checks only `is_enabled` and filters on no `sw_if_index` or FIB (`request_node.c:155-240`). `dns.api` has no interface or client field. So there is no per-interface control, and an input ACL on the interface is the right tool. |
| **L1** | fixed | `Enable` doc comment no longer claims a dependency. |
| **L4** | fixed | The harness refuses a slot outside 1..12 (`harness_test.go:41-43`). |
| **L6** | fixed | `TestDNSLookupReadinessIsLiveNotStored`: slot agent and globals owner, stored document enabled, fresh fake VPP → FAILED_PRECONDITION, no output, no `dns_*` message. The positive server-level path has no committed test; my scratch test above covers it. |
| **L7** | fixed (scope limits in V2) | `resolveguard_test.go` scans every non-test Go file under `apps/agent` except `binapi`. Mutation M-g (a planted `dns.NewServiceClient(c).DNSResolveName` in `internal/agent`) fails `TestResolveMessagesOnlyThroughTheGuardedHelpers` at `zz_planted.go:10`. The scan does not cover `_test.go` files (host tests are where 04:27 came from), `test/` modules, or `Ikev2InitiateSaInit`. |
| **L9** | fixed | `TODO(PENDING-secret-channel)` on the projection-test exemption. |
| **Regen** | **byte-identical, description-only** | `3ef982b3 contract(api-client)` changes one line of `schema.d.ts`: the `GET /state/logs` summary gains "(admin only)". `53840d3f chore(cli)` changes the same summary in `operations_gen.go`. `TMPDIR=/tmp/g-rv10 tools/ci.sh gen-check` at 06ad0939 in the worktree: `clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated`, `gen-check PASSED (1m33s)`, and `git status` is empty afterwards. `48a00bfc contract(schema)` is the H1 refinement. It tightens a rule (an enabled IPv6-only cache now gets 400) rather than adding one. D-139 mandates it, and no example or fixture is affected: `services-dhcp-dns.json` has an enabled cache with one IPv4 upstream, and both proto fixtures have the cache disabled. |

## Test runs (06ad0939)

```
apps/agent $ TMPDIR=/tmp/g-rv10 go test -race -count=1 ./internal/renderers/{unbound,chrony,rsyslog}/ ./internal/descriptors/dns/ \
             ./internal/subsystems/ ./internal/agent/ ./internal/desired/ ./internal/actions/unbound-chrony-syslog/ ./internal/vpp/bootid/
ok  renderers/unbound 2.079s · renderers/chrony 1.667s · renderers/rsyslog 7.140s · descriptors/dns 7.952s · subsystems 6.838s
ok  agent 13.076s · desired 1.215s · actions/unbound-chrony-syslog 1.076s · vpp/bootid 1.074s
apps/agent $ env -u VRX_INTEGRATION TMPDIR=/tmp/g-rv10 go test -count=1 ./...        → no FAIL (every package ok / no test files)
packages/schema $ npx vitest run    Test Files 38 passed (38)   Tests 1237 passed (1237)
apps/api        $ npx vitest run    Test Files 11 passed (11)   Tests 102 passed (102)
apps/web        $ npx vitest run    Test Files 16 passed (16)   Tests 105 passed (105)   (incl. LoggingTab.test.tsx; workspace deps rebuilt first: gen-check had cleaned ui-kit/api-client dist)
```
Mutations, in the scratch copy, each restored afterwards:

| run | change | test that fails |
|---|---|---|
| M-a | old `rpc_dns.go` | `TestDNSLookupReadinessIsLiveNotStored`, `TestStateWalksAreSerialised` (dns) |
| M-b | old `desired/dns.go` | `TestVPPCacheNeedsAnIPv4Upstream`, `TestHostServicesProjection` |
| M-c | enable IPv4 check removed | `TestIPv6OnlyUpstreamsAreNeverEnabled`, `TestDNSLifecycle` |
| M-d | readiness keeps facts across an identity change | `TestReadinessDoesNotSurviveAVPPRestart` |
| M-e | journal walk slot removed | `TestStateWalksAreSerialised` (journal) |
| M-f | `!degraded` removed | **none** (V1) |
| M-g | planted `DNSResolveName` in `internal/agent` | `TestResolveMessagesOnlyThroughTheGuardedHelpers` |
| schema | old `services.ts` | `services.dns.vppCache (D-137) › an enabled cache needs at least one IPv4 upstream` |

No regression: the whole agent module, schema, API unit and web suites are green. The e2e (4/4) is the worker's slot
run; I did not re-run it (slot database).

## Open items: carry them on the rebase (doc/test only; the rebase verify checks them)

- **V1 (M):** add an agent test in which `setDegraded(true)` with a ready fact gives FAILED_PRECONDITION and zero
  `dns_*` messages, and a failed resync after a simulated restart does the same. My scratch test above does exactly
  this: `coretest` plus `v.On("dns_*")`, `hs.DNSReadiness.WithIdentity(...)`, `s.Resync`.
- **V2 (M):** in the V-item, fix the attribution (the topology run's `dns-lookup` action, not the DF-8 host test) and
  add the ikev2 path.
- **V2, manager:**
  - Add the ikev2 path to D-139 as an addendum: "`ikev2_initiate_sa_init` on a profile with a responder hostname
    resolves through the dns plugin and crashes VPP 26.06 without an IPv4 DNS server".
  - Tell P11 and F-ikev2-native (initiate actions, ActionRequest 8/11): initiate a hostname-responder profile only when
    `dns.Readiness.Ready`, or refuse hostname responders on a box without the VPP cache.
  - Extend the L7 scan to `Ikev2InitiateSaInit`, gated on whether the profile has a responder hostname, when that
    action lands.
  - Include `_test.go` files and `test/` modules in the scan, allowing DF-8's own tests.
- **V3 (L3, manager decision): I recommend a DryRun error** (fail closed) for `management.syslog[i].vrf` ≠ default.
  - Why: today the projection sets `t.Vrf = nil` and warns (`desired/syslog.go:42-48`). That silently turns an export
    the operator pinned to a management VRF into one routed by the host's default table. The export can carry
    auth/authpriv lines over plain UDP or TCP, possibly out through the data plane or WAN. RF-4's renderer refuses the
    same input (`rsyslog/model.go` `buildTarget`); the projection is the only place that bypasses that refusal.
  - Cost: one `s.Errorf` with a dedicated rule (e.g. `agent.syslog-vrf-unsupported`), one test, and an exemption for
    that rule in `TestProjectSchemaExamples` with a `TODO(F-logging)`, because `group-a-full.json` has `vrf: "mgmt"`.
  - Remove the error when F-logging implements per-VRF export.
- **Info (L):**
  - `serverAdded` reads the VPP identity after the add reply. A restart and reconnect between the reply and the next
    control_ping would bind the fact to the wrong instance. A dead connection makes the identity read fail, so this is
    not reachable in practice. Reading the identity before the add and comparing it afterwards would close it.
  - Same class: the gap between `Ready` and the resolve send.
- **Unchanged from the review:** L2, L5, L8, L10 and L11 stay open as listed there. M4 is handed to
  PENDING-agent-privileges / P10.
