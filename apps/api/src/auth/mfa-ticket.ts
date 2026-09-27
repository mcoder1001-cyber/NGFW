import { createHash, randomBytes } from 'node:crypto';
import type { Role } from '../db/schema.js';
import type { Valkey } from '../infra/valkey.js';

/** What a login already proved, carried from the first factor to the second. */
export interface MfaTicket {
  userId: number;
  username: string;
  role: Role;
  /** Credential generation the first factor was checked under, so a reset in between invalidates the ticket. */
  gen: number;
  /**
   * `code` — the user is enrolled and owes a TOTP (or recovery) code.
   * `enrol` — MFA is required for this role but the user has no factor yet, so the ticket buys enrolment and nothing
   * else. Without this an unenrolled user could never get in, and letting them in unprotected would make
   * `mfa.required` advisory.
   */
  purpose: 'code' | 'enrol';
  /** The AAA method that authenticated the first factor (audited with the login). */
  method: string;
  /** Wrong codes spent so far. */
  attempts: number;
}

/** Seconds a ticket lives: long enough to read a code off a phone, short enough to be worthless once captured. */
export const TICKET_TTL_SEC = 180;

/** Wrong codes allowed per login before the ticket dies and the password has to be presented again. */
export const MAX_ATTEMPTS = 3;

const key = (token: string) => `mfat:${createHash('sha256').update(token).digest('hex')}`;

/**
 * Tickets between the two factors of a login. A ticket is a 256-bit random bearer token; only its sha256 is stored,
 * under a TTL, and `consume` uses GETDEL so a token can be redeemed exactly once. A wrong code therefore cannot be
 * retried against the same token — the caller issues a fresh ticket with `attempts + 1`, which bounds the tries per
 * login at `MAX_ATTEMPTS` without a replayable credential ever existing.
 *
 * A ticket proves only the FIRST factor. It is not a session: it carries no access token and buys nothing but a
 * second-factor attempt (or, for `purpose: 'enrol'`, an enrolment).
 */
export class MfaTickets {
  constructor(private readonly kv: Valkey) {}

  async issue(t: MfaTicket): Promise<string> {
    const token = randomBytes(32).toString('base64url');
    await this.kv.set(key(token), JSON.stringify(t), 'EX', TICKET_TTL_SEC);
    return token;
  }

  /** Redeem a ticket: null when unknown, expired or already used. */
  async consume(token: string | undefined): Promise<MfaTicket | null> {
    if (typeof token !== 'string' || token.length === 0 || token.length > 128) return null;
    const raw = await this.kv.getdel(key(token));
    if (raw === null) return null;
    try {
      const t = JSON.parse(raw) as MfaTicket;
      return typeof t.userId === 'number' && (t.purpose === 'code' || t.purpose === 'enrol')
        ? t
        : null;
    } catch {
      return null;
    }
  }
}
