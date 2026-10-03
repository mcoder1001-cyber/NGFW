# F-management HTTPS stream recovery WIP

Branch codex/management-resume-parallel-20261003; base origin/main 19aa88a5.
Owned paths: apps/api/src/features/mgmt-tls/**; docs/user/system/management.md; docs/status/tasks/F-management-parallel-*.md.
Implemented HTTPS upgrade delegation to existing Fastify websocket handlers; await listener binding and propagate bind errors. Regression exercises real TLS/Fastify stream route with credential rejection and relay delivery. No security boundary decision.
Checks: tools/ci.sh check --base origin/main: check PASSED (0m12s).
First focused run could not resolve unbuilt @ngfw/schema; dependency build in progress.
Remaining: execute focused regression and unchanged quick gate; independent review; cert removal listener retains cert, no listener creation after empty bootstrap, state inaccuracies/racing reload remain code gaps; PostgreSQL/lab/browser acceptance NOT RUN.
Next command: pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts
Remote/local SHA: see git HEAD; checkpoint publication follows each commit.
