import { describe, expect, it } from 'vitest';
import { expiringRules, expiryInPastIssues, expiryStates } from './expiry.js';

const now = new Date('2026-09-27T12:00:00Z');
const doc = (aclExp?: string, hostExp?: string, natExp?: string) => ({
  acl: {
    lists: {
      'web/in': {
        rules: [
          {
            sequence: 10,
            action: 'permit',
            ...(aclExp ? { expiresAt: aclExp, owner: 'netops' } : {}),
          },
        ],
      },
    },
    host: {
      mgmt: {
        rules: [
          {
            sequence: 5,
            action: 'accept',
            ...(hostExp ? { expiresAt: hostExp, ticket: 'CHG-9' } : {}),
          },
        ],
      },
    },
  },
  nat: { staticMappings: [{ name: 'rdp', ...(natExp ? { expiresAt: natExp } : {}) }] },
});

describe('F-rule-expiry: expiring rules of a document', () => {
  it('finds ACL, host and NAT rules by stable identity, with escaped pointers', () => {
    expect(
      expiringRules(doc('2026-10-01T00:00:00Z', '2026-10-02T00:00:00Z', '2026-10-03T00:00:00Z')),
    ).toEqual([
      {
        kind: 'acl',
        id: 'acl/web/in/10',
        pointer: '/acl/lists/web~1in/rules/0',
        expiresAt: '2026-10-01T00:00:00Z',
        owner: 'netops',
      },
      {
        kind: 'host',
        id: 'host/mgmt/5',
        pointer: '/acl/host/mgmt/rules/0',
        expiresAt: '2026-10-02T00:00:00Z',
        ticket: 'CHG-9',
      },
      {
        kind: 'nat',
        id: 'nat/rdp',
        pointer: '/nat/staticMappings/0',
        expiresAt: '2026-10-03T00:00:00Z',
      },
    ]);
    expect(expiringRules({})).toEqual([]);
  });

  it('refuses a past expiry on a new rule or a changed date, keeps an unchanged expired rule', () => {
    const past = '2026-09-27T11:00:00Z';
    // new rule, already expired
    expect(expiryInPastIssues(doc(past), doc(), now).map((i) => [i.pointer, i.rule])).toEqual([
      ['/acl/lists/web~1in/rules/0/expiresAt', 'rule.expires-in-past'],
    ]);
    // the same expired rule, unchanged: fine
    expect(expiryInPastIssues(doc(past), doc(past), now)).toEqual([]);
    // re-dated to another past time: refused
    expect(expiryInPastIssues(doc('2026-09-27T10:00:00Z'), doc(past), now)).toHaveLength(1);
    // extended into the future: fine
    expect(expiryInPastIssues(doc('2026-12-31T00:00:00Z'), doc(past), now)).toEqual([]);
    // exactly now counts as past
    expect(
      expiryInPastIssues(doc(undefined, undefined, now.toISOString()), doc(), now),
    ).toHaveLength(1);
  });

  it('splits expiring (within the window) from expired', () => {
    const s = expiryStates(
      doc('2026-09-29T00:00:00Z', '2026-09-27T11:59:00Z', '2026-12-01T00:00:00Z'),
      now,
      3,
    );
    expect(s.expiring.map((r) => r.id)).toEqual(['acl/web/in/10']);
    expect(s.expired.map((r) => r.id)).toEqual(['host/mgmt/5']);
  });
});
