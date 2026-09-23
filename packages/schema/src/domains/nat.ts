import { z } from 'zod';
import { ipAddress, ipv4Cidr, ipv6Cidr, objectName, vppInterfaceName } from '../primitives.js';
import { withUi } from '../ui.js';
import { ipPrefix, l4PortNumber } from './objects.js';

/**
 * `nat` — every translator VPP 26.06 offers (WBS D4.1–D4.7; TNSR "NAT" is the reference).
 *
 * The top level of `nat` IS NAT44 (docs/04 `mode, inside, outside, …`; prompts/features/F-nat44-ed-sessions.md
 * `mode: "ed", inside, outside, pools, staticMappings, timeouts, sessionLimit`). The other translators are
 * siblings so the API path stays `/config/nat/<translator>/…`:
 *
 *   nat44 (top level)  enabled, mode ed|ei, inside[], outside[], outputFeature[], insideVrf?, outsideVrf?,
 *                      forwarding, staticMappingOnly, connectionTracking, sessionLimit?, pools[], staticMappings[]
 *                      (1:1 when no ports, port-forward with ports), identityMappings[], loadBalancedMappings[],
 *                      timeouts { udp, tcpEstablished, tcpTransitory, icmp }
 *   ipfix              NAT logging (CGNAT logging, D4.4) — plugin-wide
 *   nat64              prefixes[], pools[], staticBibs[], inside/outside, timeouts
 *   nat66              inside/outside, staticMappings[]
 *   nptv6              bindings[] { interface, internal, external }
 *   det44              deterministic CGNAT: mappings[] { inside, outside }, inside/outside, timeouts
 *   dslite             AFTR/B4 addresses, pools[]
 *   map                MAP-E / MAP-T / LW4o6 domains[] + rules, parameters
 *   cnat               translations[] (1:1 or load-balanced) + snat policy (D4.6)
 *
 * Addresses in NAT are literal (a pool is an IPv4 range string `a.b.c.d-a.b.c.e`, F-nat44 shape). Cross-field
 * and cross-item rules — interfaces exist and inside/outside are disjoint, pools ordered and non-overlapping,
 * external port requires protocol, VRFs exist … — live in `../semantic/nat.ts`. 464XLAT = a `nat64` PLAT here plus
 * a `map` MAP-T domain on the CLAT side; nothing extra to model.
 *
 * Guardrail (vdom.md #1): every binding that VPP scopes by FIB carries `vrf` (a name from `vrfs`; `default`
 * is implicit) — never assume VRF 0.
 *
 * Only P02b edits this file.
 */

// ---------------------------------------------------------------------------------------------------------------
// Local primitives (P02a owns primitives.ts — see docs/status/tasks/P02b-questions.md). `ipv4Address` and
// `ipv6Address` are deliberately NOT exported: P02a's primitives.ts exports the same names and index.ts re-exports
// every module with `export *` (two identical names would be TS2308).
// ---------------------------------------------------------------------------------------------------------------

const ipv4Address = withUi(z.ipv4(), { title: 'IPv4 address', widget: 'ip' });
const ipv6Address = withUi(z.ipv6(), { title: 'IPv6 address', widget: 'ip' });

const IPV4_RANGE_RE = /^(\d{1,3}(?:\.\d{1,3}){3})(?:-(\d{1,3}(?:\.\d{1,3}){3}))?$/;

/** Split `"a.b.c.d-a.b.c.e"` (or a single `"a.b.c.d"`) into its ends; `undefined` if it is not that shape. */
export function splitIpv4Range(text: string): { start: string; end: string } | undefined {
  const m = IPV4_RANGE_RE.exec(text);
  if (m === null || m[1] === undefined) return undefined;
  return { start: m[1], end: m[2] ?? m[1] };
}

/**
 * Inclusive IPv4 address range `10.5.0.10-10.5.0.20` (or one address). Ordering (start ≤ end) and overlaps between
 * pools are semantic rules so they report a pointer to the offending pool.
 */
export const ipv4AddressRange = withUi(
  z
    .string()
    .regex(IPV4_RANGE_RE, 'expected an IPv4 range like 10.0.0.10-10.0.0.20')
    .refine(
      (s) => {
        const r = splitIpv4Range(s);
        return (
          r !== undefined &&
          z.ipv4().safeParse(r.start).success &&
          z.ipv4().safeParse(r.end).success
        );
      },
      { message: 'both ends must be valid IPv4 addresses' },
    ),
  { title: 'Address range', widget: 'ip-range', help: '10.0.0.10-10.0.0.20' },
);

