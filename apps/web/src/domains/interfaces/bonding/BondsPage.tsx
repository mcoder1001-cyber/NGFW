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
import MenuItem from '@mui/material/MenuItem';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { BondMode, bondIdOf } from '@ngfw/schema';
import { StatusChip } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { useCandidateInterfaces, usePatchInterfaces } from '../queries';
import { BondDrawer } from './BondDrawer';
import { bondStatus, memberStatus, nextBondName, type BondItem } from './model';
import { bondKeys, BONDS_POLL_MS, fetchBondsState } from './queries';

/** Grid row: one bond of `/state/interfaces/bonds`. */
export interface BondRow {
  id: string;
  name: string;
  item: BondItem;
  mode: string;
  loadBalance: string;
  members: string;
  active: string;
  status: string;
}

type BondCfg = { mode?: string; loadBalance?: string; members?: Record<string, unknown> };

export function toBondRow(item: BondItem): BondRow {
  const s = item.state;
  // fallback when VPP has no such bond: the candidate (a new bond) or the running configuration
  const cfg = (item.candidate ?? item.running ?? {}) as BondCfg;
  const members = s ? s.members.map((m) => m.interface) : Object.keys(cfg.members ?? {});
  return {
    id: item.name,
    name: item.name,
    item,
    mode: s?.mode ?? cfg.mode ?? '',
    loadBalance: s?.loadBalance ?? cfg.loadBalance ?? '',
    members: members.join(' '),
    active: s ? `${s.activeMemberCount}/${s.memberCount}` : '',
    status: bondStatus(s) ?? '',
  };
}

function cmp(a: unknown, b: unknown): number {
  return String(a ?? '').localeCompare(String(b ?? ''), undefined, { numeric: true });
}

/** Paging/sort/filter of the (small) bond table; the API returns every bond in one answer. */
export function pageOfBonds(rows: BondRow[], req: ServerPageRequest): { rows: BondRow[]; total: number } {
  let list = rows;
  const q = req.quickFilter.map((x) => x.toLowerCase());
  if (q.length > 0) list = list.filter((r) => q.every((x) => `${r.name} ${r.mode} ${r.members}`.toLowerCase().includes(x)));
  for (const f of req.filter) {
    const needle = String(f.value ?? '').toLowerCase();
    list = list.filter((r) => String(r[f.field as keyof BondRow] ?? '').toLowerCase().includes(needle));
  }
  if (req.sort.length > 0) {
    list = [...list].sort((a, b) => {
      for (const s of req.sort) {
        const c = cmp(a[s.field as keyof BondRow], b[s.field as keyof BondRow]);
        if (c !== 0) return s.dir === 'asc' ? c : -c;
      }
      return 0;
    });
  }
  const start = req.page * req.pageSize;
  return { rows: list.slice(start, start + req.pageSize), total: list.length };
}

const LTR = { dir: 'ltr', inputMode: 'numeric' } as const;
const DEFAULT_MODE: BondMode = 'lacp';

