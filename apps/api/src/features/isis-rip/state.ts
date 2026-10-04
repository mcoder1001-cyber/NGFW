import { z } from 'zod';
import type { RoutingStateResponse } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';

export const IsisRipStateOut = z.strictObject({
  frrRunning: z.boolean(),
  unavailable: z
    .enum(['frr-unavailable', 'reader-unavailable', 'reader-invalid', 'reader-limit-exceeded'])
    .nullable(),
  scope: z.enum(['all-vrfs', 'default-vrf']),
  offset: z.int().nonnegative(),
  limit: z.int().min(1).max(100),
  total: z.int().nonnegative(),
  rows: z.array(z.record(z.string(), z.union([z.string(), z.number(), z.boolean()]))).max(100),
});
export type IsisRipState = z.infer<typeof IsisRipStateOut>;
const safeText = (v: unknown): v is string =>
  typeof v === 'string' && v.length <= 96 && /^[A-Za-z0-9_.:/ -]*$/.test(v);
const fields = new Set([
  'lspId',
  'lsp-id',
  'sequenceNumber',
  'sequence',
  'checksum',
  'lifetime',
  'holdtime',
  'level',
  'area',
  'vrf',
  'systemId',
  'prefix',
  'protocol',
  'distance',
  'metric',
  'selected',
  'installed',
  'nexthop',
  'ip',
  'interfaceName',
  'interface',
  'active',
  'fib',
]);
export function observed(
  response: RoutingStateResponse,
  reader: string,
  offset: number,
  limit: number,
): IsisRipState {
  const out: IsisRipState = {
    frrRunning: response.frrRunning,
    unavailable: null,
    scope: reader.endsWith('Status') ? 'default-vrf' : 'all-vrfs',
    offset,
    limit,
    total: 0,
    rows: [],
  };
  if (!response.frrRunning) {
    out.unavailable = 'frr-unavailable';
    return out;
  }
  const raw = response.readers[reader];
  if (raw === undefined) {
    out.unavailable = 'reader-unavailable';
    return out;
  }
  if (Buffer.byteLength(raw) > 1024 * 1024) {
    out.unavailable = 'reader-limit-exceeded';
    return out;
  }
  try {
    const root: unknown = JSON.parse(raw);
    if (!isPlainObject(root)) throw new Error('shape');
    const rows: IsisRipState['rows'] = [];
    const add = (row: Record<string, unknown>) => {
      const result: IsisRipState['rows'][number] = {};
      for (const [key, value] of Object.entries(row)) {
        if (
          safeText(value) ||
          (typeof value === 'number' && Number.isFinite(value)) ||
          typeof value === 'boolean'
        )
          result[key] = value;
        else throw new Error('public row');
      }
      rows.push(result);
    };
    if (reader.endsWith('Status')) {
      if (
        root['vrf'] !== 'default' ||
        !['rip', 'ripng'].includes(String(root['protocol'])) ||
        !Array.isArray(root['peers'])
      )
        throw new Error('status');
      for (const peer of root['peers']) {
        if (
          !isPlainObject(peer) ||
          !safeText(peer['address']) ||
          !safeText(peer['lastUpdate']) ||
          !['badPackets', 'badRoutes', 'distance'].every(
            (k) => Number.isSafeInteger(peer[k]) && Number(peer[k]) >= 0,
          )
        )
          throw new Error('peer');
        add({
          vrf: 'default',
          address: peer['address'],
          badPackets: peer['badPackets'],
          badRoutes: peer['badRoutes'],
          distance: peer['distance'],
          lastUpdate: peer['lastUpdate'],
        });
      }
    } else if (reader === 'isisNeighbors') {
      if (
        Object.keys(root).length !== 0 &&
        !Array.isArray(root['areas']) &&
        !Array.isArray(root['vrfs'])
      )
        throw new Error('neighbors');
      const vrfs = Array.isArray(root['vrfs'])
        ? root['vrfs']
        : [{ vrf: 'default', areas: root['areas'] ?? [] }];
      for (const vrf of vrfs) {
        if (!isPlainObject(vrf) || !Array.isArray(vrf['areas'])) throw new Error('areas');
        for (const area of vrf['areas']) {
          if (!isPlainObject(area) || !Array.isArray(area['circuits'])) throw new Error('circuits');
          for (const circuit of area['circuits']) {
            if (!isPlainObject(circuit)) throw new Error('circuit');
            const systemId = circuit['adj'] ?? circuit['system-id'];
            if (systemId === undefined || systemId === '') continue;
            const state = circuit['state'] ?? circuit['adj-state'];
            if (
              !safeText(systemId) ||
              !safeText(circuit['interface']) ||
              !safeText(state) ||
              !['string', 'number'].includes(typeof circuit['level'])
            )
              throw new Error('adjacency');
            add({
              vrf: vrf['vrf'] ?? vrf['vrf_name'] ?? 'default',
              area: area['area'] ?? '',
              systemId,
              interface: circuit['interface'],
              level: circuit['level'],
              state,
            });
          }
        }
      }
    } else {
      let visited = 0;
      const walk = (value: unknown, path: string[], depth: number): void => {
        if (++visited > 10000 || depth > 12) throw new Error('limit');
        if (Array.isArray(value)) {
          value.forEach((v, i) => walk(v, [...path, String(i)], depth + 1));
          return;
        }
        if (!isPlainObject(value)) return;
        const row: Record<string, unknown> = {};
        for (const [k, v] of Object.entries(value))
          if (fields.has(k) && (safeText(v) || typeof v === 'number' || typeof v === 'boolean'))
            row[k] = v;
        const relevant = reader.endsWith('Routes')
          ? 'protocol' in row || 'prefix' in row
          : 'lspId' in row || 'lsp-id' in row;
        if (relevant) {
          const prefix = path.find((p) => p.includes('/'));
          if (prefix && safeText(prefix)) row['prefix'] ??= prefix;
          add(row);
        }
        for (const [k, v] of Object.entries(value))
          if (Array.isArray(v) || isPlainObject(v)) walk(v, [...path, k], depth + 1);
      };
      walk(root, [], 0);
      if (
        Object.keys(root).length > 0 &&
        rows.length === 0 &&
        !Object.values(root).every((v) => isPlainObject(v) && Object.keys(v).length === 0)
      )
        throw new Error('unrecognized observation');
    }
    if (rows.length > 10000) throw new Error('limit');
    out.total = rows.length;
    out.rows = rows.slice(offset, offset + limit);
  } catch {
    out.unavailable = 'reader-invalid';
  }
  return out;
}
