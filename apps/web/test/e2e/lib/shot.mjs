// apps/web/test/e2e/lib/shot.mjs — screenshot naming and capture, shared by shots.mjs and flow.e2e.mjs.
import { join } from 'node:path';

/** `<slug>-<lang>-<theme>-<step>.png` — the one naming convention every screen's evidence uses (WEB-3). */
export function shotName({ slug, lang, theme = 'light', step }) {
  return `${slug}-${lang}-${theme}-${step}`;
}

/** `apps/web/src/i18n-config.ts`'s RTL languages — kept as a small local list (not imported: this is a plain script,
 * the app is a TS/Vite source tree) since only `fa` exists today and `locales.mjs` already fails loudly on a typo'd
 * language directory. */
const RTL_LANGS = new Set(['fa']);

/**
 * Screenshots `page` to `<outDir>/<name>.png` after transitions settle, and logs the rendered `<html dir/lang>` plus
 * the current URL (relative to `base`, when given) so a run's console output alone documents what was captured.
 *
 * Pass `lang` and `check` (a checklist's `check`, see `lib/checklist.mjs`) to turn the logged `dir` into a real,
 * run-failing assertion instead of something only a human reading the log would notice: review finding L3 — a
 * regression that left `dir="ltr"` under `fa` would previously pass silently. Both are optional so `flow.e2e.mjs`
 * (which has its own 43 fixed checks, not to be perturbed by this) can keep calling `shot()` without either.
 */
export async function shot(page, outDir, name, { base, settleMs = 400, lang, check } = {}) {
  await page.waitForTimeout(settleMs);
  const path = join(outDir, `${name}.png`);
  await page.screenshot({ path });
  const dir = await page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);
  const url = base ? page.url().replace(base, '') : page.url();
  console.log(`shot ${name}.png  (${dir})  ${url}`);
  if (lang && check) {
    const expected = RTL_LANGS.has(lang) ? 'rtl' : 'ltr';
    const actual = dir.split('/')[0];
    check(actual === expected, `${name}.png: <html dir="${actual}"> matches ${lang} (expected "${expected}")`);
  }
  return path;
}
