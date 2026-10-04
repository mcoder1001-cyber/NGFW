import { z } from 'zod';
import { RootConfig } from './index.js';
import { hostname, timezone, ipv4Cidr, hostOrIp } from './primitives.js';
import { parentInterfaceName } from './domains/interfaces.js';
import { diff } from './diff.js';

export const SetupInputSchema = z
  .strictObject({
    language: z.enum(['en', 'fa']),
    timezone,
    ntp: z.array(hostOrIp).min(1).max(16),
    hostname,
    wan: parentInterfaceName,
    wanMode: z.enum(['dhcp', 'static']),
    wanAddress: ipv4Cidr.optional(),
    wanGateway: z.ipv4().optional(),
    lan: parentInterfaceName,
    lanAddress: ipv4Cidr,
    dhcp: z.boolean(),
    rerun: z.boolean().default(false),
  })
  .superRefine((s, ctx) => {
    if (s.wan === s.lan)
      ctx.addIssue({ code: 'custom', path: ['lan'], message: 'WAN and LAN must differ' });
    if (s.wanMode === 'static' && (!s.wanAddress || !s.wanGateway))
      ctx.addIssue({
        code: 'custom',
        path: ['wanAddress'],
        message: 'static WAN address and gateway required',
      });
    const prefix = Number(s.lanAddress.split('/')[1]);
    if (prefix < 8 || prefix > 29)
      ctx.addIssue({
        code: 'custom',
        path: ['lanAddress'],
        message: 'LAN prefix must be /8 through /29',
      });
  });
export type SetupInput = z.infer<typeof SetupInputSchema>;

const ipv4Number = (ip: string) => ip.split('.').reduce((n, octet) => n * 256 + Number(octet), 0);
const ipv4Text = (n: number) =>
  [24, 16, 8, 0].map((bits) => Math.floor(n / 2 ** bits) % 256).join('.');

export function setupHostName(iface: string): string {
  let value = 2166136261;
  for (const ch of iface) value = Math.imul(value ^ ch.charCodeAt(0), 16777619) >>> 0;
  return `sln-${value.toString(16).padStart(8, '0')}`;
}

/** Stable suggested pool excludes network, broadcast, and the router itself. */
export function setupPool(cidr: string) {
  const [address = '', prefix = ''] = cidr.split('/');
  const ip = ipv4Number(address);
  const size = 2 ** (32 - Number(prefix));
  const network = Math.floor(ip / size) * size;
  if (ip === network || ip === network + size - 1)
    throw new Error('LAN address cannot be network or broadcast');
  const low = ip + 1 < network + size - 1 ? ip + 1 : network + 1;
  const high = low > ip ? network + size - 2 : ip - 1;
  return {
    subnet: `${ipv4Text(network)}/${prefix}`,
    gateway: address,
    start: ipv4Text(low),
    end: ipv4Text(Math.min(high, low + 199)),
  };
}

