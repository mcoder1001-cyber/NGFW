import Chip, { type ChipProps } from '@mui/material/Chip';
import { useTheme } from '@mui/material/styles';
import { useTranslation } from 'react-i18next';
import { UI_KIT_NS } from '../i18n/index.js';
import type { VrxStatus } from './createVrxTheme.js';

export interface StatusChipProps extends Omit<ChipProps, 'color' | 'label'> {
  status: VrxStatus;
  /** Override the translated label (e.g. to append a reason). */
  label?: ChipProps['label'];
}

/** Coloured status indicator; colour from `theme.vrx.status`, label from the `ui-kit` namespace. */
export function StatusChip({ status, label, sx, ...rest }: StatusChipProps) {
  const theme = useTheme();
  const { t } = useTranslation(UI_KIT_NS);
  const colour = theme.vrx.status[status];
  return (
    <Chip
      role="status"
      variant="outlined"
      label={label ?? t(`status.${status}`)}
      sx={{ color: colour, borderColor: colour, fontWeight: 600, ...sx }}
      {...rest}
    />
  );
}
