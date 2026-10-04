#!/usr/bin/env node
// Real-browser factory wizard preview/screenshots: no endpoint mocks and no commit/NIC changes.
// Requires existing WAN/LAN names, real factory API, admin password file, Playwright/Chrome paths.
import { readFileSync, mkdirSync } from 'node:fs';
import { randomBytes } from 'node:crypto';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchBrowser, newPage } from '../lib/browser.mjs';
import { login } from '../lib/auth.mjs';
import { createTranslator } from '../lib/locales.mjs';
import { shot } from '../lib/shot.mjs';

const webRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../..');
const base = process.env.NGFW_E2E_BASE;
const wan = process.env.NGFW_E2E_SETUP_WAN;
const lan = process.env.NGFW_E2E_SETUP_LAN;
const passwordFile = process.env.NGFW_E2E_ADMIN_PASSWORD_FILE;
if (!base || !wan || !lan || !passwordFile) throw new Error('Set NGFW_E2E_BASE, NGFW_E2E_SETUP_WAN/LAN and NGFW_E2E_ADMIN_PASSWORD_FILE');
const current = readFileSync(passwordFile, 'utf8').trim();
const langs = ['en', 'fa']; const tr = createTranslator(webRoot, langs);
const out = resolve(process.env.NGFW_E2E_SHOTS ?? '/tmp/ngfw-setup-shots'); mkdirSync(out, { recursive: true });
const browser = await launchBrowser();
try {
  for (const lang of langs) {
    const { page, ctx, assertNoPageErrors } = await newPage(browser, { lang });
    try {
      await page.goto(`${base}/login`); await login(page, tr, lang, process.env.NGFW_E2E_ADMIN_USER ?? 'admin', current);
      await page.waitForURL('**/system/setup');
      const t = (key) => tr(lang, `setup:${key}`);
      const capture = async (step) => {
        const expected = lang === 'fa' ? 'rtl' : 'ltr';
        if (await page.locator('html').getAttribute('dir') !== expected) throw new Error('wrong language direction');
        await shot(page, out, `setup-${lang}-light-${step}`, { base });
      };
      const next = () => page.getByRole('button', { name: t('next'), exact: true }).click();
      const select = async (key, name) => { await page.getByRole('combobox', { name: t(key), exact: true }).click(); await page.getByRole('option', { name, exact: true }).click(); };
      await page.getByRole('heading', { name: t('title') }).waitFor();
      await capture('1-time'); await next();
      await page.getByLabel(t('current'), { exact: true }).fill(current);
      await page.getByLabel(t('newPassword'), { exact: true }).fill(randomBytes(24).toString('base64url'));
      await capture('2-password'); await next();
      await page.getByLabel(t('hostname'), { exact: true }).fill(process.env.NGFW_E2E_SETUP_HOSTNAME ?? 'ngfw');
      await capture('3-hostname'); await next();
      await select('wan', wan); await capture('4-wan'); await next();
      await select('lan', lan); await page.getByLabel(t('lanAddress'), { exact: true }).fill(process.env.NGFW_E2E_SETUP_LAN_ADDRESS ?? '192.168.40.1/24');
      await capture('5-lan'); await next(); await capture('6-defaults'); await next();
      await page.getByRole('button', { name: t('commit'), exact: true }).waitFor(); await capture('7-summary');
      assertNoPageErrors(); console.log(`PASS setup ${lang}: seven real-API preview steps; no configuration applied`);
    } finally { await ctx.close(); }
  }
} finally { await browser.close(); }
