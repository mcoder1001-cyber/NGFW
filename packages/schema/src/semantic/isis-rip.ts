import type { ValidatorDefinition, SemanticIssue } from './registry.js';
import { jsonPointer } from '../pointer.js';
import { prefixKey } from '../ip.js';

export const isisRipValidators: readonly ValidatorDefinition[] = [
  {
    name: 'routing.isis-rip-consistency',
    domains: ['routing'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const isis = config.routing.isis;
      const masks = { 'level-1': 1, 'level-2': 2, 'level-1-2': 3 };
      if (isis)
        for (const [name, iface] of Object.entries(isis.interfaces)) {
          if (!iface.ipv4 && !iface.ipv6)
            issues.push({
              pointer: jsonPointer('routing', 'isis', 'interfaces', name, 'ipv4'),
              message: 'at least one address family must be enabled',
            });
          if (iface.circuitType && masks[iface.circuitType] & ~masks[isis.level])
            issues.push({
              pointer: jsonPointer('routing', 'isis', 'interfaces', name, 'circuitType'),
              message: 'circuit type must be compatible with the IS level',
            });
        }
      const seen = new Set<string>();
      for (const [i, network] of (config.routing.ripng?.networks ?? []).entries()) {
        const key = prefixKey(network);
        if (seen.has(key))
          issues.push({
            pointer: jsonPointer('routing', 'ripng', 'networks', i),
            message: 'network is listed more than once',
          });
        seen.add(key);
      }
      return issues;
    },
  },
];
