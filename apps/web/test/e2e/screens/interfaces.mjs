// apps/web/test/e2e/screens/interfaces.mjs — WEB-3 proof screen: the Interfaces grid (P07a/P08).
export default async function interfaces(ctx) {
  const { page, t, nav, shot, check } = ctx;
  await nav('interfaces');
  const heading = page.getByRole('heading', { level: 2, name: t('interfaces:title') });
  await heading.waitFor();
  const grid = page.getByRole('grid', { name: t('interfaces:title') });
  await grid.waitFor();
  await page.waitForTimeout(800); // live counters (ws stream / initial poll) settle
  check(await grid.isVisible(), `[${ctx.lang}/${ctx.theme}] interfaces grid is visible`);
  await shot('1-grid');
}
