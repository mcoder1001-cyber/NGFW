import AddIcon from '@mui/icons-material/Add';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip, UI_KIT_NS, useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../auth/AuthProvider';
import { ProblemAlert } from '../../config/ProblemAlert';
import { PageHeader } from '../../shell/PageHeader';
import { InterfaceDrawer } from './InterfaceDrawer';
import { addressesOf, adminStatus, linkStatus, type InterfaceItem } from './model';
import { fetchInterfacesState, ifaceKeys, STATE_POLL_MS, usePatchInterfaces } from './queries';
import { useIfaceRates, type Rate } from './rates';
import { Sparkline } from './Sparkline';

/** Grid row: one (sub-)interface of `/state/interfaces` plus its live rates. */
export interface Row {
  id: string;
  name: string;
  item: InterfaceItem;
  type: string;
  admin: string;
  link: string;
  mtu: number | null;
  addresses: string;
  vrf: string;
  errors: number;
}

export function toRow(item: InterfaceItem): Row {
  const s = item.state;
  const cfg = (item.config ?? {}) as { mtu?: number; vrf?: string };
  return {
    id: item.name,
    name: item.name,
    item,
    type: s?.type ?? (item.kind === 'subinterface' ? 'sub-interface' : ''),
    admin: adminStatus(s) ?? '',
    link: linkStatus(s) ?? '',
    mtu: s?.mtu ?? cfg.mtu ?? null,
    addresses: addressesOf(item).join(' '),
    vrf: s?.vrf ?? cfg.vrf ?? 'default',
    errors: Number(item.counters?.errors ?? 0) + Number(item.counters?.drops ?? 0),
  };
}

function cmp(a: unknown, b: unknown): number {
  if (typeof a === 'number' && typeof b === 'number') return a - b;
  return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true });
}

/** Paging/sort/filter of the (small) interface table; the API returns the whole live table in one answer. */
export function pageOf(rows: Row[], req: ServerPageRequest): { rows: Row[]; total: number } {
  let list = rows;
  const q = req.quickFilter.map((x) => x.toLowerCase());
  if (q.length > 0) list = list.filter((r) => q.every((x) => `${r.name} ${r.type} ${r.addresses} ${r.vrf}`.toLowerCase().includes(x)));
  for (const f of req.filter) {
    const needle = String(f.value ?? '').toLowerCase();
    list = list.filter((r) => String(r[f.field as keyof Row] ?? '').toLowerCase().includes(needle));
  }
  if (req.sort.length > 0) {
    list = [...list].sort((a, b) => {
      for (const s of req.sort) {
        const c = cmp(a[s.field as keyof Row], b[s.field as keyof Row]);
        if (c !== 0) return s.dir === 'asc' ? c : -c;
      }
      return 0;
    });
  }
  const start = req.page * req.pageSize;
  return { rows: list.slice(start, start + req.pageSize), total: list.length };
}

const LTR = { dir: 'ltr' } as const;
const NAME_RE = /^[A-Za-z][A-Za-z0-9_./-]{0,62}$/;

