# R1 conditional ACL scope review

APPROVE reviewed source commit `009bdda0dd9be21799832ce33d08f4c7c8990a17` in the isolated gate-fix worktree. Integrated without source changes as local `0bbc78fa`; published source tree checkpoint `d71297c378d9f67c0c7ed12ad1f188bbe267138a` additionally includes already reviewed lint comments and lab-boundary documentation.

The fix addresses the actual cause: unrelated interface/security transactions previously expanded into ACL deletion scope. Expansion now requires configured ACLs, enabled AutoBlock, runtime entries or prior context needed for cleanup. Apply, DryRun, drift and dynamic planning use consistent effective scopes. Requested domains, stored desired state and persisted Managed metadata remain unchanged. Explicit ACL authority still rejects deleting ACLs required by ABF policies. Original PBR Apply, idempotency, rollback and restart assertions remain intact.

Independent read-only final verification by correctness_review:

```text
go test -race -count=1 ./internal/agent ./internal/subsystems -run 'Test(InterfaceScope|AutoBlock|GlobalBlocking|DynamicSource|PBR|RpfAdlPbr|ACLBridge)'
ok ngfw/agent/internal/agent 4.803s
ok ngfw/agent/internal/subsystems 1.756s

go test -race -count=1 ./internal/agent -run '^TestDisabledAutoBlockDoesNotBroadenUnrelatedInterfaceScope$'
ok ngfw/agent/internal/agent 1.173s
```

No unresolved correctness findings. Complete hosted quick remains required before merge; these scoped tests are not an aggregate gate or packet acceptance.
