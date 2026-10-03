# R2 security — HTTPS stream recovery

Reviewer /root, independent of management developer.
Reviewed local product SHA `1eb20ff89225201f4376489414637eba3f7d64a3`, base19aa88a5; local tree `f3ea8c0a7c76821bf11da7a5e5e02212a51c6658`. Manager reports remotefa40ba1cd same tree; tree equivalence must remain verified for integration.

No BLOCKER or MAJOR security finding in this bounded delta. HTTPS upgrade delegates the original request, TLS socket and unread head to Fastify's already-registered upgrade listener. That listener uses the same existing stream preValidation/AuthService credential path and RelayService principal, so no unauthenticated second endpoint or duplicate authorization implementation is introduced. No-handler case destroys the socket. Existing production Fastify websocket setup keeps maxPayload64KiB and subprotocol negotiation; those controls reside in app.ts and are reused by delegation. Direct TLS socket retains encryption/remote-peer facts for the established transport policy.

Port/bind environment, routes/credentials/session model, secret lookup and privilege/capability boundaries remain unchanged. New listen promise propagates bind failure rather than claiming bootstrap success. Unit fixtures use generated temporary keys outside the repository and synthetic access strings; no private key or token was added to status/docs. Existing test-only fixed openssl argv exemption remains.

Inspected production app.ts configureApp, telemetry/stream.route.ts and mgmt-tls.service.ts; `git diff --check 19aa88a5..1eb20ff8` exited0 without output. The first independent focus run could not collect tests because the schema package dist was not built; it ran0tests and is not claimed PASS. T1 rerun after prerequisite build is separate evidence.

This review covers stream recovery only. Certificate-removal listener behavior, delayed listener creation, state accuracy and concurrent reload sequencing remain explicit follow-ups; no whole management or lab/browser acceptance is certified.

Verdict: APPROVE

## Sanitized final99 recheck

Local7b57a2849938d7fd361cd3adcf292a89108252f3, tree482ea283bf0cb9d4d606294462c723331715cc62; published7f7b81eb9e8af2b1918da22b98b29883f7418074. Diff versus initial reviewed source changes only test nonce/client pooling/error context and evidence; product service unchanged. Runtime nonce prevents fixed-public-RFC-sample scanner false positive; no security scanner exemption/weakening introduced. Independent focus10/10PASS on frozen final source. Verdict: APPROVE (bounded delta only).
