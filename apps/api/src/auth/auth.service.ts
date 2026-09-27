import { createHash, randomBytes } from 'node:crypto';
import { Inject, Injectable, Logger } from '@nestjs/common';
import { and, count, eq, isNull, lte, or, sql } from 'drizzle-orm';
import { AuditService } from '../audit/audit.service.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { Bus } from '../infra/bus.js';
import { VALKEY, type Valkey } from '../infra/valkey.js';
import { ENV, type Env } from '../config.js';
import { problems, ProblemError } from '../common/problem.js';
import { clientKey, lowerRole, ROLE_RANK, type Principal } from '../common/principal.js';
import { DB, type Db } from '../db/db.js';
import { aaaExternalIdentity, apiKey, appUser, ROLES, type Role } from '../db/schema.js';
import {
  AaaService,
  EXTERNAL_METHODS,
  mfaRequiredFor,
  type ExternalMethod,
  type LoginPolicy,
} from '../features/aaa/aaa.service.js';
import { MfaService } from '../features/aaa/mfa.service.js';
import { authorizationUrl, discover, newState, pkce, redeem } from '../features/aaa/oidc.js';
import { releaseKeyLocks } from '../datastore/pg-repo.js';
import { Lockout, type LockSubject } from './lockout.js';
import { hashPassword, verifyPassword } from './password.js';
import { PASSWORD_MIN } from '../users/password-policy.js';
import { apiKeyHash, newApiKeyToken, TokensService } from './tokens.service.js';
import { tlsRequired } from './transport.js';

/**
 * PostgreSQL advisory lock (single 64-bit key space; TD-10a's commit lock uses the two-int space, which never overlaps)
 * that serialises the account-wide lock decisions of `registerFailure` (review L1).
 */
const LOCK_DECISION_KEY = 7_310_100_001;

/**
 * 403 `locked`: the step-up of a locked account (the caller is authenticated, so the state is not hidden). ONE body for
 * every `locked` answer (TD-4 review H1): whether the guess reached argon2 before the lock or not, the answer is the
 * same, so a burst of guesses learns nothing from which `locked` it got.
 */
function accountLocked(): ProblemError {
  return new ProblemError(
    403,
    'locked',
    'Account locked',
    'the account is locked after too many failed password checks',
  );
}

/**
 * F-aaa-login: an external identity has no local password, so a password step-up (API-key creation, own MFA
 * set-up) cannot be satisfied from its login session. Refused (D-100 option (b); re-authenticating against the
 * backend is the listed alternative).
 */
function externalNoStepUp(): ProblemError {
  return new ProblemError(
    403,
    'external-principal',
    'Not available for external identities',
    'this account logs in through an external identity source and has no local password to confirm this with',
  );
}

/** The step-up of `POST /auth/api-keys` (D-100 (2)): the caller's current password and how the request arrived. */
export interface KeyStepUp {
  current: string | undefined;
  /** `secureTransport(req)`: TLS or a loopback peer. */
  secure: boolean;
}

/** F-aaa-login: a login challenge lives this long (seconds) and takes this many wrong answers. */
const MFA_CHALLENGE_SEC = 300;
const MFA_CHALLENGE_TRIES = 5;
/** F-aaa-login: an OIDC authorisation request (state, PKCE verifier, nonce) lives this long (seconds). */
const OIDC_STATE_SEC = 600;

const sha256hex = (s: string) => createHash('sha256').update(s).digest('hex');
const b64url = (n: number) => randomBytes(n).toString('base64url');

/** F-aaa-login: the first factor passed and a second one is needed (no session yet, no cookie). */
export interface MfaChallenge {
  mfaRequired: true;
  /** single-use, 5 min; sent back to `POST /auth/mfa/verify` (and `/auth/mfa/enroll` when not enrolled yet) */
  challenge: string;
  /** false: the policy requires a factor this user has not set up — enrol first (the challenge allows only that) */
  enrolled: boolean;
  expiresIn: number;
}

interface StoredChallenge {
  uid: number;
  gen: number;
  enrolled: boolean;
  via: string;
}

/** A user whose first factor passed (local password or external backend + roleMap). */
interface LocalOk {
  id: number;
  username: string;
  role: Role;
  credentialGen: number;
}

export interface LoginResult {
  accessToken: string;
  tokenType: 'Bearer';
  /** Seconds the access token lives (≤ VRX_ACCESS_TTL_SEC, ≤ the session's end). */
  expiresIn: number;
  refreshToken: string;
  /** Seconds the refresh cookie lives: the idle TTL capped by VRX_SESSION_MAX_SEC (TD-10b, review 2.3d). */
  refreshMaxAge: number;
  user: { id: number; username: string; role: Role };
}

/**
 * Local users (argon2id), JWT access + rotating refresh, API keys, login rate limit and lockout (P06 §6; TD-10b: the
 * login lockout is per (user, client address), the last admin is only throttled — lockout.ts).
 * Every login outcome is audited here with the real reason; the client only ever sees "invalid credentials".
 * TD-10b (review 2.3e): refresh failures and logouts are audited too (auth.refresh / auth.logout).
 */
@Injectable()
export class AuthService {
  private readonly log = new Logger('Auth');
  private readonly lockout: Lockout;
  /** last system_event per throttled last admin (ms), so an attack writes one event a minute, not one per guess */
  private readonly throttleEventAt = new Map<number, number>();
  /** F-aaa-login: login sessions known to have passed the second factor (positive cache of `mfasid:<sid>`) */
  private readonly mfaSids = new Set<string>();
  private readonly kv: Valkey;

  constructor(
    @Inject(DB) private readonly db: Db,
    @Inject(ENV) private readonly env: Env,
    private readonly tokens: TokensService,
    private readonly audit: AuditService,
    private readonly bus: Bus,
    @Inject(VALKEY) kv: Valkey,
    private readonly events: SystemEventsService,
    private readonly aaa: AaaService,
    private readonly mfa: MfaService,
  ) {
    this.kv = kv;
    this.lockout = new Lockout(kv, db, env);
  }

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

