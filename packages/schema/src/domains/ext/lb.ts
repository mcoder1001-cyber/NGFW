import { z } from 'zod';
import { ipAddress, ipNetwork, objectName, vppInterfaceName } from '../../primitives.js';
import { ipFamily, parseCidr, parseIp } from '../../semantic/tunnels-common.js';
import { withUi } from '../../ui.js';
import { transportPort } from '../_shared/primitives.js';

/**
 * `services.lb` — the VPP load-balancer plugin (F-lb, WBS D7.9, tier T3 "solved outside the product by HAProxy/nginx":
 * the minimal, honest version). Virtual IPs (VIPs) whose new flows are spread over application servers (ASes) with
 * VPP's Maglev-style consistent hashing (the new-flows table), encapsulated to the chosen AS by GRE (gre4/gre6), by
 * rewriting the DSCP of the packet for L3 direct server return (l3dsr), or by NAT (nat4/nat6, with the NAT in2out
 * feature on the interfaces in `natInterfaces`). No health checks and no L7: an AS stays in the table until it is
 * removed from the configuration.
 *
 * VPP 26.06 cannot report any of it correctly (V20, D-063): every lb object is write-only, a deleted VIP or AS stays in
 * VPP as "removed" until VPP's lb garbage collection, which only the globals owner triggers (D-090). `settings` is a
 * VPP-global (`lb_conf`, D-071): only the globals owner applies it.
 *
 * Intra-`lb` rules are refinements here (pointers inside `services.lb`): encapsulation ↔ VIP family, server family =
 * encapsulation family, port ↔ protocol, powers of two, unique (prefix, protocol, port), VPP's per-prefix rules (one
 * all-port VIP or per-port VIPs of one encapsulation), NAT VIPs need a port, a target port and a `natInterfaces` entry
 * of their family, and no two NAT VIPs share an (AS address, target port) pair (VPP keys its SNAT mapping by exactly
 * that pair — V20 follow-up). The interface references are checked in `../../semantic/lb.ts` (`services.lb-*`).
 */

const UI_GROUP = 'lb';

export const LB_PROTOCOLS = ['any', 'tcp', 'udp'] as const;
export const LbProtocol = z.enum(LB_PROTOCOLS);
export type LbProtocol = z.infer<typeof LbProtocol>;

/** Encapsulation towards the application servers (VPP `lb_encap_type`). */
export const LB_ENCAPS = ['gre4', 'gre6', 'l3dsr', 'nat4', 'nat6'] as const;
export const LbEncap = z.enum(LB_ENCAPS);
export type LbEncap = z.infer<typeof LbEncap>;

/** Service type of a NAT VIP (VPP `lb_srv_type`): the SNAT source is the VIP (clusterip) or `settings.ip4Source`/`ip6Source` (nodeport). */
export const LB_SRV_TYPES = ['clusterip', 'nodeport'] as const;
export const LbSrvType = z.enum(LB_SRV_TYPES);
export type LbSrvType = z.infer<typeof LbSrvType>;

export const LB_NAT_FAMILIES = ['ip4', 'ip6'] as const;
export const LbNatFamily = z.enum(LB_NAT_FAMILIES);
export type LbNatFamily = z.infer<typeof LbNatFamily>;

/** Bounds: new-flows table entries per VIP (4 bytes each) and sticky-table buckets per worker (64 bytes each). */
export const LB_FLOWS_TABLE_MAX = 1 << 20;
export const LB_FLOW_BUCKETS_MAX = 1 << 20;
export const LB_FLOW_TIMEOUT_MAX = 86400;
export const LB_SERVERS_MAX = 1024;

/** Address family of an encapsulation: the family of its application servers (VPP lb_encap_is_ip4). */
export function lbEncapFamily(encap: LbEncap): 4 | 6 {
  return encap === 'gre6' || encap === 'nat6' ? 6 : 4;
}

/** VIP families an encapsulation allows (VPP lb_vip_add: l3dsr and nat4 are IPv4-only VIPs, nat6 IPv6-only). */
export function lbEncapVipFamilies(encap: LbEncap): readonly (4 | 6)[] {
  if (encap === 'l3dsr' || encap === 'nat4') return [4];
  if (encap === 'nat6') return [6];
  return [4, 6];
}

