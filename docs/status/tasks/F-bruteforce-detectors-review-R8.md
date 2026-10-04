# F-bruteforce-detectors — R8 operability review

Exact product SHA `5c88cb5f2b09f59a161dcfa6d72cf89b831c207a`, documentation head `1df528d4ec77c1e7c9b238b26f34da8cd2e811b0`, base `06e4368c`. Independent review, no product edits.

No BLOCKER/MAJOR/MINOR findings. This source extends the existing observation/event path and API scan accounting; no package dependency, installation/removal script, migration, systemd unit or hosted CI gate changes. Existing API shutdown removes audit/commit/agent/ready subscriptions, stops the publisher and clears both timers. Agent watcher uses cancelable context and owned ticker; native ownership-safe reader remains unchanged. Distinct windows are bounded and prune expiry; unsupported thresholds warn during configuration reload. Operational documentation states saturation can miss attacks, matched API/agent deployment is required, legacy port-less observations are ignored, and real SSH testing requires pinned host keys and a disposable authentication fixture. The driver uses fixed argv, rejects transport/rate-limit failures as authentication evidence and exercises expiry/manual unblock through the existing API. It introduces no daemon lifecycle and declares the existing assigned-slot rig and lab lock prerequisites. Persisted active block enforcement/replay is unchanged; scan observations are transient runtime evidence, not new persistent data.

Independent exact-source evidence already run by this reviewer:

```text
go test -race -count=1 ./internal/detectors
ok ngfw/agent/internal/detectors 1.032s
go test -race -count=1 ./internal/agent -run TestAutoBlockObservationPreservesPortThroughStream
ok ngfw/agent/internal/agent 1.078s
python3 -m unittest test_driver.py
Ran 4 tests in 0.007s
OK
```

Commands used `/workspace/scratch/e4f791ef53f7/go/bin/go`, with Go cwd `detectors/apps/agent` and Python cwd `detectors/test/topology/autoblock`. No real host authentication/packet/expiry/restart acceptance or whole CI gate success is claimed. Those lab-only acceptance items remain deferred.

Verdict: **APPROVE**.
