import { Inject, Injectable, Logger } from '@nestjs/common';
import { decodeProtectedHeader, jwtVerify, SignJWT, type JWTPayload } from 'jose';
import { createHash, randomBytes } from 'node:crypto';
import { readFileSync, type Stats } from 'node:fs';
import { ENV, type Env } from '../config.js';
import { ROLES, type Role } from '../db/schema.js';
import { Bus } from '../infra/bus.js';
import { VALKEY, type Valkey } from '../infra/valkey.js';
import { checkKeyFile, KeyFileError } from './key-file.js';

const ISSUER = 'vrx-api';
const AUDIENCE = 'vrx';
/** How often a VRX_JWT_KEY_FILE is checked for a change (stat only; re-read when it changed). */
const RING_CHECK_MS = 5_000;
const REFRESH_TOKEN = /^[A-Za-z0-9_-]{16}\.[A-Za-z0-9_-]{43}$/;

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
  /** Seconds this refresh token (and its cookie) lives: the idle TTL, capped by the session's end. */
  maxAge: number;
  /** TD-10b (review 2.3d): the session's absolute end (epoch seconds) = login + VRX_SESSION_MAX_SEC. */
  notAfter: number;
}

/** `consumeRefresh` outcomes other than a live chain (TD-10b: each one audited with its reason). */
export type ConsumedRefresh =
  | { uid: number; family: string }
  /** a rotated-away token presented again: the family is revoked (theft detection) */
  | { reuse: true; family: string; uid?: number }
  /** a real token of a chain that no longer exists: logged out, revoked, expired */
  | { ended: true; uid: number }
  | null;

interface RingKey {
  kid: string;
  key: Uint8Array;
}

function sha256(s: string): string {
  return createHash('sha256').update(s).digest('hex');
}

function b64url(bytes: number): string {
  return randomBytes(bytes).toString('base64url');
}

/** `kid` of a signing key: a truncated hash, so a token names its key without revealing anything about it. */
function ringKey(secret: string | Uint8Array): RingKey {
  const key = typeof secret === 'string' ? new TextEncoder().encode(secret) : secret;
  return {
    kid: createHash('sha256').update('vrx-jwt-kid:').update(key).digest('hex').slice(0, 16),
    key,
  };
}

/**
 * VRX_JWT_KEY_FILE content → keys, newest (the signing key) first. One key per line (≥ 32 characters), `#` comments
 * and blank lines ignored. Errors name the line, never its content.
 */
export function parseKeyRing(text: string, path: string): string[] {
  const keys: string[] = [];
  text.split('\n').forEach((raw, i) => {
    const line = raw.trim();
    if (line === '' || line.startsWith('#')) return;
    if (line.length < 32)
      throw new KeyFileError(path, `line ${i + 1}: a key needs ≥ 32 characters`);
    keys.push(line);
  });
  if (keys.length === 0) throw new KeyFileError(path, 'holds no key');
  return keys;
}

/**
 * Access tokens: HS256 JWT, 15 min by default (VRX_ACCESS_TTL_SEC), signed with the newest key of a key ring (`kid`
 * header; TD-10b: VRX_JWT_KEY_FILE rotates without ending sessions). Refresh tokens: opaque `<family>.<secret>`,
 * stored hashed in Valkey, single use and rotated on every refresh; presenting a used token again revokes the whole
 * family (theft detection). They travel only in the httpOnly SameSite=Strict cookie. A login session (family) lives
 * at most VRX_SESSION_MAX_SEC from its login (TD-10b), and a logout ends it at once — refresh chain AND access tokens.
 */
@Injectable()
export class TokensService {
  private readonly log = new Logger('Tokens');
  /** Signing key first, then the keys that still verify (rotation). */
  private ring: RingKey[];
  private ringStat: Pick<Stats, 'mtimeMs' | 'size' | 'ino'> | undefined;
  private ringCheckedAt = 0;
  /**
   * D-097/D-102 (TD-2 review H2/L3, verify V1/V3): every user has a credential GENERATION, `app_user.credential_gen`
   * in PostgreSQL — the authority, bumped in the same transaction as the password write. Refresh chains (`rtfam` =
   * `<uid>:<gen>:<login time>`) and access tokens (`gen` claim) carry the generation they were issued under. A
   * refresh continues a chain only when its generation equals the column (read after the token is consumed), and
   * API-key creation compares the caller's generation with the column under `FOR SHARE`. So a reset is effective the
   * moment its transaction commits, even if Valkey is unreachable afterwards (V3). Access tokens below the revoked
   * generation are refused (except the caller's own session `keep`): recorded here first, then in Valkey
   * (`atrev:<uid>`, TTL = access lifetime, reloaded at boot).
   */
  private readonly revoked = new Map<number, { gen: number; keep?: string; until: number }>();
  /**
   * TD-10b (review 2.3c): ended login sessions (logout, refresh-token reuse) → until (ms). Their access tokens are
   * refused for the rest of their lifetime; persisted as `atrevsid:<sid>` (TTL = access lifetime), reloaded at boot.
   */
  private readonly revokedSids = new Map<string, number>();

