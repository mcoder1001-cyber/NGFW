import { jsonPointer } from '../pointer.js';
import { vrfExists } from '../domains/vrfs.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `system`. Structural rules (RFC 1123 hostname, IANA time zone) are enforced by the Zod
 * primitives; here only cross-domain references are checked.
 */
export const systemValidators: readonly ValidatorDefinition[] = [
  {
    name: 'system.vrf-exists',
    domains: ['system', 'vrfs'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      for (const section of ['ntp', 'dns'] as const) {
        const vrf = config.system[section].vrf;
        if (!vrfExists(config.vrfs, vrf)) {
          issues.push({
            pointer: jsonPointer('system', section, 'vrf'),
            message: `VRF '${vrf}' does not exist`,
          });
        }
      }
      return issues;
    },
  },
  {
    name: 'system.ntp-server-unique',
    domains: ['system'],
    validate: (config) => {
      const issues: SemanticIssue[] = [];
      const seen = new Set<string>();
      for (const [i, server] of config.system.ntp.servers.entries()) {
        const key = server.address.toLowerCase();
        if (seen.has(key)) {
          issues.push({
            pointer: jsonPointer('system', 'ntp', 'servers', i, 'address'),
            message: `NTP server '${server.address}' is listed more than once`,
          });
        }
        seen.add(key);
      }
      return issues;
    },
  },
];
