import { Inject, Injectable, Logger } from '@nestjs/common';
import type { AuthMethod } from '@ngfw/schema';
import { and, count, eq, isNull, lte, or, sql } from 'drizzle-orm';
import { AuditService } from '../audit/audit.service.js';
import { SystemEventsService } from '../audit/system-events.service.js';
import { Bus } from '../infra/bus.js';
import { VALKEY, type Valkey } from '../infra/valkey.js';
import { ENV, type Env } from '../config.js';
import { problems, ProblemError } from '../common/problem.js';
import { clientKey, lowerRole, type Principal } from '../common/principal.js';
import { DB, type Db } from '../db/db.js';
import { apiKey, appUser, ROLES, type Role } from '../db/schema.js';
import { releaseKeyLocks } from '../datastore/pg-repo.js';
import { AaaService } from '../features/aaa/aaa.service.js';
import { mfaRequiredFor, MfaService, type MfaStatus } from '../features/aaa/mfa.service.js';
import { Lockout, type LockSubject } from './lockout.js';
import { authSequence, stepEligible } from './login-order.js';
import { MAX_ATTEMPTS, MfaTickets, TICKET_TTL_SEC } from './mfa-ticket.js';
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
 * F-aaa-login: `app_user.source` of an account this box created for an externally authenticated identity
 * (`aaa:radius`, `aaa:ldap`, …). The config user sync only deletes `source = 'config'` rows, so these survive a
 * commit; they carry no password hash, so they can never log in through the `local` step.
 */
const SHADOW_SOURCE = 'aaa:';

/** Whether `source` marks a shadow account of an external identity rather than a local one. */
export function isShadowSource(source: string): boolean {
  return source.startsWith(SHADOW_SOURCE);
}

/** Role order, for deciding whether a role change is a demotion (same ranking as the config user sync). */
const ROLE_RANK: Record<Role, number> = { readonly: 1, operator: 2, admin: 3 };

/** What one step of the authentication order answered: a session, or a reason the walk continues. */
type StepOutcome = { done: LoginOutcome } | { pass: 'absent' | 'unreachable' | 'unsupported' };

/** Audits one login failure and returns the problem to throw. `method` names the AAA method when there was one. */
type FailFn = (
  reason: string,
  userId: number | null,
  status?: number,
  method?: string,
) => Promise<ProblemError>;

