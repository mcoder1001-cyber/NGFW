# T1 independent HTTPS stream fixture evidence

Tester /root independent of developer.
Local testedSHA7b57a2849938d7fd361cd3adcf292a89108252f3 tree482ea283bf0cb9d4d606294462c723331715cc62; publishedPR99 head7f7b81eb9e8af2b1918da22b98b29883f7418074.
Worktree work/NGFW-management-resume; tree frozen for this run.

Actual command/output (TLS fixture certificate logs omitted):
```text
pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
RUN v3.2.7 .../NGFW-management-resume/apps/api
✓ src/features/mgmt-tls/mgmt-tls.test.ts (10 tests)608ms
Test Files1passed(1)
Tests10passed(10)
Start at12:07:49
Duration10.41s(transform3.57s, setup0ms, collect8.64s, tests608ms, environment0ms, prepare244ms)
```

Scenario coverage: invalid/missing stream credentials401; valid TLS upgrade101 and relay bytes; request socket encrypted; actual peer test certificate subject; TLS validator/key mismatch/expiry/protocol floor and committed context rotation. Real TLS sockets and Fastify plugin/stream route are exercised with fixture AuthService/relay, not a live database/VPP/browser stack. No lab acceptance claim.

Historical initial independent run at1eb20ff8 collected0tests due unbuilt schema dist; it was not PASS. Developer's intermediate request-client pooling failure was fixed before this final testedSHA and fixture nonce is generated at runtime; production authorization implementation was preserved.

Focused scenario verdict: PASS.
Overall T1 integration verdict: PENDING — unchanged complete hosted quick and missing mandatory fresh panel remain requirements.
