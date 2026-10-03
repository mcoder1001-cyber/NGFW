import { createHash } from 'node:crypto';
import { describe, expect, it } from 'vitest';
import { allowedUrl, authorizationUrl, identityOf, jwksFor, pkce } from './oidc.js';

const cfg = {
  issuer: 'https://idp.example.net',
  clientId: 'ngfw',
  redirectUri: 'https://fw/api/v1/auth/oidc/callback',
  scopes: ['openid', 'profile'],
  usernameClaim: 'preferred_username',
  roleClaim: 'groups',
};

describe('oidc helpers', () => {
  it('JWKS: one instance per issuer reused by later callbacks; a changed jwks_uri replaces it', () => {
    const a = jwksFor('https://j.example.net', 'https://j.example.net/keys');
    expect(jwksFor('https://j.example.net', 'https://j.example.net/keys')).toBe(a);
    const b = jwksFor('https://j.example.net', 'https://j.example.net/keys2');
    expect(b).not.toBe(a);
    expect(jwksFor('https://j.example.net', 'https://j.example.net/keys2')).toBe(b);
    expect(jwksFor('https://other.example.net', 'https://j.example.net/keys2')).not.toBe(b);
  });
  it('PKCE S256: challenge = base64url(sha256(verifier))', () => {
    const p = pkce();
    expect(p.verifier).toMatch(/^[A-Za-z0-9_-]{43}$/);
    expect(p.challenge).toBe(createHash('sha256').update(p.verifier).digest('base64url'));
  });
  it('builds the authorisation URL with state, nonce and the S256 challenge', () => {
    const u = new URL(
      authorizationUrl(
        {
          issuer: cfg.issuer,
          authorization_endpoint: 'https://idp.example.net/auth',
          token_endpoint: '',
          jwks_uri: '',
        },
        cfg,
        { state: 's', nonce: 'n', challenge: 'c' },
      ),
    );
    expect(Object.fromEntries(u.searchParams)).toEqual({
      response_type: 'code',
      client_id: 'ngfw',
      redirect_uri: cfg.redirectUri,
      scope: 'openid profile',
      state: 's',
      nonce: 'n',
      code_challenge: 'c',
      code_challenge_method: 'S256',
    });
  });
  it('https only, http for loopback', () => {
    expect(allowedUrl('https://idp.example.net/x')).toBe(true);
    expect(allowedUrl('http://127.0.0.1:9/x')).toBe(true);
    expect(allowedUrl('http://idp.example.net/x')).toBe(false);
    expect(allowedUrl('javascript:alert(1)')).toBe(false);
  });
  it('username and groups from the ID token claims; unsafe names refused', () => {
    expect(identityOf({ sub: 'x', preferred_username: 'w1dave', groups: ['ops', 1] }, cfg)).toEqual(
      {
        username: 'w1dave',
        groups: ['ops'],
        subject: 'x',
      },
    );
    expect(
      identityOf({ sub: 's', preferred_username: 'w1dave', groups: 'ops' }, cfg).groups,
    ).toEqual(['ops']);
    expect(() => identityOf({ sub: 's', preferred_username: '../admin' }, cfg)).toThrow();
    expect(() => identityOf({ preferred_username: 'w1dave' }, cfg)).toThrow(/sub/);
    expect(() => identityOf({}, cfg)).toThrow();
  });
});
