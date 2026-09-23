import { useMemo, useState } from 'react';
import { CssBaseline, ThemeProvider } from '@mui/material';
import AppBar from '@mui/material/AppBar';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Toolbar from '@mui/material/Toolbar';
import Typography from '@mui/material/Typography';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useTranslation } from 'react-i18next';
import { createVrxTheme } from '@ngfw/ui-kit';
import { RTL_LANGS } from './i18n';
import { HealthCard } from './HealthCard';

const queryClient = new QueryClient();

export function App() {
  const { t, i18n } = useTranslation();
  const [mode, setMode] = useState<'light' | 'dark'>('light');
  const dir = RTL_LANGS.has(i18n.language) ? 'rtl' : 'ltr';
  const theme = useMemo(() => createVrxTheme(mode, dir), [mode, dir]);
  document.documentElement.setAttribute('dir', dir);
  document.documentElement.setAttribute('lang', i18n.language);

  return (
    <QueryClientProvider client={queryClient}>
      <ThemeProvider theme={theme}>
        <CssBaseline />
        <AppBar position="static">
          <Toolbar>
            <Typography variant="h6" sx={{ flexGrow: 1 }}>
              {t('appName')}
            </Typography>
            <Button color="inherit" onClick={() => setMode(mode === 'light' ? 'dark' : 'light')}>
              {t('theme.toggle')}
            </Button>
            <Button
              color="inherit"
              onClick={() => i18n.changeLanguage(i18n.language === 'fa' ? 'en' : 'fa')}
            >
              {t('lang.toggle')}
            </Button>
          </Toolbar>
        </AppBar>
        <Box component="main" sx={{ p: 2 }}>
          <HealthCard />
        </Box>
      </ThemeProvider>
    </QueryClientProvider>
  );
}
