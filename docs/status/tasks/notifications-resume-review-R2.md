# Notifications resume — independent R2 security review

Exact reviewed product HEAD: `a0b6e34ba09a790ba99e504033959f9c26f11fba`; tree `14a5c587061707f4359ebff892d5e984954274f8`. Base `53a43ce5b91e71f3fedc282f9c5c54ba22fc9fb8`. Reviewer branch `codex/notifications-security-review-20261002`, isolated worktree `/workspace/scratch/de92de7d9874/NGFW-notifications-r2`. No product edits or publication.

## Findings and verdict

No confirmed R2 BLOCKER, MAJOR, or MINOR. **APPROVE** for R2 security at the exact product HEAD above. Other panels, unchanged complete quick gate, and current integration-tree verification remain manager gates; this does not claim feature completion or real delivery acceptance.

Read shared review/security instructions and historical independent reports `F-notifications-review-security-023fa022.md` (BLOCK) and `F-notifications-review-security-4e7e03d2.md` (APPROVE). Fresh static review confirms the historical SMTP address restriction fix is retained: canonical IPv4/IPv6 parsing rejects mapped or expanded loopback and the complete link-local range before transport construction, checks all DNS answers, and pins the approved address while preserving private relays, TLS hostname verification, and STARTTLS requirement. `transport.ts` SHA-256 is `177e9037698e2e7f660c7d51b6b44928457d9c4fa4b51c254196751bfe805cc9`, identical to the old R2 worktree. Diff from recovered commit `aeefa86c` to reviewed HEAD for that file exits 0.

Webhook checks reject non-public answers, credentials/fragments and non-HTTPS URLs, pin the approved address, require verified TLS, sign the exact body and do not follow redirects. SMTP disables file/URL content access. Decryption uses existing encrypted secret references and parameterized Drizzle lookup. Delivery history contains bounded generic failure reasons rather than secrets, provider responses, URLs, recipients or full event payloads.

Global AuthGuard and AuditInterceptor cover new routes; test action is explicitly admin-only and notification config joins ADMIN_ONLY_POINTERS. Queue/history/dedup/retry bounds remain intact. Current configure change retains `test:<channel>` throttles across reload; object names cannot collide with that colon-prefixed namespace. Removal of an unnecessary regex escape preserves sanitization. Schema refinement continues to reject CR/LF/NUL in SMTP username while preserving existing channel restrictions. Telegram is not a supported channel. Non-default VRF binding and IPsec event mapping remain explicitly unimplemented; in-memory queue/restart limitations are disclosed.

Dependency check: lockfile resolves Nodemailer 10.0.13; installed package declares MIT-0. Maintainer advisory GHSA-4ffr-jq9g-5ffx lists affected versions through 10.0.12 and patched 10.0.13; official September 30 release notes corroborate the fix. Sources inspected 2026-10-02: https://github.com/nodemailer/nodemailer/security/advisories/GHSA-4ffr-jq9g-5ffx and https://github.com/nodemailer/nodemailer/releases . No claim that an advisory search proves absence of all vulnerabilities.

## Commands actually executed

Pinned tool PATH `/workspace/scratch/96b8b6fbc8a7/toolchain/bin`. Dependencies reused through symlinks to current notification worker node_modules; tests execute this review worktree's source, while workspace package build dependencies resolve through the worker's installed workspace. Direct schema tests below execute the reviewed schema source. No clean-install or independent full-build claim.

In apps/api:
```text
node node_modules/vitest/vitest.mjs run src/features/notifications/transport.test.ts src/features/notifications/notifications.test.ts
transport.test.ts (24 tests) 155ms
notifications.test.ts (30 tests) 301ms
Test Files 2 passed (2)
Tests 54 passed (54)
Duration 16.40s
```

In packages/schema:
```text
node node_modules/vitest/vitest.mjs run src/domains/ext/notifications.test.ts
notifications.test.ts (5 tests) 9ms
Test Files 1 passed (1)
Tests 5 passed (5)
Duration 1.34s
```

Both commands exited 0. Tests use mocked transports; no real SMTP/webhook sent.

- `gitleaks detect --no-git --redact -s apps/api/src/features/notifications --config .github/gitleaks.toml --no-banner`: exit 0, 31.25 KB scanned, no leaks found.
- Same scan with `-s docs/status/tasks`: exit 0, approximately 5.63 MB scanned, no leaks found.
- `git diff --exit-code HEAD -- apps/api/src/features/notifications packages/schema/src/domains/ext/notifications.ts`: exit 0 after verification.

No full quick gate, live appliance/network delivery, browser acceptance, or exhaustive dependency audit performed by this reviewer.
