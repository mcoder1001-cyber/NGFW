# Multi-WAN bounded monitor slice — independent R1 correctness review

Reviewed remote PR65 product `987c2c9f1511b8694c5332783a9fd7eafdae8ae2`, tree `b7c62b157300d9f588a2c5784685480eb1044004`, local equivalent `4a7b0b50`; review worktree starts at documentation follow-up `7e211100`. Branch `review/wan-r1-monitor`. No product changes by reviewer.

## Findings

**BLOCKER R1-1 — watcher activates desired state whose durable save failed.** `apps/agent/internal/agent/rpc_wan.go:62-75` reads `svc.st.desired` and feeds it to `ReplaceWithIdentity`, claiming to observe only durably stored state. `service.go:521,560-565` first merges the new desired state and refreshes interface lookup, then attempts persistence. An authoritative state-write failure leaves that in-memory document installed and changes the response to DEGRADED; it does not restore the old desired document. The new watcher nevertheless starts the uncommitted WAN probes. The same issue can prematurely remove/replace an existing monitor configuration, and a restart restores a different configuration. This violates the explicit durable-commit lifecycle contract even in this monitor-only scope.

Reproduced with the existing state `saveHook`: apply a durable empty routing baseline; inject an error at stage `state`; apply a WAN group and assert DEGRADED; clear the hook; start the watcher. Its snapshot contains the group despite failed persistence. Temporary reviewer test source is preserved as `multiwan-monitor-review-R1-repro.txt` beside this report (copy to `apps/agent/internal/agent/review_wan_durable_test.go` to run). No real VPP or privileged network operation is involved.

Fix: use a coherent immutable WAN desired/interface snapshot published only after successful authoritative persistence (a mirror-only error is already durable), initialize it from recovered durable state, and keep monitor device lookup aligned with that snapshot. Test rejected add/removal/interface replacement, successful commit, rollback, and restart. Avoid changing the established broader DEGRADED data-plane semantics solely for this watcher.

## Checked scope

- Generation replacement cancels and drains old workers, rejects late completions, hides health while draining, and clones/sorts observations.
- Hysteresis is per monitor and ANDed across monitors; unobserved links are down. Identical configuration retains streaks; interface identity changes reset them.
- Owner mismatch, unknown requested group, duplicate requested group, unwired runtime and empty Active behavior have focused tests.
- Successful durable Apply and removal have a real Service test over fake VPP. Their failure-path gap exposed R1-1; this is not laboratory evidence.
- Route installation, forwarding failover/balance, NAT cleanup, dynamic gateway, nondefault VRF and ABF remain implementation follow-ups, not completed lab-only acceptance. This review does not certify those features.

## Verification

Pinned Go 1.26.0, `GOTOOLCHAIN=local`, reviewer cache `/tmp/wan-r1-cache`, `GOMAXPROCS=2`, `GOFLAGS=-p=2`.

```text
go -C apps/agent test -race ./internal/agent -run '^TestReviewWANFailedPersistenceMustNotStart$' -count=1
--- FAIL: TestReviewWANFailedPersistenceMustNotStart (0.03s)
    review_wan_durable_test.go:28: WAN watcher activated a monitor whose desired-state save failed (Apply DEGRADED)
FAIL ngfw/agent/internal/agent 0.070s
```

Existing suite (reviewer's independent execution):

```text
go -C apps/agent test -race ./internal/multiwan ./internal/agent -run 'Test(Runtime|HTTPProbe|DNSProbe|ICMPProbe|DeviceProbe|Wan)' -count=1
ok  ngfw/agent/internal/multiwan 1.209s
ok  ngfw/agent/internal/agent 3.075s
```

The task envelope explicitly reserves unchanged complete hosted quick for the manager and forbids duplicate broad local gates. Full quick and lab acceptance were therefore NOT RUN by R1. This finding is independently reproducible even if the existing quick gate is green.

Verdict: **BLOCK** — 1 BLOCKER; no other graded findings.
