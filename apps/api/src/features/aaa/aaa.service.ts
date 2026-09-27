import { Inject, Injectable, type OnModuleDestroy } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { isPlainObject } from '@ngfw/schema';
import { eq } from 'drizzle-orm';
import { ProblemError } from '../../common/problem.js';
import { ENV, type Env } from '../../config.js';
import { DatastoreService } from '../../datastore/datastore.service.js';
import { DB, type Db } from '../../db/db.js';
import { secret, type Role } from '../../db/schema.js';
import { Bus } from '../../infra/bus.js';
import { radiusAuthenticate, type RadiusServer } from './radius.js';

type Json = Record<string, unknown>;

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

/** What a backend answered for one (username, password) — before role mapping. */
interface BackendResult {
  reachable: boolean;
  authenticated: boolean;
  groups: string[];
  detail: string;
}

/**
 * F-aaa-login: the answer the login path acts on. `accept` with `role === null` means the credential was good but
 * no `roleMap` entry matched — the login is refused (least privilege: an external identity gets no implicit role).
 */
export type ExternalAuth =
  | { answer: 'accept'; groups: string[]; role: Role | null; detail: string }
  | { answer: 'reject'; detail: string }
  | { answer: 'unreachable'; detail: string }
  | { answer: 'unsupported'; detail: string };

/** The parts of `management.aaa` the login path needs. */
export interface AaaPolicy {
  order: readonly string[];
  fallbackLocal: boolean;
  mfa: { required: 'none' | 'admins' | 'all'; issuer: string };
}

const DEFAULT_POLICY: AaaPolicy = {
  order: ['local'],
  fallbackLocal: true,
  mfa: { required: 'none', issuer: 'vrx' },
};

/**
 * F-aaa: reads `management.aaa` from the running configuration and authenticates against the configured external
 * backends — both for the admin test route (increment 1, no session issued) and, since increment 2 (F-aaa-login),
 * for the login path itself (`AuthService.login` walks `aaa.order`). RADIUS (PAP) is implemented; LDAP/OIDC/SAML/
 * TACACS+ answer `unsupported`. Everything is in the API (D-040); secrets are read from the API's own store.
 *
 * The policy is cached and reloaded when a commit applies (same pattern as `AutoBlockService`), so a login does not
 * read the running document from PostgreSQL every time.
 */
@Injectable()
export class AaaService implements OnModuleDestroy {
  private cached: { aaa: Json; policy: AaaPolicy } | undefined;
  /** In-flight load, so a burst of logins triggers one read, not one each. */
  private loading: Promise<{ aaa: Json; policy: AaaPolicy }> | undefined;
  private readonly offCommit: () => void;

  constructor(
    @Inject(ENV) private readonly env: Env,
    private readonly ds: DatastoreService,
    private readonly moduleRef: ModuleRef,
    @Inject(DB) private readonly db: Db,
    private readonly bus: Bus,
  ) {
    this.offCommit = this.bus.onPublish((m) => {
      if (m.topic !== 'commit.events') return;
      const type = (m.data as { type?: string } | null)?.type;
      if (type === 'applied' || type === 'confirmed' || type === 'reverted') this.invalidate();
    });
  }

  onModuleDestroy(): void {
    this.offCommit();
  }

  /** Drop the cached policy; the next read reloads it. */
  invalidate(): void {
    this.cached = undefined;
    this.loading = undefined;
  }

