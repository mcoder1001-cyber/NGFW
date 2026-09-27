import Alert from '@mui/material/Alert';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Stack from '@mui/material/Stack';
import Typography from '@mui/material/Typography';
import { useTranslation } from 'react-i18next';

/** F-aaa-login: the single-use recovery codes, shown once right after a factor is enabled. */
export function RecoveryCodes({ codes, onDone }: { codes: string[]; onDone: () => void }) {
  const { t } = useTranslation('aaa');
  return (
    <Stack gap={2} data-testid="recovery-codes">
      <Typography component="h2" variant="h6">
        {t('recovery.title')}
      </Typography>
      <Alert severity="warning">{t('recovery.intro')}</Alert>
      <Box
        component="ul"
        dir="ltr"
        sx={{ columns: 2, fontFamily: (th) => th.vrx.monoFontFamily, m: 0 }}
      >
        {codes.map((c) => (
          <li key={c}>{c}</li>
        ))}
      </Box>
      <Stack direction="row" gap={1}>
        <Button onClick={() => void navigator.clipboard?.writeText(codes.join('\n'))}>
          {t('recovery.copy')}
        </Button>
        <Button variant="contained" onClick={onDone}>
          {t('recovery.done')}
        </Button>
      </Stack>
    </Stack>
  );
}
