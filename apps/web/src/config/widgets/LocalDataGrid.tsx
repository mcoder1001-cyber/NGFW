import { ServerDataGrid, type GridValidRowModel, type ServerDataGridProps, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import type { QueryKey } from '@tanstack/react-query';
import { useCallback, useId, useRef } from 'react';
import { pageRows, type CellValue } from './paging';

export interface LocalDataGridProps<R extends GridValidRowModel> extends Omit<ServerDataGridProps<R>, 'fetchPage' | 'queryKey'> {
  /** Every row of the table (the API answers the whole list); the grid pages, sorts and filters them. */
  rows: readonly R[];
  /** Grid identity in the query cache (`['kit', 'secrets']`); the rows' version is appended. Never an API path. */
  gridKey: QueryKey;
  /** Quick-filter text of a row (default: its primitive members). */
  searchText?: ((row: R) => string) | undefined;
  /** Sort/filter value of a cell (default: the row member named like the column). */
  valueOf?: CellValue<R> | undefined;
}

/** A new number whenever the `rows` array is a new array (so the grid re-pages it); stable otherwise. */
function useVersion(rows: readonly unknown[]): number {
  const ref = useRef<{ rows: readonly unknown[]; v: number }>({ rows, v: 0 });
  if (ref.current.rows !== rows) ref.current = { rows, v: ref.current.v + 1 };
  return ref.current.v;
}

/**
 * ServerDataGrid over rows that are already in the browser (a configured collection, a small live table): the same
 * grid, paging protocol, empty/error overlays and RTL/locale handling as a server-paged table (docs/05 "Big tables").
 */
export function LocalDataGrid<R extends GridValidRowModel>({ rows, gridKey, searchText, valueOf, ...grid }: LocalDataGridProps<R>) {
  const version = useVersion(rows);
  // one id per mount (review L2): without it, a remount within the app's staleTime reuses a page cached by an
  // earlier mount under the same [...gridKey, version] key (e.g. Secrets → another page → back shows the pre-load page)
  const inst = useId();
  const latest = useRef(rows);
  latest.current = rows;
  const fetchPage = useCallback((req: ServerPageRequest) => Promise.resolve(pageRows(latest.current, req, searchText, valueOf)), [searchText, valueOf]);
  return <ServerDataGrid<R> {...grid} queryKey={[...gridKey, inst, version]} fetchPage={fetchPage} />;
}
