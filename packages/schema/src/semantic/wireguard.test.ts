import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { validateSemantics } from './index.js';
import { sortIssues } from './registry.js';
import { BASE, example } from './vpn.fixtures.js';
import { wireguardValidators } from './wireguard.js';

const PUB_A = 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=';
const PUB_B = 'xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=';

const wg = (instance: number, extra: Record<string, unknown> = {}) => ({
  instance,
  listenAddress: '198.51.100.2',
  listenPort: 51820 + instance,
  privateKeyRef: `key/wg${instance}`,
  peers: {},
  ...extra,
});
const doc = (interfaces: Record<string, unknown>) => ({ ...BASE, vpn: { wireguard: { interfaces } } });
const run = (d: unknown, name?: string) => {
  const config = RootConfig.parse(d);
  return sortIssues(
    wireguardValidators
      .filter((v) => name === undefined || v.name === name)
      .flatMap((v) => v.validate(config)),
  );
};

describe('F-wireguard semantic rules', () => {
  it('are registered in the aggregate, prefixed vpn. and read vpn', () => {
    for (const v of wireguardValidators) {
      expect(v.name.startsWith('vpn.wireguard-')).toBe(true);
      expect(v.domains).toContain('vpn');
    }
    expect(validateSemantics(RootConfig.parse(example('vpn-wireguard.json')))).toEqual([]);
  });

  it('routeAllowedIps defaults to false', () => {
    const c = RootConfig.parse(doc({ a: wg(0) }));
    expect(c.vpn.wireguard.interfaces['a']?.routeAllowedIps).toBe(false);
  });

  it('vpn.wireguard-public-key-unique: a public key on two interfaces points at the second peer', () => {
    const d = doc({
      a: wg(0, { peers: { p1: { publicKey: PUB_A, allowedIps: ['10.0.0.0/24'] } } }),
      b: wg(1, {
        peers: {
          ok: { publicKey: PUB_B, allowedIps: ['10.1.0.0/24'] },
          dup: { publicKey: PUB_A, allowedIps: ['10.2.0.0/24'] },
        },
      }),
    });
    expect(run(d, 'vpn.wireguard-public-key-unique')).toEqual([
      {
        pointer: '/vpn/wireguard/interfaces/b/peers/dup/publicKey',
        message:
          "public key is already used by peer 'p1' of WireGuard interface 'a' (VPP keys peers by public key across all interfaces)",
      },
    ]);
    // the whole pipeline reports it too (and nothing else)
    expect(validateSemantics(RootConfig.parse(d))).toEqual(run(d, 'vpn.wireguard-public-key-unique'));
  });

  it('vpn.wireguard-public-key-unique leaves duplicates inside one interface to vpn.wireguard-unique', () => {
    const d = doc({
      a: wg(0, {
        peers: {
          p1: { publicKey: PUB_A, allowedIps: ['10.0.0.0/24'] },
          p2: { publicKey: PUB_A, allowedIps: ['10.1.0.0/24'] },
        },
      }),
    });
    expect(run(d, 'vpn.wireguard-public-key-unique')).toEqual([]);
  });

  it('vpn.wireguard-allowed-ips: host bits and overlaps between peers of one interface', () => {
    const d = doc({
      a: wg(0, {
        peers: {
          p1: { publicKey: PUB_A, allowedIps: ['10.0.0.0/16', '192.168.1.7/24', 'fd00::/64'] },
          p2: { publicKey: PUB_B, allowedIps: ['10.0.5.0/24', '10.0.0.0/16', 'fd00::1:0/112', '172.16.0.0/12'] },
        },
      }),
      // another interface may reuse the prefixes
      b: wg(1, { peers: { p3: { publicKey: 'AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=', allowedIps: ['10.0.0.0/16'] } } }),
    });
    expect(run(d, 'vpn.wireguard-allowed-ips')).toEqual([
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/p1/allowedIps/1',
        message: '192.168.1.7/24 has host bits set (a network prefix is expected)',
      },
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/p2/allowedIps/0',
        message: "10.0.5.0/24 overlaps 10.0.0.0/16 of peer 'p1' (allowed IPs of one interface must not overlap)",
      },
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/p2/allowedIps/2',
        message: "fd00::1:0/112 overlaps fd00::/64 of peer 'p1' (allowed IPs of one interface must not overlap)",
      },
    ]);
  });

  it('vpn.wireguard-endpoint-family: an IP endpoint has the listen family; hostnames are not checked', () => {
    const d = doc({
      a: wg(0, {
        peers: {
          v6: { publicKey: PUB_A, allowedIps: ['10.0.0.0/24'], endpoint: { address: '2001:db8::7', port: 51820 } },
          v4: { publicKey: PUB_B, allowedIps: ['10.1.0.0/24'], endpoint: { address: '203.0.113.9', port: 51820 } },
          dns: {
            publicKey: 'AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=',
            allowedIps: ['10.2.0.0/24'],
            endpoint: { address: 'vpn.example.net', port: 51820 },
          },
        },
      }),
    });
    expect(run(d, 'vpn.wireguard-endpoint-family')).toEqual([
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/v6/endpoint/address',
        message: 'endpoint 2001:db8::7 is IPv6 but the interface listens on IPv4 address 198.51.100.2',
      },
    ]);
  });
});
