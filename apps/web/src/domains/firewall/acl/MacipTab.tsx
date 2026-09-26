import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import IconButton from '@mui/material/IconButton';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { SchemaForm } from '@ngfw/ui-kit/schema-form';
import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { qk } from '../../../config/queries';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { objectModelWidgets } from '../object-model';
import { EntryDialog, useArrayEdit } from './EntryDialog';
import {
  esc,
  interfaceChoices,
  LIST_NAME_RE,
  localizeSchema,
  macipAttachmentSchema,
  macipListSchema,
  optionalObjectsAsJson,
  problemFor,
  samePlain,
  type MacipAttachment,
  type MacipList,
  type MacipListItem,
} from './model';
import { ConfirmDialog, LiveAlerts, Mono, PendingChip, RefreshButton, useAclT } from './parts';
import {
  aclKeys,
  candidateAt,
  useAclEdit,
  useAclLists,
  useCandidateAt,
  useRunningAt,
} from './queries';

const LTR = { dir: 'ltr' } as const;
const PATH = ['acl', 'macipAttachments'] as const;
const PENDING = 'pending' as const;

/**
 * MACIP tab: MACIP lists (source MAC + mask + source prefix, input only) with live VPP index and rule count, and
 * `acl.macipAttachments` — VPP holds one MACIP list per interface.
 */
