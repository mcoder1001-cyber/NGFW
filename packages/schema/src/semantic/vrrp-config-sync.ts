import type { ValidatorDefinition } from './registry.js';
import { interfaceIndex, ipFamily, parseIp } from './tunnels-common.js';
import { jsonPointer } from '../pointer.js';
export const vrrpConfigSyncValidators: readonly ValidatorDefinition[] = [
  {
    name: 'ha.vrrp-config-sync-owner-address',
    domains: ['ha', 'interfaces', 'tunnels', 'vpn'],
    validate(config) {
      const index = interfaceIndex(config);
      return Object.entries(config.ha.vrrp).flatMap(([name, v]) => {
        if (v.priority !== 255) return [];
        const iface = index.get(v.interface);
        const owns = v.addresses.some((ip) =>
          iface?.addresses.some((c) => c.family === ipFamily(ip) && c.address === parseIp(ip)),
        );
        return owns
          ? []
          : [
              {
                pointer: jsonPointer('ha', 'vrrp', name, 'priority'),
                message: 'priority 255 requires an interface-owned virtual address',
              },
            ];
      });
    },
  },
];
