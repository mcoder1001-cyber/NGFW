import { Inject, Injectable } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { isPlainObject } from '@ngfw/schema';
import { eq } from 'drizzle-orm';
import { ProblemError } from '../../common/problem.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret, type Role } from '../../db/schema.js';
import { ldapAuthenticate, type LdapClientFactory } from './ldap.js';
import { radiusAuthenticate } from './radius.js';

type Json = Record<string, unknown>;

/** The external methods this build implements behind the one `AuthBackend` shape. */
export const EXTERNAL_METHODS = ['radius', 'ldap'] as const;
export type ExternalMethod = (typeof EXTERNAL_METHODS)[number];

export type MfaRequired = 'none' | 'admins' | 'all';

/** What `management.aaa` says about logins (read from the RUNNING configuration). */
export interface LoginPolicy {
  order: string[];
  fallbackLocal: boolean;
  mfaRequired: MfaRequired;
  mfaIssuer: string;
}

/** One external backend's answer (the `AuthBackend` contract: accept with groups | reject | unreachable). */
export type BackendResult =
  | { status: 'accept'; method: ExternalMethod; server: string; groups: string[] }
  | { status: 'reject'; method: ExternalMethod; server: string; message: string }
  | { status: 'unreachable'; method: ExternalMethod; error: string };

export interface AaaTestResult {
  method: string;
  reachable: boolean;
  authenticated: boolean;
  /** External groups returned (before role mapping). */
  groups: string[];
  /** The local role the roleMap would assign, or null when none matched. */
  role: Role | null;
  detail: string;
}

const RANK: Record<Role, number> = { readonly: 1, operator: 2, admin: 3 };
const POLICY_TTL_MS = 5_000;

/** roleMap: the HIGHEST privilege among the user's mapped groups (open question, flagged); null = unmapped. */
export function mapRole(
  roleMap: readonly { group: string; role: Role }[],
  groups: readonly string[],
): Role | null {
  const set = new Set(groups.map((g) => g.toLowerCase()));
  let best: Role | null = null;
  for (const m of roleMap) {
    if (!set.has(m.group.toLowerCase())) continue;
    if (best === null || RANK[m.role] > RANK[best]) best = m.role;
  }
  return best;
}

/** MFA policy for a role: `admins` covers the admin role only; `all` every role. */
export function mfaRequiredFor(policy: MfaRequired, role: Role): boolean {
  return policy === 'all' || (policy === 'admins' && role === 'admin');
}

/**
 * F-aaa: external authentication backends (RADIUS PAP, LDAP bind+search) and the login policy of `management.aaa`.
 * Everything is in the API (D-040). Shared secrets / bind passwords are resolved from the secret store at use time,
 * per attempt, and never cached, logged or returned. The login-order walk itself is in AuthService.login (one call
 * per method into `authenticate`).
 */
@Injectable()
export class AaaService {
  private policyCache: { at: number; value: LoginPolicy } | undefined;
  /** Test seam: the LDAP client factory (default: ldapts). */
  ldapFactory: LdapClientFactory | undefined;

  constructor(
    private readonly ds: DatastoreService,
    private readonly moduleRef: ModuleRef,
    @Inject(DB) private readonly db: Db,
  ) {}

  private async aaa(): Promise<Json> {
    const { doc } = await this.ds.getRunning();
    const mgmt = isPlainObject(doc['management']) ? (doc['management'] as Json) : {};
    return isPlainObject(mgmt['aaa']) ? (mgmt['aaa'] as Json) : {};
  }

  /** The login policy, read fresh (login path). */
  async policy(): Promise<LoginPolicy> {
    const aaa = await this.aaa();
    const mfa = isPlainObject(aaa['mfa']) ? (aaa['mfa'] as Json) : {};
    const req = mfa['required'];
    const value: LoginPolicy = {
      order: Array.isArray(aaa['order']) ? (aaa['order'] as unknown[]).map(String) : ['local'],
      fallbackLocal: aaa['fallbackLocal'] !== false,
      mfaRequired: req === 'admins' || req === 'all' ? req : 'none',
      mfaIssuer: typeof mfa['issuer'] === 'string' ? mfa['issuer'] : 'vrx',
    };
    this.policyCache = { at: Date.now(), value };
    return value;
  }

  /** The last policy read (fallback when the datastore cannot be read right now), or undefined. */
  lastPolicy(): LoginPolicy | undefined {
    return this.policyCache?.value;
  }

  /** The login policy, at most POLICY_TTL_MS old (the per-request MFA check of access tokens). */
  async cachedPolicy(): Promise<LoginPolicy> {
    const c = this.policyCache;
    if (c !== undefined && Date.now() - c.at < POLICY_TTL_MS) return c.value;
    return this.policy();
  }

  private async readSecret(ref: string): Promise<string | null> {
    const [row] = await this.db
      .select({ ciphertext: secret.ciphertext })
      .from(secret)
      .where(eq(secret.ref, ref))
      .limit(1);
    if (!row) return null;
    const { SecretsService } = await import('../../secrets/secrets.service.js');
    return this.moduleRef.get(SecretsService, { strict: false }).decrypt(row.ciphertext, ref);
  }

