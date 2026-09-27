import { Inject, Injectable } from '@nestjs/common';
import { ModuleRef } from '@nestjs/core';
import { and, eq, isNull, lt, sql } from 'drizzle-orm';
import { hashPassword, verifyPassword } from '../../auth/password.js';
import { DB, type Db } from '../../db/db.js';
import { aaaMfa, aaaMfaRecovery, apiKey, appUser } from '../../db/schema.js';
import { generateSecret, otpauthUri, recoveryCodes, totpCode } from './totp.js';

const STEP_MS = 30_000;

/** The AAD every seed ciphertext is bound to (a row's ciphertext cannot be moved to another user). */
const aad = (userId: number) => `mfa/${userId}`;

export interface Enrolment {
  /** base32 TOTP secret — shown ONCE (the response of the enrol call), never stored in clear, never logged. */
  secret: string;
  otpauthUri: string;
}

/**
 * F-aaa-login: per-user TOTP (RFC 6238) second factor. The seed is AES-256-GCM encrypted with the secret-store master
 * key (SecretsService.encrypt, AAD `mfa/<user id>`); recovery codes are argon2id hashes, each usable once. A code is
 * accepted at most once: `last_step` only moves forward, in ONE conditional UPDATE (`… where last_step < step`), so
 * a replay — sequential or concurrent — of the same (or an older) step is refused.
 */
@Injectable()
export class MfaService {
  constructor(
    @Inject(DB) private readonly db: Db,
    private readonly moduleRef: ModuleRef,
  ) {}

  private async secrets() {
    const { SecretsService } = await import('../../secrets/secrets.service.js');
    return this.moduleRef.get(SecretsService, { strict: false });
  }

  /** Enrolled = an ENABLED factor (a pending enrolment does not count). */
  async enrolled(userId: number): Promise<boolean> {
    const [r] = await this.db
      .select({ enabled: aaaMfa.enabled })
      .from(aaaMfa)
      .where(eq(aaaMfa.userId, userId));
    return r?.enabled === true;
  }

  /**
   * F-aaa-mfa-lockout: at least one enabled admin account has an enabled (active) factor — the precondition for
   * raising `management.aaa.mfa.required` over the admin role without locking every admin out.
   */
  async anyAdminEnrolled(): Promise<boolean> {
    const [r] = await this.db
      .select({ n: sql<number>`count(*)::int` })
      .from(aaaMfa)
      .innerJoin(appUser, eq(appUser.id, aaaMfa.userId))
      .where(and(eq(aaaMfa.enabled, true), eq(appUser.role, 'admin'), eq(appUser.disabled, false)));
    return (r?.n ?? 0) > 0;
  }

  async status(userId: number): Promise<{ enrolled: boolean; recoveryCodesLeft: number }> {
    const enrolled = await this.enrolled(userId);
    const [c] = await this.db
      .select({ n: sql<number>`count(*)::int` })
      .from(aaaMfaRecovery)
      .where(and(eq(aaaMfaRecovery.userId, userId), isNull(aaaMfaRecovery.usedAt)));
    return { enrolled, recoveryCodesLeft: enrolled ? (c?.n ?? 0) : 0 };
  }

  /**
   * Start (or restart) an enrolment: a new seed, stored encrypted and NOT enabled. Refused (null) when a factor is
   * already enabled — replacing it needs an admin reset first, so a stolen session cannot swap the second factor.
   */
  async begin(userId: number, account: string, issuer: string): Promise<Enrolment | null> {
    if (await this.enrolled(userId)) return null;
    const seed = generateSecret();
    const blob = (await this.secrets()).encrypt(seed, aad(userId));
    await this.db
      .insert(aaaMfa)
      .values({ userId, seed: blob, enabled: false })
      .onConflictDoUpdate({
        target: aaaMfa.userId,
        set: { seed: blob, enabled: false, lastStep: 0, createdAt: new Date(), enabledAt: null },
        where: eq(aaaMfa.enabled, false),
      });
    return { secret: seed, otpauthUri: otpauthUri(seed, account, issuer) };
  }