export function BondsPage() {
  const { t } = useTranslation(['bonding', 'interfaces']);
  const perms = usePermissions();
  const qc = useQueryClient();
  const candidate = useCandidateInterfaces();
  const patch = usePatchInterfaces();
  const [selected, setSelected] = useState<string | null>(null);
  const [adding, setAdding] = useState<{ id: string; mode: string } | null>(null);
  const [lastError, setLastError] = useState<unknown>(null);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const data = await qc.fetchQuery({ queryKey: bondKeys.state, queryFn: () => fetchBondsState(signal), staleTime: 1000 });
      return pageOfBonds(data.items.map(toBondRow), req);
    },
    [qc],
  );

  const columns = useMemo<GridColDef<BondRow>[]>(
    () => [
      {
        field: 'name',
        headerName: t('col.name'),
        minWidth: 170,
        flex: 1,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
              {p.row.name}
            </Box>
            {p.row.item.hasPendingChange && <Chip size="small" color="warning" variant="outlined" label={t('pending')} />}
          </Stack>
        ),
      },
      { field: 'mode', headerName: t('col.mode'), width: 130, valueFormatter: (v: string) => (v ? t(`mode.${v}`, { defaultValue: v }) : '') },
      { field: 'loadBalance', headerName: t('col.loadBalance'), width: 130 },
      {
        field: 'status',
        headerName: t('col.status'),
        width: 130,
        renderCell: (p) =>
          p.row.item.state ? (
            <StatusChip size="small" status={bondStatus(p.row.item.state)!} label={t(`status.${bondStatus(p.row.item.state)!}`)} />
          ) : (
            <Typography variant="body2" color="text.secondary">
              {t('notInVpp')}
            </Typography>
          ),
      },
      { field: 'active', headerName: t('col.active'), width: 110 },
      {
        field: 'members',
        headerName: t('col.members'),
        minWidth: 260,
        flex: 2,
        sortable: false,
        renderCell: (p) => {
          const s = p.row.item.state;
          if (!s) {
            return (
              <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, fontSize: 12 }}>
                {p.row.members}
              </Box>
            );
          }
          return (
            <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%', overflow: 'hidden' }}>
              {s.members.map((m) => (
                <StatusChip
                  key={m.interface}
                  size="small"
                  status={memberStatus(m)}
                  label={m.lacp ? t('memberLacp', { name: m.interface, state: t(`lacp.mux.${m.lacp.muxState}`, { defaultValue: m.lacp.muxState }) }) : m.interface}
                />
              ))}
            </Stack>
          );
        },
      },
    ],
    [t],
  );

  const taken = [...Object.keys(candidate.data ?? {})];
  const openAdd = () => {
    const name = nextBondName(taken);
    setAdding({ id: String(bondIdOf(name) ?? 0), mode: DEFAULT_MODE });
  };
  const idValid = adding !== null && /^(0|[1-9][0-9]{0,9})$/.test(adding.id) && bondIdOf(`BondEthernet${adding.id}`) !== undefined;
  const newName = adding ? `BondEthernet${adding.id}` : '';
  const exists = adding !== null && taken.includes(newName);

  const addBond = async () => {
    if (!adding) return;
    try {
      setLastError(null);
      await patch.mutateAsync({ [newName]: { enabled: true, bond: { mode: adding.mode, members: {} } } });
      setAdding(null);
      setSelected(newName);
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
            <Button variant="contained" startIcon={<AddIcon />} disabled={!perms.editConfig || !candidate.isSuccess} onClick={openAdd}>
              {t('add')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {lastError !== null && <ProblemAlert error={lastError} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 420 }}>
        <ServerDataGrid<BondRow>
          aria-label={t('title')}
          columns={columns}
          queryKey={['state', 'interfaces', 'bonds', 'grid']}
          fetchPage={fetchPage}
          refetchInterval={BONDS_POLL_MS}
          initialPageSize={25}
          onRowClick={(p) => setSelected(p.row.name)}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <BondDrawer name={selected} onClose={() => setSelected(null)} />
      <Dialog open={adding !== null} onClose={() => setAdding(null)} fullWidth maxWidth="xs">
        <DialogTitle>{t('addTitle')}</DialogTitle>
        <DialogContent>
          <DialogContentText sx={{ mb: 2 }}>{t('addHelp')}</DialogContentText>
          <Stack gap={2}>
            <TextField
              autoFocus
              fullWidth
              label={t('bondId')}
              value={adding?.id ?? ''}
              onChange={(e) => setAdding((a) => (a ? { ...a, id: e.target.value.trim() } : a))}
              error={!idValid || exists}
              helperText={!idValid ? t('badId') : exists ? t('exists', { name: newName }) : t('willBe', { name: newName })}
              slotProps={{ htmlInput: LTR }}
            />
            <TextField select fullWidth label={t('field.mode.title')} value={adding?.mode ?? DEFAULT_MODE} onChange={(e) => setAdding((a) => (a ? { ...a, mode: e.target.value } : a))}>
              {BondMode.options.map((m) => (
                <MenuItem key={m} value={m}>
                  {t(`mode.${m}`)}
                </MenuItem>
              ))}
            </TextField>
          </Stack>
          {patch.isError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {t('addFailed')}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setAdding(null)}>{t('cancel')}</Button>
          <Button variant="contained" disabled={!idValid || exists || patch.isPending} onClick={() => void addBond()}>
            {t('add')}
          </Button>
        </DialogActions>
      </Dialog>
    </PageHeader>
  );
}
