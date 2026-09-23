import { Inject, Injectable, Logger } from '@nestjs/common';
import { and, count, eq, sql } from 'drizzle-orm';
import { AuditService } from '../audit/audit.service.js';
import { ENV, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import { lowerRole, type Principal } from '../common/principal.js';
import { DB, type Db } from '../db/db.js';
import { apiKey, appUser, ROLES, type Role } from '../db/schema.js';
import { hashPassword, verifyPassword } from './password.js';
import { apiKeyHash, newApiKeyToken, TokensService } from './tokens.service.js';

export interface LoginResult {
  accessToken: string;
  tokenType: 'Bearer';
  expiresIn: number;
  refreshToken: string;
  user: { id: number; username: string; role: Role };
}

/**
 * Local users (argon2id), JWT access + rotating refresh, API keys, login rate limit and lockout (P06 §6).
 * Every login outcome is audited here with the real reason; the client only ever sees "invalid credentials".
 */
@Injectable()
export class AuthService {
  private readonly log = new Logger('Auth');

  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(ENV) private readonly env: Env,
    private readonly tokens: TokensService,
    private readonly audit: AuditService,
  ) {}

  /** D-048: the API seeds the first admin when there is no user at all. Returns true when it created one. */
  async seedBootstrapAdmin(): Promise<boolean> {
    const [c] = await this.db.select({ n: count() }).from(appUser);
    if ((c?.n ?? 0) > 0) return false;
    const password = this.env.VRX_BOOTSTRAP_ADMIN_PASSWORD;
    if (password === undefined) {
      this.log.warn('no users and VRX_BOOTSTRAP_ADMIN_PASSWORD is not set: nobody can log in');
      return false;
    }
    const inserted = await this.db
      .insert(appUser)
      .values({
        username: this.env.VRX_BOOTSTRAP_ADMIN_USER,
        passwordHash: await hashPassword(password),
        role: 'admin',
        source: 'bootstrap',
      })
      .onConflictDoNothing()
      .returning({ id: appUser.id });
    if (inserted.length > 0)
      this.log.log(`bootstrap admin '${this.env.VRX_BOOTSTRAP_ADMIN_USER}' created`);
    return inserted.length > 0;
  }

  async login(username: string, password: string, ip: string): Promise<LoginResult> {
    const fail = async (reason: string, userId: number | null, status = 401) => {
      await this.audit.write({
        userId,
        username,
        sourceIp: ip,
        action: 'auth.login',
        resource: username,
        after: { reason },
        result: 'failure',
        status,
      });
      return status === 429
        ? problems.tooMany('too many login attempts; try again in a minute')
        : problems.unauthorized('invalid credentials');
    };
    if ((await this.tokens.hit(`login:${ip}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw await fail('rate-limited', null, 429);
    }
    const [u] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    const ok = await verifyPassword(u?.passwordHash, password);
    if (u === undefined) throw await fail('unknown-user', null);
    const now = new Date();
    if (u.lockedUntil !== null && u.lockedUntil > now) throw await fail('locked', u.id);
    if (!ok) {
      const failures = u.failedLogins + 1;
      const lock = failures >= this.env.VRX_LOGIN_MAX_FAILURES;
      await this.db
        .update(appUser)
        .set({
          failedLogins: lock ? 0 : failures,
          lockedUntil: lock
            ? new Date(now.getTime() + this.env.VRX_LOGIN_LOCKOUT_SEC * 1000)
            : u.lockedUntil,
        })
        .where(eq(appUser.id, u.id));
      throw await fail(lock ? 'bad-password-locked' : 'bad-password', u.id);
    }
    if (u.disabled) throw await fail('disabled', u.id);
    await this.db
      .update(appUser)
      .set({ failedLogins: 0, lockedUntil: null, lastLogin: now })
      .where(eq(appUser.id, u.id));
    await this.audit.write({
      userId: u.id,
      username: u.username,
      sourceIp: ip,
      action: 'auth.login',
      resource: u.username,
      result: 'success',
      status: 200,
    });
    return this.session({ id: u.id, username: u.username, role: u.role });
  }

  private async session(
    user: { id: number; username: string; role: Role },
    family?: string,
  ): Promise<LoginResult> {
    const refresh = await this.tokens.issueRefresh(user.id, family);
    return {
      accessToken: await this.tokens.signAccess(user),
      tokenType: 'Bearer',
      expiresIn: this.tokens.accessTtl,
      refreshToken: refresh.token,
      user,
    };
  }

  async refresh(token: string | undefined, ip: string): Promise<LoginResult> {
    if (!token) throw problems.unauthorized('no refresh token');
    const r = await this.tokens.consumeRefresh(token);
    if (r === null || 'reuse' in r) {
      if (r !== null) {
        await this.audit.write({
          userId: null,
          username: null,
          sourceIp: ip,
          action: 'auth.refresh',
          resource: null,
          after: { reason: 'refresh-token-reuse: family revoked' },
          result: 'failure',
          status: 401,
        });
      }
      throw problems.unauthorized('invalid refresh token');
    }
    const [u] = await this.db.select().from(appUser).where(eq(appUser.id, r.uid));
    if (u === undefined || u.disabled || (u.lockedUntil !== null && u.lockedUntil > new Date())) {
      throw problems.unauthorized('invalid refresh token');
    }
    return this.session({ id: u.id, username: u.username, role: u.role }, r.family);
  }

  async logout(token: string | undefined): Promise<void> {
    if (token) await this.tokens.revokeRefresh(token);
  }

  /** `Authorization: Bearer <jwt>` or `Authorization: ApiKey <key>` → principal, or null. */
  async authenticate(header: string | undefined): Promise<Principal | null> {
    if (!header) return null;
    const [scheme, value] = header.split(/\s+/, 2);
    if (!value) return null;
    if (scheme?.toLowerCase() === 'bearer') {
      const c = await this.tokens.verifyAccess(value);
      return c === null ? null : { ...c, via: 'jwt' };
    }
    if (scheme?.toLowerCase() === 'apikey') return this.authenticateApiKey(value);
    return null;
  }

  private async authenticateApiKey(token: string): Promise<Principal | null> {
    if (!/^vrxk_[A-Za-z0-9_-]{43}$/.test(token)) return null;
    const [row] = await this.db
      .select({ key: apiKey, user: appUser })
      .from(apiKey)
      .innerJoin(appUser, eq(appUser.id, apiKey.userId))
      .where(eq(apiKey.hash, apiKeyHash(token)));
    if (row === undefined) return null;
    const now = new Date();
    if (row.user.disabled || (row.key.expiresAt !== null && row.key.expiresAt <= now)) return null;
    void this.db
      .update(apiKey)
      .set({ lastUsed: now })
      .where(eq(apiKey.id, row.key.id))
      .catch(() => undefined);
    const cap = row.key.scopes.find((s): s is Role => (ROLES as readonly string[]).includes(s));
    return {
      id: row.user.id,
      username: row.user.username,
      role: cap === undefined ? row.user.role : lowerRole(row.user.role, cap),
      via: 'apikey',
    };
  }

  async createApiKey(
    user: Principal,
    name: string,
    role: Role | undefined,
    expiresInDays: number | undefined,
  ) {
    const token = newApiKeyToken();
    const scope = role === undefined ? user.role : lowerRole(user.role, role);
    const [row] = await this.db
      .insert(apiKey)
      .values({
        userId: user.id,
        name,
        hash: apiKeyHash(token),
        scopes: [scope],
        expiresAt:
          expiresInDays === undefined ? null : new Date(Date.now() + expiresInDays * 86400_000),
      })
      .returning();
    return { id: row!.id, name, role: scope, expiresAt: row!.expiresAt, key: token };
  }

  async listApiKeys(user: Principal) {
    const rows = await this.db
      .select({
        id: apiKey.id,
        name: apiKey.name,
        scopes: apiKey.scopes,
        expiresAt: apiKey.expiresAt,
        lastUsed: apiKey.lastUsed,
        createdAt: apiKey.createdAt,
      })
      .from(apiKey)
      .where(eq(apiKey.userId, user.id));
    return rows.map(({ scopes, ...r }) => ({ ...r, role: scopes[0] ?? null }));
  }

  async deleteApiKey(user: Principal, id: string): Promise<void> {
    if (!/^[0-9a-f-]{36}$/.test(id)) throw problems.notFound(`API key ${id} does not exist`);
    const cond =
      user.role === 'admin'
        ? eq(apiKey.id, id)
        : and(eq(apiKey.id, id), eq(apiKey.userId, user.id));
    const deleted = await this.db.delete(apiKey).where(cond).returning({ id: apiKey.id });
    if (deleted.length === 0) throw problems.notFound(`API key ${id} does not exist`);
  }

  async changePassword(user: Principal, current: string, next: string): Promise<void> {
    const [u] = await this.db.select().from(appUser).where(eq(appUser.id, user.id));
    if (u === undefined || !(await verifyPassword(u.passwordHash, current))) {
      throw problems.forbidden('the current password is wrong');
    }
    await this.db
      .update(appUser)
      .set({ passwordHash: await hashPassword(next), failedLogins: 0, lockedUntil: sql`null` })
      .where(eq(appUser.id, user.id));
  }

  async me(user: Principal) {
    const [u] = await this.db
      .select({
        id: appUser.id,
        username: appUser.username,
        role: appUser.role,
        lastLogin: appUser.lastLogin,
      })
      .from(appUser)
      .where(eq(appUser.id, user.id));
    if (u === undefined) throw problems.unauthorized('user no longer exists');
    return { ...u, effectiveRole: user.role, via: user.via };
  }
}
