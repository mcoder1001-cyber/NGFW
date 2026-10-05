import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';
import { usePermissions } from '../../../auth/AuthProvider';
const key = ['state', 'ha', 'sync'] as const;
export function StateSyncPanel() {
  const { t } = useTranslation('ha-state-sync');
  const permissions = usePermissions();
  const qc = useQueryClient();
  const state = useQuery({
    queryKey: key,
    queryFn: async ({ signal }) => call(api.GET('/api/v1/state/ha/sync', { signal })),
    refetchInterval: 30_000,
  });
  const action = useMutation({
    mutationFn: () => call(api.POST('/api/v1/actions/ha/sync/resync')),
    onSettled: () => qc.invalidateQueries({ queryKey: key }),
  });
  const observationUnavailable =
    state.isPending || state.isError || !!state.data?.data.observationError;
  const data = observationUnavailable ? undefined : state.data?.data;
  return (
    <Paper variant="outlined" sx={{ p: 2 }}>
      <Stack spacing={2}>
        <Typography variant="h6">{t('title')}</Typography>
        <Alert severity="warning">{t('security')}</Alert>
        {state.isPending && <Typography>{t('loading')}</Typography>}
        {state.error && <Alert severity="error">{t('failed')}</Alert>}
        {state.data?.data.observationError && (
          <Alert severity="warning">{state.data.data.observationError}</Alert>
        )}
        {observationUnavailable && state.data && <Alert severity="warning">{t('stale')}</Alert>}
        {data?.kinds.map((k) => (
          <Stack key={k.kind} spacing={1}>
            <Typography>{t(k.kind)}</Typography>
            <Stack direction="row" spacing={1}>
              <Chip
                label={t(k.supported ? 'supported' : 'unsupported')}
                color={k.supported ? 'success' : 'warning'}
              />
              <Chip label={t(k.active ? 'active' : 'inactive')} />
              {k.configured && <Chip label={t('configured')} />}
            </Stack>
            <Typography variant="body2">{t(`reason-${k.kind}`)}</Typography>
          </Stack>
        ))}
        <Typography>
          {t('last')}: {data?.lastResync ?? t('unavailable')}
        </Typography>
        <Typography>
          {t('completed')}: {data?.resyncCount ?? t('unavailable')}
        </Typography>
        <Typography>
          {t('missed')}: {data?.lastMissedCount ?? t('unavailable')}
        </Typography>
        <Typography>{t('counters')}</Typography>
        {action.error && <Alert severity="error">{action.error.message}</Alert>}
        <Button
          disabled={
            observationUnavailable ||
            state.isFetching ||
            permissions.role !== 'admin' ||
            !data?.actionsAllowed ||
            !data.kinds.some((k) => k.kind === 'nat44-ei' && k.active) ||
            action.isPending
          }
          onClick={() => action.mutate()}
        >
          {t('resync')}
        </Button>
      </Stack>
    </Paper>
  );
}
