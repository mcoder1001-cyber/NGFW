import { describe, expect, it } from 'vitest';
import { ProblemError } from '../../common/problem.js';
import type { Principal } from '../../common/principal.js';
import { assertMfaRaiseSafe, mfaRaised, MFA_REQUIRED_POINTER } from './mfa-guard.js';

const doc = (required?: string) =>
  required === undefined ? {} : { management: { aaa: { mfa: { required } } } };
const admin: Principal = { id: 1, username: 'admin', role: 'admin', via: 'jwt', sid: 's1' };
const deps = (session: boolean, enrolled: boolean) => ({
  sessionPassedMfa: async () => session,
  anyAdminEnrolled: async () => enrolled,
});

describe('F-aaa-mfa-lockout: raising mfa.required', () => {
  it('detects a raise only (none < admins < all; absent = none)', () => {
    expect(mfaRaised(doc(), doc('admins'))).toEqual({ from: 'none', to: 'admins' });
    expect(mfaRaised(doc('admins'), doc('all'))).toEqual({ from: 'admins', to: 'all' });
    expect(mfaRaised(doc('all'), doc('admins'))).toBeNull();
    expect(mfaRaised(doc('admins'), doc('none'))).toBeNull();
    expect(mfaRaised(doc('admins'), doc('admins'))).toBeNull();
    expect(mfaRaised(doc('none'), doc())).toBeNull();
  });

  it('refuses a raise without an enrolled admin or without an MFA session (400, pointer, remedy)', async () => {
    for (const [s, e] of [
      [false, false],
      [true, false],
      [false, true],
    ] as const) {
      const err = await assertMfaRaiseSafe(deps(s, e), admin, doc('none'), doc('admins')).catch(
        (x: unknown) => x,
      );
      expect(err).toBeInstanceOf(ProblemError);
      const p = err as ProblemError;
      expect(p.getStatus()).toBe(400);
      expect(p.errors?.[0]?.pointer).toBe(MFA_REQUIRED_POINTER);
      expect(p.detail).toMatch(/Enrol an admin first/);
    }
  });

  it('allows the raise once an admin is enrolled and the session passed MFA', async () => {
    await expect(
      assertMfaRaiseSafe(deps(true, true), admin, doc('none'), doc('all')),
    ).resolves.toBeUndefined();
  });

  it('never blocks lowering or an unchanged policy', async () => {
    await expect(
      assertMfaRaiseSafe(deps(false, false), admin, doc('all'), doc('none')),
    ).resolves.toBeUndefined();
    await expect(
      assertMfaRaiseSafe(deps(false, false), admin, doc('admins'), doc('admins')),
    ).resolves.toBeUndefined();
  });
});
