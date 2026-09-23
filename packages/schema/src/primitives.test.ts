import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  asNumber,
  cpuCore,
  descriptionText,
  hostname,
  hostOrIp,
  ipAddress,
  ipCidr,
  ipNetwork,
  ipv4Address,
  ipv4Cidr,
  ipv4Network,
  ipv6Address,
  ipv6Cidr,
  ipv6Network,
  isKnownTimeZone,
  macAddress,
  multilineText,
  secretRefOf,
  mtu,
  objectName,
  passwordHash,
  pciAddress,
  portNumber,
  routerId,
  secretRef,
  timezone,
  uint32,
  username,
  vlanId,
  vppInterfaceName,
  vrfName,
} from './primitives.js';

const cp = (...points: number[]): string => String.fromCodePoint(...points);
const NUL = cp(0);
const ZWSP = cp(0x200b); // zero-width space
const RLO = cp(0x202e); // right-to-left override
const ARABIC_INDIC_TEN = cp(0x661, 0x660); // "10" in Arabic-Indic digits
const FULLWIDTH_TEN = cp(0xff54, 0xff45, 0xff4e); // "ten" in full-width Latin
const UMLAUT_U = cp(0xfc);
const CHECK_MARK = cp(0x2713);
// eslint-disable-next-line no-control-regex -- the test deliberately looks for control characters
const CONTROL_CHARS = /[\u0000-\u0008\u000a-\u001f]/;

/**
 * Inputs every string primitive must reject, whatever it models: empty, whitespace, control characters, path
 * traversal, shell metacharacters, Unicode look-alikes (Arabic-Indic digits, full-width letters, zero-width space,
 * bidi override), null bytes, huge strings, non-strings.
 */
const NASTY = [
  '',
  ' ',
  '\t',
  '\n',
  NUL,
  '../etc/passwd',
  '$(reboot)',
  '`id`',
  '; rm -rf /',
  `${ARABIC_INDIC_TEN}.0.0.1`,
  FULLWIDTH_TEN,
  `10.0.0.1${ZWSP}`,
  `${RLO}1.0.0.01`,
  'x'.repeat(10_000),
];
const NON_STRINGS = [null, undefined, 1, true, {}, [], 1n, Symbol('s'), () => 'x'];

const LABEL63 = 'a'.repeat(63);
const HOST253 = `${LABEL63}.${LABEL63}.${LABEL63}.${'a'.repeat(61)}`; // 63+1+63+1+63+1+61 = 253

