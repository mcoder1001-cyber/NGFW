import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
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
import {
  REDISTRIBUTE_SOURCES,
  useCandidateRouting,
  useRunningRouting,
  type RoutingIgp,
} from '../ospf/queries';

registerIgpLocale();

const SESSIONS = 'sessions' as const;
const REDIST = 'redistribution' as const;
const BFD = 'bfd' as const;
const CHECK = '✓';
const TARGETS = ['bgp', 'ospf', 'isis', 'rip'] as const;
type Target = (typeof TARGETS)[number];

/** Redistribution matrix: for each target protocol, which sources it redistributes (null when it is not configured). */
export function redistributionMatrix(r: RoutingIgp): Record<Target, string[] | null> {
  const out = {} as Record<Target, string[] | null>;
  for (const p of TARGETS) {
    const cfg = r[p];
    out[p] = cfg ? REDISTRIBUTE_SOURCES.filter((s) => cfg.redistribute?.[s] !== undefined) : null;
  }
  return out;
}

function Sessions() {
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const sessions = cand.data?.bfd?.sessions ?? [];
  return (
    <Paper variant="outlined" sx={{ p: 2, flex: 1, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('bfd.sessions')}
      </Typography>
      {sessions.length === 0 ? (
        <Typography color="text.secondary">{t('bfd.empty')}</Typography>
      ) : (
        <Box sx={{ overflowX: 'auto' }}>
          <Table size="small" aria-label={t('bfd.sessions')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('bfd.interface')}</TableCell>
                <TableCell>{t('bfd.local')}</TableCell>
                <TableCell>{t('bfd.peer')}</TableCell>
                <TableCell>{t('bfd.timers')}</TableCell>
                <TableCell>{t('bfd.enabled')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {sessions.map((s) => (
                <TableRow
                  key={`${s.interface}|${s.peerAddress}`}
                  data-testid={`bfd-${s.peerAddress}`}
                >
                  <TableCell dir="ltr">{s.interface}</TableCell>
                  <TableCell dir="ltr">{s.localAddress}</TableCell>
                  <TableCell dir="ltr">{s.peerAddress}</TableCell>
                  <TableCell dir="ltr">
                    {`${s.desiredMinTxUs ?? 300000} / ${s.requiredMinRxUs ?? 300000} × ${s.detectMultiplier ?? 3}`}
                  </TableCell>
                  <TableCell>{s.enabled === false ? t('no') : t('yes')}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Box>
      )}
    </Paper>
  );
}

function Redistribution() {
  const { t } = useTranslation(NS);
  const cand = useCandidateRouting();
  const m = redistributionMatrix(cand.data ?? {});
  return (
    <Paper variant="outlined" sx={{ p: 2, width: '100%' }}>
      <Typography component="h3" variant="h6" gutterBottom>
        {t('bfd.matrix')}
      </Typography>
      <Box sx={{ overflowX: 'auto' }}>
        <Table size="small" aria-label={t('bfd.matrix')}>
          <TableHead>
            <TableRow>
              <TableCell>{t('bfd.into')}</TableCell>
              {REDISTRIBUTE_SOURCES.map((s) => (
                <TableCell key={s} dir="ltr">
                  {s}
                </TableCell>
              ))}
            </TableRow>
          </TableHead>
          <TableBody>
            {TARGETS.map((p) => (
              <TableRow key={p} data-testid={`redist-${p}`}>
                <TableCell dir="ltr">{p}</TableCell>
                {m[p] === null ? (
                  <TableCell colSpan={REDISTRIBUTE_SOURCES.length}>
                    <Chip size="small" label={t('bfd.notRunning')} />
                  </TableCell>
                ) : (
                  REDISTRIBUTE_SOURCES.map((s) => (
                    <TableCell key={s}>
                      {s === p ? t('none') : m[p]?.includes(s) ? CHECK : null}
                    </TableCell>
                  ))
                )}
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Box>
    </Paper>
  );
}

/**
 * Routing › BFD and redistribution (WEB-4a, D-123): schema-driven `routing.bfd` sessions and a read-only
 * redistribution matrix (edited in each protocol's form). Merged UNROUTED — F-bfd-redistribution adds the route/nav
 * entry and live session state.
 */
export function BfdPage() {
  const { t } = useTranslation([NS, 'config']);
  const cand = useCandidateRouting();
  const running = useRunningRouting();
  const [tab, setTab] = useState<'sessions' | 'redistribution'>('sessions');
  return (
    <PageHeader title={t('bfd.title')}>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('bfd.intro')}
      </Typography>
      {(cand.isPending || running.isPending) && (
        <LinearProgress aria-label={t('config:loading')} sx={{ mb: 2 }} />
      )}
      {cand.isError && <ProblemAlert error={cand.error} sx={{ mb: 2 }} />}
      {running.isError && <ProblemAlert error={running.error} sx={{ mb: 2 }} />}
      <Tabs
        value={tab}
        onChange={(_, v: 'sessions' | 'redistribution') => setTab(v)}
        sx={{ mb: 2 }}
      >
        <Tab value={SESSIONS} label={t('bfd.tabs.sessions')} />
        <Tab value={REDIST} label={t('bfd.tabs.redistribution')} />
      </Tabs>
      {tab === 'sessions' ? (
        <Stack direction={{ xs: 'column', lg: 'row' }} spacing={3} alignItems="flex-start">
          <ProtocolForm proto={BFD} label={BFD.toUpperCase()} />
          <Sessions />
        </Stack>
      ) : (
        <Redistribution />
      )}
    </PageHeader>
  );
}
