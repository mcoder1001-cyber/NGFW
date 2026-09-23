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

/** Client address as Fastify sees it (no proxy trust by default). */
export function sourceIp(req: FastifyRequest): string {
  return req.ip;
}
