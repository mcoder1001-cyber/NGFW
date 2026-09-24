import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { createMergePatch } from '../model';
import { BridgeDomainDrawer } from './BridgeDomainDrawer';
import {
  END_CELL,
  formSchemas,
  L2_TABLES,
  l2Patch,
  presence,
  NAME_RE,
  portsOf,
  rangesText,
  type BridgeDomainItem,
  type BridgeL2Config,
} from './model';
import {
  BRIDGE_POLL_MS,
  bridgeKeys,
  fetchBridgeDomains,
  useBridgeDomains,
  useCandidateL2,
  useCandidatePorts,
  usePatchRouting,
} from './queries';
import { RecordDialog } from './RecordDialog';

const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');
const MONO = { fontFamily: 'monospace' } as const;
const LIVE = 'live';

/** Grid row of the bridge-domain list: one record of `/state/l2/bridge-domains`. */
export interface DomainRow {
  id: string;
  name: string;
  bdId: number | null;
  item: BridgeDomainItem;
  status: string;
  members: number;
  bvi: string;
  learned: number | null;
  macAge: number | null;
}

export function toDomainRow(item: BridgeDomainItem): DomainRow {
  const run = (item.running ?? {}) as { macAgeMin?: number };
  return {
    id: item.name,
    name: item.name,
    bdId: item.id,
    item,
    status: item.state ? 'live' : 'missing',
    members: item.state?.members.length ?? 0,
    bvi: item.state?.bvi ?? '',
    learned: item.state ? item.state.learnedMacs : null,
    macAge: item.state?.macAgeMin ?? run.macAgeMin ?? null,
  };
}

/** Client-side page of the (small) bridge-domain list; the API returns it whole. */
export function pageOfDomains(
  rows: DomainRow[],
  req: ServerPageRequest,
): { rows: DomainRow[]; total: number } {
  const q = req.quickFilter.map((x) => x.toLowerCase());
  let list = q.length
    ? rows.filter((r) =>
        q.every((x) => `${r.name} ${r.bdId ?? ''} ${r.bvi}`.toLowerCase().includes(x)),
      )
    : rows;
  const s = req.sort[0];
  if (s) {
    list = [...list].sort((a, b) => {
      const c = String(a[s.field as keyof DomainRow] ?? '').localeCompare(
        String(b[s.field as keyof DomainRow] ?? ''),
        undefined,
        { numeric: true },
      );
      return s.dir === 'asc' ? c : -c;
    });
  }
  const start = req.page * req.pageSize;
  return { rows: list.slice(start, start + req.pageSize), total: list.length };
}

/** "Bridging" (F-bridge-l2): bridge domains, cross-connects and the time-range MAC filter. */
export function BridgingPage() {
  const { t } = useTranslation('bridge-l2');
  const [tab, setTab] = useState(0);
  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      <Tabs value={tab} onChange={(_e, v: number) => setTab(v)} sx={{ mb: 2 }}>
        <Tab label={t('tab.domains')} />
        <Tab label={t('tab.xconnects')} />
        <Tab label={t('tab.macFilter')} />
      </Tabs>
      {tab === 0 && <DomainsTab />}
      {tab === 1 && <XconnectsTab />}
      {tab === 2 && <MacFilterTab />}
    </PageHeader>
  );
}

