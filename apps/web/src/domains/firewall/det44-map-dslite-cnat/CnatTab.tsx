import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Divider from '@mui/material/Divider';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { usePermissions } from '../../../auth/AuthProvider';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { NS, SUBTREES } from './model';
import { useCnatPurge, useCnatSessions } from './queries';
import { SubtreeForm } from './SubtreeForm';

const PAGE_SIZE = 100;

/** CNAT: the schema-driven `nat.cnat` form, the paged session table and the (admin) purge. */
export function CnatTab() {
  const { t } = useTranslation(NS);
  const perms = usePermissions();
  const [page, setPage] = useState(0);
  const q = useCnatSessions(page, PAGE_SIZE);
  const purge = useCnatPurge();
  const pages = q.data ? Math.max(1, Math.ceil(q.data.total / PAGE_SIZE)) : 1;
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('cnat.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREES.cnat} />
      <Divider sx={{ my: 3 }} />
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
        <Typography variant="h6" component="h3" sx={{ flexGrow: 1 }}>
          {t('cnat.sessions', { total: q.data?.total ?? 0 })}
        </Typography>
        <Button disabled={page === 0} onClick={() => setPage(page - 1)}>
          {t('prev')}
        </Button>
        <Typography>{t('page', { page: page + 1, pages })}</Typography>
        <Button disabled={page + 1 >= pages} onClick={() => setPage(page + 1)}>
          {t('next')}
        </Button>
        {perms.manageUsers && (
          <Button
            color="warning"
            variant="outlined"
            disabled={purge.isPending}
            onClick={() => purge.mutate()}
          >
            {t('cnat.purge')}
          </Button>
        )}
      </Box>
      {q.isError && <ProblemAlert error={q.error} sx={{ mb: 1 }} />}
      {purge.isError && <ProblemAlert error={purge.error} sx={{ mb: 1 }} />}
      <Paper variant="outlined">
        <Table size="small" aria-label={t('cnat.sessions', { total: q.data?.total ?? 0 })}>
          <TableHead>
            <TableRow>
              <TableCell>{t('col.dst')}</TableCell>
              <TableCell>{t('col.src')}</TableCell>
              <TableCell>{t('col.protocol')}</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {q.data?.items.length === 0 && (
              <TableRow>
                <TableCell colSpan={3}>{t('empty')}</TableCell>
              </TableRow>
            )}
            {q.data?.items.map((s, i) => (
              <TableRow key={`${s.dstAddress}:${s.dstPort}|${s.srcAddress}:${s.srcPort}|${i}`}>
                <TableCell dir="ltr">{`${s.dstAddress}:${s.dstPort}`}</TableCell>
                <TableCell dir="ltr">{`${s.srcAddress}:${s.srcPort}`}</TableCell>
                <TableCell>{s.protocol}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Paper>
    </Box>
  );
}