  /**
   * Accept a TOTP `code` for the user's factor — the enabled one, or (with `pending`) the enrolment in progress.
   * Each time step is accepted once (replay guard).
   */
  async verifyCode(
    userId: number,
    code: string,
    pending = false,
    atMs = Date.now(),
  ): Promise<boolean> {
    const trimmed = code.trim();
    if (!/^[0-9]{6}$/.test(trimmed)) return false;
    const [row] = await this.db.select().from(aaaMfa).where(eq(aaaMfa.userId, userId));
    if (row === undefined || row.enabled === pending) return false;
    const seed = (await this.secrets()).decrypt(row.seed, aad(userId));
    const now = Math.floor(atMs / STEP_MS);
    for (const step of [now - 1, now, now + 1]) {
      if (step <= row.lastStep) continue;
      if (totpCode(seed, step * STEP_MS, STEP_MS) !== trimmed) continue;
      const moved = await this.db
        .update(aaaMfa)
        .set({ lastStep: step })
        .where(and(eq(aaaMfa.userId, userId), lt(aaaMfa.lastStep, step)))
        .returning({ id: aaaMfa.userId });
      return moved.length === 1;
    }
    return false;
  }

  /** Enable the pending factor and mint the recovery codes (returned once; only their hashes are stored). */
  async activate(userId: number): Promise<string[]> {
    const codes = recoveryCodes(10);
    const hashes = await Promise.all(codes.map((c) => hashPassword(c)));
    await this.db.transaction(async (tx) => {
      await tx
        .update(aaaMfa)
        .set({ enabled: true, enabledAt: new Date() })
        .where(eq(aaaMfa.userId, userId));
      await tx.delete(aaaMfaRecovery).where(eq(aaaMfaRecovery.userId, userId));
      await tx.insert(aaaMfaRecovery).values(hashes.map((hash) => ({ userId, hash })));
    });
    return codes;
  }

  /** Spend a recovery code: single use (the UPDATE only succeeds while `used_at` is null). */
  async useRecovery(userId: number, code: string): Promise<boolean> {
    const c = code.trim().toLowerCase();
    if (!/^[0-9a-f]{10}$/.test(c)) return false;
    if (!(await this.enrolled(userId))) return false;
    const rows = await this.db
      .select({ id: aaaMfaRecovery.id, hash: aaaMfaRecovery.hash })
      .from(aaaMfaRecovery)
      .where(and(eq(aaaMfaRecovery.userId, userId), isNull(aaaMfaRecovery.usedAt)));
    for (const r of rows) {
      if (!(await verifyPassword(r.hash, c))) continue;
      const spent = await this.db
        .update(aaaMfaRecovery)
        .set({ usedAt: new Date() })
        .where(and(eq(aaaMfaRecovery.id, r.id), isNull(aaaMfaRecovery.usedAt)))
        .returning({ id: aaaMfaRecovery.id });
      return spent.length === 1;
    }
    return false;
  }

  /** Admin reset (lost device): the factor and its recovery codes are deleted. Returns whether one existed. */
  /**
   * Deletes the user's factor and recovery codes. S-aaa-key-reset: in the same transaction every API key of the user
   * loses `mfa_verified` (the factor it was minted under no longer exists), so under an MFA policy those keys are
   * refused (401 mfa-required) until the user re-enrols and mints new ones. Returns the number of keys cleared, or
   * null when the user had no factor (nothing changed).
   */
  async reset(userId: number): Promise<number | null> {
    return this.db.transaction(async (tx) => {
      await tx.delete(aaaMfaRecovery).where(eq(aaaMfaRecovery.userId, userId));
      const gone = await tx
        .delete(aaaMfa)
        .where(eq(aaaMfa.userId, userId))
        .returning({ id: aaaMfa.userId });
      if (gone.length === 0) return null;
      const keys = await tx
        .update(apiKey)
        .set({ mfaVerified: false })
        .where(and(eq(apiKey.userId, userId), eq(apiKey.mfaVerified, true)))
        .returning({ id: apiKey.id });
      return keys.length;
    });
  }
}
