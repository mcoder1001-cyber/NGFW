# F-kea-dhcp-relay — focused verify of fix round 1

Reviewer: the author of `F-kea-dhcp-relay-review.md`. Date: 2026-09-25.
Scope: `task/F-kea-dhcp-relay` @ `f6c41176`, 6 fix commits since the review commit `bfedc2b8`, 16 files. Only the
review's findings are checked. There were no host runs: slot 2 is being cleaned and af_packet creates are blocked until
TD-25.

## Verdict: **APPROVE**

- **Findings:** M1, M2, M3, Q7, L1, L3 and L7 are fixed. Each new test **fails on a mutant of the fix**, as shown below.
- **tools/app:** the change is exactly the one `VRX_KEA_MODE=off` token on the agent line.
- **Regressions:** none. One web test failed once while the host load was 48; the fix round does not touch it, and it
  passes on a re-run (details under "Tests").
- **New findings:** V1–V3 are all L and can wait (owners below).
- **Merge:** main merges cleanly. Two merge notes (N1, N2) for the merger.

## Findings

| # | status | evidence (file:line @ f6c41176) |
|---|---|---|
| **M1** | **fixed** | `renderers/kea/status.go:59`: a configuration without an embedded render input returns only `Running` (no file, idle, or foreign such as the packaged commented `/etc/kea`). The UI shows a neutral "Not configured" chip (`ConfigTabs.tsx:240-242`, en+fa keys). `TestStatusForeignConfigNotConfigured` covers the commented file while stopped, foreign JSON while running, and no file. **Mutant** (the check reduced to `cfg == nil`): FAIL `stopped daemon, packaged file: {… Active:true ActionRequired:start … Err:kea: configuration: invalid character '/' …}`, the exact symptom in the review. `DhcpPage.test.tsx` asserts "Not configured" and no "Stopped". |
| **M2** | **fixed** | `status.go:223-257` `cachedLeases`: one read per family in flight (singleflight), reused for `LeaseCacheTTL` = 10 s; errors are not cached. The read is detached from the caller with a 60 s bound; a cancelled caller returns `ctx.Err()`. `renderer.go:310`: `Apply` drops the cache. Paging is unchanged (`Leases`, `LeasePageSize` = 1000 per message, `MaxLeases`). `TestLeaseReadsSharedAndCached` checks four cases: 8 concurrent calls send 3 page requests (one read of 2500 leases); calls within the TTL send none; after the TTL one read; after Apply one read. **Mutant** (`LeasePage` calling `r.Leases` directly): FAIL `8 concurrent calls made 24 lease4-get-page requests, want 3`. Stress: `-race -count=20` over both new kea tests passes. |
| **M3** | **fixed** | `subsystems/kea_test.go:36-97` `TestRegisterKeaRelayScope`: zero `IDScope` (tables 5000 and 0), slot range inside and outside, and `IDs.All`. It checks Retrieve, a refused Create (nothing written to the fake VPP) and relay records. **Mutant** (`if err != nil { rng = nil }` at `kea.go:77`): FAIL `no_range:_fail_closed: retrieved 1 relays of table 5000, owns=false` and `table_0_too`, which is the exact refactor the review warned about. `TestRegisterKeaModes` (`:101-167`) covers the mode, IFMAP and netns table; `TestKeaTestModeMapper` covers the map scope and no address binding. |
| **Q7** | **fixed** | `subsystems/kea.go:90-95`: `VRX_KEA_MODE=test` refuses to start for owner `vrx` or `IDs.All`; three test cases. `ALLOWLIST.md:70` has a new row with a fixed argv and no shell. The product runner stays `kea.Binaries()`, which RF-3's `TestProductAllowlistHasNoTrampoline` covers. The optional WARN log was not added; the start-up log line "kea wired … mode test (netns …)" already names the mode. Accepted. |
| tools/app | **ok** | `tools/app:108`: only `VRX_KEA_MODE=off` was added (1+/1−). The product stack's stored desired state is empty (`/var/lib/vrx-app/agent/desired.pb`, 0 bytes, read-only check), so nothing changes at start-up. D-136 is **not yet in `docs/decisions/LOG.md` on main @ 56200c3e**; the manager should record it. For the effect of `off` on a document with servers, see V3. |
| **L1** | **fixed** | `3202aff2 contract(schema)` changes 1 file, 3+/2−, all comment lines (checked: no non-comment line changed). `turbo run test` ran `gen` and the worktree stayed clean, so the generated outputs are byte-identical. |
| **L3** | **fixed** | `descriptors/dhcp/relay.go:148-171`: write, fsync, close, rename, then fsync of the directory. `:130-134`: a corrupt file is treated as empty. `TestFileRelayStoreCorruptFile` covers Load, Retrieve returning 0 without error, and Put rewriting the file. Nit: the corrupt file is dropped without a log line. The review asked for one; this can wait. |
| **L7** | **fixed** | `docs/user/services/kea-dhcp-relay.md:121-124`: a known-issue line for sub-interfaces, af_packet and loopbacks. "Physical NIC not affected" is correct on main (untagged NICs are skipped) and on TD-11c (a lease is never claimed). |

## Deferred review items: all can safely wait