  constructor(
    @Inject(ENV) private readonly env: Env,
    @Inject(VALKEY) private readonly kv: Valkey,
    private readonly bus: Bus,
  ) {
    if (env.VRX_JWT_KEY_FILE !== undefined) {
      // a bad key file stops the API at boot (owner, mode, content) — never a silent fallback to a weaker key
      this.ring = this.readRing(env.VRX_JWT_KEY_FILE);
    } else if (env.VRX_JWT_SECRET !== undefined) {
      this.ring = [ringKey(env.VRX_JWT_SECRET)];
    } else {
      this.log.warn(
        'neither VRX_JWT_KEY_FILE nor VRX_JWT_SECRET is set: using a random per-process key (sessions end when the API restarts)',
      );
      this.ring = [ringKey(randomBytes(32))];
    }
  }

  get accessTtl(): number {
    return this.env.VRX_ACCESS_TTL_SEC;
  }
  get refreshTtl(): number {
    return this.env.VRX_REFRESH_TTL_SEC;
  }
  get sessionMax(): number {
    return this.env.VRX_SESSION_MAX_SEC;
  }

  private readRing(path: string): RingKey[] {
    const st = checkKeyFile(path);
    const ring = parseKeyRing(readFileSync(path, 'utf8'), path).map((k) => ringKey(k));
    this.ringStat = { mtimeMs: st.mtimeMs, size: st.size, ino: st.ino };
    return ring;
  }

  /** Re-read VRX_JWT_KEY_FILE when it changed (checked at most every 5 s); a bad file keeps the current ring. */
  private refreshRing(): void {
    const path = this.env.VRX_JWT_KEY_FILE;
    if (path === undefined || Date.now() - this.ringCheckedAt < RING_CHECK_MS) return;
    this.ringCheckedAt = Date.now();
    try {
      const st = checkKeyFile(path);
      const s = this.ringStat;
      if (s !== undefined && s.mtimeMs === st.mtimeMs && s.size === st.size && s.ino === st.ino)
        return;
      const before = this.ring[0]?.kid;
      this.ring = this.readRing(path);
      this.log.log(
        `JWT key ring reloaded: ${this.ring.length} key(s), signing key ${before === this.ring[0]?.kid ? 'unchanged' : 'rotated'}`,
      );
    } catch (e) {
      this.log.error(`JWT key ring NOT reloaded, the previous keys stay: ${(e as Error).message}`);
    }
  }

  /**
   * An access token for `c`, valid VRX_ACCESS_TTL_SEC — never beyond `notAfter` (epoch seconds; TD-10b: the end of
   * the login session, VRX_SESSION_MAX_SEC).
   */
  signAccess(c: AccessClaims, notAfter?: number): Promise<string> {
    this.refreshRing();
    const k = this.ring[0]!;
    const now = Math.floor(Date.now() / 1000);
    return new SignJWT({
      username: c.username,
      role: c.role,
      typ: 'access',
      /** credential generation the session was issued under (D-097) */
      gen: c.gen ?? 0,
      ...(c.sid ? { sid: c.sid } : {}),
    })
      .setProtectedHeader({ alg: 'HS256', kid: k.kid })
      .setSubject(String(c.id))
      .setIssuer(ISSUER)
      .setAudience(AUDIENCE)
      .setIssuedAt(now)
      .setJti(b64url(12))
      .setExpirationTime(Math.min(now + this.accessTtl, notAfter ?? Number.MAX_SAFE_INTEGER))
      .sign(k.key);
  }

  /** Verified claims, or null for anything invalid/expired/revoked. */
  async verifyAccess(token: string): Promise<AccessClaims | null> {
    this.refreshRing();
    let kid: unknown;
    try {
      kid = decodeProtectedHeader(token).kid;
    } catch {
      return null;
    }
    // a token names its key; a token from before the key ring (no kid) is tried against every key
    const keys = typeof kid === 'string' ? this.ring.filter((k) => k.kid === kid) : this.ring;
    for (const k of keys) {
      let payload: JWTPayload;
      try {
        ({ payload } = await jwtVerify(token, k.key, {
          issuer: ISSUER,
          audience: AUDIENCE,
          algorithms: ['HS256'],
        }));
      } catch {
        continue;
      }
      return this.claims(payload);
    }
    return null;
  }

  private claims(payload: JWTPayload): AccessClaims | null {
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
    if (sid !== undefined && this.sidRevoked(sid)) return null;
    return {
      id,
      username,
      role: role as Role,
      ...(sid ? { sid } : {}),
      ...(payload.exp ? { exp: payload.exp } : {}),
      gen,
    };
  }

