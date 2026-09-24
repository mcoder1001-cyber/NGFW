import { createTheme, type Direction, type PaletteMode, type Theme } from '@mui/material/styles';

/** Network status vocabulary used by every interface / tunnel / neighbour indicator in the product. */
export type VrxStatus = 'up' | 'down' | 'degraded' | 'adminDown';

/** Semantic status colours — one hex per status. Read them through `theme.vrx.status.<status>`. */
export interface VrxStatusTokens {
  up: string;
  down: string;
  degraded: string;
  adminDown: string;
}

export interface VrxThemeTokens {
  status: VrxStatusTokens;
  /** Row height used by dense tables and the data grid. */
  denseRowHeight: number;
  /** Monospace stack for addresses, MACs, counters and CLI output. */
  monoFontFamily: string;
}

declare module '@mui/material/styles' {
  interface Theme {
    vrx: VrxThemeTokens;
  }
  interface ThemeOptions {
    vrx?: VrxThemeTokens;
  }
}

/**
 * Status tokens per mode. Every pair (token on `background.paper` of that mode) is checked by
 * `createVrxTheme.test.ts` to keep at least WCAG AA contrast for UI components (≥ 3:1) and the text
 * colours ≥ 4.5:1 — do not change a colour without running that test.
 */
export const VRX_STATUS_COLOURS: Record<PaletteMode, VrxStatusTokens> = {
  light: { up: '#1b7f3b', down: '#b3261e', degraded: '#9a5b00', adminDown: '#5f6368' },
  dark: { up: '#5dd39e', down: '#ff6b6b', degraded: '#ffb454', adminDown: '#9aa0a6' },
};

export const VRX_STATUSES: readonly VrxStatus[] = ['up', 'down', 'degraded', 'adminDown'];

const FONT_FAMILY = '"Inter", "Vazirmatn", system-ui, -apple-system, "Segoe UI", Roboto, sans-serif';
const MONO_FAMILY = '"JetBrains Mono", "Fira Mono", ui-monospace, SFMono-Regular, Menlo, monospace';

export interface CreateVrxThemeOptions {
  /** Dense layout is the default for a network appliance; `false` gives regular MUI sizing. */
  dense?: boolean;
}

/**
 * The one theme. `direction` follows the UI language (fa → rtl). Dense by default: small controls,
 * 8 px spacing grid, compact tables. Semantic status colours live under `theme.vrx.status`.
 */
export function createVrxTheme(
  mode: PaletteMode,
  direction: Direction = 'ltr',
  { dense = true }: CreateVrxThemeOptions = {},
): Theme {
  const size = dense ? 'small' : 'medium';
  const denseRowHeight = dense ? 36 : 52;
  return createTheme({
    direction,
    cssVariables: false,
    palette: {
      mode,
      primary: { main: mode === 'light' ? '#1f3864' : '#8ab4f8' },
      secondary: { main: mode === 'light' ? '#0b6e7f' : '#7fd1de' },
      background:
        mode === 'light'
          ? { default: '#f4f6f8', paper: '#ffffff' }
          : { default: '#0f1216', paper: '#171b21' },
    },
    spacing: 8,
    shape: { borderRadius: 6 },
    typography: {
      fontFamily: FONT_FAMILY,
      fontSize: dense ? 13 : 14,
      button: { textTransform: 'none', fontWeight: 600 },
    },
    components: {
      MuiTable: { defaultProps: { size } },
      MuiButton: { defaultProps: { size } },
      MuiIconButton: { defaultProps: { size } },
      MuiTextField: { defaultProps: { size, variant: 'outlined' } },
      MuiFormControl: { defaultProps: { size } },
      MuiSelect: { defaultProps: { size } },
      MuiChip: { defaultProps: { size } },
      MuiSwitch: { defaultProps: { size } },
      MuiCheckbox: { defaultProps: { size } },
      MuiRadio: { defaultProps: { size } },
      MuiListItemButton: { defaultProps: { dense } },
      MuiList: { defaultProps: { dense } },
      MuiMenuItem: { defaultProps: { dense } },
      MuiTooltip: { defaultProps: { arrow: true } },
      MuiCssBaseline: {
        styleOverrides: {
          // Visible focus ring for keyboard users, in both themes.
          ':focus-visible': {
            outline: `2px solid ${mode === 'light' ? '#1f3864' : '#8ab4f8'}`,
            outlineOffset: 2,
          },
        },
      },
    },
    vrx: { status: VRX_STATUS_COLOURS[mode], denseRowHeight, monoFontFamily: MONO_FAMILY },
  });
}
