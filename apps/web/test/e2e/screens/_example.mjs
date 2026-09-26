// apps/web/test/e2e/screens/_example.mjs — the pattern every feature copies (WEB-3). Copy this file to
// `screens/<your-slug>.mjs`, keep the single default export, and point shots.mjs at it:
//
//   node apps/web/test/e2e/shots.mjs --base http://127.0.0.1:<your-slot-web-port> \
//     --screens <your-slug> --out <dir> --langs en,fa
//
// shots.mjs calls this function once per (language x theme) requested on --langs/--themes, with a fresh, already
// signed-in `page` reused across every theme for one language (setTheme() runs between calls — no reload, no
// re-login). Do not call login()/newPage() yourself: shots.mjs owns the session.

/**
 * @param {object} ctx
 * @param {import('playwright-core').Page} ctx.page   the live page, already signed in
 * @param {string} ctx.lang                            'en' | 'fa' — the CURRENT UI language for this call
 * @param {string} ctx.theme                           the CURRENT theme for this call (default run: 'light')
 * @param {string} ctx.base                            the harness's --base, for relative-URL logging if you need it
 * @param {(key: string, vars?: object) => string} ctx.t   the app's OWN string for "namespace:dotted.path" — never
 *                                                          hand-type an English or Persian label, look it up so a
 *                                                          copy change cannot silently break the script
 * @param {(navKey: string) => Promise<void>} ctx.nav      clicks the left-nav entry `nav:<navKey>`, opening its
 *                                                          collapsed group first if needed (ui-nav-collapse, D-117)
 * @param {(step: string) => Promise<string>} ctx.shot     screenshot named `<slug>-<lang>-<theme>-<step>.png`
 *                                                          (also asserts `<html dir>` matches `ctx.lang`)
 * @param {(mode: 'light'|'dark'|'system') => Promise<void>} ctx.setTheme     runtime theme toggle, no reload
 * @param {(newLang: 'en'|'fa') => Promise<void>} ctx.setLanguage             runtime language toggle, no reload
 * @param {(msg: string) => void} ctx.ok                    records a passing check ("ok   <msg>")
 * @param {(cond: boolean, msg: string) => void} ctx.check  same, but throws (fails the whole run) if `cond` is false
 */
export default async function example(ctx) {
  const { page, lang, t, nav, shot, setTheme, setLanguage, check } = ctx;

  // 1. Navigate with the nav key from apps/web/src/nav/nav.ts (the `nav:<key>` label) — never page.goto() a domain
  //    screen directly, so the harness also proves the left nav (and its collapsed-group handling) still works.
  await nav('dashboard'); // replace with your screen's key, e.g. 'users', or a feature's own domain key

  // 2. Wait for something that means the screen actually rendered (a heading, a grid, a testid) — never a fixed
  //    sleep alone. PageHeader renders an <h2>, so most domain/system screens share this pattern:
  const heading = page.getByRole('heading', { level: 2, name: t('common:dashboard.title') });
  await heading.waitFor();

  // 3. A check ties the screenshot to something the harness actually verified (optional, but cheap and worth it):
  check(await heading.isVisible(), `[${ctx.lang}/${ctx.theme}] dashboard heading is visible`);

  // 4. Shoot. One call per interesting state (a dialog open, a row selected, ...); `step` becomes the filename's
  //    last segment. `shot()` also asserts `<html dir>` matches `ctx.lang` (rtl for fa, ltr otherwise) — a
  //    regression that left the wrong direction would fail here, not just look wrong in a screenshot a human
  //    happens to open (WEB-3 review L3).
  await shot('1-loaded');

  // 5. The runtime toggles (lib/theme.mjs) exist so a screen with expensive setup can be shot in every combination
  //    inside one signed-in session — prove they actually do something, don't just trust the click (review M2):
  //    `VrxThemeProvider` (packages/ui-kit) keeps `document.documentElement.style.colorScheme` equal to the
  //    resolved theme mode, so that is the ground truth for "did setTheme(...) really take effect". Toggle to
  //    whichever mode is NOT already active — with `--themes light,dark` this function also runs once already
  //    in 'dark' (shots.mjs sets it before calling us), so hardcoding a target here would be a no-op on that pass.
  const colorSchemeBefore = await page.evaluate(() => document.documentElement.style.colorScheme);
  const target = colorSchemeBefore === 'dark' ? 'light' : 'dark';
  await setTheme(target);
  const colorSchemeAfter = await page.evaluate(() => document.documentElement.style.colorScheme);
  check(colorSchemeAfter === target && colorSchemeAfter !== colorSchemeBefore, `[${lang}] setTheme('${target}') set <html style.colorScheme> to "${colorSchemeAfter}" (was "${colorSchemeBefore}")`);
  await shot(`2-theme-${target}`);
  await setTheme(ctx.theme); // restore this pass's own theme (shots.mjs reuses one page across --themes for one lang)

  // setLanguage() flips <html dir> at runtime — the same thing ui-nav-collapse's Settings popover does for a real
  // user, just driven by the harness. Restore the original language afterward: this page is reused for the next
  // --themes iteration, and ctx.shot()'s own dir check (point 4 above) asserts against ctx.lang, not whatever the
  // page happens to show.
  const otherLang = lang === 'fa' ? 'en' : 'fa';
  const dirBefore = await page.evaluate(() => document.documentElement.dir);
  await setLanguage(otherLang);
  const dirAfter = await page.evaluate(() => document.documentElement.dir);
  check(dirAfter !== dirBefore, `[${lang}] setLanguage('${otherLang}') flipped <html dir> ("${dirBefore}" -> "${dirAfter}")`);
  await setLanguage(lang); // restore
}
