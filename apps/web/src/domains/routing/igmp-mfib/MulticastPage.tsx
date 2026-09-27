import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
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
import { useQueryClient } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { multicastKeys, useMroutes, useMulticastGroups, usePimNeighbors } from './queries';

/** F-igmp-mfib: `/routing/multicast` — live IGMP groups, the VPP mFIB and PIM neighbours. Config is under Config → Routing. */
const ANY_SOURCE = '*';

export function MulticastPage() {
  const { t } = useTranslation('igmp-mfib');
  const qc = useQueryClient();
  const groups = useMulticastGroups();
  const mroutes = useMroutes();
  const pim = usePimNeighbors();
  const agentError = groups.data?.agentError ?? mroutes.data?.agentError ?? pim.data?.agentError ?? null;

  return (
    <Box>
      <PageHeader title={t('title')}>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2, maxInlineSize: 900 }}>
          {t('intro')}
        </Typography>
      </PageHeader>

      <Stack direction="row" sx={{ mb: 1 }}>
        <Box sx={{ flex: 1 }} />
        <Button startIcon={<RefreshIcon />} onClick={() => void qc.invalidateQueries({ queryKey: multicastKeys.all })}>
          {t('refresh')}
        </Button>
      </Stack>

      {(groups.isPending || mroutes.isPending || pim.isPending) && (
        <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />
      )}
      {groups.isError && <ProblemAlert error={groups.error} sx={{ mb: 1 }} />}
      {agentError && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('agentError', { error: agentError })}
        </Alert>
      )}

      <Section title={t('groups.title')}>
        {(groups.data?.groups ?? []).length === 0 ? (
          <Alert severity="info">{t('groups.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            <Table size="small" aria-label={t('groups.title')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('groups.col.interface')}</TableCell>
                  <TableCell>{t('groups.col.group')}</TableCell>
                  <TableCell>{t('groups.col.sources')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(groups.data?.groups ?? []).map((g, i) => (
                  <TableRow key={`${g.interface}-${g.group}-${i}`}>
                    <TableCell dir="ltr">{g.interface}</TableCell>
                    <TableCell dir="ltr">{g.group}</TableCell>
                    <TableCell dir="ltr">{g.sources.join(', ')}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Section>

      <Section title={t('mroutes.title')}>
        {(mroutes.data?.mroutes ?? []).length === 0 ? (
          <Alert severity="info">{t('mroutes.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            <Table size="small" aria-label={t('mroutes.title')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('mroutes.col.vrf')}</TableCell>
                  <TableCell>{t('mroutes.col.group')}</TableCell>
                  <TableCell>{t('mroutes.col.source')}</TableCell>
                  <TableCell>{t('mroutes.col.accept')}</TableCell>
                  <TableCell>{t('mroutes.col.forward')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(mroutes.data?.mroutes ?? []).map((m, i) => (
                  <TableRow key={`${m.vrf}-${m.group}-${m.source}-${i}`}>
                    <TableCell dir="ltr">{m.vrf}</TableCell>
                    <TableCell dir="ltr">{m.group}</TableCell>
                    <TableCell dir="ltr">{m.source || ANY_SOURCE}</TableCell>
                    <TableCell dir="ltr">{m.accept}</TableCell>
                    <TableCell dir="ltr">{m.forward.join(', ')}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Section>

      <Section title={t('pim.title')}>
        {(pim.data?.neighbors ?? []).length === 0 ? (
          <Alert severity="info">{t('pim.empty')}</Alert>
        ) : (
          <TableContainer component={Paper} variant="outlined">
            <Table size="small" aria-label={t('pim.title')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('pim.col.interface')}</TableCell>
                  <TableCell>{t('pim.col.address')}</TableCell>
                  <TableCell sx={{ textAlign: 'end' }}>{t('pim.col.uptime')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {(pim.data?.neighbors ?? []).map((n, i) => (
                  <TableRow key={`${n.interface}-${n.address}-${i}`}>
                    <TableCell dir="ltr">{n.interface}</TableCell>
                    <TableCell dir="ltr">{n.address}</TableCell>
                    <TableCell dir="ltr" sx={{ textAlign: 'end' }}>{n.uptimeSec}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </TableContainer>
        )}
      </Section>
    </Box>
  );
}

function Section({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Box sx={{ mb: 3 }}>
      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {title}
      </Typography>
      {children}
    </Box>
  );
}
