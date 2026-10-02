# F-notifications independent R8 operability review

Reviewed local commit `97f6863056c630069f48750104ca8d4d8392fe7a`, tree `03d0466129e90c91c00d7ac1cec7f7ed088a5793`, against main `31355cef80b4e8ba3aaa6f95a47c7e7afc055fb6`. Parent associates this product with PR 66 / remote `426b1603`; remote identity not independently checked by this reviewer. Isolated branch `review/notifications-r8`, worktree `/workspace/scratch/de92de7d9874/notifications-r8`. No product edits; focused reproduction only, no duplicate full gate.

## Findings

1. **BLOCKER — configuration recovery leaves queued notifications stranded.** `apps/api/src/features/notifications/notifications.service.ts:128-129,155-156,280-281`. Recovery invokes `configure()` and its `wake()` before clearing the prior `runtimeError`. `wake()` returns because the error is still set. If the old queue timer already fired during the failure, no timer remains after success. State reports healthy (`error: null`, not busy) with queued messages that never drain until an unrelated event/test/config reload wakes them. Clear the error before successful configuration schedules work (or explicitly wake after clearing), with failed-read/recovery and watchdog/recovery regression tests.

2. **BLOCKER — SMTP cancellation does not cancel the active SMTP connection.** `apps/api/src/features/notifications/transport.ts:142-145,155-160`. With `pool:false`, nodemailer 10.0.13 `SMTPTransport.close()` only removes OAuth2 listeners and emits `close`; its `send()` owns a separate local SMTPConnection and does not subscribe it to that close event. The dispatcher calls this ineffective abort on configuration replacement, disable, destruction and ten-second deadline. An email can still be transmitted after abort; repeated SMTP activity also defeats the inactivity-only socket timeout. Use a transport/socket cancellation mechanism that actually tears down the current connection, and test pending send → abort → connection closed/no eventual send. A Promise race without socket teardown is insufficient.

3. **MINOR — saturation losses are not counted or visible.** `apps/api/src/features/notifications/notifications.service.ts:224-228`. The bounded queue silently drops remaining channels/events at capacity, with no history entry or drop counter. Operator can see queue length but cannot establish that notifications were lost. Consider bounded aggregate dropped count / overflow indicator. This is optional for this review, not a request for an unbounded audit stream.

## Evidence actually run

Dependencies were reused by symlinks to the existing notification worktree. The reviewed service source remained at the SHA above.

Temporary independent fake-timer reproduction: configure a channel/rule, emit an alarm, make `getRunning()` reject once then resolve on automatic retry; advance 1100 ms; assert reads=2, state error=null, queued=1, delivery calls=0. This deliberately asserts the faulty behavior; it is not feature acceptance. Temporary test removed after execution.

```text
node node_modules/vitest/vitest.mjs run src/features/notifications/r8-repro.test.ts
Test Files 1 passed (1)
Tests 1 passed (1)
Duration 4.67s
```

Actual installed nodemailer instantiated with `{host:'192.0.2.2',pool:false,secure:true}`; only SMTPConnection prototype connect/send/close replaced with controlled callbacks (no network). Start `sendMail`, invoke transport.close while connect callback pending, then release connect callback:

```text
after transport.close: {"sendCalls":0,"closeCalls":0}
send resolved after close: {"sendCalls":1,"closeCalls":1}
```

The connection was not closed by abort; it sent successfully afterward and closed only on normal send completion. Inspected dependency `nodemailer/dist/cjs/smtp-transport/index.js` lines 141-261 and 362-367.

## Other operability assessment / explicit scope

- Queue 256, history 500, dedup 2048 and channel/rule schema limits bound memory; one delivery and one configuration read are in flight. Retry is limited to three attempts with 1s/2s backoff. Secret database/DNS waits are not abortable; a stuck wait retains one busy worker and exposes a timeout state rather than spawning further workers. This is a remaining availability limitation, not proof of successful timeout recovery.
- Bus listeners are removed, pending timers cleared and queued work discarded on shutdown; in-memory queue/history loss across API restart is explicitly documented. SMTP teardown still has blocker 2.
- Secret resolution uses encrypted references and sanitized errors; delivery history is exposed through state/UI. No provider response logging was found. No new OS dependency, migration, packaging or service unit was introduced; nodemailer is a locked application dependency.
- Non-default management VRF binding is explicitly unimplemented. It is missing product behavior, not a lab-only test; review does not certify complete F-notifications acceptance.
- StrongSwan already has a producer/adapter in `apps/agent/internal/renderers/strongswan/events.go:68-89`: `ToProto()` uses attributes `source=strongswan`, event/state/up/connection/tunnel, with unspecified/error event kind. Notification service only handles link/WireGuard; IPsec notification mapping is missing. Documentation should distinguish this missing integration from absence of an existing producer and dedicated enum. No live end-to-end producer delivery was established.
- Actual SMTP/HTTPS fixture reception, browser acceptance, routing and database restart tests are not claimed. Explicitly deferred lab-only checks are not code blockers. Manager still needs unchanged quick gate on the final corrected integration tree.

Verdict: **BLOCK** — two reproduced product defects; verification required after worker fixes.
