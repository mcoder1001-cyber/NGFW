import { describe, expect, it } from 'vitest';
import { diffPointerLabel, diffValueText } from './diff-presentation';

describe('configuration diff presentation', () => {
  it('labels known metadata paths without changing record keys or pointer escaping', () => {
    expect(diffPointerLabel('/services/dns/vppCache/upstreams')).toBe(
      '/services/dns/dataplaneCache/upstreams',
    );
    expect(diffPointerLabel('/routing/static/0/viaFrr')).toBe(
      '/routing/static/0/viaRoutingService',
    );
    for (const pointer of [
      '/interfaces/vpp~1frr~0strongswan/mtu',
      '/vpn/ipsec/tunnels/vppCache/description',
      '/ha/vrrp/viaFrr/engine',
      '/system/hostname',
      '',
      'invalid',
    ])
      expect(diffPointerLabel(pointer)).toBe(pointer);
  });

  it('labels nested schema-owned members and engine enums while preserving source configuration', () => {
    const value = {
      services: { dns: { vppCache: { enabled: true } } },
      routing: { static: [{ viaFrr: true, nextHops: [{ interface: 'vpp' }] }] },
      vpn: {
        ipsec: {
          tunnels: {
            frr: { engine: 'strongswan', description: 'strongSwan peer with VPP and FRR' },
          },
        },
      },
      ha: { vrrp: { strongswan: { engine: 'vpp', interface: 'frr' } } },
      interfaces: { vppCache: { description: 'vpp', unnumbered: 'strongswan' } },
      unknown: { vppCache: true, viaFrr: false, engine: 'strongswan' },
    };
    const original = structuredClone(value);
    expect(JSON.parse(diffValueText(value, '')!)).toEqual({
      ...value,
      services: { dns: { dataplaneCache: { enabled: true } } },
      routing: { static: [{ viaRoutingService: true, nextHops: [{ interface: 'vpp' }] }] },
      vpn: {
        ipsec: {
          tunnels: {
            frr: { engine: 'IPsec service', description: 'strongSwan peer with VPP and FRR' },
          },
        },
      },
      ha: { vrrp: { strongswan: { engine: 'Dataplane', interface: 'frr' } } },
    });
    expect(value).toEqual(original);
  });

  it('localizes declared engine choices, including scalar changes, and preserves arbitrary strings', () => {
    expect(diffValueText('vpp-ikev2', '/vpn/ipsec/tunnels/branch/engine')).toBe('"Native IKEv2"');
    expect(diffValueText('strongswan', '/vpn/ipsec/tunnels/branch/engine', 'fa-IR')).toBe(
      '"سرویس IPsec"',
    );
    expect(diffValueText('vpp', '/ha/vrrp/branch/engine', 'fa')).toBe('"صفحهٔ داده"');
    expect(diffValueText('vpp', '/vpn/ipsec/tunnels/branch/engine')).toBe('"vpp"');
    expect(diffValueText('strongswan', '/interfaces/engine/description')).toBe('"strongswan"');
    expect(diffValueText('vpp', '/unknown/engine')).toBe('"vpp"');
    expect(diffValueText('strongswan', '/system/hostname')).toBe('"strongswan"');
  });

  it('preserves empty values, unknown objects and every member when a display label collides', () => {
    for (const value of [
      undefined,
      null,
      [],
      {},
      [null, {}, []],
      { arbitrary: ['FRR', 'VPP', 'strongSwan'] },
    ]) {
      expect(diffValueText(value, '/unknown')).toBe(JSON.stringify(value, null, 2));
    }
    expect(
      diffValueText(
        { vppCache: { enabled: true }, dataplaneCache: 'future setting' },
        '/services/dns',
      ),
    ).toBe(
      '{\n  "dataplaneCache": {\n    "enabled": true\n  },\n  "dataplaneCache": "future setting"\n}',
    );
  });
});
