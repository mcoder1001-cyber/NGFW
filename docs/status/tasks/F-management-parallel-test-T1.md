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

## Exact final typing correction verification
Independent root tested f93bf72f83cc3b3f88b507203c6ebd2932f2580b.
Published e405145f73e1cd681d60dc10b1ea9e6ca641e3cc.
Independent GitHub commit API confirms tree687bd1706f02c72ea8bae8f7c9ba606bb86b6e14.
```text
pnpm --filter @ngfw/api typecheck
$ tsc -p tsconfig.json
(exit0, no errors)
pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
✓ src/features/mgmt-tls/mgmt-tls.test.ts(10tests)606ms
Test Files1passed(1)
Tests10passed(10)
Start at12:24:51
Duration10.00s
git rev-parse HEAD
f93bf72f83cc3b3f88b507203c6ebd2932f2580b
git status --short
(no output)
```
The earlier hosted TS2345 failure is resolved in source and independently reproduced typecheck now passes. Final complete hosted quick remains pending; T1 integration not claimed PASS until that result and final tree checks.
