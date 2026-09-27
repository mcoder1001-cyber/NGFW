import { createHash } from 'node:crypto';
import { Inject, Injectable } from '@nestjs/common';
import { and, eq, isNull } from 'drizzle-orm';
import { problems } from '../../common/problem.js';
import { DB, type Db } from '../../db/db.js';
import { appUser, userMfaRecovery, type Role } from '../../db/schema.js';
import { SecretsService } from '../../secrets/secrets.service.js';
import { generateSecret, otpauthUri, recoveryCodes, verifyTotp } from './totp.js';

/** How many recovery codes an enrolment gets. */
const RECOVERY_CODES = 10;

/** Binds an MFA seed ciphertext to one user (AES-GCM AAD), so a blob cannot be moved between accounts. */
const seedRef = (userId: number) => `mfa/${userId}`;

/** The codes carry 80 bits, so a fast hash is enough (the same reasoning as for API keys). */
const codeHash = (code: string) =>
  createHash('sha256').update(code.trim().toLowerCase()).digest('hex');

export interface MfaStatus {
  /** A verified factor exists. */
  enrolled: boolean;
  /** `management.aaa.mfa.required` covers this user's role. */
  required: boolean;
  /** A seed was handed out but never proved with a code. */
  pending: boolean;
  /** Unused recovery codes left. */
  recoveryCodesLeft: number;
}

/** Whether the MFA policy covers `role`. */
export function mfaRequiredFor(required: 'none' | 'admins' | 'all', role: Role): boolean {
  return required === 'all' || (required === 'admins' && role === 'admin');
}

/**
 * F-aaa-login: per-user TOTP enrolment and second-factor verification.
 *
 * The seed is generated here, handed to the user once (as a base32 string and an `otpauth://` URI) and stored only as
 * AES-256-GCM ciphertext under the secret store's master key. It becomes ACTIVE only when the user proves it with a
 * code — so a started-and-abandoned enrolment can never lock anybody out. Confirming an enrolment also mints the
 * recovery codes, returned once and kept only as hashes.
 *
 * Local and external (shadow) accounts are the same `app_user` rows, so an identity that RADIUS or a directory
 * authenticates gets MFA from this box exactly as a local user does.
 */
@Injectable()
export class MfaService {
  constructor(
    @Inject(DB) private readonly db: Db,
    private readonly secrets: SecretsService,
  ) {}

  async status(
    userId: number,
    role: Role,
    required: 'none' | 'admins' | 'all',
  ): Promise<MfaStatus> {
    const [u] = await this.db
      .select({ secret: appUser.mfaSecret, pending: appUser.mfaPendingSecret })
      .from(appUser)
      .where(eq(appUser.id, userId));
    const left = await this.db
      .select({ id: userMfaRecovery.id })
      .from(userMfaRecovery)
      .where(and(eq(userMfaRecovery.userId, userId), isNull(userMfaRecovery.usedAt)));
    return {
      enrolled: u?.secret != null,
      required: mfaRequiredFor(required, role),
      pending: u?.pending != null,
      recoveryCodesLeft: left.length,
    };
  }

  async isEnrolled(userId: number): Promise<boolean> {
    const [u] = await this.db
      .select({ secret: appUser.mfaSecret })
      .from(appUser)
      .where(eq(appUser.id, userId));
    return u?.secret != null;
  }

  /**
   * Start (or restart) enrolment: a fresh seed, stored as `mfa_pending_secret`. Restarting replaces a pending seed;
   * an ACTIVE factor is never replaced silently — the caller has to reset it first, so a stolen session cannot
   * quietly swap the second factor for its own.
   */
  async begin(
    userId: number,
    username: string,
    issuer: string,
  ): Promise<{ secret: string; otpauthUri: string }> {
    if (await this.isEnrolled(userId)) {
      throw problems.conflict(
        'mfa-already-enrolled',
        'this account already has a verified second factor; reset it before enrolling again',
      );
    }
    const secret = generateSecret();
    await this.db
      .update(appUser)
      .set({ mfaPendingSecret: this.secrets.encrypt(secret, seedRef(userId)) })
      .where(eq(appUser.id, userId));
    return { secret, otpauthUri: otpauthUri(secret, username, issuer) };
  }

  /**
   * Confirm the pending enrolment with a code from the authenticator. Returns the recovery codes — the only time they
   * are ever readable. Re-confirming replaces the whole set, so old codes stop working.
   */
  async confirm(userId: number, code: string): Promise<string[]> {
    const [u] = await this.db
      .select({ pending: appUser.mfaPendingSecret })
      .from(appUser)
      .where(eq(appUser.id, userId));
    if (u?.pending == null) {
      throw problems.badRequest('no enrolment is in progress for this account; start one first');
    }
    if (!verifyTotp(this.secrets.decrypt(u.pending, seedRef(userId)), code)) {
      throw problems.forbidden(
        'that code does not match the enrolment; check the authenticator clock',
      );
    }
    const codes = recoveryCodes(RECOVERY_CODES);
    const pending = u.pending;
    await this.db.transaction(async (tx) => {
      await tx
        .update(appUser)
        .set({ mfaSecret: pending, mfaPendingSecret: null, mfaEnrolledAt: new Date() })
        .where(eq(appUser.id, userId));
      await tx.delete(userMfaRecovery).where(eq(userMfaRecovery.userId, userId));
      await tx
        .insert(userMfaRecovery)
        .values(codes.map((c) => ({ userId, codeHash: codeHash(c) })));
    });
    return codes;
  }

  /**
   * Verify a second factor: the current TOTP code, or one unused recovery code — which is spent in the same statement
   * that finds it, so two parallel logins cannot both redeem it.
   */
  async verify(userId: number, code: string): Promise<boolean> {
    const [u] = await this.db
      .select({ secret: appUser.mfaSecret })
      .from(appUser)
      .where(eq(appUser.id, userId));
    if (u?.secret == null) return false;
    if (verifyTotp(this.secrets.decrypt(u.secret, seedRef(userId)), code)) return true;
    const spent = await this.db
      .update(userMfaRecovery)
      .set({ usedAt: new Date() })
      .where(
        and(
          eq(userMfaRecovery.userId, userId),
          eq(userMfaRecovery.codeHash, codeHash(code)),
          isNull(userMfaRecovery.usedAt),
        ),
      )
      .returning({ id: userMfaRecovery.id });
    return spent.length > 0;
  }

  /**
   * Admin reset by name: clears the factors of `username` so the user can enrol again (a lost authenticator with no
   * recovery code left). Returns false when there is no such user — the caller turns that into a 404.
   */
  async resetFor(username: string): Promise<boolean> {
    const [u] = await this.db
      .select({ id: appUser.id })
      .from(appUser)
      .where(eq(appUser.username, username));
    if (u === undefined) return false;
    await this.reset(u.id);
    return true;
  }

  /** Clear every factor of a user (an admin reset, or the user turning MFA off). */
  async reset(userId: number): Promise<void> {
    await this.db.transaction(async (tx) => {
      await tx
        .update(appUser)
        .set({ mfaSecret: null, mfaPendingSecret: null, mfaEnrolledAt: null })
        .where(eq(appUser.id, userId));
      await tx.delete(userMfaRecovery).where(eq(userMfaRecovery.userId, userId));
    });
  }
}
