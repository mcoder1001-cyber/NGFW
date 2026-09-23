import Alert from '@mui/material/Alert';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { StatusChip, useFormatters, type VrxStatus, VRX_STATUSES } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PageHeader } from '../../shell/PageHeader';

interface DemoRow {
  id: number;
  name: string;
  state: VrxStatus;
  mtu: number;
  rx: number;
  updated: number;
}

/** Synthetic rows generated in the browser (labelled as such on the page). */
const TOTAL = 20_000;
const BASE_TIME = Date.UTC(2026, 8, 23, 12, 0, 0);

function makeRows(): DemoRow[] {
  return Array.from({ length: TOTAL }, (_x, i) => ({
    id: i + 1,
    name: `demo-${String(i + 1).padStart(5, '0')}`,
    state: VRX_STATUSES[(i * 7) % VRX_STATUSES.length]!,
    mtu: [1500, 9000, 1400, 9216][i % 4]!,
    rx: ((i * 7919) % 1000) * 1_000_000,
    updated: BASE_TIME - (i % 3600) * 1000,
  }));
}

function compare(a: unknown, b: unknown): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return String(a).localeCompare(String(b));
}

function matches(row: DemoRow, f: ServerPageRequest['filter'][number]): boolean {
  const v = row[f.field as keyof DemoRow];
  const needle = f.value === undefined ? '' : String(f.value).toLowerCase();
  const hay = String(v).toLowerCase();
  switch (f.operator) {
    case 'contains':
      return hay.includes(needle);
    case 'doesNotContain':
      return !hay.includes(needle);
    case 'equals':
    case '=':
    case 'is':
      return hay === needle;
    case 'doesNotEqual':
    case '!=':
    case 'not':
      return hay !== needle;
    case 'startsWith':
      return hay.startsWith(needle);
    case 'endsWith':
      return hay.endsWith(needle);
    case '>':
      return Number(v) > Number(f.value);
    case '>=':
      return Number(v) >= Number(f.value);
    case '<':
      return Number(v) < Number(f.value);
    case '<=':
      return Number(v) <= Number(f.value);
    case 'isEmpty':
      return v === undefined || v === '';
    case 'isNotEmpty':
      return v !== undefined && v !== '';
    case 'isAnyOf':
      return Array.isArray(f.value) && f.value.map(String).includes(String(v));
    default:
      return true;
  }
}

export function DataGridDemoPage() {
  const { t } = useTranslation(['dev', 'common']);
  const fmt = useFormatters();
  const rows = useMemo(makeRows, []);

  const fetchPage = useMemo(
    () => async (req: ServerPageRequest, signal: AbortSignal) => {
      await new Promise((r) => setTimeout(r, 150));
      if (signal.aborted) throw new Error('aborted');
      let list = rows;
      if (req.filter.length > 0) {
        list = list.filter((row) => (req.filterLogic === 'or' ? req.filter.some((f) => matches(row, f)) : req.filter.every((f) => matches(row, f))));
      }
      if (req.quickFilter.length > 0) {
        list = list.filter((row) => req.quickFilter.every((q) => `${row.name} ${row.state}`.toLowerCase().includes(q.toLowerCase())));
      }
      if (req.sort.length > 0) {
        list = [...list].sort((a, b) => {
          for (const s of req.sort) {
            const c = compare(a[s.field as keyof DemoRow], b[s.field as keyof DemoRow]);
            if (c !== 0) return s.dir === 'asc' ? c : -c;
          }
          return 0;
        });
      }
      const start = req.page * req.pageSize;
      return { rows: list.slice(start, start + req.pageSize), total: list.length };
    },
    [rows],
  );

  const columns = useMemo<GridColDef<DemoRow>[]>(
    () => [
      { field: 'id', headerName: t('dev:dataGrid.columns.id'), type: 'number', width: 90 },
      { field: 'name', headerName: t('dev:dataGrid.columns.name'), width: 180 },
      {
        field: 'state',
        headerName: t('dev:dataGrid.columns.state'),
        width: 150,
        type: 'singleSelect',
        valueOptions: [...VRX_STATUSES],
        renderCell: (p) => <StatusChip status={p.row.state} size="small" />,
      },
      { field: 'mtu', headerName: t('dev:dataGrid.columns.mtu'), type: 'number', width: 100, valueFormatter: (v: number) => fmt.integer(v) },
      { field: 'rx', headerName: t('dev:dataGrid.columns.rx'), type: 'number', width: 140, valueFormatter: (v: number) => fmt.rate(v, 'bps') },
      { field: 'updated', headerName: t('dev:dataGrid.columns.updated'), width: 220, valueFormatter: (v: number) => fmt.dateTime(v) },
    ],
    [t, fmt],
  );

  return (
    <PageHeader title={t('dev:dataGrid.title')}>
      <Stack gap={2}>
        <Alert severity="warning">{t('common:dev.banner')}</Alert>
        <Typography>{t('dev:dataGrid.body', { count: TOTAL })}</Typography>
        <Paper sx={{ blockSize: 560, display: 'flex', flexDirection: 'column' }}>
          <ServerDataGrid<DemoRow> columns={columns} queryKey={['dev', 'data-grid']} fetchPage={fetchPage} initialPageSize={50} showToolbar />
        </Paper>
      </Stack>
    </PageHeader>
  );
}
