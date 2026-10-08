import { describe, expect, it } from 'vitest';
import { RootConfig, type RootConfigInput } from '../index.js';
import { pppoeValidators } from './pppoe.js';

const run = (doc: RootConfigInput) => pppoeValidators[0]!.validate(RootConfig.parse(doc));

const wan = (over: Record<string, unknown> = {}, pppoe: Record<string, unknown> = {}) => ({
  enabled: true,
  pppoe: { username: 'u', passwordRef: 'password/isp', ...pppoe },
  ...over,
});

describe('F-pppoe-client semantic rules', () => {
  it('accepts a valid PPPoE client (no static address, parent implicit)', () => {
    expect(run({ interfaces: { wan0: wan() } })).toEqual([]);
  });

  it('rejects static addresses on a PPPoE client', () => {
    expect(run({ interfaces: { wan0: wan({ ipv4: ['192.0.2.1/24'] }) } })).toEqual([
      expect.objectContaining({ pointer: '/interfaces/wan0/ipv4' }),
    ]);
  });

  it('rejects an unknown dial-over parent', () => {
    expect(run({ interfaces: { wan0: wan({}, { parent: 'nope0' }) } })).toEqual([
      expect.objectContaining({
        pointer: '/interfaces/wan0/pppoe/parent',
        message: expect.stringContaining("'nope0' does not exist"),
      }),
    ]);
  });

  it('rejects a disabled dial-over parent', () => {
    expect(
      run({
        interfaces: {
          eth1: { enabled: false },
          wan0: wan({}, { parent: 'eth1' }),
        },
      }),
    ).toEqual([
      expect.objectContaining({
        pointer: '/interfaces/wan0/pppoe/parent',
        message: expect.stringContaining('disabled'),
      }),
    ]);
  });

  it('rejects an MTU that does not fit the parent link, accepts one that does', () => {
    expect(run({ interfaces: { eth0: wan({ mtu: 1500 }, { mtu: 1496 }) } })).toEqual([
      expect.objectContaining({
        pointer: '/interfaces/eth0/pppoe/mtu',
        message: expect.stringContaining('at most 1492'),
      }),
    ]);
    expect(run({ interfaces: { eth0: wan({ mtu: 1500 }, { mtu: 1492 }) } })).toEqual([]);
  });

  it('requires the IPv6 minimum link MTU when IPv6 is on', () => {
    expect(run({ interfaces: { wan0: wan({}, { ipv6: 'slaac', mtu: 1279 }) } })).toEqual([
      expect.objectContaining({
        pointer: '/interfaces/wan0/pppoe/mtu',
        message: expect.stringContaining('IPv6 minimum link MTU 1280'),
      }),
    ]);
    expect(run({ interfaces: { wan0: wan({}, { ipv6: 'dhcpv6', mtu: 1280 }) } })).toEqual([]);
    expect(run({ interfaces: { wan0: wan({}, { ipv6: 'off', mtu: 1279 }) } })).toEqual([]);
  });
});

describe('explicit PPP DHCPv6 delegation', () => {
  const target = { interface: 'lan0', subnetId: 1 };
  const config = (ppp: Record<string, unknown> = {}, lan: Record<string, unknown> = {}) => ({
    interfaces: {
      wan0: wan({}, { ipv6: 'dhcpv6', delegationTargets: [target], ...ppp }),
      lan0: { enabled: true, ...lan },
    },
  });
  it('accepts a same-VRF enabled LAN without static IPv6 or RA', () =>
    expect(run(config())).toEqual([]));
  it('rejects non-DHCPv6, missing, disabled, PPP, static and cross-VRF targets', () => {
    for (const doc of [
      config({ ipv6: 'slaac' }),
      config({ delegationTargets: [{ ...target, interface: 'missing' }] }),
      config({}, { enabled: false }),
      config({}, { ipv6: ['2001:db8::1/64'] }),
      config({}, { ipv6Ra: {} }),
      config({}, { vrf: 'other' }),
      config({}, { pppoe: { username: 'u', passwordRef: 'password/isp' } }),
    ]) {
      expect(run(doc).length).toBeGreaterThan(0);
    }
  });
  it('rejects duplicate target IDs/interfaces and multiple WAN ownership', () => {
    expect(run(config({ delegationTargets: [target, target] })).length).toBeGreaterThan(0);
    const doc = config();
    Object.assign(doc.interfaces, {
      wan1: wan({}, { ipv6: 'dhcpv6', delegationTargets: [target] }),
    });
    expect(run(doc).some((issue) => issue.message.includes('already assigned'))).toBe(true);
  });
  it('rejects unrepresentable or negative subnet IDs at parse time', () => {
    for (const subnetId of [-1, 0.5, 4294967296]) {
      expect(() =>
        RootConfig.parse(config({ delegationTargets: [{ ...target, subnetId }] })),
      ).toThrow();
    }
  });
});
