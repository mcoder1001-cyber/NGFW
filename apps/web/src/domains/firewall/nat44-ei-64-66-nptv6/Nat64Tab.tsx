import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { nat64SessionId, NS, type Nat64Session, type Nat64SessionsPage } from './model';
import type { Subtree } from './model';
import { fetchNat64Sessions, keys, POLL_MS } from './queries';
import { SubtreeForm } from './SubtreeForm';

const SUBTREE: Subtree = 'nat64';

interface Row extends Nat64Session {
  id: string;
}

const PAGE_SIZES = [25, 50, 100, 500, 1000];
const PROTOCOLS = ['', 'tcp', 'udp', 'icmp'];
const ep = (a: string, p: number) => (a.includes(':') ? `[${a}]:${p}` : `${a}:${p}`);

/**
 * NAT64 (and the PLAT side of 464XLAT): the schema-driven `nat.nat64` form, then the live NAT64 session table
 * (`GET /state/nat/nat64/sessions`, server-side paged, read-only: VPP has no NAT64 session delete).
 */
export function Nat64Tab() {
  const { t } = useTranslation(NS);
  const fmt = useFormatters();
  const qc = useQueryClient();
  const [protocol, setProtocol] = useState('');
  const [meta, setMeta] = useState<Pick<
    Nat64SessionsPage,
    'total' | 'totalClients' | 'truncated'
  > | null>(null);
  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const page = await fetchNat64Sessions(req.page, req.pageSize, protocol, signal);
      setMeta({ total: page.total, totalClients: page.totalClients, truncated: page.truncated });
      return {
        rows: page.items.map((s): Row => ({ ...s, id: nat64SessionId(s) })),
        total: page.total,
      };
    },
    [protocol],
  );
  const columns = useMemo<GridColDef<Row>[]>(
    () => [
      { field: 'protocol', headerName: t('col.protocol'), width: 90, sortable: false },
      {
        field: 'client',
        headerName: t('col.client'),
        minWidth: 220,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.client, r.clientPort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      {
        field: 'pool',
        headerName: t('col.pool'),
        minWidth: 160,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.poolAddress, r.poolPort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      {
        field: 'remote',
        headerName: t('col.remote'),
        minWidth: 160,
        flex: 1,
        sortable: false,
        valueGetter: (_v, r) => ep(r.remote, r.remotePort),
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      {
        field: 'remoteIpv6',
        headerName: t('col.remoteIpv6'),
        minWidth: 200,
        flex: 1,
        sortable: false,
        renderCell: (p) => <span dir="ltr">{p.value as string}</span>,
      },
      { field: 'vrf', headerName: t('col.vrf'), width: 90, sortable: false },
    ],
    [t],
  );

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('nat64.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREE} />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('nat64.sessions')}
      </Typography>
      <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap" sx={{ mb: 1 }}>
        <TextField
          select
          size="small"
          label={t('filter.protocol')}
          value={protocol}
          onChange={(e) => setProtocol(e.target.value)}
          sx={{ inlineSize: 130 }}
        >
          {PROTOCOLS.map((p) => (
            <MenuItem key={p} value={p}>
              {p === '' ? t('col.any') : p}
            </MenuItem>
          ))}
        </TextField>
        <Typography variant="body2" role="status" data-testid="nat64-sessions-total">
          {meta
            ? t('nat64.total', {
                sessions: fmt.integer(meta.total),
                clients: fmt.integer(meta.totalClients),
              })
            : t('loading')}
        </Typography>
        {meta?.truncated && <Chip size="small" color="warning" label={t('sessions.truncated')} />}
        <Button
          size="small"
          onClick={() => void qc.invalidateQueries({ queryKey: keys.nat64Sessions })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      <Paper variant="outlined" sx={{ blockSize: 440 }}>
        <ServerDataGrid<Row>
          aria-label={t('nat64.sessions')}
          columns={columns}
          queryKey={[...keys.nat64Sessions, protocol]}
          fetchPage={fetchPage}
          refetchInterval={POLL_MS}
          initialPageSize={100}
          pageSizeOptions={PAGE_SIZES}
          disableColumnFilter
        />
      </Paper>
    </Box>
  );
}
