import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Divider from '@mui/material/Divider';
import LinearProgress from '@mui/material/LinearProgress';
import Paper from '@mui/material/Paper';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { NS } from './model';
import type { Subtree } from './model';
import { useNptv6State } from './queries';
import { SubtreeForm } from './SubtreeForm';

const SUBTREE: Subtree = 'nptv6';

/**
 * NPTv6 (RFC 6296): the schema-driven `nat.nptv6` form, then the running bindings from `GET /state/nat/nptv6` with
 * their status: write-only (VPP 26.06 has no npt66 dump; the agent re-applies every binding on each resync).
 */
export function Nptv6Tab() {
  const { t } = useTranslation(NS);
  const state = useNptv6State();
  return (
    <Box>
      <Typography color="text.secondary" sx={{ mb: 2 }}>
        {t('nptv6.intro')}
      </Typography>
      <SubtreeForm subtree={SUBTREE} />
      <Divider sx={{ my: 3 }} />
      <Typography variant="h6" component="h3" sx={{ mb: 1 }}>
        {t('nptv6.running')}
      </Typography>
      {state.isPending && <LinearProgress aria-label={t('loading')} />}
      {state.isError && <ProblemAlert error={state.error} sx={{ mb: 1 }} />}
      {state.isSuccess && (
        <Paper variant="outlined">
          <Table size="small" aria-label={t('nptv6.running')}>
            <TableHead>
              <TableRow>
                <TableCell>{t('field.interface.title')}</TableCell>
                <TableCell>{t('field.internal.title')}</TableCell>
                <TableCell>{t('field.external.title')}</TableCell>
                <TableCell>{t('field.description.title')}</TableCell>
                <TableCell>{t('col.status')}</TableCell>
              </TableRow>
            </TableHead>
            <TableBody>
              {state.data.bindings.length === 0 && (
                <TableRow>
                  <TableCell colSpan={5}>{t('nptv6.empty')}</TableCell>
                </TableRow>
              )}
              {state.data.bindings.map((b) => (
                <TableRow key={`${b.interface}|${b.internal}`}>
                  <TableCell dir="ltr">{b.interface}</TableCell>
                  <TableCell dir="ltr">{b.internal}</TableCell>
                  <TableCell dir="ltr">{b.external}</TableCell>
                  <TableCell>{b.description ?? ''}</TableCell>
                  <TableCell>
                    <Tooltip title={t('nptv6.writeOnlyHelp')}>
                      <Chip
                        size="small"
                        variant="outlined"
                        color="info"
                        label={t('status.writeOnly')}
                      />
                    </Tooltip>
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
