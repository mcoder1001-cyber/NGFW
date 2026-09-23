import { describe, expect, it } from 'vitest';
import {
  addressConfiguredInVrf,
  cidrContains,
  cidrContainsIp,
  cidrsOverlap,
  compareIp,
  interfaceIndex,
  ipFamily,
  isMulticast,
  isUnspecified,
  knownVrfs,
  parseCidr,
  parseIp,
  prefixesOfInterfaces,
  recordKeys,
  sameCidr,
  walkStrings,
} from './tunnels-common.js';

const cidr = (text: string) => {
  const c = parseCidr(text);
  if (c === undefined) throw new Error(`bad test cidr ${text}`);
  return c;
};
const ip = (text: string) => {
  const v = parseIp(text);
  if (v === undefined) throw new Error(`bad test ip ${text}`);
  return v;
};

describe('IP math', () => {
  it('parseIp handles IPv4, IPv6 (compressed, embedded IPv4) and rejects garbage', () => {
    expect(parseIp('10.0.0.1')).toBe(0x0a000001n);
    expect(parseIp('255.255.255.255')).toBe(0xffffffffn);
    expect(parseIp('::1')).toBe(1n);
    expect(parseIp('::')).toBe(0n);
    expect(parseIp('2001:db8::')).toBe(0x20010db8n << 96n);
    expect(parseIp('2001:0db8:0000:0000:0000:0000:0000:0001')).toBe((0x20010db8n << 96n) | 1n);
    expect(parseIp('::ffff:192.0.2.1')).toBe(0xffffc0000201n);
    expect(parseIp('1:2:3:4:5:6:7:8')).toBe(0x00010002000300040005000600070008n);
    for (const bad of [
      '256.1.1.1',
      '1.2.3',
      '1.2.3.4.5',
      'abc',
      '01a.1.1.1',
      '1::2::3',
      'fe80::1%eth0',
      '1:2:3:4:5:6:7:8:9',
      '1:2:3:4:5:6:7',
      'g::1',
      ':::',
      '::ffff:999.0.2.1',
      '12345::1',
    ]) {
      expect(parseIp(bad), bad).toBeUndefined();
    }
  });
  it('ipFamily by syntax', () => {
    expect(ipFamily('10.0.0.1')).toBe(4);
    expect(ipFamily('::ffff:10.0.0.1')).toBe(6);
  });
  it('parseCidr keeps host bits and computes the range', () => {
    expect(parseCidr('10.0.0.1/24')).toEqual({
      family: 4,
      address: 0x0a000001n,
      prefixLength: 24,
      first: 0x0a000000n,
      last: 0x0a0000ffn,
    });
    expect(parseCidr('0.0.0.0/0')).toMatchObject({ first: 0n, last: 0xffffffffn });
    expect(parseCidr('::/0')).toMatchObject({ family: 6, first: 0n, last: (1n << 128n) - 1n });
    expect(parseCidr('2001:db8::1/128')).toMatchObject({
      first: (0x20010db8n << 96n) | 1n,
      last: (0x20010db8n << 96n) | 1n,
    });
    for (const bad of [
      '10.0.0.1',
      '10.0.0.1/33',
      '2001:db8::/129',
      'x/24',
      '10.0.0.0/2a',
      '10.0.0.0/',
      '/24',
      '10.0.0.0/-1',
    ]) {
      expect(parseCidr(bad), bad).toBeUndefined();
    }
  });
  it('containment, overlap and equality', () => {
    const a = cidr('10.0.0.0/8');
    const b = cidr('10.1.0.0/16');
    const c = cidr('192.168.0.0/16');
    const v6 = cidr('2001:db8::/32');
    expect(cidrContains(a, b)).toBe(true);
    expect(cidrContains(b, a)).toBe(false);
    expect(cidrContains(a, c)).toBe(false);
    expect(cidrContains(a, v6)).toBe(false);
    expect(cidrsOverlap(a, b)).toBe(true);
    expect(cidrsOverlap(b, a)).toBe(true);
    expect(cidrsOverlap(a, c)).toBe(false);
    expect(cidrsOverlap(a, v6)).toBe(false);
    expect(cidrsOverlap(cidr('10.0.0.0/25'), cidr('10.0.0.128/25'))).toBe(false);
    expect(sameCidr(cidr('10.0.0.1/24'), cidr('10.0.0.2/24'))).toBe(true);
    expect(sameCidr(cidr('10.0.0.1/24'), cidr('10.0.0.1/25'))).toBe(false);
    expect(sameCidr(a, v6)).toBe(false);
    expect(cidrContainsIp(a, 4, ip('10.255.255.255'))).toBe(true);
    expect(cidrContainsIp(a, 4, ip('11.0.0.0'))).toBe(false);
    expect(cidrContainsIp(a, 6, 0n)).toBe(false);
  });
  it('multicast, unspecified, compare', () => {
    expect(isMulticast('224.0.0.1')).toBe(true);
    expect(isMulticast('239.255.255.255')).toBe(true);
    expect(isMulticast('223.255.255.255')).toBe(false);
    expect(isMulticast('240.0.0.0')).toBe(false);
    expect(isMulticast('ff02::1')).toBe(true);
    expect(isMulticast('fe80::1')).toBe(false);
    expect(isMulticast('bad')).toBe(false);
    expect(isUnspecified('0.0.0.0')).toBe(true);
    expect(isUnspecified('::')).toBe(true);
    expect(isUnspecified('0.0.0.1')).toBe(false);
    expect(isUnspecified('bad')).toBe(false);
    expect(compareIp('10.0.0.1', '10.0.0.2')).toBe(-1);
    expect(compareIp('10.0.0.2', '10.0.0.1')).toBe(1);
    expect(compareIp('::1', '0::1')).toBe(0);
    expect(compareIp('10.0.0.1', '::1')).toBeUndefined();
    expect(compareIp('bad', '::1')).toBeUndefined();
  });
});

