import { z } from 'zod';
import { withUi } from '../ui.js';
import {
  snmpMonitorsField,
  snmpSubagentField,
  snmpSysServicesField,
  snmpViewRefField,
  snmpViewsField,
} from './ext/snmp.js';
import { HostStackSchema } from './ext/host-stack.js'; // F-host-stack (unanchored)
import { hostname, ipAddress, macAddress, objectName, vppInterfaceName } from '../primitives.js';
import {
  cidrContainsIp,
  cidrsOverlap,
  compareIp,
  ipFamily,
  parseCidr,
  parseIp,
} from '../semantic/tunnels-common.js';
import {
  descriptionField,
  dnsName,
  enabledFlag,
  hostOrIpAddress,
  ipv4OrIpv6Cidr,
  resolvableHostname,
  secretRefOf,
  transportPort,
  u32Int,
  vrfRef,
} from './_shared/primitives.js';
import { servicesNsimField } from './ext/loopback-bvi-gso-lldp-span.js'; // wave-A: F-loopback-bvi-gso-lldp-span

/**
 * `services` — DHCP server (Kea) and relay (VPP dhcp proxy), DNS (Unbound resolver + VPP caching plugin), SNMP
 * (net-snmp), LLDP (VPP plugin), IPFIX/flowprobe + sFlow (VPP plugins), NTP (chrony, the *only* NTP model —
 * D-050 removed `system.ntp`) and QoS (VPP policer + qos plugins, D-052) — WBS D7.1–D7.6, D7.8.
 *
 * Guardrail vdom.md #5: Kea servers and Unbound resolvers are records of *instances*; each instance names its own
 * `vrf` and the interfaces / listen addresses it binds to, so "one daemon instance per VRF/interface/tenant" is a
 * property of the document, never an assumption of the renderer. Guardrail #1: `vrf` is explicit everywhere.
 *
 * Secrets rule (00-CONTEXT #10, D-051): SNMP community strings and SNMPv3 passphrases are `password/<name>`
 * references, NTP symmetric keys and NTS server keys `key/<name>`, NTS certificates `cert/<name>` (see
 * `./_shared/primitives.ts`); every object is strict, so an inline `community: "public"` or `password` field is
 * rejected by the schema.
 *
 * Intra-object rules (pools inside the subnet, families, uniqueness) are schema refinements here; rules that need
 * other domains (interfaces exist, subnet within an interface prefix, VRF exists) live in `semantic/services.ts`.
 *
 * Domain root: every sub-tree is `.prefault({})` (D-017, D-053), so `RootConfig.parse({}).services.ntp` is the
 * disabled default object. Exports of this file are published by `index.ts` (`export *`): the DNS/NTP schemas are
 * named `Services*` so they cannot collide with P02a's `SystemDnsSchema` (D-047). Owner: P02c. Only P02c edits
 * this file.
 */

type Path = (string | number)[];
const add = (ctx: z.RefinementCtx, path: Path, message: string): void => {
  ctx.addIssue({ code: 'custom', path, message });
};

/** Report every repeated value of `items` (by `key`) at `path/<index>`. */
function checkUnique<T>(
  ctx: z.RefinementCtx,
  items: readonly T[],
  key: (item: T) => string | undefined,
  path: Path,
  what: string,
): void {
  const seen = new Set<string>();
  items.forEach((item, i) => {
    const k = key(item);
    if (k === undefined) return;
    if (seen.has(k)) add(ctx, [...path, i], `duplicate ${what} ${k}`);
    seen.add(k);
  });
}

const interfaceList = (title: string, help: string) =>
  withUi(z.array(vppInterfaceName).min(1).max(64), { title, widget: 'interface-picker', help });

const listenAddress = (defaultPort: number) =>
  z.strictObject({
    address: withUi(ipAddress, { title: 'Address' }),
    port: withUi(transportPort.default(defaultPort), { title: 'Port' }),
  });

const printable = (max: number) =>
  z
    .string()
    .min(1)
    .max(max)
    .regex(/^[ -~]+$/, 'printable ASCII without control characters');

const addressFamily = withUi(z.enum(['ipv4', 'ipv6']).default('ipv4'), {
  title: 'Address family',
  widget: 'select',
});

const leaseTime = withUi(z.int().min(60).max(2592000), {
  title: 'Lease time (s)',
  widget: 'number',
});

// ---------------------------------------------------------------------------------------------------------------
// DHCP (Kea server instances, VPP relay)
// ---------------------------------------------------------------------------------------------------------------

export const DhcpOptionSchema = z.strictObject({
  code: withUi(z.int().min(1).max(65535), {
    title: 'Option code',
    widget: 'number',
    help: 'DHCPv4: 1–254, DHCPv6: 1–65535',
  }),
  data: withUi(printable(1024), {
    title: 'Value',
    help: 'Kea option data: text, comma-separated list or hex string',
  }),
  alwaysSend: withUi(z.boolean().default(false), { title: 'Always send', widget: 'switch' }),
});

export const DhcpPoolSchema = z
  .strictObject({
    start: withUi(ipAddress, { title: 'First address' }),
    end: withUi(ipAddress, { title: 'Last address' }),
  })
  .superRefine((p, ctx) => {
    const order = compareIp(p.start, p.end);
    if (order === undefined)
      add(ctx, ['end'], 'pool start and end must be of the same address family');
    else if (order > 0) add(ctx, ['end'], 'pool end is lower than pool start');
  });

/** Kea DUID: colon-separated hex bytes (2–130 bytes). */
const duid = withUi(
  z
    .string()
    .regex(/^(?:[0-9a-fA-F]{2}:){1,129}[0-9a-fA-F]{2}$/, 'expected a DUID like 00:01:00:01:...'),
  { title: 'DUID' },
);

export const DhcpReservationSchema = z
  .strictObject({
    mac: withUi(macAddress, { title: 'MAC address' }).optional(),
    duid: duid.optional(),
    ip: withUi(ipAddress, { title: 'Reserved address' }),
    hostname: withUi(hostname, { title: 'Hostname' }).optional(),
    options: withUi(z.array(DhcpOptionSchema).max(32).default([]), { title: 'Options' }),
  })
  .superRefine((r, ctx) => {
    if ((r.mac === undefined) === (r.duid === undefined)) {
      add(ctx, ['mac'], 'a reservation identifies the client by exactly one of mac or duid');
    }
    checkUnique(ctx, r.options, (o) => String(o.code), ['options'], 'option code');
  });

