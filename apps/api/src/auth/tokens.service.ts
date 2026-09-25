import { Inject, Injectable, Logger } from '@nestjs/common';
import { jwtVerify, SignJWT } from 'jose';
import { createHash, randomBytes } from 'node:crypto';
import { ENV, type Env } from '../config.js';
import { ROLES, type Role } from '../db/schema.js';
import { Bus } from '../infra/bus.js';
import { VALKEY, type Valkey } from '../infra/valkey.js';

const ISSUER = 'vrx-api';
const AUDIENCE = 'vrx';

export interface AccessClaims {
  id: number;
  username: string;
  role: Role;
  /** refresh-token family (login session) */
  sid?: string;
  exp?: number;
  /** credential generation (D-097) */
  gen?: number;
}

export interface IssuedRefresh {
  token: string;
  family: string;
  /** credential generation the chain belongs to (D-097) */
  gen: number;
}

function sha256(s: string): string {
  return createHash('sha256').update(s).digest('hex');
}

function b64url(bytes: number): string {
  return randomBytes(bytes).toString('base64url');
}

/**
 * Access tokens: HS256 JWT, 15 min by default (VRX_ACCESS_TTL_SEC). Refresh tokens: opaque `<family>.<secret>`,
 * stored hashed in Valkey, single use and rotated on every refresh; presenting a used token again revokes the whole
 * family (theft detection). They travel only in the httpOnly SameSite=Strict cookie.
 */
@Injectable()
export class TokensService {
  private readonly log = new Logger('Tokens');
  private readonly key: Uint8Array;
  /**
   * D-097/D-102 (TD-2 review H2/L3, verify V1/V3): every user has a credential GENERATION, `app_user.credential_gen`
   * in PostgreSQL — the authority, bumped in the same transaction as the password write. Refresh chains (`rtfam` =
   * `<uid>:<gen>`) and access tokens (`gen` claim) carry the generation they were issued under. A refresh continues a
   * chain only when its generation equals the column (read after the token is consumed), and API-key creation
   * compares the caller's generation with the column under `FOR SHARE`. So a reset is effective the moment its
   * transaction commits, even if Valkey is unreachable afterwards (V3). Access tokens below the revoked generation are
   * refused (except the caller's own session `keep`): recorded here first, then in Valkey (`atrev:<uid>`, TTL = access
   * lifetime, reloaded at boot).
   */
  private readonly revoked = new Map<number, { gen: number; keep?: string; until: number }>();

  constructor(
    @Inject(ENV) private readonly env: Env,
    @Inject(VALKEY) private readonly kv: Valkey,
    private readonly bus: Bus,
  ) {
    if (env.VRX_JWT_SECRET === undefined) {
      this.log.warn(
        'VRX_JWT_SECRET is not set: using a random per-process key (sessions end when the API restarts)',
      );
      this.key = randomBytes(32);
    } else {
      this.key = new TextEncoder().encode(env.VRX_JWT_SECRET);
    }
  }

  get accessTtl(): number {
    return this.env.VRX_ACCESS_TTL_SEC;
  }
  get refreshTtl(): number {
    return this.env.VRX_REFRESH_TTL_SEC;
  }

  signAccess(c: AccessClaims): Promise<string> {
    return new SignJWT({
      username: c.username,
      role: c.role,
      typ: 'access',
      /** credential generation the session was issued under (D-097) */
      gen: c.gen ?? 0,
      ...(c.sid ? { sid: c.sid } : {}),
    })
      .setProtectedHeader({ alg: 'HS256' })
      .setSubject(String(c.id))
      .setIssuer(ISSUER)
      .setAudience(AUDIENCE)
      .setIssuedAt()
      .setJti(b64url(12))
      .setExpirationTime(`${this.accessTtl}s`)
      .sign(this.key);
  }

  /** Verified claims, or null for anything invalid/expired. */
  async verifyAccess(token: string): Promise<AccessClaims | null> {
    try {
      const { payload } = await jwtVerify(token, this.key, {
        issuer: ISSUER,
        audience: AUDIENCE,
        algorithms: ['HS256'],
      });
      const id = Number(payload.sub);
      const role = payload['role'];
      const username = payload['username'];
      if (
        payload['typ'] !== 'access' ||
        !Number.isInteger(id) ||
        typeof username !== 'string' ||
        !(ROLES as readonly unknown[]).includes(role)
      ) {
        return null;
      }
      const sid = typeof payload['sid'] === 'string' ? payload['sid'] : undefined;
      const gen = typeof payload['gen'] === 'number' ? payload['gen'] : -1;
      const rev = this.revoked.get(id);
      if (rev !== undefined) {
        if (rev.until < Date.now()) this.revoked.delete(id);
        else if (gen < rev.gen && !(rev.keep !== undefined && sid === rev.keep)) return null;
      }
      return {
        id,
        username,
        role: role as Role,
        ...(sid ? { sid } : {}),
        ...(payload.exp ? { exp: payload.exp } : {}),
        gen,
      };
    } catch {
      return null;
    }
  }

