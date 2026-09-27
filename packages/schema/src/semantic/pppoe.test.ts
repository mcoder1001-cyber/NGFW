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
});