export const DhcpSubnetSchema = z
  .strictObject({
    description: descriptionField.optional(),
    subnet: withUi(ipv4OrIpv6Cidr, {
      title: 'Subnet',
      help: 'Must lie within a prefix configured on one of the server interfaces',
    }),
    pools: withUi(z.array(DhcpPoolSchema).min(1).max(32), { title: 'Address pools' }),
    gateway: withUi(ipAddress, { title: 'Default gateway (DHCPv4 routers option)' }).optional(),
    dnsServers: withUi(z.array(ipAddress).max(4).default([]), { title: 'DNS servers' }),
    ntpServers: withUi(z.array(ipAddress).max(4).default([]), { title: 'NTP servers' }),
    domainName: withUi(hostname, { title: 'Domain name' }).optional(),
    domainSearch: withUi(z.array(hostname).max(8).default([]), { title: 'Domain search list' }),
    leaseTimeSec: leaseTime.optional(),
    options: withUi(z.array(DhcpOptionSchema).max(64).default([]), { title: 'Options' }),
    reservations: withUi(z.record(objectName, DhcpReservationSchema).default({}), {
      title: 'Reservations',
      widget: 'record',
    }),
  })
  .superRefine((s, ctx) => {
    const net = parseCidr(s.subnet);
    if (net === undefined) return; // unreachable after the CIDR primitive; kept for safety
    const fam = net.family;
    const inside = (address: string): boolean => {
      const ip = parseIp(address);
      return ip !== undefined && cidrContainsIp(net, ipFamily(address), ip);
    };
    s.pools.forEach((p, i) => {
      if (ipFamily(p.start) !== fam) {
        add(ctx, ['pools', i, 'start'], `pool family does not match subnet ${s.subnet}`);
        return;
      }
      if (!inside(p.start)) add(ctx, ['pools', i, 'start'], `${p.start} is outside ${s.subnet}`);
      if (!inside(p.end)) add(ctx, ['pools', i, 'end'], `${p.end} is outside ${s.subnet}`);
    });
    s.pools.forEach((a, i) => {
      for (let j = 0; j < i; j++) {
        const b = s.pools[j] as typeof a;
        if (compareIp(a.start, b.end) !== 1 && compareIp(b.start, a.end) !== 1) {
          add(ctx, ['pools', i], `pool overlaps pool ${j}`);
          break;
        }
      }
    });
    if (s.gateway !== undefined) {
      if (fam === 6) add(ctx, ['gateway'], 'gateway applies to DHCPv4 subnets only (IPv6 uses RA)');
      else if (!inside(s.gateway)) add(ctx, ['gateway'], `${s.gateway} is outside ${s.subnet}`);
    }
    (['dnsServers', 'ntpServers'] as const).forEach((field) => {
      s[field].forEach((a, i) => {
        if (ipFamily(a) !== fam) add(ctx, [field, i], `${a} does not match the subnet family`);
      });
      checkUnique(ctx, s[field], (a) => a, [field], 'server');
    });
    checkUnique(ctx, s.options, (o) => String(o.code), ['options'], 'option code');
    if (fam === 4) {
      s.options.forEach((o, i) => {
        if (o.code > 254) add(ctx, ['options', i, 'code'], 'DHCPv4 option codes are 1–254');
      });
    }
    const ips = new Set<string>();
    const ids = new Set<string>();
    for (const [name, r] of Object.entries(s.reservations)) {
      if (ipFamily(r.ip) !== fam || !inside(r.ip)) {
        add(ctx, ['reservations', name, 'ip'], `${r.ip} is outside ${s.subnet}`);
      }
      if (ips.has(r.ip)) add(ctx, ['reservations', name, 'ip'], `${r.ip} is reserved twice`);
      ips.add(r.ip);
      if (r.duid !== undefined && fam === 4) {
        add(ctx, ['reservations', name, 'duid'], 'DUID reservations apply to DHCPv6 only');
      }
      const id = (r.mac ?? r.duid ?? '').toLowerCase().replace(/-/g, ':');
      if (id !== '') {
        if (ids.has(id)) add(ctx, ['reservations', name], `client ${id} is reserved twice`);
        ids.add(id);
      }
    }
  });

export const DhcpServerSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    family: addressFamily,
    vrf: vrfRef,
    interfaces: interfaceList(
      'Interfaces',
      'Interfaces this Kea instance listens on (all in the instance VRF)',
    ),
    leaseTimeSec: leaseTime.default(3600),
    renewTimerSec: withUi(z.int().min(30).max(2592000), {
      title: 'Renew timer T1 (s)',
      widget: 'number',
    }).optional(),
    rebindTimerSec: withUi(z.int().min(30).max(2592000), {
      title: 'Rebind timer T2 (s)',
      widget: 'number',
    }).optional(),
    authoritative: withUi(z.boolean().default(true), { title: 'Authoritative', widget: 'switch' }),
    subnets: withUi(z.record(objectName, DhcpSubnetSchema).default({}), {
      title: 'Subnets',
      widget: 'record',
    }),
    options: withUi(z.array(DhcpOptionSchema).max(64).default([]), { title: 'Global options' }),
  })
  .superRefine((s, ctx) => {
    checkUnique(ctx, s.interfaces, (i) => i, ['interfaces'], 'interface');
    if (s.renewTimerSec !== undefined && s.renewTimerSec >= s.leaseTimeSec) {
      add(ctx, ['renewTimerSec'], 'renew timer must be shorter than the lease time');
    }
    if (s.rebindTimerSec !== undefined && s.rebindTimerSec >= s.leaseTimeSec) {
      add(ctx, ['rebindTimerSec'], 'rebind timer must be shorter than the lease time');
    }
    if (
      s.renewTimerSec !== undefined &&
      s.rebindTimerSec !== undefined &&
      s.renewTimerSec >= s.rebindTimerSec
    ) {
      add(ctx, ['renewTimerSec'], 'renew timer must be shorter than the rebind timer');
    }
    checkUnique(ctx, s.options, (o) => String(o.code), ['options'], 'option code');
    if (s.family === 'ipv4') {
      s.options.forEach((o, i) => {
        if (o.code > 254) add(ctx, ['options', i, 'code'], 'DHCPv4 option codes are 1–254');
      });
    }
    const wantFamily = s.family === 'ipv4' ? 4 : 6;
    const parsed = Object.entries(s.subnets).map(
      ([name, sub]) => [name, parseCidr(sub.subnet)] as const,
    );
    parsed.forEach(([name, net], i) => {
      if (net === undefined) return;
      if (net.family !== wantFamily) {
        add(
          ctx,
          ['subnets', name, 'subnet'],
          `subnet family does not match server family ${s.family}`,
        );
        return;
      }
      for (let j = 0; j < i; j++) {
        const [otherName, other] = parsed[j] as (typeof parsed)[number];
        if (other !== undefined && cidrsOverlap(net, other)) {
          add(ctx, ['subnets', name, 'subnet'], `overlaps subnet '${otherName}'`);
        }
      }
    });
  });