| # | why it can wait | owner |
|---|---|---|
| L2: DHCPv4 option data ≤ 255 in Zod vs Go | Fail-safe: the renderer refuses at apply and the transaction rolls back. Nothing reaches Kea; only the 400 pointer is missing. | TD-22 (small follow-ups): an additive `contract(schema)` rule in `kea-dhcp-relay.ts` |
| L4: VPP DHCP client committed before the V19 guard in the topology test | Needs a host run, and none is allowed before TD-25. The veths are down and the interface is sanitized on create (D-095). **Must be fixed before the next host run of `test/topology/kea-dhcp-relay`.** | The Q5 core-fix row, which must extend this test to BOUND anyway |
| L5: `/state/dhcp/relays` retrieves the whole `services` domain | Bounded (no FIB or session walk), polled at 30 s. It grows only when F-unbound's descriptors join the domain. | F-unbound-chrony-syslog, at its rebase as the second `services` lander: measure it, and add a tech-debt row if the poll exceeds about 1 s |
| L6: Kea control calls inside Retrieve can hold the scheduler for up to 30 s | Needs a hung daemon; the same class as VPP calls. | TD-9 (bounded calls): add the daemon control sockets to its scope |
| L8: a future secret field would be copied into the embedded input | No secret field exists in `DhcpServer` today (no DDNS/TSIG). | The task that adds DDNS/TSIG (behind PENDING-secret-channel): strip the field from `kea.Input` and add a guard test |
| L9: `coretest` `dhcpModels` `sync.Map` is never cleared | Test-only memory; no effect on behaviour. | This branch's worker at the rebase onto TD-23: move the state into the `RegisterExtension` closure. It is not a squash-time edit (D-134). |

## New in this round (all L, none blocks)

- **V1:** `status.go:238-248`. A shared read already in flight when `Apply` runs `dropLeaseCache` (`:260`) stores its
  pre-Apply result afterwards. For at most 10 s the lease grid can then show leases read before the Apply. The effect
  is harmless: leases are status, and subnet and server names come from a fresh `Status` on every call.
  - Fix: a generation counter checked before storing.
  - Owner: TD-22.
- **V2:** `descriptor_test.go:444-449`, the "cancelled caller" step. If the detached read finishes before the caller
  reaches `select` (`status.go:251-256`), both cases are ready and Go may pick `f.done`, so the test fails. It passed
  20 times under `-race`, so it is a low-probability flake.
  - Fix: `cachedLeases` returns `ctx.Err()` before it joins or starts a read. That also avoids starting a 60 s read for
    a caller that has already gone.
  - Owner: TD-22.
- **V3:** with `VRX_KEA_MODE=off`, which tools/app now uses, a document with a DHCP server fails the whole Apply. The
  error is "validation failed: no descriptor registered for kea.dhcp4", pointed at `/services/dhcp/servers`
  (`scheduler/reconciler.go:380`), and it is fail-safe: nothing is written. Such a commit is never stored, so a resync
  cannot hit it, and the stack's stored state is empty today.
  - Nicer: in `off` mode, project the servers as `agent.unsupported-field` warnings.
  - Owner: P12, which removes `off` from tools/app.

## Merge notes (additions to the review's list)

- **N1: `tools/app:108` conflicts when merged:**
  - with `task/F-host-acl-nftables`: the same line gains `VRX_HOST_ACL_MODE=check`;
  - with `task/TD-8b`: the next line gains `VRX_VPP_TABLE_BASE=13000`;
  - both are confirmed with `merge-tree`, and both are resolved by keeping every env token (mechanical).
  - With TD-8b's range, the product stack's relay families own tables 13000–13999. Q7 still refuses test mode (owner
    `vrx`).
- **N2: main has moved to `56200c3e`.** `git merge-tree --write-tree --name-only main task/F-kea-dhcp-relay` →
  `5ff4133a…`, exit 0: no conflicts. The review's earlier notes still apply: M4 against F-unbound, the TD-23
  registration line, and the squash subject `contract(`.

## Tests (this tree, f6c41176)

```
$ cd apps/agent && go test -race -count=1 ./internal/renderers/kea/... ./internal/descriptors/dhcp/... ./internal/subsystems/... ./internal/agent/... ./internal/desired/...
ok  	ngfw/agent/internal/renderers/kea	5.716s
ok  	ngfw/agent/internal/descriptors/dhcp	1.290s
ok  	ngfw/agent/internal/subsystems	6.963s
ok  	ngfw/agent/internal/agent	13.385s
ok  	ngfw/agent/internal/desired	1.299s
$ go test -race -count=20 -run 'TestLeaseReadsSharedAndCached|TestStatusForeignConfigNotConfigured' ./internal/renderers/kea/
ok  	ngfw/agent/internal/renderers/kea	47.611s
$ go test -count=1 ./internal/descriptors/core/... ./internal/scheduler/...        (neighbours: ok, ok)
mutants (go test -overlay, tree untouched): M1 FAIL · M2 FAIL · M3 FAIL (outputs quoted in the table)
$ pnpm turbo run test --filter=@ngfw/web --filter=@ngfw/api --filter=@ngfw/schema
  13/14 tasks ok; @ngfw/web: 1 failed | 106 passed — src/flows.test.tsx "login and protected routes > redirects to /login…"
  (26.8 s, body still empty: load average 48.7; the fix round touches no file on that path)
$ apps/web: vitest run src/flows.test.tsx src/domains/services/kea-dhcp-relay      Tests 14 passed (14)
$ apps/web: vitest run (full, re-run)                                              Test Files 15 passed · Tests 107 passed
$ apps/api: vitest run (unit)                                                      Test Files 10 passed · Tests 96 passed
```
The `dist/` and `openapi.json` build outputs from the test runs were removed, and the worktree is clean apart from this
file.
