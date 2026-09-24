import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import LinearProgress from '@mui/material/LinearProgress';
import { useFormatters } from '@ngfw/ui-kit';
import { useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { api } from '../api';
import { ApiError, call, isUnreachable, serverOffsetMs } from '../api-problem';
import { usePermissions } from '../auth/AuthProvider';
import { useNow } from '../hooks/useNow';
import { confirmStore, formatCountdown, useConfirmState } from './confirm-store';
import { useConfirm, usePending } from './queries';
import { ProblemAlert } from './ProblemAlert';

/** Which revision (if any) was written for `txnId`: revisions are persisted only after CONFIRMED (D-P06-5). */
async function revisionOf(txnId: string): Promise<number | undefined> {
  const { data } = await call(api.GET('/api/v1/config/revisions', { params: { query: { limit: 10, offset: 0 } } }));
  return data.items.find((r) => r.txnId === txnId)?.id;
}

/**
 * Countdown banner of a confirmed commit (P07 §7, docs/05). The server's pending commit is authoritative while it
 * answers; when it stops answering the countdown continues locally with "reconnecting…" — expected when the commit cut
 * the operator's own access. When the window closes the outcome is read back (revision written = confirmed, none =
 * reverted), never assumed.
 */
export function ConfirmBanner() {
  const { t } = useTranslation('config');
  const fmt = useFormatters();
  const perms = usePermissions();
  const pendingQ = usePending();
  const { tracked, outcome } = useConfirmState();
  const confirm = useConfirm();
  const resolving = useRef<string | null>(null);

  const answered = pendingQ.isSuccess && !pendingQ.isError;
  const serverPending = answered ? pendingQ.data.pending : null;
  const unreachable = pendingQ.isError && isUnreachable(pendingQ.error);
  const active = serverPending !== null || (tracked !== null && (!answered || pendingQ.dataUpdatedAt < tracked.trackedAt));
  const now = useNow(active ? 1000 : null);

  // Local deadline: the server's (clock offset from the `Date` header) and ours, whichever is earlier.
  let deadline: number | null = null;
  if (serverPending && pendingQ.data) {
    deadline = Date.parse(serverPending.deadline) - serverOffsetMs(pendingQ.data.response, pendingQ.data.t0);
    if (tracked?.txnId === serverPending.txnId) deadline = Math.min(deadline, tracked.deadlineMs);
  } else if (tracked) {
    deadline = tracked.deadlineMs;
  }
  const mine = tracked !== null && (serverPending === null || serverPending.txnId === tracked.txnId);

  useEffect(() => {
    if (serverPending && tracked?.txnId === serverPending.txnId && deadline !== null) confirmStore.adjustDeadline(tracked.txnId, deadline);
  }, [serverPending, tracked, deadline]);

  // The server answered "nothing pending" after we started tracking: find out what happened.
  useEffect(() => {
    if (!tracked || !answered || pendingQ.data.pending !== null || pendingQ.dataUpdatedAt < tracked.trackedAt) return;
    if (resolving.current === tracked.txnId) return;
    resolving.current = tracked.txnId;
    void revisionOf(tracked.txnId).then(
      (rev) => confirmStore.resolve(rev === undefined ? 'reverted' : 'confirmedElsewhere', tracked.txnId, rev),
      () => {
        resolving.current = null; // try again on the next poll
      },
    );
  }, [tracked, answered, pendingQ.data, pendingQ.dataUpdatedAt]);

  const onConfirm = () => {
    const txnId = serverPending?.txnId ?? tracked?.txnId;
    confirm.mutate(undefined, {
      onSuccess: (r) => {
        if (txnId) confirmStore.resolve('confirmed', txnId, r.revision?.id);
      },
      onError: (e) => {
        if (txnId && e instanceof ApiError && e.slug === 'commit-reverted') confirmStore.resolve('reverted', txnId);
      },
    });
  };

  if (!active && outcome) {
    const sev = outcome.kind === 'reverted' ? 'warning' : 'success';
    return (
      <Alert severity={sev} onClose={() => confirmStore.dismiss()} sx={{ mb: 2 }} data-testid="confirm-outcome">
        <AlertTitle>{t(`confirm.outcome.${outcome.kind}.title`)}</AlertTitle>
        {t(`confirm.outcome.${outcome.kind}.body`, { revision: fmt.integer(outcome.revision ?? 0) })}
      </Alert>
    );
  }
  if (!active || deadline === null) return null;

  const remaining = deadline - now;
  const time = fmt.digits(formatCountdown(remaining));
  const expired = remaining <= 0;

  return (
    <Box sx={{ mb: 2 }} data-testid="confirm-banner">
      <Alert
        severity={unreachable ? 'error' : 'warning'}
        variant="filled"
        action={
          !expired && (
            <Button
              color="inherit"
              variant="outlined"
              size="small"
              onClick={onConfirm}
              disabled={!perms.commit || confirm.isPending || unreachable}
              title={!perms.commit ? t('perm.commit') : undefined}
            >
              {confirm.isPending ? t('confirm.confirming') : t('confirm.button')}
            </Button>
          )
        }
      >
        <AlertTitle>
          {unreachable ? t('confirm.reconnecting') : mine ? t('confirm.titleMine') : t('confirm.titleOther')}
        </AlertTitle>
        <span aria-live="off">
          {expired
            ? unreachable
              ? t('confirm.expiredWaiting')
              : t('confirm.expiredChecking')
            : t('confirm.countdown', { time })}
        </span>{' '}
        {!expired && t('confirm.expected')}
        {serverPending?.comment ? (
          <Box component="span" sx={{ display: 'block', mt: 0.5 }}>
            {t('confirm.comment', { comment: serverPending.comment })}
          </Box>
        ) : null}
      </Alert>
      {unreachable && <LinearProgress color="error" aria-label={t('confirm.reconnecting')} />}
      {confirm.error && !(confirm.error instanceof ApiError && confirm.error.slug === 'commit-reverted') && (
        <ProblemAlert error={confirm.error} sx={{ mt: 1 }} />
      )}
    </Box>
  );
}