export const DhcpRelaySchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    family: addressFamily,
    vrf: withUi(vrfRef, {
      title: 'Client VRF',
      widget: 'vrf-picker',
      help: 'VRF of the client-facing interfaces (VPP rx VRF)',
    }),
    serverVrf: withUi(objectName, {
      title: 'Server VRF',
      widget: 'vrf-picker',
      help: 'VRF the DHCP servers are reached in; defaults to the client VRF',
    }).optional(),
    interfaces: interfaceList('Client interfaces', 'Interfaces whose DHCP requests are relayed'),
    servers: withUi(z.array(ipAddress).min(1).max(8), { title: 'DHCP servers' }),
    sourceAddress: withUi(ipAddress, {
      title: 'Relay source address',
      help: 'giaddr / link-address and source of the relayed request; must be configured on an interface in the server VRF (serverVrf, else the client VRF)',
    }),
  })
  .superRefine((r, ctx) => {
    const want = r.family === 'ipv4' ? 4 : 6;
    checkUnique(ctx, r.interfaces, (i) => i, ['interfaces'], 'interface');
    checkUnique(ctx, r.servers, (s) => s, ['servers'], 'server');
    r.servers.forEach((s, i) => {
      if (ipFamily(s) !== want) add(ctx, ['servers', i], `${s} does not match family ${r.family}`);
    });
    if (ipFamily(r.sourceAddress) !== want) {
      add(ctx, ['sourceAddress'], `${r.sourceAddress} does not match family ${r.family}`);
    }
  });

export const DhcpSchema = z.strictObject({
  servers: withUi(z.record(objectName, DhcpServerSchema).default({}), {
    title: 'DHCP servers (Kea instances)',
    widget: 'record',
  }),
  relays: withUi(z.record(objectName, DhcpRelaySchema).default({}), {
    title: 'DHCP relays',
    widget: 'record',
  }),
});

// ---------------------------------------------------------------------------------------------------------------
// DNS (Unbound resolver instances + VPP dns cache plugin)
// ---------------------------------------------------------------------------------------------------------------

export const DNS_RECORD_TYPES = ['A', 'AAAA', 'CNAME', 'MX', 'NS', 'PTR', 'SRV', 'TXT'] as const;

export const DnsUpstreamSchema = z
  .strictObject({
    address: withUi(ipAddress, { title: 'Server address' }),
    port: withUi(transportPort.default(53), { title: 'Port' }),
    tls: withUi(z.boolean().default(false), { title: 'DNS over TLS', widget: 'switch' }),
    tlsServerName: withUi(hostname, { title: 'TLS server name' }).optional(),
  })
  .superRefine((u, ctx) => {
    if (u.tlsServerName !== undefined && !u.tls) {
      add(ctx, ['tlsServerName'], 'tlsServerName applies to DNS-over-TLS upstreams only');
    }
  });

export const DnsForwardZoneSchema = z.strictObject({
  zone: withUi(dnsName, { title: 'Zone' }),
  forwarders: withUi(z.array(DnsUpstreamSchema).min(1).max(8), { title: 'Forwarders' }),
  forwardFirst: withUi(z.boolean().default(false), {
    title: 'Forward first',
    widget: 'switch',
    help: 'Fall back to recursion when the forwarders fail',
  }),
});

export const DnsRecordSchema = z
  .strictObject({
    name: withUi(dnsName, { title: 'Name' }),
    type: withUi(z.enum(DNS_RECORD_TYPES), { title: 'Type', widget: 'select' }),
    ttlSec: withUi(z.int().min(0).max(604800).default(3600), {
      title: 'TTL (s)',
      widget: 'number',
    }),
    data: withUi(printable(1024), { title: 'Data' }),
  })
  .superRefine((r, ctx) => {
    const bad = (m: string): void => add(ctx, ['data'], m);
    switch (r.type) {
      case 'A':
        if (!z.ipv4().safeParse(r.data).success) bad('an A record needs an IPv4 address');
        break;
      case 'AAAA':
        if (!z.ipv6().safeParse(r.data).success) bad('an AAAA record needs an IPv6 address');
        break;
      case 'CNAME':
      case 'NS':
      case 'PTR':
        if (!dnsName.safeParse(r.data).success) bad(`a ${r.type} record needs a DNS name`);
        break;
      case 'MX':
        if (!/^\d{1,5} \S+$/.test(r.data)) bad('an MX record is "<priority> <mail host>"');
        break;
      case 'SRV':
        if (!/^\d{1,5} \d{1,5} \d{1,5} \S+$/.test(r.data)) {
          bad('an SRV record is "<priority> <weight> <port> <target>"');
        }
        break;
      case 'TXT':
        break;
    }
  });

export const DnsLocalZoneSchema = z
  .strictObject({
    zone: withUi(dnsName, { title: 'Zone' }),
    type: withUi(
      z
        .enum(['static', 'transparent', 'redirect', 'refuse', 'deny', 'nodefault'])
        .default('static'),
      { title: 'Zone type', widget: 'select', help: 'Unbound local-zone type' },
    ),
    records: withUi(z.array(DnsRecordSchema).max(4096).default([]), { title: 'Records' }),
  })
  .superRefine((zone, ctx) => {
    checkUnique(
      ctx,
      zone.records,
      (r) => `${r.name.toLowerCase()} ${r.type} ${r.data}`,
      ['records'],
      'record',
    );
  });

export const DnsAccessControlSchema = z.strictObject({
  prefix: withUi(ipv4OrIpv6Cidr, { title: 'Client prefix' }),
  action: withUi(z.enum(['allow', 'deny', 'refuse', 'allow_snoop']).default('allow'), {
    title: 'Action',
    widget: 'select',
  }),
});

export const DnsResolverSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    vrf: vrfRef,
    listen: withUi(z.array(listenAddress(53)).min(1).max(16), {
      title: 'Listen addresses',
      help: 'Addresses configured on interfaces in the VRF (0.0.0.0 / :: = all addresses of the VRF)',
    }),
    accessControl: withUi(z.array(DnsAccessControlSchema).max(64).default([]), {
      title: 'Access control',
      help: 'Empty = allow the listen prefixes only (Unbound default)',
    }),
    forwarders: withUi(z.array(DnsUpstreamSchema).max(8).default([]), {
      title: 'Forwarders',
      help: 'Upstream resolvers for everything not covered by a forward zone; empty = full recursion',
    }),
    forwardZones: withUi(z.array(DnsForwardZoneSchema).max(256).default([]), {
      title: 'Forward zones',
    }),
    localZones: withUi(z.array(DnsLocalZoneSchema).max(256).default([]), { title: 'Local zones' }),
    dnssec: withUi(
      z.strictObject({
        enabled: withUi(z.boolean().default(true), {
          title: 'DNSSEC validation',
          widget: 'switch',
        }),
        trustAnchorAuto: withUi(z.boolean().default(true), {
          title: 'Automatic trust anchor (RFC 5011)',
          widget: 'switch',
        }),
      }),
      { title: 'DNSSEC' },
    ).prefault({}),
    cache: withUi(
      z
        .strictObject({
          minTtlSec: withUi(z.int().min(0).max(86400).default(0), { title: 'Minimum TTL (s)' }),
          maxTtlSec: withUi(z.int().min(0).max(604800).default(86400), {
            title: 'Maximum TTL (s)',
          }),
          prefetch: withUi(z.boolean().default(false), { title: 'Prefetch', widget: 'switch' }),
          msgCacheMb: withUi(z.int().min(1).max(4096).default(4), { title: 'Message cache (MB)' }),
          rrsetCacheMb: withUi(z.int().min(1).max(8192).default(8), { title: 'RRset cache (MB)' }),
        })
        .superRefine((c, ctx) => {
          if (c.minTtlSec > c.maxTtlSec)
            add(ctx, ['minTtlSec'], 'minimum TTL exceeds the maximum TTL');
        }),
      { title: 'Cache' },
    ).prefault({}),
    threads: withUi(z.int().min(1).max(64).default(1), { title: 'Threads', widget: 'number' }),
    qnameMinimisation: withUi(z.boolean().default(true), {
      title: 'QNAME minimisation',
      widget: 'switch',
    }),
    hideIdentity: withUi(z.boolean().default(true), { title: 'Hide identity', widget: 'switch' }),
    hideVersion: withUi(z.boolean().default(true), { title: 'Hide version', widget: 'switch' }),
    logQueries: withUi(z.boolean().default(false), { title: 'Log queries', widget: 'switch' }),
  })
  .superRefine((r, ctx) => {
    checkUnique(ctx, r.listen, (l) => `${l.address}:${l.port}`, ['listen'], 'listen address');
    checkUnique(ctx, r.forwardZones, (f) => f.zone.toLowerCase(), ['forwardZones'], 'forward zone');
    checkUnique(ctx, r.localZones, (l) => l.zone.toLowerCase(), ['localZones'], 'local zone');
    checkUnique(ctx, r.accessControl, (a) => a.prefix, ['accessControl'], 'prefix');
    checkUnique(ctx, r.forwarders, (f) => `${f.address}:${f.port}`, ['forwarders'], 'forwarder');
    const listening = new Set(r.listen.map((l) => `${l.address}:${l.port}`));
    r.forwarders.forEach((f, i) => {
      if (listening.has(`${f.address}:${f.port}`)) {
        add(ctx, ['forwarders', i], 'a forwarder must not be one of the listen addresses (loop)');
      }
    });
  });

