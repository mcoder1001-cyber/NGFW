# Native certificate independent tests — 2026-10-04

Verdict: PASS for the focused host-independent scope authorized by the manager. Full/hosted quick CI and lab negotiation/packet acceptance were waived/deferred by the owner, not reported passed.

Tested frozen integration: `9e927a97e40bb0e619649ade69d84173f8a64a9e`; tree `c1b1a22a3435197af1fc98ac8575fe88e6028c0b`.
Tester branch: `codex/native-cert-independent-tests-20261004`; isolated worktree `/root/.codex/worktrees/8c19/native-cert-independent-tests`. Owned files: this report only. No product edits, shared VPP mutations, host package installation or daemon restart.

## Actual commands and output

`tools/ci.sh check --base origin/main`:
```
check PASSED (0m12s)
```

`go test -race -count=1 ./internal/desired ./internal/descriptors/ikev2 ./internal/subsystems` from apps/agent:
```
ok  ngfw/agent/internal/desired 34.405s
ok  ngfw/agent/internal/descriptors/ikev2 1.211s
ok  ngfw/agent/internal/subsystems 25.224s
```
`go vet ./internal/desired ./internal/descriptors/ikev2 ./internal/subsystems`: exit 0, no output.

`pnpm exec vitest run src/domains/vpn.test.ts src/semantic/vpn.test.ts` from packages/schema:
```
Test Files  2 passed (2)
Tests  124 passed (124)
```
`pnpm exec vitest run src/features/pki/pki.controller.test.ts src/secrets/secret-delivery.service.test.ts` from apps/api:
```
Test Files  2 passed (2)
Tests  23 passed (23)
```
`pnpm exec vitest run src/domains/vpn/ipsec/IpsecPage.test.tsx src/domains/vpn/pki/PkiActions.test.tsx` from apps/web:
```
Test Files  2 passed (2)
Tests  11 passed (11)
```

`pnpm exec turbo run typecheck --filter=@ngfw/api --filter=@ngfw/web --filter=@ngfw/schema --filter=@ngfw/api-client`:
```
Tasks: 17 successful, 17 total
Cached: 6 cached, 17 total
Time: 2m15.454s
```
This scheduler run regenerated schema, proto, YANG and API client through their normal generators. `git status --porcelain` afterward was empty: generated outputs match the frozen source. Three existing OpenAPI warnings for capture deletion and OIDC redirects were emitted; no typecheck/build error.

## Coverage and limits

| Scenario | Observed | Result |
| --- | --- | --- |
| Explicit peer pin, no remoteCa trust claim, local/peer reference constraints | Schema and semantic suites | PASS |
| Public-only peer leaf import, no private-key output, CA/key options rejected before writes | PKI controller/service unit fixtures | PASS |
| Sealed delivery resolves local key and peer public certificate without inventing a peer key | Secret-delivery unit fixture | PASS |
| Certificate import UI switches and native IPsec form compatibility | UI fixtures | PASS |
| Preflight negative material/identity cases and singleton snapshot/compensation ownership safety | Go race suites, including new native certificate unit files | PASS |
| Generated contract compatibility across consumers | Regeneration clean and typechecks 17/17 | PASS |
| Production sealed-cache scheduler recovery fixture | Independently inspected source; manager executes disposable VPP | NOT EXECUTED by tester |
| Peer negotiation, traffic, reboot, full API database E2E/browser E2E | Outside this focused host-independent run | DEFERRED |

Fresh worktree prerequisite builds completed 8/8. Initial API/UI launches made before prerequisite builds completed failed package-entry resolution for @ngfw/proto and @ngfw/api-client; reruns after normal dependency builds on the final frozen snapshot passed as above. These setup failures were not hidden or treated as product success.

Evidence logs: `/tmp/native-cert-independent-{guard,go,vet,schema,api,ui,typecheck}.log`. No current failure or remaining tester code; next action is manager integration and expected-head merge verification.
