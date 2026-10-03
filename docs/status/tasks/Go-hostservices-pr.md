# PR title

fix(test): align host-service rollback fixture with canonical paths and empty projections

# PR description

The complete local quick gate fails TestHostServicesApplyRetrieveRollback on a host with its real config-check binaries installed. The fixture looks in a temporary directory that newSvc replaces with hostDirOf(t,stateDir). Reuse one stateDir/hostDirOf pair, preserving all render, retrieve, state, idempotence and rollback checks.

Correctly reaching rollback exposes a second outdated expectation: the existing DHCP and QoS projection contracts always emit empty service containers. Require proto.Equal to exactly those canonical empty containers after rollback. DNS/NTP and all other configured services must be absent; unexpected or nonempty configuration still fails. Management=nil and idle Unbound rendering checks remain. This changes only the test fixture, with no production code, tool hiding, new skips, daemon operations or live VPP changes.

Validation: before correction, targeted race test reproduced missing Unbound config (FAIL 0.482s); path correction then exposed whole-services=nil mismatch (FAIL 0.649s). Final targeted race test passed twice (1.727s/1.902s); related DNS/host-service agent tests passed (1.883s); full subsystems race package passed (22.550s); unchanged check gate passed 9s. Raw logs and both failures are preserved in Go-hostservices-wip.md.

The unchanged COMPLETE local quick gate PASSED (exit 0, 10m23s), including all 35 turbo tasks, the full agent lint/race/build stage (agent package 46.385s), CLI checks, all test-module unit/compile/vet checks, and all unchanged VPP shellcheck/fake-host apply-startup shards. Raw log `/tmp/go-hostservices-quick.log`; detailed logs `/root/ngfw-wt/logs/ci/NGFW-go-hostservices-20261003-133014-396580`. No live VPP or real lab integration claim. Unchanged hosted quick is requested on the exact published head and must pass before merge; if main changes, validate the final integration tree. Independent review required; developer neither self-reviews nor merges.
