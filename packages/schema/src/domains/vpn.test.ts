import { describe, expect, it } from 'vitest';
import type { z } from 'zod';
import {
  EspProposalSchema,
  IkeProposalSchema,
  IpsecTunnelSchema,
  PkiCertificateSchema,
  RemoteAccessProfileSchema,
  VpnSchema,
  WireguardInterfaceSchema,
} from './vpn.js';
import {
  SECRET_KINDS,
  hostOrIpAddress,
  ipv4OrIpv6Cidr,
  secretRefOf,
  secretReference,
  transportPort,
  wireguardKey,
} from './_shared/primitives.js';

/** PEM banners are assembled at runtime so the source never matches the CI private-key scan (review F1). */
const PEM_PRIVATE = ['-----BEGIN', 'PRIVATE KEY-----'].join(' ');

const ok = (schema: z.ZodType, value: unknown): boolean => schema.safeParse(value).success;
/** Paths of all issues; Zod 4 reports unknown keys with an empty path and the keys in `issue.keys`. */
const errorPaths = (schema: z.ZodType, value: unknown): string[] => {
  const r = schema.safeParse(value);
  if (r.success) return [];
  return r.error.issues.flatMap((i) =>
    i.code === 'unrecognized_keys'
      ? i.keys.map((k) => [...i.path, k].join('.'))
      : [i.path.join('.')],
  );
};

