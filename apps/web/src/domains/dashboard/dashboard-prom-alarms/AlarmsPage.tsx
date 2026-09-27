import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { useSearchParams } from 'react-router';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { useAckAlarm, useAlarms, type Alarm } from './queries';

const ALARMS_KEY = ['state', 'alarms'];

const SEV_COLOR: Record<string, 'error' | 'warning' | 'info' | 'default'> = {
  critical: 'error',
  warning: 'warning',
  info: 'info',
};

type TabKey = 'active' | 'all';
const ACTIVE: TabKey = 'active';
const ALL: TabKey = 'all';
function isTab(v: string | null): v is TabKey {
  return v === 'active' || v === 'all';
}

/** F-dashboard-prom-alarms: the alarms list with acknowledge. Rules and targets are edited under Config (management). */
export function AlarmsPage() {
  const { t } = useTranslation('dashboard-prom-alarms');
  const fmt = useFormatters();
  const perms = usePermissions();
  const qc = useQueryClient();
  const [params, setParams] = useSearchParams();
  const tab: TabKey = isTab(params.get('tab')) ? (params.get('tab') as TabKey) : 'active';
  const q = useAlarms(tab === 'active' ? true : undefined);
  const ack = useAckAlarm();
  const [error, setError] = useState<unknown>(null);

  const doAck = async (id: number) => {
    setError(null);
    try {
      await ack.mutateAsync(id);
    } catch (e) {
      setError(e);
    }
  };

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>
      <Stack direction="row" alignItems="center" sx={{ mb: 1 }}>
        <Tabs
          value={tab}
          onChange={(_e, v: TabKey) => setParams({ tab: v })}
          sx={{ flex: 1 }}
          aria-label={t('tabsLabel')}
        >
          <Tab value={ACTIVE} label={t('tabs.active')} />
          <Tab value={ALL} label={t('tabs.all')} />
        </Tabs>
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: ALARMS_KEY })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {error !== null && <ProblemAlert error={error} sx={{ mb: 1 }} />}
      {q.data && q.data.items.length === 0 && <Alert severity="success">{t('empty')}</Alert>}
      {q.data && q.data.items.length > 0 && (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('col.severity')}</TableCell>
                <TableCell>{t('col.rule')}</TableCell>
                <TableCell>{t('col.message')}</TableCell>
                <TableCell>{t('col.raised')}</TableCell>
                <TableCell>{t('col.state')}</TableCell>
                <TableCell>{t('col.ack')}</TableCell>
                <TableCell />
              </TableRow>
            </TableHead>
            <TableBody>
              {q.data.items.map((a: Alarm) => (
                <TableRow key={a.id}>
                  <TableCell>
                    <Chip
                      size="small"
                      color={SEV_COLOR[a.severity] ?? 'default'}
                      variant="outlined"
                      label={t(`severity.${a.severity}`, { defaultValue: a.severity })}
                    />
                  </TableCell>
                  <TableCell>
                    <bdi>{a.rule}</bdi>
                    {a.instance && (
                      <Typography
                        variant="caption"
                        color="text.secondary"
                        sx={{ display: 'block' }}
                        dir="ltr"
                      >
                        {a.instance}
                      </Typography>
                    )}
                  </TableCell>
                  <TableCell>{a.message}</TableCell>
                  <TableCell dir="ltr">{fmt.dateTime(new Date(a.raisedAt))}</TableCell>
                  <TableCell>
                    <Chip
                      size="small"
                      variant="outlined"
                      color={a.state === 'active' ? 'warning' : 'default'}
                      label={t(`state.${a.state}`, { defaultValue: a.state })}
                    />
                  </TableCell>
                  <TableCell>{a.ackedBy ? <bdi>{a.ackedBy}</bdi> : '—'}</TableCell>
                  <TableCell>
                    {a.ackedAt === null && (
                      <Button
                        size="small"
                        disabled={!perms.editConfig || ack.isPending}
                        onClick={() => void doAck(a.id)}
                      >
                        {t('ack')}
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}
    </Box>
  );
}
