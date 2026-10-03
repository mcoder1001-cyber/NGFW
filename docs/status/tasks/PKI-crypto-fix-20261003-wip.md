# PKI inherited crypto fixes

Branch `codex/pki-crypto-fix-20261003`, base `3ca241a4`, isolated worktree `/root/.codex/worktrees/0b16/developers/PKI-crypto-fix`. Final local SHA is this document's commit. Publication, full validation and independent re-review are manager owned.

Owned files: `apps/api/src/features/pki/asn1.ts`, `pkcs12.ts`, `revocation.ts`, unique `crypto-budget-boundaries.test.ts`, and this status document. DTO/service and existing consumer tests untouched. Addresses the three concrete findings in `PKI-inherited-crypto-independent-review-20261003.md` (Notifications-fix commit `27d56be0`).

DER recursive parsing now uses the parent's content end as its read limit, checking child headers, length octets and content against it. PKCS12 uses a shared 100,000 digest-round request budget across MAC, PBKDF2 and legacy key/IV derivations; each operation also caps iterations at 100,000. Derived block count multiplies cost (e.g. SHA1 AES256 costs two rounds per iteration). Charges precede synchronous crypto. Default 2048-round exports remain comfortably within budget. Expensive bundles must be re-exported with fewer iterations. This deliberately bounds event-loop work, but is not a wall-clock deadline or a worker-thread implementation; timing varies by platform.

OCSP without nextUpdate is valid for at most 24h plus 5min clock skew, checking both thisUpdate and producedAt. Future times beyond skew and reversed/inconsistent validity ranges are refused. Existing explicit nextUpdate validity is retained. Delegated signer certificate lifetime and additional CRL semantics were outside these three reproduced findings and remain separate review scope.

Added 15 offline regressions: DER malformed/valid nesting; oversized MAC iterations, cumulative multiblock PBKDF2 budget, per-op multiblock budget, ordinary 2048 iterations, direct legacy KDF cap; signed historical/recent/no-next age/future/range/current-next OCSP fixtures. PBKDF2 is mocked in budget tests so cost-rejection coverage does not consume expensive real rounds. No network listeners or live delivery.

Actual checks: prettier write and `git diff --check`; three direct small offline Node22 probes on temporary exact source copies (only import suffixes changed) now reject the original overlapping DER, signed2000/no-next OCSP checked2026, and direct100001-round KDF before work. Vitest/typecheck/full gate NOT run here, per manager instruction; no expensive test launch.

Manager finite commands on immutable commit:

```sh
pnpm --filter @ngfw/api exec vitest run src/features/pki/crypto-budget-boundaries.test.ts src/features/pki/pkcs12.test.ts src/features/pki/x509.test.ts src/features/pki/revocation.test.ts
pnpm --filter @ngfw/api typecheck
```

Root owns existing integration preparation and full PKI/hosted quick gate. No generated contracts changed. Remaining work: focused/full checks and independent review; implement any observed failure before merge.
