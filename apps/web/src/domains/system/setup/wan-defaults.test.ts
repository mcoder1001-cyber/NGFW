import { describe, expect, it } from 'vitest';
import { RootConfig } from '@ngfw/schema';
import { wanDefaults } from './wan-defaults';

describe('WAN defaults from system configuration', () => {
  it('prefills the interface, static address and gateway from an existing default route', () => {
    const doc = RootConfig.parse({
      interfaces: { uplink: { ipv4: ['203.0.113.2/24'] }, internal: {} },
      routing: {
        static: [
          { prefix: '0.0.0.0/0', nextHops: [{ interface: 'uplink', address: '203.0.113.1' }] },
        ],
      },
    });
    expect(wanDefaults(doc, ['internal', 'uplink'])).toMatchObject({
      wan: 'uplink',
      wanMode: 'static',
      wanAddress: '203.0.113.2/24',
      wanGateway: '203.0.113.1',
    });
  });
  it('finds an address-only default next hop using the interface subnet', () => {
    const doc = RootConfig.parse({
      interfaces: { uplink: { ipv4: ['203.0.113.2/24'] }, internal: {} },
      routing: { static: [{ prefix: '0.0.0.0/0', nextHops: [{ address: '203.0.113.1' }] }] },
    });
    expect(wanDefaults(doc, ['internal', 'uplink'])).toMatchObject({
      wan: 'uplink',
      wanMode: 'static',
      wanGateway: '203.0.113.1',
    });
  });
  it('keeps a DHCP WAN in DHCP mode despite its current lease address', () => {
    const doc = RootConfig.parse({
      interfaces: { uplink: { dhcpClient: {}, ipv4: ['192.0.2.2/24'] }, internal: {} },
    });
    expect(wanDefaults(doc, ['internal', 'uplink'])).toMatchObject({
      wan: 'uplink',
      wanMode: 'dhcp',
    });
  });
  it('does not select an arbitrary interface when detection is ambiguous', () => {
    expect(wanDefaults(RootConfig.parse({ interfaces: { a: {}, b: {} } }), ['a', 'b'])).toEqual({});
  });
  it('does not hide multiple default routes with a weaker NAT hint', () => {
    const doc = RootConfig.parse({
      interfaces: { a: {}, b: {} },
      nat: { outside: ['a'] },
      routing: {
        static: [
          {
            prefix: '0.0.0.0/0',
            nextHops: [
              { interface: 'a', address: '192.0.2.1' },
              { interface: 'b', address: '198.51.100.1' },
            ],
          },
        ],
      },
    });
    expect(wanDefaults(doc, ['a', 'b'])).toEqual({});
  });
  it('maps the configured logical PPPoE WAN to its physical parent', () => {
    const doc = RootConfig.parse({
      interfaces: {
        uplink: {},
        'setup-pppoe': {
          pppoe: { parent: 'uplink', username: 'isp', passwordRef: 'password/isp' },
        },
      },
      nat: { outside: ['setup-pppoe'] },
    });
    expect(wanDefaults(doc, ['uplink'])).toMatchObject({
      wan: 'uplink',
      wanMode: 'pppoe',
      wanPppoe: { username: 'isp', passwordRef: 'password/isp' },
    });
  });
  it('does not select host-owned or unavailable interfaces through a default route', () => {
    const doc = RootConfig.parse({
      interfaces: { host: {} },
      routing: {
        static: [{ prefix: '0.0.0.0/0', nextHops: [{ interface: 'host', address: '192.0.2.1' }] }],
      },
    });
    expect(wanDefaults(doc, [])).toEqual({});
  });
});
