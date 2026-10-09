# PPPoE delegated LAN R1 correctness and tests

Product source reviewed: local `fcb90b1f459017932b92ad4ce04c99cacc5d99ee`, matching published source `912c658c6660c8a7570c35df2543922770e391d8` (author receipt). Claim-test checkpoint `355163d991657bad5fe92d2fdd2ff9fe2508082a`. Enhanced scheduler test bytes independently tested: SHA256 `2add5072950701d15befdf459bd4fc3bf97c964143c3fc48ef8f03c35b3f0a20`; final containing local commit `2e736187`, matching published `94dd307898ce8df2d8a37c2024fe6d4c1ab9ecc7`, reported remote tree `6e1f5fa0c1a9a80e66e7f7fcee1549f27e814030`; independently observed local tree `26173b9c9ee68a9d4b385a137d050451f665ded4` (author publication receipt).

## Findings

No remaining source correctness finding in the scoped PD foundation. Explicit target allocation validates whole assignments and stable relative /64 selection. Desired projection withdraws invalid/unready/expired/deprecated plans, rejects static IPv6/RA and cross-WAN overlap, and rounds RA lifetimes down without extending lease deadlines. DHCP events must match current refresher generation and current admission; invalid renewal removes the prior lease.

Dynamic address, prefix and RA descriptor instances retain distinct stable scheduler keys while sharing the existing proven VPP operations. Persistent PD ownership partitions static Create/Retrieve from dynamic ownership. Creation refuses preexisting static state, claims before mutation, supplies partial-create rollback authority after uncertain/observed mutation, and releases ownership only after verified deletion. Dependencies order dynamic address before prefix before RA. No generated VPP bindings were changed.

Resolved test-evidence gap: initial scheduler renewal-failure test asserted only the restored address despite describing all LAN objects. Enhanced test now retrieves and compares the complete original address, prefix/lifetimes and RA values before retry. Static prefix/config adoption refusal tests additionally verify preexisting state remains intact. These enhanced tests passed independently.

## Independent focused validation

All commands used Go 1.26.0 with race detection, count 1, readonly modules and bounded build parallelism.

```sh
go -C apps/agent test -race -count=1 -v -run '^(TestPppoeDelegation.*|TestAllocateDelegation.*)$' ./internal/desired ./internal/renderers/pppoe ./internal/subsystems
```

Observed exit 0, no skips: desired lease lifecycle, static/cross-WAN overlap, lifetime budgets: PASS, 1.112s; allocation containment/duplicate rejection/renewal: PASS, 1.017s; scheduler lifecycle and static-address adoption refusal: PASS, 1.112s.

```sh
go -C apps/agent test -race -count=1 -v -run '^(TestDHCP6ScriptRecordsDelegatedPrefix|TestReadIPv6DelegationExpiry|TestRaClaimFailurePrecedesMutation|TestRaPrefixPartialFailureRetainsRollbackOwnership|TestRaConfigPartialFailureRetainsRollbackOwnership)$' ./internal/renderers/pppoe ./internal/descriptors/ip6_nd
```

Observed exit 0, no skips: actual private DHCP event/admission and expiry controls PASS, 1.385s; claim-before-mutation and uncertain prefix/config rollback controls PASS, 1.016s.

```sh
go -C apps/agent test -race -count=1 -v -run '^TestPppoeDelegation(ProductLifecycle|RefusesStaticAdoption|RefusesStaticRAAdoption)$' ./internal/subsystems
```

Enhanced scheduler/static-address/static-RA controls: PASS, exit 0, no skips, 1.152s. Scheduler uses stateful fake VPP through actual descriptors and durable ledgers, not native VPP acceptance. Independent `git diff --check`: PASS. No aggregate CI or native host actions performed.

## Required combined acceptance

This branch supplies a foundation and fail-closed seam. Production registration of `registerPppoeDelegation(reg, rt.DelegationSnapshot)`, the actual `CarrierDelegationReady` method, verified exact-admission carrier forwarding readback, and public target validation remain parent integration work. Standalone snapshot returns no assignments when that integration is absent. Current scheduler tests explicitly register the dynamic descriptors and project leases; they do not prove the public combined registration path. Require a combined wiring/readiness test covering loss, generation rotation, renewal, rollback and restart before claiming complete PD functionality. Native DHCPv6/RA packet acceptance and final unchanged aggregate gate remain separately required.

A failed VPP deletion may retain an owned object under the dynamic retry/quarantine policy. The code reports/retries failure; this review does not claim instantaneous withdrawal on a rejected operation.

Verdict: APPROVE scoped source and independently executed PD foundation controls. Whole-feature acceptance remains pending actual combined parent wiring/readiness and required final/native checks.

## Operational assembly exclusion verification

Author identified an additional assembly correctness gap: type-based static assembly could consume dynamic PD address/RA values supplied by all-descriptor retrieval. Narrow guards now exclude the three PD descriptor identities in interface and RA assembly, retaining ordinary static paths. `TestPppoeDelegationDoesNotBecomeStaticConfiguration` supplies a real LAN alias plus dynamic address/prefix/config KVs and verifies no runtime address or RA becomes static configuration. An intermediate new fixture failed compilation due to an invalid `Live` implementation; the author corrected the fixture, and independent focused execution subsequently passed.

```sh
go -C apps/agent test -race -count=1 -v -run '^TestPppoeDelegationDoesNotBecomeStaticConfiguration$' ./internal/desired
```

Actual output, exit 0:

```text
=== RUN   TestPppoeDelegationDoesNotBecomeStaticConfiguration
--- PASS: TestPppoeDelegationDoesNotBecomeStaticConfiguration (0.00s)
PASS
ok  ngfw/agent/internal/desired 1.069s
```

Scoped source review includes these assembly guards; final frozen local commit `2e736187`, matching published `94dd307898ce8df2d8a37c2024fe6d4c1ab9ecc7`, reported remote tree `6e1f5fa0c1a9a80e66e7f7fcee1549f27e814030`; independently observed local tree `26173b9c9ee68a9d4b385a137d050451f665ded4`. Independent final product-source diff check clean; previously tested enhanced scheduler-test bytes remained identical. Combined parent session-renderer/readiness/registration integration remains required as described above.

Exact product/tree publication correspondence must be verified by the manager before integration; the review independently exercised the recorded local source and test bytes.
