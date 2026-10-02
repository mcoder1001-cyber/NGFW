# F-dashboard-prom-alarms-host — agent wiring (2026-10-01)

Implemented real govpp v0.13.0 stats-segment reader, the loopback collector registration,
management.prometheus singleton projection/retrieval, and external listener descriptor.
The reader owns its connection, reconnects after errors, and is closed on agent shutdown.
No scrape uses the binary API. Worker ratios use raw per-thread node counter matrices;
interfaces, buffer pools and node errors use the pinned govpp StatsProvider API.

VPP stats only provides aggregate interface drops and no interface state flags. The source
publishes vrx_interface_drops_total and omits unavailable directional drops/link/admin samples.
It does not invent CPU percentages. Existing alarm link-event subscriptions remain unchanged.
The external listener's same-address updates now replace its allow-list handler. A failed
bind to a replacement address preserves the existing listener and retrieved configuration.
A failed scrape responds 500 without exposing partial dataplane families.

## Verification

`GOMAXPROCS=2 GOFLAGS=-p=2 go test -race -count=1 ./internal/promexport ./internal/desired`
followed by `go test -race -count=1 ./internal/subsystems -run '^TestPrometheusListenerLifecycle$'`:

```text
ok  ngfw/agent/internal/promexport  1.032s
ok  ngfw/agent/internal/desired     10.659s
ok  ngfw/agent/internal/subsystems  1.128s
```

Tests cover multi-thread vector/clock ratios, buffer/error mapping, omission of unavailable
state, cancellation/failure, projection/defaults/validation/assembly, listener create/retrieve,
same-address ACL update and rollback, occupied-address preservation, deletion and recreation.

`golangci-lint run` (full agent module, pinned 2.13.2):

```text
0 issues.
```

`tools/ci.sh check --base main`:

```text
check PASSED (0m02s)
```

The broader focused race run cannot pass in this execution sandbox:

```text
--- FAIL: TestLinuxNetdevKindOnThisHost (0.00s)
    netdev_test.go:43: lo: kind "" exists false err netlink RTM_GETLINK: operation not permitted
--- FAIL: TestMetricsCollectors (0.00s)
    seams_test.go:145: listen <test socket>: listen unix <test socket>: socket: operation not permitted
```

Other existing agent gRPC tests also fail at AF_UNIX socket creation with EPERM.
The complete quick gate was attempted unchanged and failed at API unit tests:

```text
@ngfw/api:test: Tests 8 failed | 299 passed | 63 skipped (370)
Tasks: 34 successful, 35 total
Failed: @ngfw/api#test
```

Those socket-dependent tests are blocked by the same sandbox AF_UNIX policy, as confirmed by
the manager's baseline run. The gate needs the manager's unrestricted CI execution surface. No skipped/failing suite counts as a complete green gate or merge approval.

## Pending live acceptance

`TestGovppLiveStats` is opt-in with VRX_INTEGRATION=1 and requires a real VPP stats socket;
it verifies a real snapshot and explicit connection close/reconnect. This cloud workspace
has no VPP lab, and it was not run as passing evidence. Rig traffic counter growth, actual
VPP restart reconnection, link-down/up webhook delivery, API/agent restart without duplicate
notifications and dashboard screenshot against the real endpoint remain pending on the lab.
No host daemon packages, startup.conf or generated contracts were changed.

## Shared hunks

- subsystems.go: extend Management descriptors and one named registration call.
- projection.go: management-scoped project/assemble calls.
- agent/seams_test.go: assert production dashboard registration, then isolate test collectors
  to retain the empty-list and every existing failure/panic/deadline accounting assertion.

## Independent R1 shutdown fix (2026-10-01)

The independent review found that graceful HTTP shutdown could time out while active
scrapes survived, and the reconnect-capable source.Close could let queued requests reopen
the mapping after agent teardown. This is fixed by a distinct terminal GovppSource.Stop:
it signals cancellation before waiting for an active reader, disposes the mapping, and
rejects queued/later reads even after a transient Close. Wiring closers now execute source
Stop before listener close. The listener force-closes remaining connections when graceful
Shutdown times out, cancelling their request contexts before discarding the server.
The failed-bind test also verifies retrieved configuration remains proto.Equal to its
pre-update value.

Barrier-based tests hold one active source reader, start a queued reader, trigger terminal
shutdown, release the active read, and verify both return cancellation, mapping disposal
and no subsequent connection attempt. A separate HTTP test holds an active request through
the graceful shutdown deadline and verifies request cancellation and completion. Transient
Close is tested separately to ensure it still reconnects until terminal Stop.

Actual R1 commands and output:

```text
go test -race -count=1 ./internal/promexport ./internal/desired ./internal/subsystems -run 'Test(Govpp|Prometheus|Handler|Listener)'
ok  ngfw/agent/internal/promexport  2.055s
ok  ngfw/agent/internal/desired     1.216s
ok  ngfw/agent/internal/subsystems  1.187s

golangci-lint run
0 issues.
```

After adding the explicit transient-reset test, `go test -race -count=1 ./internal/promexport`
passed (`ok ngfw/agent/internal/promexport 2.040s`); `tools/ci.sh check --base main` passed
(`check PASSED (0m06s)`).

The unchanged full local quick gate was not repeated (baseline socket EPERM remains).
Full unrestricted CI and all real VPP rig acceptance remain pending blockers; this patch
finishes the reviewed agent code fixes, not full host-task acceptance.

## Recovery checkpoint (2026-10-02)

Recovered the remotely preserved dashboard branch onto current main `471c61fa`; the only
conflict was the decision log, resolved by retaining both CI D-162 and dashboard D-164.
Product code is unchanged from the independently approved remote checkpoint.
Owner authorization now defers live laboratory/browser acceptance to
`docs/status/DEFERRED-ACCEPTANCE.md`; these tests remain NOT RUN.
Full hosted quick CI remains a required pre-merge check.
