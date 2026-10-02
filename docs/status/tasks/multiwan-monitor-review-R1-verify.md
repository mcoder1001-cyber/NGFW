# Multi-WAN R1 correction verification

Independent review of product `29e5e2deac10a59d7709b792db4812511ccc626d`, checked out with documentation follow-up `d746440f` in isolated `review/wan-r1-verify`. Original BLOCK report and reproduction remain in `multiwan-monitor-review-R1.md` and `multiwan-monitor-review-R1-repro.txt` (original report commit `6a784785`). No product edits by reviewer.

**R1-1 resolved.** `state.wanSaved` is an immutable clone initialized from recovered state and advanced only after successful authoritative `WriteAtomic`. A mirror-only error occurs after publication and therefore does not incorrectly hide durable configuration. The watcher reads groups and interface identity from this snapshot under the transaction lock. The generation-specific probe closure captures the same immutable interface mapping; older draining workers cannot switch to a newer or unpersisted device mapping. Broader DEGRADED Apply behavior remains unchanged.

Independent focused race execution on pinned Go1.26.0 (`GOTOOLCHAIN=local`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`, private reviewer build cache):

```text
go -C apps/agent test -race ./internal/multiwan ./internal/agent -run 'Test(Runtime|HTTPProbe|DNSProbe|ICMPProbe|DeviceProbe|Wan|ReviewWAN|StateCrashInjection)' -count=3
ok  ngfw/agent/internal/multiwan 2.757s
ok  ngfw/agent/internal/agent 7.457s
```

This includes the exact original failed-save activation regression (previously failed), failed replacement/removal snapshot preservation, interface snapshot isolation, successful save/removal, durable mirror-error semantics, state reload, confirmed rollback, existing persistence crash tests, generation cancellation, conjunction/hysteresis, device identity invalidation, RPC filtering/owner checks and protocol tests. The aggregate member `Since` correction was inspected and its regression ran in the same suite; R3 supplies specialized contract review.

Verdict: **APPROVE** for bounded monitor-only correctness scope; zero unresolved R1 findings. Unchanged complete hosted quick remains a separate required merge gate and was not duplicated locally. Real interface binding/network namespace/VPP/browser acceptance remains NOT RUN. Forwarding routes, failover/balance, NAT cleanup, dynamic gateways, nondefault VRF and ABF remain code follow-ups; full F-multiwan-host is not complete.
