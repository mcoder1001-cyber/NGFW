import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import { useFormatters } from '@ngfw/ui-kit';
import { useTranslation } from 'react-i18next';
import type { CommitResult } from './queries';
import { useDomainList } from './useDomainList';

const SEVERITY: Record<CommitResult['status'], 'success' | 'warning' | 'info'> = {
  applied: 'success',
  confirmed: 'success',
  'partially-applied': 'warning',
  'not-applied': 'warning',
  pending: 'info',
  unchanged: 'info',
};

/**
 * The commit answer as P06 reports it (D-P06-15/16): `partially-applied` / `not-applied` name the domains the agent
 * does not implement yet (`notApplied`) — stored in running but NOT enforced on the data plane. Shown, never hidden.
 */
export function CommitResultView({ result }: { result: CommitResult }) {
  const { t } = useTranslation('config');
  const domainList = useDomainList();
  const fmt = useFormatters();
  return (
    <Alert severity={SEVERITY[result.status]} data-testid="commit-result">
      <AlertTitle>{t(`result.${result.status}`)}</AlertTitle>
      {result.revision && (
        <Box>
          {t('result.revision', { id: fmt.integer(result.revision.id) })}
          {result.txnId && (
            <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, marginInlineStart: 1, fontSize: '0.75rem' }}>
              {result.txnId}
            </Box>
          )}
        </Box>
      )}
      {result.notApplied.length > 0 && (
        <Box sx={{ mt: 0.5 }}>
          {t('result.notApplied', {
            domains: domainList(result.notApplied),
            count: result.notApplied.length,
          })}
        </Box>
      )}
      {result.warnings.length > 0 && (
        <Box component="ul" sx={{ m: 0, mt: 0.5, paddingInlineStart: 2.5 }} aria-label={t('result.warnings')}>
          {result.warnings.map((w, i) => (
            <li key={`${w.pointer}:${i}`}>
              <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                {w.pointer || '/'}
              </Box>
              {' — '}
              {w.message}
            </li>
          ))}
        </Box>
      )}
      {result.sync && result.sync.state !== 'in-sync' && (
        <Box sx={{ mt: 0.5 }}>{t('sync.inline', { state: t(`sync.state.${result.sync.state}`), reason: result.sync.reason })}</Box>
      )}
    </Alert>
  );
}
