import Box from '@mui/material/Box';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import type { Lease, LeasesQuery } from './model';
import { dhcpKeys, fetchLeases, useCandidateServices, DHCP_POLL_MS } from './queries';

const V4 = 'ipv4' as const;
const V6 = 'ipv6' as const;

export interface LeaseRow extends Lease {
  id: string;
}

/**
 * The grid's page request → the API's lease query: server-side paging (1-based pages), the first quick-filter term as
 * the agent's substring filter, server and family from the selectors above the grid.
 */
export function leasesQuery(req: ServerPageRequest, server: string, family: string): LeasesQuery {
  const filter = (req.quickFilter[0] ?? '').trim().slice(0, 64); // the agent matches one substring
  return {
    page: req.page + 1,
    pageSize: req.pageSize,
    ...(filter ? { filter } : {}),
    ...(server ? { server } : {}),
    ...(family === 'ipv4' || family === 'ipv6' ? { family } : {}),
  };
}

export function LeasesTab() {
  const { t } = useTranslation('kea-dhcp-relay');
  const fmt = useFormatters();
  const candidate = useCandidateServices();
  const [server, setServer] = useState('');
  const [family, setFamily] = useState('');
  const [truncated, setTruncated] = useState(false);
  const servers = Object.keys(candidate.data?.dhcp?.servers ?? {}).sort();

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const data = await fetchLeases(leasesQuery(req, server, family), signal);
      setTruncated(data.truncated);
      return {
        rows: data.items.map((l) => ({ ...l, id: `${l.family}/${l.address}/${l.prefixLen}` })),
        total: data.total,
      };
    },
    [server, family],
  );

  const columns = useMemo<GridColDef<LeaseRow>[]>(
    () => [
      {
        field: 'address',
        headerName: t('col.address'),
        minWidth: 150,
        flex: 1,
        sortable: false,
        renderCell: (p) => (
          <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
            {p.row.prefixLen ? `${p.row.address}/${p.row.prefixLen}` : p.row.address}
          </Box>
        ),
      },
      {
        field: 'client',
        headerName: t('col.client'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        valueGetter: (_v, row) => row.hwAddress || row.duid || row.clientId,
        renderCell: (p) => (
          <Box
            component="span"
            dir="ltr"
            sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 12 }}
          >
            {p.value as string}
          </Box>
        ),
      },
      { field: 'hostname', headerName: t('col.hostname'), minWidth: 120, flex: 1, sortable: false },
      { field: 'server', headerName: t('col.server'), width: 110, sortable: false },
      { field: 'subnet', headerName: t('col.subnet'), width: 110, sortable: false },
      {
        field: 'state',
        headerName: t('col.state'),
        width: 130,
        sortable: false,
        valueFormatter: (v: string) => t(`leaseState.${v}`, { defaultValue: v }),
      },
      {
        field: 'expiresAt',
        headerName: t('col.expires'),
        width: 180,
        sortable: false,
        valueFormatter: (v: string | null) => (v ? fmt.dateTime(v) : ''),
      },
    ],
    [t, fmt],
  );

  return (
    <Box>
      <Stack direction="row" gap={2} sx={{ mb: 1 }} alignItems="center" flexWrap="wrap">
        <TextField
          select
          size="small"
          label={t('leases.server')}
          value={server}
          onChange={(e) => setServer(e.target.value)}
          sx={{ minInlineSize: 180 }}
        >
          <MenuItem value="">{t('leases.allServers')}</MenuItem>
          {servers.map((s) => (
            <MenuItem key={s} value={s} dir="ltr">
              {s}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          size="small"
          label={t('leases.family')}
          value={family}
          onChange={(e) => setFamily(e.target.value)}
          sx={{ minInlineSize: 140 }}
        >
          <MenuItem value="">{t('leases.allFamilies')}</MenuItem>
          <MenuItem value={V4}>{t('family.ipv4')}</MenuItem>
          <MenuItem value={V6}>{t('family.ipv6')}</MenuItem>
        </TextField>
        <Typography variant="body2" color="text.secondary" sx={{ flex: 1 }}>
          {truncated ? t('leases.truncated') : t('leases.hint')}
        </Typography>
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: 520 }}>
        <ServerDataGrid<LeaseRow>
          aria-label={t('tabs.leases')}
          columns={columns}
          queryKey={[...dhcpKeys.leases, 'grid', server, family]}
          fetchPage={fetchPage}
          refetchInterval={DHCP_POLL_MS}
          initialPageSize={25}
          showToolbar
          disableColumnFilter
        />
      </Paper>
    </Box>
  );
}
