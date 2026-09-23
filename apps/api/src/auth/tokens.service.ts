import { Inject, Injectable, Logger } from '@nestjs/common';
import { jwtVerify, SignJWT } from 'jose';
import { createHash, randomBytes } from 'node:crypto';
import { ENV, type Env } from '../config.js';
import { ROLES, type Role } from '../db/schema.js';
import { VALKEY, type Valkey } from '../infra/valkey.js';

const ISSUER = 'vrx-api';
const AUDIENCE = 'vrx';

export interface AccessClaims {
  id: number;
  username: string;
  role: Role;
}

export interface IssuedRefresh {
  token: string;
  family: string;
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

  constructor(
    @Inject(ENV) private readonly env: Env,
    @Inject(VALKEY) private readonly kv: Valkey,
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
    return new SignJWT({ username: c.username, role: c.role, typ: 'access' })
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
      return { id, username, role: role as Role };
    } catch {
      return null;
    }
  }

  /** New refresh token; `family` continues a rotation chain (omit to start one at login). */
  async issueRefresh(userId: number, family = b64url(12)): Promise<IssuedRefresh> {
    const token = `${family}.${b64url(32)}`;
    const ttl = this.refreshTtl;
    await this.kv
      .multi()
      .set(`rt:${sha256(token)}`, JSON.stringify({ uid: userId, fam: family }), 'EX', ttl)
      .set(`rtfam:${family}`, String(userId), 'EX', ttl)
      .exec();
    return { token, family };
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

export function apiKeyHash(token: string): string {
  return sha256(token);
}

export function newApiKeyToken(): string {
  return `vrxk_${b64url(32)}`;
}