/** Pure builder: preserve unrelated objects; replace only the named wizard policy and selected interface addressing. */
export function buildSetup(base: RootConfig, raw: SetupInput, completedAt: string): RootConfig {
  const input = SetupInputSchema.parse(raw);
  if (base.system.setup.completed && !input.rerun)
    throw new Error('Setup completed; explicit rerun acknowledgement required');
  if (!base.interfaces[input.wan] || !base.interfaces[input.lan])
    throw new Error('Select existing interfaces');
  if (base.interfaces[input.wan]!.vrf !== base.interfaces[input.lan]!.vrf)
    throw new Error('WAN and LAN must use the same VRF');
  const doc = structuredClone(base);
  const wan = doc.interfaces[input.wan]!;
  const lan = doc.interfaces[input.lan]!;
  const pool = setupPool(input.lanAddress);
  doc.system.hostname = input.hostname;
  doc.system.timezone = input.timezone;
  doc.system.setup = { completed: true, completedAt };
  doc.services.ntp = {
    ...doc.services.ntp,
    enabled: true,
    servers: input.ntp.map((address) => ({ address, iburst: true, prefer: false, nts: false })),
    pools: [],
  };
  wan.enabled = true;
  wan.ipv4 = input.wanMode === 'static' ? [input.wanAddress!] : [];
  wan.ipv6 = [];
  delete wan.unnumbered;
  delete wan.dhcpClient;
  delete wan.pppoe;
  if (input.wanMode === 'dhcp') wan.dhcpClient = { setBroadcastFlag: false };
  lan.enabled = true;
  lan.ipv4 = [input.lanAddress];
  lan.ipv6 = [];
  delete lan.unnumbered;
  delete lan.dhcpClient;
  delete lan.pppoe;
  lan.lcp = {
    ...lan.lcp,
    hostIfName: lan.lcp?.hostIfName ?? setupHostName(input.lan),
    hostIfType: 'tap',
  };
  doc.services.dhcp.servers['setup-lan'] = {
    enabled: input.dhcp,
    family: 'ipv4',
    vrf: lan.vrf,
    interfaces: [input.lan],
    authoritative: true,
    leaseTimeSec: 3600,
    options: [],
    subnets: input.dhcp
      ? {
          lan: {
            subnet: pool.subnet,
            pools: [{ start: pool.start, end: pool.end }],
            gateway: pool.gateway,
            dnsServers: doc.system.dns.servers.some((ip) => !ip.includes(':'))
              ? doc.system.dns.servers.filter((ip) => !ip.includes(':'))
              : ['1.1.1.1', '9.9.9.9'],
            ntpServers: [],
            domainSearch: [],
            options: [],
            reservations: {},
          },
        }
      : {},
  };
  doc.routing.static = doc.routing.static.filter(
    (r) => r.description !== 'Setup WAN default route',
  );
  if (input.wanMode === 'static')
    doc.routing.static.push({
      prefix: '0.0.0.0/0',
      vrf: wan.vrf,
      nextHops: [{ address: input.wanGateway!, interface: input.wan, weight: 1 }],
      distance: 1,
      blackhole: false,
      description: 'Setup WAN default route',
    });
  doc.nat = {
    ...doc.nat,
    enabled: true,
    mode: 'ed',
    inside: [input.lan],
    outside: [input.wan],
    insideVrf: lan.vrf,
    outsideVrf: wan.vrf,
    outputFeature: [],
    forwarding: false,
    pools: [{ name: 'setup-wan', interface: input.wan, twiceNat: false }],
  };
  doc.acl.lists['setup-lan-out'] = {
    tags: [],
    rules: [
      {
        sequence: 10,
        action: 'reflect',
        ipVersion: 'ipv4',
        enabled: true,
        source: { kind: 'prefix', prefix: pool.subnet },
        destination: { kind: 'any' },
        service: { kind: 'any' },
        log: false,
      },
    ],
  };
  doc.acl.lists['setup-wan-in'] = {
    tags: [],
    rules: [
      {
        sequence: 10,
        action: 'deny',
        ipVersion: 'any',
        enabled: true,
        source: { kind: 'any' },
        destination: { kind: 'any' },
        service: { kind: 'any' },
        log: false,
      },
    ],
  };
  doc.acl.attachments = doc.acl.attachments.filter(
    (a) =>
      !['setup-lan-out', 'setup-wan-in'].includes(a.list) &&
      !(
        a.target.kind === 'interface' &&
        [input.wan, input.lan].includes(a.target.interface) &&
        a.direction === 'in'
      ),
  );
  for (const [iface, list] of [
    [input.lan, 'setup-lan-out'],
    [input.wan, 'setup-wan-in'],
  ] as const)
    doc.acl.attachments.push({
      list,
      target: { kind: 'interface', interface: iface },
      direction: 'in',
      sequence: 10,
      enabled: true,
    });
  doc.acl.lists['setup-lan-out']!.rules.unshift({
    sequence: 1,
    action: 'permit',
    ipVersion: 'ipv4',
    enabled: true,
    source: { kind: 'any' },
    destination: { kind: 'any' },
    service: {
      kind: 'inline',
      spec: { protocol: 'udp', sourcePorts: ['68'], destinationPorts: ['67'] },
    },
    log: false,
  });
  if (input.wanMode === 'dhcp')
    doc.acl.lists['setup-wan-in']!.rules.unshift({
      sequence: 1,
      action: 'permit',
      ipVersion: 'ipv4',
      enabled: true,
      source: { kind: 'any' },
      destination: { kind: 'any' },
      service: {
        kind: 'inline',
        spec: { protocol: 'udp', sourcePorts: ['67'], destinationPorts: ['68'] },
      },
      log: false,
    });
  doc.acl.host['setup-input'] = {
    tags: [],
    rules: input.dhcp
      ? [
          {
            sequence: 10,
            action: 'accept',
            ipVersion: 'ipv4',
            enabled: true,
            source: { kind: 'any' },
            destination: { kind: 'any' },
            service: {
              kind: 'inline',
              spec: { protocol: 'udp', sourcePorts: ['68'], destinationPorts: ['67'] },
            },
            interface: lan.lcp.hostIfName!,
            log: false,
          },
        ]
      : [],
  };
  doc.acl.hostAttachments = doc.acl.hostAttachments.filter((a) => a.chain !== 'input');
  doc.acl.hostAttachments.push({ list: 'setup-input', chain: 'input', priority: 0, enabled: true });
  doc.acl.hostSettings = {
    ...doc.acl.hostSettings,
    allowIcmp: true,
    defaultInput: 'drop',
    antiLockout: {
      ...doc.acl.hostSettings?.antiLockout,
      enabled: true,
      sources: [pool.subnet],
      interfaces: [lan.lcp.hostIfName!],
      ports: [...new Set([...(doc.acl.hostSettings?.antiLockout.ports ?? [22, 443]), 22, 443])],
    },
  };
  return RootConfig.parse(doc);
}
export function setupDiff(base: RootConfig, input: SetupInput, completedAt: string) {
  return diff(base, buildSetup(base, input, completedAt));
}
