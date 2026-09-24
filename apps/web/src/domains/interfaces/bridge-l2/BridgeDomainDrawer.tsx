import AddIcon from '@mui/icons-material/Add';
import CloseIcon from '@mui/icons-material/Close';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { ServerDataGrid, type GridColDef, type ServerPageRequest } from '@ngfw/ui-kit/data-grid';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useCallback, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { problemFor } from '../InterfaceDrawer';
import { createMergePatch, dropPhantomOptionals, localizeSchema } from '../model';
import {
  END_CELL,
  formSchemas,
  L2_TABLES,
  l2Patch,
  macKind,
  presence,
  roleOf,
  membersOf,
  portPatch,
  portsOf,
  tagRewriteText,
  type BridgeDomainItem,
  type MacRow,
  type Port,
} from './model';
import {
  bridgeKeys,
  fetchMacs,
  useCandidateL2,
  useCandidatePorts,
  usePatchPorts,
  usePatchRouting,
} from './queries';
import { RecordDialog } from './RecordDialog';

const esc = (s: string) => s.replace(/~/g, '~0').replace(/\//g, '~1');
const MONO = { fontFamily: 'monospace' } as const;

/** Right-hand drawer of one bridge domain: settings (schema form), members (interfaces.<if>.l2), the live MAC table. */
export function BridgeDomainDrawer({
  item,
  onClose,
}: {
  item: BridgeDomainItem | null;
  onClose: () => void;
}) {
  return (
    <Drawer
      anchor="right"
      open={item !== null}
      onClose={onClose}
      sx={{ zIndex: (th) => th.zIndex.modal }}
      slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 720 } } } }}
    >
      {item !== null && <DrawerBody key={item.name} item={item} onClose={onClose} />}
    </Drawer>
  );
}

