import type { Language } from '../i18n-config';

export type ThemeMode = 'light' | 'dark' | 'system';

export interface UiSettings {
  mode: ThemeMode;
  lang: Language;
  /** Render digits as Persian (۰–۹) in numbers and dates. */
  persianDigits: boolean;
  dense: boolean;
}

export const DEFAULT_SETTINGS: UiSettings = { mode: 'system', lang: 'en', persianDigits: false, dense: true };
const STORAGE_KEY = 'vrx.ui.settings';

export function loadSettings(): UiSettings {
  try {
    const raw = globalThis.localStorage?.getItem(STORAGE_KEY);
    if (!raw) return DEFAULT_SETTINGS;
    const parsed = JSON.parse(raw) as Partial<UiSettings>;
    return {
      mode: parsed.mode === 'light' || parsed.mode === 'dark' || parsed.mode === 'system' ? parsed.mode : DEFAULT_SETTINGS.mode,
      lang: parsed.lang === 'fa' || parsed.lang === 'en' ? parsed.lang : DEFAULT_SETTINGS.lang,
      persianDigits: typeof parsed.persianDigits === 'boolean' ? parsed.persianDigits : DEFAULT_SETTINGS.persianDigits,
      dense: typeof parsed.dense === 'boolean' ? parsed.dense : DEFAULT_SETTINGS.dense,
    };
  } catch {
    return DEFAULT_SETTINGS;
  }
}

export function saveSettings(settings: UiSettings): void {
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, JSON.stringify(settings));
  } catch {
    // private mode / quota: settings simply do not persist
  }
}