const stringCases: [string, z.ZodType, string[], string[]][] = [
  [
    'ipv4Address',
    ipv4Address,
    ['0.0.0.0', '10.0.0.1', '255.255.255.255', '192.168.100.200'],
    [
      '10.0.0',
      '10.0.0.1.1',
      '256.0.0.1',
      '01.2.3.4',
      '1.2.3.-4',
      '10.0.0.1/24',
      '10.0.0.1 ',
      ' 10.0.0.1',
      '0x0a.0.0.1',
      '10.0.0.1%eth0',
      '::1',
    ],
  ],
  [
    'ipv6Address',
    ipv6Address,
    [
      '::',
      '::1',
      '2001:db8::1',
      '2001:DB8:0:0:0:0:0:1',
      'fe80::1',
      '::ffff:10.0.0.1',
      '1:2:3:4:5:6:7:8',
      '1:2:3:4:5:6:7::',
    ],
    [
      ':',
      ':::',
      '1::2::3',
      '1:2:3:4:5:6:7',
      '1:2:3:4:5:6:7:8:9',
      '12345::',
      'g::1',
      'fe80::1%eth0',
      '2001:db8::1/64',
      '10.0.0.1',
      '[::1]',
    ],
  ],
  [
    'ipAddress',
    ipAddress,
    ['10.0.0.1', '2001:db8::1', '::1'],
    ['10.0.0.1/24', '10.0.0', 'fe80::1%eth0', 'localhost', '1::2::3'],
  ],
  [
    'ipv4Cidr',
    ipv4Cidr,
    ['10.0.0.1/24', '0.0.0.0/0', '192.168.1.0/32', '255.255.255.255/32'],
    [
      '10.0.0.1',
      '10.0.0.1/33',
      '10.0.0.1/-1',
      '10.0.0.1/08',
      '10.0.0.1/1.5',
      '256.0.0.0/8',
      '010.0.0.1/24',
      '10.0.0.1/24/',
      '10.0.0.1 /24',
      '2001:db8::/32',
    ],
  ],
  [
    'ipv6Cidr',
    ipv6Cidr,
    ['2001:db8::1/64', '::/0', '::1/128', 'fe80::/10'],
    [
      '2001:db8::1',
      '2001:db8::1/129',
      '2001:db8::1/064',
      '10.0.0.1/24',
      '2001:db8::1%eth0/64',
      '2001:db8::1/',
    ],
  ],
  [
    'ipCidr',
    ipCidr,
    ['10.0.0.1/24', '2001:db8::1/64'],
    ['10.0.0.1', '2001:db8::1', '10.0.0.1/33', '::/129'],
  ],
  [
    'ipv4Network',
    ipv4Network,
    ['10.0.0.0/24', '0.0.0.0/0', '10.0.0.1/32', '10.0.0.128/25', '128.0.0.0/1'],
    ['10.0.0.1/24', '10.0.0.128/24', '128.0.0.0/0', '10.0.0.0/33', '10.0.0.0', '2001:db8::/32'],
  ],
  [
    'ipv6Network',
    ipv6Network,
    ['2001:db8::/64', '::/0', '2001:db8::1/128', '2001:db8:8000::/33'],
    ['2001:db8::1/64', '2001:db8:8000::/32', '8000::/0', '2001:db8::/129', '10.0.0.0/8'],
  ],
  [
    'ipNetwork',
    ipNetwork,
    ['10.0.0.0/8', '2001:db8::/32'],
    ['10.0.0.1/8', '2001:db8::1/32', '10.0.0.0'],
  ],
  [
    'macAddress',
    macAddress,
    ['aa:bb:cc:dd:ee:ff', 'AA-BB-CC-DD-EE-FF', '02:00:00:00:00:01', 'fe:ff:ff:ff:ff:ff'],
    [
      'aa:bb:cc:dd:ee',
      'aa:bb:cc:dd:ee:ff:00',
      'aa:bb-cc:dd:ee:ff',
      'zz:bb:cc:dd:ee:ff',
      'aabb.ccdd.eeff',
      'aabbccddeeff',
      '01:00:5e:00:00:01',
      'ff:ff:ff:ff:ff:ff',
      '00:00:00:00:00:00',
      '00-00-00-00-00-00',
      'aa:bb:cc:dd:ee:f',
    ],
  ],
  [
    'vppInterfaceName',
    vppInterfaceName,
    [
      'TenGigabitEthernet0/0/0',
      'GigabitEthernet0/8/0.100',
      'vmxnet3-0/b/0/0',
      'loop0',
      'BondEthernet0',
      'host-w1-eth0',
      'memif0/0',
      'vxlan_tunnel0',
      'ipsec0',
      'local0',
      'a'.repeat(63),
    ],
    [
      '0abc',
      'Gig 0/0/0',
      'Gig0//0',
      'Gig0/0/',
      '/loop0',
      'loop0.',
      'loop0.1.2',
      'loop0.a',
      'loop0/g',
      'a'.repeat(64),
      'Ten0/0/0 ',
      '-loop0',
    ],
  ],
  [
    'pciAddress',
    pciAddress,
    ['0000:0b:00.0', '0000:13:00.1', 'ffff:ff:1f.7', '0000:0B:00.0'],
    [
      '0b:00.0',
      '0000:0b:00',
      '0000:0b:00.8',
      '00000:0b:00.0',
      '0000:0b:00.0 ',
      '0000-0b-00.0',
      'g000:0b:00.0',
    ],
  ],
  [
    'hostname',
    hostname,
    [
      'vrx-a',
      'vrx-a.lab.example',
      'a',
      '1router',
      'a.b.c.d',
      LABEL63,
      HOST253,
      'xn--mgbh0fb.example',
      '1.a',
    ],
    [
      '-vrx',
      'vrx-',
      'vrx_a',
      'a..b',
      '.a',
      'a.',
      'a'.repeat(64),
      `${HOST253}a`,
      '123',
      'a.123',
      'vrx a',
      'vrx.a-',
      `${UMLAUT_U}n`,
      'a/b',
      'a:b',
    ],
  ],
  [
    'hostOrIp',
    hostOrIp,
    ['10.0.0.1', '2001:db8::1', 'pool.ntp.org'],
    ['10.0.0.1/24', 'a..b', '-x', 'fe80::1%eth0'],
  ],
  [
    'objectName',
    objectName,
    ['default', 'customer-a', 'web_servers.v2', '1', 'A.b-C_d', 'a'.repeat(63)],
    ['-x', '_x', '.x', 'a b', 'a/b', 'a:b', 'a*b', 'a'.repeat(64), `${UMLAUT_U}n`],
  ],
  ['vrfName', vrfName, ['default', 'customer-a', 'mgmt'], ['-x', 'a b', 'a/b', 'a'.repeat(64)]],
  [
    'username',
    username,
    ['admin', 'noc', '_svc', 'user-1', 'a'.repeat(32), 'ops_team'],
    [
      'Admin',
      '1user',
      '-user',
      'a'.repeat(33),
      'root:0',
      'a b',
      'user@host',
      `${UMLAUT_U}n`,
      'user.name',
    ],
  ],
  [
    'descriptionText',
    descriptionText,
    [
      '',
      ' ',
      'uplink to ISP',
      'x'.repeat(255),
      `unicode ${CHECK_MARK} ok`,
      'tab\tok',
      '../not/a/path $(id)',
    ],
    [
      'x'.repeat(256),
      'two\nlines',
      'cr\rhere',
      NUL,
      `esc${cp(0x1b)}[31m`,
      `del${cp(0x7f)}`,
      `c1${cp(0x85)}`,
    ],
  ],
  [
    'routerId',
    routerId,
    ['10.255.0.1', '0.0.0.0'],
    ['10.255.0', '2001:db8::1', '256.0.0.1', '10.255.0.1/32'],
  ],
  [
    'secretRef',
    secretRef,
    [
      'psk/site-a',
      'psk/radius-primary',
      'cert/api',
      'key/api.2026',
      'password/bgp_upstream',
      'token/enrol',
      `psk/${'a'.repeat(63)}`,
    ],
    [
      'ipsec/psk/site-a',
      'aaa/radius/primary',
      'psk/a/b',
      'psk/',
      '/psk',
      'psk/-x',
      'psk/a b',
      `psk/${'a'.repeat(64)}`,
      'PSK/x',
      'hunter2',
      'psk=hunter2',
      '-----BEGIN PRIVATE KEY-----',
      'psk/x\n',
    ],
  ],
  [
    'passwordHash',
    passwordHash,
    [
      '$6$rounds=5000$saltsalt$abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789./abcdefghijklmnopqrstuv',
      '$argon2id$v=19$m=65536,t=3,p=4$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA',
      '$2b$12$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ012345',
      '$vrx-test$VRX_TEST_HASH_admin',
    ],
    [
      'plaintext',
      'hunter2',
      '$',
      '$6$',
      '$$abc',
      '$6 $abc',
      `$6$${'a'.repeat(512)}`,
      '$6$abc def',
      '$6$abc\nabc',
      '$argon2id$v=19$m=65536,t=3,p=4$salt$hash;',
      `$${UMLAUT_U}$abc`,
    ],
  ],
  [
    'timezone',
    timezone,
    [
      'UTC',
      'Asia/Tehran',
      'Europe/Berlin',
      'America/Argentina/Buenos_Aires',
      'Etc/GMT+3',
      'Etc/UTC',
    ],
    [
      'utc',
      'asia/tehran',
      'Mars/Olympus',
      'Asia/tehran',
      'Asia//Tehran',
      'Asia/Tehran/',
      '/UTC',
      'UTC+3',
      'GMT+03:00',
      `Asia/Tehran${ZWSP}`,
      'a'.repeat(65),
    ],
  ],
];

