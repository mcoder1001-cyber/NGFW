# PR title

fix(test): align host-service rollback fixture with canonical paths and empty projections

# PR description

The complete local quick gate fails TestHostServicesApplyRetrieveRollback on a host with its real config-check binaries installed. The fixture looks in a temporary directory that newSvc replaces with hostDirOf(t,stateDir). Reuse one stateDir/hostDirOf pair, preserving all render, retrieve, state, idempotence and rollback checks.

Correctly reaching rollback exposes a second outdated expectation: the existing DHCP and QoS projection contracts always emit empty service containers. Require proto.Equal to exactly those canonical empty containers after rollback. DNS/NTP and all other configured services must be absent; unexpected or nonempty configuration still fails. Management=nil and idle Unbound rendering checks remain. This changes only the test fixture, with no production code, tool hiding, new skips, daemon operations or live VPP changes.

Validation: before correction, targeted race test reproduced missing Unbound config (FAIL0.482s); path correction then exposed whole-services=nil mismatch (FAIL0.649s). Final targeted race test passed twice (1.727s/1.902s); related DNS/host-service agent tests passed (1.883s); full subsystems race package passed (22.550s); unchanged check gate passed9s. Raw logs and both failures are preserved in Go-hostservices-wip.md. Complete unchanged local quick is running; unchanged hosted quick is requested on the exact published head and must pass before merge. Independent review required; developer neither self-reviews nor merges.
