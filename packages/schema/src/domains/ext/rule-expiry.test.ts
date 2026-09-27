import { describe, expect, it } from 'vitest';
import { AclRuleSchema } from '../acl.js';
import { ruleExpired } from './rule-expiry.js';

describe('F-rule-expiry schema fields', () => {
  const base = { sequence: 10, action: 'permit' };
  it('accepts RFC 3339 with an offset, owner and ticket', () => {
    const r = AclRuleSchema.parse({ ...base, expiresAt: '2026-10-02T18:00:00+03:30', owner: 'netops', ticket: 'CHG-1234' });
    expect(r.expiresAt).toBe('2026-10-02T18:00:00+03:30');
  });
  it('refuses a date without an offset, and control or bidi characters in owner/ticket (D-049)', () => {
    expect(AclRuleSchema.safeParse({ ...base, expiresAt: '2026-10-02 18:00' }).success).toBe(false);
    expect(AclRuleSchema.safeParse({ ...base, expiresAt: '2026-10-02T18:00:00' }).success).toBe(false);
    expect(AclRuleSchema.safeParse({ ...base, owner: 'a\u0007b' }).success).toBe(false);
    expect(AclRuleSchema.safeParse({ ...base, ticket: 'CHG‮1' }).success).toBe(false);
    expect(AclRuleSchema.safeParse({ ...base, owner: 'x'.repeat(65) }).success).toBe(false);
  });
  it('ruleExpired: at or before now', () => {
    const now = new Date('2026-09-27T12:00:00Z');
    expect(ruleExpired('2026-09-27T12:00:00Z', now)).toBe(true);
    expect(ruleExpired('2026-09-27T15:30:01+03:30', now)).toBe(false);
    expect(ruleExpired(undefined, now)).toBe(false);
  });
});
