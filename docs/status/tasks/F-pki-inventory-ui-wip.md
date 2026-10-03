# Public PKI inventory UI checkpoint — 2026-10-03

Branch: codex/pki-inventory-ui-20261003. Base: 23c98f36968a34432e42f242a7ef5e6f88a78e56. Worktree: PKI-inventory-ui. Owned: two new apps/web/src/domains/vpn/pki/PkiInventoryPanel source/test files and this report. Root handles publication/integration/testing.

Standalone read-only panel uses generated GET /api/v1/state/vpn/pki through existing authentication/deadline client and TanStack Query. Public names, subject/issuer, expiry and revocation/attention summaries; total row budget100 with notice. No key/certificate refs, PEM, raw diagnostics, candidate access or mutating requests. Accessible loading/error/retry/refresh/empty states; local English/Persian resources. Tests cover empty loading/public-only requests, secret canaries, revoked/expiring status, error recovery, mixed CA/certificate row bounds, Persian labels.

Actual validation: direct Prettier completed; no heavy commands or tests executed. Root exact commands (after preparing dependency links): `cd apps/web && pnpm exec vitest run src/domains/vpn/pki/PkiInventoryPanel.test.tsx --maxWorkers=1`; `pnpm exec tsc -p tsconfig.json --noEmit`; `pnpm exec eslint src/domains/vpn/pki/PkiInventoryPanel.tsx src/domains/vpn/pki/PkiInventoryPanel.test.tsx`. Product is standalone until coordinated VPN tabs F-pki anchor registration. No shared tabs/router/locales edits. Full feature still needs mutation workflows, coordinated navigation and agent/live acceptance.

Next: immutable root-owned offline validation, independent UI review, then coordinated tab integration. ETA for validation-driven fixes:10–20min after root feedback. No push performed per task envelope; manager publishes prepared checkpoints.
