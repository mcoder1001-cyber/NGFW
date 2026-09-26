import DeleteSweepIcon from '@mui/icons-material/DeleteSweep';
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
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { UI_KIT_NS, useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { StaticNeighborsForm } from './StaticNeighborsForm';
import {
  fetchNeighbors,
  neighborKeys,
  NEIGHBORS_POLL_MS,
  toQuery,
  useArpFlush,
  useInterfaceNames,
  type ArpFlushResult,
  type Neighbor,
  type NeighborFilters,
} from './queries';

export const NS = 'neighbors-ra';
const LTR = { dir: 'ltr' } as const;
const FAMILIES = ['ipv4', 'ipv6'] as const;
const STATES = ['static', 'dynamic'] as const;
const TABLE_TAB = 'table';
/** A select whose "" option means all/both: show that option's label instead of an empty field. */
const SHOW_EMPTY = { select: { displayEmpty: true }, inputLabel: { shrink: true } } as const;
const STATIC_TAB = 'static';
type Family = (typeof FAMILIES)[number];

/** Grid row: one neighbour; `id` is unique across interfaces and families. */
export interface Row extends Neighbor {
  id: string;
}

export function toRow(n: Neighbor): Row {
  return { ...n, id: `${n.interface}|${n.ip}` };
}

/** Live ARP/ND table: server-side paged (the agent pages), polled, invalidated by `neighbor.events`. */
function NeighborTable() {
  const { t } = useTranslation([NS, UI_KIT_NS]);
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const names = useInterfaceNames();
  const [filters, setFilters] = useState<NeighborFilters>({});
  const [flushOpen, setFlushOpen] = useState(false);
  const [flushed, setFlushed] = useState<ArpFlushResult | null>(null);
  const invalidate = useCallback(
    () => void qc.invalidateQueries({ queryKey: neighborKeys.all }),
    [qc],
  );
  const { status: wsStatus } = useTopic<unknown>('neighbor.events', { onBatch: invalidate });

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const page = await fetchNeighbors(toQuery(req, filters), signal);
      return { rows: page.items.map(toRow), total: page.total };
    },
    [filters],
  );

  const columns = useMemo<GridColDef<Row>[]>(
    () => [
      {
        field: 'interface',
        headerName: t('col.interface'),
        minWidth: 150,
        flex: 1,
        renderCell: (p) => (
          <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
            {p.row.interface}
          </Box>
        ),
      },
      {
        field: 'ip',
        headerName: t('col.ip'),
        minWidth: 190,
        flex: 1,
        renderCell: (p) => (
          <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
            {p.row.ip}
          </Box>
        ),
      },
      {
        field: 'mac',
        headerName: t('col.mac'),
        width: 170,
        renderCell: (p) => (
          <Box component="span" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
            {p.row.mac}
          </Box>
        ),
      },
      {
        field: 'family',
        headerName: t('col.family'),
        width: 110,
        sortable: false,
        valueFormatter: (v: string) => t(`family.${v}`),
      },
      {
        field: 'state',
        headerName: t('col.state'),
        width: 170,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <Chip
              size="small"
              variant="outlined"
              color={p.row.state === 'static' ? 'primary' : 'default'}
              label={t(`state.${p.row.state}`)}
            />
            {p.row.noFibEntry && <Chip size="small" variant="outlined" label={t('noFib')} />}
          </Stack>
        ),
      },
      {
        field: 'ageSec',
        headerName: t('col.age'),
        type: 'number',
        width: 100,
        valueFormatter: (v: number, row: Row) =>
          row.state === 'static'
            ? '—'
            : t('ageValue', { value: fmt.number(v, { maximumFractionDigits: 1 }) }),
      },
      { field: 'vrf', headerName: t('col.vrf'), width: 110 },
    ],
    [t, fmt],
  );

  const select = (
    key: keyof NeighborFilters,
    label: string,
    options: readonly (readonly [string, string])[],
  ) => (
    <TextField
      select
      size="small"
      label={label}
      value={filters[key] ?? ''}
      onChange={(e) => setFilters((f) => ({ ...f, [key]: e.target.value || undefined }))}
      sx={{ minInlineSize: 150 }}
    >
      <MenuItem value="">{t('filter.any')}</MenuItem>
      {options.map(([v, l]) => (
        <MenuItem key={v} value={v}>
          {l}
        </MenuItem>
      ))}
    </TextField>
  );

  const interfaceSelect = select(
    'interface',
    t('filter.interface'),
    (names.data ?? []).map((n) => [n, n] as const),
  );
  const familySelect = select(
    'family',
    t('filter.family'),
    FAMILIES.map((f) => [f, t(`family.${f}`)] as const),
  );
  const stateSelect = select(
    'state',
    t('filter.state'),
    STATES.map((st) => [st, t(`state.${st}`)] as const),
  );

  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1, flexWrap: 'wrap' }} alignItems="center">
        {interfaceSelect}
        {familySelect}
        {stateSelect}
        <TextField
          size="small"
          label={t('filter.vrf')}
          value={filters.vrf ?? ''}
          onChange={(e) => setFilters((f) => ({ ...f, vrf: e.target.value.trim() || undefined }))}
          slotProps={{ htmlInput: LTR }}
          sx={{ inlineSize: 140 }}
        />
        <Box sx={{ flex: 1 }} />
        <Chip
          size="small"
          variant="outlined"
          label={t('liveChip', { status: t(`ws.${wsStatus}`, { ns: UI_KIT_NS }) })}
          color={wsStatus === 'open' ? 'success' : 'default'}
        />
        <Tooltip title={perms.editConfig ? '' : t('flush.readonly')}>
          <span>
            <Button
              variant="outlined"
              color="warning"
              startIcon={<DeleteSweepIcon />}
              disabled={!perms.editConfig}
              onClick={() => setFlushOpen(true)}
            >
              {t('flush.button')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {flushed && (
        <Alert severity="success" onClose={() => setFlushed(null)} sx={{ mb: 1 }}>
          {t('flush.done', {
            deleted: fmt.integer(flushed.deleted),
            interfaces: fmt.integer(flushed.interfaces),
          })}
        </Alert>
      )}
      <Paper variant="outlined" sx={{ blockSize: 560 }}>
        <ServerDataGrid<Row>
          aria-label={t('tabs.table')}
          columns={columns}
          queryKey={[...neighborKeys.all, 'grid', filters]}
          fetchPage={fetchPage}
          refetchInterval={NEIGHBORS_POLL_MS}
          initialPageSize={25}
          pageSizeOptions={[25, 50, 100, 500]}
          disableColumnFilter
          showToolbar
        />
      </Paper>
      <FlushDialog
        open={flushOpen}
        interfaces={names.data ?? []}
        initial={filters.interface}
        onClose={() => setFlushOpen(false)}
        onDone={(r) => {
          setFlushed(r);
          setFlushOpen(false);
        }}
      />
    </>
  );
}