export function MacipTab() {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const lists = useAclLists();
  const candidate = useCandidateAt<MacipAttachment[]>(PATH, []);
  const running = useRunningAt<MacipAttachment[]>(PATH, []);
  const ifaces = useCandidateInterfaces();
  const edit = useAclEdit();
  const arr = useArrayEdit<MacipAttachment>('macipAttachments');
  const [listDialog, setListDialog] = useState<{ item: MacipListItem | null } | null>(null);
  const [deletingList, setDeletingList] = useState<MacipListItem | null>(null);
  const [editing, setEditing] = useState<{
    index: number | null;
    value: MacipAttachment | undefined;
    key: number;
  } | null>(null);
  const [deleting, setDeleting] = useState<{ index: number; value: MacipAttachment } | null>(null);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;
  const data = lists.data;

  const names = useMemo(
    () => (data?.macip ?? []).filter((l) => l.pending !== 'deleted').map((l) => l.name),
    [data],
  );
  const schema = useMemo(() => localizeSchema(macipAttachmentSchema(names), tr), [names, tr]);
  const interfaceOptions = useMemo(
    () =>
      interfaceChoices(
        ifaces.data as Record<string, { subinterfaces?: Record<string, unknown> }> | undefined,
      ),
    [ifaces.data],
  );
  const rows = candidate.data ?? [];
  const isPending = (a: MacipAttachment) =>
    running.isSuccess && !(running.data ?? []).some((r) => samePlain(r, a));

  const removeList = async () => {
    if (!deletingList) return;
    setDeleteError(null);
    try {
      await edit.mutateAsync({ method: 'DELETE', path: ['acl', 'macip', deletingList.name] });
      setDeletingList(null);
    } catch (e) {
      setDeleteError(e);
    }
  };
  const removeAttachment = async () => {
    if (!deleting) return;
    setDeleteError(null);
    try {
      await arr.remove(deleting.index, deleting.value);
      setDeleting(null);
    } catch (e) {
      setDeleteError(e);
    }
  };
  /** VPP holds one MACIP list per interface. */
  const onePerInterface = (index: number | null) => (v: MacipAttachment) =>
    rows.some((a, i) => i !== index && a.interface === v.interface)
      ? t('macip.onePerInterface', { interface: v.interface })
      : null;

  return (
    <Stack gap={3}>
      <Box component="section" aria-labelledby="acl-macip-lists">
        <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
          <Typography id="acl-macip-lists" component="h3" variant="h6" sx={{ flex: 1 }}>
            {t('macip.lists')}
          </Typography>
          <Tooltip title={readOnly ? t('readonly') : ''}>
            <span>
              <Button
                variant="contained"
                size="small"
                startIcon={<AddIcon />}
                disabled={readOnly}
                onClick={() => setListDialog({ item: null })}
              >
                {t('macip.add')}
              </Button>
            </span>
          </Tooltip>
          <RefreshButton
            onClick={() => void qc.invalidateQueries({ queryKey: aclKeys.lists })}
            busy={lists.isFetching}
            updatedAt={data?.retrievedAt ?? lists.dataUpdatedAt}
          />
        </Stack>
        {data && <LiveAlerts agentError={data.agentError} />}
        {lists.isPending && <LinearProgress aria-label={t('loading')} />}
        {lists.isError && <ProblemAlert error={lists.error} />}
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('macip.listsLabel')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.name')}</TableCell>
                <TableCell>{t('col.description')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.rules')}</TableCell>
                <TableCell>{t('col.interfaces')}</TableCell>
                <TableCell>{t('col.live')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {data && data.macip.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Typography color="text.secondary">{t('macip.empty')}</Typography>
                  </TableCell>
                </TableRow>
              )}
              {data?.macip.map((l) => (
                <TableRow key={l.name} hover>
                  <TableCell>
                    <Stack direction="row" gap={0.5} alignItems="center">
                      <Mono>{l.name}</Mono>
                      <PendingChip pending={l.pending} />
                    </Stack>
                  </TableCell>
                  <TableCell>{l.description}</TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>{fmt.integer(l.rules)}</TableCell>
                  <TableCell>
                    {l.interfaces.length === 0 ? (
                      <Typography variant="body2" color="text.secondary">
                        {t('lists.notAttached')}
                      </Typography>
                    ) : (
                      <Stack direction="row" gap={0.5} flexWrap="wrap">
                        {l.interfaces.map((i) => (
                          <Chip
                            key={i}
                            size="small"
                            variant="outlined"
                            label={<bdi dir="ltr">{i}</bdi>}
                          />
                        ))}
                      </Stack>
                    )}
                  </TableCell>
                  <TableCell>
                    {l.live ? (
                      <Stack direction="row" gap={0.5} alignItems="center" flexWrap="wrap">
                        <Chip
                          size="small"
                          color="success"
                          variant="outlined"
                          label={t('live.aclIndex', {
                            index: fmt.number(l.live.aclIndex, { useGrouping: false }),
                          })}
                        />
                        <Typography variant="body2">
                          {t('live.vppRules', {
                            count: l.live.vppRules,
                            n: fmt.integer(l.live.vppRules),
                          })}
                        </Typography>
                      </Stack>
                    ) : (
                      <Typography variant="body2" color="text.secondary">
                        {data.agentError
                          ? t('live.unknown')
                          : l.pending === 'added'
                            ? t('live.notCommitted')
                            : t('live.notApplied')}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
                    <IconButton
                      size="small"
                      aria-label={t('macip.editNamed', { name: l.name })}
                      disabled={l.pending === 'deleted'}
                      onClick={() => setListDialog({ item: l })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={t('macip.deleteNamed', { name: l.name })}
                      disabled={readOnly || l.pending === 'deleted'}
                      onClick={() => {
                        setDeleteError(null);
                        setDeletingList(l);
                      }}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>

      <Box component="section" aria-labelledby="acl-macip-attachments">
        <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
          <Typography id="acl-macip-attachments" component="h3" variant="h6" sx={{ flex: 1 }}>
            {t('macip.attachments')}
          </Typography>
          <Tooltip title={readOnly ? t('readonly') : ''}>
            <span>
              <Button
                variant="contained"
                size="small"
                startIcon={<AddIcon />}
                disabled={readOnly}
                onClick={() => setEditing({ index: null, value: undefined, key: Date.now() })}
              >
                {t('macip.attach')}
              </Button>
            </span>
          </Tooltip>
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('macip.attachHelp')}
        </Typography>
        {candidate.isError && <ProblemAlert error={candidate.error} />}
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('macip.attachmentsLabel')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.list')}</TableCell>
                <TableCell>{t('col.interface')}</TableCell>
                <TableCell>{t('col.vrf')}</TableCell>
                <TableCell>{t('col.enabled')}</TableCell>
                <TableCell>{t('col.description')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {candidate.isSuccess && rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Typography color="text.secondary">{t('macip.attachmentsEmpty')}</Typography>
                  </TableCell>
                </TableRow>
              )}
              {rows.map((a, i) => (
                <TableRow key={i} hover>
                  <TableCell>
                    <Stack direction="row" gap={0.5} alignItems="center">
                      <Mono>{a.list}</Mono>
                      {isPending(a) && <PendingChip pending={PENDING} />}
                    </Stack>
                  </TableCell>
                  <TableCell>
                    <Mono>{a.interface}</Mono>
                  </TableCell>
                  <TableCell>{a.vrf ? <Mono>{a.vrf}</Mono> : null}</TableCell>
                  <TableCell>
                    {a.enabled === false ? (
                      <Chip size="small" label={t('rules.off')} />
                    ) : (
                      t('rules.on')
                    )}
                  </TableCell>
                  <TableCell>{a.description}</TableCell>
                  <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
                    <IconButton
                      size="small"
                      aria-label={t('macip.editAttachment', {
                        list: a.list,
                        interface: a.interface,
                      })}
                      disabled={readOnly}
                      onClick={() => setEditing({ index: i, value: a, key: Date.now() })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={t('macip.deleteAttachment', {
                        list: a.list,
                        interface: a.interface,
                      })}
                      disabled={readOnly}
                      onClick={() => {
                        setDeleteError(null);
                        setDeleting({ index: i, value: a });
                      }}
                    >
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>

      <Dialog
        open={listDialog !== null}
        onClose={() => setListDialog(null)}
        fullWidth
        maxWidth="md"
      >
        <DialogTitle>
          {listDialog?.item
            ? t('macip.editTitle', { name: listDialog.item.name })
            : t('macip.addTitle')}
        </DialogTitle>
        <DialogContent>
          {listDialog && (
            <MacipListForm
              key={listDialog.item?.name ?? 'new'}
              name={listDialog.item?.name ?? null}
              taken={data?.macip.map((l) => l.name) ?? []}
              readOnly={readOnly}
              onDone={() => setListDialog(null)}
            />
          )}
        </DialogContent>
      </Dialog>
      {editing && (
        <EntryDialog<MacipAttachment>
          key={editing.key}
          open
          title={editing.index === null ? t('macip.attachTitle') : t('macip.editAttachmentTitle')}
          schema={schema}
          value={editing.value}
          pointer={arr.pointer(editing.index ?? rows.length)}
          interfaceOptions={interfaceOptions}
          readOnly={readOnly}
          validate={onePerInterface(editing.index)}
          onSave={(v) => arr.save(editing.index, editing.value, v)}
          onClose={() => setEditing(null)}
        />
      )}
      <ConfirmDialog
        open={deletingList !== null}
        title={t('macip.deleteTitle', { name: deletingList?.name ?? '' })}
        text={
          deletingList && deletingList.interfaces.length > 0
            ? t('macip.deleteAttached')
            : t('macip.deleteText')
        }
        confirmLabel={t('delete')}
        busy={edit.isPending}
        error={deleteError}
        onConfirm={() => void removeList()}
        onClose={() => setDeletingList(null)}
      />
      <ConfirmDialog
        open={deleting !== null}
        title={t('macip.deleteAttachmentTitle')}
        text={
          deleting
            ? t('macip.deleteAttachmentText', {
                list: deleting.value.list,
                interface: deleting.value.interface,
              })
            : ''
        }
        confirmLabel={t('delete')}
        busy={arr.busy}
        error={deleteError}
        onConfirm={() => void removeAttachment()}
        onClose={() => setDeleting(null)}
      />
    </Stack>
  );
}

/** Create or edit a MACIP list, rules included (MACIP lists are small): PUT `/config/acl/macip/<name>`. */
function MacipListForm({
  name: initialName,
  taken,
  readOnly,
  onDone,
}: {
  name: string | null;
  taken: readonly string[];
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const edit = useAclEdit();
  const [name, setName] = useState(initialName ?? '');
  const [error, setError] = useState<unknown>(null);
  const schema = useMemo(() => localizeSchema(optionalObjectsAsJson(macipListSchema()), tr), [tr]);
  const current = useQuery({
    queryKey: qk.candidate(`acl/macip/${initialName ?? ''}`),
    queryFn: ({ signal }) =>
      candidateAt<MacipList | null>(['acl', 'macip', initialName ?? ''], null, signal),
    enabled: initialName !== null,
    staleTime: Infinity,
  });
  const exists = initialName === null && taken.includes(name);
  const nameOk = LIST_NAME_RE.test(name) && !exists;

  const save = async (v: unknown) => {
    if (!nameOk) return;
    setError(null);
    try {
      await edit.mutateAsync({ method: 'PUT', path: ['acl', 'macip', name], body: v });
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/macip/${esc(name)}`);
  if (initialName !== null && current.isPending)
    return <LinearProgress aria-label={t('loading')} />;
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('lists.name')}
        value={name}
        disabled={initialName !== null}
        onChange={(e) => setName(e.target.value.trim())}
        error={name !== '' && !nameOk}
        helperText={exists ? t('lists.exists') : t('lists.nameHelp')}
        slotProps={{ htmlInput: LTR }}
      />
      {current.isError && <ProblemAlert error={current.error} />}
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={current.data ?? undefined}
        readOnly={readOnly}
        widgets={objectModelWidgets}
        problem={problem}
        submitLabel={t('save')}
        resetLabel={t('reset')}
        onSubmit={save}
      >
        <Button onClick={onDone}>{t('cancel')}</Button>
      </SchemaForm>
    </Stack>
  );
}