  private roleMap(aaa: Json): { group: string; role: Role }[] {
    const map = Array.isArray(aaa['roleMap']) ? (aaa['roleMap'] as Json[]) : [];
    return map
      .filter((m) => typeof m['group'] === 'string' && String(m['role']) in RANK)
      .map((m) => ({ group: String(m['group']), role: String(m['role']) as Role }));
  }

  /** Map external groups to a local role through the running `roleMap`. */
  async roleFor(groups: readonly string[]): Promise<Role | null> {
    return mapRole(this.roleMap(await this.aaa()), groups);
  }

  private servers(aaa: Json, method: ExternalMethod): Json[] {
    const block = isPlainObject(aaa[method]) ? (aaa[method] as Json) : {};
    return Array.isArray(block['servers']) ? (block['servers'] as Json[]) : [];
  }

  /**
   * One method of the login order: every configured server in turn until one answers (failover on unreachable only;
   * an answer — accept or reject — is final). The credential goes to the backend only; nothing here logs it.
   */
  async authenticate(
    method: ExternalMethod,
    username: string,
    password: string,
  ): Promise<BackendResult> {
    const aaa = await this.aaa();
    const servers = this.servers(aaa, method);
    if (servers.length === 0)
      return { status: 'unreachable', method, error: 'no server configured' };
    let lastError = 'no server answered';
    for (const s of servers) {
      const r =
        method === 'radius'
          ? await this.tryRadius(s, username, password)
          : await this.tryLdap(s, username, password);
      if (r.status === 'unreachable') {
        lastError = r.error;
        continue;
      }
      return r;
    }
    return { status: 'unreachable', method, error: lastError };
  }

  private async tryRadius(s: Json, username: string, password: string): Promise<BackendResult> {
    const address = String(s['address']);
    const authPort = Number(s['authPort'] ?? 1812);
    const server = `${address}:${authPort}`;
    const ref = String(s['secretRef'] ?? '');
    const psk = ref ? await this.readSecret(ref) : null;
    if (psk === null) {
      return {
        status: 'unreachable',
        method: 'radius',
        error: `${server}: secret ${ref} not found`,
      };
    }
    const r = await radiusAuthenticate(
      { address, authPort, secret: psk, timeoutMs: Number(s['timeoutSec'] ?? 5) * 1000 },
      username,
      password,
    );
    if (r.status === 'unreachable') {
      return { status: 'unreachable', method: 'radius', error: `${server}: ${r.error}` };
    }
    if (r.status === 'reject') {
      return { status: 'reject', method: 'radius', server, message: r.message ?? 'rejected' };
    }
    return { status: 'accept', method: 'radius', server, groups: r.groups };
  }

  private async tryLdap(s: Json, username: string, password: string): Promise<BackendResult> {
    const server = String(s['url']);
    const ref = String(s['bindPasswordRef'] ?? '');
    const bindPassword = ref ? await this.readSecret(ref) : null;
    if (bindPassword === null) {
      return { status: 'unreachable', method: 'ldap', error: `${server}: secret ${ref} not found` };
    }
    const r = await ldapAuthenticate(
      {
        url: server,
        bindDn: String(s['bindDn']),
        bindPassword,
        baseDn: String(s['baseDn']),
        userFilter: String(s['userFilter'] ?? '(uid=%s)'),
        groupAttr: String(s['groupAttr'] ?? 'memberOf'),
        startTls: s['startTls'] === true,
        timeoutMs: Number(s['timeoutSec'] ?? 5) * 1000,
      },
      username,
      password,
      this.ldapFactory,
    );
    if (r.status === 'unreachable') {
      return { status: 'unreachable', method: 'ldap', error: `${server}: ${r.error}` };
    }
    if (r.status === 'reject') {
      return { status: 'reject', method: 'ldap', server, message: r.message };
    }
    return { status: 'accept', method: 'ldap', server, groups: r.groups };
  }

  /** Admin route: validate a configured backend by authenticating (username, password) against it; no session. */
  async test(method: string, username: string, password: string): Promise<AaaTestResult> {
    if (!(EXTERNAL_METHODS as readonly string[]).includes(method)) {
      throw new ProblemError(
        501,
        'not-implemented',
        'Not implemented',
        `the '${method}' backend is not implemented in this build yet`,
      );
    }
    const aaa = await this.aaa();
    const m = method as ExternalMethod;
    if (this.servers(aaa, m).length === 0) {
      throw new ProblemError(
        400,
        'aaa-not-configured',
        'Bad request',
        `no ${m} server is configured in management.aaa.${m}`,
        [{ pointer: '/method', message: `management.aaa.${m}.servers is empty` }],
      );
    }
    const r = await this.authenticate(m, username, password);
    const none = { method, groups: [] as string[], role: null };
    if (r.status === 'unreachable') {
      return { ...none, reachable: false, authenticated: false, detail: r.error };
    }
    if (r.status === 'reject') {
      return { ...none, reachable: true, authenticated: false, detail: r.message };
    }
    const role = mapRole(this.roleMap(aaa), r.groups);
    return {
      method,
      reachable: true,
      authenticated: true,
      groups: r.groups,
      role,
      detail: role
        ? `authenticated; role ${role}`
        : 'authenticated; no roleMap match (login would be refused)',
    };
  }
}
