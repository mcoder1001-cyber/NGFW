import { describe, expect, it } from 'vitest';
import { AaaSchema } from '../index.js';

const oidc = (over: Record<string, unknown> = {}) => ({
  issuer: 'https://idp.example.net/realms/x',
  clientId: 'vrx',
  clientSecretRef: 'token/oidc',
  redirectUri: 'https://fw.example.net/api/v1/auth/oidc/callback',
  ...over,
});
const fails = (v: unknown) => {
  const r = AaaSchema.safeParse(v);
  return r.success ? null : r.error.issues.map((i) => i.path.join('/'));
};

describe('F-aaa-login: management.aaa.oidc contract', () => {
  it('defaults: absent; with a block: openid scopes and standard claims', () => {
    expect(AaaSchema.parse({}).oidc).toBeUndefined();
    expect(AaaSchema.parse({ order: ['local', 'oidc'], oidc: oidc() }).oidc).toMatchObject({
      scopes: ['openid', 'profile', 'email'],
      usernameClaim: 'preferred_username',
      roleClaim: 'groups',
    });
  });
  it('oidc in order needs the block; scopes need openid', () => {
    expect(fails({ order: ['local', 'oidc'] })).toEqual(['oidc']);
    expect(fails({ oidc: oidc({ scopes: ['profile'] }) })).toEqual(['oidc/scopes']);
  });
  it('https only, except a loopback development IdP; the secret is a token/ reference', () => {
    expect(fails({ oidc: oidc({ issuer: 'http://idp.example.net' }) })).toEqual(['oidc/issuer']);
    expect(fails({ oidc: oidc({ issuer: 'http://127.0.0.1:3174/x' }) })).toBeNull();
    expect(fails({ oidc: oidc({ clientSecretRef: 's3cr3t' }) })).toEqual(['oidc/clientSecretRef']);
  });
});
