import { describe, expect, it } from 'vitest';
import { mapRole, mfaRequiredFor } from './aaa.service.js';

describe('F-aaa role mapping and MFA policy', () => {
  const map = [
    { group: 'cn=ro,dc=x', role: 'readonly' as const },
    { group: 'NetAdmins', role: 'admin' as const },
    { group: 'ops', role: 'operator' as const },
  ];
  it('maps to the highest privilege among the matched groups (case-insensitive)', () => {
    expect(mapRole(map, ['cn=ro,dc=x', 'netadmins'])).toBe('admin');
    expect(mapRole(map, ['ops', 'CN=RO,DC=X'])).toBe('operator');
  });
  it('an unmapped identity gets no role (login refused)', () => {
    expect(mapRole(map, ['other'])).toBeNull();
    expect(mapRole(map, [])).toBeNull();
    expect(mapRole([], ['ops'])).toBeNull();
  });
  it('mfa.required: none / admins / all', () => {
    expect(mfaRequiredFor('none', 'admin')).toBe(false);
    expect(mfaRequiredFor('admins', 'admin')).toBe(true);
    expect(mfaRequiredFor('admins', 'operator')).toBe(false);
    expect(mfaRequiredFor('all', 'readonly')).toBe(true);
  });
});
