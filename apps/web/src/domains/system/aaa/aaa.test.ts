import { readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { describe, expect, it } from 'vitest';
import { Session } from '../../../auth/session';
import { readSsoHandover } from './queries';
import { ssoErrorKey } from './SsoButton';

const res = (status: number, b?: unknown) =>
  new Response(b === undefined ? null : JSON.stringify(b), {
    status,
    headers: { 'content-type': 'application/json' },
  });
const sessionBody = {
  accessToken: 'a.b.c',
  tokenType: 'Bearer',
  expiresIn: 900,
  user: { id: 1, username: 'admin', role: 'admin' },
};
const CH = 'A'.repeat(43);

function fake(routes: Record<string, (body: unknown) => Response>) {
  const seen: { path: string; body: unknown }[] = [];
  const f = async (r: Request) => {
    const path = new URL(r.url, 'http://localhost').pathname;
    const body: unknown = r.method === 'POST' ? await r.json() : undefined;
    seen.push({ path, body });
    const h = routes[`${r.method} ${path}`];
    if (!h) throw new TypeError('Failed to fetch');
    return h(body);
  };
  return { f, seen };
}

describe('F-aaa-login: two-step login in the Session', () => {
  it('a password login that needs MFA yields the challenge and no session', async () => {
    const { f } = fake({
      'POST /api/v1/auth/login': () =>
        res(200, { mfaRequired: true, challenge: CH, enrolled: true, expiresIn: 300 }),
    });
    const s = new Session(f, undefined, { channel: null });
    expect(await s.login('admin', 'pw')).toEqual({ mfa: true, challenge: CH, enrolled: true });
    expect(s.state.status).not.toBe('authenticated');
    s.dispose();
  });

  it('enrol → verify signs in and returns the recovery codes once', async () => {
    const { f, seen } = fake({
      'POST /api/v1/auth/mfa/enroll': () =>
        res(200, { secret: 'JBSWY3DPEHPK3PXP', otpauthUri: 'otpauth://totp/x' }),
      'POST /api/v1/auth/mfa/verify': () =>
        res(200, { ...sessionBody, recoveryCodes: ['0123456789'] }),
    });
    const s = new Session(f, undefined, { channel: null });
    expect(await s.mfaEnroll(CH, 'T'.repeat(32))).toEqual({
      secret: 'JBSWY3DPEHPK3PXP',
      otpauthUri: 'otpauth://totp/x',
    });
    expect(await s.mfaVerify(CH, { code: '123456' })).toEqual({ recoveryCodes: ['0123456789'] });
    expect(s.state.status).toBe('authenticated');
    expect(seen[0]).toEqual({ path: '/api/v1/auth/mfa/enroll', body: { challenge: CH, token: 'T'.repeat(32) } });
    expect(seen[1]).toEqual({
      path: '/api/v1/auth/mfa/verify',
      body: { challenge: CH, code: '123456' },
    });
    s.dispose();
  });

  it('a refused code is a failure, not a session', async () => {
    const { f } = fake({
      'POST /api/v1/auth/mfa/verify': () => res(401, { detail: 'invalid code or challenge' }),
    });
    const s = new Session(f, undefined, { channel: null });
    expect(await s.mfaVerify(CH, { recoveryCode: 'abcdef0123' })).toEqual({
      status: 401,
      detail: 'invalid code or challenge',
    });
    expect(s.state.status).not.toBe('authenticated');
    s.dispose();
  });
});

describe('F-aaa-login: OIDC handover fragment', () => {
  it('reads a well-formed challenge or error slug, ignores junk', () => {
    expect(readSsoHandover({ hash: `#mfa=${CH}&enrolled=0` })).toEqual({
      step: { mfa: true, challenge: CH, enrolled: false },
    });
    expect(readSsoHandover({ hash: '#error=no-role-mapping' })).toEqual({
      error: 'no-role-mapping',
    });
    expect(readSsoHandover({ hash: '#mfa=<script>&error=<b>' })).toEqual({});
    expect(ssoErrorKey('no-role-mapping')).toBe('sso.error.no-role-mapping');
    expect(ssoErrorKey('whatever')).toBe('sso.error.oidc-failed');
  });
});

describe('F-aaa-login locales', () => {
  const here = dirname(fileURLToPath(import.meta.url));
  const load = (lang: string) =>
    JSON.parse(readFileSync(join(here, '../../../locales', lang, 'aaa.json'), 'utf8')) as Record<
      string,
      unknown
    >;
  const keys = (o: Record<string, unknown>, p = ''): string[] =>
    Object.entries(o).flatMap(([k, v]) =>
      typeof v === 'string' ? [`${p}${k}`] : keys(v as Record<string, unknown>, `${p}${k}.`),
    );
  it('en and fa aaa.json have the same keys (parity)', () => {
    expect(keys(load('fa')).sort()).toEqual(keys(load('en')).sort());
    expect(keys(load('en')).length).toBeGreaterThan(40);
  });
});
