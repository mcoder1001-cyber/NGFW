import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { problemFor } from '../InterfaceDrawer';
import { createMergePatch, dropPhantomOptionals, localizeSchema } from '../model';
import { useCandidateInterfaces } from '../queries';
import {
  ageText,
  formSchemas,
  LLDP_POINTER,
  localizeAll,
  MONO,
  neighborRows,
  parentNames,
  presence,
  type NeighborRow,
} from './model';
import {
  fetchNeighbors,
  LLDP_POLL_MS,
  lldpKeys,
  useCandidateServices,
  usePatchServices,
} from './queries';

const NS = 'loopback-bvi-gso-lldp-span';

/**
 * LLDP (F-loopback-bvi-gso-lldp-span): the `services.lldp` form (VPP lldp plugin: system name and timers, the interfaces
 * LLDP runs on) and the live neighbour table from the agent's LldpNeighbors RPC (`/state/lldp/neighbors`).
 */
export function LldpPage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const services = useCandidateServices();
  const ifs = useCandidateInterfaces();
  const patch = usePatchServices();
  const [unavailable, setUnavailable] = useState(false);
  const schema = useMemo(() => formSchemas.lldp(), []);
  const current = services.data?.lldp;

  const fetchPage = useCallback(async (req: ServerPageRequest, signal: AbortSignal) => {
    try {
      const p = await fetchNeighbors(req.page + 1, req.pageSize, signal);
      setUnavailable(false);
      return { rows: neighborRows(p), total: p.total };
    } catch (e) {
      if (e instanceof ApiError && e.status === 501) {
        setUnavailable(true);
        return { rows: [], total: 0 };
      }
      throw e;
    }
  }, []);

  const columns = useMemo<GridColDef<NeighborRow>[]>(
    () => [
      {
        field: 'interface',
        headerName: t('lldp.col.interface'),
        minWidth: 150,
        flex: 1,
        sortable: false,
        renderCell: (p) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <Box component="span" dir="ltr" sx={MONO}>
              {p.row.interface}
            </Box>
            {!p.row.configured && (
              <Chip
                size="small"
                variant="outlined"
                color="warning"
                label={t('lldp.notConfigured')}
              />
            )}
          </Stack>
        ),
      },
      {
        field: 'heard',
        headerName: t('lldp.col.status'),
        width: 140,
        sortable: false,
        renderCell: (p) => (
          <StatusChip
            size="small"
            status={presence(p.row.heard)}
            label={p.row.heard ? t('lldp.heard') : t('lldp.silent')}
          />
        ),
      },
      {
        field: 'chassisId',
        headerName: t('lldp.col.chassis'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        renderCell: (p) => (
          <span dir="ltr" title={p.row.chassisIdSubtype}>
            {p.row.chassisId}
          </span>
        ),
      },
      {
        field: 'portId',
        headerName: t('lldp.col.port'),
        minWidth: 140,
        flex: 1,
        sortable: false,
        renderCell: (p) => (
          <span dir="ltr" title={p.row.portIdSubtype}>
            {p.row.portId}
          </span>
        ),
      },
      { field: 'ttl', headerName: t('lldp.col.ttl'), type: 'number', width: 90, sortable: false },
      {
        field: 'lastHeardSecAgo',
        headerName: t('lldp.col.lastHeard'),
        width: 130,
        sortable: false,
        valueFormatter: (v: number) => ageText(v, (k, o) => t(k, o ?? {})),
      },
      {
        field: 'portDescription',
        headerName: t('lldp.col.portDescription'),
        minWidth: 150,
        flex: 1,
        sortable: false,
        renderCell: (p) => <span dir="auto">{p.row.portDescription ?? ''}</span>,
      },
    ],
    [t],
  );

  const save = async (v: unknown) => {
    const next = dropPhantomOptionals(schema, current, v);
    await patch
      .mutateAsync({ lldp: current === undefined ? next : createMergePatch(current, next) })
      .catch(() => undefined);
  };

  return (
    <PageHeader title={t('lldp.title')}>
      <Typography color="text.secondary" sx={{ mb: 1 }}>
        {t('lldp.intro')}
      </Typography>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('lldp.globalsNote')}
      </Alert>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('lldp.settings')}
      </Typography>
      {patch.isError && problemFor(patch.error, LLDP_POINTER) === null && (
        <ProblemAlert error={patch.error} sx={{ mb: 1 }} />
      )}
      {services.isSuccess && (
        <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
          <SchemaForm
            schema={localizeAll(schema, (k, o) => t(k, o ?? {}), localizeSchema)}
            value={current}
            readOnly={!perms.editConfig}
            interfaceOptions={parentNames(ifs.data)}
            submitLabel={t('save')}
            resetLabel={t('reset')}
            problem={problemFor(patch.error, LLDP_POINTER)}
            onSubmit={(v) => void save(v)}
          />
        </Paper>
      )}
      <Typography component="h3" variant="h6" gutterBottom>
        {t('lldp.neighbors')}
      </Typography>
      {unavailable && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('unavailable')}
        </Alert>
      )}
      <Paper variant="outlined" sx={{ blockSize: 380 }}>
        <ServerDataGrid<NeighborRow>
          aria-label={t('lldp.neighbors')}
          columns={columns}
          queryKey={[...lldpKeys.neighbors, 'grid']}
          fetchPage={fetchPage}
          refetchInterval={LLDP_POLL_MS}
          initialPageSize={25}
        />
      </Paper>
    </PageHeader>
  );
}