  /**
   * New refresh token under credential generation `gen` (read from app_user by the caller), atomically (one Lua
   * script, review H2):
   * - `family` given: continues that chain only while `rtfam:<family>` still says `<uid>:<gen>` — a chain of an older
   *   generation, or one deleted by a reset/logout/reuse, is refused and never re-created;
   * - no `family`: starts a new chain (login) and records the login time.
   * TD-10b (review 2.3d): a chain older than VRX_SESSION_MAX_SEC is not continued (and deleted); the token's lifetime
   * is the idle TTL capped by the time the session has left.
   * Returns null when the chain was revoked, `expired` when the session reached its maximum age.
   */
  async issueRefresh(
    userId: number,
    gen: number,
    family?: string,
  ): Promise<IssuedRefresh | 'expired' | null> {
    const fam = family ?? b64url(12);
    const token = `${fam}.${b64url(32)}`;
    const now = Math.floor(Date.now() / 1000);
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
      String(now),
      String(this.sessionMax),
    )) as [number, number?, number?];
    if (r[0] === -2) return 'expired';
    if (r[0] < 0) return null;
    return {
      token,
      family: fam,
      gen,
      maxAge: Number(r[1]),
      notAfter: Number(r[2]) + this.sessionMax,
    };
  }

  /**
   * Consume a refresh token (TD-10b: every outcome names the user where it is known, for the audit row): a live chain
   * → user id and family; a rotated-away token presented again → `reuse` (the family is revoked here); a real token
   * whose chain is gone → `ended`; anything else → null.
   */
  async consumeRefresh(token: string): Promise<ConsumedRefresh> {
    if (!REFRESH_TOKEN.test(token)) return null;
    const h = sha256(token);
    const raw = await this.kv.getdel(`rt:${h}`);
    if (raw === null) {
      const fam = await this.kv.get(`rtused:${h}`);
      if (fam === null) return null;
      const uid = uidOf(await this.kv.get(`rtfam:${fam}`));
      await this.kv.del(`rtfam:${fam}`);
      return { reuse: true, family: fam, ...(uid !== undefined ? { uid } : {}) };
    }
    const v = JSON.parse(raw) as { uid: number; fam: string };
    await this.kv.set(`rtused:${h}`, v.fam, 'EX', this.refreshTtl);
    if ((await this.kv.exists(`rtfam:${v.fam}`)) === 0) return { ended: true, uid: v.uid };
    return { uid: v.uid, family: v.fam };
  }

  /**
   * Logout (TD-10b, review 2.3c): the login session a refresh token belongs to ends — its chain is deleted and its
   * access tokens are refused from now on (`revokeSession`). Only a token the store knows counts (the current one, or
   * one rotated away): a forged `<family>.<anything>` ends nobody's session. Returns the session, or null.
   */
  async endSession(token: string): Promise<{ sid: string; uid: number | undefined } | null> {
    if (!REFRESH_TOKEN.test(token)) return null;
    const h = sha256(token);
    const raw = await this.kv.getdel(`rt:${h}`);
    let sid: string | null;
    let uid: number | undefined;
    if (raw !== null) {
      const v = JSON.parse(raw) as { uid: number; fam: string };
      sid = v.fam;
      uid = v.uid;
    } else {
      sid = await this.kv.get(`rtused:${h}`);
      if (sid === null) return null;
      uid = uidOf(await this.kv.get(`rtfam:${sid}`));
    }
    await this.endSid(sid, uid);
    return { sid, uid };
  }

  /** End login session `sid` (of `uid` when known): chain deleted, access tokens revoked. */
  async endSid(sid: string, uid: number | undefined): Promise<boolean> {
    await this.kv.del(`rtfam:${sid}`);
    if (uid !== undefined) await this.kv.srem(`rtuser:${uid}`, sid);
    return this.revokeSession(sid);
  }

  /**
   * The access tokens of login session `sid` are refused from now on (TD-10b, review 2.3c): in this process first
   * (cannot fail), then in Valkey for restarts. Returns whether it was persisted.
   */
  async revokeSession(sid: string): Promise<boolean> {
    const ttl = this.accessTtl + 5;
    const until = Date.now() + ttl * 1000;
    this.revokedSids.set(sid, until);
    if (this.revokedSids.size > 1000) {
      const now = Date.now();
      for (const [s, u] of this.revokedSids) if (u < now) this.revokedSids.delete(s);
    }
    try {
      await this.kv.set(`atrevsid:${sid}`, String(until), 'EX', ttl);
      return true;
    } catch (e) {
      this.log.error(
        `session revoked in this process, but Valkey could not be updated (${(e as Error).message}); its access tokens would be accepted again after an API restart within ${ttl}s`,
      );
      return false;
    }
  }

  private sidRevoked(sid: string): boolean {
    const until = this.revokedSids.get(sid);
    if (until === undefined) return false;
    if (until >= Date.now()) return true;
    this.revokedSids.delete(sid);
    return false;
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

  /**
   * Boot: reload the access-token revocations that are still within one token lifetime — per user (review L3) and,
   * TD-10b, per ended login session.
   */
  async loadRevocations(): Promise<number> {
    let n = 0;
    for (const full of await this.scanKeys('atrev:*')) {
      const raw = await this.kv.get(full);
      const uid = Number(full.slice('atrev:'.length));
      if (raw === null || !Number.isInteger(uid)) continue;
      const e = JSON.parse(raw) as { gen: number; keep?: string; until: number };
      const cur = this.revoked.get(uid);
      if (cur === undefined || cur.gen < e.gen) this.revoked.set(uid, e);
      n += 1;
    }
    for (const key of await this.scanKeys('atrevsid:*')) {
      const until = Number(await this.kv.get(key));
      if (!Number.isFinite(until) || until < Date.now()) continue;
      this.revokedSids.set(key.slice('atrevsid:'.length), until);
      n += 1;
    }
    return n;
  }

  /** Keys matching `pattern`, WITHOUT the client's prefix (SCAN is not prefixed by the client). */
  private async scanKeys(pattern: string): Promise<string[]> {
    const prefix = this.env.VRX_VALKEY_PREFIX;
    const out: string[] = [];
    let cursor = '0';
    do {
      const [next, keys] = await this.kv.scan(cursor, 'MATCH', `${prefix}${pattern}`, 'COUNT', 500);
      cursor = next;
      for (const k of keys) out.push(k.slice(prefix.length));
    } while (cursor !== '0');
    return out;
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

/** The user id of an `rtfam` value (`<uid>:<gen>[:<t0>]`). */
function uidOf(v: string | null): number | undefined {
  const uid = Number(v?.split(':')[0]);
  return v !== null && Number.isInteger(uid) ? uid : undefined;
}

/**
 * KEYS: rtfam, rt, rtuser · ARGV: uid, mode (new|continue), gen, idle ttl, record, family, now, session max.
 * `rtfam` = `<uid>:<gen>:<login time>`; a chain from before TD-10b (`<uid>:<gen>`) starts its clock now.
 * Returns {gen, token ttl, login time}, {-1} when the chain is revoked (continue: gone or of another generation),
 * {-2} when the session is older than the maximum (the chain is deleted). The token ttl (= the cookie's max-age) never
 * passes the session's end; the Valkey records stay up to 5 min longer so that a late refresh is recognised.
 */
const ISSUE_SCRIPT = `
local now = tonumber(ARGV[7])
local t0 = now
if ARGV[2] == 'continue' then
  local v = redis.call('GET', KEYS[1])
  if not v then return {-1} end
  local u, g, t = string.match(v, '^(%d+):(%d+):(%d+)$')
  if not u then
    u, g = string.match(v, '^(%d+):(%d+)$')
    t = ARGV[7]
  end
  if u ~= ARGV[1] or g ~= ARGV[3] then return {-1} end
  t0 = tonumber(t)
end
local left = t0 + tonumber(ARGV[8]) - now
if left <= 0 then
  redis.call('DEL', KEYS[1])
  return {-2}
end
local ttl = tonumber(ARGV[4])
local keep = ttl
if left < ttl then
  ttl = left
  -- the records outlive the session by 5 min (this script refuses them): a late refresh is audited as
  -- session-expired, not as an unknown token
  keep = left + 300
  if keep > tonumber(ARGV[4]) then keep = tonumber(ARGV[4]) end
end
redis.call('SET', KEYS[2], ARGV[5], 'EX', keep)
redis.call('SET', KEYS[1], ARGV[1] .. ':' .. ARGV[3] .. ':' .. string.format('%d', t0), 'EX', keep)
redis.call('SADD', KEYS[3], ARGV[6])
redis.call('EXPIRE', KEYS[3], tonumber(ARGV[4]))
return {tonumber(ARGV[3]), ttl, t0}
`;

/**
 * KEYS: rtfam of the kept family, atrev · ARGV: uid, keep (0|1), gen, revocation entry, ttl.
 * Moves the kept family to the new generation (keeping its login time); persists the access-token revocation unless a
 * newer one is stored.
 */
const REVOKE_SCRIPT = `
if ARGV[2] == '1' then
  local f = redis.call('GET', KEYS[1])
  if f then
    local u, g, t = string.match(f, '^(%d+):(%d+):?(%d*)$')
    if u == ARGV[1] then
      local v = ARGV[1] .. ':' .. ARGV[3]
      if t and t ~= '' then v = v .. ':' .. t end
      redis.call('SET', KEYS[1], v, 'KEEPTTL')
    end
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
