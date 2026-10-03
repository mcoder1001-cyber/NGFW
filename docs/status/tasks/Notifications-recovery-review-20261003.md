# Notifications recovery independent review

Immutable source `3b1508962d50f856be9bea32d9a356afb1f8e203`.
Verdict: APPROVE WITH LIMITS for the bounded configuration-error preservation fix.

Configuration read failures remain in `runtimeError`; delivery timeout has its own
`deliveryTimedOut` latch. State gives configuration errors priority, and emit/test/
wake/run refuse work while either condition or reload is active. The delivery
watchdog cannot overwrite configuration health, and the old worker's finally clears
only its delivery latch. A successful valid current-generation reload is the path
that clears configuration failure. `workerBusy` prevents a second delivery from
overlapping the first, so finally cannot clear another worker's timeout latch.

Regressions use fake timers and explicitly controlled transport promises, cover
both resolve/reject after failed reload and timeout, check preserved error and
blocked queued work, then successful reload recovery. An isolated delivery timeout
also clears after settlement without requiring an unrelated configuration reload.
Cleanup destroys services and restores real timers. No raw provider diagnostics or
secret rendering is added.

Source-only review; no tests run by reviewer. Coordinator owns focused validation.
An abort-ignoring transport can still keep workerBusy set until its promise settles;
this pre-existing transport liveness boundary is not solved or claimed by this fix.
