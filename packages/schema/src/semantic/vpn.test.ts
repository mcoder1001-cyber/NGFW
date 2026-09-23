import { describe, expect, it } from 'vitest';
import { RootConfig } from '../index.js';
import { BASE, example, run } from './vpn.fixtures.js';
import { vpnValidators } from './vpn.js';

/** PEM banners are assembled at runtime so the source never matches the CI private-key scan (review F1). */
const pem = (label: string): string => ['-----BEGIN', `${label}-----`].join(' ');

const PROPOSAL = { ike: { encr: 'aes256gcm16', dh: 'ecp256' }, esp: { encr: 'aes256gcm16' } };
const tunnel = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  localAddr: '198.51.100.2',
  remoteAddr: '203.0.113.10',
  auth: { method: 'psk', secretRef: 'psk/x' },
  proposal: 'p',
  localTs: ['192.168.10.0/24'],
  remoteTs: ['10.99.0.0/16'],
  ...extra,
});
const withIpsec = (tunnels: Record<string, unknown>, extra: Record<string, unknown> = {}) => ({
  ...BASE,
  ...extra,
  vpn: { ipsec: { proposals: { p: PROPOSAL }, tunnels } },
});

describe('vpn examples', () => {
  it.each(['vpn-site-to-site.json', 'vpn-wireguard.json', 'vpn-remote-access.json'])(
    '%s is semantically valid',
    (f) => {
      expect(run(example(f))).toEqual([]);
    },
  );
  it('vpn-semantic-missing-proposal.json triggers vpn.proposal-exists only', () => {
    expect(run(example('vpn-semantic-missing-proposal.json'))).toEqual([
      {
        pointer: '/vpn/ipsec/tunnels/site-b/proposal',
        message: "IPsec proposal 'does-not-exist' does not exist",
      },
    ]);
  });
  it('every validator name is prefixed with vpn and reads vpn', () => {
    for (const v of vpnValidators) {
      expect(v.name.startsWith('vpn.')).toBe(true);
      expect(v.domains).toContain('vpn');
    }
    expect(new Set(vpnValidators.map((v) => v.name)).size).toBe(vpnValidators.length);
  });
});

describe('vpn.proposal-exists / vpn.pki-reference-exists', () => {
  it('reports missing proposals on tunnels and remote-access profiles', () => {
    const doc = {
      ...BASE,
      vpn: {
        ipsec: { tunnels: { t: tunnel({ proposal: 'nope' }) } },
        pki: { certificates: { c: { certificateRef: 'cert/c', privateKeyRef: 'key/c' } } },
        remoteAccess: {
          ra: {
            localAddr: '198.51.100.2',
            certificate: 'c',
            proposal: 'gone',
            pools: [{ name: 'p', prefix: '10.250.0.0/24' }],
            users: [{ username: 'a', passwordRef: 'password/a' }],
          },
        },
      },
    };
    expect(run(doc, 'vpn.proposal-exists')).toEqual([
      { pointer: '/vpn/ipsec/tunnels/t/proposal', message: "IPsec proposal 'nope' does not exist" },
      { pointer: '/vpn/remoteAccess/ra/proposal', message: "IPsec proposal 'gone' does not exist" },
    ]);
  });
  it('reports missing certificates and CAs wherever they are referenced', () => {
    const doc = {
      ...BASE,
      vpn: {
        ipsec: {
          proposals: { p: PROPOSAL },
          tunnels: {
            t: tunnel({ auth: { method: 'cert', certificate: 'missing', remoteCa: 'no-ca' } }),
          },
        },
        pki: {
          cas: { ca1: { certificateRef: 'cert/ca1' } },
          certificates: {
            c: { certificateRef: 'cert/c', privateKeyRef: 'key/c', ca: 'ghost' },
          },
        },
        remoteAccess: {
          ra: {
            localAddr: '198.51.100.2',
            certificate: 'other',
            clientCa: 'ca2',
            auth: 'eap-tls',
            proposal: 'p',
            pools: [{ name: 'p', prefix: '10.250.0.0/24' }],
          },
        },
      },
    };
    expect(run(doc, 'vpn.pki-reference-exists').map((i) => i.pointer)).toEqual([
      '/vpn/ipsec/tunnels/t/auth/certificate',
      '/vpn/ipsec/tunnels/t/auth/remoteCa',
      '/vpn/pki/certificates/c/ca',
      '/vpn/remoteAccess/ra/certificate',
      '/vpn/remoteAccess/ra/clientCa',
    ]);
    const good = {
      ...doc,
      vpn: {
        ...doc.vpn,
        ipsec: {
          proposals: { p: PROPOSAL },
          tunnels: { t: tunnel({ auth: { method: 'cert', certificate: 'c', remoteCa: 'ca1' } }) },
        },
      },
    };
    expect(
      run(good, 'vpn.pki-reference-exists').filter((i) => i.pointer.includes('/tunnels/')),
    ).toEqual([]);
  });
});

