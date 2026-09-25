// apps/web/test/e2e/lib/auth.mjs — sign in / sign out through the real login form and user menu (never a token
// shortcut: the point of a browser harness is to exercise what a person actually clicks).
export async function login(page, tr, lang, user, password) {
  await page.getByLabel(tr(lang, 'auth:username'), { exact: false }).fill(user);
  await page.getByLabel(tr(lang, 'auth:passwordLabel'), { exact: false }).fill(password);
  await page.getByRole('button', { name: tr(lang, 'auth:signIn') }).click();
}

export async function signOut(page, tr, lang) {
  await page.getByTestId('user-menu').click();
  await page.getByRole('menuitem', { name: tr(lang, 'auth:signOut') }).click();
  await page.waitForURL(/\/login/);
}