export function isLbNat(encap: LbEncap): boolean {
  return encap === 'nat4' || encap === 'nat6';
}

const isPowerOfTwo = (n: number): boolean => n > 0 && (n & (n - 1)) === 0;

/** One application server of a VIP. */
export const LbServerSchema = z.strictObject({
  address: withUi(ipAddress, {
    title: 'Address',
    help: 'application server; IPv4 for gre4, l3dsr and nat4, IPv6 for gre6 and nat6',
    order: 1,
  }),
  flushOnDelete: withUi(z.boolean().default(false), {
    title: 'Flush on delete',
    help: 'when the server is removed, also drop its entries from the sticky flow table (existing flows move at once)',
    widget: 'switch',
    order: 2,
  }),
});
export type LbServerConfig = z.infer<typeof LbServerSchema>;

/** One virtual IP (`services.lb.vips.<name>`). */
export const LbVipSchema = z
  .strictObject({
    prefix: withUi(ipNetwork, {
      title: 'VIP prefix',
      help: 'the virtual address, usually a /32 or /128; installed in the default table (table 0)',
      order: 1,
    }),
    protocol: withUi(LbProtocol.default('any'), {
      title: 'Protocol',
      help: 'any = every protocol and port of the prefix (no port); tcp or udp need a port',
      widget: 'select',
      order: 2,
    }),
    port: withUi(transportPort.optional(), {
      title: 'Port',
      help: 'destination port of a tcp/udp VIP; absent for protocol any',
      order: 3,
    }),
    encap: withUi(LbEncap, {
      title: 'Encapsulation',
      help: 'gre4/gre6: GRE tunnel to the server; l3dsr: DSCP rewrite for direct server return (IPv4); nat4/nat6: destination NAT to the server',
      widget: 'select',
      order: 4,
    }),
    dscp: withUi(z.int().min(0).max(63).optional(), {
      title: 'DSCP (l3dsr)',
      help: 'DSCP written into the packet so the server can tell the VIP (l3dsr only; default 0)',
      order: 5,
    }),
    srvType: withUi(LbSrvType.optional(), {
      title: 'Service type (nat)',
      help: 'clusterip: SNAT source is the VIP; nodeport: SNAT source is settings.ip4Source/ip6Source (nat4/nat6 only; default clusterip)',
      widget: 'select',
      order: 6,
    }),
    targetPort: withUi(transportPort.optional(), {
      title: 'Target port (nat)',
      help: 'port on the application server the NAT VIP translates to (nat4/nat6, required)',
      order: 7,
    }),
    nodePort: withUi(transportPort.optional(), {
      title: 'Node port (nat nodeport)',
      help: 'node port of a nodeport VIP; VPP 26.06 accepts but does not use it (the API handler ignores node_port)',
      order: 8,
    }),
    newFlowsTableLength: withUi(z.int().min(1).max(LB_FLOWS_TABLE_MAX).default(1024), {
      title: 'New-flows table length',
      help: 'entries of the Maglev consistent-hash table (a power of two; more entries = finer spread over the servers)',
      order: 9,
    }),
    srcIpSticky: withUi(z.boolean().default(false), {
      title: 'Source-IP sticky',
      help: 'hash on the source address only, so every flow of a client goes to the same server',
      widget: 'switch',
      order: 10,
    }),
    servers: withUi(z.array(LbServerSchema).max(LB_SERVERS_MAX).default([]), {
      title: 'Application servers',
      help: 'the servers new flows are spread over (no health checks: a server stays until removed here)',
      itemKey: ['address'],
      order: 11,
    }),
  })
  .superRefine((v, ctx) => {
    const add = (path: (string | number)[], message: string): void => {
      ctx.addIssue({ code: 'custom', path, message });
    };
    const cidr = parseCidr(v.prefix);
    const vipFamily = cidr?.family;
    if (vipFamily !== undefined && !lbEncapVipFamilies(v.encap).includes(vipFamily)) {
      add(
        ['encap'],
        `encap ${v.encap} needs an IPv${vipFamily === 4 ? 6 : 4} VIP (VPP supports no ${v.encap} VIP on IPv${vipFamily})`,
      );
    }
    if ((v.protocol === 'any') !== (v.port === undefined)) {
      add(
        ['port'],
        v.protocol === 'any'
          ? 'protocol any covers every port: remove the port or choose tcp/udp'
          : `a ${v.protocol} VIP needs a port`,
      );
    }
    if (!isPowerOfTwo(v.newFlowsTableLength)) {
      add(['newFlowsTableLength'], `${v.newFlowsTableLength} is not a power of two`);
    }
    if (v.dscp !== undefined && v.encap !== 'l3dsr')
      add(['dscp'], 'dscp applies to l3dsr VIPs only');
    if (isLbNat(v.encap)) {
      if (v.protocol === 'any')
        add(
          ['protocol'],
          `a ${v.encap} VIP needs protocol tcp or udp and a port (VPP has no all-port NAT VIP)`,
        );
      if (v.targetPort === undefined) add(['targetPort'], `a ${v.encap} VIP needs a target port`);
      if (v.nodePort !== undefined && v.srvType !== 'nodeport')
        add(['nodePort'], 'nodePort applies to srvType nodeport only');
    } else {
      for (const k of ['srvType', 'targetPort', 'nodePort'] as const) {
        if (v[k] !== undefined) add([k], `${k} applies to nat4/nat6 VIPs only`);
      }
    }
    const want = lbEncapFamily(v.encap);
    const seen = new Set<string>();
    v.servers.forEach((s, i) => {
      if (ipFamily(s.address) !== want) {
        add(
          ['servers', i, 'address'],
          `encap ${v.encap} needs IPv${want} application servers, ${s.address} is IPv${ipFamily(s.address)}`,
        );
      }
      const k = String(parseIp(s.address) ?? s.address);
      if (seen.has(k)) add(['servers', i, 'address'], `duplicate application server ${s.address}`);
      seen.add(k);
    });
  });
