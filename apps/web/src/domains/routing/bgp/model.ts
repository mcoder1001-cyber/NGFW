import type {
  BgpConfig,
  BgpNeighborConfig,
  BgpPeerGroupConfig,
  InterfaceLcpConfig,
  PrefixListConfig,
  RouteMapConfig,
} from '@ngfw/schema';
import type { JsonSchema } from '@ngfw/ui-kit/schema-form';
import { domainSchemas } from '../../../schema/registry';

/** i18n namespace of the feature (task slug of P12's UI). */
export const NS = 'bgp';

export type {
  BgpConfig,
  BgpNeighborConfig,
  BgpPeerGroupConfig,
  InterfaceLcpConfig,
  PrefixListConfig,
  RouteMapConfig,
};

/** The routing document part this screen edits. */
export interface RoutingDoc {
  bgp?: BgpConfig;
  policy?: {
    prefixLists?: Record<string, PrefixListConfig>;
    routeMaps?: Record<string, RouteMapConfig>;
  };
}

type Props = Record<string, JsonSchema>;

const props = (s: JsonSchema | undefined): Props => (s?.properties ?? {}) as Props;

function need(s: JsonSchema | undefined, what: string): JsonSchema {
  if (!s || typeof s !== 'object') throw new Error(`${what} schema not found`);
  return s;
}

/** `routing.bgp` (the one schema, 00-CONTEXT rule 5). */
export function bgpSchema(): JsonSchema {
  return need(props(domainSchemas.routing)['bgp'], 'routing.bgp');
}

/** The global part of `routing.bgp`: everything but the neighbour and peer-group records (edited in their own grids). */
export function bgpGlobalSchema(): JsonSchema {
  const s = bgpSchema();
  const p = { ...props(s) };
  delete p['neighbors'];
  delete p['peerGroups'];
  const required = (s.required as string[] | undefined)?.filter((r) => r in p);
  return { ...s, properties: p, ...(required ? { required } : {}) } as JsonSchema;
}

/** Value schema of a record property of `routing.bgp` (neighbors, peerGroups). */
export function bgpRecordItemSchema(key: 'neighbors' | 'peerGroups'): JsonSchema {
  return need(
    props(bgpSchema())[key]?.additionalProperties as JsonSchema | undefined,
    `routing.bgp.${key} item`,
  );
}

/** Value schema of `routing.policy.prefixLists` / `routeMaps`. */
export function policyItemSchema(key: 'prefixLists' | 'routeMaps'): JsonSchema {
  const policy = props(domainSchemas.routing)['policy'];
  return need(
    props(policy)[key]?.additionalProperties as JsonSchema | undefined,
    `routing.policy.${key} item`,
  );
}

/** `interfaces.<name>.lcp`. */
export function lcpSchema(): JsonSchema {
  const item = need(
    domainSchemas.interfaces.additionalProperties as JsonSchema | undefined,
    'interfaces item',
  );
  const lcp = props(item)['lcp'];
  const variants = (lcp?.anyOf ?? lcp?.oneOf) as JsonSchema[] | undefined;
  return need(variants?.find((v) => v.type === 'object') ?? lcp, 'interfaces.<n>.lcp');
}

/** Object names of the document (primitives.ts objectName). */
export const OBJECT_NAME = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;

/** Neighbour address key check (IPv4 or IPv6, no zone). */
export function isAddress(s: string): boolean {
  if (/^(?:(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)\.){3}(?:25[0-5]|2[0-4]\d|1\d\d|[1-9]?\d)$/.test(s))
    return true;
  return /^[0-9a-fA-F:.]+$/.test(s) && s.includes(':') && !s.includes(':::');
}

/** Live neighbour state from `GET /state/bgp`. */
export interface NeighborLive {
  address: string;
  state: string;
  uptimeSec: number;
  prefixesReceived: number;
  prefixesSent: number;
  flaps: number;
}

/** Neighbour grid row: configuration + live state. */
export interface NeighborRow {
  id: string;
  address: string;
  remoteAs: string;
  peerGroup: string;
  description: string;
  families: string;
  shutdown: boolean;
  state: string;
  uptimeSec: number | undefined;
  prefixesReceived: number | undefined;
  prefixesSent: number | undefined;
  flaps: number | undefined;
}

export function neighborRows(
  bgp: BgpConfig | undefined,
  live: readonly NeighborLive[],
): NeighborRow[] {
  const byAddr = new Map(live.map((n) => [n.address.toLowerCase(), n]));
  return Object.entries(bgp?.neighbors ?? {})
    .map(([address, n]) => {
      const group = n.peerGroup !== undefined ? bgp?.peerGroups?.[n.peerGroup] : undefined;
      const l = byAddr.get(address.toLowerCase());
      const afi = { ...(group?.afi ?? {}), ...(n.afi ?? {}) };
      return {
        id: address,
        address,
        remoteAs: String(n.remoteAs ?? group?.remoteAs ?? ''),
        peerGroup: n.peerGroup ?? '',
        description: n.description ?? '',
        families: Object.entries(afi)
          .filter(([, f]) => f !== undefined && f.enabled !== false)
          .map(([k]) => (k === 'ipv4Unicast' ? 'IPv4' : 'IPv6'))
          .join(', '),
        shutdown: n.shutdown === true,
        state: l?.state ?? '',
        uptimeSec: l?.uptimeSec,
        prefixesReceived: l?.prefixesReceived,
        prefixesSent: l?.prefixesSent,
        flaps: l?.flaps,
      };
    })
    .sort((a, b) => a.address.localeCompare(b.address, undefined, { numeric: true }));
}

/** StatusChip kind of an FRR session state. */
export function stateChip(
  state: string,
  shutdown: boolean,
): 'up' | 'down' | 'adminDown' | 'degraded' {
  if (shutdown || state.startsWith('Idle (Admin)')) return 'adminDown';
  if (state === 'Established') return 'up';
  if (state === '') return 'degraded';
  return 'down';
}

/** "3d 04:05:06" style uptime. */
export function formatUptime(sec: number | undefined): string {
  if (sec === undefined || sec <= 0) return '';
  const d = Math.floor(sec / 86400);
  const hms = [Math.floor((sec % 86400) / 3600), Math.floor((sec % 3600) / 60), sec % 60]
    .map((x) => String(x).padStart(2, '0'))
    .join(':');
  return d > 0 ? `${d}d ${hms}` : hms;
}

/** Sources `routing.bgp.redistribute` accepts (BGP cannot redistribute into itself). */
export const REDISTRIBUTE_SOURCES = ['connected', 'static', 'ospf', 'isis', 'rip'] as const;
export type RedistributeSource = (typeof REDISTRIBUTE_SOURCES)[number];

/** A redistribute record with `source` switched on (`{}`, keeping existing options) or off (removed). */
export function toggleRedistribute(
  cur: Record<string, unknown> | undefined,
  source: RedistributeSource,
  on: boolean,
): Record<string, unknown> {
  const next = { ...(cur ?? {}) };
  if (on) next[source] = next[source] ?? {};
  else delete next[source];
  return next;
}
