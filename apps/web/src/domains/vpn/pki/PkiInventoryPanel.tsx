import Alert from '@mui/material/Alert';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import Typography from '@mui/material/Typography';
import { useQuery } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { api } from '../../../api';
import { call } from '../../../api-problem';

import { PkiActions } from './PkiActions';

const NS = 'pkiInventory';
const LIMIT = 100;
const ROW_SCOPE = 'row';

async function fetchInventory(signal: AbortSignal) {
  return (await call(api.GET('/api/v1/state/pki', { signal }))).data;
}

/** Public, running-state observation only. References, keys, PEM and raw server diagnostics are never rendered. */
export function PkiInventoryPanel() {
  const { t } = useTranslation(NS);
  const query = useQuery({
    queryKey: ['state', 'vpn', 'pki'],
    queryFn: ({ signal }) => fetchInventory(signal),
    retry: false,
  });
  const data = query.data;
  const groups = data
    ? [
        { title: t('authorities'), rows: data.cas.slice(0, LIMIT) },
        {
          title: t('certificates'),
          rows: data.certificates.slice(0, Math.max(0, LIMIT - data.cas.length)),
        },
      ]
    : [];
  const total = data ? data.cas.length + data.certificates.length : 0;
  return (
    <Stack spacing={2} aria-label={t('title')} aria-busy={query.isFetching}>
      <Typography variant="h3">{t('title')}</Typography>
      <Typography>{t('help')}</Typography>
      <PkiActions onChanged={() => void query.refetch()} />
      {query.isPending && <Typography role="status">{t('loading')}</Typography>}
      {query.isError && <Alert severity="error">{t('error')}</Alert>}
      <Button disabled={query.isFetching} onClick={() => void query.refetch()}>
        {t(query.isError ? 'retry' : 'refresh')}
      </Button>
      {data && (
        <>
          <Typography role="status">{t('checked', { time: data.generatedAt })}</Typography>
          {data.agentFiles.unavailable !== null && (
            <Alert severity="warning">{t('agentUnavailable')}</Alert>
          )}
          {total === 0 && <Typography>{t('empty')}</Typography>}
          {total > LIMIT && (
            <Typography role="status">{t('bounded', { shown: LIMIT, total })}</Typography>
          )}
          {groups
            .filter((group) => group.rows.length > 0)
            .map((group) => (
              <Table key={group.title} size="small" aria-label={group.title}>
                <TableHead>
                  <TableRow>
                    {[t('name'), t('subject'), t('issuer'), t('expires'), t('status')].map(
                      (label) => (
                        <TableCell key={label}>{label}</TableCell>
                      ),
                    )}
                  </TableRow>
                </TableHead>
                <TableBody>
                  {group.rows.map((item) => (
                    <TableRow key={item.name}>
                      <TableCell component="th" scope={ROW_SCOPE}>
                        {item.name}
                      </TableCell>
                      <TableCell>{item.subject ?? t('unknown')}</TableCell>
                      <TableCell>{item.issuer ?? t('unknown')}</TableCell>
                      <TableCell>{item.notAfter ?? t('unknown')}</TableCell>
                      <TableCell>
                        {t(
                          'revokedByCrl' in item &&
                            (item.revokedByCrl || item.ocsp?.status === 'revoked')
                            ? 'revoked'
                            : item.notAfter !== null &&
                                Date.parse(item.notAfter) < Date.parse(data.generatedAt)
                              ? 'expired'
                              : 'expiring' in item && item.expiring
                                ? 'expiring'
                                : item.problems.length > 0
                                  ? 'attention'
                                  : 'configured',
                        )}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ))}
        </>
      )}
    </Stack>
  );
}
