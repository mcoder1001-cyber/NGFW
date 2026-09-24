import { Inject, Injectable, Logger } from '@nestjs/common';
import { and, count, eq, isNull, lte, or, sql } from 'drizzle-orm';
import { AuditService } from '../audit/audit.service.js';
import { Bus } from '../infra/bus.js';
import { ENV, type Env } from '../config.js';
import { problems } from '../common/problem.js';
import { lowerRole, type Principal } from '../common/principal.js';
import { DB, type Db } from '../db/db.js';
import { apiKey, appUser, ROLES, type Role } from '../db/schema.js';
import { releaseKeyLocks } from '../datastore/pg-repo.js';
import { hashPassword, verifyPassword } from './password.js';
import { PASSWORD_MIN } from '../users/password-policy.js';
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
    private readonly bus: Bus,
  ) {}

  /** D-048: the API seeds the first admin when there is no user at all. Returns true when it created one. */
  async seedBootstrapAdmin(): Promise<boolean> {
    if (this.env.VRX_DEV_WEAK_PASSWORDS) {
      this.log.warn(
        `VRX_DEV_WEAK_PASSWORDS is on: new passwords shorter than ${PASSWORD_MIN} characters are accepted — development only`,
      );
    }
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
    // D-097 (review H2, verify V1): hash and credential generation come from ONE row read, so the session below is
    // issued under the generation that goes with the password that was checked
    const [u] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    const ok = await verifyPassword(u?.passwordHash, password);
    if (u === undefined) throw await fail('unknown-user', null);
    const now = new Date();
    if (u.lockedUntil !== null && u.lockedUntil > now) throw await fail('locked', u.id);
    if (!ok) {
      const lockedNow = await this.registerFailure(u.id);
      throw await fail(lockedNow ? 'bad-password-locked' : 'bad-password', u.id);
    }
    if (u.disabled) throw await fail('disabled', u.id);
    // success only if the account is not locked at THIS moment (a parallel failure may have just locked it) and the
    // password was not reset since the row was read (the reset's UPDATE bumps credential_gen; this one waits for it)
    const unlocked = await this.db
      .update(appUser)
      .set({ failedLogins: 0, lockedUntil: null, lastLogin: now })
      .where(
        and(
          eq(appUser.id, u.id),
          eq(appUser.credentialGen, u.credentialGen),
          or(isNull(appUser.lockedUntil), lte(appUser.lockedUntil, sql`now()`)),
        ),
      )
      .returning({ id: appUser.id });
    if (unlocked.length === 0) {
      const [again] = await this.db
        .select({ gen: appUser.credentialGen })
        .from(appUser)
        .where(eq(appUser.id, u.id));
      throw await fail(
        again?.gen === u.credentialGen ? 'locked' : 'credentials-changed-during-login',
        u.id,
      );
    }
    const s = await this.session({ id: u.id, username: u.username, role: u.role }, u.credentialGen);
    if (s === null) throw await fail('credentials-changed-during-login', u.id);
    await this.audit.write({
      userId: u.id,
      username: u.username,
      sourceIp: ip,
      action: 'auth.login',
      resource: u.username,
      result: 'success',
      status: 200,
    });
    return s;
  }

  /**
   * One failed password check for `userId` (login, or a wrong `current` on a password change — D-097, review M2).
   * ONE atomic statement (P06 review H1): concurrent failures each add 1 — no read-modify-write race. PostgreSQL
   * evaluates every SET expression against the old row, so both CASEs see the same pre-increment value. Returns
   * whether the account is locked now.
   */
  async registerFailure(userId: number): Promise<boolean> {
    const max = this.env.VRX_LOGIN_MAX_FAILURES;
    const hit = sql`${appUser.failedLogins} + 1 >= ${max}`;
    const [row] = await this.db
      .update(appUser)
      .set({
        failedLogins: sql`case when ${hit} then 0 else ${appUser.failedLogins} + 1 end`,
        lockedUntil: sql`case when ${hit} then now() + make_interval(secs => ${this.env.VRX_LOGIN_LOCKOUT_SEC}) else ${appUser.lockedUntil} end`,
      })
      .where(eq(appUser.id, userId))
      .returning({ lockedUntil: appUser.lockedUntil });
    return row?.lockedUntil != null && row.lockedUntil > new Date();
  }

  /**
   * Refresh chain + access token under credential generation `gen` (from app_user), or null when the chain was
   * revoked meanwhile (D-097). `family` continues a chain (refresh); without it a new one starts (login).
   */
  private async session(
    user: { id: number; username: string; role: Role },
    gen: number,
    family?: string,
  ): Promise<LoginResult | null> {
    const refresh = await this.tokens.issueRefresh(user.id, gen, family);
    if (refresh === null) return null;
    return {
      accessToken: await this.tokens.signAccess({ ...user, sid: refresh.family, gen: refresh.gen }),
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
    // verify V1/V3: the chain continues only under the generation PostgreSQL holds NOW (a reset commits it first)
    const s = await this.session(
      { id: u.id, username: u.username, role: u.role },
      u.credentialGen,
      r.family,
    );
    if (s === null) throw problems.unauthorized('invalid refresh token');
    return s;
  }

  async logout(token: string | undefined): Promise<void> {
    if (!token) return;
    await this.tokens.revokeRefresh(token);
    const sid = this.tokens.familyOf(token);
    if (sid) this.bus.sessions({ sid });
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
      keyId: row.key.id,
      keyName: row.key.name,
      ...(row.key.expiresAt ? { exp: Math.floor(row.key.expiresAt.getTime() / 1000) } : {}),
    };
  }

  /**
   * TD-2 verify V1 (D-102): key creation conflicts with a password reset in PostgreSQL. The caller's app_user row is
   * read `FOR SHARE`, which waits for a reset in progress (its UPDATE holds the row), and the caller is re-checked
   * against the committed state: a JWT must carry the current credential generation (or be the session a self-service
   * change kept), an API key must still exist. Only then is the key inserted, in the same transaction — so a reset
   * either sees the new key (and deletes it) or the request is refused.
   */
  async createApiKey(
    user: Principal,
    name: string,
    role: Role | undefined,
    expiresInDays: number | undefined,
  ) {
    const token = newApiKeyToken();
    const scope = role === undefined ? user.role : lowerRole(user.role, role);
    const row = await this.db.transaction(async (tx) => {
      const [u] = await tx
        .select({ gen: appUser.credentialGen })
        .from(appUser)
        .where(eq(appUser.id, user.id))
        .for('share');
      if (u === undefined) throw problems.unauthorized('user no longer exists');
      if (user.via === 'apikey') {
        const [k] =
          user.keyId === undefined
            ? []
            : await tx.select({ id: apiKey.id }).from(apiKey).where(eq(apiKey.id, user.keyId));
        if (k === undefined) throw problems.unauthorized('the API key was revoked');
      } else if (!this.tokens.sessionCurrent(user, u.gen)) {
        throw problems.unauthorized('the password was changed; log in again');
      }
      const [r] = await tx
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
      return r!;
    });
    return { id: row.id, name, role: scope, expiresAt: row.expiresAt, key: token };
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
    const deleted = await this.db.transaction(async (tx) => {
      const rows = await tx.delete(apiKey).where(cond).returning({ id: apiKey.id });
      // review L4: a deleted key's candidate lock goes with it (candidate discarded, never handed to its user)
      await releaseKeyLocks(
        tx,
        rows.map((r) => r.id),
      );
      return rows;
    });
    if (deleted.length === 0) throw problems.notFound(`API key ${id} does not exist`);
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
