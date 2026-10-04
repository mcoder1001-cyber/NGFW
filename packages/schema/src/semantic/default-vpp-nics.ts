import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * F-default-vpp-nics (D-164): keep a physical interface's `physical.owner` and the DPDK whitelist consistent.
 * A `owner: 'dataplane'` NIC's PCI address must be in `dataplane.pciWhitelist`/`devices` (it is handed to VPP); a
 * `owner: 'host'` NIC's must not be (releasing it to the host removes it from DPDK). Pointer: the offending
 * interface's `physical.owner`. This is how release / reclaim stays coherent (a merge patch flips both).
 */
export const defaultVppNicsValidators: readonly ValidatorDefinition[] = [
  {
    name: 'dataplane.owner-consistent',
    domains: ['dataplane', 'interfaces'],
    validate: (config) => {
      const { pciWhitelist, devices } = config.dataplane;
      const dpdk = new Set([...pciWhitelist, ...Object.keys(devices)].map((p) => p.toLowerCase()));
      const issues: SemanticIssue[] = [];
      for (const [name, itf] of Object.entries(config.interfaces)) {
        const physical = itf.physical;
        if (physical === undefined) continue;
        const inDpdk = dpdk.has(physical.pci.toLowerCase());
        if (physical.owner === 'dataplane' && !inDpdk) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'physical', 'owner'),
            message: `NIC ${name} (${physical.pci}) is owned by the engine but in neither dataplane.pciWhitelist nor dataplane.devices: add it there, or release it (physical.owner = host)`,
          });
        } else if (physical.owner === 'host' && inDpdk) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'physical', 'owner'),
            message: `NIC ${name} (${physical.pci}) is released to the host but still in dataplane.pciWhitelist or dataplane.devices: remove it there, or reclaim it (physical.owner = dataplane)`,
          });
        }
      }
      return issues;
    },
  },
  {
    // review R1R3 #9 (D-069): the DPDK device's logical name is the interface key, and one NIC is one row
    name: 'dataplane.physical-name-matches-device',
    domains: ['dataplane', 'interfaces'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const devByPci = new Map(
        Object.entries(config.dataplane.devices).map(([pci, d]) => [pci.toLowerCase(), { pci, d }]),
      );
      const rowByPci = new Map<string, string>();
      for (const [name, itf] of Object.entries(config.interfaces)) {
        const physical = itf.physical;
        if (physical === undefined) continue;
        const key = physical.pci.toLowerCase();
        const prev = rowByPci.get(key);
        if (prev !== undefined) {
          issues.push({
            pointer: jsonPointer('interfaces', name, 'physical', 'pci'),
            message: `NIC ${physical.pci} is already the physical row ${prev}`,
          });
          continue;
        }
        rowByPci.set(key, name);
        const dev = devByPci.get(key);
        if (physical.owner === 'dataplane' && dev?.d.name !== undefined && dev.d.name !== name) {
          issues.push({
            pointer: jsonPointer('dataplane', 'devices', dev.pci, 'name'),
            message: `the logical name of ${dev.pci} must be ${name} (its interface), not ${dev.d.name}`,
          });
        }
      }
      return issues;
    },
  },
];
