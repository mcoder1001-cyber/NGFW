# Dynamic WAN gateway handoff — R4 review

Reviewed local source `ff54e40d59b981dd4309a21359676d0cf86ccbfc` against `417e8fc`.

## Findings

**MAJOR — retired-address retries can delete replacement sessions.** `apps/agent/internal/agent/rpc_wan.go:152–159`: a withdrawn/renumbered DHCP address is retained in `pendingDead` across cleanup error or partial scan. If that address becomes active again in the same committed generation, the queue is not reconciled with current forwarding state before `ClearDeadSessions`. That cleaner selects sessions by outside address and VRF, without a lease-generation identity, and can therefore delete newly established replacement sessions. Remove reactivated addresses from pending cleanup (resetting scan progress), or otherwise qualify deletion to the retired generation. Add a focused cleanup-failure/lease-rebind regression.

Other reviewed source boundaries are sound: complete lease dumps admit only owned configured DHCP clients in BOUND state; missing/error observations withdraw the complete snapshot; interface/NAT/VRF identity and WAN groups fence stale observations; expiration withdraws paths; projections detach the document and avoid persisting learned addresses. Route and NAT descriptors retain existing owner/claim restrictions. PPPoE intentionally remains unavailable without a verified carrier, avoiding ordinary IP forwarding on its physical WAN.

## Independent verification

Executed from `apps/agent` using the available local toolchain and dependency cache:

```text
GOMODCACHE=/workspace/scratch/9baf7442ffbf/toolchain/module-cache GOCACHE=/workspace/scratch/9baf7442ffbf/toolchain/build-cache GOPROXY=off /workspace/scratch/9baf7442ffbf/toolchain/go/bin/go test -race -count=1 ./internal/multiwan ./internal/agent -run 'TestLearnedGateway|TestDynamicUnbound|TestRetiredLease|TestWANGateway|TestWANPBRRuntime'
ok ngfw/agent/internal/multiwan 1.038s
ok ngfw/agent/internal/agent 1.121s
```

`git diff --check` completed with no output. No full CI or host integration was run. Focused passes do not cover the retry/rebind finding. Whole WAN source completion remains dependent on PPPoE carrier work.

## Final correction recheck

Rechecked local `a28890e757431020348e6d734624b585f4c2ad2c`, tree `84cc234da5bc637423cc2dff1ba982f643bfa8ee`. The MAJOR finding is resolved. A separate retired-address queue allows reassigned addresses to be protected even before health hysteresis recovers. `PruneLiveCleanup` runs under transaction exclusion immediately before deletion, also cancelling ordinary dead-link cleanup for healthy recovered members. Pruning resets scan progress. The focused failure/rebind control retains a replacement session and deletes only the unrelated dead member's session; a separate healthy-recovery assertion covers ordinary queued cleanup.

Independent focused race command above was rerun with `TestCleanupRetryProtectsReboundLeaseSessions` added to the selection:

```text
ok ngfw/agent/internal/multiwan 1.038s
ok ngfw/agent/internal/agent 1.131s
```

No remaining R4 source finding. No full CI, packet forwarding or whole WAN acceptance pass is claimed. PPPoE carrier remains explicit unfinished source work.

Verdict: **APPROVE** for the reviewed DHCP handoff source scope.
