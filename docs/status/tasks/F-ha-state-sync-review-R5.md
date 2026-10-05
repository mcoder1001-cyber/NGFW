# F-ha-state-sync independent R5 review

Exact local source `48e5af12fd6a42323f3c83d04129eb6563cef650`, published source `4cf79548`, tree `c42eb2959321f7a7bd135b9312b595c37f0baef8`. Full source read in `/dev/shm/r5-review/snapshots/ha`; no product edits or live host faults.

## Findings

- **MINOR** — `apps/agent/internal/agent/rpc_ha_sync.go:21`: haSyncRuntimes holds Service pointers indefinitely, while Service.Close (`service.go:1213`) deletes captureManagers but not this map. The explicit one-Service-per-process architecture bounds ordinary production use; repeated Service creation/close in an embedded host or tests retains each service graph. Delete its entry during Close or store the runtime directly on Service. Not a production-scale blocker under the stated architecture.

## Bounds and timing review

Native listener/failover state is read through two getters, not a whole NAT-session table. Resync installs its watcher before request, correlates a nonzero PID, ignores unmatched completion events, and updates only one mutex-protected latest observation. Run imposes a default 15-second context, including the globals lock; cancellation closes the watcher. Lock contention waits on a stopped 10ms ticker rather than spinning. Flush neither waits for nor fabricates a resync completion. The API action budget is 20 seconds; UI read polling is 30 seconds. No copied per-client histories or new DB list queries appear in this scope.

The two-node acceptance controller requests an exact-filtered 256-item EI session page and refuses truncation/ambiguity. HTTP reads have a 1MiB bound and 3-second read or 20-second action timeout. Role/convergence loops have monotonic 15/20-second deadlines; priority restoration has a 75-second deadline. These are deadlines checked between bounded operations, not hard realtime guarantees. One persistent TCP socket uses a 10-second read timeout and its pipe controller waits 11 seconds; sequence/challenge and server-observed tuple must remain identical. The server accepts once, preventing reconnect from becoming continuity evidence. The 3.2-second delayed-echo regression covers normal default VRRP master-down delay that exceeded the old 3-second socket timeout.

## Actual verification

```text
cd /dev/shm/r5-review/snapshots/ha
env TMPDIR=/dev/shm/r5-tmp PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -v -s test/topology/ha-state-sync -p 'test_*.py'
Ran 16 tests in 3.233s
OK
```

The suite includes real local TCP continuity/delayed echo, guarded transition/ownership paths and redirect refusal; no real two-node VM, VPP kill, SSH fault or measured appliance convergence occurred. Targeted race command exited 0 in `/dev/shm/r5-review/snapshots/ha/apps/agent`:

```text
env TMPDIR=/dev/shm/r5-tmp GOTMPDIR=/dev/shm/r5-tmp GOCACHE=/dev/shm/r5-cache GOMAXPROCS=2 GOFLAGS=-p=2 GOTOOLCHAIN=local ../../../../tools/heavy.sh go test -race -count=1 ./internal/actions/ha-state-sync ./internal/descriptors/hasync ./internal/agent -run 'Test(Resync|Flush|NativeFailure|NonOwner|HaSync|Listener|Failover|Lock)'
heavy: all 3 heavy slots busy, waiting (0s): go test -race -count=1 ./internal/actions/ha-state-sync ./internal/descriptors/hasync ./internal/agent -run Test(Resync|Flush|NativeFailure|NonOwner|HaSync|Listener|Failover|Lock)
heavy: slot 1 after 150s
ok  	ngfw/agent/internal/actions/ha-state-sync	1.138s
ok  	ngfw/agent/internal/descriptors/hasync	1.093s
ok  	ngfw/agent/internal/agent	3.880s
```

The action suite exercised matching/nonmatching completion, misses, timeout without false completion, native failure and flush; HaSync RPC truth/slot-denial ran. These are unit/fake-native tests, not a live two-node test. Full quick is the manager/tester's responsibility. No throughput or scale benchmark claims.

Verdict: **APPROVE** — 0 BLOCKER, 0 MAJOR, 1 MINOR for R5 source review; pending Go command completion does not represent live lab PASS.
