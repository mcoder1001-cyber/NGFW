import Alert from '@mui/material/Alert';
import Chip from '@mui/material/Chip';
import Tabs from '@mui/material/Tabs';
import Tab from '@mui/material/Tab';
import { useState } from 'react';
import Box from '@mui/material/Box';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { NS, registerIgpLocale } from './locale';
import { ProtocolForm } from './ProtocolForm';
import { useCandidateRouting, useRunningRouting, useOspfNeighbors } from './queries';

registerIgpLocale();

const DEFAULT_AREA_TYPE = 'normal';
const V2 = 'ospf' as const;
const V3 = 'ospf6' as const;
const NEIGHBOR_COLUMNS = ['vrf', 'routerId', 'interface', 'state'] as const;

/**
 * Routing › OSPF (WEB-4a, D-123): schema-driven `routing.ospf` form plus area / interface summaries. Merged UNROUTED —
 * F-ospf adds the route/nav entry and live neighbours / LSDB (no state endpoint yet).
 */
export function OspfPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [proto, setProto] = useState<'ospf' | 'ospf6'>('ospf');
  const observed = useOspfNeighbors(proto === 'ospf' ? '2' : '3');
  const ospf = cand.data?.[proto];
  const areas = Object.entries(ospf?.areas ?? {});
  const ifaces = Object.entries(ospf?.interfaces ?? {});
  const yn = (v: boolean | undefined) => (v ? t('yes') : t('no'));
  return (
    <PageHeader title={t('ospf.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('ospf.intro')}
      </Typography>
      {proto === 'ospf' && <Alert severity="info">{t('ospf.authBoundary')}</Alert>}
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs
        value={proto}
        onChange={(_, value: 'ospf' | 'ospf6') => setProto(value)}
        aria-label={t('ospf.versions')}
      >
        <Tab value={V2} label={t('ospf.v2')} />
        <Tab value={V3} label={t('ospf.v3')} />
      </Tabs>
      <Paper variant="outlined" sx={{ p: 2, mb: 2 }}>
        <Typography component="h3" variant="h6">
          {t('ospf.neighbors')}
        </Typography>
        {observed.isPending && <LinearProgress aria-label={t('config:loading')} />}
        {observed.isError && <ProblemAlert error={observed.error} />}
        {observed.data && (
          <>
            {(observed.data.unavailable || observed.data.warning || observed.data.truncated) && (
              <Alert severity="warning">{t('ospf.observationWarning')}</Alert>
            )}
            {!observed.data.unavailable && observed.data.neighbors.length === 0 && (
              <Typography>{t('ospf.noNeighbors')}</Typography>
            )}
            <Table size="small" aria-label={t('ospf.neighbors')}>
              <TableHead>
                <TableRow>
                  {NEIGHBOR_COLUMNS.map((k) => (
                    <TableCell key={k}>{t(`ospf.${k}`)}</TableCell>
                  ))}
                </TableRow>
              </TableHead>
              <TableBody>
                {observed.data.neighbors.map((n, i) => (
                  <TableRow key={`${n.vrf}-${n.routerId}-${n.interface}-${i}`}>
                    <TableCell>{n.vrf}</TableCell>
                    <TableCell dir="ltr">{n.routerId}</TableCell>
                    <TableCell dir="ltr">{n.interface ?? '—'}</TableCell>
                    <TableCell>
                      <Chip
                        size="small"
                        color={n.state.startsWith('Full') ? 'success' : 'warning'}
                        label={n.state}
                      />
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </>
        )}
      </Paper>
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        <ProtocolForm key={proto} proto={proto} label={t(proto === V2 ? 'ospf.v2' : 'ospf.v3')} />
        <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
          <Typography component="h3" variant="h6" gutterBottom>
            {t('ospf.areas')}
          </Typography>
          {areas.length === 0 ? (
            <Typography color="text.secondary">{t('ospf.empty')}</Typography>
          ) : (
            <Table size="small" aria-label={t('ospf.areas')}>
              <TableHead>
                <TableRow>
                  <TableCell>{t('ospf.area')}</TableCell>
                  <TableCell>{t('ospf.type')}</TableCell>
                  <TableCell>{t('ospf.noSummary')}</TableCell>
                </TableRow>
              </TableHead>
              <TableBody>
                {areas.map(([id, a]) => (
                  <TableRow key={id} data-testid={`ospf-area-${id}`}>
                    <TableCell dir="ltr">{id}</TableCell>
                    <TableCell dir="ltr">{a.type ?? DEFAULT_AREA_TYPE}</TableCell>
                    <TableCell>{yn(a.noSummary)}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
          <Typography component="h3" variant="h6" gutterBottom sx={{ mt: 3 }}>
            {t('ospf.interfaces')}
          </Typography>
          {ifaces.length === 0 ? (
            <Typography color="text.secondary">{t('ospf.empty')}</Typography>
          ) : (
            <Box sx={{ overflowX: 'auto' }}>
              <Table size="small" aria-label={t('ospf.interfaces')}>
                <TableHead>
                  <TableRow>
                    <TableCell>{t('ospf.interface')}</TableCell>
                    <TableCell>{t('ospf.area')}</TableCell>
                    <TableCell>{t('ospf.cost')}</TableCell>
                    <TableCell>{t('ospf.passive')}</TableCell>
                    {proto === 'ospf' && <TableCell>{t('ospf.bfd')}</TableCell>}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {ifaces.map(([name, i]) => (
                    <TableRow key={name} data-testid={`ospf-if-${name}`}>
                      <TableCell dir="ltr">{name}</TableCell>
                      <TableCell dir="ltr">{i.area}</TableCell>
                      <TableCell dir="ltr">{i.cost ?? t('none')}</TableCell>
                      <TableCell>{yn(i.passive)}</TableCell>
                      {proto === 'ospf' && <TableCell>{yn(i.bfd)}</TableCell>}
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </Box>
          )}
        </Paper>
      </Stack>
    </PageHeader>
  );
}
