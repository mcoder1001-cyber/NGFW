import { problems } from '../../common/problem.js';
import type { AgentClient } from '../../agent/agent.client.js';

/**
 * P12's part of F-vrf-static-ecmp's FIB browser (`GET /api/v1/state/routes`, wave-A-hotspots P2 shared hunk): routes that
 * linux-nl synced from the kernel into VPP carry the FIB sources `lcp-rt` (kernel/boot/static protocols) and
 * `lcp-rt-dynamic` (routing daemons — FRR installs every route it learns with a protocol above `static`), so VPP knows
 * that a route came from FRR but not from which FRR protocol. The browser shows those routes with origin `frr`; with
 * `proto=<p>` it pushes the source filter down to the agent (`lcp-rt-dynamic`) and asks FRR's RIB (RoutingState, one
 * scoped lookup per prefix of the page, at most PROTO_PAGE_MAX) which protocol each prefix has, then keeps the page's
 * routes of that protocol. `total` then counts every FRR-dynamic route of the VRF (an upper bound: VPP cannot count by
 * FRR protocol), flagged by `protoFiltered`.
 */

/** FRR protocols `proto` accepts (FRR's own names; F-ospf/F-isis-rip routes use the same source). */
export const FRR_PROTOCOLS = [
  'bgp',
  'ospf',
  'ospf6',
  'isis',
  'rip',
  'ripng',
  'static',
  'kernel',
] as const;
export type FrrProtocol = (typeof FRR_PROTOCOLS)[number];

/** Largest page `proto` filters (one FRR RIB lookup per prefix, RF-1 review M3: no table dump). */
export const PROTO_PAGE_MAX = 100;

/** The FIB sources linux-nl uses for routes from Linux. */
export const LCP_SOURCES = ['lcp-rt', 'lcp-rt-dynamic'] as const;

/** `origin` of a FIB entry: `frr` for the linux-nl sources, else what the caller derived. */
export function frrOrigin(source: string, origin: string): string {
  return (LCP_SOURCES as readonly string[]).includes(source) ? 'frr' : origin;
}

/** The FIB source a `proto` filter pushes down: every FRR-installed protocol is `lcp-rt-dynamic`. */
export function protoSource(
  proto: FrrProtocol | undefined,
  source: string | undefined,
  pageSize: number,
): string {
  if (proto === undefined) return source ?? '';
  if (source !== undefined && source !== 'lcp-rt-dynamic')
    throw problems.badRequest(
      `proto=${proto} filters FRR routes (source lcp-rt-dynamic); drop source=${source}`,
    );
  if (pageSize > PROTO_PAGE_MAX)
    throw problems.badRequest(
      `proto filters at most ${PROTO_PAGE_MAX} routes per page (pageSize ${pageSize})`,
    );
  return 'lcp-rt-dynamic';
}

interface Annotatable {
  vrf: string;
  prefix: string;
  source: string;
  origin: string;
  proto?: string | undefined;
}

/**
 * Sets `origin: 'frr'` on the linux-nl routes of items and, when the page is small enough (or a proto filter asks for
 * it), their FRR protocol from FRR's RIB; with `proto` it returns only that protocol's routes. FRR not answering leaves
 * the protocol unset (and, with `proto`, an empty page).
 */
export async function annotateFrr<T extends Annotatable>(
  agent: AgentClient,
  items: T[],
  proto: FrrProtocol | undefined,
): Promise<T[]> {
  const frrItems = items.filter((i) => (LCP_SOURCES as readonly string[]).includes(i.source));
  for (const i of frrItems) i.origin = 'frr';
  if (frrItems.length === 0 || (proto === undefined && frrItems.length > PROTO_PAGE_MAX)) {
    return proto === undefined ? items : [];
  }
  const byVrf = new Map<string, T[]>();
  for (const i of frrItems) byVrf.set(i.vrf, [...(byVrf.get(i.vrf) ?? []), i]);
  for (const [vrf, list] of byVrf) {
    const r = await agent.routingState({
      readers: [],
      ribPrefixes: list.map((i) => i.prefix).slice(0, PROTO_PAGE_MAX),
      ribVrf: vrf === 'default' ? '' : vrf,
    });
    const selected = new Map<string, string>();
    for (const e of r.rib)
      if (e.selected && !selected.has(e.prefix)) selected.set(e.prefix, e.protocol);
    for (const i of list) {
      const p = selected.get(i.prefix);
      if (p !== undefined) i.proto = p;
    }
  }
  return proto === undefined ? items : items.filter((i) => i.proto === proto);
}