export type LbVipConfig = z.infer<typeof LbVipSchema>;

/** `services.lb.settings` — VPP-global (`lb_conf`): applied only by the globals owner (D-071). */
export const LbSettingsSchema = z
  .strictObject({
    ip4Source: withUi(z.ipv4().optional(), {
      title: 'IPv4 source',
      help: 'outer source of gre4 packets and SNAT source of nat4 nodeport VIPs (VPP default 255.255.255.255)',
      widget: 'ip',
      order: 1,
    }),
    ip6Source: withUi(z.ipv6().optional(), {
      title: 'IPv6 source',
      help: 'outer source of gre6 packets and SNAT source of nat6 nodeport VIPs (VPP default ffff:…:ffff)',
      widget: 'ip',
      order: 2,
    }),
    flowBuckets: withUi(z.int().min(1).max(LB_FLOW_BUCKETS_MAX).optional(), {
      title: 'Sticky-table buckets per worker',
      help: 'a power of two (VPP default 1024; unset keeps the current value)',
      order: 3,
    }),
    flowTimeoutSec: withUi(z.int().min(1).max(LB_FLOW_TIMEOUT_MAX).optional(), {
      title: 'Flow timeout (s)',
      help: 'idle time after which a flow may move to another server (VPP default 40; unset keeps the current value)',
      order: 4,
    }),
  })
  .superRefine((s, ctx) => {
    if (s.flowBuckets !== undefined && !isPowerOfTwo(s.flowBuckets)) {
      ctx.addIssue({
        code: 'custom',
        path: ['flowBuckets'],
        message: `${s.flowBuckets} is not a power of two`,
      });
    }
  });
export type LbSettingsConfig = z.infer<typeof LbSettingsSchema>;

/** The NAT in2out feature of one interface (return traffic of nat4/nat6 VIPs is translated back on it). */
export const LbNatInterfaceSchema = z.strictObject({
  interface: withUi(vppInterfaceName, {
    title: 'Interface',
    help: 'interface the application servers answer on (VPP lb-nat4-in2out / lb-nat6-in2out feature)',
    widget: 'interface-picker',
    order: 1,
  }),
  family: withUi(LbNatFamily, { title: 'Family', widget: 'select', order: 2 }),
});
export type LbNatInterfaceConfig = z.infer<typeof LbNatInterfaceSchema>;

/** Canonical identity of a VIP: VPP keys VIPs by (prefix, protocol, port). */
function vipIdentity(v: LbVipConfig): string | undefined {
  const c = parseCidr(v.prefix);
  if (c === undefined) return undefined;
  return `${c.family}:${c.first}/${c.prefixLength}`;
}

