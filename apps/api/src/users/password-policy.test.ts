import { describe, expect, it } from 'vitest';
import { openapi } from '../common/zod.js';
import { ProblemError } from '../common/problem.js';
import { assertPasswordPolicy, newPasswordDoc, newPasswordIn, PASSWORD_MIN } from './password-policy.js';

describe('password policy', () => {
  it('rejects a new password shorter than PASSWORD_MIN with a 400 on /password (the product rule)', () => {
    let err: unknown;
    try {
      assertPasswordPolicy('x'.repeat(PASSWORD_MIN - 1), false);
    } catch (e) {
      err = e;
    }
    expect(err).toBeInstanceOf(ProblemError);
    expect((err as ProblemError).getStatus()).toBe(400);
    expect((err as ProblemError).errors).toEqual([expect.objectContaining({ pointer: '/password' })]);
    expect(() => assertPasswordPolicy('x'.repeat(PASSWORD_MIN), false)).not.toThrow();
  });

  it('VRX_DEV_WEAK_PASSWORDS waives the length rule (any non-empty password)', () => {
    expect(() => assertPasswordPolicy('a', true)).not.toThrow();
    expect(newPasswordIn.safeParse('a').success).toBe(true);
    expect(newPasswordIn.safeParse('').success).toBe(false);
  });

  it('the documented field keeps the product rule, so the OpenAPI contract is unchanged', () => {
    expect(openapi(newPasswordDoc)).toMatchObject({ type: 'string', minLength: 12, maxLength: 1024 });
  });
});
