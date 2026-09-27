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
const CSECRET = 'VRX_TEST_PSK_FAAA_OIDC';
const MP = { 'content-type': 'application/merge-patch+json' };

describe('F-aaa-login OIDC e2e (PostgreSQL + fake IdP)', () => {
  let h: Harness;
  let admin: string;
  let idp: Server;
  let issuer: string;
  const codes = new Map<
    string,
    { challenge: string; nonce: string; user: string; groups: string[] }
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
          const basic = Buffer.from(`vrx:${CSECRET}`).toString('base64');
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
            .setAudience('vrx')
            .setSubject(`sub-${c.user}`)
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

    h = await startHarness({});
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
            clientId: 'vrx',
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
  async function authorize(user: string, groups: string[]) {
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
    });
    return { code, state: loc.searchParams.get('state')! };
  }

  it('the login page learns that SSO is offered', async () => {
    const m = await h.call(undefined, 'GET', '/api/v1/auth/methods');
    expect(m.body).toEqual({ oidc: true });
  });

  it('code + PKCE → session cookie with the mapped role; the state is single use', async () => {
    const { code, state } = await authorize('w1dave', ['ops']);
    const cb = await h.call(
      undefined,
      'GET',
      `/api/v1/auth/oidc/callback?code=${code}&state=${state}`,
    );
    expect(cb.status, cb.raw).toBe(302);
    expect(cb.headers['location']).toBe('/');
    const cookie = [cb.headers['set-cookie']]
      .flat()
      .map(String)
      .find((c) => c.startsWith('vrx_refresh='));
    expect(cookie).toBeDefined();
    const rf = await h.call(undefined, 'POST', '/api/v1/auth/refresh', undefined, {
      cookie: cookie!.split(';')[0]!,
    });
    expect(rf.status, rf.raw).toBe(200);
    expect(rf.body.user).toMatchObject({ username: 'w1dave', role: 'operator' });
    const again = await h.call(
      undefined,
      'GET',
      `/api/v1/auth/oidc/callback?code=${code}&state=${state}`,
    );
    expect(again.headers['location']).toBe('/login#error=oidc-failed');
  });

  it('unmapped groups → no-role-mapping; a forged state → refused', async () => {
    const a = await authorize('w1erin', ['other']);
    const cb = await h.call(
      undefined,
      'GET',
      `/api/v1/auth/oidc/callback?code=${a.code}&state=${a.state}`,
    );
    expect(cb.headers['location']).toBe('/login#error=no-role-mapping');
    const b = await authorize('w1dave', ['ops']);
    const forged = await h.call(
      undefined,
      'GET',
      `/api/v1/auth/oidc/callback?code=${b.code}&state=${randomBytes(24).toString('base64url')}`,
    );
    expect(forged.headers['location']).toBe('/login#error=oidc-failed');
  });

  it('the client secret appears in neither GET config nor the audit log', async () => {
    const cfg = await h.call(admin, 'GET', '/api/v1/config/management');
    const audit = await h.db.execute(sql`select * from audit_log`);
    const blob = JSON.stringify([cfg.body, audit.rows]);
    expect(blob).not.toContain(CSECRET);
    expect(blob).toMatch(/"method":"oidc"/);
  });
});