describe('vpn.vrf-exists / vpn.local-address-configured', () => {
  it('unknown VRFs on tunnels, WireGuard and remote access', () => {
    const doc = {
      ...BASE,
      vpn: {
        ipsec: {
          proposals: { p: PROPOSAL },
          tunnels: { t: tunnel({ vrf: 'nope' }), u: tunnel({ underlayVrf: 'under' }) },
        },
        remoteAccess: {
          ra: {
            localAddr: '198.51.100.2',
            certificate: 'c',
            proposal: 'p',
            vrf: 'ra-over',
            underlayVrf: 'ra-under',
            pools: [{ name: 'p', prefix: '10.250.0.0/24' }],
            users: [{ username: 'a', passwordRef: 'password/a' }],
          },
        },
        wireguard: {
          interfaces: {
            w: {
              instance: 0,
              listenAddress: '198.51.100.2',
              privateKeyRef: 'key/w',
              vrf: 'v1',
              underlayVrf: 'v2',
            },
          },
        },
      },
    };
    expect(run(doc, 'vpn.vrf-exists').map((i) => i.pointer)).toEqual([
      '/vpn/ipsec/tunnels/t/vrf',
      '/vpn/ipsec/tunnels/u/underlayVrf',
      '/vpn/remoteAccess/ra/underlayVrf',
      '/vpn/remoteAccess/ra/vrf',
      '/vpn/wireguard/interfaces/w/underlayVrf',
      '/vpn/wireguard/interfaces/w/vrf',
    ]);
  });
  it('local addresses must be configured on an interface in the right VRF', () => {
    const doc = {
      ...BASE,
      vpn: {
        ipsec: {
          proposals: { p: PROPOSAL },
          tunnels: {
            ok: tunnel(),
            // F4: the overlay VRF says nothing about where localAddr lives
            overlayOnly: tunnel({ vrf: 'customer-a' }),
            wrongVrf: tunnel({ underlayVrf: 'customer-a' }),
            unknown: tunnel({ localAddr: '198.51.100.3' }),
          },
        },
        wireguard: {
          interfaces: {
            ok: {
              instance: 0,
              listenAddress: '10.20.0.1',
              underlayVrf: 'customer-a',
              privateKeyRef: 'key/a',
            },
            bad: { instance: 1, listenAddress: '10.20.0.1', privateKeyRef: 'key/b' },
          },
        },
      },
    };
    expect(run(doc, 'vpn.local-address-configured')).toEqual([
      {
        pointer: '/vpn/ipsec/tunnels/unknown/localAddr',
        message: "198.51.100.3 is not configured on any interface in VRF 'default'",
      },
      {
        pointer: '/vpn/ipsec/tunnels/wrongVrf/localAddr',
        message: "198.51.100.2 is not configured on any interface in VRF 'customer-a'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/bad/listenAddress',
        message: "10.20.0.1 is not configured on any interface in VRF 'default'",
      },
    ]);
  });
});

