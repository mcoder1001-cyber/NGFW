import Alert from '@mui/material/Alert';
import AlertTitle from '@mui/material/AlertTitle';
import Box from '@mui/material/Box';
import type { SxProps, Theme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { ApiError } from '../api-problem';
import { OK_CODES } from './CommitResultView';

interface AgentResult {
  key?: string;
  op?: string;
  code?: string;
  message?: string;
  pointer?: string;
}

/** Title key for well-known P06 problem slugs; the server's own title/detail are always shown too. */
const SLUG_KEYS: Record<string, string> = {
  'candidate-locked': 'problem.locked',
  'candidate-stale': 'problem.stale',
  'candidate-dirty': 'problem.dirty',
  'commit-pending': 'problem.pending',
  'commit-reverted': 'problem.reverted',
  'no-pending-commit': 'problem.noPending',
  'agent-unavailable': 'problem.agentUnavailable',
  'running-unknown': 'problem.runningUnknown',
  validation: 'problem.validation',
  forbidden: 'problem.forbidden',
};

/**
 * Shows an API failure as the server reported it (RFC 9457): title, detail, every `errors[]` pointer, the lock owner
 * of a 409, per-object agent results and the sync state. Nothing is paraphrased away (UI honesty rule).
 */
export function ProblemAlert({ error, sx }: { error: unknown; sx?: SxProps<Theme> }) {
  const { t } = useTranslation('config');
  if (!(error instanceof ApiError)) {
    return (
      <Alert severity="error" sx={sx}>
        {t('problem.unexpected')}
      </Alert>
    );
  }
  if (error.unreachable) {
    return (
      <Alert severity="error" sx={sx}>
        <AlertTitle>{t('problem.unreachableTitle')}</AlertTitle>
        {t('problem.unreachable')}
      </Alert>
    );
  }
  const b = error.body;
  const slugKey = error.slug ? SLUG_KEYS[error.slug] : undefined;
  const lock = b['lock'] as { owner?: string | null } | undefined;
  const results = (Array.isArray(b['results']) ? (b['results'] as AgentResult[]) : []).filter((r) => !OK_CODES.has(r.code ?? ''));
  const sync = b['sync'] as { state?: string; reason?: string } | undefined;
  return (
    <Alert severity="error" sx={sx} data-testid="problem">
      <AlertTitle>
        {slugKey ? t(slugKey) : (b.title ?? t('problem.httpStatus', { status: error.status }))}
        {` (${error.status})`}
      </AlertTitle>
      {b.detail && (
        <Box sx={{ mb: 0.5 }} dir="auto">
          {b.detail}
        </Box>
      )}
      {lock?.owner && <Box>{t('problem.lockOwner', { owner: lock.owner })}</Box>}
      {b.errors && b.errors.length > 0 && (
        <Box component="ul" sx={{ m: 0, paddingInlineStart: 2.5 }}>
          {b.errors.map((e, i) => (
            <li key={`${e.pointer}:${i}`} dir="auto">
              <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                {e.pointer || '/'}
              </Box>
              {' — '}
              {e.message}
            </li>
          ))}
        </Box>
      )}
      {results.length > 0 && (
        <Box component="ul" sx={{ m: 0, paddingInlineStart: 2.5 }} aria-label={t('problem.results')}>
          {results.map((r, i) => (
            <li key={`${r.key ?? ''}:${i}`} dir="auto">
              <Box component="code" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily }}>
                {r.key ?? r.pointer}
              </Box>
              {` ${r.code ?? ''}: ${r.message ?? ''}`}
            </li>
          ))}
        </Box>
      )}
      {sync?.state && sync.state !== 'in-sync' && <Box sx={{ mt: 0.5 }}>{t('sync.inline', { state: t(`sync.state.${sync.state}`), reason: sync.reason ?? '' })}</Box>}
    </Alert>
  );
}
