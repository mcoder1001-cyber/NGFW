import { jsonPointer } from '../pointer.js';
import { splitIpv4Range, type NatConfig } from '../domains/nat.js';
import {
  ipFamily,
  ipv4ToNumber,
  interfaceExists,
  prefixLength,
  prefixToRange,
  rangesOverlap,
  vrfExists,
  type AddressRange,
} from './objects.js';
import type { SemanticIssue, ValidatorDefinition } from './registry.js';

/**
 * Semantic validators for `nat` (tier b). Pure functions of the schema-valid document, `{ pointer, message }[]`.
 *
 * Rules (name → what it rejects):
 *   nat.interfaces-exist        an inside/outside/output-feature/binding interface that is not in `interfaces`
 *   nat.inside-outside-disjoint an interface listed twice, or on both the inside and the outside of one translator
 *   nat.pools-valid             a pool range that ends before it starts, pools that overlap, duplicate pool names
 *   nat.static-mappings         external side not exactly one of ip/pool/interface, unknown pool, one-sided ports,
 *                               ports without protocol (F-nat44 rule), ports on ICMP, twiceNat+selfTwiceNat,
 *                               duplicate names, two mappings claiming the same external tuple
 *   nat.identity-mappings       not exactly one of ip/interface, port without protocol, port on ICMP
 *   nat.load-balanced-mappings  duplicate names, duplicate external tuples, duplicate local endpoints
 *   nat.mode-ed-features        twice-NAT pools/mappings, out2in-only and load balancing while `mode` is `ei`
 *   nat.vrfs-exist              any `vrf` / `insideVrf` / `outsideVrf` that is not in `vrfs` (`default` implied)
 *   nat.nat64-valid             prefix length not 32/40/48/56/64/96, two prefixes for one VRF, duplicate BIB tuples
 *   nat.nptv6-valid             internal/external prefix lengths differ, two bindings on one interface
 *   nat.det44-valid             outside prefix shorter than inside, sharing ratio > 2^15, overlapping prefixes
 *   nat.map-valid               duplicate domain names, EA bits past /64, PSID offset+length > 16, PSID that does
 *                               not fit, duplicate PSIDs, per-PSID rules with EA bits
 *   nat.cnat-valid              duplicate names/VIPs, backend family ≠ VIP family, duplicate backends
 *   nat.dslite-valid            AFTR and B4 both configured, or enabled with neither
 *
 * Only P02b edits this file.
 */

type Segment = string | number;

/** `"a.b.c.d-a.b.c.e"` (schema-valid) → inclusive IPv4 interval; throws on text the schema would have rejected. */
export function poolRange(text: string): AddressRange {
  const r = splitIpv4Range(text);
  if (r === undefined) throw new Error(`invalid IPv4 range '${text}'`);
  return { family: 4, start: BigInt(ipv4ToNumber(r.start)), end: BigInt(ipv4ToNumber(r.end)) };
}

/** Indices of items whose key repeats an earlier item's key (`undefined` keys are skipped). */
export function duplicates<T>(
  items: readonly T[],
  keyOf: (item: T) => string | undefined,
): { index: number; first: number; item: T }[] {
  const firstIndex = new Map<string, number>();
  const out: { index: number; first: number; item: T }[] = [];
  items.forEach((item, index) => {
    const key = keyOf(item);
    if (key === undefined) return;
    const first = firstIndex.get(key);
    if (first === undefined) firstIndex.set(key, index);
    else out.push({ index, first, item });
  });
  return out;
}

function duplicateIssues<T>(
  items: readonly T[],
  keyOf: (item: T) => string | undefined,
  pointer: (index: number) => string,
  message: (item: T, first: number) => string,
): SemanticIssue[] {
  return duplicates(items, keyOf).map(({ index, first, item }) => ({
    pointer: pointer(index),
    message: message(item, first),
  }));
}

/** Duplicate entries in a plain string list (interfaces). */
function listDuplicates(names: readonly string[], segments: readonly Segment[]): SemanticIssue[] {
  return duplicateIssues(
    names,
    (n) => n,
    (i) => jsonPointer('nat', ...segments, i),
    (n) => `'${n}' is listed twice`,
  );
}

