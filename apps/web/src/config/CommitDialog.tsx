import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Checkbox from '@mui/material/Checkbox';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogTitle from '@mui/material/DialogTitle';
import Divider from '@mui/material/Divider';
import FormControlLabel from '@mui/material/FormControlLabel';
import LinearProgress from '@mui/material/LinearProgress';
import Stack from '@mui/material/Stack';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { useEffect, useId, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ApiError, serverOffsetMs } from '../api-problem';
import { confirmStore } from './confirm-store';
import { CommitResultView } from './CommitResultView';
import { DiffView, type DiffChange } from './DiffView';
import { ProblemAlert } from './ProblemAlert';
import { useDomainList } from './useDomainList';
import { useCommit, useValidate, type CommitOutcome, type CommitResult } from './queries';

/** Numbers and technical values stay left-to-right in RTL layouts (review P07a L7). */
export const LTR = { dir: 'ltr' } as const;

export const DEFAULT_CONFIRM_MINUTES = 2;
export const MAX_CONFIRM_MINUTES = 60; // P06 accepts ?confirm up to 3600 s

export interface ConfirmWindow {
  enabled: boolean;
  minutes: number;
}

/** "Revert automatically in [N] minutes unless I confirm" — default ON (docs/05). */
export function ConfirmWindowField({ value, onChange }: { value: ConfirmWindow; onChange: (v: ConfirmWindow) => void }) {
  const { t } = useTranslation('config');
  const id = useId();
  const invalid = !Number.isInteger(value.minutes) || value.minutes < 1 || value.minutes > MAX_CONFIRM_MINUTES;
  return (
    <Stack direction="row" alignItems="center" gap={1} flexWrap="wrap">
      <FormControlLabel
        sx={{ marginInlineEnd: 0 }}
        control={<Checkbox checked={value.enabled} onChange={(e) => onChange({ ...value, enabled: e.target.checked })} />}
        label={t('commit.autoRevertBefore')}
      />
      <TextField
        id={id}
        type="number"
        size="small"
        value={Number.isNaN(value.minutes) ? '' : value.minutes}
        disabled={!value.enabled}
        onChange={(e) => onChange({ ...value, minutes: Number.parseInt(e.target.value, 10) })}
        error={value.enabled && invalid}
        helperText={value.enabled && invalid ? t('commit.minutesRange', { max: MAX_CONFIRM_MINUTES }) : undefined}
        slotProps={{ htmlInput: { min: 1, max: MAX_CONFIRM_MINUTES, 'aria-label': t('commit.minutesLabel'), ...LTR } }}
        sx={{ inlineSize: 88 }}
      />
      <Typography component="span">{t('commit.autoRevertAfter')}</Typography>
    </Stack>
  );
}

export function confirmWindowValid(w: ConfirmWindow): boolean {
  return !w.enabled || (Number.isInteger(w.minutes) && w.minutes >= 1 && w.minutes <= MAX_CONFIRM_MINUTES);
}

/** Start the local countdown for a commit/rollback the server holds for confirmation. */
export function trackPending(outcome: CommitOutcome, minutes: number, kind: 'commit' | 'rollback'): void {
  const { result, sentAt, response } = outcome;
  if (result.status !== 'pending' || !result.txnId) return;
  let deadlineMs = sentAt + minutes * 60_000;
  if (result.confirmDeadline) deadlineMs = Math.min(deadlineMs, Date.parse(result.confirmDeadline) - serverOffsetMs(response, sentAt));
  confirmStore.track({ txnId: result.txnId, deadlineMs, kind, trackedAt: Date.now() });
}

/**
 * Commit dialog (P07 §7): structured diff, comment, auto-revert window (default ON), a validation pass on open that
 * shows errors with pointers, warnings and the domains the agent will not apply. After a confirmed commit the dialog
 * closes and the countdown banner takes over; any other answer is shown here as the server reported it.
 */
