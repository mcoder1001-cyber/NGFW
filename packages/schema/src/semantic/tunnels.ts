import type { RootConfig } from '../index.js';
import { TUNNEL_KINDS } from '../domains/tunnels.js';
import { jsonPointer } from '../pointer.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';
import {
  addressConfiguredInVrf,
  type Cidr,
  cidrsOverlap,
  interfaceIndex,
  knownVrfs,
  parseCidr,
} from './tunnels-common.js';

/**
 * Semantic validators for `tunnels`: names unique across kinds (vdom.md #2), VRFs exist, the source address is
 * configured on an interface in the underlay VRF, no two tunnels share endpoints (GRE: per type and ERSPAN
 * session, F11; GTP-U: per TEID; L2TPv3: per session; PPPoE: per client MAC + session) or an instance, the
 * multicast interface exists, tunnel interface addresses do not overlap with each other **or with any other
 * interface** (physical, sub-interface, WireGuard) in the same VRF (F10), and the instance-keyed kinds (GRE, VXLAN,
 * IPIP) carry an `instance` (`tunnels.instance-required`, D-205: the agent names their engine interface from it).
 * Owner: P02c; S-tunnels-contract added the T1 kinds and the instance rule.
 */

type Kind = (typeof TUNNEL_KINDS)[number]['key'];

/** Every kind's members the rules read; PPPoE sessions have no src / underlay VRF / addresses. */
interface AnyTunnel {
  enabled: boolean;
  instance?: number | undefined;
  src?: string | undefined;
  dst?: string | undefined;
  underlayVrf?: string | undefined;
  vrf: string;
  ipv4?: string[] | undefined;
  ipv6?: string[] | undefined;
  vni?: number;
  type?: string;
  sessionId?: number | undefined;
  mcastInterface?: string | undefined;
  teid?: number;
  localSessionId?: number;
  clientMac?: string;
  sixrd?: unknown;
}

function* allTunnels(config: RootConfig): Generator<[Kind, string, AnyTunnel]> {
  for (const { key } of TUNNEL_KINDS) {
    for (const [name, t] of Object.entries(config.tunnels[key])) yield [key, name, t as AnyTunnel];
  }
}

const P = (kind: Kind, name: string, ...rest: (string | number)[]): string =>
  jsonPointer('tunnels', kind, name, ...rest);

const nameUnique: ValidatorDefinition = {
  name: 'tunnels.name-unique',
  domains: ['tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, Kind>();
    for (const [kind, name] of allTunnels(config)) {
      const previous = seen.get(name);
      if (previous !== undefined) {
        issues.push({
          pointer: P(kind, name),
          message: `tunnel name '${name}' is already used by tunnels.${previous}`,
        });
      } else {
        seen.set(name, kind);
      }
    }
    return issues;
  },
};

const vrfExists: ValidatorDefinition = {
  name: 'tunnels.vrf-exists',
  domains: ['tunnels', 'vrfs'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const vrfs = knownVrfs(config);
    for (const [kind, name, t] of allTunnels(config)) {
      for (const field of ['vrf', 'underlayVrf'] as const) {
        const vrf = t[field];
        if (vrf !== undefined && !vrfs.has(vrf)) {
          issues.push({
            pointer: P(kind, name, field),
            message: `VRF '${vrf}' does not exist`,
          });
        }
      }
    }
    return issues;
  },
};

const instanceRequired: ValidatorDefinition = {
  name: 'tunnels.instance-required',
  domains: ['tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    for (const k of TUNNEL_KINDS) {
      if (!k.instance) continue;
      for (const [name, t] of Object.entries(config.tunnels[k.key])) {
        const a = t as AnyTunnel;
        // a 6RD tunnel is named by the engine (ipip_6rd_add_tunnel has no instance); the agent keys it by name
        if (a.instance === undefined && a.sixrd === undefined) {
          issues.push({
            pointer: P(k.key, name, 'instance'),
            message: `a ${k.key} tunnel needs an instance: it names the engine interface (${k.vppPrefix}<instance>)`,
          });
        }
      }
    }
    return issues;
  },
};

const sourceConfigured: ValidatorDefinition = {
  name: 'tunnels.source-address-configured',
  domains: ['tunnels', 'interfaces', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    for (const [kind, name, t] of allTunnels(config)) {
      if (t.src === undefined || t.underlayVrf === undefined) continue;
      if (!addressConfiguredInVrf(index, t.src, t.underlayVrf)) {
        issues.push({
          pointer: P(kind, name, 'src'),
          message: `${t.src} is not configured on any interface in VRF '${t.underlayVrf}'`,
        });
      }
    }
    return issues;
  },
};