const doc = {
  vrfs: { default: { id: 0 }, 'customer-a': { id: 10 } },
  interfaces: {
    'TenGigabitEthernet0/0/0': {
      ipv4: ['198.51.100.2/30'],
      ipv6: ['2001:db8::2/64'],
      vrf: 'default',
    },
    'TenGigabitEthernet0/0/1': {
      ipv4: ['192.168.10.1/24'],
      subinterfaces: {
        '100': { vlanId: 100, ipv4: ['192.168.100.1/24'], vrf: 'customer-a' },
        inherit: { ipv4: ['192.168.101.1/24'] },
      },
    },
    loop0: { vrf: 'customer-a' },
    junk: 'not an object',
  },
  tunnels: {
    gre: {
      g: { instance: 3, src: '198.51.100.2', dst: '203.0.113.1', ipv4: ['10.254.0.1/30'] },
      noInstance: { src: '1.1.1.1', dst: '2.2.2.2' },
    },
    vxlan: { v: { instance: 1, src: '1.1.1.1', dst: '2.2.2.2', vni: 1, vrf: 'customer-a' } },
  },
  vpn: { wireguard: { interfaces: { w: { instance: 0, address: ['10.200.0.1/24'] } } } },
};

describe('document lookups (duck-typed)', () => {
  it('knownVrfs always includes default', () => {
    expect([...knownVrfs(doc)].sort()).toEqual(['customer-a', 'default']);
    expect([...knownVrfs({})]).toEqual(['default']);
    expect([...knownVrfs(null)]).toEqual(['default']);
  });
  it('interfaceIndex covers interfaces, sub-interfaces, instanced tunnels and WireGuard', () => {
    const index = interfaceIndex(doc);
    expect([...index.keys()].sort()).toEqual([
      'TenGigabitEthernet0/0/0',
      'TenGigabitEthernet0/0/1',
      'TenGigabitEthernet0/0/1.100',
      'TenGigabitEthernet0/0/1.inherit',
      'gre3',
      'junk',
      'loop0',
      'vxlan_tunnel1',
      'wg0',
    ]);
    expect(index.get('TenGigabitEthernet0/0/1')).toMatchObject({ vrf: 'default' });
    expect(index.get('TenGigabitEthernet0/0/1.100')).toMatchObject({ vrf: 'customer-a' });
    expect(index.get('TenGigabitEthernet0/0/1.inherit')).toMatchObject({ vrf: 'default' });
    expect(index.get('vxlan_tunnel1')).toMatchObject({ vrf: 'customer-a', addresses: [] });
    expect(index.get('wg0')?.addresses).toHaveLength(1);
    expect(index.get('junk')).toMatchObject({ vrf: 'default', addresses: [] });
    expect(interfaceIndex({}).size).toBe(0);
    expect(interfaceIndex(undefined).size).toBe(0);
  });
  it('addressConfiguredInVrf matches the address part only, per VRF and family', () => {
    const index = interfaceIndex(doc);
    expect(addressConfiguredInVrf(index, '198.51.100.2', 'default')).toBe(true);
    expect(addressConfiguredInVrf(index, '198.51.100.1', 'default')).toBe(false);
    expect(addressConfiguredInVrf(index, '198.51.100.2', 'customer-a')).toBe(false);
    expect(addressConfiguredInVrf(index, '192.168.100.1', 'customer-a')).toBe(true);
    expect(addressConfiguredInVrf(index, '2001:db8::2', 'default')).toBe(true);
    expect(addressConfiguredInVrf(index, '10.254.0.1', 'default')).toBe(true);
    expect(addressConfiguredInVrf(index, 'bad', 'default')).toBe(false);
  });
  it('prefixesOfInterfaces skips unknown names; recordKeys tolerates non-objects', () => {
    const index = interfaceIndex(doc);
    expect(prefixesOfInterfaces(index, ['TenGigabitEthernet0/0/0', 'nope'])).toHaveLength(2);
    expect([...recordKeys({ a: 1, b: 2 })]).toEqual(['a', 'b']);
    expect(recordKeys(undefined).size).toBe(0);
  });
  it('walkStrings visits every string with its path', () => {
    const seen: string[] = [];
    walkStrings({ a: 'x', b: [1, 'y', { c: 'z' }], d: null, e: 2 }, (path, value) =>
      seen.push(`${path.join('/')}=${value}`),
    );
    expect(seen).toEqual(['a=x', 'b/1=y', 'b/2/c=z']);
    walkStrings('root', (path, value) => seen.push(`${path.join('/')}=${value}`));
    expect(seen.at(-1)).toBe('=root');
  });
});
