import {
  interfaceNames,
  type InterfaceConfig,
  type SubinterfaceConfig,
} from '../domains/interfaces.js';
import type { BridgeL2PortConfig } from '../domains/ext/bridge-l2.js';
import { vrfExists } from '../domains/vrfs.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic rules of F-bridge-l2 (`interfaces.<if>.l2`, `routing.l2`; D-109 c). Pointers are built with
 * `jsonPointer()` because VPP interface names contain `/`. The "second membership" of an interface is always the
 * one reported: bridge membership (the per-port leaf) counts first, then the L2 cross-connect, then the L3
 * cross-connect.
 */

/** One (sub-)interface of the document with its pointer path and L2 leaf. */
interface Port {
  name: string;
  path: readonly (string | number)[];
  value: InterfaceConfig | SubinterfaceConfig;
  l2: BridgeL2PortConfig | undefined;
  sub: boolean;
}

function ports(config: RootConfig): Port[] {
  const out: Port[] = [];
  for (const [parent, iface] of Object.entries(config.interfaces)) {
    out.push({
      name: parent,
      path: ['interfaces', parent],
      value: iface,
      l2: iface.l2,
      sub: false,
    });
    for (const [id, sub] of Object.entries(iface.subinterfaces)) {
      out.push({
        name: `${parent}.${id}`,
        path: ['interfaces', parent, 'subinterfaces', id],
        value: sub,
        l2: sub.l2,
        sub: true,
      });
    }
  }
  return out;
}

const L2 = ['routing', 'l2'] as const;
const LOOPBACK = /^loop[0-9]+$/;

function l2Of(config: RootConfig) {
  return config.routing.l2;
}

/** Members of each bridge domain (by record name), in document order. */
function membersByDomain(config: RootConfig): Map<string, Port[]> {
  const out = new Map<string, Port[]>();
  for (const p of ports(config)) {
    const bd = p.l2?.bridgeDomain;
    if (bd === undefined) continue;
    const list = out.get(bd) ?? [];
    list.push(p);
    out.set(bd, list);
  }
  return out;
}

