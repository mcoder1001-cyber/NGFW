# F-ab-upgrade: independent R1 review

Source reviewed: eb9dbc46. Evidence-only reviewer; no product edits.

MAJOR — deploy/upgrade/ngfw-upgrade-health:22–30. After status proves the active root is the pending trial, a missing/nonexecutable probe raises OSError. The inner retry catches only CalledProcessError/TimeoutExpired. OSError reaches the outer unreadable-status handler and exits without rollback/reboot. The failed trial continues running indefinitely rather than triggering the documented bounded health fallback. Catch launch errors in the bounded trial-health loop while keeping status-read failures fail-closed; test FileNotFoundError and PermissionError separately.

Reproducer executed twice with the production script via runpy, mocking only subprocess.run and monotonic clock; status returns active_slot=B,pending_slot=B, probe launch raises the named error. No host commands execute. Actual output:
```text
FileNotFoundError exit 1 rollback False reboot False
PermissionError exit 1 rollback False reboot False
```

Other scope reviewed: signed-byte snapshot before mkfs, member/ownership/path preflight, inactive mount refusal, state invalidation before formatting, shared-state/device validation, actual GRUB env activation/confirm/rollback, pre-migration required unit ordering, preservation of identity/config. Real firmware/database/encrypted-volume acceptance remains explicitly laboratory-deferred; fixture evidence is not real boot proof.

Full independent quick gate: pending corrected frozen source.

## Repair verification

Corrected source be1448118afd246f39b0d6a7db01d2f986448482 only adds inner-loop OSError handling and targeted regression tests. Independent unchanged deploy/upgrade/tests/run.sh:
```text
Ran 17 tests in 12.206s
OK
```
Both probe launch failures now reach timed rollback/reboot; initial status launch failure makes no mutation. The MAJOR is resolved. Full unchanged quick gate on this exact head remains queued; source review has no remaining finding. Final verdict awaits gate output.


Final verdict on be1448118afd246f39b0d6a7db01d2f986448482: APPROVE. Original health-probe MAJOR independently reproduced then repaired; no remaining source finding. Independent unchanged full quick PASS9m49s exit0; real generation and final source status clean. Exact tests/cache/lab limits are in T1 report. Approval does not replace required latest-main D112 integration gate.
