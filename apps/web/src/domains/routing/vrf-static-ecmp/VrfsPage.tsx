import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { ApiError } from '../../../api-problem';
import { useCandidate, useFibQueryDefaults, usePatchConfig, useRefreshRoutes, useRoutes } from './api';
import { createMergePatch } from '../../interfaces/model';
import { esc, Mono, pageOf, problemFor } from './common';
import { localizeSchema, NS, vrfItemSchema, vrfRows, type VrfConfig, type VrfRow, type VrfsConfig } from './model';

const NAME_RE = /^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$/;
const LTR = { dir: 'ltr' } as const;

/**
 * Live status of a VRF: how many FIB entries its table has (the agent's ListRoutes total). Read once and on the page's
 * refresh button, never polled: the count is a full walk of the table in VPP (review H1).
 */
function LiveRoutes({ vrf }: { vrf: string }) {
  const { t } = useTranslation(NS);
  const q = useRoutes({ vrf, page: 1, pageSize: 1 });
  if (q.isError)
    return q.error instanceof ApiError && q.error.status === 404 ? (
      <Chip size="small" color="warning" variant="outlined" label={t('vrfs.notInDataplane')} />
    ) : (
      <Tooltip title={q.error.message}>
        <Chip size="small" variant="outlined" label={t('vrfs.fibUnknown')} />
      </Tooltip>
    );
  if (!q.data) return null;
  return <Chip size="small" color="success" variant="outlined" label={t('vrfs.fibEntries', { count: q.data.total })} />;
}

/** Candidate interface names (parents and "<parent>.<id>") for the source-select interface picker. */
function useInterfaceNames(): string[] {
  const ifs = useCandidate<Record<string, { subinterfaces?: Record<string, unknown> }>>('interfaces');
  return useMemo(
    () =>
      Object.entries(ifs.data ?? {})
        .flatMap(([n, i]) => [n, ...Object.keys(i.subinterfaces ?? {}).map((s) => `${n}.${s}`)])
        .sort(),
    [ifs.data],
  );
}

