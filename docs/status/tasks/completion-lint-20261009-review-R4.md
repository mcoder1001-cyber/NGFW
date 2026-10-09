# R4 final campaign correction review

Verdict: **APPROVE** the security and shared-host scope of lint correction `09bc6cf9d22e79e9b8026428014b64b93012d693` and constructor-only inventory injection delta `d7e0970e0b8ce10bd9e24eaf2f81a740c5500d70`. This is a delta review against hosted candidate `69e288596de0167c9921e4808530a99bc4c88302`, not a statement that its failed CI passed or that later unrelated deltas were reviewed here.

The original hosted Go stage reported 62 lint findings. The reviewer inspected its retained log and applied both author deltas in a separate checkout. No product code was authored by the reviewer.

## Security corrections

- The signed VTR enum is explicitly rejected if negative before conversion to uint32. All remaining int32 values fit uint32; the existing finite POP operation producer remains unchanged.
- Both valid and preferred delegated lifetimes now have explicit positive uint32 bounds before conversion. Existing preferred-before-valid validation and conservative 30-second rounding remain intact.
- The recovery fixture uses a uint64 sequence directly, removing its signed conversion without altering product allocation.
- Resolver preparation returns a named error and joins deferred file-close failure with any previous error. O_NOFOLLOW, regular-file and single-inode checks still precede mode change and truncation. No close failure is silently suppressed.
- The three product G304 annotations are attached only to fixed resolver basename, finite packaged hook names, and validated carrier-derived state filenames. Product roots and validated session construction were traced; no request supplies a path or executable. The other three annotations document a required non-production credential marker and private test-directory fixture writes. They are line/rule scoped, not package/global exclusions.
- Fixture file/directory modes are tightened. Remaining changes are comments, local shadow-name cleanup and removal of an unused forwarding wrapper. Linter configuration, CI scripts, workflows and agent Makefile are unchanged by these deltas.

## Inventory injection

The optional function is a Go construction parameter in internal `subsystems.Env`, passed only to descriptor inventory. Repository search found assignments only in three test wiring constructors. Nil still calls the real fixed broker for inventory, including empty desired PPP state and orphan/revocation discovery. It introduces no environment, CLI, API or persisted configuration bypass. Provision/remove and the actual runtime broker adapter remain unchanged. Real carrier safety tests continue using their broker/ownership fixtures rather than the empty generic test inventory.

## Independent checks

Go1.26, offline local caches, race enabled, count one, from `apps/agent`:

```text
go test -race -count=1 ./internal/descriptors/pppoe ./internal/desired ./internal/subsystems -run 'TestCarrierVLAN|TestPppoeDelegation|TestCarrierResolverPrivateFile|TestCarrierForwardingProduct'
ok ngfw/agent/internal/descriptors/pppoe 1.054s
ok ngfw/agent/internal/desired 1.216s
ok ngfw/agent/internal/subsystems 1.359s

go test -race -count=1 ./internal/subsystems -run 'TestCarrierBrokerRejectsMismatchedReceipt|TestCarrierRegistrationKeepsRATapOwnershipSeparate|TestRegisterGuardsEveryDescriptor|TestRAScopedOwnershipNormalApplyAndRollbackRemainDisjoint'
ok ngfw/agent/internal/subsystems 1.191s
```

Both commands completed with no skips. `git diff --check` passed. Pinned full linter execution and the remaining hosted campaign are author/manager gates and were still running when this source review was recorded. No native service/namespace/packet operation was activated. No unresolved R4 blocker or major finding was found in these two deltas.