describe('vpn.route-based-ipip', () => {
  const ipip = (extra: Record<string, unknown> = {}) => ({
    src: '198.51.100.2',
    dst: '203.0.113.10',
    ...extra,
  });
  const rb = (ipipInterface: string, extra: Record<string, unknown> = {}) =>
    tunnel({ localTs: [], remoteTs: [], routeBased: { ipipInterface }, ...extra });
  it('accepts a matching p2p IPIP and a p2mp IPIP for %any', () => {
    const doc = withIpsec(
      {
        a: rb('i1'),
        b: rb('hub', { remoteAddr: '%any', startAction: 'trap' }),
        c: rb('i2', { remoteAddr: 'peer.example' }),
      },
      {
        tunnels: {
          ipip: {
            i1: ipip(),
            hub: ipip({ mode: 'p2mp', dst: undefined }),
            i2: ipip({ dst: '203.0.113.99' }),
          },
        },
      },
    );
    expect(run(doc, 'vpn.route-based-ipip')).toEqual([]);
  });
  it.each([
    ['missing IPIP tunnel', { a: rb('missing') }, { i1: ipip() }, /does not exist in tunnels.ipip/],
    [
      'source mismatch',
      { a: rb('i1') },
      { i1: ipip({ src: '192.168.10.1' }) },
      /source 192.168.10.1 differs from localAddr/,
    ],
    [
      'underlay VRF mismatch',
      { a: rb('i1') },
      { i1: ipip({ underlayVrf: 'customer-a' }) },
      /underlay VRF 'customer-a' differs/,
    ],
    [
      'overlay VRF mismatch (F4)',
      { a: rb('i1', { vrf: 'customer-a' }) },
      { i1: ipip() },
      /VRF 'default' differs from the IPsec overlay VRF 'customer-a'/,
    ],
    [
      '%any with a p2p IPIP',
      { a: rb('i1', { remoteAddr: '%any', startAction: 'trap' }) },
      { i1: ipip() },
      /needs a point-to-multipoint/,
    ],
    [
      'fixed peer with a p2mp IPIP',
      { a: rb('i1') },
      { i1: ipip({ mode: 'p2mp', dst: undefined }) },
      /must be point-to-point/,
    ],
    [
      'destination mismatch',
      { a: rb('i1') },
      { i1: ipip({ dst: '203.0.113.11' }) },
      /destination 203.0.113.11 differs from remoteAddr/,
    ],
    [
      'IPIP shared by two tunnels',
      { a: rb('i1'), b: rb('i1', { remoteAddr: '203.0.113.10', vrf: 'default' }) },
      { i1: ipip() },
      /already protected by IPsec tunnel 'a'/,
    ],
  ])('%s', (_label, tunnels, ipips, message) => {
    const issues = run(withIpsec(tunnels, { tunnels: { ipip: ipips } }), 'vpn.route-based-ipip');
    expect(issues.length).toBeGreaterThan(0);
    expect(
      issues.some(
        (i) => message.test(i.message) && i.pointer.endsWith('/routeBased/ipipInterface'),
      ),
    ).toBe(true);
  });
});

describe('vpn.ipsec-peer-unique', () => {
  it('two enabled tunnels with the same local/remote/VRF clash; disabled ones do not', () => {
    const doc = withIpsec({
      a: tunnel(),
      b: tunnel(),
      c: tunnel({ enabled: false }),
      d: tunnel({ underlayVrf: 'customer-a' }),
      e: tunnel({ vrf: 'customer-a' }),
    });
    expect(run(doc, 'vpn.ipsec-peer-unique')).toEqual([
      {
        pointer: '/vpn/ipsec/tunnels/b/remoteAddr',
        message: "tunnel 'a' already uses 198.51.100.2 → 203.0.113.10 in VRF 'default'",
      },
      {
        pointer: '/vpn/ipsec/tunnels/e/remoteAddr',
        message: "tunnel 'a' already uses 198.51.100.2 → 203.0.113.10 in VRF 'default'",
      },
    ]);
  });
  it('several %any responders on one address are fine when their IDs or auth differ (F6)', () => {
    const any = (extra: Record<string, unknown>) =>
      tunnel({ remoteAddr: '%any', startAction: 'trap', ...extra });
    const doc = withIpsec(
      {
        pskSpokes: any({
          localId: 'hub.example',
          auth: { method: 'psk', secretRef: 'psk/spokes' },
        }),
        certSpokes: any({ localId: 'hub.example', auth: { method: 'cert', certificate: 'c' } }),
        partner: any({ remoteId: 'partner.example' }),
        clash: any({ localId: 'hub.example', auth: { method: 'psk', secretRef: 'psk/other' } }),
      },
      {},
    );
    expect(run(doc, 'vpn.ipsec-peer-unique')).toEqual([
      {
        pointer: '/vpn/ipsec/tunnels/clash/remoteAddr',
        message:
          "tunnel 'pskSpokes' already accepts %any on 198.51.100.2 in VRF 'default' with the same localId/remoteId/auth method — indistinguishable responders",
      },
    ]);
  });
});