function DrawerBody({ item, onClose }: { item: BridgeDomainItem; onClose: () => void }) {
  const { t } = useTranslation('bridge-l2');
  const perms = usePermissions();
  const readOnly = !perms.editConfig;
  const l2 = useCandidateL2();
  const ifs = useCandidatePorts();
  const patchRouting = usePatchRouting();
  const patchPorts = usePatchPorts();
  const [memberEdit, setMemberEdit] = useState<{ port: Port | null } | null>(null);
  const name = item.name;
  const record = l2.data?.bridgeDomains?.[name];
  const ports = useMemo(() => portsOf(ifs.data), [ifs.data]);
  const members = membersOf(ports, name);
  const live = new Map((item.state?.members ?? []).map((m) => [m.interface, m]));
  const domainSchema = useMemo(() => formSchemas.domain(), []);
  const portSchema = useMemo(() => formSchemas.port(), []);

  const saveSettings = async (value: unknown) => {
    const cleaned = dropPhantomOptionals(domainSchema, record, value);
    await patchRouting
      .mutateAsync(
        l2Patch(
          'bridgeDomains',
          name,
          record === undefined ? cleaned : createMergePatch(record, cleaned),
        ),
      )
      .catch(() => undefined);
  };
  const removeDomain = async () => {
    await patchRouting
      .mutateAsync(l2Patch(L2_TABLES.bridgeDomains, name, null))
      .catch(() => undefined);
    if (!patchRouting.isError) onClose();
  };
  const saveMember = async (portName: string, value: unknown) => {
    const port = ports.find((p) => p.name === portName);
    if (!port) return;
    const next = { ...(value as Record<string, unknown>), bridgeDomain: name };
    const body = port.l2 === undefined ? next : createMergePatch(port.l2, next);
    try {
      await patchPorts.mutateAsync(portPatch(port, body));
      setMemberEdit(null);
    } catch {
      /* shown in the dialog */
    }
  };
  const removeMember = async (port: Port) => {
    await patchPorts.mutateAsync(portPatch(port, null)).catch(() => undefined);
  };

  const fetchPage = useCallback(
    async (req: ServerPageRequest, signal: AbortSignal) => {
      if (item.id === null || item.state === null)
        return { rows: [] as (MacRow & { id: string })[], total: 0 };
      const r = await fetchMacs(item.id, req.page + 1, req.pageSize, signal);
      return { rows: r.items.map((m) => ({ ...m, id: m.mac })), total: r.total };
    },
    [item.id, item.state],
  );
  const macColumns = useMemo<GridColDef<MacRow & { id: string }>[]>(
    () => [
      {
        field: 'mac',
        headerName: t('col.mac'),
        minWidth: 170,
        flex: 1,
        sortable: false,
        renderCell: (p) => (
          <span dir="ltr" style={MONO}>
            {p.row.mac}
          </span>
        ),
      },
      {
        field: 'interface',
        headerName: t('col.interface'),
        minWidth: 160,
        flex: 1,
        sortable: false,
        renderCell: (p) => <span dir="ltr">{p.row.interface}</span>,
      },
      {
        field: 'type',
        headerName: t('col.type'),
        width: 120,
        sortable: false,
        valueGetter: (_v, r) => t(`type.${macKind(r)}`),
      },
    ],
    [t],
  );

  const pointerOfMember = (portName: string) => {
    const p = ports.find((x) => x.name === portName);
    if (!p) return '/interfaces';
    return p.sub === null
      ? `/interfaces/${esc(p.parent)}/l2`
      : `/interfaces/${esc(p.parent)}/subinterfaces/${p.sub}/l2`;
  };
  const candidates = ports.filter((p) => p.l2?.bridgeDomain === undefined).map((p) => p.name);

  return (
    <Box sx={{ p: 2 }}>
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="h6" sx={{ flex: 1 }}>
          {t('domain.drawerTitle', { name })}
        </Typography>
        <IconButton aria-label={t('close')} onClick={onClose}>
          <CloseIcon />
        </IconButton>
      </Stack>
      <Stack direction="row" gap={1} sx={{ mb: 2 }} alignItems="center">
        <StatusChip
          size="small"
          status={presence(item.state !== null)}
          label={item.state ? t('status.live') : t('status.missing')}
        />
        {item.id !== null && (
          <Typography variant="body2" color="text.secondary" dir="ltr">
            {t('domain.vppId', { id: item.id })}
          </Typography>
        )}
      </Stack>

      <Typography component="h4" variant="subtitle1" sx={{ mb: 1 }}>
        {t('domain.settings')}
      </Typography>
      {patchRouting.isError && <ProblemAlert error={patchRouting.error} sx={{ mb: 1 }} />}
      {l2.isSuccess && (
        <SchemaForm
          schema={localizeSchema(domainSchema, (k, o) => t(k, o ?? {}))}
          value={record}
          readOnly={readOnly}
          submitLabel={t('save')}
          resetLabel={t('reset')}
          problem={problemFor(patchRouting.error, `/routing/l2/bridgeDomains/${esc(name)}`)}
          onSubmit={(v) => void saveSettings(v)}
        >
          {record && (
            <Tooltip title={members.length > 0 ? t('domain.removeHint') : ''}>
              <span>
                <Button
                  color="error"
                  variant="outlined"
                  startIcon={<DeleteIcon />}
                  disabled={readOnly || members.length > 0 || patchRouting.isPending}
                  onClick={() => void removeDomain()}
                >
                  {t('domain.removeDomain')}
                </Button>
              </span>
            </Tooltip>
          )}
        </SchemaForm>
      )}

      <Divider sx={{ my: 2 }} />
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h4" variant="subtitle1" sx={{ flex: 1 }}>
          {t('domain.members')}
        </Typography>
        <Button
          size="small"
          startIcon={<AddIcon />}
          disabled={readOnly || record === undefined}
          onClick={() => setMemberEdit({ port: null })}
        >
          {t('domain.addMember')}
        </Button>
      </Stack>
      {patchPorts.isError && memberEdit === null && (
        <ProblemAlert error={patchPorts.error} sx={{ mb: 1 }} />
      )}
      {members.length === 0 ? (
        <Typography variant="body2" color="text.secondary">
          {t('domain.noMembers')}
        </Typography>
      ) : (
        <Table size="small" aria-label={t('domain.members')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.interface')}</TableCell>
              <TableCell>{t('col.role')}</TableCell>
              <TableCell>{t('col.shg')}</TableCell>
              <TableCell>{t('col.tagRewrite')}</TableCell>
              <TableCell>{t('col.live')}</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {members.map((m) => {
              const role = roleOf(m.l2);
              const lv = live.get(m.name);
              return (
                <TableRow key={m.name}>
                  <TableCell dir="ltr" sx={MONO}>
                    {m.name}
                  </TableCell>
                  <TableCell>{t(`role.${role}`)}</TableCell>
                  <TableCell>{m.l2?.shg ?? 0}</TableCell>
                  <TableCell dir="ltr">{tagRewriteText(m.l2)}</TableCell>
                  <TableCell>
                    <StatusChip
                      size="small"
                      status={presence(lv !== undefined)}
                      label={lv ? t('status.live') : t('status.missing')}
                    />
                  </TableCell>
                  <TableCell sx={END_CELL}>
                    <IconButton
                      size="small"
                      aria-label={`${t('edit')} ${m.name}`}
                      disabled={readOnly}
                      onClick={() => setMemberEdit({ port: m })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={`${t('remove')} ${m.name}`}
                      disabled={readOnly || patchPorts.isPending}
                      onClick={() => void removeMember(m)}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              );
            })}
          </TableBody>
        </Table>
      )}

      <Divider sx={{ my: 2 }} />
      <Typography component="h4" variant="subtitle1">
        {t('domain.macs')}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
        {t('domain.macsHelp')}
      </Typography>
      <Paper variant="outlined" sx={{ blockSize: 320 }}>
        <ServerDataGrid<MacRow & { id: string }>
          aria-label={t('domain.macs')}
          columns={macColumns}
          queryKey={[...bridgeKeys.macs(item.id ?? 0), 'grid']}
          fetchPage={fetchPage}
          refetchInterval={5_000}
          initialPageSize={25}
        />
      </Paper>

      {memberEdit !== null && (
        <RecordDialog
          open
          title={
            memberEdit.port
              ? t('domain.memberTitle', { name: memberEdit.port.name })
              : t('domain.addMember')
          }
          keyLabel={t('domain.interfacePick')}
          keyHelp={t('domain.interfaceHelp')}
          keyOptions={memberEdit.port ? [memberEdit.port.name] : candidates}
          {...(memberEdit.port ? { fixedKey: memberEdit.port.name } : {})}
          schema={portSchema}
          value={memberEdit.port?.l2}
          error={patchPorts.error}
          pending={patchPorts.isPending}
          readOnly={readOnly}
          pointerOf={pointerOfMember}
          onClose={() => setMemberEdit(null)}
          onSubmit={(k, v) => void saveMember(k, v)}
        />
      )}
    </Box>
  );
}