/** `/routing/vrfs`: VRFs (FIB tables) with source VRF select; edits go into the candidate (commit from the pending bar). */
export function VrfsPage() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const vrfs = useCandidate<VrfsConfig>('vrfs');
  const patch = usePatchConfig();
  const ifNames = useInterfaceNames();
  useFibQueryDefaults();
  const refreshCounts = useRefreshRoutes();
  const [edit, setEdit] = useState<{ name: string; value: VrfConfig | undefined } | null>(null);
  const [adding, setAdding] = useState(false);
  const [newName, setNewName] = useState('');
  const schema = useMemo(() => localizeSchema(vrfItemSchema(), (k, o) => t(k, o ?? {})), [t]);
  const rows = useMemo(() => vrfRows(vrfs.data), [vrfs.data]);

  const fetchPage = useCallback(
    async (req: ServerPageRequest) => pageOf(rows, req, (r) => `${r.name} ${r.tableId} ${r.description}`),
    [rows],
  );

  const columns = useMemo<GridColDef<VrfRow>[]>(
    () => [
      { field: 'name', headerName: t('vrfs.col.name'), minWidth: 160, flex: 1, renderCell: (p) => <Mono>{p.row.name}</Mono> },
      { field: 'tableId', headerName: t('vrfs.col.table'), type: 'number', width: 120 },
      { field: 'description', headerName: t('vrfs.col.description'), minWidth: 180, flex: 1 },
      { field: 'sourceSelect', headerName: t('vrfs.col.sourceSelect'), type: 'number', width: 150 },
      {
        field: 'live',
        headerName: t('vrfs.col.live'),
        sortable: false,
        filterable: false,
        width: 170,
        renderCell: (p) => <LiveRoutes vrf={p.row.name} />,
      },
    ],
    [t],
  );

  const save = async (value: unknown) => {
    if (!edit) return;
    try {
      // merge patch from what the form opened with: removed members become null (RFC 7386)
      const body = edit.value === undefined ? value : createMergePatch(edit.value, value);
      await patch.mutateAsync({ key: 'vrfs', patch: { [edit.name]: body } });
      setEdit(null);
    } catch {
      // shown in the dialog (pointers mapped onto the fields)
    }
  };

  const remove = async (name: string) => {
    try {
      await patch.mutateAsync({ key: 'vrfs', patch: { [name]: null } });
      setEdit(null);
    } catch {
      // shown in the dialog
    }
  };

  return (
    <PageHeader title={t('vrfs.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('vrfs.intro')}
      </Typography>
      <Stack direction="row" gap={1} sx={{ mb: 1 }}>
        <Tooltip title={perms.editConfig ? '' : t('readonly')}>
          <span>
            <Button variant="contained" startIcon={<AddIcon />} disabled={!perms.editConfig} onClick={() => setAdding(true)}>
              {t('vrfs.add')}
            </Button>
          </span>
        </Tooltip>
        <Tooltip title={t('fib.refreshHelp')}>
          <Button variant="outlined" startIcon={<RefreshIcon />} onClick={refreshCounts}>
            {t('vrfs.refreshCounts')}
          </Button>
        </Tooltip>
      </Stack>
      {vrfs.isError && <ProblemAlert error={vrfs.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined" sx={{ blockSize: 420 }}>
        <ServerDataGrid<VrfRow>
          aria-label={t('vrfs.title')}
          columns={columns}
          queryKey={['config', 'candidate', 'vrfs', 'grid', vrfs.dataUpdatedAt]}
          fetchPage={fetchPage}
          initialPageSize={25}
          onRowClick={(p) =>
            setEdit({ name: p.row.name, value: vrfs.data?.[p.row.name] ?? (p.row.name === 'default' ? ({ id: 0 } as VrfConfig) : undefined) })
          }
          sx={{ '& .MuiDataGrid-row': { cursor: 'pointer' } }}
        />
      </Paper>

      <Dialog open={adding} onClose={() => setAdding(false)} fullWidth maxWidth="xs">
        <DialogTitle>{t('vrfs.addTitle')}</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            fullWidth
            sx={{ mt: 1 }}
            label={t('vrfs.col.name')}
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            error={newName !== '' && !NAME_RE.test(newName.trim())}
            helperText={newName !== '' && !NAME_RE.test(newName.trim()) ? t('vrfs.badName') : ' '}
            slotProps={{ htmlInput: LTR }}
          />
          <Stack direction="row" gap={1} justifyContent="flex-end">
            <Button onClick={() => setAdding(false)}>{t('cancel')}</Button>
            <Button
              variant="contained"
              disabled={!NAME_RE.test(newName.trim()) || vrfs.data?.[newName.trim()] !== undefined}
              onClick={() => {
                setAdding(false);
                setEdit({ name: newName.trim(), value: undefined });
                setNewName('');
              }}
            >
              {t('next')}
            </Button>
          </Stack>
        </DialogContent>
      </Dialog>

      <Dialog open={edit !== null} onClose={() => setEdit(null)} fullWidth maxWidth="md">
        <DialogTitle>
          {edit?.value ? t('vrfs.editTitle', { name: edit.name }) : t('vrfs.newTitle', { name: edit?.name ?? '' })}
        </DialogTitle>
        <DialogContent>
          {edit && (
            <Box sx={{ pt: 1 }}>
              <Alert severity="info" sx={{ mb: 2 }}>
                {t('vrfs.sourceSelectHelp')}
              </Alert>
              {patch.isError && <ProblemAlert error={patch.error} sx={{ mb: 1 }} />}
              <SchemaForm
                key={edit.name}
                schema={schema}
                value={edit.value}
                readOnly={!perms.editConfig}
                interfaceOptions={ifNames}
                submitLabel={t('save')}
                resetLabel={t('reset')}
                problem={problemFor(patch.error, `/vrfs/${esc(edit.name)}`)}
                onSubmit={save}
              >
                {edit.value && edit.name !== 'default' && (
                  <Button color="error" variant="outlined" startIcon={<DeleteIcon />} disabled={!perms.editConfig || patch.isPending} onClick={() => void remove(edit.name)}>
                    {t('remove')}
                  </Button>
                )}
              </SchemaForm>
            </Box>
          )}
        </DialogContent>
      </Dialog>
    </PageHeader>
  );
}