const description = withUi(z.string().max(255), { title: 'Description', widget: 'textarea' });
const enabled = withUi(z.boolean().default(false), { title: 'Enabled' });
const flag = (title: string, help?: string) =>
  withUi(z.boolean().default(false), help === undefined ? { title } : { title, help });

/** VRF reference (name from `vrfs`; `default` always exists). */
const vrf = withUi(objectName, { title: 'VRF', widget: 'vrf-picker' }).optional();

const interfaceList = (title: string, help: string) =>
  withUi(z.array(vppInterfaceName).max(1024).default([]), {
    title,
    widget: 'interface-picker',
    help,
  });

const timeoutSeconds = (title: string, dflt: number) =>
  withUi(z.number().int().min(1).max(2_147_483_647).default(dflt), {
    title,
    widget: 'number',
    help: 'seconds',
  });

/** Session timeouts (VPP defaults: udp 300, tcp-established 7440, tcp-transitory 240, icmp 60). */
export const NatTimeoutsSchema = withUi(
  z.strictObject({
    udp: timeoutSeconds('UDP', 300),
    tcpEstablished: timeoutSeconds('TCP established', 7440),
    tcpTransitory: timeoutSeconds('TCP transitory', 240),
    icmp: timeoutSeconds('ICMP', 60),
  }),
  { title: 'Timeouts' },
);

const natProtocol = withUi(z.enum(['tcp', 'udp', 'icmp']), { title: 'Protocol' });

// ---------------------------------------------------------------------------------------------------------------
// NAT44 (top level)
// ---------------------------------------------------------------------------------------------------------------

export const NatPoolSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    description: description.optional(),
    range: ipv4AddressRange,
    vrf,
    twiceNat: flag('Twice-NAT pool', 'Addresses used for the source side of twice-NAT mappings'),
  }),
  { title: 'Address pool' },
);

/**
 * Static mapping (F-nat44-ed-sessions §Contract: `{ name, local{ip,port?}, external{ip|pool, port?}, protocol?,
 * vrf?, twiceNat? }`). No ports = 1:1 NAT for the whole address; `local.port` + `external.port` + `protocol` =
 * port forward. `external` names exactly one of: a literal `ip`, a `pool` from `nat.pools` (the renderer uses the
 * pool's first address — VPP mappings take one address) or an `interface` whose address is used (VPP external
 * sw_if_index; e.g. a DHCP-assigned WAN). Cross-field rules are in `../semantic/nat.ts` (`nat.static-mappings`).
 */
export const NatStaticMappingSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    description: description.optional(),
    protocol: natProtocol.optional(),
    local: withUi(
      z.strictObject({
        ip: withUi(ipv4Address, { title: 'Local address' }),
        port: withUi(l4PortNumber, { title: 'Local port' }).optional(),
      }),
      { title: 'Local' },
    ),
    external: withUi(
      z.strictObject({
        ip: withUi(ipv4Address, {
          title: 'External address',
          help: 'Exactly one of ip / pool / interface',
        }).optional(),
        pool: withUi(objectName, {
          title: 'External pool',
          widget: 'object-picker',
          help: 'Name of an entry in nat.pools; its first address is used',
        }).optional(),
        interface: withUi(vppInterfaceName, {
          title: 'External interface',
          help: 'Use this interface’s address',
        }).optional(),
        port: withUi(l4PortNumber, {
          title: 'External port',
          help: 'Requires protocol and local.port',
        }).optional(),
      }),
      { title: 'External' },
    ),
    vrf,
    twiceNat: flag('Twice-NAT'),
    selfTwiceNat: flag('Self twice-NAT', 'Twice-NAT only when the source is the external address'),
    out2inOnly: flag('Out-to-in only'),
  }),
  { title: 'Static mapping' },
);

/** Identity mapping: an address (or interface address) + optional protocol/port that is never translated. */
export const NatIdentityMappingSchema = withUi(
  z.strictObject({
    description: description.optional(),
    ip: withUi(ipv4Address, { title: 'Address' }).optional(),
    interface: withUi(vppInterfaceName, {
      title: 'Interface',
      help: 'Exactly one of ip / interface',
    }).optional(),
    protocol: natProtocol.optional(),
    port: withUi(l4PortNumber, { title: 'Port', help: 'Requires protocol' }).optional(),
    vrf,
  }),
  { title: 'Identity mapping' },
);