describe('vpn.proposal-compatible (F14)', () => {
  const withProposals = (tunnels: Record<string, unknown>) => ({
    ...BASE,
    vpn: {
      ipsec: {
        proposals: {
          p: PROPOSAL,
          classic: {
            ike: { encr: 'aes256', integ: 'sha256', dh: 'modp2048' },
            esp: { encr: 'aes256', integ: 'sha256' },
          },
          ah: {
            ike: { encr: 'aes256', integ: 'sha256', dh: 'modp2048' },
            esp: { encr: 'null', integ: 'sha256' },
          },
          modern: {
            ike: { encr: 'aes256', integ: 'sha256', dh: 'curve25519' },
            esp: { encr: 'aes256', integ: 'sha256', dh: 'curve448' },
          },
        },
        tunnels,
      },
    },
  });
  it('accepts matching combinations and skips unknown proposals', () => {
    expect(
      run(
        withProposals({
          a: tunnel({ protocol: 'ah', proposal: 'ah' }),
          b: tunnel({ ikeVersion: 1, proposal: 'classic', remoteAddr: '203.0.113.11' }),
          c: tunnel({ proposal: 'modern', remoteAddr: '203.0.113.12' }),
          d: tunnel({ proposal: 'gone', remoteAddr: '203.0.113.13' }),
        }),
        'vpn.proposal-compatible',
      ),
    ).toEqual([]);
  });
  it('rejects encrypting AH, AEAD IKE SAs and IKEv2-only groups with IKEv1', () => {
    expect(
      run(
        withProposals({
          a: tunnel({ protocol: 'ah', proposal: 'classic' }),
          b: tunnel({ ikeVersion: 1, proposal: 'p', remoteAddr: '203.0.113.11' }),
          c: tunnel({ ikeVersion: 1, proposal: 'modern', remoteAddr: '203.0.113.12' }),
        }),
        'vpn.proposal-compatible',
      ).map((i) => [i.pointer, i.message]),
    ).toEqual([
      [
        '/vpn/ipsec/tunnels/a/proposal',
        "AH does not encrypt: proposal 'classic' must use esp.encr 'null' with an integrity algorithm",
      ],
      [
        '/vpn/ipsec/tunnels/b/proposal',
        "IKEv1 has no AEAD IKE SAs: proposal 'p' uses ike.encr 'aes256gcm16'",
      ],
      [
        '/vpn/ipsec/tunnels/c/proposal',
        "curve25519 is defined for IKEv2 only: proposal 'modern' uses it in ike.dh",
      ],
      [
        '/vpn/ipsec/tunnels/c/proposal',
        "curve448 is defined for IKEv2 only: proposal 'modern' uses it in esp.dh",
      ],
    ]);
  });
});

