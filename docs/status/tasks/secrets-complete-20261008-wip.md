# Sealed production credentials — 2026-10-08

Branch: `codex/secrets-complete-20261008`, base `417e8fcd`.
Owned: API operational secret selectors/tests; agent startup secret bindings; WireGuard sealed DF-5 adapters; SNMP scheduler generation wrappers/renderer binding/tests.

WireGuard now selects strict base64 32-byte keys from the transaction-selected sealed cache and resolves historical DF-5 generations from retained snapshots. SNMP binds scheduler values and private reference-only records to exact keyed generations. Product SNMP does not use environment fixture credentials. Rotation, rollback after reopening the sealed store, deletion/revocation and absence of plaintext metadata are covered by focused tests.

API selectors extended for WireGuard, SNMP, BGP, NTP symmetric keys, TLS syslog and host-stack namespaces. CA signing-key delivery remains prohibited. The existing version-pinned channel transports all selected credentials; no new plaintext persistence.

Focused `go test -race` tests `TestWireguardSealed*` and `TestSnmpSealed*`: PASS (secretchannel 1.021s, subsystems 1.093s). Initial existing SNMP suite exposed one stale error-text assertion; changed to errors.Is against the unchanged error sentinel. Rerun pending. API selector tests and consumer worker integration pending. Full CI deliberately deferred until all source work is complete, per owner.

Next: finish API coverage, run focused Go/API suites, independently review. Real daemon and packet acceptance remains lab-only after source acceptance. This checkpoint is not a completion claim.

## Follow-up validation

Published checkpoint: `7c6eb473a8bf72c0aba49fc5c868d56327031850` (tree matches local `1407496e`).
API focused selector suite: **32 PASS**, including all six consumers, revision pins, disabled selection and CA alias exclusion.
Go race: actual WireGuard service DryRun, Apply, rotation, confirm revert and restart **PASS** (1.159s); SNMP tests including community/USM auth/privacy rotation and existing regressions **PASS** (1.105s); sealed WireGuard tests **PASS** (1.018s); scheduler binding persistence/malformed metadata **PASS** (1.077s).
API typecheck initially blocked solely by absent local `@ngfw/yang` build artifact; after building local dependency artifacts, API typecheck and focused ESLint both **PASS**. No CI run triggered.

## R4 security correction

R4 found that an undeclared CA certificate/key alias could be delivered through TLS syslog. Every syslog private-key selection now requires a parsed, non-CA paired certificate before key lookup, independently of other consumers sharing that reference. After decryption the private key must match every selected leaf certificate; failures zero the bundle. Five added controls cover hidden CA, mixed host-stack alias, missing certificate, pinned valid pair and mismatched pair. Focused API suite: **36 PASS**. This closes the documented R4 bypass; independent re-review pending.
