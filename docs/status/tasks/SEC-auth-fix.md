# SEC-auth-fix — findings M1 and L1 of SEC-auth (cloud session charming-johnson, 2026-09-27)

## M1 — an API-key-minted key never outlives its caller key (`apps/api/src/auth/auth.service.ts` createApiKey)
A key created by an API-key caller gets `expiresAt = min(requested, caller key's expiry)`; with no `expiresInDays` it
inherits the caller's expiry instead of `null`. A caller key without expiry, and a login session (JWT + step-up), mint
exactly as before. The role cap is unchanged.

## L1 — the secret store's master key goes through the key-file checks (`secrets/secrets.service.ts`, `auth/key-file.ts`)
`masterKey()` reads `VRX_SECRET_KEY_FILE` with the new `readKeyFileBytes` (the JWT ring's `withKeyFile`: O_NOFOLLOW,
regular file, owner = API user or root, no group/other bits) instead of `existsSync` + `chmodSync` + `readFileSync`.
A missing key is still created 0600 with `wx` (O_EXCL). A symlink, a foreign owner or group/other access is a 503 that
names the path and the problem; the file is no longer chmod-ed (which followed a symlink).
Compatibility: keys the API created were 0600 already (created so, and chmod-ed on every start).

## Evidence (this session: local PostgreSQL 16 + Redis, `VRX_TEST_PREFIX=w1`)
- `test/e2e/sec-auth.e2e.test.ts` (new): an expiring key minting without/with a longer/with a shorter expiry; a
  non-expiring key and a JWT session unchanged. Before the fix: `expected null not to be null` (the child never
  expired). After: 2/2.
- `src/secrets/secrets.masterkey.test.ts` (new): created 0600 and read back; a symlink refused and its target not
  chmod-ed; a 0640 key refused. 3/3.
- `vitest src/auth src/secrets`: 41/41. Full API e2e: 206 passed, 8 failed — the same 8 fail on main without this
  change (bgp, config "actions answer 501", neighbors-ra arp-flush, unbound-chrony-syslog dns-lookup, wireguard);
  they are a separate row (E2E-red-main), not this fix.
