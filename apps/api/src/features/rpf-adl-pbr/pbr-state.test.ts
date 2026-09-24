import { describe, expect, it } from 'vitest';
import { pbrState } from './pbr-state.js';

const doc = (pbr: unknown) => ({ routing: { static: [], pbr } });
const page = { page: 1, pageSize: 100 };

describe('pbrState', () => {
  const running = doc({
    policies: {
      'via-wan2': {
        acl: 'lan-b',
        priority: 10,
        paths: [{ address: '203.0.113.1', interface: 'Gi0/9/0', vrf: 'default', weight: 1 }],
      },
      lookup: { acl: 'lan-b', priority: 100, paths: [{ vrf: 'wan2', weight: 1 }] },
      waiting: { acl: 'nope', priority: 100, paths: [{ vrf: 'default', weight: 1 }] },
    },
    attachments: [
      { policy: 'via-wan2', interface: 'loop0', family: 'ipv4' },
      { policy: 'waiting', interface: 'loop0', family: 'ipv4' },
    ],
  });
  const actual = doc({
    policies: {
      'via-wan2': {
        acl: 'lan-b',
        priority: 10,
        paths: [{ address: '203.0.113.1', interface: 'Gi0/9/0', vrf: 'default', weight: 1 }],
      },
      lookup: { acl: 'lan-b', priority: 100, paths: [{ vrf: 'wan2', weight: 2 }] },
      '#10042': { acl: 'lan-b', priority: 100, paths: [{ vrf: 'default', weight: 1 }] },
    },
    attachments: [
      { policy: 'via-wan2', interface: 'loop0', family: 'ipv4' },
      { policy: '#10042', interface: 'loop1', family: 'ipv6' },
    ],
  });

  it('classifies policies and attachments against Retrieve', () => {
    const s = pbrState(running, running, actual, new Date(0), page);
    expect(s.policies.map((p) => [p.name, p.status, p.attachments])).toEqual([
      ['#10042', 'unmanaged', 0],
      ['lookup', 'drift', 0],
      ['via-wan2', 'in-sync', 1],
      ['waiting', 'missing', 1],
    ]);
    expect(s.attachments.items.map((a) => [a.policy, a.interface, a.family, a.status])).toEqual([
      ['via-wan2', 'loop0', 'ipv4', 'in-sync'],
      ['waiting', 'loop0', 'ipv4', 'missing'],
      ['#10042', 'loop1', 'ipv6', 'unmanaged'],
    ]);
    expect(s.retrievedAt).toBe('1970-01-01T00:00:00.000Z');
    expect(s.pendingChange).toBe(false);
    expect(s.counters.available).toBe(false);
  });

  it('fills defaults before comparing (priority 100, vrf default, weight 1) and pages attachments', () => {
    const short = doc({
      policies: { p: { acl: 'a', paths: [{ address: '192.0.2.1' }] } },
      attachments: [],
    });
    const full = doc({
      policies: {
        p: {
          acl: 'a',
          priority: 100,
          paths: [{ address: '192.0.2.1', vrf: 'default', weight: 1 }],
        },
      },
    });
    expect(pbrState(short, short, full, undefined, page).policies[0]?.status).toBe('in-sync');
    const many = doc({
      policies: {},
      attachments: Array.from({ length: 5 }, (_, i) => ({
        policy: 'p',
        interface: `loop${i}`,
        family: 'ipv4',
      })),
    });
    const s = pbrState(many, doc(undefined), doc(undefined), undefined, { page: 2, pageSize: 2 });
    expect(s.attachments.total).toBe(5);
    expect(s.attachments.items.map((a) => a.interface)).toEqual(['loop2', 'loop3']);
    expect(s.pendingChange).toBe(true);
  });

  it('is empty without routing.pbr', () => {
    const s = pbrState({}, {}, {}, undefined, page);
    expect(s.policies).toEqual([]);
    expect(s.attachments.total).toBe(0);
  });
});
