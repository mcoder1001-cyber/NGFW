import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import { useFormatters } from '@ngfw/ui-kit';
import { useTranslation } from 'react-i18next';
import { expiryState } from './acl/model';

/**
 * F-rule-expiry: a rule's expiry for the rule tables (ACL, host ACL, NAT static mappings) — "expired" (red),
 * "expires soon" (amber, within EXPIRY_WARN_DAYS) or the date; owner and ticket in the tooltip; "—" without expiry.
 */
export function ExpiryChip({
  expiresAt,
  owner,
  ticket,
}: {
  expiresAt?: string | undefined;
  owner?: string | undefined;
  ticket?: string | undefined;
}) {
  const { t } = useTranslation('acl');
  const fmt = useFormatters();
  const state = expiryState(expiresAt, Date.now());
  if (state === null || expiresAt === undefined) return <>—</>;
  const by = owner || ticket ? t('expiry.by', { owner: owner ?? '—', ticket: ticket ?? '—' }) : '';
  return (
    <Tooltip title={by}>
      <Chip
        size="small"
        variant={state === 'later' ? 'outlined' : 'filled'}
        color={state === 'expired' ? 'error' : state === 'soon' ? 'warning' : 'default'}
        label={
          state === 'later'
            ? fmt.dateTime(expiresAt)
            : `${t(`expiry.${state}`)} · ${fmt.dateTime(expiresAt)}`
        }
      />
    </Tooltip>
  );
}
