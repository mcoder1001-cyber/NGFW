import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/** Semantic validators for `dataplane`: core placement, PCI whitelist/devices/management consistency. */
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
  {
    name: 'dataplane.devices-pci-unique',
    domains: ['dataplane'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<string, string>();
      for (const pci of Object.keys(config.dataplane.devices)) {
        const key = pci.toLowerCase();
        const prev = seen.get(key);
        if (prev !== undefined) {
          issues.push({
            pointer: jsonPointer('dataplane', 'devices', pci),
            message: `PCI device ${pci} is the same device as ${prev}`,
          });
        }
        seen.set(key, pci);
      }
      return issues;
    },
  },
  {
    name: 'dataplane.management-not-dpdk',
    domains: ['dataplane'],
    validate: (config) => {
      const { managementPci, pciWhitelist, devices } = config.dataplane;
      const dpdk = new Set([...pciWhitelist, ...Object.keys(devices)].map((p) => p.toLowerCase()));
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const [i, pci] of managementPci.entries()) {
        const key = pci.toLowerCase();
        if (seen.has(key)) {
          issues.push({
            pointer: jsonPointer('dataplane', 'managementPci', i),
            message: `management NIC ${pci} is listed more than once`,
          });
        } else if (dpdk.has(key)) {
          issues.push({
            pointer: jsonPointer('dataplane', 'managementPci', i),
            message: `management NIC ${pci} must never be a DPDK device (remove it from pciWhitelist/devices)`,
          });
        }
        seen.add(key);
      }
      return issues;
    },
  },
  {
    name: 'dataplane.logical-name-unique',
    domains: ['dataplane'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Map<string, string>();
      for (const [pci, dev] of Object.entries(config.dataplane.devices)) {
        if (dev.name === undefined) continue;
        const prev = seen.get(dev.name);
        if (prev !== undefined) {
          issues.push({
            pointer: jsonPointer('dataplane', 'devices', pci, 'name'),
            message: `logical name ${dev.name} is already used by ${prev}`,
          });
        }
        seen.set(dev.name, pci);
      }
      return issues;
    },
  },
  {
    name: 'dataplane.descriptors-power-of-two',
    domains: ['dataplane'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const [pci, dev] of Object.entries(config.dataplane.devices)) {
        for (const field of ['rxDesc', 'txDesc'] as const) {
          const v = dev[field];
          if (v !== undefined && (v & (v - 1)) !== 0) {
            issues.push({
              pointer: jsonPointer('dataplane', 'devices', pci, field),
              message: `${field} ${v} must be a power of two`,
            });
          }
        }
      }
      return issues;
    },
  },
];
