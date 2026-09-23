import { jsonPointer } from '../pointer.js';
import { splitIpv4Range, type NatConfig } from '../domains/nat.js';
import {
  addressToBigInt,
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
 *   nat.interfaces-exist        an inside/outside/output-feature/binding/pool/MAP/CNAT interface not in `interfaces`
 *   nat.inside-outside-disjoint an interface listed twice, or on both the inside and the outside of one translator
 *   nat.pools-valid             a pool range that ends before it starts, range pools of the same twice-NAT class
 *                               that overlap (VPP keeps normal and twice-NAT addresses in separate lists), the same
 *                               interface twice as an interface pool of one class, duplicate pool names
 *   nat.static-mappings         external side not exactly one of ip/pool/interface, unknown pool, one-sided ports,
 *                               ports without protocol (F-nat44 rule), ports on ICMP, twiceNat+selfTwiceNat,
 *                               duplicate names, two mappings claiming the same external tuple (a `pool` is
 *                               resolved to its start address / interface first, so `ip` and `pool` collide)
 *   nat.identity-mappings       not exactly one of ip/interface, port without protocol, port on ICMP
 *   nat.load-balanced-mappings  duplicate names, duplicate external tuples, duplicate local endpoints
 *   nat.mode-ed-features        twice-NAT pools/mappings, out2in-only and load balancing while `mode` is `ei`
 *   nat.vrfs-exist              any `vrf` / `insideVrf` / `outsideVrf` that is not in `vrfs` (`default` implied)
 *   nat.nat64-valid             prefix length not 32/40/48/56/64/96, two prefixes for one VRF, duplicate BIB tuples
 *   nat.nptv6-valid             internal/external prefix lengths differ, two bindings on one interface
 *   nat.det44-valid             outside prefix shorter than inside, sharing ratio > 2^15, overlapping prefixes
 *   nat.map-valid               duplicate domain names, EA bits past /64, PSID offset+length > 16, PSID that does
 *                               not fit, duplicate PSIDs, per-PSID rules with EA bits, `mode` vs `ipv6Source`
 *                               length (map-e/lw4o6 ⇒ /128 BR address; map-t ⇒ /64 or /96 DMR — VPP
 *                               ip4_map_t_embedded_address), an interface bound to MAP twice
 *   nat.cnat-valid              duplicate names/VIPs, backend family ≠ VIP family, duplicate backends, an interface
 *                               twice in one SNAT table, a table the selected SNAT policy never consults
 *   nat.dslite-valid            AFTR and B4 both configured, or enabled with neither
 *   nat.prefixes-are-networks   a NAT64 / NPTv6 / DET44 / MAP / CNAT-exclude prefix whose address has host bits set
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

/** True when the address part of `cidr` is the first address of its prefix (no host bits set). */
export function isNetworkPrefix(cidr: string): boolean {
  return prefixToRange(cidr).start === addressToBigInt(cidr.slice(0, cidr.indexOf('/')));
}

/** `cidr` with its host bits cleared: `10.0.0.1/24` → `10.0.0.0/24`, `64:ff9b::1/96` → `64:ff9b::/96` (RFC 5952). */
export function networkOf(cidr: string): string {
  const { family, start } = prefixToRange(cidr);
  const length = cidr.slice(cidr.indexOf('/'));
  if (family === 4) {
    return (
      [24, 16, 8, 0].map((shift) => String((start >> BigInt(shift)) & 255n)).join('.') + length
    );
  }
  const groups = Array.from({ length: 8 }, (_, i) =>
    Number((start >> BigInt((7 - i) * 16)) & 0xffffn),
  );
  let best = { at: -1, length: 0 };
  let i = 0;
  while (i < 8) {
    if (groups[i] !== 0) {
      i += 1;
      continue;
    }
    let j = i;
    while (j < 8 && groups[j] === 0) j += 1;
    if (j - i > best.length) best = { at: i, length: j - i };
    i = j;
  }
  const hex = groups.map((g) => g.toString(16));
  if (best.length < 2) return hex.join(':') + length;
  return `${hex.slice(0, best.at).join(':')}::${hex.slice(best.at + best.length).join(':')}${length}`;
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

/**
 * Ordered, non-overlapping ranges within one pool list; `label(i)` names a pool for messages. Interface-address
 * pools (no `range`) are skipped — VPP resolves them at run time. Overlap is checked within one twice-NAT class
 * only: VPP keeps `addresses` and `twice_nat_addresses` as separate lists, so a twice-NAT pool may repeat a normal
 * pool's address.
 */
function poolListIssues(
  pools: readonly { range?: string | undefined; twiceNat?: boolean | undefined }[],
  segments: readonly Segment[],
  label: (index: number) => string,
): SemanticIssue[] {
  const issues: SemanticIssue[] = [];
  const seen: (AddressRange & { index: number; twiceNat: boolean })[] = [];
  pools.forEach((pool, i) => {
    if (pool.range === undefined) return;
    const pointer = jsonPointer('nat', ...segments, i, 'range');
    const r = poolRange(pool.range);
    if (r.start > r.end) {
      issues.push({ pointer, message: `range '${pool.range}' ends before it starts` });
      return;
    }
    const twiceNat = pool.twiceNat ?? false;
    const hit = seen.find((other) => other.twiceNat === twiceNat && rangesOverlap(other, r));
    if (hit !== undefined) {
      issues.push({ pointer, message: `range '${pool.range}' overlaps ${label(hit.index)}` });
    }
    seen.push({ ...r, index: i, twiceNat });
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

/**
 * What `external.pool` stands for when comparing external tuples: a range pool's start address (the address the
 * renderer uses) or `if:<interface>` for an interface pool — so `{ ip: X }` and `{ pool: p }` with `p` starting at
 * `X` are the same external side. Unknown pools resolve to `pool:<name>` (reported separately).
 */
function poolResolver(pools: NatConfig['pools']): (name: string) => string {
  const byName = new Map<string, string>();
  for (const p of pools) {
    if (byName.has(p.name)) continue; // duplicate names are reported by nat.pools-valid; first one wins here
    byName.set(
      p.name,
      'range' in p ? (splitIpv4Range(p.range)?.start ?? p.range) : `if:${p.interface}`,
    );
  }
  return (name) => byName.get(name) ?? `pool:${name}`;
}

/** The external side of a static mapping as a comparable key (`undefined` when it is not exactly one thing). */
function externalKey(
  m: NatConfig['staticMappings'][number],
  resolvePool: (name: string) => string,
): string | undefined {
  const { ip, pool, interface: iface, port } = m.external;
  if ([ip, pool, iface].filter((v) => v !== undefined).length !== 1) return undefined;
  let where: string;
  if (ip !== undefined) where = ip;
  else if (iface !== undefined) where = `if:${iface}`;
  else if (pool !== undefined) where = resolvePool(pool);
  else return undefined;
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
      nat.pools.forEach((p, i) => {
        if ('interface' in p) refs.push([jsonPointer('nat', 'pools', i, 'interface'), p.interface]);
      });
      for (const key of ['nat64', 'nat66', 'det44'] as const) {
        list([key, 'inside'], nat[key].inside);
        list([key, 'outside'], nat[key].outside);
      }
      nat.nptv6.bindings.forEach((b, i) => {
        refs.push([jsonPointer('nat', 'nptv6', 'bindings', i, 'interface'), b.interface]);
      });
      nat.map.interfaces.forEach((b, i) => {
        refs.push([jsonPointer('nat', 'map', 'interfaces', i, 'interface'), b.interface]);
      });
      nat.cnat.snat.interfaces.forEach((b, i) => {
        refs.push([jsonPointer('nat', 'cnat', 'snat', 'interfaces', i, 'interface'), b.interface]);
      });
      if (nat.cnat.snat.addresses.interface !== undefined) {
        refs.push([
          jsonPointer('nat', 'cnat', 'snat', 'addresses', 'interface'),
          nat.cnat.snat.addresses.interface,
        ]);
      }
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
      ...duplicateIssues(
        nat.pools,
        (p) => ('interface' in p ? `${p.interface}|${p.twiceNat}` : undefined),
        (i) => jsonPointer('nat', 'pools', i, 'interface'),
        (p, first) =>
          `interface ${'interface' in p ? `'${p.interface}'` : ''} is already used by pool '${nat.pools[first]?.name}'`,
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
      const resolvePool = poolResolver(nat.pools);
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
          (m) => externalKey(m, resolvePool),
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
      list(
        ['pools'],
        nat.pools.map((p) => ({ vrf: 'vrf' in p ? p.vrf : undefined })), // interface pools carry no vrf
      );
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
      issues.push(
        ...duplicateIssues(
          map.interfaces,
          (b) => b.interface,
          (i) => jsonPointer('nat', 'map', 'interfaces', i, 'interface'),
          (b) => `interface '${b.interface}' is already bound to MAP`,
        ),
      );
      map.domains.forEach((d, i) => {
        const at = (...s: Segment[]): string => jsonPointer('nat', 'map', 'domains', i, ...s);
        const sourceLength = prefixLength(d.ipv6Source);
        if (d.mode === 'map-t') {
          if (sourceLength !== 64 && sourceLength !== 96) {
            issues.push({
              pointer: at('ipv6Source'),
              message:
                'MAP-T needs the DMR prefix as ipv6Source, length 64 or 96 (VPP ip4_map_t_embedded_address)',
            });
          }
        } else if (sourceLength !== 128) {
          issues.push({
            pointer: at('ipv6Source'),
            message: `${d.mode === 'lw4o6' ? 'lw4o6' : 'MAP-E'} needs the BR address as ipv6Source (/128, the encapsulation source)`,
          });
        }
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
      const { snat } = cnat;
      issues.push(
        ...duplicateIssues(
          snat.interfaces,
          (b) => `${b.interface}|${b.table}`,
          (i) => jsonPointer('nat', 'cnat', 'snat', 'interfaces', i),
          (b) => `interface '${b.interface}' is already in SNAT table '${b.table}'`,
        ),
      );
      snat.interfaces.forEach((b, i) => {
        const perFamily = b.table === 'include-v4' || b.table === 'include-v6';
        if ((snat.policy === 'interface' && !perFamily) || (snat.policy === 'k8s' && perFamily)) {
          issues.push({
            pointer: jsonPointer('nat', 'cnat', 'snat', 'interfaces', i, 'table'),
            message: `SNAT policy '${snat.policy}' never consults table '${b.table}' (interface → include-v4/include-v6, k8s → pod/host)`,
          });
        }
      });
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
  {
    // `ipv4Cidr`/`ipv6Cidr` allow host bits (interface addresses need them); these fields are *networks* handed to
    // nat64_add_del_prefix / npt66_binding_add_del / det44_add_del_map / map_add_domain / cnat_snat_policy_add_del_pfx,
    // where stray host bits silently change the arithmetic.
    name: 'nat.prefixes-are-networks',
    domains: ['nat'],
    validate: ({ nat }) => {
      const refs: [string, string][] = [];
      nat.nat64.prefixes.forEach((p, i) => {
        refs.push([jsonPointer('nat', 'nat64', 'prefixes', i, 'prefix'), p.prefix]);
      });
      nat.nptv6.bindings.forEach((b, i) => {
        refs.push(
          [jsonPointer('nat', 'nptv6', 'bindings', i, 'internal'), b.internal],
          [jsonPointer('nat', 'nptv6', 'bindings', i, 'external'), b.external],
        );
      });
      nat.det44.mappings.forEach((m, i) => {
        refs.push(
          [jsonPointer('nat', 'det44', 'mappings', i, 'inside'), m.inside],
          [jsonPointer('nat', 'det44', 'mappings', i, 'outside'), m.outside],
        );
      });
      nat.map.domains.forEach((d, i) => {
        refs.push(
          [jsonPointer('nat', 'map', 'domains', i, 'ipv4Prefix'), d.ipv4Prefix],
          [jsonPointer('nat', 'map', 'domains', i, 'ipv6Prefix'), d.ipv6Prefix],
          [jsonPointer('nat', 'map', 'domains', i, 'ipv6Source'), d.ipv6Source],
        );
      });
      nat.cnat.snat.excludePrefixes.forEach((p, i) => {
        refs.push([jsonPointer('nat', 'cnat', 'snat', 'excludePrefixes', i), p]);
      });
      return refs
        .filter(([, prefix]) => !isNetworkPrefix(prefix))
        .map(([pointer, prefix]) => ({
          pointer,
          message: `'${prefix}' has host bits set; the network is ${networkOf(prefix)}`,
        }));
    },
  },
];
