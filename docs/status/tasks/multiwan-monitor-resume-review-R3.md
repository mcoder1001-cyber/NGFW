# Multi-WAN monitor resume — R3 contracts/API review

Product reviewed: local `4a7b0b50eba79f79c199c4b0d423cd9312efbf75`, tree `b7c62b157300d9f588a2c5784685480eb1044004`, against `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`. Manager identifies remote PR 65 checkpoint as `987c2c9f`; this reviewer inspected the local product. Independent branch `review/wan-monitor-r3-20261002`, worktree `/workspace/scratch/de92de7d9874/NGFW-wan-monitor-r3`. No product changes.

## Findings

### MAJOR — member `since` reports monitor changes even when member health does not change

Locations: `apps/agent/internal/multiwan/runtime.go:191-192,236-237`; existing contract `packages/proto/vrx/v1/dataplane.proto:7571-7572` (`WanMemberState.since`, field 7).

The existing field means when **this member** last changed up/down state and must remain unset if never. Runtime stores timestamps per monitor and selects their maximum for the member. For a member with two monitors, keep one failing continuously and allow the other to transition up. The aggregate member remains down throughout, but the RPC now exposes a non-null `since` despite no member transition. Later partial monitor recovery similarly resets the apparent member downtime. The existing REST controller forwards the incorrect timestamp unchanged.

Fix: maintain aggregate member up/down state and a member transition timestamp under the runtime lock; update that timestamp only when the aggregate AND health changes. Preserve nil before any aggregate transition. Add coverage for one failed monitor plus another changing state, and for full recovery followed by partial failure/recovery. Do not change the existing proto field meaning to fit the implementation.

Actual focused reproduction used a temporary reviewer-only `TestR3MemberSinceContract` in the isolated multiwan package (removed afterwards). A runtime had one member and two HTTP monitors, `UpAfter=DownAfter=1`, interval 100 ms, timeout 50 ms, loss threshold 100. Probe returned `{Sent:1}` for target `failing` and `{Sent:1,Received:1}` for target `passing`. After `Replace`, the test polled snapshots for one second and failed if the never-up member exposed non-nil `Since`.

```text
go test -count=1 ./internal/multiwan -run TestR3MemberSinceContract
--- FAIL: TestR3MemberSinceContract (0.00s)
    r3_contract_probe_test.go:16: contract violation: member has never transitioned up, but since=2026-10-02 16:25:18.349207949 +0000 UTC
FAIL
FAIL ngfw/agent/internal/multiwan 0.008s
FAIL
```

No additional R3 blocker identified. Manager notified for the existing developer to correct; reviewer did not implement the fix.

## Other contract checks

- `git diff --name-only 31355cef..HEAD -- packages/schema packages/proto apps/agent/gen packages/api-client` produced no output. Existing `WanState` RPC is implemented without changing fields, numbers, generated code or endpoint shapes; no new contract generation is necessary for this slice.
- Owner validation reuses `Service.checkOwner`: omitted owner means this agent; foreign owner is InvalidArgument. Context cancellation is converted to gRPC status. Unwired runtime returns Unavailable. Unknown requested groups return NotFound; duplicate filters deduplicate; empty filter means all.
- Snapshots sort groups by name and members by interface and return independent messages. Weight/priority come from config; latency units remain milliseconds and loss remains percent. Initial unobserved members are down.
- `Active` intentionally remains empty because this slice installs no route; reporting a selected healthy member as an installed default route would be false. The owner-approved monitor-only envelope, new user documentation status notice and projection warning explicitly defer forwarding, NAT cleanup, dynamic gateway, nondefault VRF and ABF. This is not full F-multiwan-host acceptance.
- Existing API/client code forwards fields and catches unavailable/older agents into its established `agentError` envelope. No new HTTP validation or pagination contract is introduced. No stored schema migration or secret-handling change.
- R1's independently reported durable-state/persistence bug is being fixed separately. This report does not duplicate that review or approve its current behavior; any resulting watcher delta needs the applicable verification.

## Verification scope

Read repository AGENTS, shared context/contribution/decision rules, R3/shared review instructions, feature prompt, recovery envelope/WIP and inherited runtime WIP. Reviewed runtime/RPC against existing schema, protobuf and API/controller/client behavior.

In `apps/agent`, pinned Go toolchain, existing read-only module cache, writable `/tmp/dashboard-r2-gocache`, `GOTOOLCHAIN=local`, `GOFLAGS='-p=2 -buildvcs=false'`:

```text
go test -count=1 ./internal/agent -run '^TestWanRPCObservedStateFilteringAndOwner$'
ok ngfw/agent/internal/agent 0.018s
```

This focused RPC test covers owner/filter errors, unavailable runtime, observed units and empty active route. The failing reproduction above is separate, not part of the committed product tests. No full quick duplicate, live binding/network namespace/VPP/browser acceptance, forwarding or production readiness claim. Hosted quick and other mandatory reviewers remain required.

**Verdict: BLOCK** pending the MAJOR member timestamp correction (or explicitly accepted owner/date tech-debt row under the review policy). Recheck only the correction and relevant integration delta.
