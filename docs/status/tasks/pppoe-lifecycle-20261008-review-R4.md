# PPPoE lifecycle — R4 review

Reviewed source: `86e02052b1ca7e4223d00e7e05313493f98a09f5` (tree `043b9fd22b92187c0a64b54b8f79b673990418ea`) against `4d4723f`; includes the earlier source fixes and regressions in `955b6b8` and `668a324`.

## Findings

No blocking R4 source finding in the committed change.

The persisted admission fence is established before helper shutdown, including the no-PID case. Restart/removal paths stop old units before deleting state or reopening admission. Rotation rejects hooks already queued with old admission, while removal leaves the fence intact. The refresher inherits a parent pidfd and observes the original process lifetime rather than repeatedly accepting a recycled numeric PID. Refresher and DHCP-child shutdown retains the existing identity checks and pidfd-only signalling; invalid evidence aborts edits. The committed missing-helper branch additionally rejects existing or unreadable process evidence instead of reporting a successful stop and erasing it.

Product unit operations remain restricted to the globals owner. Slot Apply uses private renderer paths, and new tests substitute private paths for sysctls, address observation and DHCP process execution. They do not start host daemon units, modify host sysctls, or create VPP objects. Child processes are explicitly owned and fixture cleanup calls the fenced shutdown routine.

The final `TestIPv6MissingHelperPreservesProcessEvidence` regression removes only a private helper, verifies shutdown rejects the missing helper without deleting its live writer's evidence, and restores the helper before fixture cleanup. Source review of that addition is approved; it has not been executed by this reviewer.

Final recovery recheck: pending transition inventory is established before shutdown and retained on file/reload/start failure. It forces identical retries through stop, replacement, reload and startup, including already-deleted unit files. Pending acknowledgement follows successful restart. Removed-session admission and fence retirement follows verified shutdown, unit stop, file removal and successful reload; missing admission rejects queued hooks, and host recreation allocates a fresh token. Departed-parent up hooks leave replacement state alone, and down hooks reject a mismatched recorded parent. New controls use temporary file trees and recording unit runners. The final inventory change propagates unreadable pending/unit directories as errors before destructive mutation, preserving the fail-closed boundary.

## Verification limits

Independent source review only. `git diff --check` completed with no output on the final source tree. `go` is unavailable in the reviewer environment; no compiled/race or packet-level acceptance pass is claimed. The unchanged mandatory quick gate and lifecycle tests require successful execution evidence before merge. This repair does not establish complete PPPoE feature acceptance. Review is of local source; no publication or remote-tree parity is claimed.

Verdict: **APPROVE** (source process ownership and shared-host safety; execution gates remain required).
