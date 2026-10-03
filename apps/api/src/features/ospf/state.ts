import { isIP } from 'node:net';
import type { RoutingStateResponse } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';
import { OSPF_ROW_LIMIT, type OspfNeighbor, type OspfState } from './dto.js';

export const OSPF_NEIGHBORS_READER = 'ospfNeighbors';
const MAX_READER_BYTES = 1024 * 1024;
const MAX_ROWS_INSPECTED = 2000;
const MAX_INSTANCES = 128;
type Unavailable = NonNullable<OspfState['unavailable']>;
class ReaderError extends Error {
  constructor(readonly reason: Unavailable) {
    super(reason);
  }
}
function requireShape(condition: boolean): asserts condition {
  if (!condition) throw new ReaderError('reader-invalid');
}
const name = (value: unknown): string | null =>
  typeof value === 'string' &&
  value.length > 0 &&
  value.length <= 96 &&
  /^[A-Za-z0-9_.:/-]+$/.test(value)
    ? value
    : null;
const address = (value: unknown): string | null =>
  typeof value === 'string' && value.length <= 96 && isIP(value) !== 0 ? value : null;
const priority = (value: unknown): number | null =>
  typeof value === 'number' && Number.isInteger(value) && value >= 0 && value <= 255 ? value : null;
function parseNeighbors(raw: string): { rows: OspfNeighbor[]; partial: boolean } {
  if (Buffer.byteLength(raw, 'utf8') > MAX_READER_BYTES)
    throw new ReaderError('reader-limit-exceeded');
  const top: unknown = JSON.parse(raw);
  requireShape(isPlainObject(top));
  const instances = Object.hasOwn(top, 'neighbors')
    ? [['default', top] as const]
    : Object.entries(top);
  if (instances.length > MAX_INSTANCES) throw new ReaderError('reader-limit-exceeded');
  const rows: OspfNeighbor[] = [];
  let inspected = 0;
  let partial = false;
  for (const [vrf, instance] of instances) {
    requireShape(name(vrf) !== null && isPlainObject(instance));
    const neighbors = instance['neighbors'];
    requireShape(isPlainObject(neighbors));
    const routerIds = Object.entries(neighbors);
    if (routerIds.length > MAX_ROWS_INSPECTED) throw new ReaderError('reader-limit-exceeded');
    for (const [routerId, value] of routerIds) {
      // FRR uses this placeholder for NBMA Attempt rows before a router ID is known.
      const placeholder = routerId === 'neighbor';
      requireShape(placeholder || isIP(routerId) === 4);
      const entries = Array.isArray(value) ? value : [value];
      inspected += entries.length;
      if (inspected > MAX_ROWS_INSPECTED) throw new ReaderError('reader-limit-exceeded');
      for (const entry of entries) {
        requireShape(isPlainObject(entry));
        if (placeholder) {
          partial = true;
          continue;
        }
        const observed = entry['nbrState'] ?? entry['state'];
        const state =
          typeof observed === 'string' &&
          /^(Down|Attempt|Init|2-Way|ExStart|Exchange|Loading|Full|Deleted)(\/(DR|Backup|DROther|-))?$/.test(
            observed,
          )
            ? observed
            : 'Unknown';
        const iface =
          typeof entry['ifaceName'] === 'string' ? entry['ifaceName'].split(':', 1)[0] : null;
        rows.push({
          vrf,
          routerId,
          address: address(entry['ifaceAddress']) ?? address(entry['address']),
          interface: name(iface),
          state,
          priority: priority(entry['nbrPriority']) ?? priority(entry['priority']),
        });
      }
    }
  }
  rows.sort(
    (a, b) =>
      a.vrf.localeCompare(b.vrf) ||
      a.routerId.localeCompare(b.routerId) ||
      (a.interface ?? '').localeCompare(b.interface ?? '') ||
      (a.address ?? '').localeCompare(b.address ?? ''),
  );
  return { rows, partial };
}

/** Project only public adjacency facts; never return raw reader JSON or agent diagnostics. */
export function ospfStateOut(response: RoutingStateResponse): OspfState {
  const timestamp = response.retrievedAt;
  const state: OspfState = {
    frrRunning: response.frrRunning,
    retrievedAt: timestamp && Number.isFinite(timestamp.getTime()) ? timestamp.toISOString() : null,
    unavailable: null,
    warning: response.error ? 'routing-observation-partial' : null,
    truncated: false,
    neighbors: [],
  };
  if (!response.frrRunning) {
    state.unavailable = 'frr-unavailable';
    return state;
  }
  const raw = response.readers[OSPF_NEIGHBORS_READER];
  if (raw === undefined) {
    state.unavailable = 'reader-unavailable';
    return state;
  }
  try {
    const { rows, partial } = parseNeighbors(raw);
    if (partial) state.warning = 'routing-observation-partial';
    state.neighbors = rows.slice(0, OSPF_ROW_LIMIT);
    state.truncated = rows.length > OSPF_ROW_LIMIT;
  } catch (error) {
    state.unavailable = error instanceof ReaderError ? error.reason : 'reader-invalid';
  }
  return state;
}
