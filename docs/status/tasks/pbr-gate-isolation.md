# PBR/ACL transaction-scope regression correction

Base integration6eda5e94; branch fix/pbr-gate-isolation-20261004.

## Confirmed cause and behavior

Unchanged main already failed three RPF/PBR fixtures after auto-block introduced
unconditional ACL-family scope expansion whenever interfaces/security was selected.
An interfaces-only transaction with no ACL/overlay context planned deletion of a
same-owner existing ACL used by its PBR policies; the mandatory-dependency check
correctly refused those policies. Isolated RPF tests reproduced this; it was not suite
ordering or shared mutable fake state. TestPBRPolicyNamesFACLList itself passed.

An initial investigation incorrectly called omitted Subsystems full-authoritative;
source inspection of authoritative() disproved that. It selects document-present
domains. Merely making requests explicit cannot fix the unconditional expansion.

The fix makes scopeOf exact and lets projectWithBasePolicy conditionally expand ACL
only when stored/prospective ACL configuration, enabled AutoBlock configuration, or
cached runtime entries needs dependent ACL reconciliation. Both old and new context
are inspected so disabling/removing enforcement still clears it. Disabled AutoBlock
without entries or ACL context does not widen unrelated interfaces scope.

projected.scopeDomains propagates the effective projection scope to Apply, DryRun,
drift planning and active dynamic-source merged planning. Requested/persisted domain
metadata remains unchanged; implicit ACL work does not claim ACL authority afterwards.
Retrieve respects its requested domain scope. No ownership checks, dependencies,
privileges or generated contracts changed.

## Regression coverage

New agent tests prove an unrelated interfaces transaction and DryRun preserve a
preseeded owned external ACL; an inactive AutoBlock setting also preserves it. With
an active runtime overlay, an interface-only update's DryRun/Apply binds blocking to
a newly added interface; removing the AutoBlock field clears all VPP/host bindings.
The implicit expansion does not append ACL to stored managed-domain metadata.

Original unspecified-Subsystems Apply/DryRun behavior, idempotency, invalid input,
restart, rollback and Retrieve assertions remain in the old PBR fixtures. The stale
WithoutFAcl name/helper claimed product lacked F-acl although Register already included
it; it now honestly tests existing ACL references through product registration and
checks that a selected feature transaction preserves the referenced ACL. A separate
explicit ACL-authority omission still fails with dependency errors and no deletion.
The historical ACLBridgeRegistration test remains unchanged. Wiring cleanup now closes
the product fixture's own workers as well as its service.

## Actual checks

Before source fix: full subsystem package `go test -count=2 ./internal/subsystems`
failed RpfAdlPbrOnFake, PolicyNamesPersist, WithoutFAcl and LinuxNetdevKindOnThisHost
twice. `/tmp/pbr-before.log`; isolated RPF failure `/tmp/pbr-isolated.log`.

After source fix: focused PBR/ACL bridge `go test -race -count=2 ./internal/subsystems
-run '^Test(RpfAdlPbr|PBRPolicyNamesFACLList|ACLBridgeRegistration)'` passed1.591s.
Full subsystem package-count2 now fails ONLY LinuxNetdevKindOnThisHost twice:
`netlink RTM_GETLINK: operation not permitted`; `/tmp/pbr-scope-full.log`,21.272s.
This environment failure remains explicit; the unit test/gate was not skipped/weakened.

Focused new scope+runtime agent race-count2 passed2.004s. Independent R1 focused
agent dynamic-source/scope/ACL race passed4.100s; subsystem PBR/ACL bridge passed1.370s.
Final broader agent `go test -race -count=2 ./internal/agent -run
'^Test(InterfaceScope|DisabledAutoBlock|AutoBlock|GlobalBlocking|ACL|Acl)'` passed5.538s;
/tmp/pbr-final-agent.log. Pinned golangci-lint2.13.2 on internal/agent/... returned
`0 issues.`; /tmp/pbr-final-lint.log. Source/security check passed with pinned gitleaks.

No lab acceptance or complete CI GATE PASSED claim. Root will run the unchanged
hosted gate after independent applicable reviews and publication.
