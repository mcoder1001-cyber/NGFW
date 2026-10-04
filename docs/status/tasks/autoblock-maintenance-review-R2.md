# AutoBlock maintenance — R2 security review

Reviewed frozen head: `7a98a307fdab4ad4eb1a84b773114c2c764ba9ca`; baseline `009bdda0dd9be21799832ce33d08f4c7c8990a17`. Independent read-only product review.

Findings: no remaining BLOCKER, MAJOR, MINOR or NIT security findings.

The inactive heartbeat was a product authority bug, not merely unrelated test events: a blank initial fingerprint caused authoritative ACL reconciliation without enabled configuration or runtime state. The new guard executes under the service transaction lock and skips only when AutoBlock is disabled/unconfigured, the snapshot is empty and the runtime is clean. Thus unrelated owner ACLs survive idle maintenance. Dirty empty snapshots still clear stale overlay state; retained entries still reconcile; enabled protection retains expiry, recovery and fingerprint checks. No ownership/auth boundary changed. The original requested-resync event assertions were not filtered or weakened.

Executed verification on frozen product content:

```text
cd /workspace/scratch/e4f791ef53f7/gate-fix/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/agent -run '^Test(InactiveAutoBlockMaintenancePreservesExternalACL|DirtyEmptyAutoBlockMaintenanceClearsStaleOverlay|AutoBlockRuntimeLifecycleOnFake)$'
ok ngfw/agent/internal/agent 3.725s
/workspace/scratch/e4f791ef53f7/go/bin/go test -count=1 ./internal/agent -run '^TestActiveAutoBlockMaintenanceExpiresCachedEntries$'
ok ngfw/agent/internal/agent 1.051s
```

The active expiry regression verifies both VPP ACL and host enforcement disappear at expiry after a real watcher tick. Original TestRequestedResyncsAreRateLimited could not execute locally: Unix socket creation failed with operation not permitted at seams_test.go:1069, before behavior. Hosted execution remains required; no full CI pass or live host acceptance is claimed. Source/test delta inspected for secrets and injection: no new external commands, dependencies, sensitive material or public contract changes.

Verdict: **APPROVE**.
