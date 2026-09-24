import { jsonPointer } from '../pointer.js';
import { DEFAULT_VRF } from '../domains/vrfs.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `vrfs`: table ids are unique and table 0 is reserved for `default`.
 * Reference checks ("this VRF exists") live with the referencing domain (`interfaces.vrf-exists`, …).
 */
export const vrfsValidators: readonly ValidatorDefinition[] = [
  {
    name: 'vrfs.id-unique',
    domains: ['vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<number, string>();
      for (const [name, vrf] of Object.entries(config.vrfs)) {
        const first = seen.get(vrf.id);
        if (first === undefined) seen.set(vrf.id, name);
        else {
          issues.push({
            pointer: jsonPointer('vrfs', name, 'id'),
            message: `table id ${vrf.id} is already used by VRF '${first}'`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'vrfs.default-is-table-zero',
    domains: ['vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [name, vrf] of Object.entries(config.vrfs)) {
        if (name === DEFAULT_VRF && vrf.id !== 0) {
          issues.push({
            pointer: jsonPointer('vrfs', name, 'id'),
            message: `the '${DEFAULT_VRF}' VRF is VPP table 0 and cannot use id ${vrf.id}`,
          });
        } else if (name !== DEFAULT_VRF && vrf.id === 0) {
          issues.push({
            pointer: jsonPointer('vrfs', name, 'id'),
            message: `table id 0 is the '${DEFAULT_VRF}' VRF; choose another id for '${name}'`,
          });
        }
      }
      return issues;
    },
  },
];
