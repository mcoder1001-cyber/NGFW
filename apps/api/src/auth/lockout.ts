import { and, eq, isNull, lte, ne, or, sql } from 'drizzle-orm';
import { clientKey } from '../common/principal.js';
import type { Env } from '../config.js';
import type { Db } from '../db/db.js';
import { appUser, type Role } from '../db/schema.js';
import type { Valkey } from '../infra/valkey.js';

/** A throttle never exceeds this (seconds), nor VRX_LOGIN_LOCKOUT_SEC. */
export const THROTTLE_CAP_SEC = 60;

export type LockState = 'open' | 'locked' | 'throttled';
export type FailOutcome = 'counted' | 'locked' | 'throttled';

export interface LockSubject {
  id: number;
  /** credential generation: a password reset or a disable (new generation) leaves every old lock behind */
  credentialGen: number;
  role: Role;
  disabled: boolean;
}

/**
 * TD-10b (review 2.3a): the LOGIN lockout, keyed by (user, credential generation, client address) in Valkey — so a
 * remote guesser locks the account only for its own address, never for the owner elsewhere (before: one lock per
 * account, and ~10 guesses per 15 min kept any account, the only admin included, locked for everyone).
 *
 * - `lkf:<uid>:<gen>:<client>` counts failures (sliding window VRX_LOGIN_LOCKOUT_SEC); at VRX_LOGIN_MAX_FAILURES
 *   `lk:<uid>:<gen>:<client>` = `lock` for VRX_LOGIN_LOCKOUT_SEC.
 * - The LAST ADMIN is only throttled, never locked: when no other enabled admin could still log in from that client
 *   address (not locked there, not locked account-wide), the lock is `throttle` for 1, 2, 4 … ≤ 60 s instead. Decided
 *   atomically in one script together with the other admins' keys, so two parallel bursts cannot lock two admins.
 * - `<client>` is `clientKey(ip)`: IPv4, or the IPv6 /64.
 * - A generation bump (password reset, D-102 config reset, disable) starts from a clean slate; `unlockUser`
 *   (break-glass.ts) clears every key of a user.
 * The account-wide lock in app_user (`locked_until`) stays for password checks made from INSIDE a session (TD-4's
 * API-key step-up, TD-2's own-password change): the caller already holds a session of that account (D-097/D-100).
 */
export class Lockout {
  constructor(
    private readonly kv: Valkey,
    private readonly db: Db,
    private readonly env: Env,
  ) {}

  private keys(uid: number, gen: number, ip: string) {
    const c = clientKey(ip);
    return { fails: `lkf:${uid}:${gen}:${c}`, lock: `lk:${uid}:${gen}:${c}` };
  }

  async state(u: Pick<LockSubject, 'id' | 'credentialGen'>, ip: string): Promise<LockState> {
    const v = await this.kv.get(this.keys(u.id, u.credentialGen, ip).lock);
    return v === null ? 'open' : v === 'throttle' ? 'throttled' : 'locked';
  }

  /** One failed login of `u` from `ip`. */
  async fail(u: LockSubject, ip: string): Promise<FailOutcome> {
    const k = this.keys(u.id, u.credentialGen, ip);
    const admin = u.role === 'admin' && !u.disabled;
    // the other admins who could log in: enabled and not locked account-wide (their per-address locks: in the script)
    const others = admin
      ? await this.db
          .select({ id: appUser.id, gen: appUser.credentialGen })
          .from(appUser)
          .where(
            and(
              eq(appUser.role, 'admin'),
              eq(appUser.disabled, false),
              ne(appUser.id, u.id),
              or(isNull(appUser.lockedUntil), lte(appUser.lockedUntil, sql`now()`)),
            ),
          )
      : [];
    const lockSec = this.env.VRX_LOGIN_LOCKOUT_SEC;
    const r = (await this.kv.eval(
      FAIL_SCRIPT,
      2 + others.length,
      k.fails,
      k.lock,
      ...others.map((o) => this.keys(o.id, o.gen, ip).lock),
      String(this.env.VRX_LOGIN_MAX_FAILURES),
      String(lockSec),
      admin ? '1' : '0',
      String(Math.min(THROTTLE_CAP_SEC, lockSec)),
    )) as number;
    return r === 1 ? 'locked' : r === 2 ? 'throttled' : 'counted';
  }

  /**
   * The last gate of a login with the right password (P06 review H1, for the per-address key): refused when a
   * parallel wrong guess locked or throttled this (user, address) since `state()`; otherwise the address's failure
   * count restarts. One script, so no failure lands between the check and the reset.
   */
  async admit(u: Pick<LockSubject, 'id' | 'credentialGen'>, ip: string): Promise<boolean> {
    const k = this.keys(u.id, u.credentialGen, ip);
    return (await this.kv.eval(ADMIT_SCRIPT, 2, k.lock, k.fails)) === 1;
  }
}

/** KEYS: lock, fails. 0 = locked/throttled, 1 = admitted (failures reset). */
const ADMIT_SCRIPT = `
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
redis.call('DEL', KEYS[2])
return 1
`;

/**
 * KEYS: fails, lock, the other admins' lock keys for the same client · ARGV: max, lock seconds, admin (0|1),
 * throttle cap. Returns 0 counted, 1 locked, 2 throttled.
 */
const FAIL_SCRIPT = `
local n = redis.call('INCR', KEYS[1])
redis.call('EXPIRE', KEYS[1], tonumber(ARGV[2]))
local max = tonumber(ARGV[1])
if n < max then return 0 end
local canLock = ARGV[3] ~= '1'
if not canLock then
  for i = 3, #KEYS do
    local v = redis.call('GET', KEYS[i])
    if (not v) or v == 'throttle' then canLock = true break end
  end
end
if canLock then
  redis.call('SET', KEYS[2], 'lock', 'EX', tonumber(ARGV[2]))
  redis.call('DEL', KEYS[1])
  return 1
end
local d = 1
local e = n - max
while e > 0 and d < tonumber(ARGV[4]) do d = d * 2 e = e - 1 end
if d > tonumber(ARGV[4]) then d = tonumber(ARGV[4]) end
redis.call('SET', KEYS[2], 'throttle', 'EX', d)
return 2
`;
