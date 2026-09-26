import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import ListAltIcon from '@mui/icons-material/ListAlt';
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
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { TagChips, objectModelWidgets, useCandidateObjects } from '../object-model';
import {
  esc,
  formatBytes,
  LIST_NAME_RE,
  listMetaSchema,
  localizeSchema,
  mergePatch,
  problemFor,
  targetText,
  type AclListItem,
  type AclListsState,
} from './model';
import { ConfirmDialog, LiveAlerts, Mono, PendingChip, RefreshButton, useAclT } from './parts';
import { aclKeys, useAclEdit, useAclLists } from './queries';

const LTR = { dir: 'ltr' } as const;

/** Lists tab: every L3/L4 list with its rule count, attachments, pending mark and live VPP status. */
export function ListsTab({ onOpenRules }: { onOpenRules: (name: string) => void }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const lists = useAclLists();
  const objects = useCandidateObjects();
  const edit = useAclEdit();
  const [dialog, setDialog] = useState<{ item: AclListItem | null } | null>(null);
  const [deleting, setDeleting] = useState<AclListItem | null>(null);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;
  const data = lists.data;

  const remove = async () => {
    if (!deleting) return;
    setDeleteError(null);
    try {
      await edit.mutateAsync({ method: 'DELETE', path: ['acl', 'lists', deleting.name] });
      setDeleting(null);
    } catch (e) {
      setDeleteError(e);
    }
  };

  return (
    <>
      <Stack direction="row" gap={1} sx={{ mb: 1 }} alignItems="center" flexWrap="wrap">
        <Tooltip title={readOnly ? t('readonly') : ''}>
          <span>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              disabled={readOnly}
              onClick={() => setDialog({ item: null })}
            >
              {t('lists.add')}
            </Button>
          </span>
        </Tooltip>
        <Box sx={{ flex: 1 }} />
        <RefreshButton
          onClick={() => void qc.invalidateQueries({ queryKey: aclKeys.lists })}
          busy={lists.isFetching}
          updatedAt={data?.retrievedAt ?? lists.dataUpdatedAt}
        />
      </Stack>
      {data && (
        <LiveAlerts
          agentError={data.agentError}
          countersAvailable={data.countersAvailable}
          countersReason={data.countersReason}
        />
      )}
      {lists.isPending && <LinearProgress aria-label={t('loading')} />}
      {lists.isError && <ProblemAlert error={lists.error} />}
      <TableContainer component={Paper} variant="outlined">
        <Table size="small" aria-label={t('lists.tableLabel')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.name')}</TableCell>
              <TableCell>{t('col.description')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.rules')}</TableCell>
              <TableCell>{t('col.attachments')}</TableCell>
              <TableCell>{t('col.tags')}</TableCell>
              <TableCell>{t('col.live')}</TableCell>
              <TableCell>{t('col.hits')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {data && data.lists.length === 0 && (
              <TableRow>
                <TableCell colSpan={8}>
                  <Typography color="text.secondary">{t('lists.empty')}</Typography>
                </TableCell>
              </TableRow>
            )}
            {data?.lists.map((l) => (
              <TableRow key={l.name} hover>
                <TableCell>
                  <Stack direction="row" gap={0.5} alignItems="center">
                    <Mono>{l.name}</Mono>
                    <PendingChip pending={l.pending} />
                  </Stack>
                </TableCell>
                <TableCell>{l.description}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>
                  <Count value={l.rules} />
                </TableCell>
                <TableCell>
                  <AttachmentChips item={l} />
                </TableCell>
                <TableCell>
                  <TagChips tags={l.tags} objects={objects.data} />
                </TableCell>
                <TableCell>
                  <LiveCell item={l} state={data} />
                </TableCell>
                <TableCell>
                  <HitsCell item={l} state={data} />
                </TableCell>
                <TableCell sx={{ textAlign: 'end', whiteSpace: 'nowrap' }}>
                  <Tooltip title={t('lists.openRules')}>
                    <IconButton
                      size="small"
                      aria-label={t('lists.openRulesOf', { name: l.name })}
                      onClick={() => onOpenRules(l.name)}
                    >
                      <ListAltIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                  <IconButton
                    size="small"
                    aria-label={t('lists.editNamed', { name: l.name })}
                    disabled={readOnly || l.pending === 'deleted'}
                    onClick={() => setDialog({ item: l })}
                  >
                    <EditIcon fontSize="small" />
                  </IconButton>
                  <IconButton
                    size="small"
                    aria-label={t('lists.deleteNamed', { name: l.name })}
                    disabled={readOnly || l.pending === 'deleted'}
                    onClick={() => {
                      setDeleteError(null);
                      setDeleting(l);
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
      <Dialog open={dialog !== null} onClose={() => setDialog(null)} fullWidth maxWidth="sm">
        <DialogTitle>
          {dialog?.item ? t('lists.editTitle', { name: dialog.item.name }) : t('lists.addTitle')}
        </DialogTitle>
        <DialogContent>
          {dialog && (
            <ListForm
              key={dialog.item?.name ?? 'new'}
              item={dialog.item}
              taken={data?.lists.map((l) => l.name) ?? []}
              readOnly={readOnly}
              onDone={() => setDialog(null)}
            />
          )}
        </DialogContent>
      </Dialog>
      <ConfirmDialog
        open={deleting !== null}
        title={t('lists.deleteTitle', { name: deleting?.name ?? '' })}
        text={
          <>
            {t('lists.deleteText', {
              count: deleting?.rules ?? 0,
              n: fmt.integer(deleting?.rules ?? 0),
            })}
            {deleting && deleting.attachments.length > 0
              ? ` ${t('lists.deleteAttached', { count: deleting.attachments.length, n: fmt.integer(deleting.attachments.length) })}`
              : ''}
          </>
        }
        confirmLabel={t('delete')}
        busy={edit.isPending}
        error={deleteError}
        onConfirm={() => void remove()}
        onClose={() => setDeleting(null)}
      />
    </>
  );
}

function Count({ value }: { value: number }) {
  const fmt = useFormatters();
  return <Box component="span">{fmt.integer(value)}</Box>;
}

function AttachmentChips({ item }: { item: AclListItem }) {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  if (item.attachments.length === 0) {
    return (
      <Typography variant="body2" color="text.secondary">
        {t('lists.notAttached')}
      </Typography>
    );
  }
  return (
    <Stack direction="row" gap={0.5} flexWrap="wrap">
      {item.attachments.map((a, i) => (
        <Chip
          key={i}
          size="small"
          variant="outlined"
          color={a.enabled ? 'default' : 'warning'}
          label={t('lists.attachment', {
            target: `⁨${targetText(a.target, tr)}⁩`,
            direction: t(`enum.direction.${a.direction}`, { defaultValue: a.direction }),
          })}
          title={a.enabled ? undefined : t('lists.attachmentDisabled')}
        />
      ))}
    </Stack>
  );
}

function LiveCell({ item, state }: { item: AclListItem; state: AclListsState }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  if (!item.live) {
    const why = state.agentError
      ? t('live.unknown')
      : item.pending === 'added'
        ? t('live.notCommitted')
        : t('live.notApplied');
    return (
      <Typography variant="body2" color="text.secondary">
        {why}
      </Typography>
    );
  }
  return (
    <Stack direction="row" gap={0.5} alignItems="center" flexWrap="wrap">
      <Chip
        size="small"
        color="success"
        variant="outlined"
        label={t('live.aclIndex', {
          index: fmt.number(item.live.aclIndex, { useGrouping: false }),
        })}
      />
      <Typography variant="body2">
        {t('live.vppRules', { count: item.live.vppRules, n: fmt.integer(item.live.vppRules) })}
      </Typography>
    </Stack>
  );
}

function HitsCell({ item, state }: { item: AclListItem; state: AclListsState }) {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const fmt = useFormatters();
  if (!state.countersAvailable || !item.live) {
    return (
      <Tooltip title={state.countersAvailable ? t('live.notApplied') : state.countersReason}>
        <Typography component="span" variant="body2" color="text.secondary">
          —
        </Typography>
      </Tooltip>
    );
  }
  return (
    <Typography variant="body2" sx={{ fontVariantNumeric: 'tabular-nums' }}>
      {t('live.hits', {
        packets: fmt.integer(item.live.packets),
        bytes: formatBytes(item.live.bytes, fmt, tr),
      })}
    </Typography>
  );
}

/** Create a list (name + description + tags → PUT) or edit its description/tags (merge patch); rules have their own editor. */
function ListForm({
  item,
  taken,
  readOnly,
  onDone,
}: {
  item: AclListItem | null;
  taken: readonly string[];
  readOnly: boolean;
  onDone: () => void;
}) {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const edit = useAclEdit();
  const [name, setName] = useState(item?.name ?? '');
  const [error, setError] = useState<unknown>(null);
  const schema = useMemo(() => localizeSchema(listMetaSchema(), tr), [tr]);
  const value = useMemo(
    () =>
      item
        ? {
            ...(item.description !== null ? { description: item.description } : {}),
            tags: item.tags,
          }
        : undefined,
    [item],
  );
  const exists = !item && taken.includes(name);
  const nameOk = LIST_NAME_RE.test(name) && !exists;

  const save = async (v: unknown) => {
    if (!nameOk) return;
    setError(null);
    try {
      if (item) {
        const patch = mergePatch(value, v);
        if (patch && typeof patch === 'object' && Object.keys(patch).length > 0)
          await edit.mutateAsync({ method: 'PATCH', path: ['acl', 'lists', name], body: patch });
      } else {
        await edit.mutateAsync({
          method: 'PUT',
          path: ['acl', 'lists', name],
          body: { ...(v as object), rules: [] },
        });
      }
      onDone();
    } catch (e) {
      setError(e);
    }
  };
  const problem = problemFor(error, `/acl/lists/${esc(name)}`);
  return (
    <Stack gap={2} sx={{ pt: 1 }}>
      <TextField
        label={t('lists.name')}
        value={name}
        disabled={item !== null}
        onChange={(e) => setName(e.target.value.trim())}
        error={name !== '' && !nameOk}
        helperText={exists ? t('lists.exists') : t('lists.nameHelp')}
        slotProps={{ htmlInput: LTR }}
      />
      {error !== null && problem === null && <ProblemAlert error={error} />}
      <SchemaForm
        schema={schema}
        value={value}
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
