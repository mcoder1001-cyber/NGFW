import { z } from 'zod';
import { problems } from '../common/problem.js';

/** Minimum length of a new password (P06; documented as `minLength` in the OpenAPI contract). */
export const PASSWORD_MIN = 1;

/**
 * Product passwords accept 1–1024 characters. The legacy development flag remains
 * compatible; non-empty short passwords are also accepted without it.
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
export function assertPasswordPolicy(password: string, _devWeakPasswords: boolean): void {
  if (password.length >= PASSWORD_MIN && password.length <= 1024) return;
  throw problems.badRequest('invalid request body', [
    { pointer: '/password', message: `must contain ${PASSWORD_MIN}–1024 characters` },
  ]);
}
