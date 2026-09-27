import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { interfaceIndex } from './tunnels-common.js';

/**
 * F-mpls-ldp: cross-object rules for `routing.mpls.ldp` (RFC 5036). The LDP interfaces must be MPLS-enabled and in the
 * default VRF, the transport address must be a configured local address, and no static label route or SR binding-SID
 * in table 0 may fall inside the LDP dynamic label range (LDP owns that block).
 */

/** Every IPv4 address configured on a default-VRF interface or loopback (for the transport-address check). */
function localIpv4Addresses(config: { interfaces?: Record<string, unknown> }): Set<string> {
  const out = new Set<string>();
  for (const iface of Object.values(config.interfaces ?? {})) {
    const i = iface as { ipv4?: unknown[]; vrf?: string } | undefined;
    if (i?.vrf !== undefined && i.vrf !== 'default') continue;
    for (const a of i?.ipv4 ?? []) {
      if (typeof a === 'string') out.add(a.split('/')[0]!);
    }
  }
  return out;
}

const mplsLdp: ValidatorDefinition = {
  name: 'routing.mpls-ldp',
  domains: ['routing', 'interfaces', 'tunnels', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const mpls = config.routing.mpls;
    const ldp = mpls?.ldp;
    if (ldp === undefined) return issues;

    const ifaces = interfaceIndex(config);
    const mplsEnabled = new Set(mpls?.interfaces ?? []);

    ldp.interfaces.forEach((ifName, i) => {
      if (!ifaces.has(ifName)) {
        issues.push({ pointer: jsonPointer('routing', 'mpls', 'ldp', 'interfaces', i), message: `interface '${ifName}' does not exist` });
      } else if (!mplsEnabled.has(ifName)) {
        issues.push({
          pointer: jsonPointer('routing', 'mpls', 'ldp', 'interfaces', i),
          message: `interface '${ifName}' is not MPLS-enabled (add it to routing.mpls.interfaces)`,
        });
      }
    });

    // transportAddress must be a configured local address
    const locals = localIpv4Addresses(config as { interfaces?: Record<string, unknown> });
    if (locals.size > 0 && !locals.has(ldp.transportAddress)) {
      issues.push({
        pointer: jsonPointer('routing', 'mpls', 'ldp', 'transportAddress'),
        message: `'${ldp.transportAddress}' is not configured on any default-VRF interface or loopback`,
      });
    }

    // no static label route / SR BSID in table 0 inside the LDP dynamic range
    const range = ldp.labelRange;
    if (range !== undefined) {
      const routes = mpls?.labelRoutes ?? [];
      routes.forEach((r, i) => {
        if ((r.table ?? 0) === 0 && r.label >= range.min && r.label <= range.max) {
          issues.push({
            pointer: jsonPointer('routing', 'mpls', 'labelRoutes', i, 'label'),
            message: `static label ${r.label} is inside the LDP dynamic range ${range.min}–${range.max}`,
          });
        }
      });
      // SR-MPLS policies are keyed by their binding SID (a decimal label); table 0.
      for (const bsidKey of Object.keys(mpls?.sr?.policies ?? {})) {
        const bsid = Number(bsidKey);
        if (Number.isInteger(bsid) && bsid >= range.min && bsid <= range.max) {
          issues.push({
            pointer: jsonPointer('routing', 'mpls', 'sr', 'policies', bsidKey),
            message: `SR binding-SID ${bsid} is inside the LDP dynamic range ${range.min}–${range.max}`,
          });
        }
      }
    }

    return issues;
  },
};

export const mplsLdpValidators: readonly ValidatorDefinition[] = [mplsLdp];