/** Load-balanced static mapping: one external ip:port spread over weighted local endpoints. */
export const NatLoadBalancedMappingSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    description: description.optional(),
    protocol: withUi(z.enum(['tcp', 'udp']), { title: 'Protocol' }),
    external: withUi(
      z.strictObject({
        ip: withUi(ipv4Address, { title: 'External address' }),
        port: withUi(l4PortNumber, { title: 'External port' }),
      }),
      { title: 'External' },
    ),
    locals: withUi(
      z
        .array(
          z.strictObject({
            ip: withUi(ipv4Address, { title: 'Local address' }),
            port: withUi(l4PortNumber, { title: 'Local port' }),
            probability: withUi(z.number().int().min(1).max(255).default(1), {
              title: 'Weight',
            }),
            vrf,
          }),
        )
        .min(1)
        .max(256),
      { title: 'Local endpoints' },
    ),
    affinity: withUi(z.number().int().min(0).max(2_147_483_647).default(0), {
      title: 'Affinity',
      help: 'Client affinity in seconds; 0 = none',
    }),
    twiceNat: flag('Twice-NAT'),
    selfTwiceNat: flag('Self twice-NAT'),
    out2inOnly: flag('Out-to-in only'),
  }),
  { title: 'Load-balanced mapping' },
);

/** NAT IPFIX logging (nat44 / det44 / nat64 events). */
export const NatIpfixSchema = withUi(
  z.strictObject({
    enabled,
    domainId: withUi(z.number().int().min(1).max(4_294_967_295), {
      title: 'Observation domain id',
    }).optional(),
    sourcePort: withUi(l4PortNumber, { title: 'Source port' }).optional(),
  }),
  { title: 'IPFIX logging' },
);

// ---------------------------------------------------------------------------------------------------------------
// NAT64 / NAT66 / NPTv6
// ---------------------------------------------------------------------------------------------------------------

export const Nat64Schema = withUi(
  z.strictObject({
    enabled,
    inside: interfaceList('Inside (IPv6)', 'IPv6-only side'),
    outside: interfaceList('Outside (IPv4)', 'IPv4 side'),
    prefixes: withUi(
      z
        .array(
          z.strictObject({
            prefix: withUi(ipv6Cidr, {
              title: 'NAT64 prefix',
              help: 'RFC 6052 length 32/40/48/56/64/96; default 64:ff9b::/96',
            }),
            vrf,
          }),
        )
        .max(256)
        .default([]),
      { title: 'Prefixes' },
    ),
    pools: withUi(
      z
        .array(z.strictObject({ range: ipv4AddressRange, vrf }))
        .max(256)
        .default([]),
      { title: 'IPv4 pools' },
    ),
    staticBibs: withUi(
      z
        .array(
          z.strictObject({
            description: description.optional(),
            protocol: natProtocol,
            inside: withUi(
              z.strictObject({
                ip: withUi(ipv6Address, { title: 'IPv6 address' }),
                port: withUi(l4PortNumber, { title: 'Port' }),
              }),
              { title: 'Inside (IPv6)' },
            ),
            outside: withUi(
              z.strictObject({
                ip: withUi(ipv4Address, { title: 'IPv4 address' }),
                port: withUi(l4PortNumber, { title: 'Port' }),
              }),
              { title: 'Outside (IPv4)' },
            ),
            vrf,
          }),
        )
        .default([]),
      { title: 'Static BIB entries' },
    ),
    timeouts: NatTimeoutsSchema.prefault({}),
  }),
  { title: 'NAT64' },
);

export const Nat66Schema = withUi(
  z.strictObject({
    enabled,
    inside: interfaceList('Inside', ''),
    outside: interfaceList('Outside', ''),
    staticMappings: withUi(
      z
        .array(
          z.strictObject({
            description: description.optional(),
            local: withUi(ipv6Address, { title: 'Local address' }),
            external: withUi(ipv6Address, { title: 'External address' }),
            vrf,
          }),
        )
        .default([]),
      { title: 'Static mappings' },
    ),
  }),
  { title: 'NAT66' },
);

