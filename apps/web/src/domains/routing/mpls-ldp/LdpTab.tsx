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
import TablePagination from '@mui/material/TablePagination';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { ldpKeys, useLdpBindings, useLdpNeighbors, useLdpSync } from './queries';

const NS = 'mpls-ldp';

/** F-mpls-ldp: the LDP tab of the Routing → MPLS page — live neighbours, LIB bindings and the FRR→VPP sync status. */
export function LdpTab() {
  const { t } = useTranslation(NS);
  const qc = useQueryClient();
  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);
  const neighbors = useLdpNeighbors();
  const bindings = useLdpBindings(page + 1, pageSize);
  const sync = useLdpSync();
  const agentError = neighbors.data?.agentError ?? sync.data?.agentError ?? null;
  const s = sync.data;

  return (
    <Box>
      <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 2 }}>
        <Typography variant="body2" color="text.secondary" sx={{ flex: 1, maxInlineSize: 800 }}>
          {t('intro')}
        </Typography>
        <Button startIcon={<RefreshIcon />} onClick={() => void qc.invalidateQueries({ queryKey: ldpKeys.all })}>
          {t('refresh')}
        </Button>
      </Stack>

      {(neighbors.isPending || sync.isPending) && <LinearProgress aria-label={t('loading')} sx={{ mb: 1 }} />}
      {neighbors.isError && <ProblemAlert error={neighbors.error} sx={{ mb: 1 }} />}
      {agentError && (
        <Alert severity="info" sx={{ mb: 2 }}>
          {t('agentError', { error: agentError })}
        </Alert>
      )}

      {s && !agentError && (
        <Paper variant="outlined" sx={{ p: 2, mb: 3, maxInlineSize: 800 }}>
          <Stack direction="row" gap={1} alignItems="center" flexWrap="wrap">
            <Typography component="h3" variant="subtitle2" sx={{ flex: 1 }}>
              {t('sync.title')}
            </Typography>
            <Chip
              size="small"
              color={s.lastError ? 'error' : s.source ? 'success' : 'default'}
              label={s.lastError ? t('sync.error') : s.source ? t('sync.ok') : t('sync.idle')}
            />
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            {t('sync.detail', { installed: s.installed, conflicts: s.conflicts, source: s.source || '—' })}
          </Typography>
          {s.lastError && (
            <Typography variant="body2" color="error" sx={{ mt: 0.5 }}>
              {s.lastError}
            </Typography>
          )}
        </Paper>
      )}

      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {t('neighbors.title')}
      </Typography>
      {(neighbors.data?.neighbors ?? []).length === 0 ? (
        <Alert severity="info" sx={{ mb: 3 }}>{t('neighbors.empty')}</Alert>
      ) : (
        <TableContainer component={Paper} variant="outlined" sx={{ mb: 3 }}>
          <Table size="small" aria-label={t('neighbors.title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('neighbors.col.lsrId')}</TableCell>
                <TableCell>{t('neighbors.col.address')}</TableCell>
                <TableCell>{t('neighbors.col.state')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('neighbors.col.uptime')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(neighbors.data?.neighbors ?? []).map((n) => (
                <TableRow key={n.lsrId}>
                  <TableCell dir="ltr">{n.lsrId}</TableCell>
                  <TableCell dir="ltr">{n.address}</TableCell>
                  <TableCell>
                    <Chip size="small" variant="outlined" label={n.state} />
                  </TableCell>
                  <TableCell dir="ltr" sx={{ textAlign: 'end' }}>{n.uptimeSec}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </TableContainer>
      )}

      <Typography component="h3" variant="subtitle1" sx={{ mb: 1 }}>
        {t('bindings.title')}
      </Typography>
      {(bindings.data?.total ?? 0) === 0 ? (
        <Alert severity="info">{t('bindings.empty')}</Alert>
      ) : (
        <TableContainer component={Paper} variant="outlined">
          <Table size="small" aria-label={t('bindings.title')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('bindings.col.prefix')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('bindings.col.localLabel')}</TableCell>
                <TableCell>{t('bindings.col.peer')}</TableCell>
                <TableCell sx={{ textAlign: 'end' }}>{t('bindings.col.remoteLabel')}</TableCell>
                <TableCell>{t('bindings.col.inUse')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {(bindings.data?.bindings ?? []).map((b, i) => (
                <TableRow key={`${b.prefix}-${b.localLabel}-${i}`}>
                  <TableCell dir="ltr">{b.prefix}</TableCell>
                  <TableCell dir="ltr" sx={{ textAlign: 'end' }}>{b.localLabel}</TableCell>
                  <TableCell dir="ltr">{b.peer}</TableCell>
                  <TableCell dir="ltr" sx={{ textAlign: 'end' }}>{b.remoteLabel}</TableCell>
                  <TableCell>{b.inUse ? t('bindings.yes') : t('bindings.no')}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
          <TablePagination
            component="div"
            count={bindings.data?.total ?? 0}
            page={page}
            onPageChange={(_e, p) => setPage(p)}
            rowsPerPage={pageSize}
            onRowsPerPageChange={(e) => {
              setPageSize(Number(e.target.value));
              setPage(0);
            }}
            rowsPerPageOptions={[50, 100, 200]}
          />
        </TableContainer>
      )}
    </Box>
  );
}
