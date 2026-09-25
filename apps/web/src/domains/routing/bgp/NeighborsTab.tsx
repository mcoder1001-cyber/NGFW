import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import FormControlLabel from '@mui/material/FormControlLabel';
import FormGroup from '@mui/material/FormGroup';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import type { GridColDef } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { localizeSchema } from '../vrf-static-ecmp/model';
import { Mono, problemFor } from '../vrf-static-ecmp/common';
import { useBgpState, useCandidate, usePatch, useRoutingEvents } from './api';
import {
  bgpGlobalSchema,
  bgpRecordItemSchema,
  formatUptime,
  isAddress,
  neighborRows,
  NS,
  OBJECT_NAME,
  REDISTRIBUTE_SOURCES,
  stateChip,
  toggleRedistribute,
  type NeighborRow,
  type RoutingDoc,
} from './model';
import { RecordEditor } from './RecordEditor';

const BGP_POINTER = '/routing/bgp';
const NEIGHBORS = { k: 'neighbors', path: ['bgp', 'neighbors'] } as const;
const PEER_GROUPS = { k: 'peerGroups', path: ['bgp', 'peerGroups'] } as const;

/**
 * BGP: the global settings (AS, router id, networks, redistribution toggles, graceful restart, RFC 8212), the neighbours
 * with their live session (state, uptime, prefixes received/sent, flaps — `GET /state/bgp`, refreshed on `routing.events`)
 * and the peer groups. Changes go into the candidate; commit them from the bar at the bottom.
 */
