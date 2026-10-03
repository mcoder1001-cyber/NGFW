# F-management HTTPS stream recovery WIP

Branch codex/management-resume-parallel-20261003; base origin/main 19aa88a5.
Owned paths: apps/api/src/features/mgmt-tls/**; docs/user/system/management.md; docs/status/tasks/F-management-parallel-*.md.
Implemented HTTPS upgrade delegation to existing Fastify websocket handlers; await listener binding and propagate bind errors. Regression exercises real TLS/Fastify stream route with credential rejection and relay delivery. No security boundary decision.
Checks: tools/ci.sh check --base origin/main: check PASSED (0m12s).
First focused run could not resolve unbuilt @ngfw/schema; schema build subsequently passed.
Initial transport regression: 9 passed / 1 failed, socket hang up. Diagnosis: the HTTPS test client's pooled connection reused a socket closed by the rejected upgrade. The regression now uses a separate real TLS connection per upgrade (`agent: false`); production source unchanged.
Focused rerun: `pnpm --filter @ngfw/api exec vitest run src/features/mgmt-tls/mgmt-tls.test.ts` exit0, 1 file /10 tests passed (16.51s); authenticated relay payload, certificate subject and encrypted socket proved, missing/invalid credentials401.
Complete local quick was CANCELLED for capacity, not passed or failed: host load50.46, exact owned process601084 descendants stopped; interrupted generated tracked outputs restored. Hosted unchanged mandatory quick remains required.
PR: https://github.com/mcoder1001-cyber/NGFW/pull/97
First remote checkpoint fa40ba1cd1b3f139f3d28e08dd94f4af83ba2b80 has identical tree to local1eb20ff8; current correction publication follows this checkpoint.
Applicable independent reviews: R1/R2/R7 and R6 docs, plus root T1 frozen test.
Remaining: unchanged hosted quick gate and independent review; cert removal listener retains cert, no listener creation after empty bootstrap, state inaccuracies/racing reload remain code gaps; PostgreSQL/lab/browser acceptance NOT RUN.
Next command: independent frozen review and hosted CI inspection; do not merge from developer.
Remote/local SHA: see git HEAD; checkpoint publication follows each commit.

Hosted PR97 gate failed gitleaks generic-api-key at mgmt-tls.test.ts219: fixed public RFC6455 handshake nonce, not an actual credential. Replaced with runtime randomBytes(16); no scanner exemptions. History remains on archival PR97; sanitized single-commit candidate is published on codex/management-reviewed-candidate-20261003 with origin/main parent. Focused ESLint exit0 (existing module-type warning).
