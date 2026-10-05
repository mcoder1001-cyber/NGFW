# F-bfd-redistribution independent R5 review

Source local: `8f3d2f0789daa63a48d8254e6ee5df3dadea521f`; published source: `5a5c967cc088b235dc6379912bf7a576f010c572`; exact tree: `25359bb6d003ed24ae5347e5842933be721ea636`.
Reviewer owns only this report and independent review evidence. Product source was read in a private RAM snapshot; no product changes, shared VPP operations or throughput benchmarks.

## Findings

- **MAJOR** — `apps/agent/internal/subsystems/bfd_multihop.go:17` (Lookup), `:43` (Claim): each tuple lookup scans every persistent endpoint record under the claims mutex. `descriptors/bfd/multihop.go:59` invokes Lookup from per-session ownership, which both `descriptors/bfd/events.go:112` Sessions and `descriptors/bfd/bfd.go:541` Retrieve call for each dumped session. At the supported 1,024 configured sessions this is 1,048,576 record-prefix checks per complete multihop pass; BfdState performs two passes. Simultaneous event decoding repeats this work and contends on the same map. Generic durable storage does not require this newly introduced extra full scan. Fix with an exact tuple index/map or one batch snapshot per family, preserving duplicate/ambiguous rejection, owner and complete boot identity checks. Add a multi-session operation-count regression proving lookup does not scan unrelated tuples and a recovery/index-rebuild regression. This is a source complexity finding, not a measured latency/throughput claim.
- **MAJOR** — `apps/agent/internal/subsystems/bfd.go:214`: recordBfdState stores every observed owner/session key and never removes deleted session keys during normal operation. The sole removal is Wiring.Close (`:117`). A long-lived agent with repeated add/event/delete configurations accumulates an unbounded history despite the 1,024 concurrent configuration cap. Recreating the same key also inherits the old lastFlap, contradicting the documented initial/restart unknown semantics. Fix with lifecycle-aware owner-scoped pruning/removal of absent/deleted sessions, including disconnect/reconnect invalidation as needed; do not clear foreign owners. Add churn, delete/recreate and owner-isolation regressions.

## Reviewed bounds

Descriptor Retrieve performs one interface dump and lazily at most one complete boot-identity read per batch; no per-session native identity query. FRR millisecond conversion rejects uint32 microsecond overflow. Redistribution uses registered scoped summaries rather than a full RIB. Event output buffer is 64; cancellation-aware sends, 3-second unregister cleanup, 5-second bounded watcher stop, one-second retry backoff and 30-second UI polling were read. Pre-existing per-event interface dump is inherited and is not graded as a new defect here.

## Verification

Actual command, in `/dev/shm/r5-review/snapshots/bfd/apps/agent`:

```text
env TMPDIR=/dev/shm/r5-tmp GOTMPDIR=/dev/shm/r5-tmp GOCACHE=/dev/shm/r5-cache GOMAXPROCS=2 GOFLAGS=-p=2 GOTOOLCHAIN=local ../../../../tools/heavy.sh go test -race -count=1 ./internal/descriptors/bfd ./internal/agent ./internal/renderers/frr/bfd ./internal/renderers/frr/redistribute ./internal/subsystems
heavy: slot 2 after 30s
```

The complete targeted command exited 0. Actual output:

```text
heavy: all 3 heavy slots busy, waiting (0s): go test -race -count=1 ./internal/descriptors/bfd ./internal/agent ./internal/renderers/frr/bfd ./internal/renderers/frr/redistribute ./internal/subsystems
heavy: slot 2 after 30s
ok  	ngfw/agent/internal/descriptors/bfd	1.142s
ok  	ngfw/agent/internal/agent	51.053s
ok  	ngfw/agent/internal/renderers/frr/bfd	1.121s
ok  	ngfw/agent/internal/renderers/frr/redistribute	1.112s
ok  	ngfw/agent/internal/subsystems	27.637s
```

All five selected packages passed their unmodified race suites. This does not resolve the two source scalability findings. Existing multihop tests use an O(1) map fake for claims and therefore do not exercise the production full-map scan. Findings derive from exact source loops and absence of delete-time observation invalidation, independently of suite completion. No live packet/event latency acceptance or full quick gate was run by R5.

Verdict: **BLOCK** — 0 BLOCKER, 2 MAJOR, 0 MINOR. These code defects cannot be deferred as lab acceptance.