export function NeighborsTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const routing = useCandidate<RoutingDoc>('routing');
  const ifs = useCandidate<Record<string, { lcp?: unknown }>>('interfaces');
  const live = useBgpState();
  useRoutingEvents();
  const patch = usePatch();
  const bgp = routing.data?.bgp;
  const tr = (k: string, o?: Record<string, unknown>) => t(k, o ?? {});
  const globalSchema = useMemo(() => localizeSchema(bgpGlobalSchema(), tr), [t]);
  const neighborSchema = useMemo(() => localizeSchema(bgpRecordItemSchema('neighbors'), tr), [t]);
  const groupSchema = useMemo(() => localizeSchema(bgpRecordItemSchema('peerGroups'), tr), [t]);
  const liveNeighbors = useMemo(
    () => (live.data?.instances ?? []).flatMap((i) => i.neighbors),
    [live.data],
  );
  const rows = useMemo(() => neighborRows(bgp, liveNeighbors), [bgp, liveNeighbors]);
  const ifNames = useMemo(() => Object.keys(ifs.data ?? {}), [ifs.data]);

  const columns = useMemo<GridColDef<NeighborRow>[]>(
    () => [
      {
        field: 'address',
        headerName: t('neighbors.col.address'),
        minWidth: 150,
        flex: 1,
        renderCell: (p) => <Mono>{p.row.address}</Mono>,
      },
      { field: 'remoteAs', headerName: t('neighbors.col.remoteAs'), width: 110 },
      { field: 'peerGroup', headerName: t('neighbors.col.peerGroup'), width: 120 },
      { field: 'families', headerName: t('neighbors.col.families'), width: 110 },
      {
        field: 'state',
        headerName: t('neighbors.col.state'),
        width: 150,
        renderCell: (p) => (
          <StatusChip
            size="small"
            status={stateChip(p.row.state, p.row.shutdown)}
            label={p.row.shutdown ? t('neighbors.shutdown') : p.row.state || t('neighbors.unknown')}
          />
        ),
      },
      {
        field: 'uptimeSec',
        headerName: t('neighbors.col.uptime'),
        width: 120,
        renderCell: (p) => <Mono>{formatUptime(p.row.uptimeSec)}</Mono>,
      },
      { field: 'prefixesReceived', headerName: t('neighbors.col.rx'), type: 'number', width: 100 },
      { field: 'prefixesSent', headerName: t('neighbors.col.tx'), type: 'number', width: 100 },
      { field: 'flaps', headerName: t('neighbors.col.flaps'), type: 'number', width: 80 },
      { field: 'description', headerName: t('neighbors.col.description'), minWidth: 140, flex: 1 },
    ],
    [t],
  );

  const groupRows = useMemo(
    () =>
      Object.entries(bgp?.peerGroups ?? {}).map(([name, g]) => ({
        id: name,
        name,
        remoteAs: String(g.remoteAs ?? ''),
        description: g.description ?? '',
        members: Object.values(bgp?.neighbors ?? {}).filter((n) => n.peerGroup === name).length,
      })),
    [bgp],
  );
  type GroupRow = (typeof groupRows)[number];
  const groupColumns = useMemo<GridColDef<GroupRow>[]>(
    () => [
      { field: 'name', headerName: t('peerGroups.col.name'), flex: 1 },
      { field: 'remoteAs', headerName: t('peerGroups.col.remoteAs'), width: 120 },
      { field: 'members', headerName: t('peerGroups.col.members'), type: 'number', width: 110 },
      { field: 'description', headerName: t('peerGroups.col.description'), flex: 1 },
    ],
    [t],
  );

  const saveGlobal = async (v: unknown) => {
    try {
      await patch.mutateAsync({ key: 'routing', patch: { bgp: v as Record<string, unknown> } });
    } catch {
      // shown above the form
    }
  };
  const removeBgp = () =>
    void patch.mutateAsync({ key: 'routing', patch: { bgp: null } }).catch(() => undefined);
  const setRedistribute = (source: (typeof REDISTRIBUTE_SOURCES)[number], on: boolean) =>
    void patch
      .mutateAsync({
        key: 'routing',
        patch: {
          bgp: {
            redistribute: toggleRedistribute(
              bgp?.redistribute as Record<string, unknown>,
              source,
              on,
            ),
          },
        },
      })
      .catch(() => undefined);

  return (
    <Box>
      <Stack direction="row" gap={2} alignItems="center" sx={{ mb: 2 }}>
        <Typography color="text.secondary" sx={{ flex: 1 }}>
          {t('intro')}
        </Typography>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void live.refetch()}
          disabled={live.isFetching}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {live.data && !live.data.frrRunning && bgp && (
        <Alert severity="warning" sx={{ mb: 2 }}>
          {t('frrDown', { error: live.data.error ?? '' })}
        </Alert>
      )}
      {live.data?.frrRunning && (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('frrVersion', { version: live.data.frrVersion ?? '' })}
        </Typography>
      )}
      {live.isError && <ProblemAlert error={live.error} sx={{ mb: 2 }} />}
      <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 1 }}>
          {t('global.title')}
        </Typography>
        {!bgp && (
          <Alert severity="info" sx={{ mb: 2 }}>
            {t('global.absent')}
          </Alert>
        )}
        {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
        <SchemaForm
          key={routing.dataUpdatedAt}
          schema={globalSchema}
          value={bgp ? { ...bgp, neighbors: undefined, peerGroups: undefined } : undefined}
          readOnly={!perms.editConfig}
          submitLabel={t('save')}
          resetLabel={t('reset')}
          problem={problemFor(patch.error, BGP_POINTER)}
          onSubmit={saveGlobal}
        >
          {bgp && (
            <Button
              color="error"
              variant="outlined"
              disabled={!perms.editConfig || patch.isPending}
              onClick={removeBgp}
            >
              {t('global.remove')}
            </Button>
          )}
        </SchemaForm>
        {bgp && (
          <Box sx={{ mt: 2 }}>
            <Typography variant="subtitle1">{t('global.redistribute')}</Typography>
            <FormGroup row>
              {REDISTRIBUTE_SOURCES.map((s) => (
                <FormControlLabel
                  key={s}
                  control={
                    <Switch
                      checked={
                        (bgp.redistribute as Record<string, unknown> | undefined)?.[s] !== undefined
                      }
                      disabled={!perms.editConfig || patch.isPending}
                      onChange={(_, on) => setRedistribute(s, on)}
                    />
                  }
                  label={t(`global.source.${s}`)}
                />
              ))}
            </FormGroup>
          </Box>
        )}
      </Paper>
      {bgp && (
        <>
          <RecordEditor<NeighborRow>
            {...NEIGHBORS}
            record={bgp.neighbors as Record<string, unknown>}
            schema={neighborSchema}
            rows={rows}
            columns={columns}
            checkKey={(k) =>
              isAddress(k)
                ? bgp.neighbors?.[k]
                  ? t('neighbors.exists')
                  : ''
                : t('neighbors.badKey')
            }
            interfaceOptions={ifNames}
            version={routing.dataUpdatedAt + live.dataUpdatedAt}
          />
          <RecordEditor<GroupRow>
            {...PEER_GROUPS}
            record={bgp.peerGroups as Record<string, unknown>}
            schema={groupSchema}
            rows={groupRows}
            columns={groupColumns}
            checkKey={(k) =>
              OBJECT_NAME.test(k)
                ? bgp.peerGroups?.[k]
                  ? t('peerGroups.exists')
                  : ''
                : t('badName')
            }
            interfaceOptions={ifNames}
            version={routing.dataUpdatedAt}
          />
        </>
      )}
    </Box>
  );
}
