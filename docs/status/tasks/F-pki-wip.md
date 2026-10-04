# F-pki implementation checkpoint — 2026-10-04

Branch: `codex/complete-pki-20261004`. Published prior remote checkpoint: `e29bfac47ec348dc1bfcddcb20f8825c81d81cf9` (exact tree verified). This checkpoint will be published before integration.

Owned source: agent internal/pki, desired/subsystems PKI; minimal registration/projection/cache and public RPC seams; API socket secret delivery/tests; web PKI actions/inventory/tests and bilingual locales; user docs and precise secret-channel scope.

Completed: atomic materialization, verified PEM/key match/CRL signatures, isolated root and key permissions, symlink protections, manifest cleanup and drift fingerprints, sealed-cache literal-reference adapter, public state; CA/CSR/sign/import/export/CRL/OCSP dialogs with role controls, safe errors and sensitive-field clearing. Stored-only responses are distinct from successful staging. CA signing/CSR-only keys stay API-side; actual CA certificate aliases blocked before operational key delivery.

Actual results: all four focused agent packages (pki/subsystems/desired/agent) passed. Existing API PKI suites and serialization passed; expanded socket delivery10 tests passed; UI inventory8/actions5 passed. API/web typechecks and focused lint passed on final source. Independent reviewer approved source and UI; exact final SHA acknowledgment pending.

Critical regression: real sealed cache with same refs rotated or old refs omitted, then later descriptor failure -> ROLLED_BACK with exact previous files and no cache-selection mutation. Previous disk generation is bounded/opaque and wiped on replacement/close. Restart re-materialization, unsafe symlinks, signing CA rejection and corrupt-manifest sanitized RPC checked.

Remaining: publish final checkpoint, bind independent review to SHA, integration against updated main. Hosted/full CI waived; deployed browser/daemon acceptance NOT RUN. Native cert tunnel consumption remains the separate task.

Next command: `pnpm --filter @ngfw/web exec vitest run src/domains/vpn/pki/PkiActions.test.tsx`.