  private async load(): Promise<{ aaa: Json; policy: AaaPolicy }> {
    if (this.cached !== undefined) return this.cached;
    this.loading ??= (async () => {
      const { doc } = await this.ds.getRunning();
      const mgmt = isPlainObject(doc['management']) ? (doc['management'] as Json) : {};
      const aaa = isPlainObject(mgmt['aaa']) ? (mgmt['aaa'] as Json) : {};
      const mfa = isPlainObject(aaa['mfa']) ? (aaa['mfa'] as Json) : {};
      const required = String(mfa['required'] ?? 'none');
      const loaded = {
        aaa,
        policy: {
          order: Array.isArray(aaa['order'])
            ? (aaa['order'] as unknown[]).map((m) => String(m))
            : DEFAULT_POLICY.order,
          fallbackLocal: aaa['fallbackLocal'] !== false,
          mfa: {
            required: (required === 'admins' || required === 'all' ? required : 'none') as
              'none' | 'admins' | 'all',
            issuer: String(mfa['issuer'] ?? DEFAULT_POLICY.mfa.issuer),
          },
        } satisfies AaaPolicy,
      };
      this.cached = loaded;
      return loaded;
    })().finally(() => {
      this.loading = undefined;
    });
    return this.loading;
  }

  /** The login policy from the running configuration; the defaults (`local` only) when nothing is configured. */
  async policy(): Promise<AaaPolicy> {
    return (await this.load()).policy;
  }

  private async aaa(): Promise<Json> {
    return (await this.load()).aaa;
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
  private roleFor(aaa: Json, groups: string[]): Role | null {
    const map = Array.isArray(aaa['roleMap']) ? (aaa['roleMap'] as Json[]) : [];
    const set = new Set(groups);
    for (const m of map) {
      if (set.has(String(m['group']))) return String(m['role']) as Role;
    }
    return null;
  }

  /** Validate a configured backend by authenticating (username, password) against it. Admin-only route. */
  async test(method: string, username: string, password: string): Promise<AaaTestResult> {
    const aaa = await this.aaa();
    if (method !== 'radius') {
      // LDAP/OIDC/SAML/TACACS+ backends land in a later increment
      throw new ProblemError(
        501,
        'not-implemented',
        'Not implemented',
        `the '${method}' backend is not implemented in this build yet`,
      );
    }
    const servers = this.radiusServers(aaa);
    if (servers.length === 0) {
      throw new ProblemError(
        400,
        'aaa-not-configured',
        'Bad request',
        'no RADIUS server is configured in management.aaa.radius',
      );
    }
    const r = await this.radiusBackend(servers, username, password);
    if (!r.reachable || !r.authenticated) {
      return { method: 'radius', role: null, ...r };
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

  /**
   * F-aaa-login: authenticate a login against one external method. Unlike `test()` this never throws for a
   * misconfigured backend — a method with no usable server answers `unreachable`, so the walk in `AuthService.login`
   * moves on to the next method (and the local fallback) instead of turning a login into a 500.
   */
  async authenticate(method: string, username: string, password: string): Promise<ExternalAuth> {
    if (method !== 'radius') {
      return { answer: 'unsupported', detail: `the '${method}' backend is not implemented` };
    }
    const aaa = await this.aaa();
    const servers = this.radiusServers(aaa);
    if (servers.length === 0) {
      return { answer: 'unreachable', detail: 'no RADIUS server configured' };
    }
    const r = await this.radiusBackend(servers, username, password);
    if (!r.reachable) return { answer: 'unreachable', detail: r.detail };
    if (!r.authenticated) return { answer: 'reject', detail: r.detail };
    return {
      answer: 'accept',
      groups: r.groups,
      role: this.roleFor(aaa, r.groups),
      detail: r.detail,
    };
  }

  private radiusServers(aaa: Json): Json[] {
    const radius = isPlainObject(aaa['radius']) ? (aaa['radius'] as Json) : {};
    return Array.isArray(radius['servers']) ? (radius['servers'] as Json[]) : [];
  }

  /**
   * Try the configured RADIUS servers in order: the first one that answers decides (accept or reject); a server that
   * is unreachable, or whose shared secret is missing from the store, is skipped and the next one tried.
   */
  private async radiusBackend(
    servers: Json[],
    username: string,
    password: string,
  ): Promise<BackendResult> {
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
          reachable: true,
          authenticated: false,
          groups: [],
          detail: r.message ?? 'rejected',
        };
      }
      return { reachable: true, authenticated: true, groups: r.groups, detail: 'authenticated' };
    }
    return { reachable: false, authenticated: false, groups: [], detail: lastError };
  }
}
