# PKI inventory expiry review repair — 2026-10-03

Branch codex/pki-inventory-expiry-20261003; base immutable76ca893b. Own only new panel/test and this WIP. Original UI tree remains frozen.

Corrected independently reviewed P2: both certificate authorities and certificates now display localized Expired when notAfter is strictly before inventory.generatedAt. Revoked remains highest priority. Expired precedes expiring/attention/configured. The rounded daysLeft count is not used to infer expiry.

Added one combined regression covering both groups one second before/exactly at/one second after inventory time, missing expiry, rounded zero days, expired-and-expiring and expired-and-revoked precedence. Existing five UI tests retained. Actual validation: Prettier completed only. No heavy tests or typecheck run by developer; root owns finite validation.

Next commands from apps/web: pnpm exec vitest run src/domains/vpn/pki/PkiInventoryPanel.test.tsx --maxWorkers=1; pnpm exec tsc -p tsconfig.json --noEmit; pnpm exec eslint src/domains/vpn/pki/PkiInventoryPanel.tsx src/domains/vpn/pki/PkiInventoryPanel.test.tsx. ISO re-review requested. No shared navigation/locale edits, no push per envelope.

Corrected Notifications independent review blocker: controller and generated client expose `/api/v1/state/pki`; panel and fake API fixtures now use that exact typed route without casts. Earlier standalone checkpoint used the wrong route; it must not be integrated without this successor.