/** NPTv6 (RFC 6296): stateless prefix translation bound to an interface; internal/external lengths must match. */
export const Nptv6Schema = withUi(
  z.strictObject({
    bindings: withUi(
      z
        .array(
          z.strictObject({
            description: description.optional(),
            interface: vppInterfaceName,
            internal: withUi(ipv6Cidr, { title: 'Internal prefix' }),
            external: withUi(ipv6Cidr, { title: 'External prefix' }),
          }),
        )
        .max(1024)
        .default([]),
      { title: 'Bindings' },
    ),
  }),
  { title: 'NPTv6' },
);

// ---------------------------------------------------------------------------------------------------------------
// DET44, DS-Lite, MAP
// ---------------------------------------------------------------------------------------------------------------

/** Deterministic CGNAT: each inside prefix maps algorithmically onto an outside prefix (no per-user state). */
export const Det44Schema = withUi(
  z.strictObject({
    enabled,
    inside: interfaceList('Inside', ''),
    outside: interfaceList('Outside', ''),
    insideVrf: withUi(objectName, { title: 'Inside VRF', widget: 'vrf-picker' }).optional(),
    outsideVrf: withUi(objectName, { title: 'Outside VRF', widget: 'vrf-picker' }).optional(),
    mappings: withUi(
      z
        .array(
          z.strictObject({
            description: description.optional(),
            inside: withUi(ipv4Cidr, { title: 'Inside prefix' }),
            outside: withUi(ipv4Cidr, {
              title: 'Outside prefix',
              help: 'Length ≥ inside length; ≤ 15 bits difference',
            }),
          }),
        )
        .max(1024)
        .default([]),
      { title: 'Mappings' },
    ),
    timeouts: NatTimeoutsSchema.prefault({}),
  }),
  { title: 'Deterministic NAT (DET44)' },
);

export const DsliteSchema = withUi(
  z.strictObject({
    enabled,
    aftr: withUi(
      z.strictObject({
        ipv6: withUi(ipv6Address, { title: 'AFTR IPv6 address' }),
        ipv4: withUi(ipv4Address, { title: 'AFTR IPv4 address' }).optional(),
      }),
      { title: 'AFTR' },
    ).optional(),
    b4: withUi(
      z.strictObject({
        ipv6: withUi(ipv6Address, { title: 'B4 IPv6 address' }),
        ipv4: withUi(ipv4Address, { title: 'B4 IPv4 address' }).optional(),
      }),
      { title: 'B4 (CE mode)' },
    ).optional(),
    pools: withUi(
      z
        .array(z.strictObject({ range: ipv4AddressRange }))
        .max(256)
        .default([]),
      {
        title: 'IPv4 pools',
      },
    ),
  }),
  { title: 'DS-Lite' },
);

/** One MAP domain (RFC 7597 MAP-E, RFC 7599 MAP-T, RFC 7596 lw4o6 = MAP-E with per-subscriber rules). */
export const MapDomainSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    description: description.optional(),
    mode: withUi(z.enum(['map-e', 'map-t', 'lw4o6']), { title: 'Mode' }),
    ipv4Prefix: withUi(ipv4Cidr, { title: 'Rule IPv4 prefix' }),
    ipv6Prefix: withUi(ipv6Cidr, { title: 'Rule IPv6 prefix' }),
    ipv6Source: withUi(ipv6Cidr, { title: 'BR IPv6 source / DMR prefix' }),
    eaBitsLength: withUi(z.number().int().min(0).max(64).default(0), {
      title: 'EA bits length',
      help: 'ipv6Prefix length + EA bits ≤ 64',
    }),
    psidOffset: withUi(z.number().int().min(0).max(16).default(0), { title: 'PSID offset' }),
    psidLength: withUi(z.number().int().min(0).max(16).default(0), {
      title: 'PSID length',
      help: 'offset + length ≤ 16',
    }),
    mtu: withUi(z.number().int().min(1280).max(9216), { title: 'MTU' }).optional(),
    rules: withUi(
      z
        .array(
          z.strictObject({
            psid: withUi(z.number().int().min(0).max(65535), { title: 'PSID' }),
            ipv6Destination: withUi(ipv6Address, { title: 'IPv6 destination' }),
          }),
        )
        .max(65536)
        .default([]),
      { title: 'Rules', help: 'Per-PSID destinations (lw4o6 / MAP-E with rules)' },
    ),
  }),
  { title: 'MAP domain' },
);

