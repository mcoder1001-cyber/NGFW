# S-license-all

## What
- `COMMUNITY` (entitlements.ts) = no gated feature, every limit 0 (product owner answer to the licensing matrix).
  Grandfathering of running config (D-144) and 30-day grace unchanged.
- `VRX_LICENSE_PUBLIC_KEYS` (comma-separated PEMs, `\n` escapes allowed) replaces the embedded placeholder key list
  (`LicensingOptions.publicKeys`). Tested with keys generated at test time; no private key committed.
- Docs: PENDING-licensing-matrix -> DEC-licensing-matrix with Decision section; LOG D-151; docs/user/system/licensing.md;
  web en/fa community help text.

## Verification
- `npx turbo run lint typecheck test --filter=@ngfw/api --filter=@ngfw/web`: Tasks 17 successful; api 18 files / web 19 files passed.
- `tools/ci.sh check`: check PASSED.
- No other feature's api test needed changes (they do not use the default community set).

## Out of scope / open
- Real product signing key: release engineering must set VRX_LICENSE_PUBLIC_KEYS or replace the embedded key.