describe('vpn.wireguard-unique', () => {
  const PEER_PUB_A = 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=';
  const PEER_PUB_B = 'xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=';
  const wg = (instance: number, extra: Record<string, unknown> = {}) => ({
    instance,
    listenAddress: '198.51.100.2',
    privateKeyRef: `key/wg${instance}`,
    peers: {},
    ...extra,
  });
  it('instances and listen sockets are unique; peer keys and allowed prefixes are unique per interface', () => {
    const doc = {
      ...BASE,
      vpn: {
        wireguard: {
          interfaces: {
            a: wg(0, {
              peers: {
                p1: { publicKey: PEER_PUB_A, allowedIps: ['10.0.0.0/24', '10.0.0.0/24'] },
                p2: { publicKey: PEER_PUB_A, allowedIps: ['10.0.0.5/24', '10.1.0.0/16'] },
              },
            }),
            b: wg(0, { listenPort: 51821 }),
            c: wg(1, { peers: { p1: { publicKey: PEER_PUB_A, allowedIps: ['10.0.0.0/24'] } } }),
            d: wg(2, {
              listenAddress: '198.51.100.2',
              underlayVrf: 'customer-a',
              peers: { p1: { publicKey: PEER_PUB_B, allowedIps: ['0.0.0.0/0'] } },
            }),
          },
        },
      },
    };
    expect(run(doc, 'vpn.wireguard-unique')).toEqual([
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/p2/allowedIps/0',
        message: "10.0.0.5/24 is already routed to peer 'p1'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/a/peers/p2/publicKey',
        message: "same public key as peer 'p1'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/b/instance',
        message: "instance 0 (wg0) is already used by 'a'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/c/listenPort',
        message: "198.51.100.2:51820 in VRF 'default' is already used by 'a'",
      },
    ]);
  });
});

describe('vpn.wireguard-address-overlap (F10)', () => {
  it('WireGuard addresses must not overlap physical or other WireGuard prefixes in their VRF', () => {
    const wg = (instance: number, address: string[], extra: Record<string, unknown> = {}) => ({
      instance,
      listenAddress: '198.51.100.2',
      listenPort: 51820 + instance,
      privateKeyRef: `key/wg${instance}`,
      address,
      ...extra,
    });
    const doc = {
      ...BASE,
      vpn: {
        wireguard: {
          interfaces: {
            a: wg(0, ['10.200.0.1/24', '192.168.10.77/24']),
            b: wg(1, ['10.200.0.129/25']),
            c: wg(2, ['10.20.0.5/24'], { vrf: 'customer-a' }),
            d: wg(3, ['10.20.0.5/24', 'fd00::1/64']),
          },
        },
      },
    };
    expect(run(doc, 'vpn.wireguard-address-overlap')).toEqual([
      {
        pointer: '/vpn/wireguard/interfaces/a/address/1',
        message:
          "192.168.10.77/24 overlaps an address of interface 'TenGigabitEthernet0/0/1' in VRF 'default'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/b/address/0',
        message: "10.200.0.129/25 overlaps an address of WireGuard interface 'a' in VRF 'default'",
      },
      {
        pointer: '/vpn/wireguard/interfaces/c/address/0',
        message:
          "10.20.0.5/24 overlaps an address of interface 'TenGigabitEthernet0/0/2' in VRF 'customer-a'",
      },
    ]);
  });
});

describe('vpn.remote-access-pools-no-overlap', () => {
  it('pools across profiles must not overlap', () => {
    const profile = (prefix: string) => ({
      localAddr: '198.51.100.2',
      certificate: 'c',
      proposal: 'p',
      pools: [{ name: 'p', prefix }],
      users: [{ username: 'a', passwordRef: 'password/a' }],
    });
    const doc = {
      ...BASE,
      vpn: {
        remoteAccess: {
          a: profile('10.250.0.0/24'),
          b: profile('10.250.0.128/25'),
          c: profile('10.251.0.0/24'),
        },
      },
    };
    expect(run(doc, 'vpn.remote-access-pools-no-overlap')).toEqual([
      {
        pointer: '/vpn/remoteAccess/b/pools/0/prefix',
        message: "client pool 10.250.0.128/25 overlaps pool 'a/p'",
      },
    ]);
  });
});