  /**
   * New refresh token under credential generation `gen` (read from app_user by the caller), atomically (one Lua
   * script, review H2):
   * - `family` given: continues that chain only while `rtfam:<family>` still says `<uid>:<gen>` — a chain of an older
   *   generation, or one deleted by a reset/logout/reuse, is refused and never re-created;
   * - no `family`: starts a new chain (login).
   * Returns null when the chain was revoked.
   */
  async issueRefresh(userId: number, gen: number, family?: string): Promise<IssuedRefresh | null> {
    const fam = family ?? b64url(12);
    const token = `${fam}.${b64url(32)}`;
    const r = (await this.kv.eval(
      ISSUE_SCRIPT,
      3,
      `rtfam:${fam}`,
      `rt:${sha256(token)}`,
      `rtuser:${userId}`,
      String(userId),
      family === undefined ? 'new' : 'continue',
      String(gen),
      String(this.refreshTtl),
      JSON.stringify({ uid: userId, fam }),
      fam,
    )) as number;
    return r < 0 ? null : { token, family: fam, gen };
  }

  /**
   * Consume a refresh token: returns the user id and family, or `{reuse: true}` when a rotated-away token is presented
   * again (the family is revoked), or null when unknown/expired/revoked.
   */
  async consumeRefresh(
    token: string,
  ): Promise<{ uid: number; family: string } | { reuse: true } | null> {
    if (!/^[A-Za-z0-9_-]{16}\.[A-Za-z0-9_-]{43}$/.test(token)) return null;
    const h = sha256(token);
    const raw = await this.kv.getdel(`rt:${h}`);
    if (raw === null) {
      const fam = await this.kv.get(`rtused:${h}`);
      if (fam !== null) {
        await this.kv.del(`rtfam:${fam}`);
        return { reuse: true };
      }
      return null;
    }
    const v = JSON.parse(raw) as { uid: number; fam: string };
    await this.kv.set(`rtused:${h}`, v.fam, 'EX', this.refreshTtl);
    if ((await this.kv.exists(`rtfam:${v.fam}`)) === 0) return null;
    return { uid: v.uid, family: v.fam };
  }

  /** Logout: the token and its whole family stop working. */
  async revokeRefresh(token: string): Promise<void> {
    const family = token.split('.')[0];
    if (family === undefined || !/^[A-Za-z0-9_-]{16}$/.test(family)) return;
    await this.kv.del(`rt:${sha256(token)}`, `rtfam:${family}`);
  }

  /**
   * Sessions of `userId` end after a password set committed generation `gen` (D-097/D-102) — except `keep`, the
   * session (refresh family) of a user who changed their own password. Order (verify V3): the in-process revocation
   * and the WebSocket close come first and cannot fail; Valkey follows (kept family moved to `gen`, the other families
   * deleted, `atrev` persisted for other processes/restarts). A Valkey failure is logged and reported
   * (`persisted: false`) but does not undo anything: refresh and key creation check the generation in PostgreSQL.
   */
  async revokeUser(
    userId: number,
    gen: number,
    keep?: string,
  ): Promise<{ persisted: boolean; families: number }> {
    const ttl = this.accessTtl + 5;
    const entry = { gen, until: Date.now() + ttl * 1000, ...(keep !== undefined ? { keep } : {}) };
    const cur = this.revoked.get(userId);
    if (cur === undefined || cur.gen <= gen) this.revoked.set(userId, entry);
    this.bus.sessions({ userId, ...(keep !== undefined ? { exceptSid: keep } : {}) });
    try {
      await this.kv.eval(
        REVOKE_SCRIPT,
        2,
        `rtfam:${keep ?? '-'}`,
        `atrev:${userId}`,
        String(userId),
        keep === undefined ? '0' : '1',
        String(gen),
        JSON.stringify(entry),
        String(ttl),
      );
      const fams = (await this.kv.smembers(`rtuser:${userId}`)).filter((f) => f !== keep);
      if (fams.length > 0) {
        await this.kv.del(...fams.map((f) => `rtfam:${f}`));
        await this.kv.srem(`rtuser:${userId}`, ...fams);
      }
      return { persisted: true, families: fams.length };
    } catch (e) {
      this.log.error(
        `user ${userId}: sessions revoked in this process and by the database generation ${gen}, but Valkey could not be updated (${(e as Error).message}); old access tokens would be accepted again after an API restart within ${ttl}s`,
      );
      return { persisted: false, families: 0 };
    }
  }

