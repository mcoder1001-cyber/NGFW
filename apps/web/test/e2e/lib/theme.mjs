// apps/web/test/e2e/lib/theme.mjs — runtime language/theme toggles through the app's own Settings popover
// (`apps/web/src/shell/SettingsPopover.tsx`), never a `localStorage` write + reload: a script can shoot the same
// screen in every lang/theme combination inside one signed-in session, which matters once a screen's setup is
// expensive (e.g. seeding thousands of NAT sessions) — logging in again per combination would multiply that cost.
//
// `lang` is the CURRENT UI language at the time of the call (used to find the Settings button and its menu items);
// after `setLanguage`, pass the NEW language to every subsequent `tr()`/helper call.

async function openSettings(page, tr, lang) {
  await page.getByRole('button', { name: tr(lang, 'common:menu.settings') }).click();
  return page.getByRole('group', { name: tr(lang, 'common:menu.settings') });
}

/** `mode`: 'light' | 'dark' | 'system'. */
export async function setTheme(page, tr, lang, mode) {
  const group = await openSettings(page, tr, lang);
  await group.getByLabel(tr(lang, 'common:theme.label'), { exact: false }).click();
  await page.getByRole('option', { name: tr(lang, `common:theme.${mode}`) }).click();
  await page.keyboard.press('Escape');
}

/** `newLang`: 'en' | 'fa'. The select's options are language names (`LANGUAGE_NAMES`), matched by the option's own
 * `lang` attribute (`apps/web/src/shell/SettingsPopover.tsx` sets `<MenuItem lang={l}>`) rather than a translated
 * string, since the name of e.g. Persian is not itself translated per-UI-language. */
export async function setLanguage(page, tr, lang, newLang) {
  const group = await openSettings(page, tr, lang);
  await group.getByLabel(tr(lang, 'common:lang.label'), { exact: false }).click();
  await page.locator(`[role="option"][lang="${newLang}"]`).click();
  await page.keyboard.press('Escape');
}
