# Management TLS shutdown tested checkpoint
Branch codex/management-shutdown-20261003; reviewed-parent dependency local eabeb473 (PR100, itself stacked on PR99). Developer owns only declared TLS source/test, user management docs, unique task docs. Initial local envelope checkpoint464c8758; remote publication is coordinator responsibility, not yet verified by developer.
Completed: listener-owned upgraded socket set with removal on close; refuse upgrade callbacks once shutdown starts; destroy owned upgraded sockets after reload queue settles before closing HTTPS listener. Existing authentication/route forwarding, live certificate hot reload and removal retention policy remain intact.
Real test uses production HTTPS listener, real Fastify/WebSocket plugin and existing stream route with isolated auth seam. A valid authenticated101 TLS socket stays live, shutdown waits for a deliberately held secret read, a new valid upgrade is refused before attach, then existing socket closes and listener reports disabled. No realDB/appliance acceptance claim.

Actual commands/results:
- pnpm install --offline --frozen-lockfile: PASS6.1s,595reused,0downloaded; no host package installation.
- pnpm --filter @ngfw/schema build, @ngfw/proto build, @ngfw/yang build: all exit0 prerequisites.
- Before production correction, new focused shutdown fixture against frozen PR100 source: 1FAILED/13excluded by -t, test1.321s, total8.97s; Error live HTTPS upgrade survived shutdown. Finally cleanup closed own client/service/Fastify, no hanging test process.
- pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts: after initial correction14/14PASS8.08s; after heldreload/newupgrade coverage final14/14PASS7.86s, tests718ms, no skips.
- Focused ESLint for changed source/test exit0; inherited root eslint config emits Node module-type warning only.
- Initial APItypecheck correctly found TS2722 at new test callbackrelease. Fixed definite assignment assertion after synchronous promise initializer. Final isolated pnpm --filter @ngfw/api typecheck exit0, no output beyond tsc header; no compiler config changes.
- tools/ci.sh check --base eabeb473fc2b2e51af8ed2c17253e82593cae697: PASS0m09s, gitleaks0; git diff --check exit0.

Not run: full localquick (separate inherited Go fixture failure is under another task), unchanged complete hostedquick, independent applicable R1/R2/R5/R6/R7 panel, DB-backed commit/rollback/browser/appliance tests. None waived. Parent99/100 must integrate before this bounded followup; verify new current-main composed tree and complete gate before any merge. Whole management task remains incomplete.
Exact next action: coordinator publishes this frozen checkpoint, opens reviewable stacked PR, dispatches independent aspects; developer applies actual findings only and never self-reviews/merges.