describe('vpn.no-inline-secret-material', () => {
  it('flags PEM banners of any kind and hex/base64 key blobs in all four group-(c) domains, but not exempt fields', () => {
    const doc = withIpsec(
      { t: tunnel({ description: pem('RSA PRIVATE KEY') }) },
      {
        tunnels: {
          gre: {
            g: {
              src: '198.51.100.2',
              dst: '203.0.113.1',
              description: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
            },
            h: {
              src: '198.51.100.2',
              dst: '203.0.113.2',
              description: 'AAECAwQFBgcICQoLDA0ODw==',
            },
            i: {
              src: '198.51.100.2',
              dst: '203.0.113.3',
              description: '000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f',
            },
          },
        },
        services: {
          snmp: {
            sysContact: pem('PGP PRIVATE KEY BLOCK'),
            engineId: '800000090300aabbccddeeff00112233',
          },
          dns: {
            resolvers: {
              r: {
                listen: [{ address: '0.0.0.0' }],
                localZones: [
                  {
                    zone: 'example.com',
                    records: [
                      { name: 'k._domainkey', type: 'TXT', data: 'AAECAwQFBgcICQoLDA0ODw==' },
                    ],
                  },
                ],
              },
            },
          },
        },
        ha: {
          vrrp: {
            lan: {
              interface: 'TenGigabitEthernet0/0/1',
              vrId: 1,
              addresses: ['192.168.10.254'],
              description: pem('CERTIFICATE'),
            },
          },
        },
      },
    );
    (doc.vpn as Record<string, unknown>).wireguard = {
      interfaces: {
        w: {
          instance: 0,
          listenAddress: '198.51.100.2',
          privateKeyRef: 'key/w',
          peers: {
            p: {
              publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
              allowedIps: ['10.0.0.0/8'],
            },
          },
        },
      },
    };
    const PEM_MSG =
      'PEM material is not allowed in the configuration; store it through POST /api/v1/secrets and reference it';
    const KEY_MSG =
      'this looks like key material (base64 or hex); keys are referenced through a secret reference, never inline';
    expect(run(doc, 'vpn.no-inline-secret-material')).toEqual([
      { pointer: '/ha/vrrp/lan/description', message: PEM_MSG },
      { pointer: '/services/snmp/sysContact', message: PEM_MSG },
      { pointer: '/tunnels/gre/g/description', message: KEY_MSG },
      { pointer: '/tunnels/gre/h/description', message: KEY_MSG },
      { pointer: '/tunnels/gre/i/description', message: KEY_MSG },
      { pointer: '/vpn/ipsec/tunnels/t/description', message: PEM_MSG },
    ]);
  });
  it('is quiet on ordinary text, interface names and secret references', () => {
    expect(
      run(
        withIpsec({
          t: tunnel({
            description: 'plain description = fine',
            auth: { method: 'psk', secretRef: 'psk/Branch2VpnPrivateKey01' },
          }),
        }),
        'vpn.no-inline-secret-material',
      ),
    ).toEqual([]);
    expect(
      run(
        {
          ...BASE,
          services: { lldp: { interfaces: [{ interface: 'TenGigabitEthernet0/0/10' }] } },
        },
        'vpn.no-inline-secret-material',
      ),
    ).toEqual([]);
  });
  it('the schema itself rejects secrets pasted into RADIUS, SNMPv3 and NTP references (D-051)', () => {
    const radius = {
      vpn: {
        remoteAccess: {
          ra: {
            localAddr: '198.51.100.2',
            certificate: 'c',
            proposal: 'p',
            auth: 'eap-radius',
            pools: [{ name: 'p', prefix: '10.250.0.0/24' }],
            radius: { servers: [{ address: '10.0.0.1', secretRef: 'SuperSecretRadius1' }] },
          },
        },
      },
    };
    const snmp = {
      services: {
        snmp: { v3Users: { u: { authRef: 'noc-auth-pass-2026', privRef: 'password/p' } } },
      },
    };
    const ntp = {
      services: { ntp: { servers: [{ address: '10.0.0.1', keyRef: 'SHA1 HEX:1a2b' }] } },
    };
    const paths = (doc: unknown) =>
      RootConfig.safeParse(doc).error?.issues.map((i) => i.path.join('/'));
    expect(paths(radius)).toEqual(['vpn/remoteAccess/ra/radius/servers/0/secretRef']);
    expect(paths(snmp)).toEqual(['services/snmp/v3Users/u/authRef']);
    expect(paths(ntp)).toEqual(['services/ntp/servers/0/keyRef']);
  });
});