const endpointsUnique: ValidatorDefinition = {
  name: 'tunnels.endpoints-unique',
  domains: ['tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const [kind, name, t] of allTunnels(config)) {
      if (kind === 'pppoe') {
        const key = `pppoe|${t.clientMac ?? ''}|${t.sessionId ?? ''}`;
        const previous = seen.get(key);
        if (previous !== undefined) {
          issues.push({
            pointer: P(kind, name, 'sessionId'),
            message: `same client MAC and session id as session '${previous}'`,
          });
        } else {
          seen.set(key, name);
        }
        continue;
      }
      const key = [
        kind,
        t.underlayVrf,
        t.src,
        t.dst ?? '',
        t.vni ?? '',
        t.type ?? '',
        t.sessionId ?? '',
        t.teid ?? '',
        t.localSessionId ?? '',
      ].join('|');
      const previous = seen.get(key);
      if (previous !== undefined) {
        const extra =
          t.vni !== undefined
            ? ' and VNI'
            : t.sessionId !== undefined
              ? ' and ERSPAN session'
              : t.teid !== undefined
                ? ' and TEID'
                : t.localSessionId !== undefined
                  ? ' and session id'
                  : '';
        issues.push({
          pointer: P(kind, name, 'dst'),
          message: `same endpoints${extra} as tunnel '${previous}'`,
        });
      } else {
        seen.set(key, name);
      }
    }
    return issues;
  },
};

const instanceUnique: ValidatorDefinition = {
  name: 'tunnels.instance-unique',
  domains: ['tunnels'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const seen = new Map<string, string>();
    for (const [kind, name, t] of allTunnels(config)) {
      if (t.instance === undefined) continue;
      const key = `${kind}|${t.instance}`;
      const previous = seen.get(key);
      if (previous !== undefined) {
        issues.push({
          pointer: P(kind, name, 'instance'),
          message: `instance ${t.instance} is already used by tunnel '${previous}'`,
        });
      } else {
        seen.set(key, name);
      }
    }
    return issues;
  },
};

const mcastInterfaceExists: ValidatorDefinition = {
  name: 'tunnels.mcast-interface-exists',
  domains: ['tunnels', 'interfaces'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    const index = interfaceIndex(config);
    for (const [kind, name, t] of allTunnels(config)) {
      if (t.mcastInterface !== undefined && !index.has(t.mcastInterface)) {
        issues.push({
          pointer: P(kind, name, 'mcastInterface'),
          message: `interface '${t.mcastInterface}' does not exist`,
        });
      }
    }
    return issues;
  },
};

const TUNNEL_NAME = new RegExp(`^(?:${TUNNEL_KINDS.map((k) => k.vppPrefix).join('|')})[0-9]+$`);

const addressOverlap: ValidatorDefinition = {
  name: 'tunnels.address-overlap',
  domains: ['tunnels', 'interfaces', 'vpn'],
  validate(config) {
    const issues: SemanticIssue[] = [];
    // Seed with every non-tunnel interface (physical, sub-interfaces, WireGuard) so a tunnel address cannot
    // collide with a prefix that already exists in its VRF; instanced tunnels are added from `allTunnels` below.
    const seen: { owner: string; vrf: string; cidr: Cidr }[] = [];
    for (const info of interfaceIndex(config).values()) {
      if (TUNNEL_NAME.test(info.name)) continue;
      for (const cidr of info.addresses)
        seen.push({ owner: `interface '${info.name}'`, vrf: info.vrf, cidr });
    }
    for (const [kind, name, t] of allTunnels(config)) {
      for (const field of ['ipv4', 'ipv6'] as const) {
        (t[field] ?? []).forEach((text, i) => {
          const cidr = parseCidr(text);
          if (cidr === undefined) return;
          const clash = seen.find((s) => s.vrf === t.vrf && cidrsOverlap(s.cidr, cidr));
          if (clash !== undefined) {
            issues.push({
              pointer: P(kind, name, field, i),
              message: `${text} overlaps an address of ${clash.owner} in VRF '${t.vrf}'`,
            });
          }
          seen.push({ owner: `tunnel '${name}'`, vrf: t.vrf, cidr });
        });
      }
    }
    return issues;
  },
};

export const tunnelsValidators: readonly ValidatorDefinition[] = [
  nameUnique,
  vrfExists,
  instanceRequired,
  sourceConfigured,
  endpointsUnique,
  instanceUnique,
  mcastInterfaceExists,
  addressOverlap,
];
