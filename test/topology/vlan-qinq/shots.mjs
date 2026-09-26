// test/topology/vlan-qinq/shots.mjs — F-vlan-qinq evidence: screenshots (en, fa/RTL) of the interface drawer's
// sub-interface table and the QinQ edit dialog against the real stack (TestVlanQinqScreenshots starts it).
//
//   node shots.mjs <baseUrl> <outDir> <adminPasswordFile> <parent>
//
// Nothing is installed: VRX_PLAYWRIGHT_CORE is the path of an existing playwright-core package (the npx cache) and
// VRX_CHROME a Chrome-for-Testing headless shell (its libraries via LD_LIBRARY_PATH), as in P07a/P07b/P08. Test code
// only — not part of the product and not in any pnpm package.
import { mkdirSync, readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const require = createRequire(import.meta.url);
const { chromium } = require(process.env.VRX_PLAYWRIGHT_CORE);
const [BASE, OUT, PW_FILE, PARENT] = process.argv.slice(2);
const PW = readFileSync(PW_FILE, 'utf8').trim();
const LOCALES = join(dirname(fileURLToPath(import.meta.url)), '../../../apps/web/src/locales');
const L = (lang, ns) => JSON.parse(readFileSync(join(LOCALES, lang, `${ns}.json`), 'utf8'));
const fill = (s, vars) => s.replace(/\{\{(\w+)\}\}/g, (_, k) => vars[k]);
const flat = (s) => s.replace(/\s+/g, ' ').trim();
mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({ executablePath: process.env.VRX_CHROME, args: ['--no-sandbox'] });
let failures = 0;
for (const lang of ['en', 'fa']) {
  const auth = L(lang, 'auth');
  const ifs = L(lang, 'interfaces');
  const vq = L(lang, 'vlan-qinq');
  const ctx = await browser.newContext({ viewport: { width: 1440, height: 1250 } });
  await ctx.addInitScript((s) => localStorage.setItem('vrx.ui.settings', JSON.stringify(s)), {
    mode: 'light',
    lang,
    persianDigits: false,
    dense: true,
  });
  const page = await ctx.newPage();
  const errors = [];
  page.on('pageerror', (e) => errors.push(String(e)));
  await page.goto(`${BASE}/login`);
  await page.getByLabel(auth.username, { exact: false }).fill('admin');
  await page.getByLabel(auth.passwordLabel, { exact: false }).fill(PW);
  await page.getByRole('button', { name: auth.signIn }).click();
  await page.waitForURL((u) => !u.pathname.startsWith('/login'));
  await page.goto(`${BASE}/interfaces`);
  await page.getByRole('grid').getByText(PARENT, { exact: true }).first().click();
  const drawer = page.getByRole('region', { name: fill(ifs.drawer.label, { name: PARENT }) });
  const table = drawer.getByRole('table', { name: fill(vq.tableLabel, { parent: PARENT }) });
  await table.waitFor();
  await table.getByRole('status').first().waitFor(); // live state arrived (admin/link chips)
  await table.scrollIntoViewIfNeeded();
  await page.waitForTimeout(800);
  const rows = (await table.locator('tbody tr').allInnerTexts()).map(flat);
  const dir = await page.evaluate(() => `${document.documentElement.dir}/${document.documentElement.lang}`);
  const shot = `vlan-qinq-drawer-${lang}${lang === 'fa' ? '-rtl' : ''}.png`;
  await page.screenshot({ path: join(OUT, shot) });
  // close-up of the table (+ its heading and the exact-match caption): a clip of the page, because the drawer paper
  // scrolls on its own and an element screenshot would clip the wrong region
  const box = await table.boundingBox();
  if (box) {
    await page.screenshot({
      path: join(OUT, `vlan-qinq-table-${lang}.png`),
      clip: { x: Math.max(0, box.x - 12), y: Math.max(0, box.y - 52), width: box.width + 24, height: box.height + 100 },
    });
  }
  console.log(`${shot}  html dir/lang=${dir}  rows=${JSON.stringify(rows)}  pageErrors=${errors.length}`);
  if (lang === 'en') {
    // the edit dialog of the QinQ row: the generated SubinterfaceSchema form (encapsulation: VLAN ID, inner VLAN, 802.1ad)
    await table.getByText(`${PARENT}.200`, { exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.waitFor();
    await page.waitForTimeout(600);
    await page.screenshot({ path: join(OUT, 'vlan-qinq-dialog-en.png') });
    console.log(`vlan-qinq-dialog-en.png  title=${JSON.stringify(flat(await dialog.getByRole('heading').first().innerText()))}  pageErrors=${errors.length}`);
    await page.keyboard.press('Escape');
  }
  if (errors.length > 0) {
    console.log(`[${lang}] page errors: ${JSON.stringify(errors)}`);
    failures++;
  }
  await ctx.close();
}
await browser.close();
process.exit(failures > 0 ? 1 : 0);
