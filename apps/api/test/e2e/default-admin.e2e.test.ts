import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthService } from '../../src/auth/auth.service.js';
import { startHarness, type Harness } from '../support/harness.js';

describe('product default credentials and short passwords', () => {
  let h: Harness;
  beforeAll(async () => {
    h = await startHarness({
      NODE_ENV: 'production',
      NGFW_BOOTSTRAP_ADMIN_PASSWORD: '',
      NGFW_LOGIN_RATE_PER_MIN: '100',
    });
  });
  afterAll(async () => h?.close());
  it('logs in with defaults, accepts a one-character change and never reseeds an existing account', async () => {
    expect(h.env.NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    const token = await h.login('admin', 'admin');
    const empty = await h.call(token, 'POST', '/api/v1/users/admin/password', {
      current: 'admin',
      password: '',
    });
    expect(empty.status).toBe(400);
    const changed = await h.call(token, 'POST', '/api/v1/users/admin/password', {
      current: 'admin',
      password: 'a',
    });
    expect(changed.status).toBe(200);
    expect(await h.app.get(AuthService).seedBootstrapAdmin()).toBe(false);
    expect(await h.login('admin', 'a')).toBeTruthy();
    expect(
      (await h.call('', 'POST', '/api/v1/auth/login', { username: 'admin', password: 'admin' }))
        .status,
    ).toBe(401);
  });
});