describe.each(stringCases)('%s', (_name, schema, valid, invalid) => {
  it.each(valid)('accepts %j', (v) => expect(schema.safeParse(v).error?.issues).toBeUndefined());
  it.each(invalid)('rejects %j', (v) => expect(schema.safeParse(v).success).toBe(false));
  it('rejects nasty strings', () => {
    for (const v of NASTY) {
      // free text (descriptionText) legitimately accepts spaces, shell text and Unicode — its control-character
      // and length limits are covered by its own invalid list above
      if (schema === descriptionText && !CONTROL_CHARS.test(v) && v.length <= 255) continue;
      expect(schema.safeParse(v).success, JSON.stringify(v)).toBe(false);
    }
  });
  it('rejects non-strings', () => {
    for (const v of NON_STRINGS) expect(schema.safeParse(v).success).toBe(false);
  });
  it('emits a title for the form renderer', () =>
    expect(z.toJSONSchema(schema).title).toBeTruthy());
});

const numberCases: [string, z.ZodType, number[], unknown[]][] = [
  [
    'vlanId',
    vlanId,
    [1, 100, 4094],
    [0, 4095, -1, 1.5, Number.NaN, Number.POSITIVE_INFINITY, '100', null],
  ],
  ['mtu', mtu, [68, 1500, 9000, 9216], [67, 9217, 0, 1500.5, '1500']],
  ['portNumber', portNumber, [1, 22, 65535], [0, 65536, -22, 22.5, '22']],
  ['asNumber', asNumber, [1, 65000, 4200000000, 4294967295], [0, 4294967296, -1, 1.5, '65000']],
  ['uint32', uint32, [0, 1, 4294967295], [-1, 4294967296, 0.5, Number.NaN, '0', null]],
  ['cpuCore', cpuCore, [0, 1, 1023], [-1, 1024, 0.5, '1']],
];

