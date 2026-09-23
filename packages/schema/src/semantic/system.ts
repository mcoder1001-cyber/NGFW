import { jsonPointer } from '../pointer.js';
import { vrfExists } from '../domains/vrfs.js';
import { canonicalIp } from '../ip.js';
import type { ValidatorDefinition } from './registry.js';
import { duplicateIssues } from './unique.js';

/**
 * Semantic validators for `system`. Structural rules (RFC 1123 hostname, IANA time zone, banner alphabet) are
 * enforced by the Zod primitives; here only references and uniqueness are checked.
 */
export const systemValidators: readonly ValidatorDefinition[] = [
  {
    name: 'system.vrf-exists',
    domains: ['system', 'vrfs'],
    validate: (config) => {
      const vrf = config.system.dns.vrf;
      return vrfExists(config.vrfs, vrf)
        ? []
        : [
            {
              pointer: jsonPointer('system', 'dns', 'vrf'),
              message: `VRF '${vrf}' does not exist`,
            },
          ];
    },
  },
  {
    name: 'system.dns-server-unique',
    domains: ['system'],
    validate: (config) =>
      duplicateIssues(
        config.system.dns.servers,
        (server) => canonicalIp(server) ?? server,
        (_server, i) => ['system', 'dns', 'servers', i],
        (server) => `name server ${server} is listed more than once`,
      ),
  },
];