/** Ordered, non-overlapping ranges within one pool list; `label(i)` names a pool for messages. */
function poolListIssues(
  pools: readonly { range: string }[],
  segments: readonly Segment[],
  label: (index: number) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const seen: (AddressRange & { index: number })[] = [];
  pools.forEach((pool, i) => {
    const pointer = jsonPointer('nat', ...segments, i, 'range');
    const r = poolRange(pool.range);
    if (r.start > r.end) {
      issues.push({ pointer, message: `range '${pool.range}' ends before it starts` });
      return;
    }
    const hit = seen.find((other) => rangesOverlap(other, r));
    if (hit !== undefined) {
      issues.push({ pointer, message: `range '${pool.range}' overlaps ${label(hit.index)}` });
    }
    seen.push({ ...r, index: i });
  });
  return issues;
}

function prefixOverlapIssues(
  prefixes: readonly string[],
  pointer: (index: number) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const seen: (AddressRange & { index: number })[] = [];
  prefixes.forEach((prefix, i) => {
    const r = prefixToRange(prefix);
    const hit = seen.find((other) => rangesOverlap(other, r));
    if (hit !== undefined) {
      issues.push({ pointer: pointer(i), message: `'${prefix}' overlaps entry ${hit.index}` });
    }
    seen.push({ ...r, index: i });
  });
  return issues;
}

/** The external side of a static mapping as a comparable key (`undefined` when it is not exactly one thing). */
function externalKey(m: NatConfig['staticMappings'][number]): string | undefined {
  const { ip, pool, interface: iface, port } = m.external;
  const chosen = [ip, pool, iface].filter((v) => v !== undefined);
  if (chosen.length !== 1) return undefined;
  const where = ip ?? (pool !== undefined ? `pool:${pool}` : `if:${iface}`);
  return [m.protocol ?? '*', where, port ?? '*', m.vrf ?? 'default'].join('|');
}

const NAT64_PREFIX_LENGTHS = new Set([32, 40, 48, 56, 64, 96]);

