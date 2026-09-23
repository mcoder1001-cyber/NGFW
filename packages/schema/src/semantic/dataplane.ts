import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/** Semantic validators for `dataplane`: core placement and PCI whitelist consistency. */
export const dataplaneValidators: readonly ValidatorDefinition[] = [
  {
    name: 'dataplane.workers-match-corelist',
    domains: ['dataplane'],
    validate: (config) => {
      const { workers, corelist } = config.dataplane;
      if (workers === undefined || corelist === undefined || workers === corelist.length) return [];
      return [
        {
          pointer: jsonPointer('dataplane', 'workers'),
          message: `workers (${workers}) must equal the number of cores in corelist (${corelist.length})`,
        },
      ];
    },
  },
  {
    name: 'dataplane.corelist-unique',
    domains: ['dataplane'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<number>();
      for (const [i, core] of (config.dataplane.corelist ?? []).entries()) {
        if (seen.has(core)) {
          issues.push({
            pointer: jsonPointer('dataplane', 'corelist', i),
            message: `core ${core} is listed more than once`,
          });
        }
        seen.add(core);
      }
      return issues;
    },
  },
  {
    name: 'dataplane.main-core-not-worker',
    domains: ['dataplane'],
    validate: (config) => {
      const { mainCore, corelist } = config.dataplane;
      if (mainCore === undefined || !(corelist ?? []).includes(mainCore)) return [];
      return [
        {
          pointer: jsonPointer('dataplane', 'mainCore'),
          message: `main core ${mainCore} is also listed as a worker core`,
        },
      ];
    },
  },
  {
    name: 'dataplane.pci-unique',
    domains: ['dataplane'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const [i, pci] of config.dataplane.pciWhitelist.entries()) {
        const key = pci.toLowerCase();
        if (seen.has(key)) {
          issues.push({
            pointer: jsonPointer('dataplane', 'pciWhitelist', i),
            message: `PCI device ${pci} is listed more than once`,
          });
        }
        seen.add(key);
      }
      return issues;
    },
  },
];
