# PPP observation failure withdrawal

- Branch: `codex/pppoe-observation-fix-20261009`; base `13379ae8`.
- Envelope: delegated R4 blocker; exclusive product ownership of `apps/agent/internal/subsystems/pppoe_watch.go` and new `pppoe_observation_failure_test.go`. No other carrier product files changed.
- Invalid IPCP local/peer data is rejected before replacing the remembered mirror. Observation errors revoke carrier and delegated-prefix readiness first, withdraw the old VPP mirror, and independently withdraw the verified kernel carrier policy. Failed mirror removal retains its tracking; the applied identity remains available to retry broker withdrawal even after mirror cleanup succeeds. Joined errors retain the observation and cleanup failures. No daemon output is added.
- Focused race run through `tools/heavy.sh`: `go test -race ./internal/subsystems -run 'TestPppoe(ObservationFailureWithdrawsAndRetries|RenderedHookUpDownWithdrawAndRepair|PartialMirrorFailureIsTrackedAndWithdrawn)' -count=1` PASS, `ok ngfw/agent/internal/subsystems 1.281s`. New malformed/unreadable hook cases test both VPP and kernel cleanup failure/retry and denied delegated-prefix admission.
- An exploratory run also included existing `TestPppoeIPv6MirrorFollowsHookState`; that existing test failed at line139 because its IPv6 summary lacked expected PD. Reported to carrier owner; not changed in this narrow fix, and not claimed passing.
- No full CI or native host activation. Next: independent R4 review of frozen source, carrier owner cherry-pick, combined CI only when all source is ready.
- Remote checkpoint pending connector publication; exact SHA reported to manager after successful publication.
