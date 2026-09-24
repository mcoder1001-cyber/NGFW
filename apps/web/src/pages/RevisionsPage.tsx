import CompareArrowsIcon from '@mui/icons-material/CompareArrows';
import HistoryIcon from '@mui/icons-material/History';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { diff } from '@ngfw/schema';
import { useFormatters } from '@ngfw/ui-kit';
import { ServerDataGrid, type FetchPage, type GridColDef } from '@ngfw/ui-kit/data-grid';
import { useMemo, useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api';
import { call, isUnreachable } from '../api-problem';
import { usePermissions } from '../auth/AuthProvider';
import {
  ConfirmWindowField,
  DEFAULT_CONFIRM_MINUTES,
  confirmWindowValid,
  trackPending,
  type ConfirmWindow,
} from '../config/CommitDialog';
import { CommitResultView } from '../config/CommitResultView';
import { confirmStore } from '../config/confirm-store';
import { DiffView } from '../config/DiffView';
import { ProblemAlert } from '../config/ProblemAlert';
import { useEffectiveChanges } from '../config/effective';
import {
  qk,
  usePending,
  useRevision,
  useRollback,
  useRunning,
  type CommitResult,
  type RevisionMeta,
} from '../config/queries';
import { applyOutcome, type ApplyOutcome } from '../net';
import { PageHeader } from '../shell/PageHeader';

const REV_INPUT = { min: 1, dir: 'ltr' } as const;

const fetchRevisions: FetchPage<RevisionMeta> = async ({ page, pageSize }, signal) => {
  const { data } = await call(
    api.GET('/api/v1/config/revisions', {
      params: { query: { limit: pageSize, offset: page * pageSize } },
      signal,
    }),
  );
  return { rows: data.items, total: data.total };
};

function Why({ reason, children }: { reason: string | undefined; children: ReactElement }) {
  return reason ? (
    <Tooltip title={reason}>
      <span>{children}</span>
    </Tooltip>
  ) : (
    children
  );
}

/** Diff between two revisions (`from` 0 = the empty document before the first revision), computed from their payloads. */
function RevisionDiff({ from, to }: { from: number; to: number }) {
  const { t } = useTranslation('revisions');
  const a = useRevision(from > 0 ? from : null);
  const b = useRevision(to);
  const changes = useMemo(
    () =>
      b.data && (from === 0 || a.data)
        ? diff(from === 0 ? {} : a.data!.payload, b.data.payload)
        : null,
    [a.data, b.data, from],
  );
  const error = a.error ?? b.error;
  return (
    <Paper
      variant="outlined"
      sx={{ p: 2 }}
      component="section"
      aria-label={t('compare.result', { from, to })}
      data-testid="revision-diff"
    >
      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {from === 0 ? t('compare.initial', { to }) : t('compare.result', { from, to })}
      </Typography>
      {error ? (
        <ProblemAlert error={error} />
      ) : changes === null ? (
        <LinearProgress aria-label={t('loading')} />
      ) : (
        <DiffView changes={changes} />
      )}
    </Paper>
  );
}

type LostAnswer = ApplyOutcome | 'looking' | 'lookup-failed';

/**
 * TD-10a (review 2.4a): what became of a rollback whose answer never arrived (deadline passed, route cut) — from the
 * API's own state: pending commit and sync (GET /state/system), newest revision.
 */
async function lookupOutcome(
  sentAt: number,
  beforeRevision: number | null,
): Promise<ApplyOutcome | 'lookup-failed'> {
  try {
    const [sys, revs] = await Promise.all([
      call(api.GET('/api/v1/state/system')),
      call(api.GET('/api/v1/config/revisions', { params: { query: { limit: 1, offset: 0 } } })),
    ]);
    return applyOutcome({
      sentAt,
      beforeRevision,
      pending: sys.data.pendingCommit as { txnId?: unknown; deadline?: unknown } | null,
      sync: sys.data.sync,
      newest: revs.data.items[0] ?? null,
    });
  } catch {
    return 'lookup-failed';
  }
}

function LostAnswerAlert({ lost }: { lost: LostAnswer }) {
  const { t } = useTranslation('revisions');
  if (lost === 'looking') return <Alert severity="info">{t('rollback.outcome.looking')}</Alert>;
  if (lost === 'lookup-failed')
    return <Alert severity="error">{t('rollback.outcome.lookupFailed')}</Alert>;
  switch (lost.kind) {
    case 'applied':
      return (
        <Alert severity="success">
          {t('rollback.outcome.applied', { revision: lost.revision })}
        </Alert>
      );
    case 'pending':
      return (
        <Alert severity="warning">{t('rollback.outcome.pending', { txnId: lost.txnId })}</Alert>
      );
    case 'unknown':
      return (
        <Alert severity="error">{t('rollback.outcome.unknown', { reason: lost.reason })}</Alert>
      );
    default:
      return <Alert severity="info">{t('rollback.outcome.notApplied')}</Alert>;
  }
}

function RollbackDialog({
  target,
  runningRev,
  onClose,
}: {
  target: RevisionMeta | null;
  runningRev: number | null;
  onClose: () => void;
}) {
  const { t } = useTranslation(['revisions', 'config']);
  const running = useRunning(target !== null);
  const rev = useRevision(target?.id ?? null);
  const rollback = useRollback();
  const [comment, setComment] = useState('');
  const [revert, setRevert] = useState<ConfirmWindow>({
    enabled: true,
    minutes: DEFAULT_CONFIRM_MINUTES,
  });
  const [result, setResult] = useState<CommitResult | null>(null);
  const [lost, setLost] = useState<LostAnswer | null>(null);
  const changes = useMemo(
    () => (running.data !== undefined && rev.data ? diff(running.data, rev.data.payload) : null),
    [running.data, rev.data],
  );
  const close = () => {
    rollback.reset();
    setResult(null);
    setLost(null);
    setComment('');
    onClose();
  };
  if (!target) return null;
  const submit = () => {
    const sentAt = Date.now();
    const win = revert;
    setLost(null);
    rollback.mutate(
      { rev: target.id, comment, confirmSec: revert.enabled ? revert.minutes * 60 : undefined },
      {
        onSuccess: (outcome) => {
          if (outcome.result.status === 'pending') {
            trackPending(outcome, revert.minutes, 'rollback', changes ?? []);
            close();
          } else setResult(outcome.result);
        },
        onError: (e) => {
          // no answer is not "failed": the server may have finished it (TD-10a, review 2.4a)
          if (!isUnreachable(e)) return;
          setLost('looking');
          void lookupOutcome(sentAt, runningRev).then((o) => {
            setLost(o);
            if (o !== 'lookup-failed' && o.kind === 'pending') {
              const deadlineMs = win.enabled
                ? Math.min(o.deadlineMs, sentAt + win.minutes * 60_000)
                : o.deadlineMs;
              confirmStore.track({
                txnId: o.txnId,
                deadlineMs,
                kind: 'rollback',
                trackedAt: Date.now(),
              });
            }
          });
        },
      },
    );
  };
  const settled =
    lost !== null &&
    lost !== 'looking' &&
    lost !== 'lookup-failed' &&
    (lost.kind === 'applied' || lost.kind === 'pending');
  return (
    <Dialog
      open
      onClose={rollback.isPending ? undefined : close}
      maxWidth="md"
      fullWidth
      aria-labelledby="rollback-title"
    >
      <DialogTitle id="rollback-title">{t('rollback.title', { id: target.id })}</DialogTitle>
      <DialogContent dividers>
        {result ? (
          <CommitResultView result={result} />
        ) : (
          <Stack gap={2}>
            <DialogContentText>{t('rollback.body', { id: target.id })}</DialogContentText>
            <Typography component="h3" variant="subtitle2">
              {t('rollback.willChange')}
            </Typography>
            {running.error || rev.error ? (
              <ProblemAlert error={running.error ?? rev.error} />
            ) : changes === null ? (
              <LinearProgress aria-label={t('loading')} />
            ) : (
              <>
                <DiffView changes={changes} />
                {changes.length === 0 && (
                  <Alert severity="info">{t('rollback.noVisibleChange')}</Alert>
                )}
              </>
            )}
            <TextField
              label={t('config:commit.comment')}
              placeholder={t('rollback.defaultComment', { id: target.id })}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              slotProps={{ htmlInput: { maxLength: 1024 } }}
            />
            <ConfirmWindowField value={revert} onChange={setRevert} />
            {lost ? (
              <LostAnswerAlert lost={lost} />
            ) : (
              rollback.isError && <ProblemAlert error={rollback.error} />
            )}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        {result || settled ? (
          <Button variant="contained" onClick={close}>
            {t('config:close')}
          </Button>
        ) : (
          <>
            <Button onClick={close} disabled={rollback.isPending}>
              {t('config:cancel')}
            </Button>
            <Button
              variant="contained"
              color="warning"
              onClick={submit}
              disabled={
                rollback.isPending ||
                lost === 'looking' ||
                !confirmWindowValid(revert) ||
                changes === null
              }
            >
              {rollback.isPending ? t('rollback.running') : t('rollback.submit', { id: target.id })}
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}

/** System › Revisions (P07 §8): history, diff between any two, rollback with confirmation (and auto-revert window). */
export function RevisionsPage() {
  const { t } = useTranslation(['revisions', 'config', 'common']);
  const fmt = useFormatters();
  const perms = usePermissions();
  const eff = useEffectiveChanges();
  const candidate = eff.diff;
  const pending = usePending();
  const [cmp, setCmp] = useState<{ from: number; to: number } | null>(null);
  const [fromInput, setFromInput] = useState('');
  const [toInput, setToInput] = useState('');
  const [target, setTarget] = useState<RevisionMeta | null>(null);

  // same rules as the bar: hidden write-only edits count as dirty (H1), a stale lock does not block (L2)
  const dirty = eff.dirty;
  const lockedByOther = eff.lockedByOther;
  const runningRev = candidate.data?.baseRevision ?? null;
  const rollbackBlocked = !perms.commit
    ? t('config:perm.commit')
    : pending.data?.pending
      ? t('config:bar.pendingBlocks')
      : dirty
        ? t('rollback.dirty')
        : lockedByOther
          ? t('config:bar.lockedByOther', { owner: eff.lock?.owner })
          : undefined;

  const columns = useMemo<GridColDef<RevisionMeta>[]>(
    () => [
      {
        field: 'id',
        headerName: t('col.id'),
        width: 90,
        sortable: false,
        renderCell: ({ row }) => (
          <Stack direction="row" gap={0.5} alignItems="center" sx={{ blockSize: '100%' }}>
            <span>{fmt.integer(row.id)}</span>
            {row.id === runningRev && <Chip size="small" color="success" label={t('running')} />}
          </Stack>
        ),
      },
      {
        field: 'createdAt',
        headerName: t('col.createdAt'),
        width: 190,
        sortable: false,
        valueFormatter: (v: string) => fmt.dateTime(v),
      },
      {
        field: 'author',
        headerName: t('col.author'),
        width: 120,
        sortable: false,
        valueFormatter: (v: string | null) => v ?? t('col.system'),
      },
      {
        field: 'kind',
        headerName: t('col.kind'),
        width: 110,
        sortable: false,
        renderCell: ({ value }) => (
          <Chip
            size="small"
            variant="outlined"
            label={t(`kind.${String(value)}`, { defaultValue: String(value) })}
          />
        ),
      },
      { field: 'comment', headerName: t('col.comment'), flex: 1, minWidth: 160, sortable: false },
      {
        field: 'actions',
        headerName: t('col.actions'),
        width: 250,
        sortable: false,
        renderCell: ({ row }) => (
          <Stack direction="row" gap={1} alignItems="center" sx={{ blockSize: '100%' }}>
            <Button
              size="small"
              startIcon={<CompareArrowsIcon />}
              onClick={() => setCmp({ from: row.parentId ?? 0, to: row.id })}
            >
              {t('changes')}
            </Button>
            <Why reason={row.id === runningRev ? t('rollback.isRunning') : rollbackBlocked}>
              <Button
                size="small"
                color="warning"
                startIcon={<HistoryIcon />}
                onClick={() => setTarget(row)}
                disabled={row.id === runningRev || rollbackBlocked !== undefined}
              >
                {t('rollback.button')}
              </Button>
            </Why>
          </Stack>
        ),
      },
    ],
    [t, fmt, runningRev, rollbackBlocked],
  );

  const parsedFrom = Number.parseInt(fromInput, 10);
  const parsedTo = Number.parseInt(toInput, 10);
  const canCompare =
    Number.isInteger(parsedFrom) &&
    Number.isInteger(parsedTo) &&
    parsedFrom >= 1 &&
    parsedTo >= 1 &&
    parsedFrom !== parsedTo;

  return (
    <PageHeader title={t('title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('intro')}
      </Typography>
      {!perms.commit && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('readonlyNote', { role: t(`auth:role.${perms.role ?? 'readonly'}`) })}
        </Alert>
      )}
      <Box sx={{ blockSize: 420, mb: 2 }}>
        <ServerDataGrid<RevisionMeta>
          queryKey={[...qk.config, 'revisions', 'grid']}
          fetchPage={fetchRevisions}
          columns={columns}
          initialPageSize={10}
          pageSizeOptions={[10, 25, 50]}
          disableColumnFilter
          disableColumnSorting
          disableRowSelectionOnClick
          getRowId={(r) => r.id}
          aria-label={t('title')}
        />
      </Box>
      <Stack
        component="form"
        direction="row"
        gap={1}
        alignItems="center"
        flexWrap="wrap"
        sx={{ mb: 2 }}
        onSubmit={(e) => {
          e.preventDefault();
          if (canCompare) setCmp({ from: parsedFrom, to: parsedTo });
        }}
      >
        <Typography>{t('compare.label')}</Typography>
        <TextField
          size="small"
          type="number"
          label={t('compare.from')}
          value={fromInput}
          onChange={(e) => setFromInput(e.target.value)}
          sx={{ inlineSize: 120 }}
          slotProps={{ htmlInput: REV_INPUT }}
        />
        <TextField
          size="small"
          type="number"
          label={t('compare.to')}
          value={toInput}
          onChange={(e) => setToInput(e.target.value)}
          sx={{ inlineSize: 120 }}
          slotProps={{ htmlInput: REV_INPUT }}
        />
        <Button
          type="submit"
          variant="outlined"
          disabled={!canCompare}
          startIcon={<CompareArrowsIcon />}
        >
          {t('compare.submit')}
        </Button>
      </Stack>
      {cmp && <RevisionDiff from={cmp.from} to={cmp.to} />}
      <RollbackDialog target={target} runningRev={runningRev} onClose={() => setTarget(null)} />
    </PageHeader>
  );
}
