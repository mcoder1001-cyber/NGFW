import { z } from 'zod';
import { problems } from '../common/problem.js';

/** Minimum length of a new password (P06; documented as `minLength` in the OpenAPI contract). */
export const PASSWORD_MIN = 12;

/** The new-password field as documented: the product rule. */
export const newPasswordDoc = z.string().min(PASSWORD_MIN).max(1024);
/**
 * The new-password field as parsed: any non-empty value. The length rule is enforced by `assertPasswordPolicy`, the one
 * place that can waive it (VRX_DEV_WEAK_PASSWORDS), so the contract keeps documenting the product rule.
 */
export const newPasswordIn = z.string().min(1).max(1024);

/** 400 with pointer `/password` (as the schema check it replaces) unless the password is long enough or dev mode is on. */
export function assertPasswordPolicy(password: string, devWeakPasswords: boolean): void {
  if (devWeakPasswords || password.length >= PASSWORD_MIN) return;
  throw problems.badRequest('invalid request body', [
    { pointer: '/password', message: `must be at least ${PASSWORD_MIN} characters` },
  ]);
}
