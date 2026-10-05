import { jsonPointer } from '../pointer.js';
import type { ValidatorDefinition } from './registry.js';

/** The approved engine has no remote-access EAP/address-assignment path yet. */
export const raVpnValidators: readonly ValidatorDefinition[] = [
  {
    name: 'vpn.remote-access-native-capability',
    domains: ['vpn'],
    validate(config) {
      return Object.entries(config.vpn.remoteAccess)
        .filter(([, profile]) => profile.enabled)
        .sort(([a], [b]) => a.localeCompare(b))
        .map(([name]) => ({
          pointer: jsonPointer('vpn', 'remoteAccess', name, 'enabled'),
          message:
            'Remote-access VPN is unavailable on the approved native IKEv2 engine; disable this profile before committing. Disabled profiles are inactive drafts.',
        }));
    },
  },
];
