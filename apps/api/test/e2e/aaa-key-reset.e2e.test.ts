import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { totpCode } from '../../src/features/aaa/totp.js';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * S-aaa-key-reset e2e (PostgreSQL + Valkey): an admin reset of a user's second factor clears `mfa_verified` on all
 * of that user's API keys (same transaction) — under an MFA policy the key minted under the old factor is refused
 * (401 mfa-required) until the user re-enrols and mints a new one. The reset is audited with the count.
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const PW = runSecret();

describe('S-aaa-key-reset e2e (PostgreSQL)', () => {
  let h: Harness;
  let admin: string;

  beforeAll(async () => {
    h = await startHarness();
    admin = await h.login('admin', h.adminPassword);
  });
  afterAll(async () => h?.close());

  /** Login-time enrolment of `name` with an admin-issued token → an MFA-verified access token. */
  const enrolAtLogin = async (name: string) => {
    const tok = await h.call(admin, 'POST', `/api/v1/auth/mfa/users/${name}/enrolment-token`);
    expect(tok.status, tok.raw).toBe(200);
    const l = await h.call(undefined, 'POST', '/api/v1/auth/login', {
      username: name,
      password: PW,
    });
    expect(l.body).toMatchObject({ mfaRequired: true, enrolled: false });
    const en = await h.call(undefined, 'POST', '/api/v1/auth/mfa/enroll', {
      challenge: l.body.challenge,
      token: tok.body.token,
    });
    expect(en.status, en.raw).toBe(200);
    const v = await h.call(undefined, 'POST', '/api/v1/auth/mfa/verify', {
      challenge: l.body.challenge,
      code: totpCode(en.body.secret as string),
    });
    expect(v.status, v.raw).toBe(200);
    return v.body.accessToken as string;
  };
  const mint = async (token: string, name: string) => {
    const k = await h.call(token, 'POST', '/api/v1/auth/api-keys', { name, current: PW });
    expect(k.status, k.raw).toBe(201);
    return { authorization: `ApiKey ${k.body.key as string}` };
  };
  const me = (key: Record<string, string>) =>
    h.call(undefined, 'GET', '/api/v1/auth/me', undefined, key);

  it('factor reset → the key minted under it is refused; re-enrol + new key works', async () => {
    // the policy raise needs an enrolled admin committing from an MFA-verified session
    const tok = await h.call(admin, 'POST', '/api/v1/auth/mfa/users/admin/enrolment-token');
    const su = await h.call(admin, 'POST', '/api/v1/auth/mfa/setup', {
      current: h.adminPassword,
      token: tok.body.token,
    });
    expect(su.status, su.raw).toBe(200);
    const act = await h.call(admin, 'POST', '/api/v1/auth/mfa/activate', {
      code: totpCode(su.body.secret as string),
    });
    expect(act.status, act.raw).toBe(200);
    await h.createUsers(admin, [{ username: 'w10ops', role: 'admin', password: PW }]);
    const p = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      { aaa: { mfa: { required: 'admins' } } },
      MP,
    );
    expect(p.status, p.raw).toBe(200);
    const c = await h.call(admin, 'POST', '/api/v1/config/commit?comment=mfa');
    expect(c.status, c.raw).toBe(200);

    // mint from the MFA session → works under the policy
    const ops = await enrolAtLogin('w10ops');
    const key1 = await mint(ops, 'k1');
    await expect
      .poll(async () => (await me(key1)).status, { timeout: 8000, interval: 250 })
      .toBe(200);

    // admin resets the factor → the key loses mfa_verified → 401 mfa-required
    const r = await h.call(admin, 'DELETE', '/api/v1/auth/mfa/users/w10ops');
    expect(r.status, r.raw).toBe(204);
    const refused = await me(key1);
    expect(refused.status, refused.raw).toBe(401);
    expect(refused.body.type).toMatch(/mfa-required$/);
    const flags = await h.db.execute(
      sql`select k.mfa_verified from api_key k join app_user u on u.id = k.user_id where u.username = 'w10ops'`,
    );
    expect(flags.rows).toEqual([{ mfa_verified: false }]);
    const audit = await h.db.execute(
      sql`select after from audit_log where resource = 'user/w10ops' and action like 'DELETE %'`,
    );
    expect(audit.rows).toEqual([{ after: { mfaReset: true, apiKeysMfaCleared: 1 } }]);

    // re-enrol, mint a new key → works; the old key stays refused
    const ops2 = await enrolAtLogin('w10ops');
    const key2 = await mint(ops2, 'k2');
    expect((await me(key2)).status).toBe(200);
    expect((await me(key1)).status).toBe(401);
  });
});
