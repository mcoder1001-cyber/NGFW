# Notifications bounded slice — independent R5 review

Frozen product local `80c6fcd9` / remote PR66 `d2e81e64`, base `c76774e8`; isolated branch `review/notifications-r4-r5`. Product files unchanged by reviewer.

Findings: no R5 blockers or majors.

- Schema caps 32 channels, 64 rules, 32 channel references per rule and 32 recipients per email. Dispatcher queue is capped at 256 plus one active job, history at 500, dedup at 2048. Throttle keys are bounded by configured rules/test channels and pruned on reload. Scalar payload fields and schema strings have length limits.
- Event matching scans bounded rules/channel arrays. Queue scheduling/revalidation and state-history copies operate on the explicit caps. There is no new bulk VPP dump, per-packet loop, database table load or event-driven parallel network fanout.
- A single worker owns delivery. At most three attempts have 1s/2s retry backoff; test sends have a 10s per-channel throttle. Reload uses one outstanding datastore read plus a latest follow-up even under commit storms. Configuration retries wait for the failed read to settle and back off 1s.
- Shutdown unsubscribes both buses, clears timers/queue and aborts the active delivery. Config replacement aborts active delivery and queued jobs are revalidated. SMTP owns and destroys its socket, closes transport, removes abort listeners; webhook responses are destroyed without buffering bodies.
- DNS/secret datastore calls are not cancellable and may keep the one worker occupied; the watchdog reports the fault without spawning more workers. This existing documented operability limit is bounded, not hidden as successful timeout recovery. Queue/history loss on restart is also explicitly documented.

Independent focused execution on final source:

```text
node node_modules/vitest/vitest.mjs run src/features/notifications/notifications.test.ts src/features/notifications/transport.test.ts --maxWorkers=2
transport.test.ts 26 passed
notifications.test.ts 31 passed
Test Files 2 passed (2)
Tests 57 passed (57)
Duration 4.39s
```

Evidence includes 100-event throttle, 1000-source queue cap, 600-delivery history cap, bounded retry, blocked-worker serialization, 1000-reload singleflight, shutdown late-read handling, failed-read recovery and socket cancellation. This is deterministic unit evidence, not a throughput benchmark or receiver/lab acceptance. No full gate was rerun locally.

Verdict: **APPROVE** for R5 bounded scope. Missing management VRF/IPsec functionality and real receiver acceptance remain explicit follow-ups; unchanged full hosted quick remains required before merge.
