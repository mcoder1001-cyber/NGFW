import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { totpCode } from '../../src/features/aaa/totp.js';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-aaa-mfa-lockout e2e (PostgreSQL + Valkey + fake agent): a commit that RAISES management.aaa.mfa.required is
 * refused (400, pointer /management/aaa/mfa/required) until an admin has an active factor AND the committer's own
 * session passed MFA; lowering the policy is never blocked (not even from an API key with no admin enrolled).
 */
const MP = { 'content-type': 'application/merge-patch+json' };
const POINTER = '/management/aaa/mfa/required';

describe('F-aaa-mfa-lockout e2e (PostgreSQL + fake agent)', () => {
  let h: Harness;
  let admin: string;
  let apiKey: string;

  const asKey = { authorization: '' };
  const mfaKey = { authorization: '' };
  const stage = (token: string | undefined, required: string, headers?: Record<string, string>) =>
    h.call(
      token,
      'PATCH',
      '/api/v1/config/management',
      { aaa: { mfa: { required } } },
      {
        ...MP,
        ...headers,
      },
    );
  const commit = (token: string | undefined, headers?: Record<string, string>) =>
    h.call(token, 'POST', '/api/v1/config/commit', undefined, headers);
  const running = async () =>
    (await h.call(admin, 'GET', '/api/v1/config/management')).body?.aaa?.mfa?.required ?? 'none';

  beforeAll(async () => {
    h = await startHarness();
    admin = await h.login('admin', h.adminPassword);
    const k = await h.call(admin, 'POST', '/api/v1/auth/api-keys', {
      name: 'automation',
      current: h.adminPassword,
    });
    expect(k.status, k.raw).toBe(201);
    apiKey = k.body.key as string;
    asKey.authorization = `ApiKey ${apiKey}`;
  });
  afterAll(async () => h?.close());

  it('raise refused while no admin has a factor (commit and validate: 400 with the pointer and the remedy)', async () => {
    expect((await stage(admin, 'admins')).status).toBe(200);
    for (const url of ['/api/v1/config/validate', '/api/v1/config/commit']) {
      const r = await h.call(admin, 'POST', url);
      expect(r.status, `${url} ${r.raw}`).toBe(400);
      expect(r.headers['content-type']).toMatch(/application\/problem\+json/);
      expect(r.body.errors[0].pointer).toBe(POINTER);
      expect(r.body.detail).toMatch(/Enrol an admin first/);
    }
    expect(await running()).toBe('none');
    // the session still works: nobody was locked out
    expect((await h.call(admin, 'GET', '/api/v1/auth/me')).status).toBe(200);
    await h.call(admin, 'POST', '/api/v1/config/discard');
  });

  it('raise allowed after the admin enrolled from this session (MFA-verified session)', async () => {
    const tok = await h.call(admin, 'POST', '/api/v1/auth/mfa/users/admin/enrolment-token');
    expect(tok.status, tok.raw).toBe(200);
    const setup = await h.call(admin, 'POST', '/api/v1/auth/mfa/setup', {
      current: h.adminPassword,
      token: tok.body.token,
    });
    expect(setup.status, setup.raw).toBe(200);
    const act = await h.call(admin, 'POST', '/api/v1/auth/mfa/activate', {
      code: totpCode(setup.body.secret as string),
    });
    expect(act.status, act.raw).toBe(200);

    // an API key is not a login session that passed MFA: still refused, even with an admin enrolled
    expect((await stage(undefined, 'admins', asKey)).status).toBe(200);
    const byKey = await commit(undefined, asKey);
    expect(byKey.status, byKey.raw).toBe(400);
    expect(byKey.body.errors[0].pointer).toBe(POINTER);
    await h.call(undefined, 'POST', '/api/v1/config/discard', undefined, asKey);

    expect((await stage(admin, 'admins')).status).toBe(200);
    const c = await commit(admin);
    expect(c.status, c.raw).toBe(200);
    expect(await running()).toBe('admins');
    expect((await h.call(admin, 'GET', '/api/v1/auth/me')).status).toBe(200);

    // F-aaa-hardening: the key minted before the raise (no MFA session) no longer bypasses the policy …
    // (the login policy is cached for up to 5 s, as for Bearer sessions)
    const me = () => h.call(undefined, 'GET', '/api/v1/auth/me', undefined, asKey);
    await expect.poll(async () => (await me()).status, { timeout: 8000, interval: 250 }).toBe(401);
    const old = await me();
    expect(old.body.type).toMatch(/mfa-required$/);
    // … a key minted from this MFA-verified session works
    const k2 = await h.call(admin, 'POST', '/api/v1/auth/api-keys', {
      name: 'automation-mfa',
      current: h.adminPassword,
    });
    expect(k2.status, k2.raw).toBe(201);
    mfaKey.authorization = `ApiKey ${k2.body.key as string}`;
    expect((await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, mfaKey)).status).toBe(200);
  });

  it('lowering is always allowed — even from an API key (no MFA session)', async () => {
    // S-aaa-key-reset: a factor reset clears mfa_verified on the owner's keys, so the lowering runs from the
    // MFA-minted key BEFORE the admin's factor is reset (after it, that key is refused while the policy covers admins)
    // raising further (admins → all) from the API key is refused …
    expect((await stage(undefined, 'all', mfaKey)).status).toBe(200);
    const up = await commit(undefined, mfaKey);
    expect(up.status, up.raw).toBe(400);
    expect(up.body.errors[0].pointer).toBe(POINTER);
    // … lowering (admins → none) is not
    expect((await stage(undefined, 'none', mfaKey)).status).toBe(200);
    const down = await commit(undefined, mfaKey);
    expect(down.status, down.raw).toBe(200);
    // the factor goes (admin reset of the own factor): the admin's sessions end, no admin is enrolled any more
    const r = await h.call(undefined, 'DELETE', '/api/v1/auth/mfa/users/admin', undefined, mfaKey);
    expect(r.status, r.raw).toBe(204);
    admin = await h.login('admin', h.adminPassword); // no factor needed any more
    expect(await running()).toBe('none');
    // refuse-at-use, not revocation: with the policy lowered the pre-MFA key works again
    await expect
      .poll(
        async () => (await h.call(undefined, 'GET', '/api/v1/auth/me', undefined, asKey)).status,
        { timeout: 8000, interval: 250 },
      )
      .toBe(200);
  });
});
