import { describe, expect, it } from 'vitest';
import { z } from 'zod';
import {
  hostname,
  ipAddress,
  ipv4Cidr,
  ipv6Cidr,
  macAddress,
  objectName,
  vppInterfaceName,
} from './primitives.js';

// Minimal scaffold — TODO(P02a): exhaustive nasty-input tables and 100% branch coverage.
const cases: [string, z.ZodType, string[], string[]][] = [
  [
    'ipv4Cidr',
    ipv4Cidr,
    ['10.0.0.1/24', '0.0.0.0/0', '192.168.1.0/32'],
    ['10.0.0.1', '10.0.0.1/33', '256.0.0.0/8', ''],
  ],
  [
    'ipv6Cidr',
    ipv6Cidr,
    ['2001:db8::1/64', '::/0'],
    ['2001:db8::1', '2001:db8::1/129', '10.0.0.1/24'],
  ],
  [
    'ipAddress',
    ipAddress,
    ['10.0.0.1', '2001:db8::1', '::1'],
    ['10.0.0.1/24', '10.0.0', 'fe80::1%eth0', 'localhost'],
  ],
  [
    'macAddress',
    macAddress,
    ['aa:bb:cc:dd:ee:ff', 'AA-BB-CC-DD-EE-FF'],
    ['aa:bb:cc:dd:ee', 'aa:bb:cc:dd:ee:ff:00', 'aa:bb-cc:dd:ee:ff', 'zz:bb:cc:dd:ee:ff'],
  ],
  [
    'vppInterfaceName',
    vppInterfaceName,
    [
      'TenGigabitEthernet0/0/0',
      'GigabitEthernet0/8/0.100',
      'loop0',
      'host-w1-eth0',
      'memif0/0',
      'vxlan_tunnel0',
      'local0',
    ],
    ['', '0abc', 'Gig 0/0/0', 'Gig0//0', 'Gig0/0/', 'a'.repeat(64), '../etc'],
  ],
  [
    'hostname',
    hostname,
    ['vrx-a', 'vrx-a.lab.example', 'a', '1router'],
    ['', '-vrx', 'vrx-', 'vrx_a', 'a..b', 'a'.repeat(64), `${'a'.repeat(63)}.`.repeat(4)],
  ],
  [
    'objectName',
    objectName,
    ['default', 'customer-a', 'web_servers.v2'],
    ['', '-x', 'a b', 'a/b', 'a'.repeat(64)],
  ],
];

describe.each(cases)('%s', (_name, schema, valid, invalid) => {
  it.each(valid)('accepts %s', (v) => expect(schema.safeParse(v).success).toBe(true));
  it.each(invalid)('rejects %j', (v) => expect(schema.safeParse(v).success).toBe(false));
  it('emits a title for the form renderer', () =>
    expect(z.toJSONSchema(schema).title).toBeTruthy());
});
