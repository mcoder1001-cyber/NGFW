// apps/web/test/e2e/screens/users.mjs — WEB-3 proof screen: System > Users (P07b).
export default async function users(ctx) {
  const { page, t, nav, shot, check } = ctx;
  await nav('users');
  const heading = page.getByRole('heading', { level: 2, name: t('users:title') });
  await heading.waitFor();
  await page.waitForTimeout(500);
  check(await heading.isVisible(), `[${ctx.lang}/${ctx.theme}] users heading is visible`);
  await shot('1-list');
}