export const natValidators: readonly ValidatorDefinition[] = [
  {
    name: 'nat.interfaces-exist',
    domains: ['nat', 'interfaces'],
    validate: (config) => {
      const { nat } = config;
      const refs: [string, string][] = [];
      const list = (segments: Segment[], names: readonly string[]): void => {
        names.forEach((n, i) => refs.push([jsonPointer('nat', ...segments, i), n]));
      };
      list(['inside'], nat.inside);
      list(['outside'], nat.outside);
      list(['outputFeature'], nat.outputFeature);
      nat.staticMappings.forEach((m, i) => {
        if (m.external.interface !== undefined) {
          refs.push([
            jsonPointer('nat', 'staticMappings', i, 'external', 'interface'),
            m.external.interface,
          ]);
        }
      });
      nat.identityMappings.forEach((m, i) => {
        if (m.interface !== undefined) {
          refs.push([jsonPointer('nat', 'identityMappings', i, 'interface'), m.interface]);
        }
      });
      for (const key of ['nat64', 'nat66', 'det44'] as const) {
        list([key, 'inside'], nat[key].inside);
        list([key, 'outside'], nat[key].outside);
      }
      nat.nptv6.bindings.forEach((b, i) => {
        refs.push([jsonPointer('nat', 'nptv6', 'bindings', i, 'interface'), b.interface]);
      });
      nat.cnat.snat.interfaces.forEach((b, i) => {
        refs.push([jsonPointer('nat', 'cnat', 'snat', 'interfaces', i, 'interface'), b.interface]);
      });
      return refs
        .filter(([, name]) => !interfaceExists(config, name))
        .map(([pointer, name]) => ({ pointer, message: `interface '${name}' does not exist` }));
    },
  },
  {
    name: 'nat.inside-outside-disjoint',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [];
      const sides = (
        segments: Segment[],
        inside: readonly string[],
        outside: readonly string[],
      ): void => {
        issues.push(...listDuplicates(inside, [...segments, 'inside']));
        issues.push(...listDuplicates(outside, [...segments, 'outside']));
        const insideSet = new Set(inside);
        outside.forEach((n, i) => {
          if (insideSet.has(n)) {
            issues.push({
              pointer: jsonPointer('nat', ...segments, 'outside', i),
              message: `'${n}' is also an inside interface`,
            });
          }
        });
      };
      sides([], nat.inside, nat.outside);
      issues.push(...listDuplicates(nat.outputFeature, ['outputFeature']));
      for (const key of ['nat64', 'nat66', 'det44'] as const) {
        sides([key], nat[key].inside, nat[key].outside);
      }
      return issues;
    },
  },
  {
    name: 'nat.pools-valid',
    domains: ['nat'],
    validate: ({ nat }) => [
      ...duplicateIssues(
        nat.pools,
        (p) => p.name,
        (i) => jsonPointer('nat', 'pools', i, 'name'),
        (p) => `pool name '${p.name}' is used twice`,
      ),
      ...poolListIssues(nat.pools, ['pools'], (i) => `pool '${nat.pools[i]?.name}'`),
      ...poolListIssues(nat.nat64.pools, ['nat64', 'pools'], (i) => `nat64 pool ${i}`),
      ...poolListIssues(nat.dslite.pools, ['dslite', 'pools'], (i) => `ds-lite pool ${i}`),
    ],
  },
  {
    name: 'nat.static-mappings',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [];
      const poolNames = new Set(nat.pools.map((p) => p.name));
      nat.staticMappings.forEach((m, i) => {
        const at = (...s: Segment[]): string => jsonPointer('nat', 'staticMappings', i, ...s);
        const ext = m.external;
        if ([ext.ip, ext.pool, ext.interface].filter((v) => v !== undefined).length !== 1) {
          issues.push({
            pointer: at('external'),
            message: 'exactly one of external.ip, external.pool or external.interface is required',
          });
        }
        if (ext.pool !== undefined && !poolNames.has(ext.pool)) {
          issues.push({
            pointer: at('external', 'pool'),
            message: `pool '${ext.pool}' does not exist in nat.pools`,
          });
        }
        const hasLocalPort = m.local.port !== undefined;
        const hasExternalPort = ext.port !== undefined;
        if (hasLocalPort !== hasExternalPort) {
          issues.push({
            pointer: at(hasLocalPort ? 'external' : 'local', 'port'),
            message:
              'local.port and external.port must be set together (omit both for a 1:1 mapping)',
          });
        }
        if (hasLocalPort || hasExternalPort) {
          if (m.protocol === undefined) {
            issues.push({
              pointer: at('protocol'),
              message: 'protocol is required when ports are set',
            });
          } else if (m.protocol === 'icmp') {
            issues.push({ pointer: at('protocol'), message: 'ICMP mappings cannot carry ports' });
          }
        }
        if (m.twiceNat && m.selfTwiceNat) {
          issues.push({
            pointer: at('selfTwiceNat'),
            message: 'twiceNat and selfTwiceNat are mutually exclusive',
          });
        }
      });
      issues.push(
        ...duplicateIssues(
          nat.staticMappings,
          (m) => m.name,
          (i) => jsonPointer('nat', 'staticMappings', i, 'name'),
          (m) => `mapping name '${m.name}' is used twice`,
        ),
        ...duplicateIssues(
          nat.staticMappings,
          externalKey,
          (i) => jsonPointer('nat', 'staticMappings', i, 'external'),
          (_m, first) =>
            `same external address, port, protocol and VRF as mapping '${nat.staticMappings[first]?.name}'`,
        ),
      );
      return issues;
    },
  },
  {
    name: 'nat.identity-mappings',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [];
      nat.identityMappings.forEach((m, i) => {
        const at = (...s: Segment[]): string => jsonPointer('nat', 'identityMappings', i, ...s);
        if ((m.ip === undefined) === (m.interface === undefined)) {
          issues.push({ pointer: at(), message: 'exactly one of ip or interface is required' });
        }
        if (m.port !== undefined) {
          if (m.protocol === undefined) {
            issues.push({
              pointer: at('protocol'),
              message: 'protocol is required when port is set',
            });
          } else if (m.protocol === 'icmp') {
            issues.push({
              pointer: at('protocol'),
              message: 'ICMP identity mappings cannot carry a port',
            });
          }
        }
      });
      return issues;
    },
  },
  {
    name: 'nat.load-balanced-mappings',
    domains: ['nat'],
    validate: ({ nat }) => {
      const issues: SemanticIssue[] = [
        ...duplicateIssues(
          nat.loadBalancedMappings,
          (m) => m.name,
          (i) => jsonPointer('nat', 'loadBalancedMappings', i, 'name'),
          (m) => `mapping name '${m.name}' is used twice`,
        ),
        ...duplicateIssues(
          nat.loadBalancedMappings,
          (m) => `${m.protocol}|${m.external.ip}|${m.external.port}`,
          (i) => jsonPointer('nat', 'loadBalancedMappings', i, 'external'),
          (_m, first) =>
            `same external endpoint as mapping '${nat.loadBalancedMappings[first]?.name}'`,
        ),
      ];
      nat.loadBalancedMappings.forEach((m, i) => {
        issues.push(
          ...duplicateIssues(
            m.locals,
            (l) => `${l.ip}|${l.port}|${l.vrf ?? 'default'}`,
            (j) => jsonPointer('nat', 'loadBalancedMappings', i, 'locals', j),
            (l) => `local endpoint ${l.ip}:${l.port} is listed twice`,
          ),
        );
      });
      return issues;
    },
  },
  {
    name: 'nat.mode-ed-features',
    domains: ['nat'],
    validate: ({ nat }) => {
      if (nat.mode === 'ed') return [];
      const issues: SemanticIssue[] = [];
      const needsEd = (pointer: string, feature: string): void => {
        issues.push({ pointer, message: `${feature} requires mode 'ed' (nat44-ed)` });
      };
      nat.pools.forEach((p, i) => {
        if (p.twiceNat) needsEd(jsonPointer('nat', 'pools', i, 'twiceNat'), 'twice-NAT');
      });
      nat.staticMappings.forEach((m, i) => {
        for (const flag of ['twiceNat', 'selfTwiceNat', 'out2inOnly'] as const) {
          if (m[flag]) needsEd(jsonPointer('nat', 'staticMappings', i, flag), flag);
        }
      });
      nat.loadBalancedMappings.forEach((_m, i) => {
        needsEd(jsonPointer('nat', 'loadBalancedMappings', i), 'load balancing');
      });
      return issues;
    },
  },
  {
    name: 'nat.vrfs-exist',
    domains: ['nat', 'vrfs'],
    validate: (config) => {
      const { nat } = config;
      const refs: [string, string | undefined][] = [
        [jsonPointer('nat', 'insideVrf'), nat.insideVrf],
        [jsonPointer('nat', 'outsideVrf'), nat.outsideVrf],
        [jsonPointer('nat', 'det44', 'insideVrf'), nat.det44.insideVrf],
        [jsonPointer('nat', 'det44', 'outsideVrf'), nat.det44.outsideVrf],
      ];
      const list = (segments: Segment[], items: readonly { vrf?: string | undefined }[]): void => {
        items.forEach((item, i) =>
          refs.push([jsonPointer('nat', ...segments, i, 'vrf'), item.vrf]),
        );
      };
      list(['pools'], nat.pools);
      list(['staticMappings'], nat.staticMappings);
      list(['identityMappings'], nat.identityMappings);
      nat.loadBalancedMappings.forEach((m, i) =>
        list(['loadBalancedMappings', i, 'locals'], m.locals),
      );
      list(['nat64', 'prefixes'], nat.nat64.prefixes);
      list(['nat64', 'pools'], nat.nat64.pools);
      list(['nat64', 'staticBibs'], nat.nat64.staticBibs);
      list(['nat66', 'staticMappings'], nat.nat66.staticMappings);
      const issues: SemanticIssue[] = [];
      for (const [pointer, vrf] of refs) {
        if (vrf !== undefined && !vrfExists(config, vrf)) {
          issues.push({ pointer, message: `VRF '${vrf}' does not exist` });
        }
      }
      return issues;
    },
  },
  {
    name: 'nat.nat64-valid',
    domains: ['nat'],
    validate: ({ nat: { nat64 } }) => {
      const issues: SemanticIssue[] = [];
      nat64.prefixes.forEach((p, i) => {
        if (!NAT64_PREFIX_LENGTHS.has(prefixLength(p.prefix))) {
          issues.push({
            pointer: jsonPointer('nat', 'nat64', 'prefixes', i, 'prefix'),
            message: 'NAT64 prefix length must be 32, 40, 48, 56, 64 or 96 (RFC 6052)',
          });
        }
      });
      issues.push(
        ...duplicateIssues(
          nat64.prefixes,
          (p) => p.vrf ?? 'default',
          (i) => jsonPointer('nat', 'nat64', 'prefixes', i),
          (p) => `VRF '${p.vrf ?? 'default'}' already has a NAT64 prefix`,
        ),
        ...duplicateIssues(
          nat64.staticBibs,
          (b) => `${b.protocol}|${b.inside.ip}|${b.inside.port}|${b.vrf ?? 'default'}`,
          (i) => jsonPointer('nat', 'nat64', 'staticBibs', i, 'inside'),
          (b) => `inside ${b.inside.ip}:${b.inside.port}/${b.protocol} already has a BIB entry`,
        ),
        ...duplicateIssues(
          nat64.staticBibs,
          (b) => `${b.protocol}|${b.outside.ip}|${b.outside.port}|${b.vrf ?? 'default'}`,
          (i) => jsonPointer('nat', 'nat64', 'staticBibs', i, 'outside'),
          (b) => `outside ${b.outside.ip}:${b.outside.port}/${b.protocol} already has a BIB entry`,
        ),
      );
      return issues;
    },
  },
  {
    name: 'nat.nptv6-valid',
    domains: ['nat'],
    validate: ({ nat: { nptv6 } }) => {
      const issues: SemanticIssue[] = [];
      nptv6.bindings.forEach((b, i) => {
        if (prefixLength(b.internal) !== prefixLength(b.external)) {
          issues.push({
            pointer: jsonPointer('nat', 'nptv6', 'bindings', i, 'external'),
            message: 'internal and external prefixes must have the same length (RFC 6296)',
          });
        }
      });
      issues.push(
        ...duplicateIssues(
          nptv6.bindings,
          (b) => b.interface,
          (i) => jsonPointer('nat', 'nptv6', 'bindings', i, 'interface'),
          (b) => `interface '${b.interface}' already has an NPTv6 binding`,
        ),
      );
      return issues;
    },
  },
  {
    name: 'nat.det44-valid',
    domains: ['nat'],
    validate: ({ nat: { det44 } }) => {
      const issues: SemanticIssue[] = [];
      det44.mappings.forEach((m, i) => {
        const pointer = jsonPointer('nat', 'det44', 'mappings', i, 'outside');
        const ratioBits = prefixLength(m.outside) - prefixLength(m.inside);
        if (ratioBits < 0) {
          issues.push({
            pointer,
            message:
              'outside prefix must be at least as long as the inside prefix (inside hosts share outside addresses)',
          });
        } else if (ratioBits > 15) {
          issues.push({
            pointer,
            message:
              'sharing ratio above 2^15 inside hosts per outside address leaves no ports per host',
          });
        }
      });
      issues.push(
        ...prefixOverlapIssues(
          det44.mappings.map((m) => m.inside),
          (i) => jsonPointer('nat', 'det44', 'mappings', i, 'inside'),
        ),
        ...prefixOverlapIssues(
          det44.mappings.map((m) => m.outside),
          (i) => jsonPointer('nat', 'det44', 'mappings', i, 'outside'),
        ),
      );
      return issues;
    },
  },
  {
    name: 'nat.map-valid',
    domains: ['nat'],
    validate: ({ nat: { map } }) => {
      const issues: SemanticIssue[] = duplicateIssues(
        map.domains,
        (d) => d.name,
        (i) => jsonPointer('nat', 'map', 'domains', i, 'name'),
        (d) => `domain name '${d.name}' is used twice`,
      );
      map.domains.forEach((d, i) => {
        const at = (...s: Segment[]): string => jsonPointer('nat', 'map', 'domains', i, ...s);
        if (prefixLength(d.ipv6Prefix) + d.eaBitsLength > 64) {
          issues.push({
            pointer: at('eaBitsLength'),
            message: 'ipv6Prefix length plus EA bits must not exceed 64',
          });
        }
        if (d.psidOffset + d.psidLength > 16) {
          issues.push({
            pointer: at('psidLength'),
            message: 'psidOffset plus psidLength must not exceed 16',
          });
        }
        if (d.rules.length > 0 && d.eaBitsLength > 0) {
          issues.push({
            pointer: at('rules'),
            message: 'per-PSID rules require eaBitsLength 0 (lw4o6 / shared-address MAP)',
          });
        }
        const maxPsid = 2 ** d.psidLength;
        d.rules.forEach((r, j) => {
          if (r.psid >= maxPsid) {
            issues.push({
              pointer: at('rules', j, 'psid'),
              message: `PSID ${r.psid} does not fit in ${d.psidLength} PSID bits`,
            });
          }
        });
        issues.push(
          ...duplicateIssues(
            d.rules,
            (r) => String(r.psid),
            (j) => at('rules', j, 'psid'),
            (r) => `PSID ${r.psid} is mapped twice`,
          ),
        );
      });
      return issues;
    },
  },
  {
    name: 'nat.cnat-valid',
    domains: ['nat'],
    validate: ({ nat: { cnat } }) => {
      const issues: SemanticIssue[] = [
        ...duplicateIssues(
          cnat.translations,
          (t) => t.name,
          (i) => jsonPointer('nat', 'cnat', 'translations', i, 'name'),
          (t) => `translation name '${t.name}' is used twice`,
        ),
        ...duplicateIssues(
          cnat.translations,
          (t) => `${t.protocol}|${t.vip.ip}|${t.vip.port}`,
          (i) => jsonPointer('nat', 'cnat', 'translations', i, 'vip'),
          (t) => `VIP ${t.vip.ip}:${t.vip.port}/${t.protocol} is translated twice`,
        ),
      ];
      cnat.translations.forEach((t, i) => {
        const at = (...s: Segment[]): string => jsonPointer('nat', 'cnat', 'translations', i, ...s);
        const family = ipFamily(t.vip.ip);
        t.backends.forEach((b, j) => {
          if (ipFamily(b.ip) !== family) {
            issues.push({
              pointer: at('backends', j, 'ip'),
              message: 'backend must be the same address family as the VIP',
            });
          }
        });
        issues.push(
          ...duplicateIssues(
            t.backends,
            (b) => `${b.ip}|${b.port}`,
            (j) => at('backends', j),
            (b) => `backend ${b.ip}:${b.port} is listed twice`,
          ),
        );
      });
      return issues;
    },
  },
  {
    name: 'nat.dslite-valid',
    domains: ['nat'],
    validate: ({ nat: { dslite } }) => {
      if (dslite.aftr !== undefined && dslite.b4 !== undefined) {
        return [
          {
            pointer: jsonPointer('nat', 'dslite', 'b4'),
            message: 'configure either the AFTR side or the B4 side, not both',
          },
        ];
      }
      if (dslite.enabled && dslite.aftr === undefined && dslite.b4 === undefined) {
        return [
          {
            pointer: jsonPointer('nat', 'dslite', 'enabled'),
            message: 'DS-Lite requires aftr or b4',
          },
        ];
      }
      return [];
    },
  },
];
