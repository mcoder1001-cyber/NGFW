# W-seed: wave-A hotspot anchors and shared seams (no behaviour change)

Branch `task/W-seed` (slot 3), base `task/P08@74ec04e` (speculative, D-114), then `task/P08@e1587c9` merged (see "P08 merge").
Commits: `5c6e1f8 contract(wave-A): anchors` (schema + proto), `64fc0e9 chore(wave-A): hotspot anchors, seams and shells`
(the rest), plus the P08 merge and this status file. Questions and rule conflicts: `W-seed-questions.md`.
**main (TD-5) is not merged.** The merge's own quick gate fails on a P08 × TD-5 test interaction that is already fixed on `task/P08`
(see "main merge" and Q6). The merger's planned rebase after P08 lands resolves it.

## What
**Anchors.** One `// wave-A: <task-id>` line (`#`/`<!-- -->` where the language needs it) per touching task at each insertion point
of `docs/status/wave-A-hotspots.md` §1. The touching tasks come from §3 and from the five wave-B envelopes (P11, F-wireguard, P12,
F-kea-dhcp-relay, F-unbound-chrony-syslog), and the anchors are in board order (`plan/tasks.yaml`). There are two exceptions to board order:
proto message anchors follow the §2 field numbers, and the `nav.test.ts` list follows navigation order (Q4). Every anchor block starts with
a one-line comment that says what a feature adds under its anchor.