function DomainsTab() {
  const { t } = useTranslation('bridge-l2');
  const perms = usePermissions();
  const qc = useQueryClient();
  const state = useBridgeDomains();
  const l2 = useCandidateL2();
  const patch = usePatchRouting();
  const [selected, setSelected] = useState<string | null>(null);
  const [adding, setAdding] = useState(false);
  const schema = useMemo(() => formSchemas.domain(), []);

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      const data = await qc.fetchQuery({
        queryKey: bridgeKeys.state,
        queryFn: () => fetchBridgeDomains(signal),
        staleTime: 1000,
      });
      return pageOfDomains(data.items.map(toDomainRow), req);
    },
    [qc],
  );
  const columns = useMemo<GridColDef<DomainRow>[]>(
    () => [
      {
        field: 'name',
        headerName: t('col.name'),
        minWidth: 160,
        flex: 1,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <Box component="span" dir="ltr" sx={MONO}>
              {p.row.name}
            </Box>
            {p.row.item.hasPendingChange && (
              <Chip size="small" color="warning" variant="outlined" label={t('pending')} />
            )}
          </Stack>
        ),
      },
      { field: 'bdId', headerName: t('col.id'), width: 90 },
      {
        field: 'status',
        headerName: t('col.status'),
        width: 130,
        renderCell: (p) => (
          <StatusChip
            size="small"
            status={presence(p.row.status === LIVE)}
            label={t(`status.${p.row.status}`)}
          />
        ),
      },
      { field: 'members', headerName: t('col.members'), type: 'number', width: 100 },
      {
        field: 'bvi',
        headerName: t('col.bvi'),
        width: 130,
        renderCell: (p) => <span dir="ltr">{p.row.bvi}</span>,
      },
      { field: 'learned', headerName: t('col.learned'), type: 'number', width: 130 },
      {
        field: 'macAge',
        headerName: t('col.macAge'),
        width: 120,
        valueFormatter: (v: number | null) =>
          v === null ? '' : v === 0 ? t('macAgeOff') : t('macAgeValue', { min: v }),
      },
    ],
    [t],
  );
  const selectedItem = state.data?.items.find((i) => i.name === selected) ?? null;
  const unavailable = state.error instanceof ApiError && state.error.status === 501;

  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center">
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={!perms.editConfig}
              onClick={() => setAdding(true)}
            >
              {t('add')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {unavailable && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('unavailable')}
        </Alert>
      )}
      <Paper variant="outlined" sx={{ blockSize: 420 }}>
        <ServerDataGrid<DomainRow>
          aria-label={t('tab.domains')}
          columns={columns}
          queryKey={[...bridgeKeys.state, 'grid']}
          fetchPage={fetchPage}
          refetchInterval={BRIDGE_POLL_MS}
          initialPageSize={25}
          onRowClick={(p) => setSelected(p.row.name)}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <BridgeDomainDrawer item={selectedItem} onClose={() => setSelected(null)} />
      {adding && (
        <RecordDialog
          open
          title={t('domain.addTitle')}
          help={t('domain.addHelp')}
          keyLabel={t('col.name')}
          keyRe={NAME_RE}
          takenKeys={Object.keys(l2.data?.bridgeDomains ?? {})}
          schema={schema}
          value={undefined}
          error={patch.error}
          pending={patch.isPending}
          readOnly={!perms.editConfig}
          pointerOf={(k) => `/routing/l2/bridgeDomains/${esc(k)}`}
          onClose={() => setAdding(false)}
          onSubmit={(k, v) =>
            void patch
              .mutateAsync(l2Patch(L2_TABLES.bridgeDomains, k, v))
              .then(() => {
                setAdding(false);
                setSelected(k);
              })
              .catch(() => undefined)
          }
        />
      )}
    </>
  );
}

type Table = 'xconnects' | 'l3xc' | 'macFilters';
type Editing = { table: Table; key: string | null } | null;

