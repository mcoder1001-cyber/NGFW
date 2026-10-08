# FRR lock cleanup security review R2 — initial round

Reviewed committed source `a24e9dadc2a37c35191290cb41697f2167fe67ee`; developer modification of the root topology harness is pending. Reviewer wrote no source.

**BLOCKER [shared R4] — cleanup without slot ownership.** `apps/agent/internal/agent/ospf_topology_integration_test.go:98–113`. Root harness registers destructive Stop before acquiring the exclusive slot lock. On failed contention, test fatal executes cleanup; Stop may inspect another holder's daemon pidfiles, signal those daemons and remove their paths. The new descriptor-close helper fixes a descriptor leak but does not establish cleanup ownership. Fix: register destructive cleanup only after successful lock acquisition; Stop must refuse unowned cleanup defensively. Test contention leaves holder processes/files intact, then permits reacquisition after legitimate release. R4 independently raised this issue; R2 agrees on process/privilege ownership implications.

Other changed behavior preserves argv-only execution, scoped validated harness paths and existing daemon bindings. No new authentication route, dependency, secret or production privilege assumption. Regression tests use temporary synthetic files and do not start native daemons.

Independent command attempted: `go test -race -count=1 -run 'TestFailedBaseResetRetainsSlotCleanup|TestRootFRRContendedLockClosesDescriptors' ./internal/renderers/frr/frrtest ./internal/agent`.

```text
Go executable unavailable; command did not start.
```

This is no test-pass claim. Hosted quick execution and applicable independent tester evidence remain manager prerequisites. Final SHA/security recheck required after ownership fix.

## Final source recheck

Rechecked local source `88d97a9ca9914d673499b275f7c19752ffe648fe` with clean tracked source. The developer added an early `h.lock == nil` refusal in rootFRR.Stop before every process/filesystem operation. Thus pre-acquisition registered cleanup is harmless; registration need not move when Stop itself enforces ownership. Once successfully acquired, the handle remains available for cleanup through every startup failure. New synthetic contention regression asserts holder marker and pathspace symlink survive unowned Stop. Descriptor-leak and legitimate release/reacquisition regression remains. Original BLOCKER is resolved by source inspection; no new security findings.

Source security verdict: **APPROVE** (0 outstanding BLOCKER, 0 MAJOR, 0 MINOR). This does not claim Go tests ran locally or replace hosted quick/tester acceptance. Any later product-source change requires recheck.
