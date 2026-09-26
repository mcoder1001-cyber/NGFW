import RefreshIcon from '@mui/icons-material/Refresh';
import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Chip from '@mui/material/Chip';
import Dialog from '@mui/material/Dialog';
import DialogActions from '@mui/material/DialogActions';
import DialogContent from '@mui/material/DialogContent';
import DialogContentText from '@mui/material/DialogContentText';
import DialogTitle from '@mui/material/DialogTitle';
import Link from '@mui/material/Link';
import Stack from '@mui/material/Stack';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { useFormatters } from '@ngfw/ui-kit';
import { useCallback, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Link as RouterLink } from 'react-router';
import { ProblemAlert } from '../../../config/ProblemAlert';
import { formatBytes, type BoundAcl, type Translate } from './model';

/** The `acl` namespace's `t` as a plain function for the model helpers (formatBytes, targetText, localizeSchema). */
export function useAclT(): Translate {
  const { t } = useTranslation('acl');
  return useCallback((key, opts) => t(key, opts ?? {}), [t]);
}

/**
 * ADL / Auto-SDL (access-list based L2/L3 filtering on an interface) is configured with the interface security settings
 * of F-rpf-adl-pbr, whose screen is `/firewall/adl` (its router line on task/F-rpf-adl-pbr). This page only links to it.
 */
export const ADL_ROUTE = '/firewall/adl';

export function AdlInfo() {
  const { t } = useTranslation('acl');
  return (
    <Alert severity="info" sx={{ mb: 2 }}>
      {t('adl.text')}{' '}
      <Link component={RouterLink} to={ADL_ROUTE}>
        {t('adl.link')}
      </Link>
    </Alert>
  );
}

/** Technical values (prefixes, interface names, tags) stay left-to-right inside RTL text. */
export function Mono({ children, muted = false }: { children: ReactNode; muted?: boolean }) {
  return (
    <Box
      component="span"
      dir="ltr"
      sx={{
        fontFamily: (th) => th.vrx.monoFontFamily,
        fontSize: 12,
        textAlign: 'start',
        color: muted ? 'text.secondary' : undefined,
      }}
    >
      {children}
    </Box>
  );
}

/** Candidate vs running mark of a list, rule or attachment. */
export function PendingChip({
  pending,
}: {
  pending: 'added' | 'changed' | 'deleted' | 'pending' | null | undefined;
}) {
  const { t } = useTranslation('acl');
  if (!pending) return null;
  return (
    <Chip
      size="small"
      variant="outlined"
      color={pending === 'deleted' ? 'error' : 'warning'}
      label={t(`pending.${pending}`)}
    />
  );
}

/** D-132: live data that walks VPP refreshes on demand (and at most every 30 s where a timer exists). */
export function RefreshButton({
  onClick,
  busy,
  updatedAt,
}: {
  onClick: () => void;
  busy: boolean;
  updatedAt?: number | string | null | undefined;
}) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  return (
    <Stack direction="row" gap={1} alignItems="center">
      {updatedAt ? (
        <Typography variant="caption" color="text.secondary">
          {t('updated', { when: fmt.relative(updatedAt) })}
        </Typography>
      ) : null}
      <Button
        size="small"
        variant="outlined"
        startIcon={<RefreshIcon />}
        onClick={onClick}
        disabled={busy}
      >
        {t('refresh')}
      </Button>
    </Stack>
  );
}

/** Why live columns are empty: the agent is unreachable, or counters are off on this agent (V7, D-071). */
export function LiveAlerts({
  agentError,
  countersAvailable,
  countersReason,
}: {
  agentError: string | null | undefined;
  countersAvailable?: boolean | undefined;
  countersReason?: string | undefined;
}) {
  const { t } = useTranslation('acl');
  return (
    <>
      {agentError ? (
        <Alert severity="warning" sx={{ mb: 1 }}>
          {t('live.agentError')} <bdi>{agentError}</bdi>
        </Alert>
      ) : null}
      {countersAvailable === false ? (
        <Alert severity="info" sx={{ mb: 1 }}>
          {t('live.countersOff')} {countersReason ? <bdi>{countersReason}</bdi> : null}
        </Alert>
      ) : null}
    </>
  );
}

/** A counter cell: "—" with the reason as tooltip when counters are unavailable or the rule is not in VPP. */
export function CounterCell({
  value,
  kind,
  reason,
}: {
  value: number | null;
  kind: 'packets' | 'bytes';
  reason: string;
}) {
  const tr = useAclT();
  const fmt = useFormatters();
  if (value === null) {
    return (
      <Tooltip title={reason}>
        <Typography component="span" variant="body2" color="text.secondary">
          —
        </Typography>
      </Tooltip>
    );
  }
  return (
    <Box component="span" sx={{ fontVariantNumeric: 'tabular-nums' }}>
      {kind === 'bytes' ? formatBytes(value, fmt, tr) : fmt.integer(value)}
    </Box>
  );
}

/** One ACL of a live binding chain: this agent's list by name, another owner's by tag with a "foreign" mark (D-066). */
export function BoundChip({ acl }: { acl: BoundAcl }) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const title = t('live.aclIndex', { index: fmt.number(acl.aclIndex, { useGrouping: false }) });
  if (acl.foreign || acl.name === null) {
    return (
      <Tooltip title={title}>
        <Stack direction="row" gap={0.5} alignItems="center" component="span">
          <Chip size="small" variant="outlined" label={<bdi dir="ltr">{acl.tag || '—'}</bdi>} />
          <Chip size="small" color="default" label={t('live.foreign')} />
        </Stack>
      </Tooltip>
    );
  }
  return (
    <Tooltip title={title}>
      <Chip size="small" color="primary" variant="outlined" label={<bdi>{acl.name}</bdi>} />
    </Tooltip>
  );
}

/** A yes/no confirmation (delete, bulk delete). */
export function ConfirmDialog({
  open,
  title,
  text,
  confirmLabel,
  busy,
  error,
  onConfirm,
  onClose,
}: {
  open: boolean;
  title: string;
  text: ReactNode;
  confirmLabel: string;
  busy: boolean;
  error: unknown;
  onConfirm: () => void;
  onClose: () => void;
}) {
  const { t } = useTranslation('acl');
  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <DialogContentText component="div">{text}</DialogContentText>
        {error !== null && error !== undefined && <ProblemAlert error={error} sx={{ mt: 2 }} />}
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>{t('cancel')}</Button>
        <Button color="error" variant="contained" onClick={onConfirm} disabled={busy}>
          {confirmLabel}
        </Button>
      </DialogActions>
    </Dialog>
  );
}