export function CommitDialog({ open, onClose, changes }: { open: boolean; onClose: () => void; changes: readonly DiffChange[] }) {
  const { t } = useTranslation('config');
  const [comment, setComment] = useState('');
  const [revert, setRevert] = useState<ConfirmWindow>({ enabled: true, minutes: DEFAULT_CONFIRM_MINUTES });
  const [result, setResult] = useState<CommitResult | null>(null);
  const domainList = useDomainList();
  const validate = useValidate();
  const commit = useCommit();
  const { mutate: runValidate, reset: resetValidate } = validate;
  const { reset: resetCommit } = commit;

  useEffect(() => {
    if (!open) return;
    setResult(null);
    resetCommit();
    runValidate();
    return () => resetValidate();
  }, [open, runValidate, resetValidate, resetCommit]);

  const validationFailed = validate.error instanceof ApiError && validate.error.status === 400;
  const onCommit = () => {
    const minutes = revert.minutes;
    commit.mutate(
      { comment, confirmSec: revert.enabled ? minutes * 60 : undefined },
      {
        onSuccess: (outcome) => {
          if (outcome.result.status === 'pending') {
            trackPending(outcome, minutes, 'commit');
            setComment('');
            onClose();
          } else {
            setResult(outcome.result);
            setComment('');
          }
        },
      },
    );
  };

  return (
    <Dialog open={open} onClose={commit.isPending ? undefined : onClose} maxWidth="md" fullWidth aria-labelledby="commit-title">
      <DialogTitle id="commit-title">{result ? t('commit.doneTitle') : t('commit.title')}</DialogTitle>
      <DialogContent dividers>
        {result ? (
          <CommitResultView result={result} />
        ) : (
          <Stack gap={2}>
            <DiffView changes={changes} />
            <Divider />
            <Box aria-live="polite">
              {validate.isPending && (
                <Stack gap={0.5}>
                  <Typography variant="body2">{t('commit.validating')}</Typography>
                  <LinearProgress aria-label={t('commit.validating')} />
                </Stack>
              )}
              {validate.isSuccess && (
                <Alert severity={validate.data.notApplied.length > 0 || validate.data.warnings.length > 0 ? 'warning' : 'success'} data-testid="validate-ok">
                  {t('commit.valid')}
                  {validate.data.notApplied.length > 0 && (
                    <Box sx={{ mt: 0.5 }}>{t('commit.notAppliedWarning', { count: validate.data.notApplied.length, domains: domainList(validate.data.notApplied) })}</Box>
                  )}
                  {validate.data.warnings.map((w, i) => (
                    <Box key={`${w.pointer}:${i}`} sx={{ mt: 0.5 }} dir="auto">
                      <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                        {w.pointer || '/'}
                      </Box>
                      {' — '}
                      {w.message}
                    </Box>
                  ))}
                </Alert>
              )}
              {validate.isError && <ProblemAlert error={validate.error} />}
            </Box>
            <TextField
              label={t('commit.comment')}
              value={comment}
              onChange={(e) => setComment(e.target.value)}
              multiline
              minRows={2}
              slotProps={{ htmlInput: { maxLength: 1024 } }}
            />
            <ConfirmWindowField value={revert} onChange={setRevert} />
            {commit.isError && <ProblemAlert error={commit.error} />}
          </Stack>
        )}
      </DialogContent>
      <DialogActions>
        {result ? (
          <Button onClick={onClose} variant="contained">
            {t('close')}
          </Button>
        ) : (
          <>
            <Button onClick={onClose} disabled={commit.isPending}>
              {t('cancel')}
            </Button>
            <Button
              variant="contained"
              onClick={onCommit}
              disabled={commit.isPending || validate.isPending || validationFailed || !confirmWindowValid(revert) || changes.length === 0}
            >
              {commit.isPending ? t('commit.committing') : revert.enabled ? t('commit.submitConfirm') : t('commit.submit')}
            </Button>
          </>
        )}
      </DialogActions>
    </Dialog>
  );
}
