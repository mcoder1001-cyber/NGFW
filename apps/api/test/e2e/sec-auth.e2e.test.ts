import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * SEC-auth M1 (docs/status/tasks/SEC-auth-review.md): a key minted by an expiring API key never outlives it — before
 * the fix a leaked 7-day key minted a key that never expired and survived the deletion of the leaked one.
 */
describe('SEC-auth e2e: API-key lifetime', () => {
  let h: Harness;
  const pw = runSecret();

  beforeAll(async () => {
    h = await startHarness({ NGFW_PASSWORD_RATE_PER_MIN: '1000' });
    const admin = await h.login('admin', h.adminPassword);
    await h.createUsers(admin, [{ username: 'keysec', role: 'operator', password: pw }]);
  });
  afterAll(async () => {
    await h?.close();
  });

  const mint = async (auth: { bearer?: string; key?: string }, body: Record<string, unknown>) =>
    h.call(
      auth.bearer,
      'POST',
      '/api/v1/auth/api-keys',
      body,
      auth.key ? { authorization: `ApiKey ${auth.key}` } : undefined,
    );
  const expiry = (r: { body: Record<string, unknown> }) =>
    r.body['expiresAt'] === null ? null : new Date(r.body['expiresAt'] as string).getTime();

  it('a key minted by an expiring key gets at most the caller key’s expiry', async () => {
    const tok = await h.login('keysec', pw);
    const parent = await mint({ bearer: tok }, { name: 'ci', expiresInDays: 7, current: pw });
    expect(parent.status).toBe(201);
    const parentExp = expiry(parent)!;
    expect(parentExp).toBeGreaterThan(Date.now());

    // no expiry asked: the caller's (never null)
    const child = await mint({ key: parent.body['key'] as string }, { name: 'child' });
    expect(child.status).toBe(201);
    expect(expiry(child)).not.toBeNull();
    expect(Math.abs(expiry(child)! - parentExp)).toBeLessThan(2000);

    // a longer expiry asked: clamped to the caller's
    const longer = await mint({ key: parent.body['key'] as string }, { name: 'longer', expiresInDays: 365 });
    expect(longer.status).toBe(201);
    expect(expiry(longer)!).toBeLessThanOrEqual(parentExp + 1000);

    // a shorter expiry asked: kept
    const shorter = await mint({ key: parent.body['key'] as string }, { name: 'shorter', expiresInDays: 1 });
    expect(shorter.status).toBe(201);
    expect(expiry(shorter)!).toBeLessThan(parentExp - 86_400_000);
  });

  it('a key without expiry, and a login session, mint as before', async () => {
    const tok = await h.login('keysec', pw);
    const forever = await mint({ bearer: tok }, { name: 'svc', current: pw });
    expect(forever.status).toBe(201);
    expect(expiry(forever)).toBeNull();
    const child = await mint({ key: forever.body['key'] as string }, { name: 'svc-child' });
    expect(child.status).toBe(201);
    expect(expiry(child)).toBeNull();
  });
});
