import { Inject, Injectable } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { isPlainObject } from '@ngfw/schema';
import { eq } from 'drizzle-orm';
import { ProblemError } from '../../common/problem.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret } from '../../db/schema.js';
import { radiusAuthenticate, type RadiusServer } from './radius.js';

type Json = Record<string, unknown>;

export interface AaaTestResult {
  method: string;
  reachable: boolean;
  authenticated: boolean;
  /** External groups returned (before role mapping). */
  groups: string[];
  /** The local role the roleMap would assign, or null when none matched. */
  role: 'admin' | 'operator' | 'readonly' | null;
  detail: string;
}

/**
 * F-aaa (increment 1): reads `management.aaa` from the running configuration and validates an external backend by
 * authenticating a test credential against it — without issuing a session. RADIUS (PAP) is implemented; LDAP/OIDC/
 * SAML/TACACS+ and the login-order integration + MFA enforcement are the next increment. Everything is in the API
 * (D-040); secrets are read from the API's own store.
 */
@Injectable()
export class AaaService {
  constructor(
    @Inject(ENV) private readonly env: Env,
    private readonly ds: DatastoreService,
    private readonly moduleRef: ModuleRef,
    @Inject(DB) private readonly db: Db,
  ) {}

  private async aaa(): Promise<Json> {
    const { doc } = await this.ds.getRunning();
    const mgmt = isPlainObject(doc['management']) ? (doc['management'] as Json) : {};
    return isPlainObject(mgmt['aaa']) ? (mgmt['aaa'] as Json) : {};
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

  /** Map external groups to a local role via management.aaa.roleMap (first match wins; null when none). */
  private roleFor(aaa: Json, groups: string[]): AaaTestResult['role'] {
    const map = Array.isArray(aaa['roleMap']) ? (aaa['roleMap'] as Json[]) : [];
    const set = new Set(groups);
    for (const m of map) {
      if (set.has(String(m['group']))) return String(m['role']) as AaaTestResult['role'];
    }
    return null;
  }

  /** Validate a configured backend by authenticating (username, password) against it. Admin-only route. */
  async test(method: string, username: string, password: string): Promise<AaaTestResult> {
    const aaa = await this.aaa();
    if (method === 'radius') return this.testRadius(aaa, username, password);
    // LDAP/OIDC/SAML/TACACS+ backends land in the next increment
    throw new ProblemError(
      501,
      'not-implemented',
      'Not implemented',
      `the '${method}' backend is not implemented in this build yet`,
    );
  }

  private async testRadius(aaa: Json, username: string, password: string): Promise<AaaTestResult> {
    const radius = isPlainObject(aaa['radius']) ? (aaa['radius'] as Json) : {};
    const servers = Array.isArray(radius['servers']) ? (radius['servers'] as Json[]) : [];
    if (servers.length === 0) {
      throw new ProblemError(
        400,
        'aaa-not-configured',
        'Bad request',
        'no RADIUS server is configured in management.aaa.radius',
      );
    }
    let lastError = 'no server answered';
    for (const s of servers) {
      const ref = String(s['secretRef'] ?? '');
      const secretVal = ref ? await this.readSecret(ref) : null;
      if (secretVal === null) {
        lastError = `secret ${ref} not found`;
        continue;
      }
      const server: RadiusServer = {
        address: String(s['address']),
        authPort: Number(s['authPort'] ?? 1812),
        secret: secretVal,
        timeoutMs: Number(s['timeoutSec'] ?? 5) * 1000,
      };
      const r = await radiusAuthenticate(server, username, password);
      if (r.status === 'unreachable') {
        lastError = r.error;
        continue; // try the next server
      }
      if (r.status === 'reject') {
        return {
          method: 'radius',
          reachable: true,
          authenticated: false,
          groups: [],
          role: null,
          detail: r.message ?? 'rejected',
        };
      }
      const role = this.roleFor(aaa, r.groups);
      return {
        method: 'radius',
        reachable: true,
        authenticated: true,
        groups: r.groups,
        role,
        detail: role
          ? `authenticated; role ${role}`
          : 'authenticated; no roleMap match (login would be refused)',
      };
    }
    return {
      method: 'radius',
      reachable: false,
      authenticated: false,
      groups: [],
      role: null,
      detail: lastError,
    };
  }
}