export const DnsVppCacheSchema = z.strictObject({
  enabled: withUi(z.boolean().default(false), { title: 'VPP DNS cache', widget: 'switch' }),
  upstreams: withUi(z.array(ipAddress).min(1).max(8), {
    title: 'Upstream name servers',
    help: 'Servers the VPP dns plugin resolves through',
  }),
});

export const ServicesDnsSchema = z.strictObject({
  resolvers: withUi(z.record(objectName, DnsResolverSchema).default({}), {
    title: 'Resolvers (Unbound instances)',
    widget: 'record',
  }),
  vppCache: withUi(DnsVppCacheSchema, { title: 'VPP caching DNS plugin' }).optional(),
});

// ---------------------------------------------------------------------------------------------------------------
// SNMP (net-snmp; community strings and USM keys are secrets)
// ---------------------------------------------------------------------------------------------------------------

const snmpAccess = withUi(z.enum(['ro', 'rw']).default('ro'), {
  title: 'Access',
  widget: 'select',
});

export const SnmpCommunitySchema = z.strictObject({
  secretRef: withUi(secretRefOf('password'), {
    title: 'Community string (reference)',
    help: 'The community string itself is a secret (kind password) and is never part of the document',
  }),
  access: snmpAccess,
  sources: withUi(z.array(ipv4OrIpv6Cidr).max(16).default([]), {
    title: 'Allowed sources',
    help: 'Empty = any',
  }),
  view: snmpViewRefField, // F-snmp (unanchored)
});

export const SnmpV3UserSchema = z
  .strictObject({
    securityLevel: withUi(z.enum(['noAuthNoPriv', 'authNoPriv', 'authPriv']).default('authPriv'), {
      title: 'Security level',
      widget: 'select',
    }),
    authProtocol: withUi(z.enum(['sha', 'sha256', 'sha512', 'md5']).default('sha'), {
      title: 'Authentication protocol',
      widget: 'select',
    }),
    authRef: withUi(secretRefOf('password'), {
      title: 'Authentication passphrase (reference)',
    }).optional(),
    privProtocol: withUi(z.enum(['aes', 'aes256', 'des']).default('aes'), {
      title: 'Privacy protocol',
      widget: 'select',
    }),
    privRef: withUi(secretRefOf('password'), {
      title: 'Privacy passphrase (reference)',
    }).optional(),
    access: snmpAccess,
    view: snmpViewRefField, // F-snmp (unanchored)
  })
  .superRefine((u, ctx) => {
    const needAuth = u.securityLevel !== 'noAuthNoPriv';
    const needPriv = u.securityLevel === 'authPriv';
    if (needAuth && u.authRef === undefined)
      add(ctx, ['authRef'], `${u.securityLevel} needs authRef`);
    if (!needAuth && u.authRef !== undefined)
      add(ctx, ['authRef'], 'noAuthNoPriv takes no authRef');
    if (needPriv && u.privRef === undefined) add(ctx, ['privRef'], 'authPriv needs privRef');
    if (!needPriv && u.privRef !== undefined)
      add(ctx, ['privRef'], `${u.securityLevel} takes no privRef`);
  });

export const SnmpTrapReceiverSchema = z
  .strictObject({
    address: hostOrIpAddress,
    port: withUi(transportPort.default(162), { title: 'Port' }),
    version: withUi(z.enum(['v2c', 'v3']).default('v2c'), { title: 'Version', widget: 'select' }),
    community: withUi(objectName, {
      title: 'Community',
      help: 'Name in snmp.communities',
    }).optional(),
    user: withUi(objectName, { title: 'SNMPv3 user', help: 'Name in snmp.v3Users' }).optional(),
    inform: withUi(z.boolean().default(false), {
      title: 'Send informs',
      widget: 'switch',
      help: 'Acknowledged notifications instead of traps',
    }),
  })
  .superRefine((t, ctx) => {
    if (t.version === 'v2c') {
      if (t.community === undefined) add(ctx, ['community'], 'a v2c receiver needs a community');
      if (t.user !== undefined) add(ctx, ['user'], 'a v2c receiver takes no SNMPv3 user');
    } else {
      if (t.user === undefined) add(ctx, ['user'], 'a v3 receiver needs an SNMPv3 user');
      if (t.community !== undefined) add(ctx, ['community'], 'a v3 receiver takes no community');
    }
  });

