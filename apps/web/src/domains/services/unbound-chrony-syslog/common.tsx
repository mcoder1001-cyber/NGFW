import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import type { UseQueryResult } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { NS } from './model';

export interface PendingAction {
  daemon: string;
  unit: string;
  action: string;
  reason: string;
}

/** The daemon line of a panel: running / not running, when it was read, a Refresh button (D-132: no fast timers). */
export function DaemonStatus({
  daemon,
  running,
  query,
  configPath,
}: {
  daemon: string;
  running: boolean | undefined;
  query: Pick<UseQueryResult, 'refetch' | 'isFetching' | 'dataUpdatedAt'>;
  configPath: string | undefined;
}) {
  const { t } = useTranslation(NS);
  return (
    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', flexWrap: 'wrap', mb: 1 }}>
      <Typography variant="subtitle1" component="span">
        {daemon}
      </Typography>
      {running !== undefined && (
        <Chip
          size="small"
          color={running ? 'success' : 'default'}
          label={t(running ? 'status.running' : 'status.stopped')}
        />
      )}
      {query.dataUpdatedAt > 0 && (
        <Typography variant="caption" color="text.secondary">
          {t('status.readAt', { time: new Date(query.dataUpdatedAt).toLocaleTimeString() })}
        </Typography>
      )}
      {configPath && (
        <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
          {configPath}
        </Typography>
      )}
      <Button
        size="small"
        startIcon={<RefreshIcon />}
        onClick={() => void query.refetch()}
        disabled={query.isFetching}
      >
        {t('refresh')}
      </Button>
    </Stack>
  );
}

/** Start/restart requests the renderer recorded (D-079): the file is written, the daemon applies it only after this. */
export function PendingActions({ actions }: { actions: readonly PendingAction[] | undefined }) {
  const { t } = useTranslation(NS);
  if (!actions || actions.length === 0) return null;
  return (
    <Alert severity="warning" sx={{ mb: 2 }}>
      <AlertTitle>{t('pending.title')}</AlertTitle>
      {actions.map((a) => (
        <div key={`${a.daemon}-${a.action}`}>
          {t('pending.line', {
            daemon: a.daemon,
            action: t(`pending.action.${a.action}`, { defaultValue: a.action }),
            unit: a.unit,
          })}
          {' — '}
          {a.reason}
        </div>
      ))}
    </Alert>
  );
}

/** A state read that failed (agent unreachable, old agent → 501) or a partial read reported by the agent. */
export function StateProblem({ error, partial }: { error: unknown; partial?: string | undefined }) {
  const { t } = useTranslation(NS);
  return (
    <>
      {error ? <ProblemAlert error={error} /> : null}
      {partial ? (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('status.partial', { detail: partial })}
        </Alert>
      ) : null}
    </>
  );
}
