import type { RootConfig } from '../index.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * IPsec identity rule for `vpn.ipsec.tunnels`, in addition to P02c's rules in `semantic/vpn.ts` (proposal exists,
 * PKI references, VRFs, local address, route-based IPIP, selectors, proposal/IKE-version compatibility) which are not
 * repeated here:
 *
 * - `vpn.ipsec-psk-identity-unique`: two enabled PSK tunnels that resolve to the same (local id, remote id)
 *   pair collide — the VPN service selects authentication by identities and could not tell their keys apart (independent profiles cannot disambiguate the same identity pair). Reported at the second tunnel's secretRef, in name order. Default identities are the
 *   addresses (remote `%any` for a `%any` peer), as the renderer derives them.
 */

const P = (...segments: (string | number)[]): string => jsonPointer('vpn', 'ipsec', ...segments);

const pskIdentityUnique: ValidatorDefinition = {
  name: 'vpn.ipsec-psk-identity-unique',
  domains: ['vpn'],
  validate(config: RootConfig) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    const tunnels = Object.entries(config.vpn.ipsec.tunnels).sort(([a], [b]) => a.localeCompare(b));
    for (const [name, t] of tunnels) {
      if (t.enabled === false || t.auth.method !== 'psk') continue;
      const localId = t.localId ?? t.localAddr;
      const remoteId = t.remoteId ?? t.remoteAddr;
      const key = `${localId}\u0000${remoteId}`;
      const prev = seen.get(key);
      if (prev === undefined) {
        seen.set(key, name);
        continue;
      }
      issues.push({
        pointer: P('tunnels', name, 'auth', 'secretRef'),
        message: `tunnels '${prev}' and '${name}' use the same identities (${localId} ↔ ${remoteId}) with pre-shared keys; the VPN service cannot distinguish their authentication profiles`,
      });
    }
    return issues;
  },
};

export const ipsecValidators: readonly ValidatorDefinition[] = [pskIdentityUnique];
