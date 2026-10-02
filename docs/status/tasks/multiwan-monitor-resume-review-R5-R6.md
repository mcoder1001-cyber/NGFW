# WAN monitor — independent R5/R6 review

Reviewed `f5962281289941ffd2c0b7a1c30e0165740e3436`, tree `0e7dbc5af6a287e13607a69a544b9ced494615bb`, against base `c76774e8`. Manager identifies PR65 remote `12bd2f2a`. Isolated branch `review/wan-r5-r6`; report only, no product edits. Applied shared review instructions and specialized R5/R6 prompts, UI specification, feature/recovery envelope, WIP and user docs.

## R5 bounded performance — APPROVE

- Schema bounds 16 groups × 16 members × 8 monitors = 2048 logical probe workers. Runtime validates the total before replacing a good configuration; invalid input preserves the old generation. One worker invokes each monitor synchronously, so slow probes do not overlap themselves. Replacement cancels and drains the prior generation before starting another, rejects late observations, and preserves hysteresis for unchanged configurations.
- Per-observation aggregation visits at most eight monitors. Snapshot work is linear in configured samples plus sorting at most 16 groups and 16 members/group; it creates independent bounded response messages. No accumulating event/history cache, DB query, packet path, or per-client background worker was introduced.
- Probe calls get 50–60000 ms timeouts and sleep 100–600000 ms after completion. HTTP response headers cap at 16 KiB, DNS read buffers at 4096 bytes, ICMP buffers at 2048 bytes. HTTP disables keepalive and closes the response/idle connections; DNS/ICMP close connections and cancellation hooks. Unsupported binding fails closed.
- The watcher polls once per second, re-encodes the saved interface map under the transaction lock and clones the bounded WAN groups. This is linear work, not a per-WAN database/dump N+1. Optional future optimization: retain a saved revision/hash to avoid repeated unchanged-interface serialization, especially on configurations with many non-WAN interfaces. No measured contention or correctness failure found; not a merge blocker.
- At maximum workers/minimum interval the theoretical instantaneous-probe ceiling is 20,480 probe starts/sec; this is a scheduling bound, **not** a demonstrated sustainable capacity. Startup is not jittered. No throughput/capacity benchmark or SLA is claimed. The slice is monitor-only; the original full-feature failover/flow-distribution acceptance targets are not satisfied by this review.

Actual command in this isolated worktree's `apps/agent`:

```sh
PATH=/workspace/scratch/96b8b6fbc8a7/toolchain/bin:$PATH GOTOOLCHAIN=local GOCACHE=/tmp/dashboard-r2-gocache go test -race -count=1 ./internal/multiwan -run 'TestRuntime(ReplaceDrainsAndDiscardsLateResult|MonitorIndependenceAndNoOverlap|BoundsInvalidConfigurationDoesNotReplace|DeviceIdentityChangeInvalidatesHealth|SinceTracksAggregateMemberTransitions)$'
```

```text
ok  ngfw/agent/internal/multiwan  1.575s
```

Initial attempt without GOCACHE failed before tests because `/root/.cache/go-build` is read-only; the writable-cache command above passed. No broad gate run, packet/VPP test, or load benchmark.

## R6 user contract/docs — BLOCK

**MAJOR — live screen still promises forwarding that this runtime does not implement.** `apps/web/src/locales/en/multiwan.json:4` and `apps/web/src/locales/fa/multiwan.json:4`, rendered unconditionally by `apps/web/src/domains/routing/multiwan/WanGroupsPage.tsx:35`. Both intros say the healthy priority winner carries the default route and balance mode spreads traffic by weight. The new runtime now supplies successful live member health to that screen while deliberately installing no forwarding and leaving `active` empty. The newly added user-doc warning is accurate but does not correct the misleading on-screen explanation. Although these locale lines predate the PR, wiring the successful monitor-only runtime exposes the contradiction. Replace both introductions with an explicit monitor-only statement; distinguish configured failover/balance mode from implemented forwarding and disclose no route/NAT/ABF behavior. A narrow copy correction is sufficient; no UI redesign required.

The new first-paragraph user-doc status notice accurately limits support to default namespace/default VRF Linux LCP HTTP HEAD, DNS and IPv4 ICMP observations, unsupported interfaces failing closed, empty `active`, and deferred forwarding/NAT/dynamic gateway/VRF/ABF/lab work. The old following forwarding description is explicitly labeled follow-up by that notice. Projection also emits an unsupported-field warning. No added JSX, physical-direction CSS or locale keys in the reviewed product diff. Existing active-member decoration requires a nonempty `active`, so the new RPC does not falsely highlight a forwarding winner.

Live browser/RTL screenshots and network acceptance **NOT RUN**. No screenshot names or full-feature acceptance asserted. Required hosted quick remains separate.

**Verdicts: R5 APPROVE; R6 BLOCK (one MAJOR UI-honesty finding).**
