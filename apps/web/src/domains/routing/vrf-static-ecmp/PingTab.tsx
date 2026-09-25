import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Paper from '@mui/material/Paper';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableRow from '@mui/material/TableRow';
import TextField from '@mui/material/TextField';
import Typography from '@mui/material/Typography';
import { StatusChip } from '@ngfw/ui-kit';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { usePing } from './api';
import { Mono } from './common';
import { NS } from './model';

const LTR = { dir: 'ltr' } as const;
const NUM = { dir: 'ltr', inputMode: 'numeric' } as const;
const STATS = ['transmitted', 'received', 'loss_pct'] as const;
const REPLIED = 'up' as const;
const SILENT = 'down' as const;
const ADDR_RE = /^(?:[0-9]{1,3}(?:\.[0-9]{1,3}){3}|[0-9a-fA-F:]*:[0-9a-fA-F:.]*)$/;

/**
 * Ping from the data plane: `POST /api/v1/actions/ping` → the agent's Action RPC → VPP's ping plugin. VPP's API pings
 * from the default VRF only, without a source or size, and holds the binary API for count × interval (≤ 5 s): the form
 * offers exactly that (docs/user/routing/vrf-static-ecmp.md).
 */
export function PingTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const ping = usePing();
  const [target, setTarget] = useState('');
  const [count, setCount] = useState('5');
  const [interval, setIntervalMs] = useState('1000');
  const ok = ADDR_RE.test(target.trim()) && /^[0-9]+$/.test(count) && /^[0-9]+$/.test(interval);
  const res = ping.data;

  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('ping.intro')}
      </Typography>
      <Stack
        component="form"
        direction={{ xs: 'column', md: 'row' }}
        gap={1}
        alignItems={{ md: 'flex-start' }}
        onSubmit={(e) => {
          e.preventDefault();
          if (ok) ping.mutate({ target: target.trim(), count: Number(count), intervalMs: Number(interval) });
        }}
        sx={{ mb: 2 }}
      >
        <TextField
          size="small"
          label={t('ping.target')}
          value={target}
          onChange={(e) => setTarget(e.target.value)}
          error={target !== '' && !ADDR_RE.test(target.trim())}
          helperText={target !== '' && !ADDR_RE.test(target.trim()) ? t('ping.badTarget') : t('ping.targetHelp')}
          slotProps={{ htmlInput: LTR }}
          sx={{ minInlineSize: 240 }}
        />
        <TextField size="small" label={t('ping.count')} value={count} onChange={(e) => setCount(e.target.value)} slotProps={{ htmlInput: NUM }} sx={{ inlineSize: 110 }} />
        <TextField
          size="small"
          label={t('ping.interval')}
          value={interval}
          onChange={(e) => setIntervalMs(e.target.value)}
          slotProps={{ htmlInput: NUM }}
          sx={{ inlineSize: 150 }}
        />
        <Button type="submit" variant="contained" disabled={!ok || !perms.editConfig || ping.isPending}>
          {ping.isPending ? t('ping.running') : t('ping.run')}
        </Button>
      </Stack>
      <Alert severity="info" sx={{ mb: 2 }}>
        {t('ping.limits')}
      </Alert>
      {ping.isError && <ProblemAlert error={ping.error} sx={{ mb: 2 }} />}
      {res && (
        <Paper variant="outlined" sx={{ p: 2 }} aria-label={t('ping.result')}>
          <Stack direction="row" gap={1} alignItems="center" sx={{ mb: 1 }}>
            <StatusChip
              status={res.done.exitCode === 0 ? REPLIED : SILENT}
              label={res.done.exitCode === 0 ? t('ping.reachable') : t('ping.unreachable')}
            />
          </Stack>
          {res.lines.map((l, i) => (
            <Box key={i}>
              <Mono>{l}</Mono>
            </Box>
          ))}
          <Table size="small" sx={{ mt: 1, maxInlineSize: 420 }}>
            <TableBody>
              {STATS.map((k) => (
                <TableRow key={k}>
                  <TableCell component="th">{t(`ping.stats.${k}`)}</TableCell>
                  <TableCell dir="ltr" sx={{ textAlign: 'start' }}>
                    {res.done.stats[k] ?? '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </Paper>
      )}
    </Box>
  );
}
