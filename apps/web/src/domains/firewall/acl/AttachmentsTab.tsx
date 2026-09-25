import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import EditIcon from '@mui/icons-material/Edit';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
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
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { useCandidateInterfaces } from '../../interfaces/queries';
import { EntryDialog, useArrayEdit } from './EntryDialog';
import {
  attachmentSchema,
  interfaceChoices,
  localizeSchema,
  samePlain,
  targetText,
  type AclAttachment,
  type BoundAcl,
  type LiveBinding,
} from './model';
import {
  BoundChip,
  ConfirmDialog,
  LiveAlerts,
  Mono,
  PendingChip,
  RefreshButton,
  useAclT,
} from './parts';
import { aclKeys, useAclAttachments, useAclLists, useCandidateAt, useRunningAt } from './queries';

const PATH = ['acl', 'attachments'] as const;
const PENDING = 'pending' as const;
const DOT = ' · ';

/**
 * Attachments tab: the configured `acl.attachments` (list → interface or zone, direction, sequence) with an editor,
 * and the bindings as VPP holds them per interface — other owners' ACLs included, shown but never editable (D-066).
 * No timer here: the live table refreshes on demand.
 */
export function AttachmentsTab() {
  const { t } = useTranslation('acl');
  const tr = useAclT();
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const live = useAclAttachments();
  const candidate = useCandidateAt<AclAttachment[]>(PATH, []);
  const running = useRunningAt<AclAttachment[]>(PATH, []);
  const lists = useAclLists({ poll: false });
  const ifaces = useCandidateInterfaces();
  const arr = useArrayEdit<AclAttachment>('attachments');
  const [editing, setEditing] = useState<{
    index: number | null;
    value: AclAttachment | undefined;
    key: number;
  } | null>(null);
  const [deleting, setDeleting] = useState<{ index: number; value: AclAttachment } | null>(null);
  const [deleteError, setDeleteError] = useState<unknown>(null);
  const readOnly = !perms.editConfig;

  const listNames = useMemo(
    () => (lists.data?.lists ?? []).filter((l) => l.pending !== 'deleted').map((l) => l.name),
    [lists.data],
  );
  const schema = useMemo(() => localizeSchema(attachmentSchema(listNames), tr), [listNames, tr]);
  const interfaceOptions = useMemo(
    () =>
      interfaceChoices(
        ifaces.data as Record<string, { subinterfaces?: Record<string, unknown> }> | undefined,
        (live.data?.interfaces ?? []).map((i) => i.interface),
      ),
    [ifaces.data, live.data],
  );
  const rows = candidate.data ?? [];
  const isPending = (a: AclAttachment) =>
    running.isSuccess && !(running.data ?? []).some((r) => samePlain(r, a));
  const removedCount =
    running.isSuccess && candidate.isSuccess
      ? (running.data ?? []).filter((r) => !rows.some((a) => samePlain(a, r))).length
      : 0;

  const remove = async () => {
    if (!deleting) return;
    setDeleteError(null);
    try {
      await arr.remove(deleting.index, deleting.value);
      setDeleting(null);
    } catch (e) {
      setDeleteError(e);
    }
  };

  return (
    <Stack gap={3}>
      <Box component="section" aria-labelledby="acl-attachments-configured">
        <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
          <Typography id="acl-attachments-configured" component="h3" variant="h6" sx={{ flex: 1 }}>
            {t('attachments.configured')}
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
                {t('attachments.add')}
              </Button>
            </span>
          </Tooltip>
        </Stack>
        {candidate.isPending && <LinearProgress aria-label={t('loading')} />}
        {candidate.isError && <ProblemAlert error={candidate.error} />}
        {removedCount > 0 && (
          <Typography variant="body2" color="warning.main" sx={{ mb: 1 }}>
            {t('attachments.removedPending', { count: removedCount, n: fmt.integer(removedCount) })}
          </Typography>
        )}
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('attachments.configuredLabel')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.list')}</TableCell>
                <TableCell>{t('col.target')}</TableCell>
                <TableCell>{t('col.direction')}</TableCell>
                <TableCell>{t('col.sequence')}</TableCell>
                <TableCell>{t('col.vrf')}</TableCell>
                <TableCell>{t('col.enabled')}</TableCell>
                <TableCell>{t('col.description')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('col.actions')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {candidate.isSuccess && rows.length === 0 && (
                <TableRow>
                  <TableCell colSpan={8}>
                    <Typography color="text.secondary">{t('attachments.empty')}</Typography>
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
                    {a.target.kind === 'zone' ? (
                      <bdi>{targetText(a.target, tr)}</bdi>
                    ) : (
                      <Mono>{a.target.interface}</Mono>
                    )}
                  </TableCell>
                  <TableCell>{t(`enum.direction.${a.direction ?? 'in'}`)}</TableCell>
                  <TableCell>{fmt.number(a.sequence, { useGrouping: false })}</TableCell>
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
                      aria-label={t('attachments.editNamed', {
                        list: a.list,
                        target: targetText(a.target, tr),
                      })}
                      disabled={readOnly}
                      onClick={() => setEditing({ index: i, value: a, key: Date.now() })}
                    >
                      <EditIcon fontSize="small" />
                    </IconButton>
                    <IconButton
                      size="small"
                      aria-label={t('attachments.deleteNamed', {
                        list: a.list,
                        target: targetText(a.target, tr),
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

      <Box component="section" aria-labelledby="acl-attachments-live">
        <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }} flexWrap="wrap">
          <Typography id="acl-attachments-live" component="h3" variant="h6" sx={{ flex: 1 }}>
            {t('attachments.live')}
          </Typography>
          <RefreshButton
            onClick={() => void qc.invalidateQueries({ queryKey: aclKeys.attachments })}
            busy={live.isFetching}
            updatedAt={live.data?.retrievedAt ?? live.dataUpdatedAt}
          />
        </Stack>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
          {t('attachments.liveHelp')}
        </Typography>
        {live.data && <LiveAlerts agentError={live.data.agentError} />}
        {live.isPending && <LinearProgress aria-label={t('loading')} />}
        {live.isError && <ProblemAlert error={live.error} />}
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('attachments.liveLabel')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.interface')}</TableCell>
                <TableCell>{t('col.input')}</TableCell>
                <TableCell>{t('col.output')}</TableCell>
                <TableCell>{t('col.macip')}</TableCell>
                <TableCell>{t('col.expected')}</TableCell>
                <TableCell>{t('col.inSync')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {live.data && live.data.interfaces.length === 0 && (
                <TableRow>
                  <TableCell colSpan={6}>
                    <Typography color="text.secondary">{t('attachments.liveEmpty')}</Typography>
                  </TableCell>
                </TableRow>
              )}
              {live.data?.interfaces.map((b) => (
                <TableRow key={b.interface}>
                  <TableCell>
                    <Mono>{b.interface}</Mono>
                  </TableCell>
                  <TableCell>
                    <Chain acls={b.input} />
                  </TableCell>
                  <TableCell>
                    <Chain acls={b.output} />
                  </TableCell>
                  <TableCell>{b.macip ? <BoundChip acl={b.macip} /> : <Dash />}</TableCell>
                  <TableCell>
                    <Expected binding={b} />
                  </TableCell>
                  <TableCell>
                    <SyncChip inSync={b.inSync} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      </Box>

      {editing && (
        <EntryDialog<AclAttachment>
          key={editing.key}
          open
          title={editing.index === null ? t('attachments.addTitle') : t('attachments.editTitle')}
          schema={schema}
          value={editing.value}
          pointer={arr.pointer(editing.index ?? rows.length)}
          interfaceOptions={interfaceOptions}
          readOnly={readOnly}
          onSave={(v) => arr.save(editing.index, editing.value, v)}
          onClose={() => setEditing(null)}
        />
      )}
      <ConfirmDialog
        open={deleting !== null}
        title={t('attachments.deleteTitle')}
        text={
          deleting
            ? t('attachments.deleteText', {
                list: deleting.value.list,
                target: targetText(deleting.value.target, tr),
              })
            : ''
        }
        confirmLabel={t('delete')}
        busy={arr.busy}
        error={deleteError}
        onConfirm={() => void remove()}
        onClose={() => setDeleting(null)}
      />
    </Stack>
  );
}

function Dash() {
  return (
    <Typography component="span" variant="body2" color="text.secondary">
      —
    </Typography>
  );
}

/** An interface's ACLs in evaluation order. */
function Chain({ acls }: { acls: readonly BoundAcl[] }) {
  if (acls.length === 0) return <Dash />;
  return (
    <Stack direction="row" gap={0.5} flexWrap="wrap" alignItems="center">
      {acls.map((a, i) => (
        <BoundChip key={`${a.aclIndex}-${i}`} acl={a} />
      ))}
    </Stack>
  );
}

/** What the running configuration asks for on this interface (this agent's part only). */
function Expected({ binding }: { binding: LiveBinding }) {
  const { t } = useTranslation('acl');
  const sep = t('listSeparator');
  const parts = [
    binding.expected.input.length > 0
      ? t('attachments.expectedIn', { lists: binding.expected.input.join(sep) })
      : null,
    binding.expected.output.length > 0
      ? t('attachments.expectedOut', { lists: binding.expected.output.join(sep) })
      : null,
    binding.expected.macip
      ? t('attachments.expectedMacip', { list: binding.expected.macip })
      : null,
  ].filter((p): p is string => p !== null);
  if (parts.length === 0) return <Dash />;
  const text = parts.join(DOT);
  return (
    <Typography variant="body2">
      <bdi>{text}</bdi>
    </Typography>
  );
}

function SyncChip({ inSync }: { inSync: boolean | null }) {
  const { t } = useTranslation('acl');
  if (inSync === null)
    return <Chip size="small" variant="outlined" label={t('live.syncUnknown')} />;
  return (
    <Chip
      size="small"
      variant="outlined"
      color={inSync ? 'success' : 'warning'}
      label={inSync ? t('live.inSync') : t('live.outOfSync')}
    />
  );
}
