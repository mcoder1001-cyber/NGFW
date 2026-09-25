# TD-4 — WIP (worker, slot 8)

Started 2026-09-24 15:35 (+0330), branch `task/TD-4` from `task/TD-2@4391a44` (speculative, D-114). Usage-limit stop at 16:40
(salvage `fad03d3`); continued 16:52.

## Done (17:45) — see TD-4.md
- (1) `apps/api/src/auth/transport.ts` (`isLoopback`, `secureTransport`, `tlsRequired`); users imports it; login checks it first.
- (2) `ApiKeyBody.current`; step-up in `AuthService.createApiKey` (+ `checkCurrent`); audit `via`; OpenAPI 403 texts.
- (3) `PasswordReset.reasons`; pg + memory `syncUsers` bump on a disable flip; `configResets` writes `config.user-disabled`.
- Unit: `transport.test.ts`, 3 new `commit.service.test.ts` cases; CLI `TestAPIKeyCreateStepUp`.
- e2e `td4-auth-hardening.e2e.test.ts` (10 tests); `current` in the JWT key bodies of auth/td2/td2-review/td2-verify.
- CLI `apiKeyCreate`, REPL e2e prompt, live.sh, SDK doc. Contract commit (api-client), regenerated operations_gen.go and the Python SDK.
- Whole API e2e on slot 8: 71 passed, 3 skipped. Negative control for (3): old access token 200 with TD-2's `syncUsers`.
- Generated outputs clean. CLI REPL e2e: the TD-4 test passes (P13's review test red since TD-2, questions 7).
- main merged after TD-2 landed (`e16b0f0`); `TMPDIR=/tmp/g-w8 tools/ci.sh --base main` → CI GATE PASSED.
- Slot clean: no processes, no vrx_w8, 0 Valkey keys, build outputs deleted.

## Next
- Nothing left. Manager: review and merge.