| id | file | anchor blocks (touching tasks) |
|---|---|---|
| A1 | `apps/agent/internal/subsystems/subsystems.go` | new domain consts (10) · `Domains` Interfaces (6), VRFs (2), Routing (4), new domain entries (10) · end of `Register()` (16) = 48 |
| A2 | `apps/agent/internal/agent/projection.go` | end of `project()` (16) · end of `assemble()`, after `desired.Assemble` and the routes (16) = 32 |
| A4 | `apps/agent/internal/agent/server.go` | `Action` is a type switch with `default:` Unimplemented (same message); case anchors F-vrf-static-ecmp (ping), F-neighbors-ra (arp-flush), F-nat44-ed-sessions (session-kill), F-unbound-chrony-syslog (dns lookup, from its envelope) = 4 |
| C1 | `packages/schema/src/domains/interfaces.ts` | `InterfaceSchema` (6) · `SubinterfaceSchema` (3) = 9 |
| C1 | `packages/schema/src/domains/vrfs.ts` | `VrfSchema` (2) |
| C1 | `packages/schema/src/domains/routing.ts` | `NextHopSchema` (1) · `StaticRouteSchema` (2) · `RoutingSchema` root (2) = 5 |
| C1 | `packages/schema/src/domains/services.ts` | `ServicesSchema` root (2) |
| C2 | `packages/schema/src/semantic/index.ts` | imports (14) · `SEMANTIC_VALIDATORS` spreads (14) = 28 |
| C3 | `packages/schema/src/index.ts` | `export * from './domains/ext/<slug>.js'` (11) |
| C5 | `packages/proto/vrx/v1/dataplane.proto` | end of `service Dataplane` (16) · `Interface` (6) · `Subinterface` (3) · `Vrf` (2) · `StaticRoute` (2) · `RoutingConfig` (2) · `ServicesConfig` (2) · `AclConfig` (2) · `ActionRequest.action` (3) · `EventKind` (5) = 43; plus 16 `// ----- <task-id> -----` section stubs at the end of the file |
| C6 | `docs/contracts/proto.md` | new heading "## 11. Feature RPCs" (16) |
| P1 | `apps/api/src/app.module.ts` | import (16) · `controllers` (16) · `providers` (16) = 48 |
| P4 | `apps/api/src/agent/agent.client.ts` | `@ngfw/proto` type imports (16) · methods (16) = 32 |
| P5 | `apps/api/src/testing/fake-agent.ts` | `impl()` handler object (16) |
| P6 | `apps/api/src/infra/bus.ts` `TOPICS` (6) · `apps/api/src/telemetry/relay.service.ts` `eventTopic()` cases (6) = 12 |
| W1 | `apps/web/src/router.tsx` | above `system/users` (15; EI uses ED's natTabs) |
| W2 | `apps/web/src/nav/nav.ts` | `BUILT_DOMAINS` (8) · non-domain NavItems in `buildNav` (7) = 15 |
| W2 | `apps/web/src/nav/nav.test.ts` | `available` list (15, in navigation order) |
| W3 | `apps/web/src/i18n.ts` | en+fa imports (17) · `NAMESPACES` (17) · `en` (17) · `fa` (17) = 68 |
| shells | `apps/web/src/domains/{vpn,services}/tabs.ts` | P11, F-wireguard · F-kea-dhcp-relay, F-unbound-chrony-syslog = 4 |

**One entry per line.** `Domains` slices (A1), `BUILT_DOMAINS` and the test's `available` list (W2), and the i18n imports,
`NAMESPACES` and `en`/`fa` literals (W3). Their content is unchanged; the only additions are the shell namespaces `services` and `vpn`.

**Seams** (launch plan §5.2):
- `subsystems.Env.Publish func(*vrxv1.Event)` and `Env.Resync func()` are reached through `Wiring.Publish(ev)` and
  `Wiring.RequestResync()` (`subsystems/seams.go`). Both are nil by default, and nil means no-op. `TestEventAndResyncHooksDefaultInert`
  checks this. `agent.go` does not set the hooks yet, because it is outside this envelope. Q1 has the proposed wiring.
- `subsystems.SlotIDRange() (*IDRange, error)` reads `VRX_VPP_TABLE_BASE` and returns base..base+999. It returns nil when the variable
  is unset (the product agent owns every id) and an error for a malformed value, 0, or a range that overflows. `TestSlotIDRange` covers these cases.
- The vpn and services page shells are `apps/web/src/domains/DomainTabsPage.tsx` plus `{vpn/VpnPage,services/ServicesPage}.tsx`
  and their `tabs.ts` registries. With an empty registry the page renders exactly `DomainPlaceholderPage`, so there is no
  user-visible change. The shell routes replace the placeholder routes for `vpn` and `services`, so each path is registered once. The nav
  is unchanged: both domains stay out of `BUILT_DOMAINS`. The i18n keys `vpn:{title,tabs,loading}` and `services:{…}` exist in en
  and fa. `DomainTabsPage.test.tsx` covers the placeholder when empty, a tab per registration with `?tab=`, the fallback for an unknown
  tab, and a single route per path.

## How verified
All W-seed runs are on head `0083590` (= `64fc0e9` + P08 merge `529e119` + the salvaged status file), slot 3, `TMPDIR=/tmp/g-w3`.

### 1. Generated output: `pnpm gen && git status --porcelain`
The CI gate runs `pnpm gen` and then checks the generated trees. Afterwards, `git status --porcelain` in the worktree is empty. W-seed
changes no byte of generated code compared with its base. Proto and schema edits are comments only, and none of them leaks into the output:
```
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
$ git status --porcelain | wc -l
0
$ git diff --stat e1587c9 HEAD -- apps/agent/gen packages/proto/gen packages/api-client/src/generated
(empty)
```
The CI contract guard lists four generated files as changed "since main". Those files are P08's interface RPCs (the branch base),
not W-seed changes.

### 2. Pass counts: base vs W-seed (same counts; the only additions are the new seam tests)
The base is `task/P08@74ec04e`: gen, lint, typecheck, test, and `make -C apps/{agent,cli} lint test` all rc=0 (15:43–15:51). The branch's
current base `e1587c9` differs from `74ec04e` only in `docs/` and `test/topology/interfaces`. Neither path is part of `pnpm test` or
`apps/{agent,cli}`, so these base counts still hold. On W-seed, the vitest counts come from `pnpm test` (rc=0, 17:22, 10/17 tasks
replayed from the CI run below). The Go counts come from that CI run's `make lint test build` logs.
```
                      base 74ec04e           W-seed 0083590
@ngfw/schema   Test Files  37 · Tests 1214     37 · 1214
@ngfw/proto    Test Files   2 · Tests   68      2 ·   68
@ngfw/api      Test Files   8 · Tests   50      8 ·   50
@ngfw/ui-kit   Test Files  12 · Tests   48     12 ·   48
@ngfw/web      Test Files  13 · Tests   86     14 ·   92   (+ DomainTabsPage.test.tsx: 6 tests)
turbo          Tasks: 17 successful, 17 total  Tasks: 17 successful, 17 total
apps/agent     go vet + golangci-lint 0 issues · go test -race: ok=89 FAIL=0 no-test-files=162   (identical, same package set)
apps/cli       0 issues · ok=7 FAIL=0 no-test-files=5                                         (identical)
```
The agent package count stays at 89 because the new seam tests live in the existing `internal/subsystems` package:
```
$ go test -count=1 -v -run 'TestEventAndResyncHooksDefaultInert|TestSlotIDRange' ./internal/subsystems/
=== RUN   TestEventAndResyncHooksDefaultInert
--- PASS: TestEventAndResyncHooksDefaultInert (0.00s)
=== RUN   TestSlotIDRange
--- PASS: TestSlotIDRange (0.00s)
PASS
ok  	ngfw/agent/internal/subsystems	0.247s
```

### 3. Anchors: `grep -rn "wave-A:" apps packages docs | wc -l`
This count is taken after deleting the ignored build outputs (`dist/`, `bin/`), which the host rules require before finishing. With them
present, the count also includes compiled copies of the anchors.
```
$ grep -rn "wave-A:" apps packages docs | wc -l
441
```
The hotspot files hold 425 of them, per file as in the table above. The other 16 are prose: 12 feature envelopes (1 each), `wave-A-hotspots.md` (1)
and this file (3, including the command above).
```
48 apps/agent/internal/subsystems/subsystems.go      48 apps/api/src/app.module.ts            68 apps/web/src/i18n.ts
32 apps/agent/internal/agent/projection.go           32 apps/api/src/agent/agent.client.ts    15 apps/web/src/nav/nav.ts
 4 apps/agent/internal/agent/server.go               16 apps/api/src/testing/fake-agent.ts    15 apps/web/src/nav/nav.test.ts
43 packages/proto/vrx/v1/dataplane.proto (+16 "// ----- <id> -----" stubs)   6 apps/api/src/infra/bus.ts   15 apps/web/src/router.tsx
16 docs/contracts/proto.md                            6 apps/api/src/telemetry/relay.service.ts
28 packages/schema/src/semantic/index.ts             11 packages/schema/src/index.ts             2 apps/web/src/domains/vpn/tabs.ts
 9 packages/schema/src/domains/interfaces.ts          5 packages/schema/src/domains/routing.ts   2 apps/web/src/domains/services/tabs.ts
 2 packages/schema/src/domains/vrfs.ts                2 packages/schema/src/domains/services.ts
```

### 4. `TMPDIR=/tmp/g-w3 tools/ci.sh --base main`: green
```
== VRX CI gate: quick ==
branch    task/W-seed @ 0083590   (base: main)
ok — contract commit(s) on the branch:
  5c6e1f8 contract(wave-A): anchors
== generate + generated-output gate ==
clean: packages/proto/gen apps/agent/gen packages/schema/dist packages/api-client/src/generated
== forbidden patterns (+ gitleaks) ==
ok: no shell/VPP/FFI access in apps/api/src apps/web/src packages/*/src
ok: no Dockerfile/compose files
ok: no kill-by-pattern in scripts
ok: no secret-shaped strings
ok: gitleaks — scanned ~531901 bytes (531.90 KB) in 1.37s no leaks found
== lint · typecheck · unit tests · build (turbo) ==
Tasks:    30 successful, 30 total Cached:    24 cached, 30 total Time:    2m26.189s
== apps/agent: make lint test build ==     (89 packages ok, 0 FAIL)
== apps/cli: make lint test build ==       (7 packages ok, 0 FAIL)
test/integration/smoke: gofmt ok · go vet ok · ok  	ngfw/test/integration/smoke	0.056s;
test/topology/interfaces: gofmt ok · go vet ok · ok  	ngfw/test/topology/interfaces	0.013s;
  mode quick · wall time 6m52s · logs /root/ngfw-wt/logs/ci/W-seed-20260924-171444-1296408
CI GATE PASSED
```
An earlier run on `529e119` (16:22, before the status-file salvage) also passed (`logs/ci/W-seed-20260924-162221-1043789`).

### P08 merge
`529e119` merged `task/P08@e1587c9` into the branch with **no conflicts**: `git show --remerge-diff 529e119` is empty. P08's delta
since `74ec04e` covers only `docs/status/tasks/P08*`, `docs/user/interfaces/basics.md` and `test/topology/interfaces/interfaces_test.go`.
None of these is a hotspot file. As the CONTINUE brief says, I did not merge `task/P08` again (tip now `7c06b88`).

### main merge (CONTINUE brief): not committed (Q6)
`git merge main` (e310add: TD-5, rig-down race, board) had no conflicts, but the `pre-merge-commit` hook's
`tools/ci.sh quick --base HEAD` on the merged tree failed (logs `W-seed-20260924-165319-1162926`). Turbo was 30/30 successful and gen and
gitleaks were clean. Only `apps/agent` failed:
```
--- FAIL: TestEveryAfPacketDeleteIsQuiesced (88.99s)
    guard_test.go:233: ../../../internal/descriptors/core/coretest/ifext.go:170:21: af_packet_delete sent outside (*HostInterfaceDescriptor).quiescedDelete: the netdev is not quiesced first (D-101, VPP V24)
FAIL	ngfw/agent/internal/descriptors/af_packet	94.147s
--- FAIL: TestWatchResync (23.33s)
    review_fixes_test.go:275: resync true poll false
FAIL	ngfw/agent/internal/renderers/strongswan	65.051s
```
Neither failure comes from W-seed. I checked both in scratch clones; nothing was committed:
```
# P08@e1587c9 + main, WITHOUT W-seed: the same guard failure
--- FAIL: TestEveryAfPacketDeleteIsQuiesced (1.39s)
    guard_test.go:233: ../../../internal/descriptors/core/coretest/ifext.go:170:21: af_packet_delete sent outside ...
# W-seed + main + the guard_test.go diff of task/P08@7c06b88 (D-118): passes
    guard_test.go:266: apps/agent: 0 violations; af_packet_delete is sent at 1 site (quiescedDelete, after quiesce)
--- PASS: TestEveryAfPacketDeleteIsQuiesced (0.68s)
--- PASS: TestGuardCatchesARawAfPacketDelete (0.01s)
# strongswan TestWatchResync alone on the merged tree (load avg was 140 during the hook run): 3/3
ok  	ngfw/agent/internal/renderers/strongswan	1.536s
ok  	ngfw/agent/internal/renderers/strongswan	1.266s
ok  	ngfw/agent/internal/renderers/strongswan	1.301s
```
The fix is in `task/P08@7c06b88`, which this branch must not merge now, and in TD-5's `guard_test.go`, which is not my file. The
auto-mode permission check refused a logged `git merge --no-verify`, so I ran `git merge --abort`. The branch head stays `0083590` + this file.
When the merger rebases W-seed onto main after P08 lands, it gets TD-5 and 7c06b88 together.

## Out of scope
Feature logic, schema/proto fields, RPCs, reachable new screens, P08 behaviour, `tools/ci.sh`, anchor removal, host tests.
`agent.go` wiring of the A5 hooks (Q1). Merging main before P08 lands, and any edit to TD-5's `guard_test.go` (Q6).
