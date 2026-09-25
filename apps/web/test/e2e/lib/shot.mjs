// apps/web/test/e2e/lib/shot.mjs — screenshot naming and capture, shared by shots.mjs and flow.e2e.mjs.
import { join } from 'node:path';

/** `<slug>-<lang>-<theme>-<step>.png` — the one naming convention every screen's evidence uses (WEB-3). */
export function shotName({ slug, lang, theme = 'light', step }) {
  return `${slug}-${lang}-${theme}-${step}`;
}

/**
 * Screenshots `page` to `<outDir>/<name>.png` after transitions settle, and logs the rendered `<html dir/lang>` plus
 * the current URL (relative to `base`, when given) so a run's console output alone documents what was captured.
 */
export async function shot(page, outDir, name, { base, settleMs = 400 } = {}) {
  await page.waitForTimeout(settleMs);
  const path = join(outDir, `${name}.png`);
  await page.screenshot({ path });
  const dir = await page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);
  const url = base ? page.url().replace(base, '') : page.url();
  console.log(`shot ${name}.png  (${dir})  ${url}`);
  return path;
}
