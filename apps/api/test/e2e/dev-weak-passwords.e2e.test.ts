import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { runSecret, startHarness, type Harness } from '../support/harness.js';

/**
 * VRX_DEV_WEAK_PASSWORDS=1 (development only): both password routes accept a new password shorter than 12 characters,
 * and it works for login. The product rule (flag off) is covered in auth.e2e (/auth/password) and td2.e2e (/users/…).
 * Passwords are generated per run; "short" means a slice of a few characters.
 */
describe('VRX_DEV_WEAK_PASSWORDS e2e', () => {
  let h: Harness;
  let adminPw: string;
  const RO = runSecret();

  beforeAll(async () => {
    h = await startHarness({ VRX_DEV_WEAK_PASSWORDS: '1', VRX_PASSWORD_RATE_PER_MIN: '30' });
    adminPw = h.adminPassword;
    const admin = await h.login('admin', adminPw);
    await h.createUsers(admin, [{ username: 'wro1', role: 'readonly', password: RO }]);
  });
  afterAll(async () => h?.close());

  it('own password via /auth/password: a 5-character password is accepted and logs in', async () => {
    const admin = await h.login('admin', adminPw);
    const weak = runSecret().slice(0, 5);
    const r = await h.call(admin, 'POST', '/api/v1/auth/password', { current: adminPw, password: weak });
    expect(r.status).toBe(204);
    adminPw = weak;
    expect(typeof (await h.login('admin', weak))).toBe('string');
  });

  it('admin reset via /users/{name}/password: a 3-character password is accepted and logs in; empty stays refused', async () => {
    const admin = await h.login('admin', adminPw);
    const weak = runSecret().slice(0, 3);
    const r = await h.call(admin, 'POST', '/api/v1/users/wro1/password', { password: weak });
    expect(r.status).toBe(200);
    expect(typeof (await h.login('wro1', weak))).toBe('string');
    const empty = await h.call(admin, 'POST', '/api/v1/users/wro1/password', { password: '' });
    expect(empty.status).toBe(400);
    expect(empty.body.errors[0].pointer).toBe('/password');
  });
});