export function InterfacesPage() {
  const { t } = useTranslation(['interfaces', 'common', UI_KIT_NS]);
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const { rates, status: wsStatus } = useIfaceRates();
  const [selected, setSelected] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState('');
  const patch = usePatchInterfaces();
  const [lastError, setLastError] = useState<unknown>(null);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const data = await qc.fetchQuery({ queryKey: ifaceKeys.state, queryFn: () => fetchInterfacesState(signal), staleTime: 1000 });
      return pageOf(data.items.map(toRow), req);
    },
    [qc],
  );

  const rateOf = useCallback((r: Row): Rate | undefined => rates.get(r.item.state?.vppName ?? r.name), [rates]);

  const columns = useMemo<GridColDef<Row>[]>(
    () => [
      {
        field: 'name',
        headerName: t('col.name'),
        minWidth: 170,
        flex: 1,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, paddingInlineStart: p.row.item.kind === 'subinterface' ? 2 : 0 }}>
              {p.row.name}
            </Box>
            {p.row.item.hasPendingChange && <Chip size="small" color="warning" variant="outlined" label={t('pending')} />}
          </Stack>
        ),
      },
      { field: 'type', headerName: t('col.type'), width: 120 },
      {
        field: 'admin',
        headerName: t('col.admin'),
        width: 120,
        renderCell: (p) => (p.row.admin ? <StatusChip size="small" status={adminStatus(p.row.item.state)!} /> : <Typography variant="body2" color="text.secondary">{t('notInVpp')}</Typography>),
      },
      {
        field: 'link',
        headerName: t('col.link'),
        width: 120,
        renderCell: (p) => (p.row.link ? <StatusChip size="small" status={linkStatus(p.row.item.state)!} /> : null),
      },
      { field: 'mtu', headerName: t('col.mtu'), type: 'number', width: 80, valueFormatter: (v: number | null) => (v === null ? '' : fmt.integer(v)) },
      {
        field: 'addresses',
        headerName: t('col.addresses'),
        minWidth: 180,
        flex: 1,
        renderCell: (p) => (
          <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 12 }}>
            {p.row.addresses}
          </Box>
        ),
      },
      { field: 'vrf', headerName: t('col.vrf'), width: 100 },
      {
        field: 'rx',
        headerName: t('col.rx'),
        sortable: false,
        filterable: false,
        width: 130,
        renderCell: (p) => {
          const r = rateOf(p.row);
          return r ? `${fmt.rate(r.rxBps, 'bps')} · ${fmt.rate(r.rxPps, 'pps')}` : '';
        },
      },
      {
        field: 'tx',
        headerName: t('col.tx'),
        sortable: false,
        filterable: false,
        width: 130,
        renderCell: (p) => {
          const r = rateOf(p.row);
          return r ? `${fmt.rate(r.txBps, 'bps')} · ${fmt.rate(r.txPps, 'pps')}` : '';
        },
      },
      {
        field: 'trend',
        headerName: t('col.trend'),
        sortable: false,
        filterable: false,
        width: 90,
        renderCell: (p) => {
          const r = rateOf(p.row);
          return r ? <Sparkline values={r.history} label={t('trendLabel', { name: p.row.name })} /> : null;
        },
      },
      { field: 'errors', headerName: t('col.errors'), type: 'number', width: 90, valueFormatter: (v: number) => fmt.integer(v) },
    ],
    [t, fmt, rateOf],
  );

  const addInterface = async () => {
    const name = newName.trim();
    try {
      setLastError(null);
      await patch.mutateAsync({ [name]: { enabled: false } });
      setAdding(false);
      setNewName('');
      setSelected(name);
    } catch (e) {
      setLastError(e);
    }
  };

  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center">
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button variant="contained" startIcon={<AddIcon />} disabled={!perms.editConfig} onClick={() => setAdding(true)}>
              {t('add')}
            </Button>
          </span>
        </Tooltip>
        <Box sx={{ flex: 1 }} />
        <Chip size="small" variant="outlined" label={t('liveChip', { status: t(`ws.${wsStatus}`, { ns: UI_KIT_NS }) })} color={wsStatus === 'open' ? 'success' : 'default'} />
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 520 }}>
        <ServerDataGrid<Row>
          aria-label={t('title')}
          columns={columns}
          queryKey={['state', 'interfaces', 'grid']}
          fetchPage={fetchPage}
          refetchInterval={STATE_POLL_MS}
          initialPageSize={25}
          onRowClick={(p) => setSelected(p.row.name)}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <InterfaceDrawer name={selected} onClose={() => setSelected(null)} rates={rates} />
      <Dialog open={adding} onClose={() => setAdding(false)} fullWidth maxWidth="xs">
        <DialogTitle>{t('addTitle')}</DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ mb: 2 }}>{t('addHelp')}</DialogContentText>
          <TextField
            autoFocus
            fullWidth
            label={t('col.name')}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            error={newName !== '' && !NAME_RE.test(newName.trim())}
            helperText={newName !== '' && !NAME_RE.test(newName.trim()) ? t('badName') : ' '}
            slotProps={{ htmlInput: LTR }}
          />
          {patch.isError && <Alert severity="error">{t('addFailed')}</Alert>}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAdding(false)}>{t('cancel')}</Button>
          <Button variant="contained" disabled={!NAME_RE.test(newName.trim()) || patch.isPending} onClick={() => void addInterface()}>
            {t('add')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