/** The step-up of `POST /auth/api-keys` (D-100 (2)): the caller's current password and how the request arrived. */
export interface KeyStepUp {
  current: string | undefined;
  /** `secureTransport(req)`: TLS or a loopback peer. */
  secure: boolean;
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
 * F-aaa-login: the first factor was accepted but the login is not finished — `management.aaa.mfa.required` covers this
 * user. `mfa: 'code'` means a TOTP or recovery code is owed; `mfa: 'enrol'` that MFA is required for the role and the
 * account has no factor yet, so the ticket buys enrolment and nothing else. The ticket is not a session.
 */
export interface MfaChallenge {
  mfa: 'code' | 'enrol';
  ticket: string;
  /** Seconds the ticket lives. */
  expiresIn: number;
  /** Wrong codes still allowed on this login before the password has to be presented again. */
  attemptsLeft: number;
}

/** What a login answers: a session, or a second-factor challenge. */
export type LoginOutcome = LoginResult | MfaChallenge;

/** Narrow a login outcome to the challenge case. */
export function isMfaChallenge(o: LoginOutcome): o is MfaChallenge {
  return 'mfa' in o;
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
  private readonly tickets: MfaTickets;
  /** last system_event per throttled last admin (ms), so an attack writes one event a minute, not one per guess */
  private readonly throttleEventAt = new Map<number, number>();

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
    this.lockout = new Lockout(kv, db, env);
    this.tickets = new MfaTickets(kv);
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
   * F-aaa-login: log in by walking `management.aaa.order` (default `['local']` — i.e. exactly the local login).
   *
   * A method may accept, reject, or not answer: `absent` (the identity is unknown to it) or `unreachable` (no server
   * answered). The first method that ACCEPTS or REJECTS ends the walk — a directory must not be able to overrule a
   * method that already refused the credential, and a refused credential is not replayed against every other backend
   * in turn. `management.aaa.fallbackLocal` adds a closing `local` step that runs only when a method was unreachable.
   *
   * `secure`: the request came over TLS or from a loopback peer (`secureTransport`, D-100 (1)).
   *
   * Known and accepted: with an external method in the order, a name that is not a local user costs one extra backend
   * round trip, so response time still tells a probe whether a name is a LOCAL user. That is inherent to chaining (an
   * unknown local name has to be offered to the directory) and is bounded by the per-client rate limit below;
   * password validity itself stays unobservable.
   */
  async login(
    username: string,
    password: string,
    ip: string,
    secure: boolean,
  ): Promise<LoginOutcome> {
    const fail: FailFn = async (reason, userId, status = 401, method) => {
      await this.audit.write({
        userId,
        username,
        sourceIp: ip,
        action: 'auth.login',
        resource: username,
        after: method === undefined ? { reason } : { reason, method },
        result: 'failure',
        status,
      });
      return status === 429
        ? problems.tooMany('too many login attempts; try again in a minute')
        : status === 403
          ? tlsRequired()
          : problems.unauthorized('invalid credentials');
    };
    // D-100 (1), TD-4: the transport check comes FIRST — before the rate limiter, the user lookup and argon2 — so a
    // password sent in clear by a remote peer is never acted on: no failed login counted, the lockout untouched
    if (!secure) throw await fail('tls-required', null, 403);
    // TD-10b (review 2.3b): `ip` is the client behind the trusted proxy, so this bucket is per client, not global
    if ((await this.tokens.hit(`login:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw await fail('rate-limited', null, 429);
    }
    const policy = await this.aaa.policy();
    const seq = authSequence(policy.order, policy.fallbackLocal);
    let anyUnreachable = false;
    let localAbsent = false;
    for (let i = 0; i < seq.steps.length; i++) {
      const method = seq.steps[i] as AuthMethod;
      if (!stepEligible(seq, i, anyUnreachable)) continue;
      const step =
        method === 'local'
          ? await this.localStep(username, password, ip, fail)
          : await this.externalStep(method, username, password, ip, fail);
      if ('done' in step) return step.done;
      if (step.pass === 'unreachable') anyUnreachable = true;
      else if (step.pass === 'absent' && method === 'local') localAbsent = true;
    }
    // Nothing decided. `unknown-user` keeps the reason the local-only default has always written; a chain whose
    // backends were all down says so instead, because the two need different operator action.
    throw await fail(
      anyUnreachable
        ? 'all-methods-unreachable'
        : localAbsent
          ? 'unknown-user'
          : 'no-method-answered',
      null,
    );
  }

  /**
   * The `local` step: the local login as it has always been (D-097, TD-4, TD-10b — the order of argon2, the
   * unknown-user check and the two lockouts is deliberate and unchanged here).
   *
   * It answers `absent` instead of failing the login when this box holds no local credential for the name, so the
   * walk can offer it to the next method: no such user, or a user with no argon2 hash (an external shadow account, or
   * a `management.users` entry committed without a `passwordHash`). Either way argon2 has already run against the
   * timing equaliser, so `absent` costs what a real check costs.
   */
  private async localStep(
    username: string,
    password: string,
    ip: string,
    fail: FailFn,
  ): Promise<StepOutcome> {
    // D-097 (review H2, verify V1): hash and credential generation come from ONE row read, so the session below is
    // issued under the generation that goes with the password that was checked
    const [u] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    const ok = await verifyPassword(u?.passwordHash, password);
    if (u === undefined || u.passwordHash === null) return { pass: 'absent' };
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
    return {
      done: await this.finishFirstFactor(
        { id: u.id, username: u.username, role: u.role, gen: u.credentialGen },
        ip,
        'local',
        fail,
      ),
    };
  }

  /**
   * An external step (`radius` today; `ldap`/`tacacs` answer `unsupported` until their backends land, so the walk
   * treats them as "did not answer" rather than failing the login).
   *
   * On accept the identity gets a local SHADOW account — `source: 'aaa:<method>'`, no password hash — whose role comes
   * from `management.aaa.roleMap`; sessions, revocation, API keys and the audit log then work exactly as for a local
   * user. `syncUsers` (pg-repo) only ever deletes `source = 'config'` rows, so a commit leaves shadow accounts alone,
   * and a shadow account never appears in `management.users`. Without a roleMap match the login is refused: an
   * external identity gets no implicit role (least privilege).
   */
  private async externalStep(
    method: AuthMethod,
    username: string,
    password: string,
    ip: string,
    fail: FailFn,
  ): Promise<StepOutcome> {
    const ext = await this.aaa.authenticate(method, username, password);
    if (ext.answer === 'unsupported') return { pass: 'unsupported' };
    if (ext.answer === 'unreachable') return { pass: 'unreachable' };
    // An account that already exists carries the lockout state of this (user, client address): an external reject
    // counts there too, so guessing an external identity is throttled by this box as well as by the directory.
    const [existing] = await this.db.select().from(appUser).where(eq(appUser.username, username));
    if (existing !== undefined) {
      const now = new Date();
      if (existing.lockedUntil !== null && existing.lockedUntil > now)
        throw await fail('locked', existing.id, 401, method);
      const st = await this.lockout.state(existing, ip);
      if (st !== 'open') throw await fail(st, existing.id, 401, method);
    }
    if (ext.answer === 'reject') {
      if (existing !== undefined) {
        const r = await this.lockout.fail(existing, ip);
        if (r === 'throttled') this.lastAdminThrottled(existing, ip);
      }
      throw await fail('rejected', existing?.id ?? null, 401, method);
    }
    if (ext.role === null) throw await fail('no-role-mapping', existing?.id ?? null, 401, method);
    // A directory must never be able to log in AS a local account: same name, different credential store. The
    // operator renames one of the two (audited reason `local-account`), rather than the directory silently winning.
    if (existing !== undefined && !isShadowSource(existing.source))
      throw await fail('local-account', existing.id, 401, method);
    if (existing?.disabled === true) throw await fail('disabled', existing.id, 401, method);
    if (existing !== undefined && !(await this.lockout.admit(existing, ip)))
      throw await fail('locked', existing.id, 401, method);
    const u = await this.upsertShadow(username, ext.role, method, existing);
    return {
      done: await this.finishFirstFactor(
        { id: u.id, username, role: u.role, gen: u.credentialGen },
        ip,
        method,
        fail,
        { role: u.role, groups: ext.groups },
      ),
    };
  }

  /**
   * Create or refresh the shadow account of an external identity. A DEMOTION bumps the credential generation, so the
   * sessions and API keys issued under the higher role end at once — the same rule the config user sync follows
   * (TD-10b, PENDING-session-revocation option 1); a promotion needs nothing, because a refresh reads the role from
   * `app_user`. The insert is `onConflictDoUpdate` so two parallel first logins cannot both create the row.
   */
  private async upsertShadow(
    username: string,
    role: Role,
    method: AuthMethod,
    existing: { id: number; role: Role; credentialGen: number } | undefined,
  ): Promise<{ id: number; role: Role; credentialGen: number }> {
    const source = `${SHADOW_SOURCE}${method}`;
    const lastLogin = new Date();
    if (existing === undefined) {
      const [row] = await this.db
        .insert(appUser)
        .values({ username, passwordHash: null, role, source, lastLogin })
        .onConflictDoUpdate({
          target: appUser.username,
          set: { role, source, lastLogin, failedLogins: 0, lockedUntil: null },
          // `existing` was undefined a moment ago, so a conflict here means a row appeared in between — possibly a
          // LOCAL user created by a concurrent commit. `setWhere` keeps the takeover rule above true under that race:
          // the update touches shadow rows only, and for a local one no row comes back and the login is refused.
          setWhere: sql`${appUser.source} like ${`${SHADOW_SOURCE}%`}`,
        })
        .returning({
          id: appUser.id,
          role: appUser.role,
          credentialGen: appUser.credentialGen,
        });
      if (row === undefined) throw problems.unauthorized('invalid credentials');
      return row;
    }
    const demoted = ROLE_RANK[role] < ROLE_RANK[existing.role];
    const [row] = await this.db
      .update(appUser)
      .set({
        role,
        source,
        lastLogin,
        failedLogins: 0,
        lockedUntil: null,
        ...(demoted ? { credentialGen: sql`${appUser.credentialGen} + 1` } : {}),
      })
      .where(eq(appUser.id, existing.id))
      .returning({ id: appUser.id, role: appUser.role, credentialGen: appUser.credentialGen });
    return row ?? { id: existing.id, role, credentialGen: existing.credentialGen };
  }

  /**
   * The first factor is proved. If `management.aaa.mfa.required` covers this user, answer a challenge instead of a
   * session: a ticket that buys one second-factor attempt (`code`), or, when the account has no factor yet,
   * enrolment and nothing else (`enrol`). Without the `enrol` case an unenrolled user could never get in, and letting
   * them in unprotected would make `mfa.required` advisory.
   *
   * The challenge is audited as `auth.mfa`, never as an `auth.login` failure: the password was right, and the
   * auto-block detector (F-bruteforce-block) counts `auth.login` failures — a user reaching for their phone is not a
   * brute-force attempt.
   */
  private async finishFirstFactor(
    user: { id: number; username: string; role: Role; gen: number },
    ip: string,
    method: string,
    fail: FailFn,
    extra: Record<string, unknown> = {},
  ): Promise<LoginOutcome> {
    const policy = await this.aaa.policy();
    if (mfaRequiredFor(policy.mfa.required, user.role)) {
      const purpose = (await this.mfa.isEnrolled(user.id)) ? 'code' : 'enrol';
      const ticket = await this.tickets.issue({
        userId: user.id,
        username: user.username,
        role: user.role,
        gen: user.gen,
        purpose,
        method,
        attempts: 0,
      });
      await this.audit.write({
        userId: user.id,
        username: user.username,
        sourceIp: ip,
        action: 'auth.mfa',
        resource: user.username,
        after: { stage: purpose === 'code' ? 'code-required' : 'enrolment-required', method },
        result: 'success',
        status: 200,
      });
      return { mfa: purpose, ticket, expiresIn: TICKET_TTL_SEC, attemptsLeft: MAX_ATTEMPTS };
    }
    return this.issueSession(user, ip, method, fail, extra);
  }

  /** Issue the session of a completed login and audit it. */
  private async issueSession(
    user: { id: number; username: string; role: Role; gen: number },
    ip: string,
    method: string,
    fail: FailFn,
    extra: Record<string, unknown> = {},
  ): Promise<LoginResult> {
    const s = await this.session(
      { id: user.id, username: user.username, role: user.role },
      user.gen,
    );
    if (s === null || s === 'expired')
      throw await fail(
        'credentials-changed-during-login',
        user.id,
        401,
        method === 'local' ? undefined : method,
      );
    const after =
      method === 'local' && Object.keys(extra).length === 0 ? undefined : { method, ...extra };
    await this.audit.write({
      userId: user.id,
      username: user.username,
      sourceIp: ip,
      action: 'auth.login',
      resource: user.username,
      ...(after === undefined ? {} : { after }),
      result: 'success',
      status: 200,
    });
    return s;
  }

  /**
   * Second factor of a login: redeem the ticket and, with a good code, issue the session.
   *
   * A wrong code costs the ticket (they are single-use) and buys a fresh one until `MAX_ATTEMPTS` is reached, so a
   * login allows a bounded number of tries and no ticket is ever replayable. Beyond that the password has to be
   * presented again — and THAT path is rate-limited and lockout-counted, which is what bounds code guessing overall.
   */
  async loginMfa(
    ticketToken: string,
    code: string,
    ip: string,
    secure: boolean,
  ): Promise<LoginOutcome> {
    const refuse = async (reason: string, userId: number | null, status = 401) => {
      await this.audit.write({
        userId,
        username: null,
        sourceIp: ip,
        action: 'auth.mfa',
        resource: null,
        after: { reason },
        result: 'failure',
        status,
      });
      return status === 403 ? tlsRequired() : problems.unauthorized('invalid credentials');
    };
    // the code is a credential: the same transport rule as the password (D-100 (1))
    if (!secure) throw await refuse('tls-required', null, 403);
    // A ticket is unguessable, so this is not what bounds code guessing (the 3 attempts and the rate-limited password
    // path are) — it is the same per-client budget the password route has, on its own bucket, so the second factor
    // cannot be hammered any harder than the first.
    if ((await this.tokens.hit(`mfa:${clientKey(ip)}`, 60)) > this.env.VRX_LOGIN_RATE_PER_MIN) {
      throw problems.tooMany('too many login attempts; try again in a minute');
    }
    const t = await this.tickets.consume(ticketToken);
    if (t === null) throw await refuse('no-ticket', null);
    if (t.purpose !== 'code') throw await refuse('enrolment-required', t.userId);
    const u = await this.currentUser(t.userId);
    // a reset, disable or delete between the two factors invalidates the ticket
    if (u === undefined || u.disabled || u.credentialGen !== t.gen)
      throw await refuse('credentials-changed-during-login', t.userId);
    if (!(await this.mfa.verify(t.userId, code))) {
      const attempts = t.attempts + 1;
      if (attempts >= MAX_ATTEMPTS) throw await refuse('bad-code-exhausted', t.userId);
      const ticket = await this.tickets.issue({ ...t, attempts });
      await this.audit.write({
        userId: t.userId,
        username: t.username,
        sourceIp: ip,
        action: 'auth.mfa',
        resource: t.username,
        after: { reason: 'bad-code', attempts },
        result: 'failure',
        status: 401,
      });
      return {
        mfa: 'code',
        ticket,
        expiresIn: TICKET_TTL_SEC,
        attemptsLeft: MAX_ATTEMPTS - attempts,
      };
    }
    const fail: FailFn = async (reason, userId, status = 401) => {
      await this.audit.write({
        userId,
        username: t.username,
        sourceIp: ip,
        action: 'auth.login',
        resource: t.username,
        after: { reason, method: t.method },
        result: 'failure',
        status,
      });
      return problems.unauthorized('invalid credentials');
    };
    return this.issueSession(
      { id: u.id, username: u.username, role: u.role, gen: u.credentialGen },
      ip,
      t.method,
      fail,
      { mfa: 'totp' },
    );
  }

  /**
   * Start enrolment from an `enrol` ticket (the account must set up MFA before it can have a session). Returns the
   * seed plus a FRESH ticket, because redeeming one consumes it.
   */
  async enrolWithTicket(
    ticketToken: string,
    secure: boolean,
  ): Promise<{ secret: string; otpauthUri: string; ticket: string; expiresIn: number }> {
    if (!secure) throw tlsRequired();
    const t = await this.tickets.consume(ticketToken);
    if (t === null || t.purpose !== 'enrol') throw problems.unauthorized('invalid credentials');
    const u = await this.currentUser(t.userId);
    if (u === undefined || u.disabled || u.credentialGen !== t.gen)
      throw problems.unauthorized('invalid credentials');
    const policy = await this.aaa.policy();
    const started = await this.mfa.begin(t.userId, t.username, policy.mfa.issuer);
    return {
      ...started,
      ticket: await this.tickets.issue({ ...t, attempts: 0 }),
      expiresIn: TICKET_TTL_SEC,
    };
  }

  /**
   * Confirm enrolment from an `enrol` ticket and finish the login in one step: the user has already proved the first
   * factor on this ticket and now proves possession of the new second factor, which is exactly what a login needs.
   */
  async confirmEnrolmentWithTicket(
    ticketToken: string,
    code: string,
    ip: string,
    secure: boolean,
  ): Promise<{ recoveryCodes: string[]; session: LoginResult }> {
    if (!secure) throw tlsRequired();
    const t = await this.tickets.consume(ticketToken);
    if (t === null || t.purpose !== 'enrol') throw problems.unauthorized('invalid credentials');
    const u = await this.currentUser(t.userId);
    if (u === undefined || u.disabled || u.credentialGen !== t.gen)
      throw problems.unauthorized('invalid credentials');
    const recoveryCodes = await this.mfa.confirm(t.userId, code);
    const fail: FailFn = async (reason, userId, status = 401) => {
      await this.audit.write({
        userId,
        username: t.username,
        sourceIp: ip,
        action: 'auth.login',
        resource: t.username,
        after: { reason, method: t.method },
        result: 'failure',
        status,
      });
      return problems.unauthorized('invalid credentials');
    };
    const session = await this.issueSession(
      { id: u.id, username: u.username, role: u.role, gen: u.credentialGen },
      ip,
      t.method,
      fail,
      { mfa: 'enrolled' },
    );
    return { recoveryCodes, session };
  }

  /** MFA state of the caller: enrolled, whether the policy requires it, and how many recovery codes are left. */
  async mfaStatus(p: Principal): Promise<MfaStatus> {
    const policy = await this.aaa.policy();
    return this.mfa.status(p.id, p.role, policy.mfa.required);
  }

  /**
   * Start self-enrolment from an existing session. The seed and URI are returned once; nothing is active until
   * `mfaConfirm` proves a code, so an abandoned enrolment cannot lock the account.
   */
  async mfaEnrol(p: Principal): Promise<{ secret: string; otpauthUri: string }> {
    const policy = await this.aaa.policy();
    return this.mfa.begin(p.id, p.username, policy.mfa.issuer);
  }

  /** Confirm self-enrolment; the recovery codes are returned once and only here. */
  async mfaConfirm(p: Principal, code: string): Promise<{ recoveryCodes: string[] }> {
    return { recoveryCodes: await this.mfa.confirm(p.id, code) };
  }

  /**
   * Turn MFA off for the caller. A current code is required: a stolen session must not be able to strip the second
   * factor it could not produce. Where the policy still requires MFA the next login asks for enrolment again.
   */
  async mfaDisable(p: Principal, code: string): Promise<void> {
    if (!(await this.mfa.verify(p.id, code)))
      throw problems.forbidden('that code does not match the enrolled factor');
    await this.mfa.reset(p.id);
  }
  /** The current app_user row of `id`, or undefined when it is gone. */
  private async currentUser(id: number) {
    const [u] = await this.db.select().from(appUser).where(eq(appUser.id, id));
    return u;
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
