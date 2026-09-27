import { describe, expect, it } from 'vitest';
import { mfaRequiredFor } from './mfa.service.js';

describe('F-aaa-login: who management.aaa.mfa.required covers', () => {
  it('none covers nobody', () => {
    expect(mfaRequiredFor('none', 'admin')).toBe(false);
    expect(mfaRequiredFor('none', 'operator')).toBe(false);
    expect(mfaRequiredFor('none', 'readonly')).toBe(false);
  });

  it('admins covers the admin role only', () => {
    expect(mfaRequiredFor('admins', 'admin')).toBe(true);
    expect(mfaRequiredFor('admins', 'operator')).toBe(false);
    expect(mfaRequiredFor('admins', 'readonly')).toBe(false);
  });

  it('all covers every role', () => {
    expect(mfaRequiredFor('all', 'admin')).toBe(true);
    expect(mfaRequiredFor('all', 'operator')).toBe(true);
    expect(mfaRequiredFor('all', 'readonly')).toBe(true);
  });
});
