import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';

/**
 * Semantic validators of `services.lb` (F-lb) that need other domains: every NAT interface exists. The rules inside
 * `services.lb` (families, ports, powers of two, uniqueness, NAT prerequisites) are refinements of the schema in
 * `../domains/ext/lb.ts`.
 */

const natInterfaceExists: ValidatorDefinition = {
  name: 'services.lb-nat-interface-exists',
  domains: ['services', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const lb = config.services.lb;
    if (lb === undefined) return issues;
    const index = interfaceIndex(config);
    lb.natInterfaces.forEach((n, i) => {
      if (!index.has(n.interface)) {
        issues.push({
          pointer: jsonPointer('services', 'lb', 'natInterfaces', i, 'interface'),
          message: `interface '${n.interface}' does not exist`,
        });
      }
    });
    return issues;
  },
};

export const lbValidators: readonly ValidatorDefinition[] = [natInterfaceExists];
