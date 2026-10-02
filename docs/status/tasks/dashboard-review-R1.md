# Dashboard agent wiring — independent R1 review

Reviewed commit `52a72194e5ed85ce77f395809609ea4a83c9ee43`, against the correctness/tests prompt and the explicitly limited agent-wiring status report. No product files changed.

## Findings

### MAJOR — active scrape lifecycle can outlive shutdown

`apps/agent/internal/promexport/listener.go:148–153`, `apps/agent/internal/promexport/govpp.go:35–59`, `apps/agent/internal/subsystems/dashboard_prom_alarms.go:101–102`.

`stopLocked` ignores a timed-out `http.Server.Shutdown` and immediately forgets the server. Shutdown does not cancel active requests. A scrape still running, or queued on the source mutex, can therefore survive listener deletion/replacement. Wiring closes the listener and then the stats connection, but the source's public Close deliberately allows subsequent reads to reconnect. An active request which acquires that mutex after source.Close can reopen the stats segment after the agent's close seam has completed. Existing tests only stop an idle listener and do not exercise this sequence.

Fix: retain the server until shutdown completes; explicitly close remaining connections when graceful shutdown times out, and provide a terminal source shutdown distinct from the reconnect-capable reset/Close used by the live test. Prevent reads from reopening a terminally stopped source. Add a deterministic test with an in-flight/queued reader and barriers; verify request cancellation, source disposal, and no post-shutdown reconnect. This is a code-path finding, not a claimed reproduction against a live stats segment.

### BLOCKER — full feature acceptance remains unverified

`docs/status/tasks/F-dashboard-prom-alarms-host.md:63–72`, `prompts/features/F-dashboard-prom-alarms.md` acceptance section.

Real VPP traffic counter growth, actual restart reconnection, link-down/up webhook delivery, API/agent restart without duplicate delivery, and real-endpoint dashboard evidence are explicitly pending. Unit fakes establish counter interpretation but cannot prove the complete feature. The complete quick gate is also pending unrestricted CI. Keep this feature's final acceptance/merge gate blocked until those reports exist; do not count the skipped live test as passing evidence. The report honestly preserves these limits.

### MINOR — bind-failure test does not assert retrieved configuration

`apps/agent/internal/subsystems/dashboard_prom_alarms_test.go:66–74`.

The occupied-port update test proves the old HTTP endpoint still serves, but does not re-read and compare the saved configuration immediately after failure. The implementation correctly assigns `s.value` only after Start succeeds. Add Retrieve/proto.Equal after the rejected update so a future premature state assignment is detected.

## Checks and mapping

- Pinned govpp v0.13.0 interface/error/buffer API inspected; per-thread simple-counter matrices aggregate calls/vectors/clocks consistently. Ratios are vectors/call and clocks/vector, not invented CPU percentages.
- Stats-only interfaces expose aggregate drops and omit unavailable directional-drop/link/admin samples.
- Enabled management projection, default address/port, invalid input, cloned assembly, absent configuration, and listener create/delete/recreate have focused tests.
- Same-address handler replacement uses an independent RWMutex; stage update/retrieve uses the stage mutex. Failed replacement bind occurs before stopping the old listener. Same-address rollback restores the old allow-list handler.
- Close seam runs last-registered-first, therefore stage close precedes source close. The active-handler exception is the MAJOR above.
- `TestMetricsCollectors` preserves its empty-list, failed-output suppression, panic accounting, ordering, and deadline assertions; the added production-collector check precedes isolation. The AF_UNIX-blocked seam test was not rerun per manager instruction.

## Commands actually run

Worktree: `/workspace/scratch/96b8b6fbc8a7/NGFW-dashboard/apps/agent`.

```text
GOMAXPROCS=2 GOFLAGS=-p=2 /usr/local/go/bin/go test -race -count=1 ./internal/promexport ./internal/desired ./internal/subsystems -run 'Test(Govpp|Prometheus|Handler|Listener)'
ok  ngfw/agent/internal/promexport  1.017s
ok  ngfw/agent/internal/desired     1.080s
ok  ngfw/agent/internal/subsystems  1.164s

GOMAXPROCS=2 GOFLAGS=-p=2 /usr/local/go/bin/go test -race -count=1 ./internal/promexport ./internal/desired
ok  ngfw/agent/internal/promexport  1.019s
ok  ngfw/agent/internal/desired     8.921s
```

`TestGovppLiveStats` skips without `VRX_INTEGRATION=1`; no lab was available and it was not counted as live evidence. Whole quick gate was not repeated: manager reports baseline AF_UNIX EPERM and an unrestricted hosted run is pending. An initial invocation using bare `go` failed because Go is not on PATH; commands above use the installed absolute executable.

## Verify round — commit `35c4533e723352c918bb8e842a9155e59c4d6bed`

The MAJOR is resolved. `GovppSource.Stop` now cancels its terminal lifetime before taking the reader mutex, checks that terminal state both before and after acquiring the mutex, and disposes any active mapping. Subsequent reads cannot reconnect. Close remains a deliberately reconnectable transient reset. The wiring runs terminal source stop before external listener stop; `stopLocked` now forcibly closes active HTTP connections if its graceful deadline expires. Tests use channel barriers for an active read and a second reader, assert both return cancellation, assert connection count stays one after shutdown, verify transient reset reconnects, and verify HTTP handler context cancellation after the graceful deadline.

The MINOR is resolved: after the occupied-port failure the lifecycle test retrieves the singleton and compares it with the original configuration.

Actual verification command in the same agent-module directory:

```text
GOMAXPROCS=2 GOFLAGS=-p=2 /usr/local/go/bin/go test -race -count=1 ./internal/promexport ./internal/subsystems -run 'Test(Govpp|Listener|Prometheus)'
ok  ngfw/agent/internal/promexport  2.033s
ok  ngfw/agent/internal/subsystems  1.230s
```

No full quick gate or live VPP acceptance was repeated in this verify round. The opt-in live test remains skipped, not passing evidence. Synchronous govpp calls themselves do not accept a context; terminal Stop waits for the current govpp operation to return before disposal, while ensuring no later read can reconnect.

**Code verdict: APPROVE** — the MAJOR and MINOR findings are fixed and relevant race tests pass.

**Full-feature acceptance verdict: BLOCK / pending** — the independent live-lab and unrestricted-CI gate above remains explicit; code approval does not establish completed feature acceptance.