export const SnmpSchema = z
  .strictObject({
    enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
    description: descriptionField.optional(),
    vrf: vrfRef,
    listen: withUi(z.array(listenAddress(161)).max(8).default([]), {
      title: 'Listen addresses',
      help: 'Empty = all addresses of the VRF',
    }),
    engineId: withUi(z.string().regex(/^(?:[0-9a-fA-F]{2}){5,32}$/, 'expected 5–32 hex bytes'), {
      title: 'Engine id',
      help: 'SNMPv3 engine id (hex); omit to derive from the MAC address',
    }).optional(),
    sysName: withUi(hostname, { title: 'sysName' }).optional(),
    sysLocation: withUi(printable(255), { title: 'sysLocation' }).optional(),
    sysContact: withUi(printable(255), { title: 'sysContact' }).optional(),
    communities: withUi(z.record(objectName, SnmpCommunitySchema).default({}), {
      title: 'Communities (v1/v2c)',
      widget: 'record',
    }),
    v3Users: withUi(z.record(objectName, SnmpV3UserSchema).default({}), {
      title: 'SNMPv3 users',
      widget: 'record',
    }),
    trapReceivers: withUi(z.array(SnmpTrapReceiverSchema).max(16).default([]), {
      title: 'Trap receivers',
    }),
    // F-snmp (unanchored): D-086 stand-ins, sub-schemas in ext/snmp.ts
    sysServices: snmpSysServicesField,
    views: snmpViewsField,
    monitors: snmpMonitorsField,
    subagent: snmpSubagentField,
  })
  .superRefine((s, ctx) => {
    checkUnique(ctx, s.listen, (l) => `${l.address}:${l.port}`, ['listen'], 'listen address');
    s.trapReceivers.forEach((t, i) => {
      if (t.community !== undefined && !(t.community in s.communities)) {
        add(ctx, ['trapReceivers', i, 'community'], `community '${t.community}' is not defined`);
      }
      if (t.user !== undefined && !(t.user in s.v3Users)) {
        add(ctx, ['trapReceivers', i, 'user'], `SNMPv3 user '${t.user}' is not defined`);
      }
    });
    checkUnique(
      ctx,
      s.trapReceivers,
      (t) => `${t.address}:${t.port}:${t.version}`,
      ['trapReceivers'],
      'trap receiver',
    );
  });

// ---------------------------------------------------------------------------------------------------------------
// LLDP (VPP lldp plugin)
// ---------------------------------------------------------------------------------------------------------------

export const LldpInterfaceSchema = z.strictObject({
  interface: withUi(vppInterfaceName, { title: 'Interface' }),
  portDescription: withUi(printable(255), { title: 'Port description' }).optional(),
  mgmtIpv4: withUi(z.ipv4(), { title: 'Management IPv4' }).optional(),
  mgmtIpv6: withUi(z.ipv6(), { title: 'Management IPv6' }).optional(),
  mgmtOid: withUi(
    z
      .string()
      .max(128)
      .regex(/^[0-9]+(?:\.[0-9]+)*$/, 'expected a dotted OID'),
    {
      title: 'Management OID',
    },
  ).optional(),
});

export const LldpSchema = z
  .strictObject({
    enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
    systemName: withUi(hostname, {
      title: 'System name',
      help: "Empty keeps VPP's current system name (VPP starts without one)",
    }).optional(),
    txHold: withUi(z.int().min(1).max(10).default(4), {
      title: 'TX hold multiplier',
      widget: 'number',
    }),
    txIntervalSec: withUi(z.int().min(1).max(3600).default(30), {
      title: 'TX interval (s)',
      widget: 'number',
    }),
    interfaces: withUi(z.array(LldpInterfaceSchema).max(256).default([]), {
      title: 'Interfaces',
      help: 'Interfaces LLDP is enabled on',
    }),
  })
  .superRefine((l, ctx) => {
    checkUnique(ctx, l.interfaces, (i) => i.interface, ['interfaces'], 'interface');
  });

// ---------------------------------------------------------------------------------------------------------------
// IPFIX (VPP ipfix exporter + flowprobe) and sFlow (VPP sflow plugin)
// ---------------------------------------------------------------------------------------------------------------

export const IpfixExporterSchema = z
  .strictObject({
    enabled: enabledFlag,
    description: descriptionField.optional(),
    collector: withUi(listenAddress(4739), { title: 'Collector' }),
    sourceAddress: withUi(ipAddress, {
      title: 'Source address',
      help: 'Must be configured on an interface in the VRF',
    }),
    vrf: vrfRef,
    pathMtu: withUi(z.int().min(68).max(1450).default(512), {
      title: 'Path MTU',
      widget: 'number',
    }),
    templateIntervalSec: withUi(z.int().min(1).max(3600).default(20), {
      title: 'Template interval (s)',
      widget: 'number',
    }),
    udpChecksum: withUi(z.boolean().default(false), { title: 'UDP checksum', widget: 'switch' }),
  })
  .superRefine((e, ctx) => {
    if (ipFamily(e.collector.address) !== ipFamily(e.sourceAddress)) {
      add(ctx, ['sourceAddress'], 'collector and source address must be of the same family');
    } else if (e.collector.address === e.sourceAddress) {
      add(ctx, ['collector', 'address'], 'collector equals the source address');
    }
  });

export const IpfixFlowprobeInterfaceSchema = z
  .strictObject({
    interface: withUi(vppInterfaceName, { title: 'Interface' }),
    direction: withUi(z.enum(['rx', 'tx', 'both']).default('both'), {
      title: 'Direction',
      widget: 'select',
    }),
    l2: withUi(z.boolean().default(false), { title: 'L2 flows', widget: 'switch' }),
    ip4: withUi(z.boolean().default(true), { title: 'IPv4 flows', widget: 'switch' }),
    ip6: withUi(z.boolean().default(false), { title: 'IPv6 flows', widget: 'switch' }),
  })
  .superRefine((f, ctx) => {
    if (!f.l2 && !f.ip4 && !f.ip6) add(ctx, ['ip4'], 'enable at least one of l2, ip4, ip6');
  });

export const IpfixFlowprobeSchema = z
  .strictObject({
    activeTimerSec: withUi(z.int().min(1).max(3600).default(15), {
      title: 'Active timer (s)',
      widget: 'number',
    }),
    passiveTimerSec: withUi(z.int().min(1).max(3600).default(120), {
      title: 'Passive timer (s)',
      widget: 'number',
    }),
    recordL2: withUi(z.boolean().default(false), { title: 'Record L2 fields', widget: 'switch' }),
    recordL3: withUi(z.boolean().default(true), { title: 'Record L3 fields', widget: 'switch' }),
    recordL4: withUi(z.boolean().default(true), { title: 'Record L4 fields', widget: 'switch' }),
    interfaces: withUi(z.array(IpfixFlowprobeInterfaceSchema).max(256).default([]), {
      title: 'Monitored interfaces',
    }),
  })
  .superRefine((f, ctx) => {
    checkUnique(ctx, f.interfaces, (i) => i.interface, ['interfaces'], 'interface');
    if (f.passiveTimerSec < f.activeTimerSec) {
      add(ctx, ['passiveTimerSec'], 'passive timer must not be shorter than the active timer');
    }
    if (f.interfaces.length > 0 && !f.recordL2 && !f.recordL3 && !f.recordL4) {
      add(ctx, ['recordL3'], 'enable at least one record layer');
    }
  });

export const SflowSchema = z
  .strictObject({
    enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
    samplingN: withUi(z.int().min(1).max(2147483647).default(10000), {
      title: 'Sampling rate (1 in N)',
      widget: 'number',
    }),
    pollingIntervalSec: withUi(z.int().min(1).max(3600).default(20), {
      title: 'Counter polling interval (s)',
      widget: 'number',
    }),
    headerBytes: withUi(z.int().min(64).max(256).default(128), {
      title: 'Sampled header bytes',
      widget: 'number',
    }),
    collectors: withUi(z.array(listenAddress(6343)).min(1).max(4), { title: 'Collectors' }),
    agentAddress: withUi(ipAddress, { title: 'Agent address' }).optional(),
    vrf: vrfRef,
    interfaces: withUi(z.array(vppInterfaceName).max(256).default([]), {
      title: 'Sampled interfaces',
      widget: 'interface-picker',
    }),
  })
  .superRefine((s, ctx) => {
    checkUnique(ctx, s.interfaces, (i) => i, ['interfaces'], 'interface');
    checkUnique(ctx, s.collectors, (c) => `${c.address}:${c.port}`, ['collectors'], 'collector');
  });

