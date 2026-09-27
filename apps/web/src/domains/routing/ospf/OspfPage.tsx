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
import { useCandidateRouting, useRunningRouting } from './queries';

registerIgpLocale();

const OSPF = 'ospf' as const;
const DEFAULT_AREA_TYPE = 'normal';

/**
 * Routing › OSPF (WEB-4a, D-123): schema-driven `routing.ospf` form plus area / interface summaries. Merged UNROUTED —
 * F-ospf adds the route/nav entry and live neighbours / LSDB (no state endpoint yet).
 */
export function OspfPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const ospf = cand.data?.ospf;
  const areas = Object.entries(ospf?.areas ?? {});
  const ifaces = Object.entries(ospf?.interfaces ?? {});
  const yn = (v: boolean | undefined) => (v ? t('yes') : t('no'));
  return (
    <PageHeader title={t('ospf.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('ospf.intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        <ProtocolForm proto={OSPF} label={t('ospf.title')} />
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
                    <TableCell>{t('ospf.bfd')}</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {ifaces.map(([name, i]) => (
                    <TableRow key={name} data-testid={`ospf-if-${name}`}>
                      <TableCell dir="ltr">{name}</TableCell>
                      <TableCell dir="ltr">{i.area}</TableCell>
                      <TableCell dir="ltr">{i.cost ?? t('none')}</TableCell>
                      <TableCell>{yn(i.passive)}</TableCell>
                      <TableCell>{yn(i.bfd)}</TableCell>
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
