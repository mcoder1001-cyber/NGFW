import { describe, expect, it } from 'vitest';
import { authSequence, isAuthMethod, stepEligible } from './login-order.js';

describe('F-aaa-login: authentication order', () => {
  it('the default order is local only, with no fallback step', () => {
    expect(authSequence(['local'], true)).toEqual({ steps: ['local'], fallbackAt: -1 });
    expect(authSequence(['local'], false)).toEqual({ steps: ['local'], fallbackAt: -1 });
  });

  it('keeps the configured order', () => {
    expect(authSequence(['radius', 'local', 'ldap'], false).steps).toEqual([
      'radius',
      'local',
      'ldap',
    ]);
  });

  it('appends a local fallback only when the order has no local', () => {
    const withFallback = authSequence(['radius'], true);
    expect(withFallback.steps).toEqual(['radius', 'local']);
    expect(withFallback.fallbackAt).toBe(1);
    // local is already in the order → tried at its position, no extra step
    expect(authSequence(['radius', 'local'], true)).toEqual({
      steps: ['radius', 'local'],
      fallbackAt: -1,
    });
    // fallback off → external only, a local user cannot get in
    expect(authSequence(['radius'], false)).toEqual({ steps: ['radius'], fallbackAt: -1 });
  });

  it('the fallback step runs only after an unreachable method', () => {
    const seq = authSequence(['radius'], true);
    expect(stepEligible(seq, 0, false)).toBe(true);
    // radius answered (accept/reject/absent) → the fallback must not run
    expect(stepEligible(seq, 1, false)).toBe(false);
    // radius did not answer at all → the fallback runs
    expect(stepEligible(seq, 1, true)).toBe(true);
  });

  it('a local step inside the order is never gated on unreachability', () => {
    const seq = authSequence(['radius', 'local'], true);
    expect(stepEligible(seq, 1, false)).toBe(true);
  });

  it('drops duplicates and unknown methods, and never yields an empty sequence', () => {
    expect(authSequence(['radius', 'radius', 'local'], false).steps).toEqual(['radius', 'local']);
    expect(authSequence(['oidc', 'saml'], false).steps).toEqual(['local']);
    expect(authSequence([], false).steps).toEqual(['local']);
    // an unknown-only order with the fallback on still ends up local, and not as a gated step
    expect(authSequence(['oidc'], true)).toEqual({ steps: ['local'], fallbackAt: -1 });
  });

  it('isAuthMethod only accepts the methods this build implements', () => {
    expect(isAuthMethod('local')).toBe(true);
    expect(isAuthMethod('radius')).toBe(true);
    expect(isAuthMethod('ldap')).toBe(true);
    expect(isAuthMethod('tacacs')).toBe(true);
    expect(isAuthMethod('oidc')).toBe(false);
    expect(isAuthMethod('')).toBe(false);
  });
});