/** Confirm dialog of the ARP flush (one interface or every configured interface, one or both families). */
function FlushDialog(props: {
  open: boolean;
  interfaces: readonly string[];
  initial: string | undefined;
  onClose: () => void;
  onDone: (r: ArpFlushResult) => void;
}) {
  const { t } = useTranslation(NS);
  const flush = useArpFlush();
  const [iface, setIface] = useState<string>(props.initial ?? '');
  const [family, setFamily] = useState<'' | Family>('');
  const run = async () => {
    try {
      const r = await flush.mutateAsync({
        ...(iface ? { interface: iface } : {}),
        ...(family ? { family } : {}),
      });
      flush.reset();
      props.onDone(r);
    } catch {
      // shown below
    }
  };
  return (
    <Dialog
      open={props.open}
      onClose={props.onClose}
      fullWidth
      maxWidth="xs"
      TransitionProps={{ onEnter: () => setIface(props.initial ?? '') }}
    >
      <DialogTitle>{t('flush.title')}</DialogTitle>
      <DialogContent>
        <DialogContentText sx={{ mb: 2 }}>{t('flush.text')}</DialogContentText>
        <Stack gap={2}>
          <TextField
            select
            label={t('flush.scope')}
            slotProps={SHOW_EMPTY}
            value={iface}
            onChange={(e) => setIface(e.target.value)}
          >
            <MenuItem value="">{t('flush.all')}</MenuItem>
            {props.interfaces.map((n) => (
              <MenuItem key={n} value={n} dir="ltr">
                {n}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            select
            label={t('flush.family')}
            slotProps={SHOW_EMPTY}
            value={family}
            onChange={(e) => setFamily(e.target.value as '' | Family)}
          >
            <MenuItem value="">{t('flush.both')}</MenuItem>
            {FAMILIES.map((f) => (
              <MenuItem key={f} value={f}>
                {t(`family.${f}`)}
              </MenuItem>
            ))}
          </TextField>
          {flush.error && <ProblemAlert error={flush.error} />}
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={props.onClose}>{t('flush.cancel')}</Button>
        <Button
          variant="contained"
          color="warning"
          disabled={flush.isPending}
          onClick={() => void run()}
        >
          {t('flush.confirm')}
        </Button>
      </DialogActions>
    </Dialog>
  );
}

/** Routing → Neighbours: the live table and the static entries / limits form. */
export function NeighborsPage() {
  const { t } = useTranslation(NS);
  const [params, setParams] = useSearchParams();
  const tab = params.get('tab') === STATIC_TAB ? STATIC_TAB : TABLE_TAB;
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <Tabs
        value={tab}
        onChange={(_, v: string) => setParams({ tab: v }, { replace: true })}
        aria-label={t('tabs.label')}
        sx={{ mb: 2 }}
      >
        {[TABLE_TAB, STATIC_TAB].map((v) => (
          <Tab
            key={v}
            value={v}
            label={t(`tabs.${v}`)}
            id={`neighbors-tab-${v}`}
            aria-controls={`neighbors-panel-${v}`}
          />
        ))}
      </Tabs>
      <Box role="tabpanel" id={`neighbors-panel-${tab}`} aria-labelledby={`neighbors-tab-${tab}`}>
        {tab === TABLE_TAB ? <NeighborTable /> : <StaticNeighborsForm />}
      </Box>
    </PageHeader>
  );
}
