# R1 correctness and tests

Reviewed product head: `5c88cb5f2b09f59a161dcfa6d72cf89b831c207a`.

No unresolved product correctness findings. Trusted journal record replay filtering and source validation remain in place. Validated scan destination ports survive the event stream and subscriber; repeated ports refresh timestamps rather than adding failures. Exact window expiry, source separation, management and mapped allowlists, malformed/legacy events and subscriber shutdown have meaningful regression tests. Fixed source/per-source/aggregate budgets discard overflow evidence rather than turning discarded samples into false threshold hits; capacity is reclaimed explicitly. The native implementation change is only an additive keyed observation literal, not a change to its existing deduplication policy.

Independent commands at this product head:

```text
pnpm --filter @ngfw/api exec vitest run src/features/auto-block/engine.test.ts src/features/auto-block/publisher.test.ts src/features/auto-block/port-window.test.ts src/features/auto-block/detector-events.test.ts
Test Files 4 passed (4)
Tests 31 passed (31)
Duration 4.98s

go test -race -count=1 ./internal/detectors ./internal/agent -run 'Test(Host|Native|AutoBlockObservation|JournalObservation)'
ok ngfw/agent/internal/detectors 1.036s
ok ngfw/agent/internal/agent 1.110s

go test -race -count=1 ./internal/detectors
ok ngfw/agent/internal/detectors 1.157s

python3 -m unittest test/topology/autoblock/test_driver.py
Ran 4 tests in 0.006s
OK
```

Required `tools/ci.sh --base main`: BLOCKED-ENV. Pinned tools, install, contract, generation, forbidden-pattern, secret and slot guards passed. The manager directed stopping this duplicate gate at Turbo after both baseline and the PIM gate independently reproduced platform `listen EPERM` for temporary Unix gRPC sockets and `chown EINVAL`. Ctrl-C terminated the reviewer-owned gate with exit 130; no complete quick pass is claimed. Logs: `/tmp/ngfw-ci/detectors-20261004-045755-2`. Hosted quick remains required. An earlier broader subsystem test run reproduced pre-existing PBR missing-dependency failures; it was not called a passing suite. Real local-in/forwarding journal/topology acceptance remains NOT RUN.

Verdict: APPROVE for product correctness at the stated head; integration remains conditional on the mandatory complete quick gate.