describe('group-(c) shared primitives (domains/_shared, not re-exported — D-054)', () => {
  it.each([
    'psk/site-b',
    'key/wg0',
    'cert/vrx-a.2026',
    'password/alice',
    'token/acme-eab',
    `psk/${'A'.repeat(63)}`,
  ])('secretReference accepts %s', (v) => expect(ok(secretReference, v)).toBe(true));
  it.each([
    '',
    'psk/',
    '/x',
    'a',
    'site-b',
    'wg-key/wg0',
    'x509/a',
    'PSK/site-b',
    'abc+def=',
    'x y',
    PEM_PRIVATE,
    'psk/b/c',
    'MyS3cretPSK2026',
    'correct-horse-battery-staple',
    '0123456789abcdef0123456789abcdef',
    'a'.repeat(64),
    'AAECAwQFBgcICQoLDA0ODw==',
    `psk/${'a'.repeat(64)}`,
    'psk/site b',
    'psk/.hidden',
    'psk/ключ',
  ])('secretReference rejects %j (a pasted secret is never a reference — D-051)', (v) =>
    expect(ok(secretReference, v)).toBe(false),
  );
  it('secretRefOf pins the kind', () => {
    expect(SECRET_KINDS).toEqual(['psk', 'key', 'cert', 'password', 'token']);
    expect(ok(secretRefOf('psk'), 'psk/a')).toBe(true);
    expect(ok(secretRefOf('psk'), 'key/a')).toBe(false);
    expect(ok(secretRefOf(['cert', 'key']), 'key/a')).toBe(true);
    expect(ok(secretRefOf(['cert', 'key']), 'password/a')).toBe(false);
  });

  it.each([
    'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
    'xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=',
  ])('wireguardKey accepts %s', (v) => expect(ok(wireguardKey, v)).toBe(true));
  it.each([
    ['43 chars', 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8yk='],
    ['45 chars', 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykwa='],
    ['double padding', 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8yk=='],
    ['impossible last group for 32 bytes', 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykB='],
    ['url-safe alphabet', 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8yk-='],
    ['hex', 'a'.repeat(64)],
    ['empty', ''],
  ])('wireguardKey rejects %s', (_label, v) => expect(ok(wireguardKey, v)).toBe(false));

  it('transportPort is 1–65535', () => {
    expect([1, 53, 65535].every((p) => ok(transportPort, p))).toBe(true);
    expect([0, 65536, 1.5, -1, '53'].some((p) => ok(transportPort, p))).toBe(false);
  });
  it('hostOrIpAddress and ipv4OrIpv6Cidr', () => {
    expect(ok(hostOrIpAddress, 'time.cloudflare.com')).toBe(true);
    expect(ok(hostOrIpAddress, '2001:db8::1')).toBe(true);
    expect(ok(hostOrIpAddress, 'bad host')).toBe(false);
    // F12: an all-numeric "hostname" is a mistyped address
    expect(ok(hostOrIpAddress, '203.0.113.999')).toBe(false);
    expect(ok(hostOrIpAddress, '1.2.3')).toBe(false);
    expect(ok(hostOrIpAddress, '3com.example')).toBe(true);
    expect(ok(ipv4OrIpv6Cidr, '10.0.0.0/8')).toBe(true);
    expect(ok(ipv4OrIpv6Cidr, '::/0')).toBe(true);
    expect(ok(ipv4OrIpv6Cidr, '10.0.0.1')).toBe(false);
  });
});

describe('IPsec proposals', () => {
  it('requires an integrity algorithm for classic ciphers and forbids it for AEAD', () => {
    expect(ok(IkeProposalSchema, { encr: 'aes256', integ: 'sha256', dh: 'modp2048' })).toBe(true);
    expect(errorPaths(IkeProposalSchema, { encr: 'aes256', dh: 'modp2048' })).toEqual(['integ']);
    expect(
      errorPaths(IkeProposalSchema, { encr: 'aes256gcm16', integ: 'sha256', dh: 'ecp256' }),
    ).toEqual(['integ']);
    expect(
      ok(IkeProposalSchema, { encr: 'chacha20poly1305', dh: 'curve25519', prf: 'prfsha256' }),
    ).toBe(true);
  });
  it('ESP allows null encryption with integrity and optional PFS', () => {
    expect(ok(EspProposalSchema, { encr: 'null', integ: 'sha256' })).toBe(true);
    expect(ok(EspProposalSchema, { encr: 'null' })).toBe(false);
    expect(ok(EspProposalSchema, { encr: 'aes128gcm16', dh: 'modp3072' })).toBe(true);
    expect(ok(IkeProposalSchema, { encr: 'null', dh: 'modp2048', integ: 'sha1' })).toBe(false);
  });
  it('rejects unknown algorithm names and extra keys', () => {
    expect(ok(IkeProposalSchema, { encr: 'aes256cbc', integ: 'sha256', dh: 'modp2048' })).toBe(
      false,
    );
    expect(ok(IkeProposalSchema, { encr: 'aes256', integ: 'sha256', dh: 'group14' })).toBe(false);
    expect(
      ok(IkeProposalSchema, { encr: 'aes256', integ: 'sha256', dh: 'modp2048', psk: 'x' }),
    ).toBe(false);
  });
});

const tunnel = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  localAddr: '198.51.100.2',
  remoteAddr: '203.0.113.10',
  auth: { method: 'psk', secretRef: 'psk/site-b' },
  proposal: 'p',
  localTs: ['192.168.10.0/24'],
  remoteTs: ['10.99.0.0/16'],
  ...extra,
});

describe('IpsecTunnelSchema', () => {
  it('accepts a policy-based PSK tunnel and fills defaults', () => {
    const t = IpsecTunnelSchema.parse(tunnel());
    expect(t).toMatchObject({
      enabled: true,
      engine: 'strongswan',
      ikeVersion: 2,
      mode: 'tunnel',
      protocol: 'esp',
      natT: true,
      startAction: 'start',
      closeAction: 'none',
      vrf: 'default',
      underlayVrf: 'default',
      esn: false,
      antiReplay: true,
      dpd: { enabled: true, delaySec: 30, timeoutSec: 150, action: 'restart' },
      rekey: { ikeSec: 14400, espSec: 3600, reauth: false },
    });
  });
  it('accepts a route-based tunnel without traffic selectors, %any responder and hostname peers', () => {
    expect(
      ok(
        IpsecTunnelSchema,
        tunnel({ localTs: [], remoteTs: [], routeBased: { ipipInterface: 'ipip0' } }),
      ),
    ).toBe(true);
    expect(ok(IpsecTunnelSchema, tunnel({ remoteAddr: '%any', startAction: 'trap' }))).toBe(true);
    expect(ok(IpsecTunnelSchema, tunnel({ remoteAddr: 'vpn.peer.example' }))).toBe(true);
    expect(
      ok(IpsecTunnelSchema, tunnel({ localAddr: '2001:db8::2', remoteAddr: '2001:db8:1::2' })),
    ).toBe(true);
  });
  it.each([
    ['inline PSK', tunnel({ auth: { method: 'psk', psk: 'VRX_TEST_PSK_x' } }), 'auth.psk'],
    ['PSK without secretRef', tunnel({ auth: { method: 'psk' } }), 'auth.secretRef'],
    [
      'PEM in secretRef',
      tunnel({ auth: { method: 'psk', secretRef: ['-----BEGIN', 'PSK-----'].join(' ') } }),
      'auth.secretRef',
    ],
    [
      'PSK pasted as reference',
      tunnel({ auth: { method: 'psk', secretRef: 'VRX_TEST_PSK_P02c' } }),
      'auth.secretRef',
    ],
    [
      'reference of the wrong kind',
      tunnel({ auth: { method: 'psk', secretRef: 'key/site-b' } }),
      'auth.secretRef',
    ],
    ['mistyped IPv4 peer (F12)', tunnel({ remoteAddr: '203.0.113.999' }), 'remoteAddr'],
    ['unknown top-level key', tunnel({ password: 'x' }), 'password'],
    ['vpp-ikev2 engine with IKEv1', tunnel({ engine: 'vpp-ikev2', ikeVersion: 1 }), 'ikeVersion'],
    ['MOBIKE with IKEv1', tunnel({ ikeVersion: 1, mobike: true }), 'mobike'],
    [
      'route-based in transport mode',
      tunnel({ mode: 'transport', routeBased: { ipipInterface: 'ipip0' } }),
      'routeBased',
    ],
    ['policy-based without selectors', tunnel({ localTs: [], remoteTs: [] }), 'localTs'],
    ['policy-based with only remote selectors', tunnel({ localTs: [] }), 'localTs'],
    ['%any cannot be started', tunnel({ remoteAddr: '%any' }), 'startAction'],
    ['remote equals local', tunnel({ remoteAddr: '198.51.100.2' }), 'remoteAddr'],
    ['mixed families', tunnel({ remoteAddr: '2001:db8::1' }), 'remoteAddr'],
    ['ikeVersion 3', tunnel({ ikeVersion: 3 }), 'ikeVersion'],
    ['proposal name with spaces', tunnel({ proposal: 'a b' }), 'proposal'],
    ['bad traffic selector', tunnel({ localTs: ['192.168.10.0'] }), 'localTs.0'],
    ['dpd delay 0', tunnel({ dpd: { delaySec: 0 } }), 'dpd.delaySec'],
    ['rekey shorter than 60 s', tunnel({ rekey: { espSec: 30 } }), 'rekey.espSec'],
    ['identity with double quote', tunnel({ localId: 'C=CH, CN="x"' }), 'localId'],
    ['cert auth without certificate', tunnel({ auth: { method: 'cert' } }), 'auth.certificate'],
  ])('rejects %s', (_label, value, path) => {
    expect(errorPaths(IpsecTunnelSchema, value)).toContain(path);
  });
});

const wg = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
  instance: 0,
  listenAddress: '198.51.100.2',
  privateKeyRef: 'key/wg0',
  peers: {
    b1: {
      publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
      allowedIps: ['10.200.0.2/32'],
    },
  },
  ...extra,
});

describe('WireguardInterfaceSchema', () => {
  it('fills defaults', () => {
    expect(WireguardInterfaceSchema.parse(wg())).toMatchObject({
      enabled: true,
      vrf: 'default',
      underlayVrf: 'default',
      listenPort: 51820,
      mtu: 1420,
      address: [],
      peers: { b1: { persistentKeepaliveSec: 0 } },
    });
  });
  it.each([
    ['inline private key', wg({ privateKey: 'VRX_TEST_PSK_P02c_wg' }), 'privateKey'],
    ['missing private key reference', { ...wg(), privateKeyRef: undefined }, 'privateKeyRef'],
    [
      'inline preshared key on a peer',
      wg({
        peers: {
          b1: {
            publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
            allowedIps: ['10.0.0.0/8'],
            presharedKey: 'x',
          },
        },
      }),
      'peers.b1.presharedKey',
    ],
    [
      'peer without allowed IPs',
      wg({
        peers: {
          b1: { publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=', allowedIps: [] },
        },
      }),
      'peers.b1.allowedIps',
    ],
    [
      'endpoint port 0',
      wg({
        peers: {
          b1: {
            publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
            allowedIps: ['10.0.0.0/8'],
            endpoint: { address: '203.0.113.1', port: 0 },
          },
        },
      }),
      'peers.b1.endpoint.port',
    ],
    ['negative instance', wg({ instance: -1 }), 'instance'],
    ['mtu above 9216', wg({ mtu: 9217 }), 'mtu'],
    [
      'keepalive above 65535',
      wg({
        peers: {
          b1: {
            publicKey: 'HIgo9xNzJMWLKASShiTqIybxZ0U3wGLiUeJ1PKf8ykw=',
            allowedIps: ['10.0.0.0/8'],
            persistentKeepaliveSec: 65536,
          },
        },
      }),
      'peers.b1.persistentKeepaliveSec',
    ],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(WireguardInterfaceSchema, value)).toContain(path),
  );
});

describe('PKI and remote access', () => {
  it('a certificate needs certificateRef or acme; private keys are references', () => {
    expect(errorPaths(PkiCertificateSchema, { privateKeyRef: 'key/a' })).toEqual([
      'certificateRef',
    ]);
    expect(ok(PkiCertificateSchema, { privateKeyRef: 'key/a', certificateRef: 'cert/a' })).toBe(
      true,
    );
    expect(
      ok(PkiCertificateSchema, { privateKeyRef: 'key/a', acme: { domains: ['vpn.example'] } }),
    ).toBe(true);
    expect(
      ok(PkiCertificateSchema, {
        privateKey: PEM_PRIVATE,
        certificateRef: 'cert/a',
      }),
    ).toBe(false);
    expect(ok(PkiCertificateSchema, { privateKeyRef: 'key/a', acme: { domains: [] } })).toBe(false);
  });
  const profile = (extra: Record<string, unknown> = {}): Record<string, unknown> => ({
    localAddr: '198.51.100.2',
    certificate: 'vrx-a',
    proposal: 'p',
    pools: [{ name: 'p4', prefix: '10.250.0.0/24' }],
    users: [{ username: 'alice', passwordRef: 'password/alice' }],
    ...extra,
  });
  it('accepts a local-user profile with defaults', () => {
    expect(RemoteAccessProfileSchema.parse(profile())).toMatchObject({
      auth: 'eap-mschapv2',
      vrf: 'default',
      underlayVrf: 'default',
      splitTunnel: [],
    });
  });
  it.each([
    ['eap-radius without RADIUS servers', profile({ auth: 'eap-radius' }), 'radius'],
    ['eap-mschapv2 without users or RADIUS', profile({ users: [] }), 'users'],
    ['eap-tls without client CA', profile({ auth: 'eap-tls', users: [] }), 'clientCa'],
    [
      'duplicate usernames',
      profile({
        users: [
          { username: 'a', passwordRef: 'password/a' },
          { username: 'a', passwordRef: 'password/a2' },
        ],
      }),
      'users.1.username',
    ],
    [
      'duplicate pool names',
      profile({
        pools: [
          { name: 'p', prefix: '10.0.0.0/24' },
          { name: 'p', prefix: '10.1.0.0/24' },
        ],
      }),
      'pools.1.name',
    ],
    [
      'inline password',
      profile({ users: [{ username: 'a', password: 'hunter2' }] }),
      'users.0.password',
    ],
    [
      'RADIUS secret inline',
      profile({ auth: 'eap-radius', radius: { servers: [{ address: '10.0.0.1', secret: 'x' }] } }),
      'radius.servers.0.secret',
    ],
    ['no pools', profile({ pools: [] }), 'pools'],
  ])('rejects %s', (_label, value, path) =>
    expect(errorPaths(RemoteAccessProfileSchema, value)).toContain(path),
  );
});

describe('VpnSchema root', () => {
  it('parses {} to a fully populated root (prefault, D-017/D-053)', () => {
    expect(VpnSchema.parse({})).toEqual({
      ipsec: { settings: { cryptoEngine: 'auto', asyncCrypto: false }, proposals: {}, tunnels: {} },
      wireguard: { interfaces: {} },
      pki: { cas: {}, certificates: {} },
      remoteAccess: {},
    });
  });
  it('rejects unknown keys and bad record keys', () => {
    expect(ok(VpnSchema, { openvpn: {} })).toBe(false);
    expect(
      ok(VpnSchema, {
        ipsec: {
          proposals: {
            'bad name': {
              ike: { encr: 'aes256gcm16', dh: 'ecp256' },
              esp: { encr: 'aes256gcm16' },
            },
          },
        },
      }),
    ).toBe(false);
    expect(ok(VpnSchema, { ipsec: null })).toBe(false);
  });
});
