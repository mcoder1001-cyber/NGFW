import { describe, expect, it } from 'vitest';
import { RootConfig as RootSchema } from '../index.js';
import { raVpnValidators } from './ra-vpn.js';
import { validateSemantics } from './index.js';

const profile = (enabled: boolean) => ({
  enabled,
  localAddr: '192.0.2.1',
  vrf: 'default',
  underlayVrf: 'default',
  certificate: 'server',
  proposal: 'native',
  pools: [{ name: 'clients', prefix: '10.19.200.0/24' }],
  users: [{ username: 'client', passwordRef: 'password/client' }],
});

describe('native remote-access capability gate', () => {
  it('rejects enabled profiles with exact escaped pointers without exposing credentials', () => {
    const config = RootSchema.parse({ vpn: { remoteAccess: { roadwarrior: profile(true) } } });
    expect(raVpnValidators[0]!.validate(config)).toEqual([
      {
        pointer: '/vpn/remoteAccess/roadwarrior/enabled',
        message: expect.stringContaining('unavailable'),
      },
    ]);
    expect(JSON.stringify(raVpnValidators[0]!.validate(config))).not.toContain('password/client');
    expect(
      validateSemantics(config).some(
        (issue) => issue.pointer === '/vpn/remoteAccess/roadwarrior/enabled',
      ),
    ).toBe(true);
  });
  it('keeps disabled drafts parseable and does not reject unrelated VPN configurations', () => {
    const config = RootSchema.parse({ vpn: { remoteAccess: { draft: profile(false) } } });
    expect(raVpnValidators[0]!.validate(config)).toEqual([]);
    expect(raVpnValidators[0]!.validate(RootSchema.parse({}))).toEqual([]);
  });
});

import { raVpnTransportValidator } from './ra-vpn.js';
const transport = () => ({
  outer: { vpp: '198.18.19.0/31', namespace: '198.18.19.1/31' },
  inner: { vpp: '198.18.19.2/31', namespace: '198.18.19.3/31' },
});
const routed = (changes: Record<string, unknown> = {}) => ({
  ...profile(false),
  transport: transport(),
  ...changes,
});
const checkTransport = (profiles: Record<string, unknown>, extra = {}) =>
  raVpnTransportValidator.validate(RootSchema.parse({ ...extra, vpn: { remoteAccess: profiles } }));
describe('independent remote-access explicit transit contracts', () => {
  it('accepts a disabled routed draft with explicit disjoint point-to-point links', () => {
    expect(checkTransport({ roadwarrior: routed() })).toEqual([]);
  });
  it('requires explicit transport for enabled profiles while retaining disabled drafts', () => {
    expect(checkTransport({ roadwarrior: profile(true) })).toEqual([
      {
        pointer: '/vpn/remoteAccess/roadwarrior/transport',
        message: expect.stringContaining('explicit'),
      },
    ]);
    expect(checkTransport({ draft: profile(false) })).toEqual([]);
  });
  it('rejects identical sides, mismatched subnets and unexpected families', () => {
    for (const outer of [
      { vpp: '198.18.19.0/31', namespace: '198.18.19.0/31' },
      { vpp: '198.18.19.0/31', namespace: '198.18.19.3/31' },
      { vpp: 'fd19::/127', namespace: 'fd19::1/127' },
    ])
      expect(
        checkTransport({ roadwarrior: routed({ transport: { ...transport(), outer } }) }),
      ).toEqual(
        expect.arrayContaining([
          {
            pointer: '/vpn/remoteAccess/roadwarrior/transport/outer',
            message: expect.stringContaining('distinct'),
          },
        ]),
      );
  });
  it('rejects shared transit ownership and duplicate dedicated public endpoints', () => {
    const issues = checkTransport({ first: routed(), second: routed() });
    expect(issues.some((issue) => issue.pointer === '/vpn/remoteAccess/second/localAddr')).toBe(
      true,
    );
    expect(
      issues.some((issue) => issue.pointer === '/vpn/remoteAccess/second/transport/inner'),
    ).toBe(true);
  });
  it('rejects transit/client pool overlap and requires IPv6 inner addressing', () => {
    expect(
      checkTransport({
        roadwarrior: routed({ pools: [{ name: 'clients', prefix: '198.18.19.0/24' }] }),
      }).some((issue) => issue.pointer.endsWith('/transport/outer')),
    ).toBe(true);
    expect(
      checkTransport({
        roadwarrior: routed({ pools: [{ name: 'clients', prefix: 'fd19::/64' }] }),
      }).some((issue) => issue.pointer.endsWith('/transport/innerIpv6')),
    ).toBe(true);
    expect(
      checkTransport({
        roadwarrior: routed({
          pools: [{ name: 'clients', prefix: 'fd19::/64' }],
          transport: { ...transport(), innerIpv6: { vpp: 'fd20::/127', namespace: 'fd20::1/127' } },
        }),
      }),
    ).toEqual([]);
  });
});
