import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import { ipFamily, isUnspecified, parseIp } from './tunnels-common.js';

/**
 * F-unbound-chrony-syslog cross-field rules (wave-A-hotspots C2):
 *
 * - `services.unbound-chrony-syslog-vpp-cache-port` — VPP's dns plugin (`services.dns.vppCache`), once enabled,
 *   registers UDP port 53 for every address VPP owns (dns.c `udp_register_dst_port(UDP_DST_PORT_dns)`), so an enabled
 *   Unbound resolver cannot listen on port 53 at the same time: the VPP cache would answer (or drop) its queries.
 *   Resolver listen addresses are VPP interface addresses or a wildcard (`services.bind-address-configured`), so
 *   every enabled port-53 listener conflicts.
 * - `services.unbound-chrony-syslog-forwarder-loop` — one Unbound instance serves every enabled resolver (RF-3), so a
 *   forwarder (resolver default or forward zone) equal to any enabled resolver's listen socket in the same VRF — or
 *   covered by a wildcard listener of its family and port — forwards to itself. The schema already rejects the
 *   resolver's own default forwarders; this rule adds forward zones and the other resolvers.
 * - `management.unbound-chrony-syslog-tls` — a TLS export needs `tls` (whose `caRef` the schema requires); `tls` on a
 *   udp/tcp target is rejected rather than ignored.
 */

const S = (...segments: (string | number)[]): string => jsonPointer('services', ...segments);
const M = (...segments: (string | number)[]): string => jsonPointer('management', ...segments);

/** Canonical "<ip>|<port>" (numeric address, so 2001:db8::1 and 2001:DB8:0::1 compare equal). */
function socketKey(address: string, port: number): string | undefined {
  const ip = parseIp(address);
  return ip === undefined ? undefined : `${ipFamily(address)}|${ip.toString()}|${port}`;
}

const vppCachePort: ValidatorDefinition = {
  name: 'services.unbound-chrony-syslog-vpp-cache-port',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const dns = config.services.dns;
    if (dns.vppCache?.enabled !== true) return issues;
    for (const [name, r] of Object.entries(dns.resolvers)) {
      if (!r.enabled) continue;
      r.listen.forEach((l, k) => {
        if (l.port !== 53) return;
        issues.push({
          pointer: S('dns', 'resolvers', name, 'listen', k),
          message: `resolver '${name}' cannot listen on ${l.address}:53 while the VPP DNS cache (services.dns.vppCache) is enabled: VPP answers UDP port 53 on its addresses`,
        });
      });
    }
    return issues;
  },
};

const forwarderLoop: ValidatorDefinition = {
  name: 'services.unbound-chrony-syslog-forwarder-loop',
  domains: ['services'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const resolvers = Object.entries(config.services.dns.resolvers).filter(([, r]) => r.enabled);
    // every listen socket of the (single) Unbound instance, per VRF
    const exact = new Map<string, string>(); // vrf|family|ip|port → resolver
    const wildcard = new Map<string, string>(); // vrf|family|port → resolver
    for (const [name, r] of resolvers) {
      for (const l of r.listen) {
        const fam = ipFamily(l.address);
        if (isUnspecified(l.address)) {
          if (!wildcard.has(`${r.vrf}|${fam}|${l.port}`))
            wildcard.set(`${r.vrf}|${fam}|${l.port}`, name);
          continue;
        }
        const key = socketKey(l.address, l.port);
        if (key !== undefined && !exact.has(`${r.vrf}|${key}`)) exact.set(`${r.vrf}|${key}`, name);
      }
    }
    const check = (
      pointer: string,
      resolver: string,
      vrf: string,
      own: boolean,
      f: { address: string; port: number },
    ): void => {
      const key = socketKey(f.address, f.port);
      if (key === undefined) return;
      const owner =
        exact.get(`${vrf}|${key}`) ?? wildcard.get(`${vrf}|${ipFamily(f.address)}|${f.port}`);
      if (owner === undefined) return;
      // the resolver's own default forwarders equal to its own listen socket: the schema reports that one
      if (own && owner === resolver && exact.has(`${vrf}|${key}`)) return;
      issues.push({
        pointer,
        message: `forwarder ${f.address}:${f.port} is a listen address of resolver '${owner}' in VRF '${vrf}' (one Unbound instance serves every resolver: the query would loop)`,
      });
    };
    for (const [name, r] of resolvers) {
      r.forwarders.forEach((f, j) =>
        check(S('dns', 'resolvers', name, 'forwarders', j), name, r.vrf, true, f),
      );
      r.forwardZones.forEach((z, i) =>
        z.forwarders.forEach((f, j) =>
          check(
            S('dns', 'resolvers', name, 'forwardZones', i, 'forwarders', j),
            name,
            r.vrf,
            false,
            f,
          ),
        ),
      );
    }
    return issues;
  },
};

const syslogTls: ValidatorDefinition = {
  name: 'management.unbound-chrony-syslog-tls',
  domains: ['management'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    config.management.syslog.forEach((s, i) => {
      if (s.protocol === 'tls' && s.tls === undefined) {
        issues.push({
          pointer: M('syslog', i, 'tls'),
          message: `syslog export to ${s.address}:${s.port} over TLS needs tls.caRef (the CA that verifies the collector)`,
        });
      }
      if (s.protocol !== 'tls' && s.tls !== undefined) {
        issues.push({
          pointer: M('syslog', i, 'tls'),
          message: `tls applies to protocol tls only (this target uses ${s.protocol})`,
        });
      }
    });
    return issues;
  },
};

export const unboundChronySyslogValidators: readonly ValidatorDefinition[] = [
  vppCachePort,
  forwarderLoop,
  syslogTls,
];