  /**
   * `secure`: the request came over TLS or from a loopback peer (`secureTransport`, D-100 (1)).
   *
   * F-aaa-login: the login walks `management.aaa.order` (running configuration). `local` answers for users that exist
   * locally with a password (a name unknown locally falls through to the next method); an external method (RADIUS,
   * LDAP) that ANSWERS is final — accept → roleMap → shadow user → session, reject → 401 (open question: local login
   * is NOT tried after an external reject, only when every external method was unreachable and `fallbackLocal`).
   * Every path ends in `finish()`, which demands the second factor when the MFA policy or the user's enrolment says so
   * (then the answer is a short-lived challenge, not a session). With the default order `[local]` this is the P06
   * login unchanged.
   */
  async login(
    username: string,
    password: string,
    ip: string,
    secure: boolean,
  ): Promise<LoginResult | MfaChallenge> {
    const fail = this.failer(username, ip);
    // D-100 (1), TD-4: the transport check comes FIRST — before the rate limiter, the user lookup and argon2 — so a
    // password sent in clear by a remote peer is never acted on: no failed login counted, the lockout untouched
    if (!secure) throw await fail('tls-required', null, 403);
    // TD-10b (review 2.3b): `ip` is the client behind the trusted proxy, so this bucket is per client, not global
    if ((await this.tokens.hit(`login:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw await fail('rate-limited', null, 429);
    }
    const policy = await this.aaa.policy();
    const external = policy.order.filter((m): m is ExternalMethod =>
      (EXTERNAL_METHODS as readonly string[]).includes(m),
    );
    if (external.length === 0)
      return this.finish(await this.localLogin(username, password, ip), ip, policy, 'local');
    // F-aaa-login: external attempts are also bounded per login NAME (all clients together), so a distributed spray
    // cannot lock the account out in the directory behind us; the per-client bucket above still applies
    if (
      (await this.tokens.hit(`extlogin:${username.toLowerCase()}`, 60)) >
      this.env.VRX_LOGIN_RATE_PER_MIN
    ) {
      throw await fail('rate-limited-name', null, 429);
    }
    let answered = false;
    for (const m of policy.order) {
      if (m === 'local') {
        const [u] = await this.db
          .select({ id: appUser.id, source: appUser.source })
          .from(appUser)
          .where(eq(appUser.username, username));
        if (u !== undefined && u.source !== 'external') {
          return this.finish(await this.localLogin(username, password, ip), ip, policy, 'local');
        }
        continue;
      }
      if (!(EXTERNAL_METHODS as readonly string[]).includes(m)) continue; // tacacs/oidc/saml: not in this build
      const method = m as ExternalMethod;
      // an existing shadow user locked from this client address takes no guess (and the directory sees none)
      const [shadow] = await this.db.select().from(appUser).where(eq(appUser.username, username));
      if (shadow !== undefined && shadow.source === 'external') {
        const st = await this.lockout.state(shadow, ip);
        if (st !== 'open') throw await fail(st, shadow.id);
      }
      const r = await this.aaa.authenticate(method, username, password);
      if (r.status === 'unreachable') {
        await this.auditExternal(username, ip, method, null, 'unreachable', r.error);
        continue;
      }
      answered = true;
      if (r.status === 'reject') {
        await this.auditExternal(username, ip, method, r.server, 'reject', r.message);
        if (shadow !== undefined && shadow.source === 'external') {
          const o = await this.lockout.fail(shadow, ip);
          if (o === 'throttled') this.lastAdminThrottled(shadow, ip);
          throw await fail(`${method}-reject`, shadow.id);
        }
        throw await fail(`${method}-reject`, null);
      }
      await this.auditExternal(username, ip, method, r.server, 'accept', null);
      const u = await this.provision(username, method, r.groups, ip, fail);
      return this.finish(u, ip, policy, method);
    }
    // every external method was unreachable (none answered): the break-glass local login, if allowed
    if (!answered && policy.fallbackLocal && !policy.order.includes('local')) {
      const [u] = await this.db
        .select({ source: appUser.source })
        .from(appUser)
        .where(eq(appUser.username, username));
      if (u !== undefined && u.source !== 'external') {
        return this.finish(
          await this.localLogin(username, password, ip),
          ip,
          policy,
          'local-fallback',
        );
      }
    }
    throw await fail(answered ? 'no-method-accepted' : 'all-methods-unreachable', null);
  }

  /** The audited failure of a login (the client only ever sees "invalid credentials", 429 or the transport 403). */
  private failer(username: string, ip: string) {
    return async (reason: string, userId: number | null, status = 401) => {
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
        : status === 403
          ? tlsRequired()
          : problems.unauthorized('invalid credentials');
    };
  }

  /** F-aaa-login: one audit row per external backend answer (method, server, result — never the password). */
  private async auditExternal(
    username: string,
    ip: string,
    method: string,
    server: string | null,
    result: 'accept' | 'reject' | 'unreachable',
    detail: string | null,
  ): Promise<void> {
    await this.audit.write({
      userId: null,
      username,
      sourceIp: ip,
      action: 'auth.external',
      resource: username,
      after: { method, server, result, ...(detail === null ? {} : { detail }) },
      result: result === 'accept' ? 'success' : 'failure',
      status: result === 'accept' ? 200 : 401,
    });
  }

  /**
   * F-aaa-login: an accepted external identity → roleMap → its shadow app_user (source `external`, NO password hash,
   * so it can never log in locally). Refused: no mapped group (403 `no-role-mapping`; an existing shadow user's
   * sessions end — the directory removed its rights), a name owned by a LOCAL user (401: an external identity never
   * takes over a local account), a disabled shadow user. A lower role than before bumps the credential generation
   * (the older, higher-privileged sessions end), like a demotion through the config (TD-10b).
   */
  private async provision(
    username: string,
    method: string,
    groups: string[],
    ip: string,
    fail: ReturnType<AuthService['failer']>,
  ): Promise<LocalOk> {
    const role = await this.aaa.roleFor(groups);
    const [prev] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    if (prev !== undefined && prev.source !== 'external')
      throw await fail('name-owned-by-local-user', prev.id);
    if (role === null) {
      if (prev !== undefined) {
        await this.db
          .update(appUser)
          .set({ credentialGen: sql`${appUser.credentialGen} + 1` })
          .where(eq(appUser.id, prev.id));
        await this.tokens.revokeUser(prev.id, prev.credentialGen + 1);
      }
      await this.audit.write({
        userId: prev?.id ?? null,
        username,
        sourceIp: ip,
        action: 'auth.login',
        resource: username,
        after: { reason: 'no-role-mapping', method, groups },
        result: 'failure',
        status: 403,
      });
      throw new ProblemError(
        403,
        'no-role-mapping',
        'Forbidden',
        'the identity source accepted the credentials, but none of the user’s groups is mapped to a role',
      );
    }
    if (prev?.disabled) throw await fail('disabled', prev.id);
    if (prev?.lockedUntil != null && prev.lockedUntil > new Date())
      throw await fail('locked', prev.id);
    const demoted = prev !== undefined && ROLE_RANK[role] < ROLE_RANK[prev.role];
    const [row] = await this.db
      .insert(appUser)
      .values({ username, role, source: 'external', passwordHash: null, lastLogin: new Date() })
      .onConflictDoUpdate({
        target: appUser.username,
        set: {
          role,
          lastLogin: new Date(),
          failedLogins: 0,
          ...(demoted ? { credentialGen: sql`${appUser.credentialGen} + 1` } : {}),
        },
        where: eq(appUser.source, 'external'),
      })
      .returning();
    if (row === undefined) throw await fail('name-owned-by-local-user', null);
    if (demoted) await this.tokens.revokeUser(row.id, row.credentialGen);
    await this.db
      .insert(aaaExternalIdentity)
      .values({
        userId: row.id,
        method,
        subject: username,
        lastGroups: groups,
        lastLogin: new Date(),
      })
      .onConflictDoUpdate({
        target: aaaExternalIdentity.userId,
        set: { method, subject: username, lastGroups: groups, lastLogin: new Date() },
      });
    if (!(await this.lockout.admit(row, ip))) throw await fail('locked', row.id);
    return { id: row.id, username: row.username, role: row.role, credentialGen: row.credentialGen };
  }

  /** P06/TD-10b local password check with the lockout; returns the user whose session may be issued. */
  private async localLogin(username: string, password: string, ip: string): Promise<LocalOk> {
    const fail = this.failer(username, ip);
    // D-097 (review H2, verify V1): hash and credential generation come from ONE row read, so the session below is
    // issued under the generation that goes with the password that was checked
    const [u] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    const ok = await verifyPassword(u?.passwordHash, password);
    if (u === undefined) throw await fail('unknown-user', null);
    const now = new Date();
    // account-wide: a wrong password checked INSIDE a session of this account locked it (step-up, own-password change)
    if (u.lockedUntil !== null && u.lockedUntil > now) throw await fail('locked', u.id);
    // TD-10b (review 2.3a): the login lockout of this user FROM THIS CLIENT ADDRESS; locked/throttled attempts are
    // refused without counting (the right password too — one answer, no oracle)
    const st = await this.lockout.state(u, ip);
    if (st !== 'open') throw await fail(st, u.id);
    if (!ok) {
      const r = await this.lockout.fail(u, ip);
      if (r === 'throttled') this.lastAdminThrottled(u, ip);
      throw await fail(
        r === 'locked'
          ? 'bad-password-locked'
          : r === 'throttled'
            ? 'bad-password-throttled'
            : 'bad-password',
        u.id,
      );
    }
    if (u.disabled) throw await fail('disabled', u.id);
    // P06 review H1 for the per-address lock: refused if a parallel wrong guess locked/throttled it since `state()`
    if (!(await this.lockout.admit(u, ip))) throw await fail('locked', u.id);
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
    return { id: u.id, username: u.username, role: u.role, credentialGen: u.credentialGen };
  }

  /**
   * The first factor passed. F-aaa-login: when the MFA policy covers the user's role, or the user enrolled a factor,
   * no session is issued — a single-use challenge (5 min, bound to the credential generation) is returned instead;
   * `POST /auth/mfa/verify` (or enrolment first, when the policy requires a factor the user has not set up) turns it
   * into the session.
   */
  private async finish(
    u: LocalOk,
    ip: string,
    policy: LoginPolicy,
    via: string,
  ): Promise<LoginResult | MfaChallenge> {
    const enrolled = await this.mfa.enrolled(u.id);
    if (enrolled || mfaRequiredFor(policy.mfaRequired, u.role)) {
      const challenge = b64url(32);
      const c: StoredChallenge = { uid: u.id, gen: u.credentialGen, enrolled, via };
      await this.kv.set(
        `mfach:${sha256hex(challenge)}`,
        JSON.stringify(c),
        'EX',
        MFA_CHALLENGE_SEC,
      );
      await this.audit.write({
        userId: u.id,
        username: u.username,
        sourceIp: ip,
        action: 'auth.login',
        resource: u.username,
        after: { step: 'password-ok-mfa-pending', via },
        result: 'success',
        status: 200,
      });
      return { mfaRequired: true, challenge, enrolled, expiresIn: MFA_CHALLENGE_SEC };
    }
    return this.issueLogin(u, ip, via, false);
  }

  private async issueLogin(
    u: LocalOk,
    ip: string,
    via: string,
    mfa: boolean,
  ): Promise<LoginResult> {
    const s = await this.session({ id: u.id, username: u.username, role: u.role }, u.credentialGen);
    if (s === null || s === 'expired') {
      throw await this.failer(u.username, ip)('credentials-changed-during-login', u.id);
    }
    if (mfa) await this.markMfa(s.refreshToken.split('.')[0]!);
    await this.audit.write({
      userId: u.id,
      username: u.username,
      sourceIp: ip,
      action: 'auth.login',
      resource: u.username,
      after: { via, mfa },
      result: 'success',
      status: 200,
    });
    return s;
  }

  /** The login session `sid` passed the second factor (kept for the session's maximum life). */
  private async markMfa(sid: string): Promise<void> {
    this.mfaSids.add(sid);
    await this.kv.set(`mfasid:${sid}`, '1', 'EX', this.tokens.sessionMax);
  }

  private async sidHasMfa(sid: string | undefined): Promise<boolean> {
    if (sid === undefined) return false;
    if (this.mfaSids.has(sid)) return true;
    const ok = (await this.kv.get(`mfasid:${sid}`)) === '1';
    if (ok) this.mfaSids.add(sid);
    return ok;
  }

  /**
   * F-aaa-login — no stale-session bypass: a JWT session of a role the MFA policy covers must have passed the second
   * factor. Checked on every authenticated request (policy cached ≤ 5 s) and on refresh, so a session opened before
   * `mfa.required` was raised stops working at once. (A user's own enrolment ends their other sessions instead.)
   */
  private async mfaMissing(role: Role, sid: string | undefined): Promise<boolean> {
    let policy: LoginPolicy | undefined;
    try {
      policy = await this.aaa.cachedPolicy();
    } catch {
      policy = this.aaa.lastPolicy();
    }
    if (policy === undefined || !mfaRequiredFor(policy.mfaRequired, role)) return false;
    return !(await this.sidHasMfa(sid));
  }

  /** Resolve a login challenge (not consumed); null when unknown/expired or the account changed since. */
  private async challengeUser(
    challenge: string,
  ): Promise<{ c: StoredChallenge; u: typeof appUser.$inferSelect } | null> {
    if (!/^[A-Za-z0-9_-]{43}$/.test(challenge)) return null;
    const raw = await this.kv.get(`mfach:${sha256hex(challenge)}`);
    if (raw === null) return null;
    const c = JSON.parse(raw) as StoredChallenge;
    const [u] = await this.db.select().from(appUser).where(eq(appUser.id, c.uid));
    if (u === undefined || u.disabled || u.credentialGen !== c.gen) return null;
    if (u.lockedUntil !== null && u.lockedUntil > new Date()) return null;
    return { c, u };
  }

  /**
   * `POST /auth/mfa/enroll` with a login challenge: the MFA policy requires a factor this user has not set up — the
   * new TOTP secret is returned ONCE; `mfa/verify` with a code from it enables it and issues the session.
   */
  async mfaEnrollWithChallenge(challenge: string, ip: string) {
    const r = await this.challengeUser(challenge);
    if (r === null || r.c.enrolled) throw problems.unauthorized('invalid or expired challenge');
    const policy = await this.aaa.policy();
    const e = await this.mfa.begin(r.u.id, r.u.username, policy.mfaIssuer);
    if (e === null) throw problems.unauthorized('invalid or expired challenge');
    await this.audit.write({
      userId: r.u.id,
      username: r.u.username,
      sourceIp: ip,
      action: 'auth.mfa.enroll',
      resource: `user/${r.u.username}`,
      after: { step: 'secret-issued', via: 'login-challenge' },
      result: 'success',
      status: 200,
    });
    return e;
  }

  /**
   * `POST /auth/mfa/verify`: the second login step. A TOTP code (each time step once) or a recovery code (once).
   * At most MFA_CHALLENGE_TRIES wrong answers per challenge (then it is gone); every wrong answer counts toward the
   * login lockout of the account from this client address, like a wrong password. On success the challenge is
   * consumed (atomically: GETDEL, so two parallel verifies cannot both win) and the session is issued, flagged as
   * MFA-verified. A pending enrolment is enabled here and its recovery codes are returned once.
   */
  async mfaVerify(
    challenge: string,
    answer: { code?: string; recoveryCode?: string },
    ip: string,
  ): Promise<LoginResult & { recoveryCodes?: string[] }> {
    const fail = async (reason: string, uid: number | null, name: string | null) => {
      await this.audit.write({
        userId: uid,
        username: name,
        sourceIp: ip,
        action: 'auth.mfa.verify',
        resource: name,
        after: { reason },
        result: 'failure',
        status: 401,
      });
      return problems.unauthorized('invalid code or challenge');
    };
    if ((await this.tokens.hit(`login:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw problems.tooMany('too many login attempts; try again in a minute');
    }
    const r = await this.challengeUser(challenge);
    if (r === null) throw await fail('invalid-challenge', null, null);
    const { c, u } = r;
    const key = `mfach:${sha256hex(challenge)}`;
    if ((await this.lockout.state(u, ip)) !== 'open') throw await fail('locked', u.id, u.username);
    let ok = false;
    let how = 'totp';
    if (answer.code !== undefined) ok = await this.mfa.verifyCode(u.id, answer.code, !c.enrolled);
    else if (answer.recoveryCode !== undefined && c.enrolled) {
      how = 'recovery-code';
      ok = await this.mfa.useRecovery(u.id, answer.recoveryCode);
    }
    if (!ok) {
      const n = await this.kv.incr(`${key}:n`);
      await this.kv.expire(`${key}:n`, MFA_CHALLENGE_SEC);
      if (n >= MFA_CHALLENGE_TRIES) await this.kv.del(key);
      const o = await this.lockout.fail(u, ip);
      if (o === 'throttled') this.lastAdminThrottled(u, ip);
      throw await fail(`bad-${how}`, u.id, u.username);
    }
    // single use: whoever deletes the challenge first gets the session
    if ((await this.kv.del(key)) !== 1)
      throw await fail('challenge-already-used', u.id, u.username);
    let recoveryCodes: string[] | undefined;
    if (!c.enrolled) {
      recoveryCodes = await this.mfa.activate(u.id);
      await this.audit.write({
        userId: u.id,
        username: u.username,
        sourceIp: ip,
        action: 'auth.mfa.enroll',
        resource: `user/${u.username}`,
        after: { step: 'enabled', via: 'login-challenge' },
        result: 'success',
        status: 200,
      });
    }
    const s = await this.issueLogin(
      { id: u.id, username: u.username, role: u.role, credentialGen: u.credentialGen },
      ip,
      `${c.via}+${how}`,
      true,
    );
    return recoveryCodes === undefined ? s : { ...s, recoveryCodes };
  }

  /** F-aaa-login: is OpenID Connect single sign-on offered (the login page shows the button)? */
  async oidcEnabled(): Promise<boolean> {
    return (await this.aaa.oidcConfig()) !== null;
  }

  /**
   * `GET /auth/oidc/start`: the IdP authorisation URL (code flow + PKCE S256 + nonce). The state is single use, 10 min,
   * kept server-side with the PKCE verifier and the nonce. Rate-limited with the password logins of this client.
   */
  async oidcStart(ip: string): Promise<string> {
    if ((await this.tokens.hit(`login:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw problems.tooMany('too many login attempts; try again in a minute');
    }
    const cfg = await this.aaa.oidcConfig();
    if (cfg === null) throw problems.notFound('OpenID Connect login is not configured');
    let d;
    try {
      d = await discover(cfg.issuer);
    } catch (e) {
      throw problems.unavailable(
        `the identity provider cannot be reached (${(e as Error).message})`,
      );
    }
    const { state, nonce } = newState();
    const p = pkce();
    await this.kv.set(
      `oidcst:${sha256hex(state)}`,
      JSON.stringify({ verifier: p.verifier, nonce }),
      'EX',
      OIDC_STATE_SEC,
    );
    return authorizationUrl(d, cfg, { state, nonce, challenge: p.challenge });
  }

  /**
   * `GET /auth/oidc/callback`: the state is consumed (GETDEL — single use), the code redeemed with the PKCE verifier,
   * the ID token verified; its user → roleMap → shadow user → `finish()` (so the MFA policy applies to SSO logins
   * too). Every outcome is audited as `auth.external` / `auth.login` with method `oidc`; the caller only gets a slug.
   */
  async oidcCallback(
    code: string | undefined,
    state: string | undefined,
    ip: string,
  ): Promise<LoginResult | MfaChallenge> {
    if ((await this.tokens.hit(`login:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw problems.tooMany('too many login attempts; try again in a minute');
    }
    const fail = this.failer('(oidc)', ip);
    if (!code || !state || !/^[A-Za-z0-9_-]{16,128}$/.test(state) || code.length > 2048) {
      throw await fail('oidc-bad-callback', null);
    }
    const raw = await this.kv.getdel(`oidcst:${sha256hex(state)}`);
    if (raw === null) throw await fail('oidc-unknown-state', null);
    const st = JSON.parse(raw) as { verifier: string; nonce: string };
    const cfg = await this.aaa.oidcConfig();
    if (cfg === null) throw await fail('oidc-not-configured', null);
    const secretVal = await this.aaa.oidcClientSecret(cfg.clientSecretRef);
    if (secretVal === null) {
      await this.auditExternal(
        '(oidc)',
        ip,
        'oidc',
        cfg.issuer,
        'unreachable',
        `secret ${cfg.clientSecretRef} not found`,
      );
      throw await fail('oidc-secret-missing', null);
    }
    let who;
    try {
      who = await redeem(await discover(cfg.issuer), cfg, secretVal, code, st.verifier, st.nonce);
    } catch (e) {
      await this.auditExternal('(oidc)', ip, 'oidc', cfg.issuer, 'reject', (e as Error).message);
      throw await fail('oidc-token-refused', null);
    }
    await this.auditExternal(who.username, ip, 'oidc', cfg.issuer, 'accept', null);
    const u = await this.provision(
      who.username,
      'oidc',
      who.groups,
      ip,
      this.failer(who.username, ip),
    );
    return this.finish(u, ip, await this.aaa.policy(), 'oidc');
  }

  /** `GET /auth/mfa`: the caller's second-factor state and the policy that applies to them. */
  async mfaStatus(user: Principal) {
    const st = await this.mfa.status(user.id);
    const policy = await this.aaa.cachedPolicy();
    return { ...st, required: mfaRequiredFor(policy.mfaRequired, user.role) };
  }

  /**
   * `POST /auth/mfa/setup` (voluntary enrolment from a login session): step-up with the current password first —
   * a stolen access token must not bind the attacker's authenticator to the account (the D-100 rule for API keys).
   * External identities have no local password: refused (they enrol during login when the policy requires it).
   */
  async mfaSetup(user: Principal, current: string, secure: boolean) {
    if (user.via !== 'jwt') {
      throw problems.forbidden(
        'a second factor is set up from a login session, not with an API key',
      );
    }
    if (!secure) throw tlsRequired();
    if ((await this.tokens.hit(`pwset:${user.id}`, 60)) > this.env.VRX_PASSWORD_RATE_PER_MIN) {
      throw problems.tooMany('too many password checks; try again in a minute');
    }
    await this.checkCurrent(user, current);
    const policy = await this.aaa.policy();
    const e = await this.mfa.begin(user.id, user.username, policy.mfaIssuer);
    if (e === null) {
      throw problems.conflict(
        'mfa-already-enrolled',
        'a second factor is already enabled; an admin reset removes it first',
      );
    }
    return e;
  }

  /**
   * `POST /auth/mfa/activate`: the first code from the new authenticator enables the factor. The user's OTHER
   * sessions end (they never passed it — no stale-session bypass); this one continues, flagged as MFA-verified.
   * Recovery codes are returned once.
   */
  async mfaActivate(user: Principal, code: string) {
    if (user.via !== 'jwt' || user.sid === undefined) {
      throw problems.forbidden(
        'a second factor is set up from a login session, not with an API key',
      );
    }
    if ((await this.tokens.hit(`pwset:${user.id}`, 60)) > this.env.VRX_PASSWORD_RATE_PER_MIN) {
      throw problems.tooMany('too many attempts; try again in a minute');
    }
    if (!(await this.mfa.verifyCode(user.id, code, true))) {
      throw problems.badRequest('the code does not match the authenticator being set up', [
        { pointer: '/code', message: 'invalid code' },
      ]);
    }
    const recoveryCodes = await this.mfa.activate(user.id);
    const [g] = await this.db
      .update(appUser)
      .set({ credentialGen: sql`${appUser.credentialGen} + 1` })
      .where(eq(appUser.id, user.id))
      .returning({ gen: appUser.credentialGen });
    if (g !== undefined) await this.tokens.revokeUser(user.id, g.gen, user.sid);
    await this.markMfa(user.sid);
    return { recoveryCodes };
  }

  /**
   * `DELETE /auth/mfa/users/{name}` (admin): a lost authenticator. The factor and its recovery codes are deleted and
   * the user's sessions end; with a policy that requires MFA the user enrols again at the next login.
   */
  async mfaReset(name: string): Promise<void> {
    const [u] = await this.db
      .select({ id: appUser.id })
      .from(appUser)
      .where(eq(appUser.username, name));
    if (u === undefined) throw problems.notFound(`user '${name}' does not exist`);
    if (!(await this.mfa.reset(u.id)))
      throw problems.notFound(`user '${name}' has no second factor`);
    const [g] = await this.db
      .update(appUser)
      .set({ credentialGen: sql`${appUser.credentialGen} + 1` })
      .where(eq(appUser.id, u.id))
      .returning({ gen: appUser.credentialGen });
    if (g !== undefined) await this.tokens.revokeUser(u.id, g.gen);
  }

  /** The last admin was throttled instead of locked: one system_event per admin and minute (visible, not a flood). */
  private lastAdminThrottled(u: LockSubject & { username: string }, ip: string): void {
    const now = Date.now();
    if (now - (this.throttleEventAt.get(u.id) ?? 0) < 60_000) return;
    this.throttleEventAt.set(u.id, now);
    void this.events.record(
      'warning',
      'auth',
      'LOGIN_THROTTLED',
      `failed logins for '${u.username}' from ${clientKey(ip)}: the last admin who can log in from there is throttled, not locked`,
      { user: u.username, client: clientKey(ip) },
    );
  }

  /**
   * One failed password check for `userId` made INSIDE a session of that account (a wrong `current` on the API-key
   * step-up or an own-password change — D-097, review M2; TD-4). ONE atomic statement (P06 review H1): concurrent
   * failures each add 1 — no read-modify-write race. PostgreSQL evaluates every SET expression against the old row,
   * so the CASEs see the same pre-increment value. Returns whether the account is locked now.
   * TD-10b (review 2.3a): the LAST ADMIN is never locked here — an enabled admin with no other enabled admin who is
   * not locked. Its checks stay bounded by the per-account budget (`pwset:<id>`, VRX_PASSWORD_RATE_PER_MIN) that runs
   * before argon2 on both routes; a lock would shut the owner out of every login (this lock is account-wide).
   */
  async registerFailure(userId: number): Promise<boolean> {
    const max = this.env.VRX_LOGIN_MAX_FAILURES;
    const hit = sql`${appUser.failedLogins} + 1 >= ${max}`;
    const last = sql`(${appUser.role} = 'admin' and not ${appUser.disabled} and not exists (select 1 from ${appUser} o where o.role = 'admin' and not o.disabled and o.id <> ${appUser.id} and (o.locked_until is null or o.locked_until <= now())))`;
    // review L1: lock decisions are serialised (one transaction-scoped advisory lock), so two concurrent failures of
    // two admins cannot each see the other still unlocked and lock both; the UPDATE's snapshot is taken after the lock
    const row = await this.db.transaction(async (tx) => {
      await tx.execute(sql`select pg_advisory_xact_lock(${LOCK_DECISION_KEY})`);
      const [r] = await tx
        .update(appUser)
        .set({
          failedLogins: sql`case when ${hit} then 0 else ${appUser.failedLogins} + 1 end`,
          lockedUntil: sql`case when ${hit} and not ${last} then now() + make_interval(secs => ${this.env.VRX_LOGIN_LOCKOUT_SEC}) else ${appUser.lockedUntil} end`,
        })
        .where(eq(appUser.id, userId))
        .returning({ lockedUntil: appUser.lockedUntil });
      return r;
    });
    return row?.lockedUntil != null && row.lockedUntil > new Date();
  }

  /**
   * Refresh chain + access token under credential generation `gen` (from app_user); null when the chain was revoked
   * meanwhile (D-097), `expired` when the login session reached VRX_SESSION_MAX_SEC (TD-10b). `family` continues a
   * chain (refresh); without it a new one starts (login). The access token never outlives the session.
   */
  private async session(
    user: { id: number; username: string; role: Role },
    gen: number,
    family?: string,
  ): Promise<LoginResult | 'expired' | null> {
    const refresh = await this.tokens.issueRefresh(user.id, gen, family);
    if (refresh === null || refresh === 'expired') return refresh;
    const left = refresh.notAfter - Math.floor(Date.now() / 1000);
    return {
      accessToken: await this.tokens.signAccess(
        { ...user, sid: refresh.family, gen: refresh.gen },
        refresh.notAfter,
      ),
      tokenType: 'Bearer',
      expiresIn: Math.max(1, Math.min(this.tokens.accessTtl, left)),
      refreshToken: refresh.token,
      refreshMaxAge: refresh.maxAge,
      user,
    };
  }

  /** Audit identity of a user id that may no longer exist (audit_log.user_id references app_user). */
  private async who(
    uid: number | undefined,
  ): Promise<{ userId: number | null; username: string | null }> {
    if (uid === undefined) return { userId: null, username: null };
    const [u] = await this.db
      .select({ id: appUser.id, username: appUser.username })
      .from(appUser)
      .where(eq(appUser.id, uid));
    return { userId: u?.id ?? null, username: u?.username ?? null };
  }

  /**
   * TD-10b (review 2.3e): every refused refresh is audited with its reason — per user where the token names one; an
   * unknown token (junk, long expired) as an aggregated row per client and minute, so the table cannot be flooded.
   * A request without any cookie (a page load before login) is not a failure and writes nothing. Successful refreshes
   * are not audited (one per session every ~15 min; the login row starts the session).
   */
  async refresh(token: string | undefined, ip: string): Promise<LoginResult> {
    if (!token) throw problems.unauthorized('no refresh token');
    const refuse = async (reason: string, uid: number | undefined) => {
      const w = await this.who(uid);
      await this.audit.write({
        ...w,
        sourceIp: ip,
        action: 'auth.refresh',
        resource: w.username === null ? null : `user/${w.username}`,
        after: { reason, ...(w.userId === null && uid !== undefined ? { uid } : {}) },
        result: 'failure',
        status: 401,
      });
      return problems.unauthorized('invalid refresh token');
    };
    const r = await this.tokens.consumeRefresh(token);
    if (r === null) {
      void this.audit.writeAggregated(
        {
          userId: null,
          username: null,
          sourceIp: ip,
          action: 'auth.refresh',
          resource: null,
          after: { reason: 'unknown-token' },
          result: 'failure',
          status: 401,
        },
        `auth.refresh|unknown-token|${clientKey(ip)}`,
      );
      throw problems.unauthorized('invalid refresh token');
    }
    if ('reuse' in r) {
      // theft detection: the family is gone — and (TD-10b) so are the access tokens of that session, both copies
      await this.tokens.revokeSession(r.family);
      this.bus.sessions({ sid: r.family });
      throw await refuse('refresh-token-reuse: family revoked', r.uid);
    }
    if ('ended' in r) throw await refuse('session-ended', r.uid);
    const [u] = await this.db.select().from(appUser).where(eq(appUser.id, r.uid));
    if (u === undefined) throw await refuse('user-deleted', r.uid);
    if (u.disabled) throw await refuse('disabled', u.id);
    if (u.lockedUntil !== null && u.lockedUntil > new Date()) throw await refuse('locked', u.id);
    // F-aaa-login: a session that never passed the second factor does not outlive a policy that now demands it
    if (await this.mfaMissing(u.role, r.family)) {
      await this.tokens.endSid(r.family, u.id);
      throw await refuse('mfa-required', u.id);
    }
    // verify V1/V3: the chain continues only under the generation PostgreSQL holds NOW (a reset commits it first)
    const s = await this.session(
      { id: u.id, username: u.username, role: u.role },
      u.credentialGen,
      r.family,
    );
    if (s === 'expired') throw await refuse('session-expired', u.id);
    if (s === null) throw await refuse('credentials-changed', u.id);
    return s;
  }

  /**
   * TD-10b (review 2.3c/2.3e): logout ends the login session — its refresh chain AND its access tokens (per sid, not
   * the user's other sessions) — and is audited. The session comes from the refresh cookie (the web UI) and/or a
   * Bearer token (the CLI); a cookie the store does not know ends nothing (a forged `<family>.<x>` cannot log anyone
   * out) and is only counted, aggregated. Always 204: logout never tells whether a session existed.
   */
  async logout(token: string | undefined, authorization: string | undefined, ip: string) {
    const ended: { sid: string; uid: number | undefined; via: 'refresh-cookie' | 'bearer' }[] = [];
    if (token) {
      const e = await this.tokens.endSession(token);
      if (e !== null) ended.push({ ...e, via: 'refresh-cookie' });
      else
        void this.audit.writeAggregated(
          {
            userId: null,
            username: null,
            sourceIp: ip,
            action: 'auth.logout',
            resource: null,
            after: { reason: 'unknown-token' },
            result: 'failure',
            status: 204,
          },
          `auth.logout|unknown-token|${clientKey(ip)}`,
        );
    }
    const [scheme, value] = authorization?.split(/\s+/, 2) ?? [];
    if (scheme?.toLowerCase() === 'bearer' && value) {
      const c = await this.tokens.verifyAccess(value);
      if (c?.sid !== undefined && !ended.some((e) => e.sid === c.sid)) {
        await this.tokens.endSid(c.sid, c.id);
        ended.push({ sid: c.sid, uid: c.id, via: 'bearer' });
      }
    }
    for (const e of ended) {
      this.bus.sessions({ sid: e.sid });
      const w = await this.who(e.uid);
      await this.audit.write({
        ...w,
        sourceIp: ip,
        action: 'auth.logout',
        resource: w.username === null ? null : `user/${w.username}`,
        after: { via: e.via },
        result: 'success',
        status: 204,
      });
    }
  }

  /** `Authorization: Bearer <jwt>` or `Authorization: ApiKey <key>` → principal, or null. */
  async authenticate(header: string | undefined): Promise<Principal | null> {
    if (!header) return null;
    const [scheme, value] = header.split(/\s+/, 2);
    if (!value) return null;
    if (scheme?.toLowerCase() === 'bearer') {
      const c = await this.tokens.verifyAccess(value);
      if (c === null) return null;
      // F-aaa-login: no stale-session bypass of the MFA policy (see mfaMissing)
      if (await this.mfaMissing(c.role, c.sid)) return null;
      return { ...c, via: 'jwt' };
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
   *
   * D-100 (2), TD-4 — step-up: a JWT caller must send its current password (`current`), so a stolen short-lived access
   * token cannot mint a long-lived key. An API-key caller must NOT send one (D-124): 400, never checked — a role-capped
   * key must not become an oracle for its owner's (uncapped) password; automation needs no step-up. `current` is
   * accepted over TLS/loopback only; each check spends the account's per-minute budget of password checks first
   * (review H1: Valkey INCR, so it bounds parallel guesses too, which all read `locked_until` before any failure
   * lands); a wrong one counts toward the lockout like a failed login. argon2 runs before the transaction (never under
   * the row lock); inside it, the generation read together with the checked hash must still be the committed one (as
   * in `login()`), and the account must not have been locked meanwhile.
   */
  async createApiKey(
    user: Principal,
    name: string,
    role: Role | undefined,
    expiresInDays: number | undefined,
    stepUp: KeyStepUp,
  ) {
    // a password in the body (always for a JWT caller): the transport rule of login and password set, first
    if ((user.via === 'jwt' || stepUp.current !== undefined) && !stepUp.secure) throw tlsRequired();
    if (user.via === 'apikey' && stepUp.current !== undefined) {
      throw new ProblemError(
        400,
        'current-not-allowed-with-api-key',
        'Current password not allowed',
        'an API-key caller does not send the current password; it is never checked for a key',
        [{ pointer: '/current', message: 'not allowed with Authorization: ApiKey' }],
      );
    }
    if (user.via === 'jwt' && stepUp.current === undefined) {
      throw problems.badRequest(
        'creating an API key from a login session needs the current password',
        [{ pointer: '/current', message: 'required when the caller is a login (JWT) session' }],
      );
    }
    let checkedGen: number | undefined;
    if (stepUp.current !== undefined) {
      // review H1: the per-account budget shared with password changes (`pwset:<user id>`, as in setPassword), spent
      // BEFORE argon2 — a stolen session cannot out-run the lockout with parallel guesses or queue unbounded argon2
      if ((await this.tokens.hit(`pwset:${user.id}`, 60)) > this.env.VRX_PASSWORD_RATE_PER_MIN) {
        throw problems.tooMany('too many password checks; try again in a minute');
      }
      checkedGen = await this.checkCurrent(user, stepUp.current);
    }
    const token = newApiKeyToken();
    const scope = role === undefined ? user.role : lowerRole(user.role, role);
    // SEC-auth M1: a key minted by an expiring API key never outlives it — without this a leaked 7-day key could mint a
    // key that never expires (and survives the deletion of the leaked one). A key without expiry mints as before.
    const requested =
      expiresInDays === undefined ? null : new Date(Date.now() + expiresInDays * 86400_000);
    const callerExp =
      user.via === 'apikey' && user.exp !== undefined ? new Date(user.exp * 1000) : null;
    const expiresAt =
      callerExp !== null && (requested === null || requested > callerExp) ? callerExp : requested;
    const row = await this.db.transaction(async (tx) => {
      const [u] = await tx
        .select({
          gen: appUser.credentialGen,
          disabled: appUser.disabled,
          lockedUntil: appUser.lockedUntil,
        })
        .from(appUser)
        .where(eq(appUser.id, user.id))
        .for('share');
      if (u === undefined) throw problems.unauthorized('user no longer exists');
      // D-100 (3): a disabled account mints nothing (an API-key request authenticated just before the disable)
      if (u.disabled) throw problems.unauthorized('the account is disabled');
      if (checkedGen !== undefined) {
        // the password was checked against the hash of generation `checkedGen`: a reset/disable since → refused
        if (u.gen !== checkedGen)
          throw problems.unauthorized('the password was changed; log in again');
        // a parallel wrong guess may have locked the account since the check (as in login)
        if (u.lockedUntil !== null && u.lockedUntil > new Date()) throw accountLocked();
      }
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
          expiresAt,
        })
        .returning();
      return r!;
    });
    return { id: row.id, name, role: scope, expiresAt: row.expiresAt, key: token };
  }

  /**
   * D-100 (2): check the caller's current password for a key creation. Hash and generation come from ONE row read
   * (as in login); argon2 runs here, outside any transaction. Returns that generation, which the key transaction
   * re-checks under `FOR SHARE`. A locked account takes no guess (403 `locked`, no argon2); a stale JWT session is
   * refused before argon2 (it could not mint anyway, and must not feed the lockout); a wrong password counts like
   * a failed login (`registerFailure`).
   */
  private async checkCurrent(user: Principal, current: string): Promise<number> {
    const [u] = await this.db
      .select({
        hash: appUser.passwordHash,
        gen: appUser.credentialGen,
        lockedUntil: appUser.lockedUntil,
      })
      .from(appUser)
      .where(eq(appUser.id, user.id));
    if (u === undefined) throw problems.unauthorized('user no longer exists');
    if (user.via === 'jwt' && !this.tokens.sessionCurrent(user, u.gen)) {
      throw problems.unauthorized('the password was changed; log in again');
    }
    if (u.lockedUntil !== null && u.lockedUntil > new Date()) throw accountLocked();
    // F-aaa-login (D-100 for external principals, option "refuse"): an account without a local password (an external
    // identity's shadow user) has nothing to step up with — refused, never counted toward the lockout
    if (u.hash === null) throw externalNoStepUp();
    if (!(await verifyPassword(u.hash, current))) {
      // review H1: the lock this failure caused answers exactly like any other `locked`
      if (await this.registerFailure(user.id)) throw accountLocked();
      throw problems.forbidden('the current password is wrong');
    }
    return u.gen;
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