export const MapParametersSchema = withUi(
  z.strictObject({
    fragmentation: withUi(
      z
        .strictObject({
          inner: flag('Fragment inner packet'),
          ignoreDf: flag('Ignore DF bit'),
        })
        .prefault({}),
      { title: 'Fragmentation' },
    ),
    icmpSourceAddress: withUi(ipv4Address, { title: 'ICMP relay source address' }).optional(),
    icmp6Unreachables: flag('Send ICMPv6 unreachables'),
    securityCheck: withUi(
      z
        .strictObject({
          enabled: flag('Enabled'),
          fragments: flag('Check fragments'),
        })
        .prefault({}),
      { title: 'Security check' },
    ),
    tcpMss: withUi(z.number().int().min(0).max(65535), { title: 'TCP MSS clamp' }).optional(),
    trafficClass: withUi(
      z
        .strictObject({
          copy: flag('Copy traffic class'),
          value: withUi(z.number().int().min(0).max(255), { title: 'Value' }).optional(),
        })
        .prefault({}),
      { title: 'Traffic class' },
    ),
    preResolve: withUi(
      z
        .strictObject({
          ipv4: withUi(ipv4Address, { title: 'IPv4 next hop' }).optional(),
          ipv6: withUi(ipv6Address, { title: 'IPv6 next hop' }).optional(),
        })
        .prefault({}),
      { title: 'Pre-resolve next hops' },
    ),
  }),
  { title: 'MAP parameters' },
);

export const MapSchema = withUi(
  z.strictObject({
    domains: withUi(z.array(MapDomainSchema).max(4096).default([]), { title: 'Domains' }),
    parameters: MapParametersSchema.prefault({}),
  }),
  { title: 'MAP-E / MAP-T / lw4o6' },
);

// ---------------------------------------------------------------------------------------------------------------
// CNAT
// ---------------------------------------------------------------------------------------------------------------

const endpoint = (title: string) =>
  withUi(
    z.strictObject({
      ip: withUi(ipAddress, { title: 'Address' }),
      port: withUi(l4PortNumber, { title: 'Port' }),
    }),
    { title },
  );

/** CNAT translation: a VIP mapped to one backend (policy 1:1 DNAT) or several (load-balanced). */
export const CnatTranslationSchema = withUi(
  z.strictObject({
    name: withUi(objectName, { title: 'Name' }),
    description: description.optional(),
    protocol: withUi(z.enum(['tcp', 'udp']), { title: 'Protocol' }),
    vip: endpoint('Virtual endpoint'),
    backends: withUi(z.array(endpoint('Backend')).min(1).max(1024), { title: 'Backends' }),
    lbType: withUi(z.enum(['default', 'maglev']).default('default'), { title: 'Load balancing' }),
  }),
  { title: 'CNAT translation' },
);

export const CnatSchema = withUi(
  z.strictObject({
    translations: withUi(z.array(CnatTranslationSchema).max(65536).default([]), {
      title: 'Translations',
    }),
    snat: withUi(
      z
        .strictObject({
          policy: withUi(z.enum(['none', 'interface', 'k8s']).default('none'), {
            title: 'SNAT policy',
            help: 'interface = SNAT on outside interfaces; k8s = Kubernetes semantics',
          }),
          addresses: withUi(
            z
              .strictObject({
                ipv4: withUi(ipv4Address, { title: 'IPv4 SNAT address' }).optional(),
                ipv6: withUi(ipv6Address, { title: 'IPv6 SNAT address' }).optional(),
              })
              .prefault({}),
            { title: 'SNAT addresses' },
          ),
          interfaces: withUi(
            z
              .array(
                z.strictObject({
                  interface: vppInterfaceName,
                  side: withUi(z.enum(['inside', 'outside']), { title: 'Side' }),
                }),
              )
              .max(1024)
              .default([]),
            { title: 'Policy interfaces' },
          ),
          excludePrefixes: withUi(z.array(ipPrefix).max(1024).default([]), {
            title: 'Excluded prefixes',
          }),
        })
        .prefault({}),
      { title: 'SNAT' },
    ),
  }),
  { title: 'CNAT' },
);

// ---------------------------------------------------------------------------------------------------------------
// Domain
// ---------------------------------------------------------------------------------------------------------------

