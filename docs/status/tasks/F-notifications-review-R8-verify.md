# F-notifications R8 correction verification

Original BLOCK report `F-notifications-review-R8.md` / report commit `739757c5` is preserved. Independently reviewed worker corrections `70cc4961680818d514575973c6e4261c33ecc32f`, `f0e1dd11` and `6e24bb0875b1381f3707d23cc94e44f626f289f0` in isolated reviewer worktree `/workspace/scratch/de92de7d9874/notifications-r8`, cherry-picked without modification (local verification HEAD `c1c1a445`). Documentation correction inspected at `87784db6`: existing strongSwan producer versus missing notification adapter is now accurately distinguished. Reviewer made no product changes.

## Blocking findings resolved

1. Successful configuration reload now clears the old runtime error before `configure()` wakes the queue. Worker failed-read/recovery regression passed independently. An additional independent fake-timer reproduction held the first of two deliveries, let the configuration-read watchdog expire at 5s, completed the first delivery while degraded, then released the configuration read. The remaining job drained without another event/test/reload: error=null, queued=0, delivery calls=2.

2. SMTP transport now owns the pinned TCP socket through nodemailer `getSocket`; abort and finalization destroy that socket. It retains original SMTP hostname for TLS validation, `rejectUnauthorized:true`, and required STARTTLS. The raw socket has an error listener through TLS handoff. Actual nodemailer (not a mocked transport) was exercised with controlled DNS/socket creation: abort before connection and after connection while awaiting SMTP greeting both rejected with `delivery-timeout` and destroyed the socket.

A separate local TLS test exercised the actual nodemailer TLS path: generated temporary certificate trusted through `NODE_EXTRA_CA_CERTS`, SAN `relay.example.com`; ephemeral loopback TLS fixture; mock DNS supplied permitted private relay address and socket creation redirected only that connection to the fixture. After actual validated TLS handshake, abort rejected with `delivery-timeout`; fixture server closed cleanly, with no unhandled error. No external SMTP/network request was made and no credentials were used. Temporary fixture key/cert and untracked reviewer tests were not committed.

## Commands and observed results

From reviewer `apps/api`:

```text
node node_modules/vitest/vitest.mjs run src/features/notifications/r8-watchdog.test.ts src/features/notifications/r8-cancellation.test.ts src/features/notifications/notifications.test.ts src/features/notifications/transport.test.ts
Test Files 4 passed (4)
Tests 60 passed (60)
Duration 5.10s
```

This includes unchanged focused feature suites (31 dispatcher + 26 transport tests) and three independent reviewer tests.

```text
NODE_EXTRA_CA_CERTS=/tmp/r8-notification-cert.pem node node_modules/vitest/vitest.mjs run src/features/notifications/r8-tls-cancellation.test.ts
Test Files 1 passed (1)
Tests 1 passed (1)
Duration 1.32s
```

Initial ordinary-sandbox TLS-fixture attempt failed on `listen EPERM 127.0.0.1` and hit its test timeout; the authorized localhost-only escalated rerun above passed. This is recorded as environment failure followed by successful execution, not silently counted as a pass.

## Scope and remaining limits

The optional original MINOR (no queue-drop counter) remains. Missing non-default management VRF binding and IPsec notification adapter remain explicit product limitations. In-memory restart loss and potentially stuck singleflight DNS/secret reads are unchanged. This review does not claim full feature completion, actual SMTP message acceptance, HTTPS receiver acceptance or lab routing/restart verification. It does establish actual TLS cancellation after handshake with the installed dependency.

No duplicate complete quick gate was run. Manager must run unchanged hosted quick on the final integrated tree, with all applicable panel reviews.

Verdict: **APPROVE** for R8, superseding the original BLOCK once all three reviewed correction commits (or byte-identical equivalents) are included.
