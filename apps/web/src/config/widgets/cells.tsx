import Box from '@mui/material/Box';
import Chip from '@mui/material/Chip';
import Tooltip from '@mui/material/Tooltip';
import Typography from '@mui/material/Typography';
import { StatusChip, UI_KIT_NS, useFormatters, type VrxStatus } from '@ngfw/ui-kit';
import type { ReactElement, ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import type { RowState } from '../collection/model';
import { formatCounter, type CounterValue } from './format';

/** An identifier (interface name, prefix, MAC, ref): left-to-right monospace inside any reading direction. */
export function IdText({ children, small = false }: { children: ReactNode; small?: boolean }) {
  return (
    <Box component="bdi" dir="ltr" sx={{ fontFamily: (th) => th.vrx.monoFontFamily, ...(small ? { fontSize: 12 } : {}) }}>
      {children}
    </Box>
  );
}

/** Live status of a row: a StatusChip, or a quiet text when the data plane has no such object (`not in the data plane`). */
export function StatusCell({ status, label, missing }: { status: VrxStatus | undefined; label?: ReactNode; missing?: ReactNode }) {
  const { t } = useTranslation('config');
  if (status) return <StatusChip size="small" status={status} {...(label !== undefined ? { label } : {})} />;
  return (
    <Typography variant="body2" color="text.secondary" component="span">
      {missing ?? t('kit.notInDataplane')}
    </Typography>
  );
}

/** A counter, exact for 64-bit decimal strings, in the UI's digits. */
export function CounterText({ value }: { value: CounterValue }) {
  const fmt = useFormatters();
  return <bdi dir="ltr">{formatCounter(fmt, value)}</bdi>;
}

/** A bit or packet rate ("1.2 Gbit/s"). */
export function RateText({ value, unit }: { value: number | null | undefined; unit: 'bps' | 'pps' }) {
  const fmt = useFormatters();
  return <bdi dir="ltr">{fmt.rate(value, unit)}</bdi>;
}

/** Candidate vs running of a row (`new` / `changed` / `to be removed`); nothing for an unchanged row. */
export function RowStateChip({ state }: { state: RowState }) {
  const { t } = useTranslation('config');
  if (!state) return null;
  return <Chip size="small" variant="outlined" color={state === 'removed' ? 'error' : 'warning'} label={t(`kit.state.${state}`)} />;
}

/** State of the live WebSocket feed of a screen (`Live`, `Reconnecting…`). */
export function LiveChip({ status, label }: { status: string; label?: (statusText: string) => string }) {
  const { t } = useTranslation(['config', UI_KIT_NS]);
  const text = t(`ws.${status}`, { ns: UI_KIT_NS, defaultValue: status });
  return <Chip size="small" variant="outlined" color={status === 'open' ? 'success' : 'default'} label={label ? label(text) : t('kit.live', { status: text })} />;
}

/** Explains why an action is disabled (a disabled button gets no hover events of its own). */
export function Why({ reason, children }: { reason: string | undefined; children: ReactElement }) {
  return reason ? (
    <Tooltip title={reason}>
      <span>{children}</span>
    </Tooltip>
  ) : (
    children
  );
}