/** `services.lb`. */
export const LbSchema = z
  .strictObject({
    settings: withUi(LbSettingsSchema.optional(), {
      title: 'Settings (global)',
      help: 'VPP-wide lb settings; applied only by the globals owner',
      order: 1,
    }),
    vips: withUi(z.record(objectName, LbVipSchema).default({}), {
      title: 'Virtual IPs',
      widget: 'record',
      help: 'keyed by name',
      order: 2,
    }),
    natInterfaces: withUi(z.array(LbNatInterfaceSchema).max(64).default([]), {
      title: 'NAT interfaces',
      help: 'interfaces with the lb NAT in2out feature (needed by nat4/nat6 VIPs)',
      itemKey: ['interface', 'family'],
      order: 3,
    }),
  })
  .superRefine((lb, ctx) => {
    const add = (path: (string | number)[], message: string): void => {
      ctx.addIssue({ code: 'custom', path, message });
    };
    const seenNat = new Set<string>();
    lb.natInterfaces.forEach((n, i) => {
      const k = `${n.interface}|${n.family}`;
      if (seenNat.has(k))
        add(['natInterfaces', i], `duplicate NAT interface ${n.interface} (${n.family})`);
      seenNat.add(k);
    });
    const byTuple = new Map<string, string>();
    const byPrefix = new Map<string, { name: string; allPort: boolean; encap: LbEncap }>();
    const snat = new Map<string, string>();
    for (const name of Object.keys(lb.vips).sort()) {
      const v = lb.vips[name];
      if (v === undefined) continue;
      const id = vipIdentity(v);
      if (id === undefined) continue;
      const tuple = `${id}|${v.protocol}|${v.port ?? 0}`;
      const dup = byTuple.get(tuple);
      if (dup !== undefined) {
        add(
          ['vips', name, 'prefix'],
          `VIP ${v.prefix} ${v.protocol}${v.port !== undefined ? `/${v.port}` : ''} is also VIP '${dup}'`,
        );
      } else {
        byTuple.set(tuple, name);
      }
      const allPort = v.port === undefined;
      const other = byPrefix.get(id);
      if (other === undefined) {
        byPrefix.set(id, { name, allPort, encap: v.encap });
      } else if (dup === undefined) {
        if (other.allPort || allPort) {
          add(
            ['vips', name, 'port'],
            `prefix ${v.prefix} has an all-port VIP and a per-port VIP ('${other.name}'): VPP allows one or the other`,
          );
        } else if (other.encap !== v.encap) {
          add(
            ['vips', name, 'encap'],
            `every VIP of prefix ${v.prefix} must use one encapsulation ('${other.name}' uses ${other.encap})`,
          );
        }
      }
      if (isLbNat(v.encap)) {
        const fam = v.encap === 'nat4' ? 'ip4' : 'ip6';
        if (!lb.natInterfaces.some((n) => n.family === fam)) {
          add(
            ['vips', name, 'encap'],
            `a ${v.encap} VIP needs a natInterfaces entry with family ${fam} (the interface the servers answer on)`,
          );
        }
        if (v.targetPort !== undefined) {
          v.servers.forEach((s, i) => {
            const k = `${String(parseIp(s.address) ?? s.address)}|${v.targetPort}`;
            const prev = snat.get(k);
            if (prev !== undefined && prev !== name) {
              add(
                ['vips', name, 'servers', i, 'address'],
                `${s.address} port ${v.targetPort} is already a target of NAT VIP '${prev}' (VPP keys the SNAT mapping by server address and target port only)`,
              );
            } else {
              snat.set(k, name);
            }
          });
        }
      }
    }
  });
export type LbConfig = z.infer<typeof LbSchema>;

/** The `services.lb` key line (`ServicesConfig` field 11). */
export const servicesLbField = withUi(LbSchema.optional(), {
  title: 'Load balancer',
  description:
    'VPP lb plugin: virtual IPs spread over application servers (GRE, L3DSR or NAT; Maglev hashing). Tier T3: no health checks, no L7. Write-only in VPP 26.06; deleted VIPs linger until the lb garbage collection (V20).',
  group: UI_GROUP,
  order: 30,
});
