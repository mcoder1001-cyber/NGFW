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
import { CommitResultView } from './CommitResultView';
import { POLL_MS, useConfirm, usePending } from './queries';
import { useDomainList } from './useDomainList';
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
export function ConfirmBanner({ offline = false }: { offline?: boolean } = {}) {
  const { t } = useTranslation('config');
  const fmt = useFormatters();
  const perms = usePermissions();
  const pendingQ = usePending();
  const { tracked, outcome } = useConfirmState();
  const confirm = useConfirm();
  const resolving = useRef<string | null>(null);
  /** txn this session is confirming right now: its own answer decides the outcome, not the poll. */
  const confirming = useRef<string | null>(null);

  const answered = pendingQ.isSuccess && !pendingQ.isError;
  // the last answer stays authoritative while the device does not answer (TanStack keeps `data` on a failed refetch)
  const serverPending = pendingQ.data?.pending ?? null;
  const domainList = useDomainList();
  // Unreachable = the last poll failed at the network level, OR no successful answer for 3 poll periods — a silently
  // dropped route can leave `isError` false for a while (review M3); `offline` = the page was reloaded during an outage.
  const lastOk = pendingQ.dataUpdatedAt;
  const unreachable =
    offline || (pendingQ.isError && isUnreachable(pendingQ.error)) || (lastOk > 0 && Date.now() - lastOk > 3 * POLL_MS);
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
    if (resolving.current === tracked.txnId || confirming.current === tracked.txnId) return;
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
    confirming.current = txnId ?? null;
    confirm.mutate(undefined, {
      onSuccess: (r) => {
        if (txnId) confirmStore.resolve('confirmed', txnId, r.revision?.id);
      },
      onError: (e) => {
        confirming.current = null;
        if (txnId && e instanceof ApiError && e.slug === 'commit-reverted') confirmStore.resolve('reverted', txnId);
      },
    });
  };

  if (!active && outcome) {
    const sev = outcome.kind === 'reverted' ? 'warning' : 'success';
    return (
      <Box sx={{ mb: 2 }} data-testid="confirm-outcome">
        <Alert severity={sev} onClose={() => confirmStore.dismiss()}>
          <AlertTitle>{t(`confirm.outcome.${outcome.kind}.title`)}</AlertTitle>
          {t(`confirm.outcome.${outcome.kind}.body`, { revision: fmt.integer(outcome.revision ?? 0) })}
        </Alert>
        {/* what the agent actually did when it applied it (review M1) — never just "confirmed" */}
        {outcome.kind !== 'reverted' && outcome.summary && (
          <Box sx={{ mt: 1 }}>
            <CommitResultView
              title={t('confirm.applyResult')}
              result={{ ...outcome.summary, txnId: outcome.txnId, revision: outcome.revision ? { id: outcome.revision } : undefined }}
            />
          </Box>
        )}
      </Box>
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
        {mine && tracked?.summary && tracked.summary.notApplied.length > 0 && (
          <Box component="span" sx={{ display: 'block', mt: 0.5 }} data-testid="confirm-not-applied">
            {t('confirm.notApplied', { count: tracked.summary.notApplied.length, domains: domainList(tracked.summary.notApplied) })}
          </Box>
        )}
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