export const IpfixSchema = z.strictObject({
  exporters: withUi(z.record(objectName, IpfixExporterSchema).default({}), {
    title: 'IPFIX exporters',
    widget: 'record',
  }),
  flowprobe: withUi(IpfixFlowprobeSchema, { title: 'Flow probe' }).prefault({}),
  sflow: withUi(SflowSchema, { title: 'sFlow' }).optional(),
});

// ---------------------------------------------------------------------------------------------------------------
// NTP (chrony, client and server)
// ---------------------------------------------------------------------------------------------------------------

export const ServicesNtpServerSchema = z
  .strictObject({
    address: hostOrIpAddress,
    iburst: withUi(z.boolean().default(true), { title: 'iburst', widget: 'switch' }),
    prefer: withUi(z.boolean().default(false), { title: 'Prefer', widget: 'switch' }),
    minPoll: withUi(z.int().min(-6).max(24), { title: 'Minimum poll (log2 s)' }).optional(),
    maxPoll: withUi(z.int().min(-6).max(24), { title: 'Maximum poll (log2 s)' }).optional(),
    nts: withUi(z.boolean().default(false), {
      title: 'NTS (authenticated NTP over TLS)',
      widget: 'switch',
    }),
    keyRef: withUi(secretRefOf('key'), {
      title: 'Symmetric key (reference)',
      help: 'chrony key for this server (kind key); exclusive with NTS',
    }).optional(),
  })
  .superRefine((s, ctx) => {
    if (s.minPoll !== undefined && s.maxPoll !== undefined && s.minPoll > s.maxPoll) {
      add(ctx, ['minPoll'], 'minimum poll exceeds maximum poll');
    }
    if (s.nts && s.keyRef !== undefined)
      add(ctx, ['keyRef'], 'NTS and a symmetric key are exclusive');
  });

export const ServicesNtpSchema = z
  .strictObject({
    enabled: withUi(z.boolean().default(false), { title: 'Enabled', widget: 'switch' }),
    vrf: vrfRef,
    servers: withUi(z.array(ServicesNtpServerSchema).max(16).default([]), { title: 'Servers' }),
    pools: withUi(z.array(resolvableHostname).max(8).default([]), { title: 'Pools' }),
    allow: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Serve time to',
      help: 'Client prefixes allowed to query this node (server mode); empty = client only',
    }),
    listen: withUi(z.array(ipAddress).max(8).default([]), {
      title: 'Bind addresses',
      help: 'Empty = all addresses of the VRF',
    }),
    deny: withUi(z.array(ipv4OrIpv6Cidr).max(64).default([]), {
      title: 'Refuse time to',
      help: 'Client prefixes explicitly refused (chrony deny; evaluated after allow)',
    }),
    port: withUi(z.int().min(0).max(65535).default(123), {
      title: 'Server port',
      widget: 'number',
      help: 'UDP port chrony serves on; 0 = client only (server mode disabled even with allow)',
    }),
    rateLimit: withUi(
      z.strictObject({
        interval: withUi(z.int().min(-19).max(12).default(3), {
          title: 'Interval (log2 s)',
          help: 'Minimum interval between responses per client (log2 seconds)',
        }),
        burst: withUi(z.int().min(0).max(255).default(8), { title: 'Burst' }),
        leak: withUi(z.int().min(0).max(4).default(2), { title: 'Leak (log2)' }),
      }),
      { title: 'Server rate limit', help: 'chrony ratelimit for served clients; omit to disable' },
    ).optional(),
    localStratum: withUi(z.int().min(1).max(15), {
      title: 'Local stratum',
      widget: 'number',
      help: 'Serve local time at this stratum when no source is reachable (chrony local stratum)',
    }).optional(),
    orphan: withUi(z.boolean().default(false), {
      title: 'Orphan mode',
      widget: 'switch',
      help: 'With localStratum: elect one of several isolated servers as the reference (chrony local orphan)',
    }),
    rtcSync: withUi(z.boolean().default(true), {
      title: 'Sync the RTC',
      widget: 'switch',
      help: 'Let the kernel copy system time to the real-time clock every 11 minutes (chrony rtcsync)',
    }),
    ntsServer: withUi(
      z.strictObject({
        certificateRef: withUi(secretRefOf('cert'), { title: 'NTS-KE certificate (reference)' }),
        keyRef: withUi(secretRefOf('key'), { title: 'NTS-KE private key (reference)' }),
        port: withUi(transportPort.default(4460), { title: 'NTS-KE port' }),
      }),
      {
        title: 'NTS server',
        help: 'Serve authenticated NTP (NTS-KE over TLS); the certificate and key are stored through POST /api/v1/secrets',
      },
    ).optional(),
    makestep: withUi(
      z.strictObject({
        thresholdSec: withUi(z.number().min(0.1).max(1000).default(1), { title: 'Threshold (s)' }),
        limit: withUi(z.int().min(-1).max(100).default(3), {
          title: 'Limit',
          help: 'Number of clock updates that may step; -1 = always',
        }),
      }),
      { title: 'Step the clock' },
    ).prefault({}),
  })
  .superRefine((n, ctx) => {
    checkUnique(ctx, n.servers, (s) => s.address.toLowerCase(), ['servers'], 'server');
    checkUnique(ctx, n.pools, (p) => p.toLowerCase(), ['pools'], 'pool');
    checkUnique(ctx, n.allow, (a) => a, ['allow'], 'prefix');
    checkUnique(ctx, n.deny, (a) => a, ['deny'], 'prefix');
    checkUnique(ctx, n.listen, (l) => l, ['listen'], 'address');
    if (
      n.enabled &&
      n.servers.length === 0 &&
      n.pools.length === 0 &&
      n.localStratum === undefined
    ) {
      add(ctx, ['servers'], 'an enabled NTP service needs a server, a pool or a local stratum');
    }
    if (n.orphan && n.localStratum === undefined) {
      add(ctx, ['orphan'], 'orphan mode needs localStratum');
    }
    const serving = n.allow.length > 0 || n.ntsServer !== undefined;
    if (serving && n.port === 0) {
      add(ctx, ['port'], 'port 0 disables server mode; remove allow/ntsServer or set a port');
    }
    if (n.rateLimit !== undefined && !serving) {
      add(ctx, ['rateLimit'], 'rateLimit applies to server mode (allow or ntsServer)');
    }
  });

// ---------------------------------------------------------------------------------------------------------------
// QoS (WBS D7.8, D-052): VPP policer plugin (policers; shapers as egress token buckets) and qos plugin (record /
// store the QoS bits of a packet, egress maps, mark). Hierarchical HQoS (DPDK scheduler) is excluded (V3).
// ---------------------------------------------------------------------------------------------------------------

