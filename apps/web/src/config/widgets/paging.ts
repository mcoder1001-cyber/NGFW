import type { ServerFilter, ServerPage, ServerPageRequest } from '@ngfw/ui-kit/data-grid';

/**
 * Client-side paging/sort/filter for tables the API returns whole (the interface table, a configured collection, the
 * secret references): the grid keeps talking the ServerDataGrid protocol, so a table can move to real server paging
 * without touching the screen. Generalises P08's `pageOf`.
 */

function cmp(a: unknown, b: unknown): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true });
}

/** Cell value of a row for sorting/filtering: a plain member by default. */
export type CellValue<R> = (row: R, field: string) => unknown;

const member = <R>(row: R, field: string): unknown => (row as Record<string, unknown>)[field];

/** One MUI filter item against one value (operators of the string, number and boolean column types). */
export function matchesFilter(value: unknown, f: ServerFilter): boolean {
  const hay = String(value ?? '').toLowerCase();
  const needle = f.value === undefined || f.value === null ? '' : String(f.value).toLowerCase();
  const num = Number(value);
  const want = Number(f.value);
  switch (f.operator) {
    case 'isEmpty':
      return hay === '';
    case 'isNotEmpty':
      return hay !== '';
    case 'equals':
    case 'is':
    case '=':
      return typeof value === 'number' ? num === want : hay === needle;
    case 'doesNotEqual':
    case 'not':
    case '!=':
      return typeof value === 'number' ? num !== want : hay !== needle;
    case 'doesNotContain':
      return !hay.includes(needle);
    case 'startsWith':
      return hay.startsWith(needle);
    case 'endsWith':
      return hay.endsWith(needle);
    case '>':
      return num > want;
    case '>=':
      return num >= want;
    case '<':
      return num < want;
    case '<=':
      return num <= want;
    case 'isAnyOf':
      return Array.isArray(f.value) && f.value.some((v) => String(v).toLowerCase() === hay);
    case 'contains':
    default:
      return hay.includes(needle);
  }
}

/** Default quick-filter text of a row: every primitive member. */
export function defaultSearchText(row: unknown): string {
  if (typeof row !== 'object' || row === null) return String(row ?? '');
  return Object.values(row)
    .filter((v) => typeof v === 'string' || typeof v === 'number' || typeof v === 'boolean')
    .join(' ');
}

/** One page of `rows` for a grid request (quick filter on `searchText`, column filters, multi-column sort). */
export function pageRows<R>(
  rows: readonly R[],
  req: ServerPageRequest,
  searchText: (row: R) => string = defaultSearchText,
  valueOf: CellValue<R> = member,
): ServerPage<R> {
  let list = [...rows];
  const q = req.quickFilter.map((x) => x.toLowerCase()).filter((x) => x !== '');
  if (q.length > 0) list = list.filter((r) => q.every((x) => searchText(r).toLowerCase().includes(x)));
  if (req.filter.length > 0) {
    list = list.filter((r) => {
      const hit = (f: ServerFilter) => matchesFilter(valueOf(r, f.field), f);
      return req.filterLogic === 'or' ? req.filter.some(hit) : req.filter.every(hit);
    });
  }
  if (req.sort.length > 0) {
    list.sort((a, b) => {
      for (const s of req.sort) {
        const c = cmp(valueOf(a, s.field), valueOf(b, s.field));
        if (c !== 0) return s.dir === 'asc' ? c : -c;
      }
      return 0;
    });
  }
  const start = req.page * req.pageSize;
  return { rows: list.slice(start, start + req.pageSize), total: list.length };
}
