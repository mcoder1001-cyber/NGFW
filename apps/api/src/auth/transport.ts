import type { FastifyRequest } from 'fastify';
import { ProblemError } from '../common/problem.js';

/** Loopback peers: the local TLS terminator (nginx, docs/01-architecture.md), the dev proxy, tests. */
export function isLoopback(ip: string | undefined): boolean {
  if (ip === undefined) return false;
  return ip === '::1' || /^127\./.test(ip) || /^::ffff:127\./i.test(ip);
}

/**
 * "Plaintext over TLS only" (TD-2 #1): the API itself speaks plain HTTP on loopback behind the product nginx, which
 * terminates TLS. A password is therefore accepted over a TLS socket of this process or from a loopback peer; a
 * remote peer on plain HTTP (the API bound to a public address by mistake) is refused. The body has been parsed by
 * then — the check refuses to ACT on a password that crossed the network in clear. X-Forwarded-* are not trusted.
 * ASSUMPTION (review L1, D-100 (1)): every local relay is TLS-terminated — the product nginx config (P10) must never
 * proxy `/api` from plain `:80` (redirect only); dev proxies and SSH tunnels count as loopback.
 * Used by every route whose body carries a password: password set (TD-2 #1), login and the API-key step-up (TD-4).
 */
export function secureTransport(req: FastifyRequest): boolean {
  return req.protocol === 'https' || isLoopback(req.ip);
}

/** 403 `tls-required`: the answer of every password-carrying route to a remote plain-HTTP peer. */
export function tlsRequired(): ProblemError {
  return new ProblemError(
    403,
    'tls-required',
    'TLS required',
    'passwords are accepted over TLS only (connect through https)',
  );
}