export const NatSchema = withUi(
  z.strictObject({
    enabled: withUi(z.boolean().default(false), {
      title: 'NAT44 enabled',
      group: 'General',
      order: 1,
    }),
    mode: withUi(z.enum(['ed', 'ei']).default('ed'), {
      title: 'Mode',
      group: 'General',
      order: 2,
      help: 'ed = endpoint-dependent (nat44-ed), ei = endpoint-independent (nat44-ei)',
    }),
    inside: withUi(interfaceList('Inside interfaces', 'Local side'), {
      group: 'General',
      order: 3,
    }),
    outside: withUi(interfaceList('Outside interfaces', 'Public side'), {
      group: 'General',
      order: 4,
    }),
    outputFeature: withUi(
      interfaceList('Output-feature interfaces', 'NAT applied on the output path (post-routing)'),
      { group: 'General', order: 5 },
    ),
    insideVrf: withUi(objectName, {
      title: 'Inside VRF',
      widget: 'vrf-picker',
      group: 'General',
    }).optional(),
    outsideVrf: withUi(objectName, {
      title: 'Outside VRF',
      widget: 'vrf-picker',
      group: 'General',
    }).optional(),
    forwarding: withUi(flag('Forwarding', 'Forward traffic that matches no NAT session'), {
      group: 'General',
    }),
    staticMappingOnly: withUi(flag('Static mappings only'), { group: 'General' }),
    connectionTracking: withUi(flag('Connection tracking'), { group: 'General' }),
    sessionLimit: withUi(z.number().int().min(1024).max(2_147_483_647), {
      title: 'Session limit',
      group: 'General',
      help: 'Max sessions per worker thread; omit = VPP default',
    }).optional(),
    pools: withUi(z.array(NatPoolSchema).max(1024).default([]), {
      title: 'Address pools',
      group: 'Pools',
      order: 10,
    }),
    staticMappings: withUi(z.array(NatStaticMappingSchema).max(65536).default([]), {
      title: 'Static mappings & port forwards',
      group: 'Static',
      order: 11,
    }),
    identityMappings: withUi(z.array(NatIdentityMappingSchema).max(4096).default([]), {
      title: 'Identity mappings',
      group: 'Static',
      order: 12,
    }),
    loadBalancedMappings: withUi(z.array(NatLoadBalancedMappingSchema).max(4096).default([]), {
      title: 'Load-balanced mappings',
      group: 'Static',
      order: 13,
    }),
    timeouts: withUi(NatTimeoutsSchema.prefault({}), { group: 'Timeouts', order: 20 }),
    ipfix: withUi(NatIpfixSchema.prefault({}), { group: 'Logging', order: 21 }),
    nat64: withUi(Nat64Schema.prefault({}), { group: 'NAT64', order: 30 }),
    nat66: withUi(Nat66Schema.prefault({}), { group: 'NAT66', order: 31 }),
    nptv6: withUi(Nptv6Schema.prefault({}), { group: 'NPTv6', order: 32 }),
    det44: withUi(Det44Schema.prefault({}), { group: 'CGNAT', order: 40 }),
    dslite: withUi(DsliteSchema.prefault({}), { group: 'CGNAT', order: 41 }),
    map: withUi(MapSchema.prefault({}), { group: 'CGNAT', order: 42 }),
    cnat: withUi(CnatSchema.prefault({}), { group: 'CNAT', order: 50 }),
  }),
  {
    title: 'NAT',
    description:
      'NAT44 (ED/EI) with pools, static/port-forward/identity/load-balanced mappings; NAT64, NAT66, NPTv6, DET44 CGNAT, DS-Lite, MAP-E/T, lw4o6 and CNAT.',
    order: 60,
  },
);

export type NatConfig = z.infer<typeof NatSchema>;
export type NatPool = z.infer<typeof NatPoolSchema>;
export type NatStaticMapping = z.infer<typeof NatStaticMappingSchema>;
export type NatIdentityMapping = z.infer<typeof NatIdentityMappingSchema>;
export type NatLoadBalancedMapping = z.infer<typeof NatLoadBalancedMappingSchema>;
export type NatTimeouts = z.infer<typeof NatTimeoutsSchema>;
export type Nat64Config = z.infer<typeof Nat64Schema>;
export type Nat66Config = z.infer<typeof Nat66Schema>;
export type Nptv6Config = z.infer<typeof Nptv6Schema>;
export type Det44Config = z.infer<typeof Det44Schema>;
export type DsliteConfig = z.infer<typeof DsliteSchema>;
export type MapDomain = z.infer<typeof MapDomainSchema>;
export type MapConfig = z.infer<typeof MapSchema>;
export type CnatTranslation = z.infer<typeof CnatTranslationSchema>;
export type CnatConfig = z.infer<typeof CnatSchema>;
