import { createHash, randomBytes } from 'node:crypto';
import { createRemoteJWKSet, jwtVerify, type JWTPayload } from 'jose';

/**
 * F-aaa-login: OpenID Connect authorisation-code flow with PKCE (S256) and a nonce, on `jose` + fetch (no extra
 * dependency). The ID token is verified against the IdP's JWKS (signature, `iss`, `aud` = client id, `exp`, `nonce`);
 * the userinfo endpoint is not used. The client secret is passed in by the caller (resolved from the secret store at
 * use time) and only ever sent to the discovered token endpoint (HTTP Basic, RFC 6749 §2.3.1).
 */

export interface OidcConfig {
  issuer: string;
  clientId: string;
  redirectUri: string;
  scopes: string[];
  usernameClaim: string;
  roleClaim: string;
}

export interface OidcDiscovery {
  issuer: string;
  authorization_endpoint: string;
  token_endpoint: string;
  jwks_uri: string;
}

const b64url = (n: number) => randomBytes(n).toString('base64url');
const TIMEOUT_MS = 5_000;

/** A new PKCE pair (RFC 7636, S256). */
export function pkce(): { verifier: string; challenge: string } {
  const verifier = b64url(32);
  return { verifier, challenge: createHash('sha256').update(verifier).digest('base64url') };
}

export function newState(): { state: string; nonce: string } {
  return { state: b64url(24), nonce: b64url(24) };
}

/** Same rule as the schema: https, or http to a loopback host. */
export function allowedUrl(u: string): boolean {
  try {
    const url = new URL(u);
    if (url.protocol === 'https:') return true;
    return url.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname);
  } catch {
    return false;
  }
}

const cache = new Map<string, { at: number; d: OidcDiscovery }>();

/** `<issuer>/.well-known/openid-configuration`; its `issuer` must be the configured one (OIDC Discovery §4.3). */
export async function discover(issuer: string): Promise<OidcDiscovery> {
  const hit = cache.get(issuer);
  if (hit !== undefined && Date.now() - hit.at < 300_000) return hit.d;
  const url = `${issuer.replace(/\/+$/, '')}/.well-known/openid-configuration`;
  const res = await fetch(url, { signal: AbortSignal.timeout(TIMEOUT_MS), redirect: 'error' });
  if (!res.ok) throw new Error(`discovery: HTTP ${res.status}`);
  const d = (await res.json()) as Partial<OidcDiscovery>;
  if (d.issuer !== issuer) throw new Error('discovery: issuer mismatch');
  for (const k of ['authorization_endpoint', 'token_endpoint', 'jwks_uri'] as const) {
    if (typeof d[k] !== 'string' || !allowedUrl(d[k])) throw new Error(`discovery: bad ${k}`);
  }
  const out = d as OidcDiscovery;
  cache.set(issuer, { at: Date.now(), d: out });
  return out;
}

const jwksSets = new Map<string, { uri: string; set: ReturnType<typeof createRemoteJWKSet> }>();

/**
 * F-aaa-hardening: one remote JWKS per issuer, reused across callbacks (jose caches the keys and rate-limits
 * refetches per instance). A different jwks_uri for the issuer (config or discovery changed) replaces it.
 */
export function jwksFor(issuer: string, jwksUri: string): ReturnType<typeof createRemoteJWKSet> {
  const hit = jwksSets.get(issuer);
  if (hit !== undefined && hit.uri === jwksUri) return hit.set;
  const set = createRemoteJWKSet(new URL(jwksUri), { timeoutDuration: TIMEOUT_MS });
  jwksSets.set(issuer, { uri: jwksUri, set });
  return set;
}

export function authorizationUrl(
  d: OidcDiscovery,
  c: OidcConfig,
  p: { state: string; nonce: string; challenge: string },
): string {
  const u = new URL(d.authorization_endpoint);
  u.searchParams.set('response_type', 'code');
  u.searchParams.set('client_id', c.clientId);
  u.searchParams.set('redirect_uri', c.redirectUri);
  u.searchParams.set('scope', c.scopes.join(' '));
  u.searchParams.set('state', p.state);
  u.searchParams.set('nonce', p.nonce);
  u.searchParams.set('code_challenge', p.challenge);
  u.searchParams.set('code_challenge_method', 'S256');
  return u.toString();
}

export interface OidcIdentity {
  username: string;
  groups: string[];
  subject: string;
}

/** Login names an OIDC claim may produce (the shadow user's name). */
const USERNAME = /^[A-Za-z0-9][A-Za-z0-9._@-]{0,63}$/;

/** Code → tokens → verified ID token → (username, groups). Throws on anything that does not verify. */
export async function redeem(
  d: OidcDiscovery,
  c: OidcConfig,
  clientSecret: string,
  code: string,
  verifier: string,
  nonce: string,
): Promise<OidcIdentity> {
  const basic = Buffer.from(
    `${encodeURIComponent(c.clientId)}:${encodeURIComponent(clientSecret)}`,
  ).toString('base64');
  const res = await fetch(d.token_endpoint, {
    method: 'POST',
    headers: {
      'content-type': 'application/x-www-form-urlencoded',
      authorization: `Basic ${basic}`,
    },
    body: new URLSearchParams({
      grant_type: 'authorization_code',
      code,
      redirect_uri: c.redirectUri,
      code_verifier: verifier,
    }).toString(),
    signal: AbortSignal.timeout(TIMEOUT_MS),
    redirect: 'error',
  });
  if (!res.ok) throw new Error(`token endpoint: HTTP ${res.status}`);
  const tok = (await res.json()) as { id_token?: unknown };
  if (typeof tok.id_token !== 'string') throw new Error('token endpoint: no id_token');
  const jwks = jwksFor(d.issuer, d.jwks_uri);
  const { payload } = await jwtVerify(tok.id_token, jwks, {
    issuer: d.issuer,
    audience: c.clientId,
    algorithms: ['RS256', 'ES256', 'PS256', 'EdDSA'],
  });
  if (payload['nonce'] !== nonce) throw new Error('id_token: nonce mismatch');
  return identityOf(payload, c);
}

export function identityOf(payload: JWTPayload, c: OidcConfig): OidcIdentity {
  const name = payload[c.usernameClaim];
  if (typeof name !== 'string' || !USERNAME.test(name)) {
    throw new Error(`id_token: claim ${c.usernameClaim} missing or not a valid login name`);
  }
  const raw = payload[c.roleClaim];
  const groups = Array.isArray(raw)
    ? raw.filter((g): g is string => typeof g === 'string')
    : typeof raw === 'string'
      ? [raw]
      : [];
  // review 1: `sub` is the stable, IdP-assigned identity the account is bound to; a token without it is refused
  if (typeof payload.sub !== 'string' || payload.sub.length === 0 || payload.sub.length > 255) {
    throw new Error('id_token: no sub');
  }
  return { username: name, groups, subject: payload.sub };
}
