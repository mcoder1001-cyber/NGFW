import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';
import { parseCidr, parseIp } from './tunnels-common.js';

/**
 * F-igmp-mfib: cross-checks for `routing.multicast` beyond the schema's own refinements — group addresses are real
 * multicast (224.0.0.0/4) and not the 224.0.0.0/24 link-local control block, SSM joins name sources inside an SSM
 * range, host joins are only on host-mode interfaces, and every referenced interface exists.
 */

const V4MC = parseCidr('224.0.0.0/4')!;
const V4_LINK_LOCAL = parseCidr('224.0.0.0/24')!;

function inRange(addr: string, range: { first: bigint; last: bigint }): boolean {
  const ip = parseIp(addr);
  return ip !== undefined && ip >= range.first && ip <= range.last;
}

function isMulticastGroup(addr: string): boolean {
  return inRange(addr, V4MC) && !inRange(addr, V4_LINK_LOCAL);
}

const multicast: ValidatorDefinition = {
  name: 'routing.multicast',
  domains: ['routing', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const mc = config.routing.multicast;
    if (mc === undefined) return issues;
    const ifaces = interfaceIndex(config);
    const ssm = (mc.igmp?.ssmRanges ?? []).map((c) => parseCidr(c)).filter((c): c is NonNullable<typeof c> => c !== undefined);
    const inSsm = (group: string): boolean => {
      const ip = parseIp(group);
      return ip !== undefined && ssm.some((r) => ip >= r.first && ip <= r.last);
    };

    for (const [ifName, iface] of Object.entries(mc.igmp?.interfaces ?? {})) {
      if (!ifaces.has(ifName)) {
        issues.push({ pointer: jsonPointer('routing', 'multicast', 'igmp', 'interfaces', ifName), message: `interface '${ifName}' does not exist` });
      }
      if (iface.joins.length > 0 && iface.mode !== 'host') {
        issues.push({
          pointer: jsonPointer('routing', 'multicast', 'igmp', 'interfaces', ifName, 'joins'),
          message: 'static joins are only allowed on a host-mode interface',
        });
      }
      iface.joins.forEach((j, i) => {
        if (!isMulticastGroup(j.group)) {
          issues.push({
            pointer: jsonPointer('routing', 'multicast', 'igmp', 'interfaces', ifName, 'joins', i, 'group'),
            message: `'${j.group}' is not a routable multicast group (224.0.0.0/4, excluding 224.0.0.0/24)`,
          });
        }
      });
    }

    for (const [vrf, proxy] of Object.entries(mc.igmp?.proxies ?? {})) {
      for (const [role, list] of [['upstream', [proxy.upstream]], ['downstream', proxy.downstream]] as const) {
        list.forEach((ifName, i) => {
          if (!ifaces.has(ifName)) {
            const path = role === 'upstream'
              ? jsonPointer('routing', 'multicast', 'igmp', 'proxies', vrf, 'upstream')
              : jsonPointer('routing', 'multicast', 'igmp', 'proxies', vrf, 'downstream', i);
            issues.push({ pointer: path, message: `interface '${ifName}' does not exist` });
          }
        });
      }
    }

    mc.mroutes.forEach((r, i) => {
      if (!isMulticastGroup(r.group)) {
        issues.push({
          pointer: jsonPointer('routing', 'multicast', 'mroutes', i, 'group'),
          message: `'${r.group}' is not a routable multicast group (224.0.0.0/4, excluding 224.0.0.0/24)`,
        });
      }
      if (r.source === undefined && inSsm(r.group)) {
        issues.push({
          pointer: jsonPointer('routing', 'multicast', 'mroutes', i, 'source'),
          message: `group '${r.group}' is in an SSM range; a source is required for an (S,G) route`,
        });
      }
      r.paths.forEach((p, j) => {
        if (!ifaces.has(p.interface)) {
          issues.push({ pointer: jsonPointer('routing', 'multicast', 'mroutes', i, 'paths', j, 'interface'), message: `interface '${p.interface}' does not exist` });
        }
      });
    });

    (mc.pim?.interfaces ?? []).forEach((ifName, i) => {
      if (!ifaces.has(ifName)) {
        issues.push({ pointer: jsonPointer('routing', 'multicast', 'pim', 'interfaces', i), message: `interface '${ifName}' does not exist` });
      }
    });
    (mc.pim?.rp ?? []).forEach((rp, i) => {
      rp.groups.forEach((g, j) => {
        const c = parseCidr(g);
        if (c === undefined || c.first < V4MC.first || c.last > V4MC.last) {
          issues.push({ pointer: jsonPointer('routing', 'multicast', 'pim', 'rp', i, 'groups', j), message: `'${g}' is not a multicast group range (inside 224.0.0.0/4)` });
        }
      });
    });

    return issues;
  },
};

export const igmpMfibValidators: readonly ValidatorDefinition[] = [multicast];
