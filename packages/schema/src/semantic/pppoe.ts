import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-pppoe-client cross-object rules (list-internal shapes are in `../domains/ext/pppoe.ts`):
 *  - a PPPoE client carries no static addresses of its own — the peer assigns them (IPCP / IPv6);
 *  - the `parent` interface (when set) exists and is enabled;
 *  - the PPPoE MTU fits the parent link: ≤ parent MTU − 8 (PPPoE 6 + PPP 2), checked only when the parent MTU is set.
 * The password reference is validated by the secret tier at commit, not here.
 */

const PPPOE_OVERHEAD = 8;

const pppoeRules: ValidatorDefinition = {
  name: 'interfaces.pppoe',
  domains: ['interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const ifaces = config.interfaces;
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
