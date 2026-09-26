import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useQueries } from '@tanstack/react-query';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { fetchRoutes, FIB_ON_DEMAND, FIB_STATUS_POLL_MS, routeKeys, useCandidate, usePatchConfig } from './api';
import { Mono, pageOf, problemFor } from './common';
import {
  localizeSchema,
  NS,
  prefixKey,
  routeRows,
  routeStatus,
  staticRouteSchema,
  type RouteRow,
  type RouteStatus,
  type StaticRouteConfig,
  type VrfsConfig,
} from './model';

const STATUS_CHIP: Record<RouteStatus, 'up' | 'down' | 'adminDown' | 'degraded'> = {
  installed: 'up',
  missing: 'down',
  frr: 'adminDown',
  unknown: 'degraded',
};

/**
 * `routing.static` as a grid (weighted ECMP next hops, blackhole, next-hop VRF, viaFrr) with a live status per route (is
 * the API-source entry in its VRF's FIB?) and an editor whose next-hop list is the ECMP path editor (one row per path,
 * with its weight). The whole list is written back with one PUT of `routing/static` (the generic pointer route).
 */
export function StaticRoutesTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const routing = useCandidate<{ static?: StaticRouteConfig[] }>('routing');
  const routes = { ...routing, data: routing.data?.static };
  const vrfs = useCandidate<VrfsConfig>('vrfs');
  const ifs = useCandidate<Record<string, { subinterfaces?: Record<string, unknown> }>>('interfaces');
  const put = usePatchConfig();
  const [edit, setEdit] = useState<{ index: number; value: StaticRouteConfig | undefined } | null>(null);
  // index of the edited route in the list as sent (the list is written in canonical order, so pointers follow it)
  const [sentIndex, setSentIndex] = useState<number | null>(null);
  const schema = useMemo(() => localizeSchema(staticRouteSchema(), (k, o) => t(k, o ?? {})), [t]);
  const rows = useMemo(() => routeRows(routes.data), [routes.data]);
  const ifNames = useMemo(
    () => Object.entries(ifs.data ?? {}).flatMap(([n, i]) => [n, ...Object.keys(i.subinterfaces ?? {}).map((s) => `${n}.${s}`)]),
    [ifs.data],
  );

  // live status: the entries whose best FIB source is API, per VRF a route uses (one agent page of ≤ 1000 per VRF; each
  // read is a full table walk in VPP, so once a minute — review H1). Beyond the first 1000 the status is "unknown" (L2).
  const vrfNames = useMemo(() => [...new Set(rows.map((r) => r.vrf))].sort(), [rows]);
  const live = useQueries({
    queries: vrfNames.map((vrf) => ({
      queryKey: [...routeKeys.all, { vrf, source: 'API', page: 1, pageSize: 1000 }],
      queryFn: ({ signal }: { signal: AbortSignal }) => fetchRoutes({ vrf, source: 'API', page: 1, pageSize: 1000 }, signal),
      ...FIB_ON_DEMAND,
      refetchInterval: FIB_STATUS_POLL_MS,
    })),
  });
  const [installed, partial] = useMemo(() => {
    const m = new Map<string, Set<string>>();
    const cut = new Set<string>();
    vrfNames.forEach((vrf, i) => {
      const d = live[i]?.data;
      if (!d) return;
      m.set(vrf, new Set(d.items.map((x) => prefixKey(x.prefix))));
      if (d.total > d.items.length) cut.add(vrf);
    });
    return [m, cut] as const;
  }, [vrfNames, live]);

  const fetchPage = useCallback(
    async (req: ServerPageRequest) => pageOf(rows, req, (r) => `${r.vrf} ${r.prefix} ${r.paths} ${r.description}`),
    [rows],
  );

  const columns = useMemo<GridColDef<RouteRow>[]>(
    () => [
      { field: 'vrf', headerName: t('routes.col.vrf'), width: 110 },
      { field: 'prefix', headerName: t('routes.col.prefix'), minWidth: 170, flex: 1, renderCell: (p) => <Mono>{p.row.prefix}</Mono> },
      {
        field: 'paths',
        headerName: t('routes.col.paths'),
        minWidth: 260,
        flex: 2,
        renderCell: (p) =>
          p.row.paths === '' ? (
            <Chip size="small" label={t('routes.blackhole')} />
          ) : (
            <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
              {p.row.ecmp && <Chip size="small" color="info" variant="outlined" label={t('routes.ecmp')} />}
              <Mono>{p.row.paths}</Mono>
            </Stack>
          ),
      },
      { field: 'distance', headerName: t('routes.col.distance'), type: 'number', width: 100 },
      {
        field: 'status',
        headerName: t('routes.col.status'),
        description: t('routes.statusHelp'),
        sortable: false,
        filterable: false,
        width: 150,
        renderCell: (p) => {
          const st = routeStatus(p.row, installed, partial);
          return <StatusChip size="small" status={STATUS_CHIP[st]} label={t(`routes.status.${st}`)} />;
        },
      },
      { field: 'description', headerName: t('routes.col.description'), minWidth: 140, flex: 1 },
    ],
    [t, installed, partial],
  );

  /** Writes the whole list in the agent's canonical order (VRF, then prefix) so Retrieve == running (no drift). */
  const writeAll = async (list: StaticRouteConfig[], edited?: StaticRouteConfig) => {
    const sorted = [...list].sort((a, b) => a.vrf.localeCompare(b.vrf) || a.prefix.toLowerCase().localeCompare(b.prefix.toLowerCase()));
    setSentIndex(edited ? sorted.indexOf(edited) : null);
    await put.mutateAsync({ key: 'routing', patch: { static: sorted } });
  };

  const save = async (value: unknown) => {
    if (!edit) return;
    const list = [...(routes.data ?? [])];
    const v = value as StaticRouteConfig;
    if (edit.index < 0) list.push(v);
    else list[edit.index] = v;
    try {
      await writeAll(list, v);
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };

  const remove = async (index: number) => {
    const list = (routes.data ?? []).filter((_, i) => i !== index);
    try {
      await writeAll(list);
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };

  const prefix = `/routing/static/${sentIndex ?? edit?.index ?? 0}`;
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('routes.intro')}
      </Typography>
      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button variant="contained" startIcon={<AddIcon />} disabled={!perms.editConfig} onClick={() => setEdit({ index: -1, value: undefined })}>
              {t('routes.add')}
            </Button>
          </span>
        </Tooltip>
      </Stack>
      {routes.isError && <ProblemAlert error={routes.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 460 }}>
        <ServerDataGrid<RouteRow>
          aria-label={t('routes.title')}
          columns={columns}
          queryKey={['config', 'candidate', 'routing', 'static', 'grid', routing.dataUpdatedAt]}
          fetchPage={fetchPage}
          initialPageSize={25}
          onRowClick={(p) => setEdit({ index: p.row.index, value: routes.data?.[p.row.index] })}
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>
      <Dialog open={edit !== null} onClose={() => setEdit(null)} fullWidth maxWidth="md">
        <DialogTitle>{edit?.value ? t('routes.editTitle', { prefix: edit.value.prefix, vrf: edit.value.vrf }) : t('routes.addTitle')}</DialogTitle>
        <DialogContent>
          {edit && (
            <Box sx={{ pt: 1 }}>
              <Alert severity="info" sx={{ mb: 2 }}>
                {t('routes.editorHelp', { vrfs: ['default', ...Object.keys(vrfs.data ?? {}).filter((v) => v !== 'default')].join(', ') })}
              </Alert>
              {put.isError && <ProblemAlert error={put.error} sx={{ mb: 1 }} />}
              <SchemaForm
                key={String(edit.index)}
                schema={schema}
                value={edit.value}
                readOnly={!perms.editConfig}
                interfaceOptions={ifNames}
                submitLabel={t('save')}
                resetLabel={t('reset')}
                problem={problemFor(put.error, prefix)}
                onSubmit={save}
              >
                {edit.index >= 0 && (
                  <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={!perms.editConfig || put.isPending} onClick={() => void remove(edit.index)}>
                    {t('remove')}
                  </Button>
                )}
              </SchemaForm>
            </Box>
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
}
