import FormControlLabel from '@mui/material/FormControlLabel';
import MenuItem from '@mui/material/MenuItem';
import Popover from '@mui/material/Popover';
import Stack from '@mui/material/Stack';
import Switch from '@mui/material/Switch';
import TextField from '@mui/material/TextField';
import { useTranslation } from 'react-i18next';
import { LANGUAGE_NAMES, SUPPORTED_LANGUAGES, isLanguage } from '../i18n-config';
import { useUiSettings } from '../settings/UiSettings';
import type { ThemeMode } from '../settings/storage';

const MODES: ThemeMode[] = ['light', 'dark', 'system'];

export function SettingsPopover({ anchorEl, onClose }: { anchorEl: HTMLElement | null; onClose: () => void }) {
  const { t } = useTranslation();
  const { settings, update } = useUiSettings();
  return (
    <Popover open={anchorEl !== null} anchorEl={anchorEl} onClose={onClose} anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}>
      <Stack gap={2} sx={{ p: 2, minInlineSize: 260 }} role="group" aria-label={t('menu.settings')}>
        <TextField select label={t('theme.label')} value={settings.mode} onChange={(e) => update({ mode: e.target.value as ThemeMode })}>
          {MODES.map((m) => (
            <MenuItem key={m} value={m}>
              {t(`theme.${m}`)}
            </MenuItem>
          ))}
        </TextField>
        <TextField
          select
          label={t('lang.label')}
          value={settings.lang}
          onChange={(e) => {
            if (isLanguage(e.target.value)) update({ lang: e.target.value });
          }}
        >
          {SUPPORTED_LANGUAGES.map((l) => (
            <MenuItem key={l} value={l} lang={l}>
              {LANGUAGE_NAMES[l]}
            </MenuItem>
          ))}
        </TextField>
        <FormControlLabel
          control={<Switch checked={settings.persianDigits} onChange={(e) => update({ persianDigits: e.target.checked })} />}
          label={t('digits.persian')}
        />
        <FormControlLabel control={<Switch checked={settings.dense} onChange={(e) => update({ dense: e.target.checked })} />} label={t('density.dense')} />
      </Stack>
    </Popover>
  );
}
