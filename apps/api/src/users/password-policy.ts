import { z } from 'zod';
import { problems } from '../common/problem.js';

/** Minimum length of a new password (P06; documented as `minLength` in the OpenAPI contract). */
export const PASSWORD_MIN = 12;

/**
 * The new-password field. `weak = false` is the product rule and what the OpenAPI documents; `weak = true`
 * (VRX_DEV_WEAK_PASSWORDS, development only) accepts any non-empty value. The route pipes build it from the environment
 * (EnvZodPipe), so with the flag off a request is parsed exactly as before and every issue is reported together.
 */
export function newPassword(weak = false) {
  return z
    .string()
    .min(weak ? 1 : PASSWORD_MIN)
    .max(1024);
}

/**
 * Defence in depth for any caller of UsersService.setPassword that did not come through a route pipe: 400 with pointer
 * `/password` unless the password is long enough or dev mode is on.
 */
export function assertPasswordPolicy(password: string, devWeakPasswords: boolean): void {
  if (devWeakPasswords || password.length >= PASSWORD_MIN) return;
  throw problems.badRequest('invalid request body', [
    { pointer: '/password', message: `must be at least ${PASSWORD_MIN} characters` },
  ]);
}
