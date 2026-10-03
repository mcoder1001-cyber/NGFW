# PKI precise expiry alarm boundary — 2026-10-03

Branchcodex/pki-api-gap-20261003, isolatedPKI-api-gap, baseb2c214b5. Own pki.service.ts expiry severity/message hunk, NEWexpiry.test.ts and this WIP only. Root publishes and owns finite tests/review/merge. No generated/shared/agent/P11 changes.

Concrete pre-fix evidence: date probe with now2030-01-01T00:00Z and expiry23:00Z reports stillValid:true, floor daysLeft:0, existing expired alarm:true. SourcecheckExpiry used <=0 on rounded days to set critical and expired message, so still-valid CA/certificate during last partial day were falsely called expired.

Repair uses exact notAfter<now for critical severity and derives expired wording from that severity, preserving rounded display metrics, configured alert windows, clear events and repeat dedup. Matches mounted inventory's exact-time validity boundary. No response contract changes.

Two meaningful offline service regressions use real generated P256 certificate DER and controlled service clock: CA+certificate final-hour warnings with no expired message, exact validity boundary warning, one millisecond past expiry critical/message and unchanged repeat quiet. Database/agent/bus boundaries remain test-only; no live delivery or materialization claim.

Actual validation: harmless date arithmetic probe and source inspection; Prettier completed. No vitest/tsc/lint/heavy commands yet. Exact root commands cwdapps/api: pnpm exec vitest run src/features/pki/expiry.test.ts src/features/pki/dto.test.ts --maxWorkers=1; pnpm exec tsc -p tsconfig.json --noEmit; pnpm exec eslint src/features/pki/pki.service.ts src/features/pki/expiry.test.ts. Root full hosted gate remains required. Live alarm delivery/appliance acceptance NOT RUN; P11 secret transport/materializer externally owned.
