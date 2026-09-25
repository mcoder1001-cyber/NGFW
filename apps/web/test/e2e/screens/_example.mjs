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
 * @param {(msg: string) => void} ctx.ok                    records a passing check ("ok   <msg>")
 * @param {(cond: boolean, msg: string) => void} ctx.check  same, but throws (fails the whole run) if `cond` is false
 */
export default async function example(ctx) {
  const { page, t, nav, shot, check } = ctx;

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
  //    last segment.
  await shot('1-loaded');
}
