import { describe, expect, it } from 'vitest';
import { databaseUrl, loadEnv } from './config.js';

describe('loadEnv', () => {
  it('defaults, empty = unset, coercion', () => {
    const env = loadEnv({ NGFW_JWT_SECRET: '', NGFW_HTTP_PORT: '3100', NGFW_COOKIE_SECURE: 'true' });
    expect(env.NGFW_JWT_SECRET).toBeUndefined();
    expect(env.NGFW_HTTP_PORT).toBe(3100);
    expect(env.NGFW_COOKIE_SECURE).toBe(true);
    expect(env.NGFW_ACCESS_TTL_SEC).toBe(900);
    expect(env.NGFW_LOGIN_MAX_FAILURES).toBe(10);
  });

  it('NGFW_DEV_WEAK_PASSWORDS is off unless explicitly set', () => {
    expect(loadEnv({}).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: '' }).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: '0' }).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: 'false' }).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: '1' }).NGFW_DEV_WEAK_PASSWORDS).toBe(true);
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: 'true' }).NGFW_DEV_WEAK_PASSWORDS).toBe(true);
    expect(() => loadEnv({ NGFW_DEV_WEAK_PASSWORDS: 'yes' })).toThrow(/NGFW_DEV_WEAK_PASSWORDS/);
  });

  it('refuses NGFW_DEV_WEAK_PASSWORDS in a production process (the packaged unit sets NODE_ENV=production)', () => {
    expect(() => loadEnv({ NGFW_DEV_WEAK_PASSWORDS: '1', NODE_ENV: 'production' })).toThrow(
      /NGFW_DEV_WEAK_PASSWORDS: development only/,
    );
    expect(loadEnv({ NGFW_DEV_WEAK_PASSWORDS: '0', NODE_ENV: 'production' }).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
    expect(loadEnv({ NODE_ENV: 'production' }).NGFW_DEV_WEAK_PASSWORDS).toBe(false);
  });

  it('NGFW_DATABASE_URL wins over the pg-test NGFW_PG_DSN', () => {
    expect(databaseUrl(loadEnv({ NGFW_PG_DSN: 'postgres://a/b' }))).toBe('postgres://a/b');
    expect(
      databaseUrl(loadEnv({ NGFW_PG_DSN: 'postgres://a/b', NGFW_DATABASE_URL: 'postgres://c/d' })),
    ).toBe('postgres://c/d');
  });

  it('never echoes offending values (they may be secrets)', () => {
    let msg = '';
    try {
      loadEnv({ NGFW_JWT_SECRET: 'short-secret-value' });
    } catch (e) {
      msg = (e as Error).message;
    }
    expect(msg).toContain('NGFW_JWT_SECRET');
    expect(msg).not.toContain('short-secret-value');
  });
});
