# F-licensing — questions for the manager / product owner

1. **Entitlement matrix (product owner).** `apps/api/src/features/licensing/entitlements.ts` ships a clearly marked
   SAMPLE: gated = ipsec, wireguard, bgp, ospf, isis, ha; community = wireguard (≤ 2 interfaces) + ospf. Consequence to
   note: after merge, a device **without** a licence rejects new commits that add IPsec tunnels, BGP, IS-IS or VRRP
   (existing running config is grandfathered). **Resolved for now:** community is permissive; the matrix is
   `docs/decisions/DEC-licensing-matrix.md`.
2. **Host binding default.** Optional; `serial` (DMI product serial) recommended over `machineIdHash` because VM templates
   clone `/etc/machine-id` (vrx-a is VMware). Both supported; neither required.
3. **Product signing key custody.** `licensing.config.ts` embeds a PLACEHOLDER public key whose private half was
   discarded (no licence verifies against it). Release engineering must generate the real key offline
   (`vrx-license keygen`) and replace the constant. `VRX_LICENSE_PUBKEY_FILE` adds a trusted key (dev/test only) — say if
   that override should be compiled out of production builds.
4. **Missing anchors (hunks unanchored, appended at the end of the block):** `apps/api/src/commit/validation.service.ts`
   (no `// wave-BC: F-licensing` anchor — tier union, import, optional constructor param, the stage call, warnings
   merge), `apps/web/src/router.tsx` (route placed above `system/revisions`), `apps/web/src/nav/nav.ts` +
   `nav.test.ts` (after the `F-hardening-lite` anchor), `apps/web/src/shell/AppShell.tsx` (banner after
   `<SyncBanner />` + its import). Present and used: `app.module.ts` (×3), `i18n.ts` (×4).
5. **route-guard ADMIN_ONLY list** (`apps/api/src/auth/route-guard.test.ts`, TD-2 owned) has no F-licensing anchor, so
   `PUT /api/v1/system/license` is not in it. It is `@MinRole('admin')` (asserted in `licensing.test.ts`; readonly→403 is
   already covered by the route-guard matrix). Please add `'PUT /api/v1/system/license'` to ADMIN_ONLY at merge.
6. **Problem status.** A licence rejection is `403 application/problem+json` (`type …/license-required`,
   `tier: "license"`, `errors[].pointer`), thrown from the validation stage (so `POST /config/validate` and
   `/config/commit` both return it). The commit controller's OpenAPI does not list 403-license separately (commit/** is
   not mine beyond the stage hunk).
7. **Storage.** The licence is stored as a file (`VRX_LICENSE_FILE`, default `/var/lib/vrx/license.vrxlic`), not in
   PostgreSQL — no migration. `deploy/` (systemd unit / packaging) must make `/var/lib/vrx` writable by the API user;
   F-backup-restore decides whether the file is part of a backup.
8. **Expiry event dedup.** The `license.grace` / `license.expired` `system_event` is recorded on each status transition
   seen by the process (hourly timer + every status read); after an API restart a device already in grace records it once
   more. Acceptable, or persist the last status?
9. **Environment limits of this run** (cloud container, no systemd): no PostgreSQL, Valkey, VPP or agent, and `turbo`
   could not spawn child tasks here (`Exec format error`) — packages were built with direct `pnpm -C … build`. Not done
   here and still owed on the lab host: the UI screenshot against the real endpoint (headless Chrome), the "running
   config stays applied" Retrieve before/after expiry on the slot agent, and the `apps/api/test/e2e/licensing*` e2e.
