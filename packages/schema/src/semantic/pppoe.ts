import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-pppoe-client cross-object rules (list-internal shapes are in `../domains/ext/pppoe.ts`):
 *  - a PPPoE client carries no static addresses of its own — the peer assigns them (IPCP / IPv6);
 *  - the `parent` interface (when set) exists and is enabled;
 *  - the PPPoE MTU fits the parent link: ≤ parent MTU − 8 (PPPoE 6 + PPP 2), checked only when the parent MTU is set;
 *  - IPv6 on (slaac/dhcpv6) needs the IPv6 minimum link MTU, 1280 (RFC 8200 §5; the agent renderer refuses less).
 * The password reference is validated by the secret tier at commit, not here.
 */

const PPPOE_OVERHEAD = 8;
const IPV6_MIN_MTU = 1280;

const pppoeRules: ValidatorDefinition = {
  name: 'interfaces.pppoe',
  domains: ['interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const ifaces = config.interfaces;
    const owners = new Map<string, string>();
    for (const [name, iface] of Object.entries(ifaces)) {
      const pppoe = iface.pppoe;
      if (!pppoe) continue;
      const at = (...seg: (string | number)[]) => jsonPointer('interfaces', name, 'pppoe', ...seg);

      if (iface.ipv4.length > 0 || iface.ipv6.length > 0) {
        issues.push({
          pointer: jsonPointer('interfaces', name, 'ipv4'),
          message: `interface '${name}' is a PPPoE client: its address comes from the peer, so it must not carry static addresses`,
        });
      }

      if (pppoe.ipv6 !== 'off' && pppoe.mtu < IPV6_MIN_MTU) {
        issues.push({
          pointer: at('mtu'),
          message: `PPPoE MTU ${pppoe.mtu} is below the IPv6 minimum link MTU ${IPV6_MIN_MTU}; raise it or set ipv6 to off`,
        });
      }

      const targets = pppoe.delegationTargets;
      if (targets.length && pppoe.ipv6 !== 'dhcpv6') {
        issues.push({
          pointer: at('delegationTargets'),
          message: 'LAN delegation requires DHCPv6',
        });
      }
      const subnets = new Set<number>();
      const lans = new Set<string>();
      targets.forEach((target, index) => {
        const pointer = at('delegationTargets', index);
        const lan = ifaces[target.interface];
        if (lans.has(target.interface) || subnets.has(target.subnetId)) {
          issues.push({
            pointer,
            message: 'delegated LAN interfaces and subnet IDs must be unique',
          });
        }
        lans.add(target.interface);
        subnets.add(target.subnetId);
        if (
          !lan ||
          !lan.enabled ||
          lan.pppoe ||
          target.interface === name ||
          target.interface === pppoe.parent
        ) {
          issues.push({
            pointer,
            message:
              'delegation target must be an existing enabled LAN, distinct from the PPP client and its parent',
          });
        } else {
          if (lan.ipv6.length || lan.ipv6Ra !== undefined) {
            issues.push({
              pointer,
              message:
                'delegated LAN must not have static IPv6 addresses or router advertisement configuration',
            });
          }
          if (lan.vrf !== iface.vrf) {
            issues.push({
              pointer,
              message: 'delegated LAN and PPP client must belong to the same VRF',
            });
          }
        }
        const previous = owners.get(target.interface);
        if (previous && previous !== name) {
          issues.push({
            pointer,
            message: `delegated LAN is already assigned to PPP client '${previous}'`,
          });
        }
        owners.set(target.interface, name);
      });

      const parentName = pppoe.parent;
      let parentMtu = iface.mtu;
      if (parentName !== undefined && parentName !== name) {
        const parent = ifaces[parentName];
        if (parent === undefined) {
          issues.push({
            pointer: at('parent'),
            message: `interface '${parentName}' does not exist`,
          });
          continue;
        }
        if (!parent.enabled) {
          issues.push({ pointer: at('parent'), message: `interface '${parentName}' is disabled` });
        }
        parentMtu = parent.mtu;
      }
      if (parentMtu !== undefined && pppoe.mtu > parentMtu - PPPOE_OVERHEAD) {
        issues.push({
          pointer: at('mtu'),
          message: `PPPoE MTU ${pppoe.mtu} does not fit the ${parentMtu}-byte link (at most ${parentMtu - PPPOE_OVERHEAD}, leaving ${PPPOE_OVERHEAD} bytes for PPPoE + PPP)`,
        });
      }
    }
    return issues;
  },
};

export const pppoeValidators: readonly ValidatorDefinition[] = [pppoeRules];
