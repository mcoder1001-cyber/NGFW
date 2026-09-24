import WarningAmberIcon from '@mui/icons-material/WarningAmber';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useTopic } from '@ngfw/ui-kit/ws';
import { useState, type ReactElement } from 'react';
import { useTranslation } from 'react-i18next';
import { isUnreachable } from '../api-problem';
import { useAuth, usePermissions } from '../auth/AuthProvider';
import { CommitDialog } from './CommitDialog';
import { DiffView } from './DiffView';
import { ProblemAlert } from './ProblemAlert';
import { refineChanges } from './refine';
import { invalidateConfig, useBreakLock, useDiff, useDiscard, useLock, usePending, useSystemState } from './queries';

/** A disabled button still explains itself (tooltip on a wrapper, since disabled elements get no pointer events). */
function Why({ reason, children }: { reason: string | undefined; children: ReactElement }) {
  if (!reason) return children;
  return (
    <Tooltip title={reason}>
      <span>{children}</span>
    </Tooltip>
  );
}

/** Keeps the bar live: every `commit.events` message (commit, confirm, revert, failure) refetches the config queries. */
function useCommitEvents() {
  const qc = useQueryClient();
  useTopic('commit.events', {
    onBatch: () => {
      void invalidateConfig(qc);
    },
  });
}

/**
 * The product's signature UX (docs/05): appears the moment the candidate differs from running. Count, lock owner,
 * Review / Commit… / Discard. Polls `/config/diff` + `/config/lock` and refetches on `commit.events` (WS).
 */
export function PendingChangeBar() {
  const { t } = useTranslation('config');
  const fmt = useFormatters();
  const { state } = useAuth();
  const perms = usePermissions();
  const diff = useDiff();
  const lock = useLock();
  const pending = usePending();
  const discard = useDiscard();
  const breakLock = useBreakLock();
  const [review, setReview] = useState(false);
  const [committing, setCommitting] = useState(false);
  const [discarding, setDiscarding] = useState(false);
  useCommitEvents();

  const changes = diff.data?.changes ?? [];
  const shown = refineChanges(changes).length;
  const reconnecting = diff.isError && isUnreachable(diff.error);
  if (changes.length === 0 && !committing) {
    return reconnecting ? (
      <Chip color="warning" size="small" label={t('bar.reconnecting')} sx={{ mb: 2 }} role="status" />
    ) : null;
  }

  const me = state.user?.username;
  const owner = lock.data?.locked ? lock.data.owner : null;
  const lockedByOther = owner !== null && owner !== me && (lock.data?.expiresAt ? Date.parse(lock.data.expiresAt) > Date.now() : true);
  const pendingCommit = pending.data?.pending ?? null;

  const commitBlocked = !perms.commit
    ? t('perm.commit')
    : lockedByOther
      ? t('bar.lockedByOther', { owner })
      : pendingCommit
        ? t('bar.pendingBlocks')
        : undefined;
  const discardBlocked = !perms.editConfig ? t('perm.edit') : lockedByOther ? t('bar.lockedByOther', { owner }) : undefined;

  return (
    <>
      <Paper
        elevation={3}
        role="region"
        aria-label={t('bar.label')}
        data-testid="pending-bar"
        sx={{
          position: 'sticky',
          top: { xs: 56, sm: 64 },
          zIndex: (th) => th.zIndex.appBar - 1,
          mb: 2,
          px: 2,
          py: 1,
          borderInlineStart: 4,
          borderColor: 'warning.main',
        }}
      >
        <Stack direction="row" alignItems="center" gap={1.5} flexWrap="wrap">
          <WarningAmberIcon color="warning" aria-hidden />
          <Typography sx={{ fontWeight: 600 }} data-testid="pending-count">
            {t('bar.count', { count: shown, n: fmt.integer(shown) })}
          </Typography>
          <Box sx={{ flexGrow: 1 }} />
          <Button size="small" variant="outlined" onClick={() => setReview(true)}>
            {t('bar.review')}
          </Button>
          <Why reason={commitBlocked}>
            <Button size="small" variant="contained" color="warning" onClick={() => setCommitting(true)} disabled={commitBlocked !== undefined}>
              {t('bar.commit')}
            </Button>
          </Why>
          <Why reason={discardBlocked}>
            <Button size="small" color="inherit" onClick={() => setDiscarding(true)} disabled={discardBlocked !== undefined}>
              {t('bar.discard')}
            </Button>
          </Why>
          {owner && (
            <Chip
              size="small"
              variant="outlined"
              label={owner === me ? t('bar.lockedByYou') : t('bar.lockedBy', { owner })}
              data-testid="lock-owner"
            />
          )}
          {lockedByOther && perms.breakLock && (
            <Button size="small" color="error" onClick={() => breakLock.mutate()} disabled={breakLock.isPending}>
              {t('bar.breakLock')}
            </Button>
          )}
          {reconnecting && <Chip color="warning" size="small" label={t('bar.reconnecting')} role="status" />}
        </Stack>
        {breakLock.isError && <ProblemAlert error={breakLock.error} sx={{ mt: 1 }} />}
      </Paper>

      <Dialog open={review} onClose={() => setReview(false)} maxWidth="md" fullWidth aria-labelledby="review-title">
        <DialogTitle id="review-title">{t('review.title')}</DialogTitle>
        <DialogContent dividers>
          <DiffView changes={changes} />
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setReview(false)}>{t('close')}</Button>
          <Why reason={commitBlocked}>
            <Button
              variant="contained"
              onClick={() => {
                setReview(false);
                setCommitting(true);
              }}
              disabled={commitBlocked !== undefined}
            >
              {t('bar.commit')}
            </Button>
          </Why>
        </DialogActions>
      </Dialog>

      <CommitDialog open={committing} onClose={() => setCommitting(false)} changes={changes} />

      <Dialog open={discarding} onClose={() => setDiscarding(false)} aria-labelledby="discard-title">
        <DialogTitle id="discard-title">{t('discard.title')}</DialogTitle>
        <DialogContent>
          <DialogContentText>{t('discard.body', { count: shown, n: fmt.integer(shown) })}</DialogContentText>
          {discard.isError && <ProblemAlert error={discard.error} sx={{ mt: 1 }} />}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setDiscarding(false)}>{t('cancel')}</Button>
          <Button
            color="error"
            variant="contained"
            disabled={discard.isPending}
            onClick={() => discard.mutate(undefined, { onSuccess: () => setDiscarding(false) })}
          >
            {t('discard.confirm')}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
}

/**
 * Honest data-plane status (D-P06-14): running vs data plane `unknown`/`degraded`, or the agent not answering. Shown on
 * every page because a commit made now would fail or leave the box in an unknown state.
 */
export function SyncBanner() {
  const { t } = useTranslation('config');
  const sys = useSystemState();
  if (!sys.data) return null;
  const { sync, agent } = sys.data;
  const agentDown = agent['reachable'] === false;
  if (sync.state === 'in-sync' && !agentDown) return null;
  return (
    <Stack gap={1} sx={{ mb: 2 }} data-testid="sync-banner">
      {sync.state !== 'in-sync' && (
        <Alert severity="error">
          <AlertTitle>{t(`sync.title.${sync.state === 'degraded' ? 'degraded' : 'unknown'}`)}</AlertTitle>
          {t('sync.body', { reason: sync.reason })}
        </Alert>
      )}
      {agentDown && (
        <Alert severity="warning">
          <AlertTitle>{t('sync.agentDownTitle')}</AlertTitle>
          {t('sync.agentDown')}
        </Alert>
      )}
    </Stack>
  );
}