/**
 * VPP policer algorithms: `1r2c` single rate two colours (conform / exceed), `1r3c-rfc2697` single rate three
 * colours, `2r3c-rfc2698` / `2r3c-rfc4115` / `2r3c-mef5cf1` two rates three colours (cir + eir, cb + eb).
 */
export const QOS_POLICER_TYPES = [
  '1r2c',
  '1r3c-rfc2697',
  '2r3c-rfc2698',
  '2r3c-rfc4115',
  '2r3c-mef5cf1',
] as const;

/**
 * Where the qos plugin reads or writes QoS bits: `ip` DSCP (0–63), `vlan` 802.1p PCP (0–7), `mpls` EXP (0–7),
 * `ext` an external/opaque value (0–255) carried in the buffer only.
 */
export const QOS_SOURCES = ['ext', 'vlan', 'mpls', 'ip'] as const;
export type QosSource = (typeof QOS_SOURCES)[number];

/** Highest QoS value each source can carry. */
export const QOS_SOURCE_MAX: Readonly<Record<QosSource, number>> = {
  ext: 255,
  vlan: 7,
  mpls: 7,
  ip: 63,
};

const qosValue = z.int().min(0).max(255);
const burstField = (title: string, help: string) =>
  withUi(z.int().min(0).max(Number.MAX_SAFE_INTEGER), { title, widget: 'number', help });

export const QosPolicerActionSchema = z
  .strictObject({
    action: withUi(z.enum(['transmit', 'drop', 'mark-and-transmit']).default('transmit'), {
      title: 'Action',
      widget: 'select',
    }),
    dscp: withUi(z.int().min(0).max(63), {
      title: 'DSCP',
      widget: 'number',
      help: 'DSCP written by mark-and-transmit',
    }).optional(),
  })
  .superRefine((a, ctx) => {
    if (a.action === 'mark-and-transmit' && a.dscp === undefined) {
      add(ctx, ['dscp'], 'mark-and-transmit needs a DSCP value');
    }
    if (a.action !== 'mark-and-transmit' && a.dscp !== undefined) {
      add(ctx, ['dscp'], 'dscp applies to mark-and-transmit only');
    }
  });

export const QosPolicerSchema = z
  .strictObject({
    description: descriptionField.optional(),
    type: withUi(z.enum(QOS_POLICER_TYPES).default('1r2c'), {
      title: 'Algorithm',
      widget: 'select',
      help: '1r2c = single rate, two colours; 1r3c = RFC 2697; 2r3c = RFC 2698 / RFC 4115 / MEF 5 CF1 (two rates)',
    }),
    rateUnit: withUi(z.enum(['kbps', 'pps']).default('kbps'), {
      title: 'Rate unit',
      widget: 'select',
      help: 'Rates in kbit/s (bursts in bytes) or packets/s (bursts in packets)',
    }),
    cir: withUi(u32Int.min(1), {
      title: 'Committed information rate',
      widget: 'number',
    }),
    eir: withUi(u32Int, {
      title: 'Excess / peak information rate',
      widget: 'number',
      help: 'Two-rate policers only (RFC 2698: peak rate ≥ cir; RFC 4115 / MEF: excess rate)',
    }).optional(),
    cb: burstField('Committed burst', 'Bytes (kbps) or packets (pps)'),
    eb: burstField('Excess burst', 'Three-colour policers only').optional(),
    round: withUi(z.enum(['closest', 'up', 'down']).default('closest'), {
      title: 'Rate rounding',
      widget: 'select',
      help: 'How VPP rounds the configured rate onto its token-bucket parameters',
    }),
    colorAware: withUi(z.boolean().default(false), {
      title: 'Colour aware',
      widget: 'switch',
      help: 'Honour the colour already recorded on the packet (three-colour policers)',
    }),
    conformAction: withUi(QosPolicerActionSchema.prefault({}), { title: 'Conform action' }),
    exceedAction: withUi(QosPolicerActionSchema.prefault({ action: 'drop' }), {
      title: 'Exceed action',
    }),
    violateAction: withUi(QosPolicerActionSchema.prefault({ action: 'drop' }), {
      title: 'Violate action',
      help: 'Three-colour policers only',
    }),
  })
  .superRefine((p, ctx) => {
    const twoRate = p.type.startsWith('2r3c');
    const threeColour = p.type !== '1r2c';
    if (twoRate && p.eir === undefined) add(ctx, ['eir'], `${p.type} needs eir`);
    if (!twoRate && p.eir !== undefined) add(ctx, ['eir'], 'eir applies to two-rate policers only');
    if (threeColour && p.eb === undefined) add(ctx, ['eb'], `${p.type} needs eb`);
    if (!threeColour && p.eb !== undefined)
      add(ctx, ['eb'], 'eb applies to three-colour policers only');
    if (p.type === '2r3c-rfc2698' && p.eir !== undefined && p.eir < p.cir) {
      add(ctx, ['eir'], 'RFC 2698: the peak rate (eir) must not be lower than cir');
    }
    if (p.colorAware && !threeColour) {
      add(ctx, ['colorAware'], 'colour-aware mode applies to three-colour policers only');
    }
    if (!threeColour && p.violateAction.action !== 'drop') {
      add(ctx, ['violateAction'], 'a two-colour policer has no violate action');
    }
  });

/**
 * Egress rate limit. VPP 26.06 has no queueing shaper outside the excluded HQoS scheduler, so the renderer
 * realises a shaper as a single-rate token bucket bound on output (packets above the rate are dropped, not
 * delayed). The object is kept separate from `policers` so a real shaper can replace the implementation later
 * without touching the document.
 */
export const QosShaperSchema = z.strictObject({
  description: descriptionField.optional(),
  rateKbps: withUi(u32Int.min(1), { title: 'Rate (kbit/s)', widget: 'number' }),
  burstBytes: withUi(z.int().min(64).max(Number.MAX_SAFE_INTEGER), {
    title: 'Burst (bytes)',
    widget: 'number',
    help: 'Omit to let the renderer derive it from the rate (≈ 10 ms worth of traffic)',
  }).optional(),
});

/** One row of a QoS egress map: input values (of the recorded source) → output values. */
export const QosMapEntrySchema = z.strictObject({
  from: withUi(qosValue, { title: 'Recorded value', widget: 'number' }),
  to: withUi(qosValue, { title: 'Output value', widget: 'number' }),
});

const mapRow = (source: QosSource) =>
  withUi(z.array(QosMapEntrySchema).max(256), {
    title: `Recorded from ${source}`,
    help: `Translation used when the packet's QoS bits were recorded from ${source} (0–${QOS_SOURCE_MAX[source]}); unlisted values map to 0`,
  }).optional();

