import { describe, expect, it } from 'vitest';
import { EiKillBody, Nat64SessionsQuery } from './dto.js';
import { fakeNat64Session } from './fake.js';
import { nat64SessionJson, nptv6Json } from './service.js';

describe('nat64SessionJson', () => {
  it('maps the NatSession fields of a NAT64 row (proto.md §11 NAT session variants)', () => {
    expect(nat64SessionJson(fakeNat64Session({ vrf: 'cust', tableId: 1001 }))).toEqual({
      client: 'fd00:1::10',
      clientPort: 40000,
      poolAddress: '10.1.64.1',
      poolPort: 1024,
      remote: '10.1.2.2',
      remotePort: 80,
      remoteIpv6: 'fd00:1:64::a01:202',
      protocol: 'tcp',
      vrf: 'cust',
      tableId: 1001,
    });
  });
});

describe('nptv6Json', () => {
  it('lists the running bindings with the write-only marker, tolerating an absent or odd subtree', () => {
    expect(
      nptv6Json({
        nptv6: {
          bindings: [
            { interface: 'host-w4w0', internal: 'fd00:4:10::/48', external: 'fd00:4:20::/48' },
            'junk',
          ],
        },
      }),
    ).toEqual({
      writeOnly: true,
      bindings: [
        {
          interface: 'host-w4w0',
          internal: 'fd00:4:10::/48',
          external: 'fd00:4:20::/48',
          description: null,
        },
      ],
    });
    expect(nptv6Json(undefined)).toEqual({ writeOnly: true, bindings: [] });
    expect(nptv6Json({ nptv6: { bindings: 3 } })).toEqual({ writeOnly: true, bindings: [] });
  });
});

describe('DTOs', () => {
  it('the EI kill needs only the inside endpoint; IPv6 inside is rejected', () => {
    expect(
      EiKillBody.safeParse({ protocol: 'udp', insideAddress: '10.4.1.2', insidePort: 53 }).success,
    ).toBe(true);
    expect(
      EiKillBody.safeParse({ protocol: 'udp', insideAddress: 'fd00::1', insidePort: 53 }).success,
    ).toBe(false);
  });
  it('NAT64 sessions: page defaults, pageSize ≤ 1000, protocol only', () => {
    expect(Nat64SessionsQuery.parse({})).toEqual({ page: 1, pageSize: 100 });
    expect(Nat64SessionsQuery.safeParse({ pageSize: '1001' }).success).toBe(false);
    expect(Nat64SessionsQuery.safeParse({ protocol: 'gre!' }).success).toBe(false);
  });
});
