import { createHash, randomBytes } from 'node:crypto';
import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { exportJWK, generateKeyPair, SignJWT } from 'jose';
import { sql } from 'drizzle-orm';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { startHarness, type Harness } from '../support/harness.js';

/**
 * F-aaa-login e2e: OpenID Connect code flow + PKCE against an in-process fake IdP on 127.0.0.1 (discovery, JWKS,
 * token endpoint that checks the client secret, the code and the PKCE verifier). The callback sets the refresh
 * cookie; the state is single use; the client secret never shows in GET config or the audit log.
 */
const CSECRET = 'NGFW_TEST_PSK_FAAA_OIDC';
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-aaa-login OIDC e2e (PostgreSQL + fake IdP)', () => {
  let h: Harness;
  let admin: string;
  let idp: Server;
  let issuer: string;
  const codes = new Map<
    string,
    { challenge: string; nonce: string; user: string; groups: string[]; sub: string }
  >();

  beforeAll(async () => {
    const { publicKey, privateKey } = await generateKeyPair('RS256');
    const jwk = { ...(await exportJWK(publicKey)), kid: 'k1', alg: 'RS256', use: 'sig' };
    idp = createServer((req, res) => {
      const url = new URL(req.url ?? '/', issuer);
      const json = (v: unknown, status = 200) => {
        res.writeHead(status, { 'content-type': 'application/json' });
        res.end(JSON.stringify(v));
      };
      if (url.pathname === '/.well-known/openid-configuration') {
        return json({
          issuer,
          authorization_endpoint: `${issuer}/authorize`,
          token_endpoint: `${issuer}/token`,
          jwks_uri: `${issuer}/jwks`,
        });
      }
      if (url.pathname === '/jwks') return json({ keys: [jwk] });
      if (url.pathname === '/token' && req.method === 'POST') {
        let body = '';
        req.on('data', (c: Buffer) => (body += c.toString()));
        req.on('end', () => {
          const f = new URLSearchParams(body);
          const basic = Buffer.from(`ngfw:${CSECRET}`).toString('base64');
          if (req.headers.authorization !== `Basic ${basic}`)
            return json({ error: 'invalid_client' }, 401);
          const c = codes.get(f.get('code') ?? '');
          codes.delete(f.get('code') ?? '');
          const v = createHash('sha256')
            .update(f.get('code_verifier') ?? '')
            .digest('base64url');
          if (c === undefined || v !== c.challenge) return json({ error: 'invalid_grant' }, 400);
          void new SignJWT({ nonce: c.nonce, preferred_username: c.user, groups: c.groups })
            .setProtectedHeader({ alg: 'RS256', kid: 'k1' })
            .setIssuer(issuer)
            .setAudience('ngfw')
            .setSubject(c.sub)
            .setIssuedAt()
            .setExpirationTime('5m')
            .sign(privateKey)
            .then((idToken) => json({ id_token: idToken, token_type: 'Bearer' }));
        });
        return;
      }
      json({ error: 'not found' }, 404);
    });
    await new Promise<void>((r) => idp.listen(0, '127.0.0.1', r));
    issuer = `http://127.0.0.1:${(idp.address() as AddressInfo).port}`;

    h = await startHarness({ NGFW_LOGIN_RATE_PER_MIN: '200' });
    admin = await h.login('admin', h.adminPassword);
    const s = await h.call(admin, 'POST', '/api/v1/secrets', {
      kind: 'token',
      name: 'oidc',
      value: CSECRET,
    });
    expect(s.status, s.raw).toBe(200);
    const p = await h.call(
      admin,
      'PATCH',
      '/api/v1/config/management',
      {
        aaa: {
          order: ['local', 'oidc'],
          oidc: {
            issuer,
            clientId: 'ngfw',
            clientSecretRef: 'token/oidc',
            redirectUri: 'https://fw.example.net/api/v1/auth/oidc/callback',
          },
          roleMap: [{ group: 'ops', role: 'operator' }],
        },
      },
      MP,
    );
    expect(p.status, p.raw).toBe(200);
    expect((await h.call(admin, 'POST', '/api/v1/config/commit?comment=oidc')).status).toBe(200);
  });
  afterAll(async () => {
    idp?.close();
    await h?.close();
  });

  /** start → (the IdP "logs the user in") → the callback URL parameters. */
  async function authorize(user: string, groups: string[], sub = `sub-${user}`) {
    const start = await h.call(undefined, 'GET', '/api/v1/auth/oidc/start');
    expect(start.status, start.raw).toBe(302);
    const loc = new URL(String(start.headers['location']));
    expect(loc.origin + loc.pathname).toBe(`${issuer}/authorize`);
    expect(loc.searchParams.get('code_challenge_method')).toBe('S256');
    const code = randomBytes(16).toString('hex');
    codes.set(code, {
      challenge: loc.searchParams.get('code_challenge')!,
      nonce: loc.searchParams.get('nonce')!,
      user,
      groups,
      sub,
    });
    // review 3: the browser binding cookie set by /start
    const bind = [start.headers['set-cookie']]
      .flat()
      .map(String)
      .find((c) => c.startsWith('ngfw_oidc='));
    expect(bind).toMatch(/HttpOnly/i);
    expect(bind).toMatch(/SameSite=Lax/i);
    return { code, state: loc.searchParams.get('state')!, cookie: bind!.split(';')[0]! };
  }

  /** The IdP's redirect back, in the same browser (with the binding cookie) or another one (without). */
  const callback = (
    a: { code: string; state: string; cookie: string },
    o: { state?: string; browser?: 'same' | 'other' } = {},
  ) =>
    h.call(
      undefined,
      'GET',
      `/api/v1/auth/oidc/callback?code=${a.code}&state=${o.state ?? a.state}`,
      undefined,
      o.browser === 'other' ? {} : { cookie: a.cookie },
    );

  it('the login page learns that SSO is offered', async () => {
    const m = await h.call(undefined, 'GET', '/api/v1/auth/methods');
    expect(m.body).toEqual({ oidc: true });
  });

  it('code + PKCE → session cookie with the mapped role; the state is single use', async () => {
    const a = await authorize('w1dave', ['ops']);
    const cb = await callback(a);
    expect(cb.status, cb.raw).toBe(302);
    expect(cb.headers['location']).toBe('/');
    const cookie = [cb.headers['set-cookie']]
      .flat()
      .map(String)
      .find((c) => c.startsWith('ngfw_refresh='));
    expect(cookie).toBeDefined();
    const rf = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: cookie!.split(';')[0]!,
    });
    expect(rf.status, rf.raw).toBe(200);
    expect(rf.body.user).toMatchObject({ username: 'w1dave', role: 'operator' });
    const again = await callback(a);
    expect(again.headers['location']).toBe('/login#error=oidc-failed');
  });

  it('unmapped groups → no-role-mapping; a forged state → refused', async () => {
    const a = await authorize('w1erin', ['other']);
    const cb = await callback(a);
    expect(cb.headers['location']).toBe('/login#error=no-role-mapping');
    const b = await authorize('w1dave', ['ops']);
    const forged = await callback(b, { state: randomBytes(24).toString('base64url') });
    expect(forged.headers['location']).toBe('/login#error=oidc-failed');
  });

  it('review 3: a callback URL replayed in another browser (no binding cookie) is refused', async () => {
    const a = await authorize('w1dave', ['ops']);
    const other = await callback(a, { browser: 'other' });
    expect(other.headers['location']).toBe('/login#error=oidc-failed');
    expect([other.headers['set-cookie']].flat().map(String).join()).not.toMatch(/ngfw_refresh=[^;]/);
    // a foreign browser's own cookie does not fit either
    const b = await authorize('w1dave', ['ops']);
    const mixed = await callback({ ...b, cookie: a.cookie });
    expect(mixed.headers['location']).toBe('/login#error=oidc-failed');
  });

  it('review 1: the account is bound to the IdP subject — a name claim cannot take over an account', async () => {
    const before = await h.db.execute(sql`select id from app_user where username = 'w1dave'`);
    // another subject claiming preferred_username=W1Dave (case-folded) → refused, no account touched
    const evil = await authorize('W1Dave', ['ops'], 'sub-evil');
    expect((await callback(evil)).headers['location']).toBe('/login#error=oidc-failed');
    // an LDAP-bound account: an OIDC user with preferred_username=w1alice cannot get it
    await h.db.execute(
      sql`insert into app_user (username, role, source) values ('w1alice', 'admin', 'external')`,
    );
    await h.db.execute(
      sql`insert into aaa_external_identity (user_id, method, subject) select id, 'ldap', 'uid=w1alice,dc=x' from app_user where username = 'w1alice'`,
    );
    const alice = await authorize('w1alice', ['ops'], 'sub-alice');
    expect((await callback(alice)).headers['location']).toBe('/login#error=oidc-failed');
    const ids = await h.db.execute(
      sql`select u.username, i.method from app_user u join aaa_external_identity i on i.user_id = u.id where lower(u.username) in ('w1dave', 'w1alice') order by 1`,
    );
    expect(ids.rows).toEqual([
      { username: 'w1alice', method: 'ldap' },
      { username: 'w1dave', method: 'oidc' },
    ]);
    // the right subject still gets its own account
    const good = await callback(await authorize('w1dave', ['ops']));
    expect(good.headers['location']).toBe('/');
    const after = await h.db.execute(sql`select id from app_user where username = 'w1dave'`);
    expect(after.rows).toEqual(before.rows);
  });

  it('review 4: /auth/oidc/start and /callback answer 403 tls-required to a remote plain-HTTP peer', async () => {
    for (const url of ['/api/v1/auth/oidc/start', '/api/v1/auth/oidc/callback?code=x&state=y']) {
      const res = await h.app.inject({ method: 'GET', url, remoteAddress: '192.0.2.10' });
      expect(res.statusCode, url).toBe(403);
      expect(res.body).toMatch(/tls-required/);
    }
  });

  it('the client secret appears in neither GET config nor the audit log', async () => {
    const cfg = await h.call(admin, 'GET', '/api/v1/config/management');
    const audit = await h.db.execute(sql`select * from audit_log`);
    const blob = JSON.stringify([cfg.body, audit.rows]);
    expect(blob).not.toContain(CSECRET);
    expect(blob).toMatch(/"method":"oidc"/);
  });
});