export const QosMapSchema = z
  .strictObject({
    description: descriptionField.optional(),
    id: withUi(u32Int, {
      title: 'Map id',
      widget: 'number',
      help: 'VPP qos egress map id; fix it so the renderer never renumbers (unique across maps)',
    }).optional(),
    rows: withUi(
      z.strictObject({
        ext: mapRow('ext'),
        vlan: mapRow('vlan'),
        mpls: mapRow('mpls'),
        ip: mapRow('ip'),
      }),
      { title: 'Rows', help: 'One translation table per recorded source' },
    ).prefault({}),
  })
  .superRefine((m, ctx) => {
    let entries = 0;
    for (const source of QOS_SOURCES) {
      const row = m.rows[source];
      if (row === undefined) continue;
      entries += row.length;
      checkUnique(ctx, row, (e) => String(e.from), ['rows', source], 'recorded value');
      row.forEach((e, i) => {
        if (e.from > QOS_SOURCE_MAX[source]) {
          add(ctx, ['rows', source, i, 'from'], `${source} values are 0–${QOS_SOURCE_MAX[source]}`);
        }
      });
    }
    if (entries === 0) add(ctx, ['rows'], 'a map needs at least one row entry');
  });

const qosObjectRef = (title: string, help: string) => withUi(objectName, { title, help });

/**
 * QoS actions attached to one interface (record key = VPP interface name). Ingress: `policer.input`, `record`,
 * `store`; egress: `policer.output` or `shaper`, `mark`.
 */
export const QosInterfaceSchema = z
  .strictObject({
    description: descriptionField.optional(),
    policer: withUi(
      z.strictObject({
        input: qosObjectRef('Ingress policer', 'Name in qos.policers').optional(),
        output: qosObjectRef('Egress policer', 'Name in qos.policers').optional(),
      }),
      { title: 'Policers' },
    ).optional(),
    shaper: qosObjectRef('Egress shaper', 'Name in qos.shapers').optional(),
    record: withUi(z.enum(QOS_SOURCES), {
      title: 'Record QoS bits from',
      widget: 'select',
      help: 'Ingress: copy the DSCP / PCP / EXP of this header into the packet QoS record (qos record)',
    }).optional(),
    store: withUi(
      z.strictObject({
        source: withUi(z.enum(QOS_SOURCES), { title: 'Source slot', widget: 'select' }),
        value: withUi(qosValue, { title: 'Value', widget: 'number' }),
      }),
      {
        title: 'Store QoS value',
        help: 'Ingress: give every received packet this QoS value for the source slot (qos store)',
      },
    ).optional(),
    mark: withUi(
      z.strictObject({
        map: qosObjectRef('Map', 'Name in qos.maps'),
        output: withUi(z.enum(QOS_SOURCES), {
          title: 'Output header',
          widget: 'select',
          help: 'Header rewritten on egress from the recorded QoS bits through the map (qos mark)',
        }),
      }),
      { title: 'Mark on egress' },
    ).optional(),
  })
  .superRefine((a, ctx) => {
    const hasPolicer = a.policer?.input !== undefined || a.policer?.output !== undefined;
    if (a.policer !== undefined && !hasPolicer) {
      add(ctx, ['policer'], 'name an input and/or an output policer');
    }
    if (
      !hasPolicer &&
      a.shaper === undefined &&
      a.record === undefined &&
      a.store === undefined &&
      a.mark === undefined
    ) {
      add(ctx, [], 'an attachment needs at least one of policer, shaper, record, store, mark');
    }
    if (a.shaper !== undefined && a.policer?.output !== undefined) {
      add(
        ctx,
        ['shaper'],
        'shaper and policer.output are exclusive (one egress token bucket per interface)',
      );
    }
    if (a.store !== undefined && a.store.value > QOS_SOURCE_MAX[a.store.source]) {
      add(
        ctx,
        ['store', 'value'],
        `${a.store.source} values are 0–${QOS_SOURCE_MAX[a.store.source]}`,
      );
    }
    if (a.store !== undefined && a.record === a.store.source) {
      add(ctx, ['store', 'source'], 'record and store cannot target the same source slot');
    }
  });

export const QosSchema = z.strictObject({
  policers: withUi(z.record(objectName, QosPolicerSchema).default({}), {
    title: 'Policers',
    widget: 'record',
  }),
  shapers: withUi(z.record(objectName, QosShaperSchema).default({}), {
    title: 'Shapers',
    widget: 'record',
  }),
  maps: withUi(z.record(objectName, QosMapSchema).default({}), {
    title: 'Marking maps',
    widget: 'record',
    help: 'DSCP / 802.1p / MPLS EXP translation tables used by mark',
  }),
  interfaces: withUi(z.record(vppInterfaceName, QosInterfaceSchema).default({}), {
    title: 'Interface attachments',
    widget: 'record',
    help: 'Keyed by VPP interface name',
  }),
});

// ---------------------------------------------------------------------------------------------------------------
// Domain root
// ---------------------------------------------------------------------------------------------------------------

export const ServicesSchema = withUi(
  z.strictObject({
    dhcp: withUi(DhcpSchema.prefault({}), { title: 'DHCP', group: 'dhcp', order: 1 }),
    dns: withUi(ServicesDnsSchema.prefault({}), { title: 'DNS', group: 'dns', order: 2 }),
    snmp: withUi(SnmpSchema.prefault({}), { title: 'SNMP', group: 'snmp', order: 3 }),
    lldp: withUi(LldpSchema.prefault({}), { title: 'LLDP', group: 'lldp', order: 4 }),
    ipfix: withUi(IpfixSchema.prefault({}), { title: 'IPFIX / sFlow', group: 'ipfix', order: 5 }),
    ntp: withUi(ServicesNtpSchema.prefault({}), { title: 'NTP', group: 'ntp', order: 6 }),
    qos: withUi(QosSchema.prefault({}), { title: 'QoS', group: 'qos', order: 7 }),
    // Feature keys (sub-schema in domains/ext/<slug>.ts): one key line under the feature's anchor.
    // wave-A: F-loopback-bvi-gso-lldp-span
    nsim: servicesNsimField,
    // wave-A: F-rpf-adl-pbr
    hostStack: HostStackSchema.optional(), // F-host-stack (unanchored)
  }),
  {
    title: 'Services',
    description:
      'DHCP (Kea) and relay, DNS (Unbound + VPP cache), SNMP, LLDP, IPFIX/sFlow, NTP (chrony) and QoS (policers, shapers, marking).',
    order: 110,
  },
);

export type ServicesConfig = z.infer<typeof ServicesSchema>;
export type DhcpServer = z.infer<typeof DhcpServerSchema>;
export type DhcpSubnet = z.infer<typeof DhcpSubnetSchema>;
export type DhcpRelay = z.infer<typeof DhcpRelaySchema>;
export type DnsResolver = z.infer<typeof DnsResolverSchema>;
export type ServicesDnsConfig = z.infer<typeof ServicesDnsSchema>;
export type SnmpConfig = z.infer<typeof SnmpSchema>;
export type LldpConfig = z.infer<typeof LldpSchema>;
export type IpfixExporter = z.infer<typeof IpfixExporterSchema>;
export type ServicesNtpConfig = z.infer<typeof ServicesNtpSchema>;
export type QosConfig = z.infer<typeof QosSchema>;
export type QosPolicer = z.infer<typeof QosPolicerSchema>;
export type QosMap = z.infer<typeof QosMapSchema>;
export type QosInterfaceAttachment = z.infer<typeof QosInterfaceSchema>;
