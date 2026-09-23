import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTheme } from '@mui/material/styles';
import {
  DataGrid,
  type DataGridProps,
  type GridColDef,
  type GridFilterModel,
  type GridPaginationModel,
  type GridSortModel,
  type GridValidRowModel,
} from '@mui/x-data-grid';
import { enUS, faIR } from '@mui/x-data-grid/locales';
import { keepPreviousData, useQuery, type QueryKey } from '@tanstack/react-query';
import { useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../i18n/index.js';

export interface ServerSort {
  field: string;
  dir: 'asc' | 'desc';
}

export interface ServerFilter {
  field: string;
  /** MUI operator name (`contains`, `equals`, `>`, `isEmpty`, …); the API maps it to its own query syntax. */
  operator: string;
  value?: unknown;
}

/** What the grid asks the server for. Query keys mirror API paths: `[...queryKey, request]`. */
export interface ServerPageRequest {
  page: number;
  pageSize: number;
  sort: ServerSort[];
  filter: ServerFilter[];
  filterLogic: 'and' | 'or';
  quickFilter: string[];
}

export interface ServerPage<R> {
  rows: R[];
  /** Total matching rows on the server (drives the pager). */
  total: number;
}

export type FetchPage<R> = (request: ServerPageRequest, signal: AbortSignal) => Promise<ServerPage<R>>;

type Inherited<R extends GridValidRowModel> = Omit<
  DataGridProps<R>,
  | 'rows'
  | 'rowCount'
  | 'loading'
  | 'paginationMode'
  | 'sortingMode'
  | 'filterMode'
  | 'paginationModel'
  | 'onPaginationModelChange'
  | 'sortModel'
  | 'onSortModelChange'
  | 'filterModel'
  | 'onFilterModelChange'
  | 'columns'
>;

export interface ServerDataGridProps<R extends GridValidRowModel = GridValidRowModel> extends Inherited<R> {
  columns: readonly GridColDef<R>[];
  /** Base TanStack Query key (e.g. `['state', 'interfaces']`); the page request is appended. */
  queryKey: QueryKey;
  fetchPage: FetchPage<R>;
  initialPageSize?: number | undefined;
  initialSort?: GridSortModel | undefined;
  /** Poll interval in ms for live tables (`false` = off). */
  refetchInterval?: number | false | undefined;
}

declare module '@mui/x-data-grid' {
  interface NoRowsOverlayPropsOverrides {
    error?: boolean;
    onRetry?: () => void;
  }
}

function StateOverlay({ error = false, onRetry }: { error?: boolean; onRetry?: () => void }) {
  const { t } = useTranslation(UI_KIT_NS);
  return (
    <Stack alignItems="center" justifyContent="center" gap={1} sx={{ blockSize: '100%', p: 2 }} role="status">
      <Typography color={error ? 'error' : 'text.secondary'}>{t(error ? 'grid.error' : 'grid.empty')}</Typography>
      {error && onRetry && (
        <Button size="small" variant="outlined" onClick={onRetry}>
          {t('grid.retry')}
        </Button>
      )}
    </Stack>
  );
}

function toRequest(p: GridPaginationModel, s: GridSortModel, f: GridFilterModel): ServerPageRequest {
  return {
    page: p.page,
    pageSize: p.pageSize,
    sort: s.flatMap((i) => (i.sort ? [{ field: i.field, dir: i.sort }] : [])),
    filter: f.items
      .filter((i) => i.operator === 'isEmpty' || i.operator === 'isNotEmpty' || (i.value !== undefined && i.value !== ''))
      .map((i) => ({ field: i.field, operator: i.operator, ...(i.value !== undefined ? { value: i.value } : {}) })),
    filterLogic: f.logicOperator === 'or' ? 'or' : 'and',
    quickFilter: (f.quickFilterValues ?? []).map(String),
  };
}

/**
 * MUI X DataGrid (MIT) with server-side paging/sorting/filtering bound to TanStack Query. Rows are
 * virtualised by the grid; only one page ever lives in the browser (docs/05 "Big tables").
 */
export function ServerDataGrid<R extends GridValidRowModel>({
  columns,
  queryKey,
  fetchPage,
  initialPageSize = 25,
  initialSort = [],
  refetchInterval = false,
  pageSizeOptions = [25, 50, 100],
  slots,
  slotProps,
  sx,
  ...rest
}: ServerDataGridProps<R>) {
  const { t, i18n } = useTranslation(UI_KIT_NS);
  const theme = useTheme();
  const [paginationModel, setPaginationModel] = useState<GridPaginationModel>({ page: 0, pageSize: initialPageSize });
  const [sortModel, setSortModel] = useState<GridSortModel>(initialSort);
  const [filterModel, setFilterModel] = useState<GridFilterModel>({ items: [] });
  const request = useMemo(() => toRequest(paginationModel, sortModel, filterModel), [paginationModel, sortModel, filterModel]);

  const query = useQuery({
    queryKey: [...queryKey, request],
    queryFn: ({ signal }) => fetchPage(request, signal),
    placeholderData: keepPreviousData,
    refetchInterval,
  });
  const lastTotal = useRef(0);
  if (query.data) lastTotal.current = query.data.total;

  const lang = i18n.resolvedLanguage ?? i18n.language ?? 'en';
  const localeText = (lang.startsWith('fa') ? faIR : enUS).components.MuiDataGrid.defaultProps.localeText;
  const retry = () => void query.refetch();
  // Review L5: the overlay only covers a change of page/sort/filter (placeholder rows shown), not a background poll.
  const loading = query.isPending || (query.isFetching && query.isPlaceholderData);
  // Review L5: a failed refetch while older rows are still shown must not go unnoticed.
  const staleError = query.isError && query.data !== undefined;

  return (
    <>
      {staleError && (
        <Alert
          severity="warning"
          role="alert"
          action={
            <Button color="inherit" size="small" onClick={retry}>
              {t('grid.retry')}
            </Button>
          }
        >
          {t('grid.staleError')}
        </Alert>
      )}
      <DataGrid<R>
        {...rest}
        columns={columns as GridColDef<R>[]}
        rows={query.data?.rows ?? []}
        rowCount={lastTotal.current}
        loading={loading}
        paginationMode="server"
        sortingMode="server"
        filterMode="server"
        paginationModel={paginationModel}
        onPaginationModelChange={setPaginationModel}
        sortModel={sortModel}
        onSortModelChange={(m) => {
          setSortModel(m);
          setPaginationModel((p) => ({ ...p, page: 0 }));
        }}
        filterModel={filterModel}
        onFilterModelChange={(m) => {
          setFilterModel(m);
          setPaginationModel((p) => ({ ...p, page: 0 }));
        }}
        pageSizeOptions={pageSizeOptions}
        density="compact"
        rowHeight={theme.vrx.denseRowHeight}
        columnHeaderHeight={theme.vrx.denseRowHeight + 4}
        disableRowSelectionOnClick
        localeText={{ ...localeText, ...rest.localeText }}
        slots={{ noRowsOverlay: StateOverlay, ...slots }}
        slotProps={{ ...slotProps, noRowsOverlay: { error: query.isError, onRetry: retry, ...slotProps?.noRowsOverlay } }}
        sx={[{ border: 0 }, ...(Array.isArray(sx) ? sx : sx ? [sx] : [])]}
      />
    </>
  );
}
