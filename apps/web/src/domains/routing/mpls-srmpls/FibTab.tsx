import RefreshIcon from '@mui/icons-material/Refresh';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchMplsFib, mplsStateKeys, ON_DEMAND, useRefreshMpls } from './api';
import { Mono } from './common';
import { fibPathParts, NS, tableChoices, type PathPart } from './model';
import { PathsView } from './PathsView';
import { useMpls } from './useMpls';

/** The label filter is a number, left-to-right in RTL too. */
const LABEL_INPUT = { inputMode: 'numeric', dir: 'ltr' } as const;

interface FibRow {
  id: string;
  label: number;
  eos: string;
  payload: string;
  paths: PathPart[][];
}

/**
 * The live MPLS FIB of one MPLS table (`GET /state/routing/mpls/fib`, paged by the agent). Every read is one full
 * walk of the table in VPP (D-132): the grid reads on demand — a page change, the table or label filter, or Refresh —
 * and never polls.
 */
export function FibTab() {
  const { t } = useTranslation(NS);
  const { mpls } = useMpls();
  const qc = useQueryClient();
  const refresh = useRefreshMpls();
  const [table, setTable] = useState(0);
  const [label, setLabel] = useState('');
  const [seen, setSeen] = useState<{ tableId: number; name: string }[]>([]);
  const [meta, setMeta] = useState<{ total: number; retrievedAt?: string | undefined } | null>(
    null,
  );
  useEffect(() => qc.setQueryDefaults(mplsStateKeys.all, ON_DEMAND), [qc]);
  const labelNum = /^\d+$/.test(label) ? Number(label) : undefined;
  const tr = useCallback((k: string, o?: Record<string, unknown>) => t(k, o ?? {}), [t]);

  const fetchPage = useCallback(
    async (req: ServerPageRequest) => {
      const r = await fetchMplsFib({
        table,
        ...(labelNum !== undefined ? { label: labelNum } : {}),
        page: req.page + 1,
        pageSize: req.pageSize,
      });
      setSeen(r.tables);
      setMeta({ total: r.total, retrievedAt: r.retrievedAt });
      const rows: FibRow[] = r.items.map((e) => ({
        id: `${e.label}/${e.eos ? 'eos' : 'neos'}`,
        label: e.label,
        eos: e.eos ? t('eos.yes') : t('eos.no'),
        payload: e.payload ?? '',
        paths: e.paths.map((x) => fibPathParts(x, tr)),
      }));
      return { rows, total: r.total };
    },
    [table, labelNum, tr, t],
  );

  const columns = useMemo<GridColDef<FibRow>[]>(
    () => [
      {
        field: 'label',
        headerName: t('routes.col.label'),
        type: 'number',
        width: 120,
        sortable: false,
        renderCell: (p) => <Mono>{p.row.label}</Mono>,
      },
      { field: 'eos', headerName: t('routes.col.eos'), width: 150, sortable: false },
      { field: 'payload', headerName: t('routes.col.payload'), width: 110, sortable: false },
      {
        field: 'paths',
        headerName: t('routes.col.paths'),
        minWidth: 320,
        flex: 1,
        sortable: false,
        renderCell: (p) => <PathsView paths={p.row.paths} />,
      },
    ],
    [t],
  );

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('fib.intro')}
      </Typography>
      <Stack direction="row" gap={2} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
        <TextField
          select
          size="small"
          label={t('fib.table')}
          value={table}
          onChange={(e) => setTable(Number(e.target.value))}
          sx={{ minInlineSize: 160 }}
        >
          {tableChoices(mpls, seen).map((id) => (
            <MenuItem key={id} value={id}>
              {id === 0 ? t('fib.table0') : id}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('fib.label')}
          value={label}
          onChange={(e) => setLabel(e.target.value.trim())}
          error={label !== '' && labelNum === undefined}
          inputProps={LABEL_INPUT}
        />
        <Button startIcon={<RefreshIcon />} onClick={refresh}>
          {t('refresh')}
        </Button>
        {meta && (
          <Typography variant="body2" color="text.secondary">
            {t('fib.meta', {
              total: meta.total,
              at: meta.retrievedAt ? new Date(meta.retrievedAt).toLocaleTimeString() : '',
            })}
          </Typography>
        )}
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: 520 }}>
        <ServerDataGrid<FibRow>
          aria-label={t('fib.title')}
          columns={columns}
          queryKey={[...mplsStateKeys.all, 'fib', table, labelNum ?? null]}
          fetchPage={fetchPage}
          initialPageSize={50}
          refetchInterval={false}
          disableColumnFilter
        />
      </Paper>
    </Box>
  );
}
