import { describe, expect, it } from 'vitest';
import { databaseUrl, loadEnv } from './config.js';

describe('loadEnv', () => {
  it('defaults, empty = unset, coercion', () => {
    const env = loadEnv({ VRX_JWT_SECRET: '', VRX_HTTP_PORT: '3100', VRX_COOKIE_SECURE: 'true' });
    expect(env.VRX_JWT_SECRET).toBeUndefined();
    expect(env.VRX_HTTP_PORT).toBe(3100);
    expect(env.VRX_COOKIE_SECURE).toBe(true);
    expect(env.VRX_ACCESS_TTL_SEC).toBe(900);
    expect(env.VRX_LOGIN_MAX_FAILURES).toBe(10);
  });

  it('VRX_DEV_WEAK_PASSWORDS is off unless explicitly set', () => {
    expect(loadEnv({}).VRX_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: '' }).VRX_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: '0' }).VRX_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: 'false' }).VRX_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: '1' }).VRX_DEV_WEAK_PASSWORDS).toBe(true);
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: 'true' }).VRX_DEV_WEAK_PASSWORDS).toBe(true);
    expect(() => loadEnv({ VRX_DEV_WEAK_PASSWORDS: 'yes' })).toThrow(/VRX_DEV_WEAK_PASSWORDS/);
  });

  it('refuses VRX_DEV_WEAK_PASSWORDS in a production process (the packaged unit sets NODE_ENV=production)', () => {
    expect(() => loadEnv({ VRX_DEV_WEAK_PASSWORDS: '1', NODE_ENV: 'production' })).toThrow(
      /VRX_DEV_WEAK_PASSWORDS: development only/,
    );
    expect(loadEnv({ VRX_DEV_WEAK_PASSWORDS: '0', NODE_ENV: 'production' }).VRX_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NODE_ENV: 'production' }).VRX_DEV_WEAK_PASSWORDS).toBe(false);
  });

  it('VRX_DATABASE_URL wins over the pg-test VRX_PG_DSN', () => {
    expect(databaseUrl(loadEnv({ VRX_PG_DSN: 'postgres://a/b' }))).toBe('postgres://a/b');
    expect(
      databaseUrl(loadEnv({ VRX_PG_DSN: 'postgres://a/b', VRX_DATABASE_URL: 'postgres://c/d' })),
    ).toBe('postgres://c/d');
  });

  it('never echoes offending values (they may be secrets)', () => {
    let msg = '';
    try {
      loadEnv({ VRX_JWT_SECRET: 'short-secret-value' });
    } catch (e) {
      msg = (e as Error).message;
    }
    expect(msg).toContain('VRX_JWT_SECRET');
    expect(msg).not.toContain('short-secret-value');
  });
});
