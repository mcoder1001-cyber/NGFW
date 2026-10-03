import { CacheProvider } from '@emotion/react';
import CssBaseline from '@mui/material/CssBaseline';
import { ThemeProvider, type Direction, type PaletteMode } from '@mui/material/styles';
import { useEffect, useMemo, type ReactNode } from 'react';
import { createNgfwTheme, type CreateNgfwThemeOptions } from './createNgfwTheme.js';
import { createNgfwEmotionCache } from './rtl.js';

export interface NgfwThemeProviderProps extends CreateNgfwThemeOptions {
  mode: PaletteMode;
  /** BCP 47 language tag, written to `<html lang>`. */
  lang: string;
  /** Text direction, written to `<html dir>` and used for the Emotion RTL cache. */
  dir: Direction;
  children: ReactNode;
}

/**
 * Theme + Emotion cache + CssBaseline in one place. Keeps `<html dir lang>` in sync with the selected
 * language so native controls, scrollbars and screen readers follow the UI direction.
 */
export function NgfwThemeProvider({ mode, lang, dir, dense, children }: NgfwThemeProviderProps) {
  const cache = useMemo(() => createNgfwEmotionCache(dir), [dir]);
  const theme = useMemo(
    () => createNgfwTheme(mode, dir, dense === undefined ? {} : { dense }),
    [mode, dir, dense],
  );
  useEffect(() => {
    const html = document.documentElement;
    html.setAttribute('dir', dir);
    html.setAttribute('lang', lang);
    html.style.colorScheme = mode;
  }, [dir, lang, mode]);
  return (
    <CacheProvider value={cache}>
      <ThemeProvider theme={theme}>
        <CssBaseline enableColorScheme />
        {children}
      </ThemeProvider>
    </CacheProvider>
  );
}
