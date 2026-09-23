import {
  interfaceNames,
  type SubinterfaceConfig,
  type InterfaceConfig,
} from '../domains/interfaces.js';
import { vrfExists } from '../domains/vrfs.js';
import { parseCidr, prefixesOverlap, type IpPrefix } from '../ip.js';
import { jsonPointer } from '../pointer.js';
import type { RootConfig } from '../index.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `interfaces`: VRF references resolve, no overlapping addresses inside a VRF, VLAN tags
 * unique per parent, `unnumbered` targets exist, sub-interface MTU ≤ parent MTU, MAC addresses unique. Pointers are built with `jsonPointer()`
 * because VPP interface names contain `/`.
 */

/** One (sub-)interface with the pointer segments that lead to it. */
interface Node {
  name: string;
  path: readonly (string | number)[];
  value: InterfaceConfig | SubinterfaceConfig;
}

/** Every interface and sub-interface of the document, parents first. */
function nodes(config: RootConfig): Node[] {
  const out: Node[] = [];
  for (const [parent, iface] of Object.entries(config.interfaces)) {
    out.push({ name: parent, path: ['interfaces', parent], value: iface });
    for (const [id, sub] of Object.entries(iface.subinterfaces)) {
      out.push({
        name: `${parent}.${id}`,
        path: ['interfaces', parent, 'subinterfaces', id],
        value: sub,
      });
    }
  }
  return out;
}

interface Assigned {
  node: Node;
  pointer: string;
  text: string;
  prefix: IpPrefix;
}

export const interfacesValidators: readonly ValidatorDefinition[] = [
  {
    name: 'interfaces.vrf-exists',
    domains: ['interfaces', 'vrfs'],
    validate: (config) =>
      nodes(config)
        .filter((n) => !vrfExists(config.vrfs, n.value.vrf))
        .map((n) => ({
          pointer: jsonPointer(...n.path, 'vrf'),
          message: `VRF '${n.value.vrf}' does not exist`,
        })),
  },
  {
    name: 'interfaces.address-no-overlap',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      // addresses grouped by VRF; a prefix only conflicts with prefixes in the same table
      const byVrf = new Map<string, Assigned[]>();
      for (const node of nodes(config)) {
        for (const family of ['ipv4', 'ipv6'] as const) {
          for (const [i, text] of node.value[family].entries()) {
            const prefix = parseCidr(text);
            if (prefix === undefined) continue; // cannot happen after schema validation; never crash
            const list = byVrf.get(node.value.vrf) ?? [];
            byVrf.set(node.value.vrf, list);
            const pointer = jsonPointer(...node.path, family, i);
            // the same interface may carry several addresses of one subnet, but not the same address twice
            const other = list.find(
              (o) => prefixesOverlap(o.prefix, prefix) && (o.node !== node || o.text === text),
            );
            if (other !== undefined) {
              issues.push({
                pointer,
                message:
                  other.text === text
                    ? `address ${text} is already assigned at ${other.pointer}`
                    : `${text} overlaps with ${other.text} on ${other.node.name} (${other.pointer}) in VRF '${node.value.vrf}'`,
              });
            }
            list.push({ node, pointer, text, prefix });
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.vlan-unique',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [parent, iface] of Object.entries(config.interfaces)) {
        const seen = new Map<string, string>();
        for (const [id, sub] of Object.entries(iface.subinterfaces)) {
          const tag = `${sub.dot1ad ? 'dot1ad' : 'dot1q'} ${sub.vlanId}${sub.innerVlanId === undefined ? '' : `.${sub.innerVlanId}`}`;
          const first = seen.get(tag);
          if (first === undefined) seen.set(tag, id);
          else {
            issues.push({
              pointer: jsonPointer('interfaces', parent, 'subinterfaces', id, 'vlanId'),
              message: `VLAN ${tag} is already used by sub-interface ${parent}.${first}`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.unnumbered-target-exists',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const names = interfaceNames(config.interfaces);
      for (const node of nodes(config)) {
        const target = node.value.unnumbered;
        if (target === undefined) continue;
        if (target === node.name) {
          issues.push({
            pointer: jsonPointer(...node.path, 'unnumbered'),
            message: 'an interface cannot be unnumbered to itself',
          });
        } else if (!names.has(target)) {
          issues.push({
            pointer: jsonPointer(...node.path, 'unnumbered'),
            message: `interface '${target}' does not exist`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.subinterface-mtu',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [parent, iface] of Object.entries(config.interfaces)) {
        if (iface.mtu === undefined) continue;
        for (const [id, sub] of Object.entries(iface.subinterfaces)) {
          if (sub.mtu !== undefined && sub.mtu > iface.mtu) {
            issues.push({
              pointer: jsonPointer('interfaces', parent, 'subinterfaces', id, 'mtu'),
              message: `sub-interface MTU ${sub.mtu} exceeds the MTU ${iface.mtu} of ${parent}`,
            });
          }
        }
      }
      return issues;
    },
  },
  {
    name: 'interfaces.mac-unique',
    domains: ['interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<string, string>();
      for (const [name, iface] of Object.entries(config.interfaces)) {
        if (iface.mac === undefined) continue;
        const mac = iface.mac.toLowerCase().replaceAll('-', ':');
        const first = seen.get(mac);
        if (first === undefined) seen.set(mac, name);
        else {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'mac'),
            message: `MAC address ${mac} is already used by ${first}`,
          });
        }
      }
      return issues;
    },
  },
];
