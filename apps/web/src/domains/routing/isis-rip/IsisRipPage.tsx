import { LiveState } from './LiveState';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Tab from '@mui/material/Tab';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { PageHeader } from '../../../shell/PageHeader';
import { NS, registerIgpLocale } from '../ospf/locale';
import { ProtocolForm } from '../ospf/ProtocolForm';
import { useCandidateRouting, useRunningRouting } from '../ospf/queries';

registerIgpLocale();

const ISIS = 'isis' as const;
const RIP = 'rip' as const;
const RIPNG = 'ripng' as const;

export type TabKey = 'isis' | 'rip' | 'ripng';

function Interfaces({ proto }: { proto: TabKey }) {
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const cfg = cand.data?.[proto];
  const rows = Object.entries(cfg?.interfaces ?? {}) as [
    string,
    { passive?: boolean; metric?: number; circuitType?: string },
  ][];
  const networks = proto !== 'isis' ? (cand.data?.[proto]?.networks ?? []) : [];
  return (
    <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('isisRip.interfaces')}
      </Typography>
      {rows.length === 0 ? (
        <Typography color="text.secondary">{t('isisRip.empty')}</Typography>
      ) : (
        <Table size="small" aria-label={t('isisRip.interfaces')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('isisRip.interface')}</TableCell>
              <TableCell>{t('isisRip.passive')}</TableCell>
              {proto === 'isis' && <TableCell>{t('isisRip.metric')}</TableCell>}
              {proto === 'isis' && <TableCell>{t('isisRip.circuit')}</TableCell>}
            </TableRow>
          </TableHead>
          <TableBody>
            {rows.map(([name, i]) => (
              <TableRow key={name} data-testid={`${proto}-if-${name}`}>
                <TableCell dir="ltr">{name}</TableCell>
                <TableCell>{i.passive ? t('yes') : t('no')}</TableCell>
                {proto === 'isis' && <TableCell dir="ltr">{i.metric ?? t('none')}</TableCell>}
                {proto === 'isis' && <TableCell dir="ltr">{i.circuitType ?? t('none')}</TableCell>}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}
      {proto !== 'isis' && (
        <>
          <Typography component="h3" variant="h6" gutterBottom sx={{ mt: 3 }}>
            {t('isisRip.networks')}
          </Typography>
          <Typography dir="ltr" data-testid="rip-networks">
            {networks.join(', ') || t('none')}
          </Typography>
        </>
      )}
    </Paper>
  );
}

/**
 * Routing › IS-IS and RIP (WEB-4a, D-123): schema-driven `routing.isis` / `routing.rip` forms with interface summaries.
 * Routed at /routing/isis-rip by F-isis-rip (bounded live adjacency and peer state).
 */
export function IsisRipPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [tab, setTab] = useState<TabKey>('isis');
  return (
    <PageHeader title={t('isisRip.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('isisRip.intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs value={tab} onChange={(_, v: TabKey) => setTab(v)} sx={{ mb: 2 }}>
        <Tab value={ISIS} label={t('isisRip.tabs.isis')} />
        <Tab value={RIP} label={t('isisRip.tabs.rip')} />
        <Tab value={RIPNG} label={t('isisRip.tabs.ripng')} />
      </Tabs>
      <Stack key={tab} direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
        {tab === 'rip' && (
          <Typography color="text.secondary">{t('isisRip.authBoundary')}</Typography>
        )}
        <ProtocolForm proto={tab} label={t(`isisRip.tabs.${tab}`)} />
        <Interfaces proto={tab} />
      </Stack>
      <LiveState proto={tab} />
    </PageHeader>
  );
}