describe.each(numberCases)('%s', (_name, schema, valid, invalid) => {
  it.each(valid)('accepts %j', (v) => expect(schema.safeParse(v).success).toBe(true));
  it.each(invalid)('rejects %j', (v) => expect(schema.safeParse(v).success).toBe(false));
  it('emits an integer schema with bounds and a title', () => {
    const js = z.toJSONSchema(schema);
    expect(js).toMatchObject({
      type: 'integer',
      minimum: expect.any(Number),
      maximum: expect.any(Number),
    });
    expect(js.title).toBeTruthy();
  });
});

describe('primitive JSON Schema output', () => {
  it('uses portable patterns (no lookaround) so Go/Python consumers can reuse them', () => {
    for (const [name, schema] of stringCases) {
      const js = JSON.stringify(z.toJSONSchema(schema));
      expect(js, name).not.toMatch(/\(\?<?[=!]/);
    }
  });
  it('marks the password hash write-only and CIDRs with the cidr widget', () => {
    expect(z.toJSONSchema(passwordHash)).toMatchObject({
      writeOnly: true,
      'x-vrx-ui': { secret: true, widget: 'password' },
    });
    expect(z.toJSONSchema(ipv4Cidr)).toMatchObject({
      format: 'cidrv4',
      'x-vrx-ui': { widget: 'cidr' },
    });
    expect(z.toJSONSchema(ipNetwork)).toMatchObject({
      anyOf: [expect.anything(), expect.anything()],
      'x-vrx-ui': { widget: 'cidr' },
    });
  });
});

describe('isKnownTimeZone', () => {
  it('asks the runtime ICU data', () => {
    expect(isKnownTimeZone('Asia/Tehran')).toBe(true);
    expect(isKnownTimeZone('Mars/Olympus')).toBe(false);
    expect(isKnownTimeZone('')).toBe(false);
  });
});

describe('secretRefOf (D-051)', () => {
  it('restricts the kind when asked', () => {
    const password = secretRefOf('password');
    expect(password.safeParse('password/bgp-upstream').success).toBe(true);
    expect(password.safeParse('psk/bgp-upstream').success).toBe(false);
    const tls = secretRefOf(['cert', 'key']);
    expect(tls.safeParse('cert/api').success).toBe(true);
    expect(tls.safeParse('key/api').success).toBe(true);
    expect(tls.safeParse('token/api').success).toBe(false);
  });
});

describe('multilineText (D-049)', () => {
  const banner = multilineText(20);
  it('accepts printable text with LF and TAB', () => {
    for (const ok of ['', 'Authorised\nonly', 'a\tb', `${UMLAUT_U}${CHECK_MARK}`])
      expect(banner.safeParse(ok).success).toBe(true);
  });
  it('rejects CR, ESC, BEL, NUL, DEL, C1 controls and overlong text', () => {
    for (const bad of ['a\r\nb', `x${cp(0x1b)}[2J`, `${cp(7)}`, NUL, cp(0x7f), cp(0x9b), 'x'.repeat(21)])
      expect(banner.safeParse(bad).success).toBe(false);
  });
});