export const bridgeL2Validators: readonly ValidatorDefinition[] = [
  {
    name: 'interfaces.bridge-l2-domain-exists',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const bds = l2Of(config)?.bridgeDomains ?? {};
      return ports(config)
        .filter((p) => p.l2?.bridgeDomain !== undefined && !Object.hasOwn(bds, p.l2.bridgeDomain))
        .map((p) => ({
          pointer: jsonPointer(...p.path, 'l2', 'bridgeDomain'),
          message: `bridge domain '${p.l2?.bridgeDomain}' does not exist under /routing/l2/bridgeDomains`,
        }));
    },
  },
  {
    // an interface is in at most one bridge domain or cross-connect: the second membership is reported
    name: 'interfaces.bridge-l2-single-membership',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const l2 = l2Of(config);
      const bridged = new Map<string, Port>();
      for (const p of ports(config)) if (p.l2?.bridgeDomain !== undefined) bridged.set(p.name, p);
      const xc = new Set<string>();
      for (const rx of Object.keys(l2?.xconnects ?? {})) {
        xc.add(rx);
        const member = bridged.get(rx);
        if (member !== undefined) {
          issues.push({
            pointer: jsonPointer(...L2, 'xconnects', rx),
            message: `${rx} is already a member of bridge domain '${member.l2?.bridgeDomain}' (${jsonPointer(...member.path, 'l2', 'bridgeDomain')}); an interface is in at most one bridge domain or cross-connect`,
          });
        }
      }
      for (const rx of Object.keys(l2?.l3xc ?? {})) {
        const member = bridged.get(rx);
        if (member !== undefined || xc.has(rx)) {
          issues.push({
            pointer: jsonPointer(...L2, 'l3xc', rx),
            message:
              member !== undefined
                ? `${rx} is a member of bridge domain '${member.l2?.bridgeDomain}'; an L3 cross-connect needs an L3 interface`
                : `${rx} is already the receive side of an L2 cross-connect; an L3 cross-connect needs an L3 interface`,
          });
        }
      }
      return issues;
    },
  },
  {
    // a bridged port (except the BVI) and an L2 cross-connect rx carry no L3 configuration
    name: 'interfaces.bridge-l2-no-l3-when-bridged',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const xc = new Set(Object.keys(l2Of(config)?.xconnects ?? {}));
      for (const p of ports(config)) {
        const bridged = p.l2?.bridgeDomain !== undefined && p.l2.bvi !== true;
        if (!bridged && !xc.has(p.name)) continue;
        const role = bridged
          ? `a member of bridge domain '${p.l2?.bridgeDomain}'`
          : 'an L2 cross-connect rx';
        const v = p.value;
        if (v.ipv4.length > 0) {
          issues.push({
            pointer: jsonPointer(...p.path, 'ipv4', 0),
            message: `${p.name} is ${role} and cannot have IPv4 addresses (only the BVI is routed)`,
          });
        }
        if (v.ipv6.length > 0) {
          issues.push({
            pointer: jsonPointer(...p.path, 'ipv6', 0),
            message: `${p.name} is ${role} and cannot have IPv6 addresses (only the BVI is routed)`,
          });
        }
        if (v.vrf !== 'default') {
          issues.push({
            pointer: jsonPointer(...p.path, 'vrf'),
            message: `${p.name} is ${role}; the VRF of an L2 port must stay 'default'`,
          });
        }
        if (v.unnumbered !== undefined) {
          issues.push({
            pointer: jsonPointer(...p.path, 'unnumbered'),
            message: `${p.name} is ${role} and cannot be IP-unnumbered`,
          });
        }
        if (v.dhcpClient !== undefined) {
          issues.push({
            pointer: jsonPointer(...p.path, 'dhcpClient'),
            message: `${p.name} is ${role} and cannot run a DHCP client`,
          });
        }
      }
      return issues;
    },
  },
  {
    // shg / bvi / uuFwd describe a bridge membership; bvi and uuFwd are exclusive
    name: 'interfaces.bridge-l2-port-role',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const p of ports(config)) {
        const l2 = p.l2;
        if (l2 === undefined) continue;
        if (l2.bridgeDomain === undefined) {
          for (const [field, set] of [
            ['shg', l2.shg !== 0],
            ['bvi', l2.bvi],
            ['uuFwd', l2.uuFwd],
          ] as const) {
            if (set) {
              issues.push({
                pointer: jsonPointer(...p.path, 'l2', field),
                message: `${field} needs a bridge domain (l2.bridgeDomain)`,
              });
            }
          }
        }
        if (l2.bvi && l2.uuFwd) {
          issues.push({
            pointer: jsonPointer(...p.path, 'l2', 'uuFwd'),
            message: 'a member is either the BVI or the uu-fwd port, not both',
          });
        }
        if (l2.bvi && l2.shg !== 0) {
          issues.push({
            pointer: jsonPointer(...p.path, 'l2', 'shg'),
            message: 'the BVI has no split-horizon group',
          });
        }
      }
      return issues;
    },
  },
  {
    // one BVI per bridge domain, and the BVI is a loopback; one uu-fwd port per bridge domain
    name: 'interfaces.bridge-l2-one-bvi',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [bd, members] of membersByDomain(config)) {
        for (const role of ['bvi', 'uuFwd'] as const) {
          const holders = members.filter((m) => m.l2?.[role] === true);
          for (const extra of holders.slice(1)) {
            issues.push({
              pointer: jsonPointer(...extra.path, 'l2', role),
              message: `bridge domain '${bd}' already has ${role === 'bvi' ? 'a BVI' : 'a uu-fwd port'} (${holders[0]?.name}); only one is allowed`,
            });
          }
        }
        for (const m of members) {
          if (m.l2?.bvi === true && (m.sub || !LOOPBACK.test(m.name))) {
            issues.push({
              pointer: jsonPointer(...m.path, 'l2', 'bvi'),
              message: `the BVI must be a loopback interface (loop<N>), not ${m.name}`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    // tag rewrite only on an L2 port: a bridge member (not the BVI) or an L2 cross-connect rx
    name: 'interfaces.bridge-l2-tag-rewrite-l2-only',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const xc = new Set(Object.keys(l2Of(config)?.xconnects ?? {}));
      const issues: SemanticIssue[] = [];
      for (const p of ports(config)) {
        const tr = p.l2?.tagRewrite;
        if (tr === undefined) continue;
        const member = p.l2?.bridgeDomain !== undefined && p.l2.bvi !== true;
        if (!member && !xc.has(p.name)) {
          issues.push({
            pointer: jsonPointer(...p.path, 'l2', 'tagRewrite'),
            message: `VLAN tag rewrite needs an L2 port: make ${p.name} a (non-BVI) bridge member or an L2 cross-connect rx`,
          });
        }
        // pop / translate need that many tags on the port: an untagged parent can only push
        const tags = p.sub
          ? (p.value as SubinterfaceConfig).innerVlanId === undefined
            ? 1
            : 2
          : 0;
        const pops =
          tr.op === 'pop-1' || tr.op.startsWith('translate-1')
            ? 1
            : tr.op === 'pop-2' || tr.op.startsWith('translate-2')
              ? 2
              : 0;
        if (pops > tags) {
          issues.push({
            pointer: jsonPointer(...p.path, 'l2', 'tagRewrite', 'op'),
            message: `${tr.op} removes ${pops} tag(s) but ${p.name} matches ${tags} tag(s)`,
          });
        }
      }
      return issues;
    },
  },
  {
    // mactime runs on hardware interfaces only (mactime_enable_disable rejects sub-interfaces)
    name: 'interfaces.bridge-l2-mac-filter-parent',
    domains: ['interfaces'],
    validate: (config) =>
      ports(config)
        .filter((p) => p.sub && p.l2?.macFilter === true)
        .map((p) => ({
          pointer: jsonPointer(...p.path, 'l2', 'macFilter'),
          message: `the MAC filter runs on parent (hardware) interfaces only; enable it on ${p.name.slice(0, p.name.lastIndexOf('.'))}`,
        })),
  },
  {
    name: 'routing.bridge-l2-domain-id-unique',
    domains: ['routing'],
    validate: (config) => {
      const seen = new Map<number, string>();
      const issues: SemanticIssue[] = [];
      for (const [name, bd] of Object.entries(l2Of(config)?.bridgeDomains ?? {})) {
        const other = seen.get(bd.id);
        if (other !== undefined) {
          issues.push({
            pointer: jsonPointer(...L2, 'bridgeDomains', name, 'id'),
            message: `bridge-domain id ${bd.id} is already used by '${other}'`,
          });
        } else {
          seen.set(bd.id, name);
        }
      }
      return issues;
    },
  },
  {
    // static MACs: unique per bridge domain, reached through a member of that bridge domain
    name: 'routing.bridge-l2-static-mac',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const members = membersByDomain(config);
      for (const [name, bd] of Object.entries(l2Of(config)?.bridgeDomains ?? {})) {
        const names = new Set((members.get(name) ?? []).map((m) => m.name));
        const macs = new Set<string>();
        for (const [i, e] of bd.staticMacs.entries()) {
          const mac = e.mac.toLowerCase();
          if (macs.has(mac)) {
            issues.push({
              pointer: jsonPointer(...L2, 'bridgeDomains', name, 'staticMacs', i, 'mac'),
              message: `MAC ${e.mac} is listed twice in bridge domain '${name}'`,
            });
          }
          macs.add(mac);
          if (!names.has(e.interface)) {
            issues.push({
              pointer: jsonPointer(...L2, 'bridgeDomains', name, 'staticMacs', i, 'interface'),
              message: `${e.interface} is not a member of bridge domain '${name}'`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.bridge-l2-xconnect',
    domains: ['interfaces', 'routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const known = interfaceNames(config.interfaces);
      for (const [rx, x] of Object.entries(l2Of(config)?.xconnects ?? {})) {
        if (!known.has(rx)) {
          issues.push({
            pointer: jsonPointer(...L2, 'xconnects', rx),
            message: `interface ${rx} does not exist under /interfaces`,
          });
        }
        if (x.tx === rx) {
          issues.push({
            pointer: jsonPointer(...L2, 'xconnects', rx, 'tx'),
            message: 'a cross-connect cannot transmit on its own receive interface (rx ≠ tx)',
          });
        } else if (!known.has(x.tx)) {
          issues.push({
            pointer: jsonPointer(...L2, 'xconnects', rx, 'tx'),
            message: `interface ${x.tx} does not exist under /interfaces`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'routing.bridge-l2-l3xc',
    domains: ['interfaces', 'routing', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const known = interfaceNames(config.interfaces);
      for (const [rx, x] of Object.entries(l2Of(config)?.l3xc ?? {})) {
        const base = [...L2, 'l3xc', rx] as const;
        if (!known.has(rx)) {
          issues.push({
            pointer: jsonPointer(...base),
            message: `interface ${rx} does not exist under /interfaces`,
          });
        }
        if (x.ipv4Paths.length === 0 && x.ipv6Paths.length === 0) {
          issues.push({
            pointer: jsonPointer(...base),
            message: 'an L3 cross-connect needs at least one IPv4 or IPv6 path',
          });
        }
        for (const [family, list] of [
          ['ipv4Paths', x.ipv4Paths],
          ['ipv6Paths', x.ipv6Paths],
        ] as const) {
          for (const [i, p] of list.entries()) {
            if (p.nextHop !== undefined && p.nextHop.includes(':') !== (family === 'ipv6Paths')) {
              issues.push({
                pointer: jsonPointer(...base, family, i, 'nextHop'),
                message: `${p.nextHop} is not an ${family === 'ipv6Paths' ? 'IPv6' : 'IPv4'} address`,
              });
            }
            if (p.interface !== undefined && !known.has(p.interface)) {
              issues.push({
                pointer: jsonPointer(...base, family, i, 'interface'),
                message: `interface ${p.interface} does not exist under /interfaces`,
              });
            }
            if (!vrfExists(config.vrfs, p.vrf)) {
              issues.push({
                pointer: jsonPointer(...base, family, i, 'vrf'),
                message: `VRF '${p.vrf}' does not exist`,
              });
            }
          }
        }
      }
      return issues;
    },
  },
  {
    // VPP keys the mactime device table by MAC; days of one range are distinct
    name: 'routing.bridge-l2-mac-filter',
    domains: ['routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const macs = new Map<string, string>();
      for (const [name, f] of Object.entries(l2Of(config)?.macFilters ?? {})) {
        const mac = f.mac.toLowerCase();
        const other = macs.get(mac);
        if (other !== undefined) {
          issues.push({
            pointer: jsonPointer(...L2, 'macFilters', name, 'mac'),
            message: `MAC ${f.mac} already belongs to device '${other}'`,
          });
        } else {
          macs.set(mac, name);
        }
        for (const [i, r] of f.ranges.entries()) {
          if (new Set(r.days).size !== r.days.length) {
            issues.push({
              pointer: jsonPointer(...L2, 'macFilters', name, 'ranges', i, 'days'),
              message: 'a day is listed twice',
            });
          }
        }
      }
      return issues;
    },
  },
];
