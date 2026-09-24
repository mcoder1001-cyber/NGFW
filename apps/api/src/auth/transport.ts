import type { FastifyRequest } from 'fastify';
import { requestProtocol, sourceIp } from '../common/principal.js';
import { ProblemError } from '../common/problem.js';

/** Loopback clients: a browser or CLI on this host, an SSH tunnel, tests. */
export function isLoopback(ip: string | undefined): boolean {
  if (ip === undefined) return false;
  return ip === '::1' || /^127\./.test(ip) || /^::ffff:127\./i.test(ip);
}

/**
 * "Plaintext over TLS only" (TD-2 #1): the API itself speaks plain HTTP on loopback behind the product nginx, which
 * terminates TLS. A password is accepted when the CLIENT reached us over TLS or is itself on this host; a remote
 * client on plain HTTP is refused. The body has been parsed by then — the check refuses to ACT on a password that
 * crossed the network in clear.
 * TD-10b (review 2.3b): the client and its protocol come from the trusted proxy (VRX_TRUST_PROXY, default loopback) —
 * `sourceIp` and `requestProtocol` resolve the same hop, so the rate limit, the lockout, the audit row and this rule
 * see one client. Before, every relayed request looked like a loopback peer and counted as TLS (D-100 (1)'s
 * assumption); now a trusted proxy must SAY `X-Forwarded-Proto: https` (P10's nginx sets it) — a relay of plain
 * HTTP from a remote browser (tools/app's vite on :8080, nginx proxying :80 by mistake) is refused. A loopback peer
 * that forwards nothing (SSH tunnel, CLI on the box) is its own client and stays accepted.
 * Used by every route whose body carries a password: password set (TD-2 #1), login and the API-key step-up (TD-4).
 */
export function secureTransport(req: FastifyRequest): boolean {
  return requestProtocol(req) === 'https' || isLoopback(sourceIp(req));
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