/** The L2 / L3 cross-connect list (routing.l2.xconnects / l3xc) with schema-driven add / edit. */
function XconnectsTab() {
  const { t } = useTranslation('bridge-l2');
  const perms = usePermissions();
  const l2 = useCandidateL2();
  const ifs = useCandidatePorts();
  const patch = usePatchRouting();
  const [editing, setEditing] = useState<Editing>(null);
  const cfg: Partial<BridgeL2Config> = l2.data ?? {};
  const names = portsOf(ifs.data).map((p) => p.name);
  const rows = [
    ...Object.entries(cfg.xconnects ?? {}).map(([rx, x]) => ({
      table: L2_TABLES.xconnects,
      rx,
      text: x.tx,
    })),
    ...Object.entries(cfg.l3xc ?? {}).map(([rx, x]) => ({
      table: L2_TABLES.l3xc,
      rx,
      text: [...x.ipv4Paths, ...x.ipv6Paths]
        .map((p) => [p.nextHop, p.interface].filter(Boolean).join(' via ') || `vrf ${p.vrf}`)
        .join(', '),
    })),
  ].sort((a, b) => a.rx.localeCompare(b.rx, undefined, { numeric: true }));
  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Button
          variant="contained"
          startIcon={<AddIcon />}
          disabled={!perms.editConfig}
          onClick={() => setEditing({ table: L2_TABLES.xconnects, key: null })}
        >
          {t('xc.addL2')}
        </Button>
        <Button
          variant="outlined"
          startIcon={<AddIcon />}
          disabled={!perms.editConfig}
          onClick={() => setEditing({ table: L2_TABLES.l3xc, key: null })}
        >
          {t('xc.addL3')}
        </Button>
      </Stack>
      {patch.isError && editing === null && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {rows.length === 0 ? (
        <Typography color="text.secondary">{t('xc.none')}</Typography>
      ) : (
        <Paper variant="outlined">
          <Table size="small" aria-label={t('tab.xconnects')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.kind')}</TableCell>
                <TableCell>{t('col.rx')}</TableCell>
                <TableCell>{t('col.tx')}</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {rows.map((r) => (
                <TableRow key={`${r.table}:${r.rx}`}>
                  <TableCell>{r.table === 'xconnects' ? t('xc.l2') : t('xc.l3')}</TableCell>
                  <TableCell dir="ltr" sx={MONO}>
                    {r.rx}
                  </TableCell>
                  <TableCell dir="ltr" sx={MONO}>
                    {r.text}
                  </TableCell>
                  <TableCell sx={END_CELL}>
                    <IconButton
                      size="small"
                      aria-label={`${t('edit')} ${r.rx}`}
                      disabled={!perms.editConfig}
                      onClick={() => setEditing({ table: r.table, key: r.rx })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={`${t('remove')} ${r.rx}`}
                      disabled={!perms.editConfig || patch.isPending}
                      onClick={() =>
                        void patch.mutateAsync(l2Patch(r.table, r.rx, null)).catch(() => undefined)
                      }
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
      {editing !== null && editing.table !== 'macFilters' && (
        <EditRecord
          editing={editing}
          cfg={cfg}
          keyOptions={names}
          onClose={() => setEditing(null)}
        />
      )}
    </>
  );
}

/** The time-range MAC filter devices (routing.l2.macFilters) and where the filter runs. */
function MacFilterTab() {
  const { t } = useTranslation('bridge-l2');
  const perms = usePermissions();
  const l2 = useCandidateL2();
  const ifs = useCandidatePorts();
  const patch = usePatchRouting();
  const [editing, setEditing] = useState<Editing>(null);
  const cfg: Partial<BridgeL2Config> = l2.data ?? {};
  const on = portsOf(ifs.data)
    .filter((p) => p.l2?.macFilter)
    .map((p) => p.name);
  const devices = Object.entries(cfg.macFilters ?? {}).sort(([a], [b]) => a.localeCompare(b));
  return (
    <>
      <Typography color="text.secondary" sx={{ mb: 1 }}>
        {t('mf.intro')}
      </Typography>
      <Typography variant="body2" sx={{ mb: 1 }} dir="auto">
        {on.length ? t('mf.enabledOn', { list: on.join(', ') }) : t('mf.enabledNone')}
      </Typography>
      <Button
        variant="contained"
        startIcon={<AddIcon />}
        sx={{ mb: 1 }}
        disabled={!perms.editConfig}
        onClick={() => setEditing({ table: L2_TABLES.macFilters, key: null })}
      >
        {t('mf.add')}
      </Button>
      {patch.isError && editing === null && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
      {devices.length === 0 ? (
        <Typography color="text.secondary">{t('mf.none')}</Typography>
      ) : (
        <Paper variant="outlined">
          <Table size="small" aria-label={t('tab.macFilter')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.device')}</TableCell>
                <TableCell>{t('col.mac')}</TableCell>
                <TableCell>{t('col.action')}</TableCell>
                <TableCell>{t('col.ranges')}</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {devices.map(([name, f]) => (
                <TableRow key={name}>
                  <TableCell dir="ltr">{name}</TableCell>
                  <TableCell dir="ltr" sx={MONO}>
                    {f.mac}
                  </TableCell>
                  <TableCell>{f.action}</TableCell>
                  <TableCell dir="ltr">{rangesText(f, t('mf.always'))}</TableCell>
                  <TableCell sx={END_CELL}>
                    <IconButton
                      size="small"
                      aria-label={`${t('edit')} ${name}`}
                      disabled={!perms.editConfig}
                      onClick={() => setEditing({ table: L2_TABLES.macFilters, key: name })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={`${t('remove')} ${name}`}
                      disabled={!perms.editConfig || patch.isPending}
                      onClick={() =>
                        void patch
                          .mutateAsync(l2Patch(L2_TABLES.macFilters, name, null))
                          .catch(() => undefined)
                      }
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
      {editing !== null && editing.table === 'macFilters' && (
        <EditRecord editing={editing} cfg={cfg} onClose={() => setEditing(null)} />
      )}
    </>
  );
}

/** Add / edit dialog of one record of routing.l2.{xconnects,l3xc,macFilters}. */
function EditRecord({
  editing,
  cfg,
  keyOptions,
  onClose,
}: {
  editing: NonNullable<Editing>;
  cfg: Partial<BridgeL2Config>;
  keyOptions?: string[];
  onClose: () => void;
}) {
  const { t } = useTranslation('bridge-l2');
  const perms = usePermissions();
  const patch = usePatchRouting();
  const table = editing.table;
  const schema = useMemo(
    () =>
      table === 'xconnects'
        ? formSchemas.xconnect()
        : table === 'l3xc'
          ? formSchemas.l3xc()
          : formSchemas.macFilter(),
    [table],
  );
  const records = (cfg[table] ?? {}) as Record<string, unknown>;
  const current = editing.key !== null ? records[editing.key] : undefined;
  const title =
    table === 'xconnects' ? t('xc.addL2') : table === 'l3xc' ? t('xc.addL3') : t('mf.add');
  return (
    <RecordDialog
      open
      title={editing.key !== null ? `${title}: ${editing.key}` : title}
      {...(table !== 'macFilters' ? { help: t('xc.rxHelp') } : {})}
      keyLabel={table === 'macFilters' ? t('col.device') : t('col.rx')}
      {...(table === 'macFilters'
        ? { keyRe: /^[A-Za-z0-9][A-Za-z0-9_.-]{0,47}$/ }
        : { keyOptions: keyOptions ?? [] })}
      {...(editing.key !== null ? { fixedKey: editing.key } : {})}
      takenKeys={Object.keys(records)}
      schema={schema}
      value={current}
      error={patch.error}
      pending={patch.isPending}
      readOnly={!perms.editConfig}
      pointerOf={(k) => `/routing/l2/${table}/${esc(k)}`}
      onClose={onClose}
      onSubmit={(k, v) =>
        void patch
          .mutateAsync(l2Patch(table, k, current === undefined ? v : createMergePatch(current, v)))
          .then(onClose)
          .catch(() => undefined)
      }
    />
  );
}
