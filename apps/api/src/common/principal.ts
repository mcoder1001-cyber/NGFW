import type { FastifyRequest } from 'fastify';
import type { Role } from '../db/schema.js';

/** The authenticated caller, attached to the request by the AuthGuard. */
export interface Principal {
  id: number;
  username: string;
  role: Role;
  via: 'jwt' | 'apikey';
  /** Credential expiry (epoch seconds): WebSockets close then (review L3). */
  exp?: number;
  /** Login session (refresh-token family) of a JWT: logout closes its WebSockets. */
  sid?: string;
  /** Credential generation a JWT was issued under (D-097; API-key creation re-checks it, TD-2 verify V1). */
  gen?: number;
  /** API key id/name when `via` is `apikey`: the key is its own candidate-lock owner (D-093, TD-2 #5). */
  keyId?: string;
  keyName?: string;
}

/** What a handler records for the audit interceptor (before/after are already redacted). */
export interface AuditDetail {
  resource?: string;
  before?: unknown;
  after?: unknown;
}

export interface VrxRequestExtras {
  principal?: Principal;
  audit?: AuditDetail;
}

export type VrxRequest = FastifyRequest & VrxRequestExtras;

export const ROLE_RANK: Record<Role, number> = { readonly: 1, operator: 2, admin: 3 };

export function atLeast(role: Role, required: Role): boolean {
  return ROLE_RANK[role] >= ROLE_RANK[required];
}

export function lowerRole(a: Role, b: Role): Role {
  return ROLE_RANK[a] <= ROLE_RANK[b] ? a : b;
}

/**
 * The client's address (TD-10b, review 2.3b): Fastify walks X-Forwarded-For from the socket peer back through the
 * trusted proxies (VRX_TRUST_PROXY, default loopback: the product nginx, the vite proxy) and stops at the first
 * untrusted address — so behind the proxy every client has its own address, and a remote client cannot forge one.
 * Rate limits, the lockout, the audit log and the transport rule all use this one address.
 */
export function sourceIp(req: FastifyRequest): string {
  return req.ip;
}

/**
 * How the CLIENT reached the first trusted proxy, consistent with `sourceIp`: `req.ips` is
 * `[socket peer, …trusted hops…, client]`, and every trusted hop appended one X-Forwarded-Proto entry, so the entry
 * of the outermost trusted hop is the client's. No trusted hop → this socket. A trusted hop that did not say
 * (fewer entries than hops) → `http`: a password must never be accepted on a guess.
 */
export function requestProtocol(req: FastifyRequest): 'http' | 'https' {
  const socket = (req.raw.socket as { encrypted?: boolean } | undefined)?.encrypted
    ? 'https'
    : 'http';
  const ips = (req as { ips?: string[] }).ips;
  const hops = ips === undefined ? 0 : ips.length - 1;
  if (hops <= 0) return socket;
  const raw = req.headers['x-forwarded-proto'];
  const protos = (Array.isArray(raw) ? raw.join(',') : (raw ?? ''))
    .split(',')
    .map((s) => s.trim().toLowerCase())
    .filter((s) => s !== '');
  const at = protos.length - hops;
  // `wss`: a proxied WebSocket upgrade (vite's proxy writes ws/wss)
  return at >= 0 && (protos[at] === 'https' || protos[at] === 'wss') ? 'https' : 'http';
}

/** The first 64 bits of an IPv6 address, as `a:b:c:d::/64`. */
function v6prefix64(ip: string): string {
  const addr = ip.split('%')[0] ?? ip;
  const [head = '', tail] = addr.includes('::') ? addr.split('::') : [addr, undefined];
  const h = head === '' ? [] : head.split(':');
  const t = tail === undefined || tail === '' ? [] : tail.split(':');
  // an embedded IPv4 tail (…:1.2.3.4) is two groups
  const groups = h.length + t.length + ((t.at(-1) ?? h.at(-1) ?? '').includes('.') ? 1 : 0);
  const full = tail === undefined ? h : [...h, ...Array<string>(8 - groups).fill('0'), ...t];
  return `${full
    .slice(0, 4)
    .map((g) => parseInt(g, 16).toString(16))
    .join(':')}::/64`;
}

/**
 * Key of a client address for rate limits and the lockout (TD-10b): IPv4 as is (an IPv4-mapped IPv6 address as its
 * IPv4), IPv6 by its /64 — one host usually holds a whole /64, so a per-address key would be free to evade.
 */
export function clientKey(ip: string): string {
  const mapped = /^::ffff:(\d{1,3}(?:\.\d{1,3}){3})$/i.exec(ip);
  if (mapped) return mapped[1]!;
  return ip.includes(':') ? v6prefix64(ip) : ip;
}
