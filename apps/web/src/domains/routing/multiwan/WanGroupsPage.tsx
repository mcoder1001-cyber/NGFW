import { serviceText } from '../../../product-text';
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
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { useQueryClient } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { useWanState, type WanGroupState } from './queries';

const UP = 'up' as const;
const DOWN = 'down' as const;
const WAN_KEY = ['state', 'wan'];

/** F-multiwan: `/routing/wan` — live WAN group member health. Groups are configured under Config → Routing. */
export function WanGroupsPage() {
  const { t } = useTranslation('multiwan');
  const qc = useQueryClient();
  const q = useWanState();
  const groups = q.data?.groups ?? [];
  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>
      <Stack direction="row" sx={{ mb: 1 }}>
        <Box sx={{ flex: 1 }} />
        <Button
          startIcon={<RefreshIcon />}
          onClick={() => void qc.invalidateQueries({ queryKey: WAN_KEY })}
        >
          {t('refresh')}
        </Button>
      </Stack>
      {q.isPending && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {q.data?.agentError && (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('agentError', { error: serviceText(q.data.agentError) })}
        </Alert>
      )}
      {q.data && groups.length === 0 && !q.data.agentError && (
        <Alert severity="info">{t('empty')}</Alert>
      )}
      <Stack gap={3}>
        {groups.map((g) => (
          <WanGroup key={g.name} g={g} />
        ))}
      </Stack>
    </Box>
  );
}

function WanGroup({ g }: { g: WanGroupState }) {
  const { t } = useTranslation('multiwan');
  return (
    <Box>
      <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
        <Typography component="h3" variant="subtitle1">
          <bdi>{g.name}</bdi>
        </Typography>
        <Chip
          size="small"
          variant="outlined"
          label={t(`mode.${g.mode}`, { defaultValue: g.mode })}
        />
        {g.mode === 'failover' && g.active && (
          <Typography variant="body2" color="text.secondary">
            {t('activeMember', { iface: g.active })}
          </Typography>
        )}
      </Stack>
      <TableContainer component={Paper} variant="outlined">
        <Table size="small" aria-label={t('membersOf', { name: g.name })}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.interface')}</TableCell>
              <TableCell>{t('col.health')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.loss')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.latency')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.weight')}</TableCell>
              <TableCell sx={{ textAlign: 'end' }}>{t('col.priority')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {g.members.map((m) => (
              <TableRow key={m.interface}>
                <TableCell dir="ltr">
                  <bdi>{m.interface}</bdi>
                  {g.mode === 'failover' && m.interface === g.active && (
                    <Chip
                      size="small"
                      color="success"
                      variant="outlined"
                      label={t('active')}
                      sx={{ marginInlineStart: 1 }}
                    />
                  )}
                </TableCell>
                <TableCell>
                  <StatusChip
                    size="small"
                    status={m.up ? UP : DOWN}
                    label={m.up ? t('up') : t('down')}
                  />
                </TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{m.lossPct}%</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{m.latencyMs ? `${m.latencyMs} ms` : '—'}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{m.weight}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{m.priority}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
}
