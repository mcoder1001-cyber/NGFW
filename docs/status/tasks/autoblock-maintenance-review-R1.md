# R1 inactive AutoBlock maintenance review

APPROVE exact source `7a98a307fdab4ad4eb1a84b773114c2c764ba9ca`, integrated unchanged as local `e0d7e819`.

The extra hosted event came from inactive AutoBlock maintenance, not a requested-resync rate-limit failure. Under the service transaction lock, the guard prevents clean, inactive, entry-free runtimes from reconciling unrelated ACLs. Dirty empty cleanup, retained cached entries and enabled expiry maintenance retain the existing path. The original rate-limit test and its assertions are unchanged.

Independent final read-only verification by correctness_review:

```text
GOMAXPROCS=2 go test -race -count=3 ./internal/agent -run 'Test(InactiveAutoBlockMaintenance|DirtyEmptyAutoBlockMaintenance|ActiveAutoBlockMaintenance|AutoBlockRuntimeLifecycle|InterfaceScope|DisabledAutoBlock)'
ok ngfw/agent/internal/agent 16.277s
```

Regressions verify inactive ACL preservation and no reconcile event, dirty-empty cleanup and cached expiry across VPP/host enforcement. Local original rate-limit reproduction stops at Unix socket EPERM before behavioral assertions; the unchanged hosted gate remains mandatory. No unresolved correctness findings; no source files modified during review.
