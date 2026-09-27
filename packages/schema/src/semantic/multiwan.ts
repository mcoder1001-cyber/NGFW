import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';

/**
 * F-multiwan cross-object rules (shapes are in `../domains/ext/multiwan.ts`): WAN group names are unique, and every
 * member interface exists. A member's next-hop gateway is checked by the schema refine; the routing/NAT interplay is
 * the agent's.
 */
const multiwanRules: ValidatorDefinition = {
  name: 'routing.wan-groups',
  domains: ['routing', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const groups = config.routing.wanGroups ?? [];
    const index = interfaceIndex(config);
    const seen = new Set<string>();
    groups.forEach((g, gi) => {
      if (seen.has(g.name)) {
        issues.push({
          pointer: jsonPointer('routing', 'wanGroups', gi, 'name'),
          message: `WAN group '${g.name}' is defined twice`,
        });
      }
      seen.add(g.name);
      g.members.forEach((m, mi) => {
        if (!index.has(m.interface)) {
          issues.push({
            pointer: jsonPointer('routing', 'wanGroups', gi, 'members', mi, 'interface'),
            message: `interface '${m.interface}' does not exist`,
          });
        }
      });
    });
    return issues;
  },
};

export const multiwanValidators: readonly ValidatorDefinition[] = [multiwanRules];
