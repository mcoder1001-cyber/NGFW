import { z } from 'zod';
import type { RoutingStateResponse } from '@ngfw/proto';
import { isPlainObject } from '@ngfw/schema';

export const ObservationOut = z.strictObject({
  frrRunning: z.boolean(),
  unavailable: z
    .enum(['frr-unavailable', 'reader-unavailable', 'reader-invalid', 'reader-limit-exceeded'])
    .nullable(),
  offset: z.int().nonnegative(),
  limit: z.int().min(1).max(100),
  total: z.int().nonnegative(),
  rows: z.array(z.record(z.string(), z.union([z.string(), z.number(), z.boolean()]))).max(100),
});
const fields = new Set([
  'interfaceName',
  'interface',
  'state',
  'area',
  'areaId',
  'cost',
  'networkType',
  'neighborCount',
  'neighborFullCount',
  'linkStateId',
  'lsId',
  'lsaType',
  'type',
  'age',
  'lsaAge',
  'sequenceNumber',
  'seqNum',
  'checksum',
  'advertisingRouter',
  'routerId',
  'vrfName',
]);
/** Only bounded public scalar observation fields escape; nested daemon output and secrets never do. */
export function observations(
  response: RoutingStateResponse,
  reader: string,
  offset: number,
  limit: number,
): z.infer<typeof ObservationOut> {
  const out: z.infer<typeof ObservationOut> = {
    frrRunning: response.frrRunning,
    unavailable: null,
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
    if (!isPlainObject(root) && !Array.isArray(root)) throw new Error('shape');
    const rows: Record<string, string | number | boolean>[] = [];
    let visited = 0;
    const walk = (value: unknown, path: string[], depth: number): void => {
      if (++visited > 10000 || depth > 12) throw new Error('limit');
      if (Array.isArray(value)) {
        value.forEach((v, i) => walk(v, [...path, String(i)], depth + 1));
        return;
      }
      if (!isPlainObject(value)) return;
      const row: Record<string, string | number | boolean> = {};
      for (const [k, v] of Object.entries(value))
        if (
          fields.has(k) &&
          (typeof v === 'boolean' ||
            (typeof v === 'number' && Number.isFinite(v)) ||
            (typeof v === 'string' && v.length <= 96 && /^[A-Za-z0-9_.:/ -]*$/.test(v)))
        )
          row[k] = v as string | number | boolean;
      if (
        Object.keys(row).length > 0 &&
        (!reader.endsWith('Database') || 'linkStateId' in row || 'lsId' in row)
      ) {
        const id = path.at(-1);
        if (id && id.length <= 96 && /^[A-Za-z0-9_.:/-]+$/.test(id)) row['id'] = id;
        rows.push(row);
      }
      for (const [k, v] of Object.entries(value))
        if (Array.isArray(v) || isPlainObject(v)) walk(v, [...path, k], depth + 1);
    };
    walk(root, [], 0);
    out.total = rows.length;
    out.rows = rows.slice(offset, offset + limit);
  } catch {
    out.unavailable = 'reader-invalid';
  }
  return out;
}
