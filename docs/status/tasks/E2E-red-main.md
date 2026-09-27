# E2E-red-main — API e2e red on main (cloud session charming-johnson, 2026-09-27)

Found by SEC-auth-fix: the first full API e2e run in a cloud session (local PostgreSQL 16 + Redis,
`VRX_TEST_PREFIX=w1`) had 8 failures on main — the cloud ports merged with unit gates only, no lab e2e.
None was a product defect; all were tests that features merged in parallel had made stale.

| file | cause | fix |
|---|---|---|
| config.e2e (actions 501) | `/state/neighbors` is served since F-neighbors-ra | expects 200 |
| neighbors-ra.e2e, unbound-chrony-syslog.e2e | used `/actions/ping` as "still 501"; ping is served since F-vrf-static-ecmp (400 without a body) | `/actions/reboot` (no agent action yet) |
| bgp.e2e (3), wireguard.e2e (2) | F-licensing gates `bgp` and `wireguard` (+ `wireguardInterfaces` limit); the commit got 403 license-required, the rest cascaded | `test/support/license.ts` `useTestLicense(features, limits)`: a licence signed with a per-run key (testkit), trusted via VRX_LICENSE_PUBLIC_KEYS, file in the slot run dir; restored after the file |

After: full API e2e 41 files passed, 1 skipped (the agent integration test) — 214 tests passed, 3 skipped. API
unit 282/282, typecheck and lint clean.

To run it in a cloud container: `service postgresql start; redis-server --daemonize yes --save ''`, then
`cd apps/api && VRX_TEST_PREFIX=w1 npx vitest run -c vitest.e2e.config.ts` (as root: `deploy/dev/pg-test.sh` uses
`runuser -u postgres`).
