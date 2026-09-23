import { createTheme, type Direction, type PaletteMode, type Theme } from '@mui/material/styles';

/** Semantic status colours used by every interface/tunnel/neighbour chip in the product. */
export interface VrxStatusTokens {
  up: string;
  down: string;
  degraded: string;
  adminDown: string;
}

declare module '@mui/material/styles' {
  interface Theme {
    vrx: { status: VrxStatusTokens };
  }
  interface ThemeOptions {
    vrx?: { status: VrxStatusTokens };
  }
}

const status: Record<PaletteMode, VrxStatusTokens> = {
  light: { up: '#1b7f3b', down: '#b3261e', degraded: '#b26a00', adminDown: '#5f6368' },
  dark: { up: '#5dd39e', down: '#ff6b6b', degraded: '#ffb454', adminDown: '#9aa0a6' },
};

/** The one theme. `direction` follows the UI language (fa → rtl). */
export function createVrxTheme(mode: PaletteMode, direction: Direction = 'ltr'): Theme {
  return createTheme({
    direction,
    palette: { mode, primary: { main: mode === 'light' ? '#1f3864' : '#8ab4f8' } },
    spacing: 8,
    shape: { borderRadius: 6 },
    typography: { fontFamily: '"Inter", "Vazirmatn", system-ui, sans-serif', fontSize: 13 },
    components: {
      MuiTable: { defaultProps: { size: 'small' } },
      MuiButton: { defaultProps: { size: 'small' } },
      MuiTextField: { defaultProps: { size: 'small' } },
    },
    vrx: { status: status[mode] },
  });
}
