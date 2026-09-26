import Box from '@mui/material/Box';
import type { ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import type { ProblemDetails } from '@ngfw/ui-kit/schema-form';
import type { ReactNode } from 'react';
import { ApiError } from '../../../api-problem';

/** RFC 6901 segment escape. */
export const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');

/**
 * Server pointers under `prefix` (e.g. `/routing/mpls/tunnels/t1`) → pointers relative to the edited form; the entry's
 * own pointer (a record key the server refuses) → `keyField` when given.
 */
export function problemFor(
  error: unknown,
  prefix: string,
  keyField?: string,
): ProblemDetails | null {
  if (!(error instanceof ApiError)) return null;
  const p = error.toFormProblem();
  return {
    ...p,
    errors: (p.errors ?? []).map((e) => {
      if (keyField !== undefined && e.pointer === prefix) return { ...e, pointer: `/${keyField}` };
      return {
        ...e,
        pointer: e.pointer.startsWith(`${prefix}/`) ? e.pointer.slice(prefix.length) : e.pointer,
      };
    }),
  };
}

function cmp(a: unknown, b: unknown): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true });
}

/** Client-side paging/sort/filter of a small table (the candidate is one document). */
export function pageOf<R extends object>(
  rows: R[],
  req: ServerPageRequest,
  text: (r: R) => string,
): { rows: R[]; total: number } {
  let list = rows;
  const q = req.quickFilter.map((x) => x.toLowerCase());
  if (q.length > 0) list = list.filter((r) => q.every((x) => text(r).toLowerCase().includes(x)));
  for (const f of req.filter) {
    const needle = String(f.value ?? '').toLowerCase();
    list = list.filter((r) =>
      String((r as Record<string, unknown>)[f.field] ?? '')
        .toLowerCase()
        .includes(needle),
    );
  }
  if (req.sort.length > 0) {
    list = [...list].sort((a, b) => {
      for (const s of req.sort) {
        const c = cmp(
          (a as Record<string, unknown>)[s.field],
          (b as Record<string, unknown>)[s.field],
        );
        if (c !== 0) return s.dir === 'asc' ? c : -c;
      }
      return 0;
    });
  }
  const start = req.page * req.pageSize;
  return { rows: list.slice(start, start + req.pageSize), total: list.length };
}

/** Labels, addresses and interface names stay left-to-right inside RTL text. */
export function Mono({ children }: { children: ReactNode }) {
  return (
    <Box
      component="span"
      dir="ltr"
      sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 13 }}
    >
      {children}
    </Box>
  );
}
