import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { fetchRoutes, FIB_POLL_MS, routeKeys, useCandidate, type RouteItem, type RoutesQuery } from './api';
import { Mono } from './common';
import { NS, type VrfsConfig } from './model';

const SOURCES = ['', 'API', 'interface', 'adjacency', 'recursive-resolution', 'default-route', 'special', 'svs'] as const;
const LTR = { dir: 'ltr' } as const;
const FAMILIES = ['ipv4', 'ipv6'] as const;
const AUTO_HEIGHT = () => 'auto' as const;

interface FibRow extends RouteItem {
  id: string;
}

/** One path as text: "normal 10.2.1.2 via lan w=3 (table 2001) resolve-via-host". */
function pathText(p: RouteItem['paths'][number]): string {
  return [
    p.type,
    p.nextHop ?? '',
    p.interface ? `via ${p.interface}` : '',
    `w=${p.weight}`,
    p.tableId ? `(table ${p.tableId})` : '',
    ...p.flags,
  ]
    .filter(Boolean)
    .join(' ');
}

/**
 * The FIB browser: one VRF's live FIB through `GET /api/v1/state/routes` — the agent reads the table and returns only
 * the requested page (ListRoutes), so the grid stays 1M-safe (server-side paging; no sort/filter beyond the agent's:
 * family, prefix, source). Each entry shows its best FIB source and its paths (what they resolve to: the DPO kind).
 */
export function FibTab() {
  const { t } = useTranslation(NS);
  const vrfs = useCandidate<VrfsConfig>('vrfs');
  const names = useMemo(() => ['default', ...Object.keys(vrfs.data ?? {}).filter((n) => n !== 'default').sort()], [vrfs.data]);
  const [vrf, setVrf] = useState('default');
  const [family, setFamily] = useState<'' | 'ipv4' | 'ipv6'>('');
  const [prefix, setPrefix] = useState('');
  const [source, setSource] = useState<string>('');
  const [meta, setMeta] = useState<{ tableId?: number | undefined; retrievedAt?: string | undefined; error?: string }>({});
  const prefixOk = prefix === '' || /^[0-9a-fA-F.:]+\/[0-9]{1,3}$/.test(prefix.trim());

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const q: RoutesQuery = { vrf, page: req.page + 1, pageSize: req.pageSize };
      if (family) q.family = family;
      if (prefix && prefixOk) q.prefix = prefix.trim();
      if (source) q.source = source;
      const r = await fetchRoutes(q, signal);
      setMeta({ tableId: r.tableId, retrievedAt: r.retrievedAt });
      return { rows: r.items.map((it, i) => ({ ...it, id: `${req.page}:${i}:${it.prefix}` })), total: r.total };
    },
    [vrf, family, prefix, prefixOk, source],
  );

  const columns = useMemo<GridColDef<FibRow>[]>(
    () => [
      { field: 'prefix', headerName: t('fib.col.prefix'), minWidth: 190, flex: 1, sortable: false, filterable: false, renderCell: (p) => <Mono>{p.row.prefix}</Mono> },
      {
        field: 'source',
        headerName: t('fib.col.source'),
        width: 170,
        sortable: false,
        filterable: false,
        renderCell: (p) => <Chip size="small" variant="outlined" label={p.row.source} color={p.row.source === 'API' ? 'primary' : 'default'} />,
      },
      {
        field: 'paths',
        headerName: t('fib.col.paths'),
        minWidth: 360,
        flex: 3,
        sortable: false,
        filterable: false,
        renderCell: (p) => (
          <Stack sx={{ py: 0.5 }}>
            {p.row.paths.map((x, i) => (
              <Mono key={i}>{pathText(x)}</Mono>
            ))}
          </Stack>
        ),
      },
      { field: 'statsIndex', headerName: t('fib.col.stats'), type: 'number', width: 110, sortable: false, filterable: false },
    ],
    [t],
  );

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('fib.intro')}
      </Typography>
      <Stack direction={{ xs: 'column', md: 'row' }} gap={1} sx={{ mb: 1 }} alignItems={{ md: 'center' }}>
        <TextField select size="small" label={t('fib.vrf')} value={vrf} onChange={(e) => setVrf(e.target.value)} sx={{ minInlineSize: 160 }}>
          {names.map((n) => (
            <MenuItem key={n} value={n}>
              {n}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label={t('fib.family')}
          value={family}
          onChange={(e) => setFamily(e.target.value as '' | 'ipv4' | 'ipv6')}
          sx={{ minInlineSize: 120 }}
        >
          <MenuItem value="">{t('fib.any')}</MenuItem>
          {FAMILIES.map((f) => (
            <MenuItem key={f} value={f}>
              {t(`fib.${f}`)}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          size="small"
          label={t('fib.prefix')}
          value={prefix}
          onChange={(e) => setPrefix(e.target.value)}
          error={!prefixOk}
          helperText={prefixOk ? ' ' : t('fib.badPrefix')}
          slotProps={{ htmlInput: LTR }}
          sx={{ minInlineSize: 200 }}
        />
        <TextField select size="small" label={t('fib.source')} value={source} onChange={(e) => setSource(e.target.value)} sx={{ minInlineSize: 180 }}>
          {SOURCES.map((s) => (
            <MenuItem key={s} value={s}>
              {s === '' ? t('fib.any') : s}
            </MenuItem>
          ))}
        </TextField>
        <Box sx={{ flex: 1 }} />
        {meta.tableId !== undefined && <Chip size="small" variant="outlined" label={t('fib.table', { id: meta.tableId })} />}
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: 560 }}>
        <ServerDataGrid<FibRow>
          aria-label={t('fib.title')}
          columns={columns}
          queryKey={[...routeKeys.all, 'grid', { vrf, family, prefix: prefixOk ? prefix : '', source }]}
          fetchPage={fetchPage}
          refetchInterval={FIB_POLL_MS}
          initialPageSize={100}
          pageSizeOptions={[25, 100, 500, 1000]}
          getRowHeight={AUTO_HEIGHT}
          disableColumnMenu
        />
      </Paper>
    </Box>
  );
}
