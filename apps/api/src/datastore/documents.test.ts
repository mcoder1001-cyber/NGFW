import { describe, expect, it } from 'vitest';
import { TEST_HASH } from '../testing/fixtures.js';
import { privilegedChanges } from './documents.js';

describe('privilegedChanges (review M1)', () => {
  it('flags users, AAA and every changed secret reference', () => {
    const a = {
      management: { users: [{ username: 'admin', role: 'admin' }], aaa: { order: ['local'] } },
      vpn: { ipsec: { tunnels: { t1: { presharedKeyRef: 'psk/one' } } } },
      routing: { bgp: { neighbors: { '10.0.0.2': { passwordRef: 'password/bgp' } } } },
    };
    expect(privilegedChanges(a, structuredClone(a))).toEqual([]);
    const b = structuredClone(a);
    b.vpn.ipsec.tunnels.t1.presharedKeyRef = 'psk/two';
    expect(privilegedChanges(a, b)).toEqual(['/vpn/ipsec/tunnels/t1/presharedKeyRef']);
    const c = structuredClone(a) as typeof a & {
      routing: { bgp: { neighbors: Record<string, object> } };
    };
    delete (c.routing.bgp.neighbors as Record<string, unknown>)['10.0.0.2'];
    expect(privilegedChanges(a, c)).toEqual(['/routing/bgp/neighbors/10.0.0.2/passwordRef']);
    const d = structuredClone(a);
    (d.management.users[0] as Record<string, unknown>)['passwordHash'] = TEST_HASH;
    d.management.aaa.order = ['local', 'radius'];
    expect(privilegedChanges(a, d)).toEqual(['/management/users', '/management/aaa']);
  });

  it('non-secret edits are not privileged', () => {
    const a = { system: { hostname: 'a' }, interfaces: { loop1: { ipv4: ['10.0.0.1/24'] } } };
    expect(privilegedChanges(a, { ...a, system: { hostname: 'b' } })).toEqual([]);
  });
});