  /**
   * TD-2 verify V1: may this JWT session still mint credentials, given the user's generation in PostgreSQL? Yes when
   * it was issued under that generation, or when it is the session a self-service change kept.
   */
  sessionCurrent(p: { id: number; gen?: number; sid?: string }, dbGen: number): boolean {
    if ((p.gen ?? -1) >= dbGen) return true;
    const rev = this.revoked.get(p.id);
    return rev !== undefined && rev.gen === dbGen && rev.keep !== undefined && rev.keep === p.sid;
  }

  /** Boot: reload the access-token revocations that are still within one token lifetime (review L3). */
  async loadRevocations(): Promise<number> {
    const prefix = this.env.VRX_VALKEY_PREFIX;
    let cursor = '0';
    let n = 0;
    do {
      // SCAN patterns and results are not prefixed by the client: add/strip the prefix here
      const [next, keys] = await this.kv.scan(cursor, 'MATCH', `${prefix}atrev:*`, 'COUNT', 500);
      cursor = next;
      for (const full of keys) {
        const raw = await this.kv.get(full.slice(prefix.length));
        const uid = Number(full.slice(prefix.length + 'atrev:'.length));
        if (raw === null || !Number.isInteger(uid)) continue;
        const e = JSON.parse(raw) as { gen: number; keep?: string; until: number };
        const cur = this.revoked.get(uid);
        if (cur === undefined || cur.gen < e.gen) this.revoked.set(uid, e);
        n += 1;
      }
    } while (cursor !== '0');
    return n;
  }

  /** Family of a refresh token (without consuming it). */
  familyOf(token: string): string | undefined {
    const f = token.split('.')[0];
    return f !== undefined && /^[A-Za-z0-9_-]{16}$/.test(f) ? f : undefined;
  }

  /** Fixed-window counter; returns the count within the current window. */
  async hit(key: string, windowSec: number): Promise<number> {
    const k = `rl:${key}:${Math.floor(Date.now() / 1000 / windowSec)}`;
    const [[, n] = [null, 0]] = ((await this.kv
      .multi()
      .incr(k)
      .expire(k, windowSec + 1)
      .exec()) ?? []) as [Error | null, number][];
    return Number(n);
  }
}

/**
 * KEYS: rtfam, rt, rtuser · ARGV: uid, mode (new|continue), gen, ttl, record, family.
 * Returns the generation, or -1 when the chain is revoked (continue: family gone or of another generation).
 */
const ISSUE_SCRIPT = `
local cur = ARGV[1] .. ':' .. ARGV[3]
if ARGV[2] == 'continue' and redis.call('GET', KEYS[1]) ~= cur then return -1 end
local ttl = tonumber(ARGV[4])
redis.call('SET', KEYS[2], ARGV[5], 'EX', ttl)
redis.call('SET', KEYS[1], cur, 'EX', ttl)
redis.call('SADD', KEYS[3], ARGV[6])
redis.call('EXPIRE', KEYS[3], ttl)
return tonumber(ARGV[3])
`;

/**
 * KEYS: rtfam of the kept family, atrev · ARGV: uid, keep (0|1), gen, revocation entry, ttl.
 * Moves the kept family to the new generation; persists the access-token revocation unless a newer one is stored.
 */
const REVOKE_SCRIPT = `
if ARGV[2] == '1' then
  local f = redis.call('GET', KEYS[1])
  if f and string.sub(f, 1, string.len(ARGV[1]) + 1) == (ARGV[1] .. ':') then
    redis.call('SET', KEYS[1], ARGV[1] .. ':' .. ARGV[3], 'KEEPTTL')
  end
end
local prev = redis.call('GET', KEYS[2])
if prev then
  local g = tonumber(string.match(prev, '"gen":(%d+)'))
  if g and g > tonumber(ARGV[3]) then return 0 end
end
redis.call('SET', KEYS[2], ARGV[4], 'EX', tonumber(ARGV[5]))
return 1
`;

export function apiKeyHash(token: string): string {
  return sha256(token);
}

export function newApiKeyToken(): string {
  return `vrxk_${b64url(32)}`;
}
