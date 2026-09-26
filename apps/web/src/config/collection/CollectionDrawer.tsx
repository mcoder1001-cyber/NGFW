import CloseIcon from '@mui/icons-material/Close';
import Box from '@mui/material/Box';
import Drawer from '@mui/material/Drawer';
import IconButton from '@mui/material/IconButton';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';

const LTR = { dir: 'ltr' } as const;

export interface CollectionDrawerProps {
  open: boolean;
  onClose: () => void;
  /** Heading: the item's key (rendered LTR monospace) or a caption such as "New entry". */
  title: ReactNode;
  /** `true` when `title` is an identifier (interface name, ref): LTR monospace inside RTL. */
  titleIsKey?: boolean | undefined;
  /** Accessible name of the drawer region (`Interface host-w1l0`). */
  label: string;
  /** Live-status slot next to the title (status chips). */
  status?: ReactNode;
  children?: ReactNode;
}

/**
 * The side drawer of a config list (P08's interface drawer shell): opens from the end of the reading direction — MUI
 * flips `anchor="right"` to the left in RTL — above the pending-change bar, full width on small screens.
 */
export function CollectionDrawer({ open, onClose, title, titleIsKey = true, label, status, children }: CollectionDrawerProps) {
  const { t } = useTranslation('config');
  return (
    <Drawer anchor="right" open={open} onClose={onClose} sx={{ zIndex: (th) => th.zIndex.modal }} slotProps={{ paper: { sx: { inlineSize: { xs: '100%', md: 640 } } } }}>
      {open && (
        <Box sx={{ p: 2 }} role="region" aria-label={label}>
          <Stack direction="row" alignItems="center" gap={1} sx={{ mb: 1 }}>
            <Typography
              component="h3"
              variant="h6"
              {...(titleIsKey ? LTR : {})}
              sx={titleIsKey ? { flex: 1, textAlign: 'start', fontFamily: (th) => th.vrx.monoFontFamily } : { flex: 1, textAlign: 'start' }}
            >
              {title}
            </Typography>
            {status}
            <IconButton aria-label={t('kit.close')} onClick={onClose}>
              <CloseIcon />
            </IconButton>
          </Stack>
          {children}
        </Box>
      )}
    </Drawer>
  );
}
