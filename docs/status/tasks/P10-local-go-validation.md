# P10 local Go validation

Source: 00cb2cd3; unchanged full package tests were executed. These results are not a full gate pass.

Targeted race tests: basepolicy PASS (1.042s), subsystem activation/projection PASS (1.075s).

Full package results:

## internal/agent

205 passing test entries, 14 skipped test entries, 18 failing test entries (subtests included).

| Test | Observed cause |
|---|---|
| `TestAgentStopClosesObjectsRuntimeAndMetrics` | Unix socket listen: operation not permitted. |
| `TestCnatStateOnFake` | Unix socket listen: operation not permitted. |
| `TestConfigReplyTimeoutAndMetricsOptIn` | Unix socket listen: operation not permitted. |
| `TestConnectHookHasItsOwnDeadline` | Unix socket listen: operation not permitted. |
| `TestDet44StateOnFake` | Unix socket listen: operation not permitted. |
| `TestGRPCHandlerPanicAnswersInternal` | Unix socket listen: operation not permitted. |
| `TestGRPCRoundTrip` | Unix socket listen: operation not permitted. |
| `TestInterfaceRemovedDeletesAttributesFirst` | Netlink RTM_GETLINK / netlink socket: operation not permitted. |
| `TestMetricsCollectors` | Unix socket listen: operation not permitted. |
| `TestNatEIAndNat64SessionsOverGRPC` | Unix socket listen: operation not permitted. |
| `TestNatSessionsSummaryKillOverGRPC` | Unix socket listen: operation not permitted. |
| `TestNatVariantWalksShareTheEDWalkSlot` | Unix socket listen: operation not permitted. |
| `TestOwedResyncTakesTheAgentsResyncPath` | Unix socket listen: operation not permitted. |
| `TestRequestedResyncsAreRateLimited` | Unix socket listen: operation not permitted. |
| `TestRunStopsOnCancel` | Unix socket listen: operation not permitted. |
| `TestSocketPermissionsAndInUse` | Unix socket listen: operation not permitted. |
| `TestStartIDRangeFailsClosed` | Unix socket listen: operation not permitted. |
| `TestStartWiresFeatureEventsAndResync` | Unix socket listen: operation not permitted. |

## internal/subsystems

94 passing test entries, 5 skipped test entries, 2 failing test entries (subtests included).

| Test | Observed cause |
|---|---|
| `TestLinuxNetdevKindOnThisHost` | Netlink RTM_GETLINK / netlink socket: operation not permitted. |
| `TestReachabilityTable` | New basepolicy renderer absent from TD-11a reachability metadata. Real integration failure; corrected by adding wired P10 entry and exact TestReachabilityTable rerun PASS (0.133s). |

The environment errors were not bypassed and their tests were not modified. Hosted unchanged full quick on the final integration commit remains mandatory. Reachability metadata was corrected, not marked as an environment failure.
